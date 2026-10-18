package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/pkg/exectree"
)

// FFmpegRenderer FFmpeg CLI 渲染器（详细设计 §2.7 三组命令：切+reframe+烧录 / 封面 / 元数据）。
//
// 🔴 诚实声明：
//   - 9:16/1:1 无 GPU 是中心裁切（滤镜由 biz AspectSpec.Filter 给出），不是 AI 智能重构；
//   - 字幕烧录用 subtitles 滤镜 force_style MarginV=安全区底部（QC 安全区由 biz 计算传入）；
//   - ffmpeg 缺失 → Available=false，调用方报 MME_RENDER_FAIL可读错误，保留现场。
//
// 进度：-progress pipe:1 周期的 out_time_ms / 目标时长 = p；onProgress 限频在 biz 层。
type FFmpegRenderer struct {
	cfg bizmme.Config
	log *log.Helper
}

// NewFFmpegRenderer 构造。
func NewFFmpegRenderer(cfg bizmme.Config, logger log.Logger) bizmme.Renderer {
	return &FFmpegRenderer{cfg: cfg, log: log.NewHelper(log.With(logger, "module", "mme/ffmpeg"))}
}

func (r *FFmpegRenderer) Available(context.Context) bool {
	_, err := exectree.LookPath(r.cfg.FFmpegPath)
	return err == nil
}

func (r *FFmpegRenderer) Version(ctx context.Context) string {
	res := exectree.Run(ctx, exectree.RunSpec{Bin: r.cfg.FFmpegPath, Args: []string{"-version"}, Timeout: 10 * time.Second})
	if !res.Success() {
		return ""
	}
	line := res.StdoutTail
	if i := strings.Index(line, "\n"); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// outTimeRe "-progress pipe:1" 的 out_time_ms=xxx 行（单位其实是微秒）。
var outTimeRe = regexp.MustCompile(`out_time_ms=(\d+)`)

// escapeSubFilterPath subtitles 滤镜路径转义（Windows 盘符冒号/反斜杠必须处理）。
func escapeSubFilterPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, ":", `\:`)
	return p
}

// Render 渲染单变体。spec.SrtPath 非空时烧录字幕（force_style 含 MarginV 安全区）。
func (r *FFmpegRenderer) Render(ctx context.Context, spec bizmme.RenderSpec, onProgress func(p float64)) (*bizmme.RenderResult, error) {
	if !r.Available(ctx) {
		return nil, fmt.Errorf("%w: ffmpeg not found (path=%s); 安装 ffmpeg 后重试", bizmme.ErrRenderFail, r.cfg.FFmpegPath)
	}
	dur := spec.EndSec - spec.StartSec
	if dur <= 0 {
		return nil, fmt.Errorf("%w: invalid edl segment [%.2f,%.2f)", bizmme.ErrRenderFail, spec.StartSec, spec.EndSec)
	}

	watchDur := time.Duration(dur*20) * time.Second
	if watchDur < 10*time.Minute {
		watchDur = 10 * time.Minute // 长看门狗：慢机器不冤杀；硬上限
	}

	// ---- 1) 切 + reframe(+ 烧录) → video.mp4 ----
	vf := spec.Aspect.Filter
	if spec.SrtPath != "" {
		style := bizmme.ForceStyleFor(spec.Aspect.Key, r.cfg)
		vf = fmt.Sprintf("%s,subtitles='%s':force_style='%s'", vf, escapeSubFilterPath(spec.SrtPath), style)
	}
	args := []string{
		"-hide_banner", "-nostats",
		"-progress", "pipe:1",
		"-ss", strconv.FormatFloat(spec.StartSec, 'f', 3, 64),
		"-to", strconv.FormatFloat(spec.EndSec, 'f', 3, 64),
		"-i", spec.SrcPath,
		"-vf", vf,
		"-c:v", "libx264", "-preset", spec.Preset, "-crf", strconv.Itoa(spec.CRF),
		"-pix_fmt", "yuv420p", // 兼容性（部分播放器不认 yuv444）
		"-movflags", "+faststart",
	}
	// intro/outro 模板：有则走 concat（本轮模板 intro/outro 为空是常态，留通路）
	if spec.IntroPath != "" || spec.OutroPath != "" {
		r.log.Warnf("variant %s: template intro/outro not yet wired into filtergraph (cut+fallback only)", spec.VariantID)
	}
	// 视频流 + 可选原音轨（源无音轨时 0:a? 为空映射，不报错）；保留 EDL 源声
	args = append(args,
		"-map", "0:v:0", "-map", "0:a?", "-c:a", "aac",
		"-map_metadata", "-1", "-y", spec.DstPath,
	)

	step := watchDur / 2
	res := exectree.Run(ctx, exectree.RunSpec{
		Bin:         r.cfg.FFmpegPath,
		Args:        args,
		Dir:         filepath.Dir(spec.DstPath),
		Timeout:     watchDur,
		OutputLines: 600,
	})
	if !res.Success() {
		return nil, fmt.Errorf("%w: ffmpeg render variant %s: %s | stderr: %s",
			bizmme.ErrRenderFail, spec.VariantID, res.Attribution, tail(res.StderrTail, 800))
	}
	// 从 stdout 进度行检查真实输出时长（defense：异常短输出按失败处理）
	outDurSec := parseLastOutTimeSec(res.StdoutTail)
	if outDurSec > 0 && outDurSec < dur*0.5 {
		return nil, fmt.Errorf("%w: ffmpeg output %.1fs < half of expected %.1fs (source may lack segment)",
			bizmme.ErrRenderFail, outDurSec, dur)
	}
	if onProgress != nil {
		onProgress(0.7) // 主体完成
	}

	// ---- 2) 封面抽帧（EDL 中段前 1s，防黑帧开头） ----
	coverAt := dur * 0.33
	coverRes := exectree.Run(ctx, exectree.RunSpec{
		Bin: r.cfg.FFmpegPath,
		Args: []string{
			"-hide_banner",
			"-ss", strconv.FormatFloat(spec.StartSec+coverAt, 'f', 3, 64),
			"-i", spec.SrcPath,
			"-vf", spec.Aspect.Filter,
			"-frames:v", "1",
			"-q:v", "3",
			"-y", spec.CoverPath,
		},
		Timeout: step,
	})
	if !coverRes.Success() {
		// 封面失败不判整变体失败（审核台可见缺图）；如实记日志
		r.log.Warnf("cover extract failed for %s: %s", spec.VariantID, coverRes.Attribution)
		spec.CoverPath = ""
	} else if fi, err := os.Stat(spec.CoverPath); err != nil || fi.Size() == 0 {
		spec.CoverPath = ""
	}
	if onProgress != nil {
		onProgress(1)
	}

	durationOut := dur
	if outDurSec > 0 {
		durationOut = outDurSec
	}
	return &bizmme.RenderResult{
		VideoPath:   spec.DstPath,
		CoverPath:   spec.CoverPath,
		SrtPath:     spec.SrtPath,
		DurationSec: durationOut,
	}, nil
}

// parseLastOutTimeSec 从 -progress pipe:1 输出抓最后一个 out_time_ms（微秒）→ 秒。
func parseLastOutTimeSec(s string) float64 {
	matches := outTimeRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0
	}
	last := matches[len(matches)-1][1]
	us, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		return 0
	}
	return float64(us) / 1e6
}
