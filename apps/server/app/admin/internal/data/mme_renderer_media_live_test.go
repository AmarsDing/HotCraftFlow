package data

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/pkg/exectree"
)

// 活性委托桥测试：对活 media 服务 + 真 ffmpeg 跑完整委托链
// （自铸 JWT → media 验签 → submit → 真 SSE 进度 → 真 ffmpeg → 结果落地）。
//
// 双门控（缺一不可，常态 CI 全 skip）：
//   - MME_MEDIA_LIVE_URL：活 media 服务地址（如 http://127.0.0.1:7790）；
//   - MEDIA_FFMPEG_PATH 或 PATH 上的 ffmpeg（测试源片用 lavfi 现做）。
//
// 令牌键须与 media 服务一致（MME_MEDIA_LIVE_KEY 覆盖，缺省开发密钥）。
func TestMediaServiceRendererLiveRender(t *testing.T) {
	base := os.Getenv("MME_MEDIA_LIVE_URL")
	if base == "" {
		t.Skip("MME_MEDIA_LIVE_URL 未设置，跳过活性委托测试")
	}
	ffmpeg := os.Getenv("MEDIA_FFMPEG_PATH")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	if _, err := exectree.LookPath(ffmpeg); err != nil {
		t.Skip("ffmpeg 未就位，跳过活性委托测试")
	}
	key := os.Getenv("MME_MEDIA_LIVE_KEY")
	if key == "" {
		key = "CHANGE_ME_DEV_JWT_SECRET" // 与 media config/server.yaml 同钥（开发基线）
	}

	// lavfi 源片 + SRT。
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	res := exectree.Run(t.Context(), exectree.RunSpec{
		Bin: ffmpeg,
		Args: []string{
			"-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=duration=8:size=1280x720:rate=15",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=8",
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "32",
			"-c:a", "aac", "-shortest", "-y", src,
		},
		Timeout: 120 * time.Second,
	})
	if !res.Success() {
		t.Fatalf("lavfi 源片: %s | %s", res.Attribution, res.StderrTail)
	}
	srt := filepath.Join(dir, "src.srt")
	_ = os.WriteFile(srt, []byte("1\n00:00:00,500 --> 00:00:03,000\n活性委托桥验证。\n"), 0o644)

	r := NewMediaServiceRenderer(base, key, log.NewStdLogger(io.Discard))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()

	// doctor 探活：当前宿主上 ffmpeg 版探间歇性崩/挂（gyan gcc16 构建在非控制
	// 台下的已知拆解怪癖，doctor.go 有详注），版探红不代表渲染不可用。故这里
	// 只把 Available 当"服务可达"的信号——真委托渲染在下方 Render 里验；若
	// Render 因 ffmpeg 真不可用而失败，那才是该报的红。
	if !r.Available(ctx) {
		t.Log("警告: media doctor 探活未全绿（常见为 ffmpeg 版探间歇崩/挂；Render 才是真验证）")
	}
	if v := r.Version(ctx); v != "" {
		t.Logf("media doctor ffmpeg version: %s", v)
	}

	sp := bizmme.RenderSpec{
		VariantID: "v_live_bridge",
		SrcPath:   src,
		DstPath:   filepath.Join(dir, "dst.mp4"),
		CoverPath: filepath.Join(dir, "cover.jpg"),
		SrtPath:   srt,
		StartSec:  0,
		EndSec:    6,
		Preset:    "ultrafast",
		CRF:       32,
	}
	sp.Aspect = bizmme.AspectSpec{Key: "9:16", Width: 1080, Height: 1920, Filter: "crop=ih*9/16:ih,scale=1080:1920"}
	sp.Margin = bizmme.SafetyMargin{Bottom: 380}

	var progCount int
	out, err := r.Render(ctx, sp, func(p float64) { progCount++ })
	if err != nil {
		t.Fatalf("委托渲染失败: %v", err)
	}
	if progCount == 0 {
		t.Error("应收到 SSE 进度回调（活性下至少一帧）")
	}
	fi, err := os.Stat(out.VideoPath)
	if err != nil || fi.Size() < 8<<10 {
		t.Fatalf("产物缺失或过小: %v (%v)", err, fi)
	}
	if ci, err := os.Stat(out.CoverPath); err != nil || ci.Size() == 0 {
		t.Fatalf("封面缺失或为空: %v", err)
	}
	if out.DurationSec < 3 || out.DurationSec > 7 {
		t.Fatalf("duration 异常: %.2f（期望 ~6s）", out.DurationSec)
	}
	head := make([]byte, 12)
	f, _ := os.Open(out.VideoPath)
	_, _ = f.Read(head)
	_ = f.Close()
	if string(head[4:8]) != "ftyp" {
		t.Fatalf("输出不是 MP4: %q", head)
	}
}
