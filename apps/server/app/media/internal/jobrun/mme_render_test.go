package jobrun_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"framework/app/media/internal/jobrun"
	"framework/app/media/internal/jobstore"
	"framework/pkg/exectree"
)

// testSchema 本包专属 PG schema（并行包二进制互不误伤；纪律同 jobstore/sseapi 测试）。
const testSchema = "jobrun_mme_test"

// ffmpegBin 从环境/PATH 解析 ffmpeg；缺则返回空串（测试据此 skip）。
func ffmpegBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("MEDIA_FFMPEG_PATH")
	if bin == "" {
		bin = "ffmpeg"
	}
	p, err := exectree.LookPath(bin)
	if err != nil {
		return ""
	}
	return p
}

// wireStack DSN 门控的 store+executor 最小栈（注册真实 mme.render handler）。
func wireStack(t *testing.T, ffmpeg string) *jobrun.Executor {
	t.Helper()
	dsn := os.Getenv("MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("MEDIA_TEST_DSN 未设置，跳过 mme.render 集成测试")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+testSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	_ = admin.Close()

	s, err := jobstore.Open(dsn+" search_path="+testSchema, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	ex := jobrun.New(s, jobrun.Config{
		Lanes:            map[string]int32{"render": 2},
		ProgressMinGapMs: 1,
	}, "mme-render-test", nil)
	ex.RegisterHandler("mme.render", jobrun.MMERenderHandler(jobrun.MMERenderDeps{FFmpegPath: ffmpeg}))
	if err := ex.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = ex.Stop(ctx)
		_ = s.Close()
		if admin, err := sql.Open("pgx", dsn); err == nil {
			_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+testSchema+` CASCADE`)
			_ = admin.Close()
		}
	})
	return ex
}

// mkSource 用 lavfi 生成竖切测试源片 + 两行 SRT，返回 (src, srt)。
func mkSource(t *testing.T, ffmpeg string, seconds int) (string, string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	res := exectree.Run(t.Context(), exectree.RunSpec{
		Bin: ffmpeg,
		Args: []string{
			"-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=duration=" + itoa(seconds) + ":size=1280x720:rate=15",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=" + itoa(seconds),
			"-c:v", "libx264", "-preset", "ultrafast", "-crf", "32",
			"-c:a", "aac", "-shortest", "-y", src,
		},
		Timeout: 120 * time.Second,
	})
	if !res.Success() {
		t.Fatalf("lavfi 源片生成失败: %s | %s", res.Attribution, res.StderrTail)
	}
	srt := filepath.Join(dir, "src.srt")
	content := "1\n00:00:00,500 --> 00:00:03,000\n热榜矩阵渲染验证第一句。\n\n2\n00:00:03,200 --> 00:00:05,500\n第二句：字幕安全区烧录。\n"
	if err := os.WriteFile(srt, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return src, srt
}

func itoa(v int) string { return strconv.Itoa(v) }

// waitTerminal 轮询至终态。
func waitTerminal(t *testing.T, ex *jobrun.Executor, dsn string, id string, timeout time.Duration) jobstore.Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	store := storeOf(t, ex, dsn)
	for time.Now().Before(deadline) {
		j, err := store.Get(t.Context(), id)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if j.State.Terminal() {
			return j
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("等待终态超时（%s 未结束）", id)
	return jobstore.Job{}
}

// storeOf 测试里绕不开 store 句柄：executor 不暴露，重开一条只读连接。
func storeOf(t *testing.T, _ *jobrun.Executor, dsn string) *jobstore.Store {
	t.Helper()
	s, err := jobstore.Open(dsn+" search_path="+testSchema, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// waitByStore 直接以测试自有 store 等终态（cancel 用例不走 storeOf 二次连接）。
func waitByStore(t *testing.T, s *jobstore.Store, id string, timeout time.Duration) jobstore.Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j, err := s.Get(t.Context(), id)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if j.State.Terminal() {
			return j
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("等待终态超时（%s 未结束）", id)
	return jobstore.Job{}
}

func TestMMERenderHappyPath(t *testing.T) {
	ffmpeg := ffmpegBin(t)
	if ffmpeg == "" {
		t.Skip("ffmpeg 未就位（MEDIA_FFMPEG_PATH/PATH），跳过渲染集成测试")
	}
	ex := wireStack(t, ffmpeg)

	src, srt := mkSource(t, ffmpeg, 8)
	dir := t.TempDir()
	dst := filepath.Join(dir, "video.mp4")
	cover := filepath.Join(dir, "cover.jpg")
	payload := map[string]any{
		"variant_id": "v_test_happy",
		"src_path":   src,
		"dst_path":   dst,
		"cover_path": cover,
		"srt_path":   srt,
		"aspect":     map[string]any{"key": "9:16", "width": 1080, "height": 1920, "filter": "crop=ih*9/16:ih,scale=1080:1920"},
		"start_sec":  0,
		"end_sec":    6,
		"margin":     map[string]int{"bottom": 380},
		"preset":     "ultrafast",
		"crf":        32,
	}
	raw, _ := json.Marshal(payload)
	j, _, err := ex.Submit(t.Context(), "render", "mme.render", raw, "mme-render-test-happy")
	if err != nil {
		t.Fatal(err)
	}

	dsn := os.Getenv("MEDIA_TEST_DSN")
	final := waitTerminal(t, ex, dsn, j.ID, 3*time.Minute)
	if final.State != jobstore.StateDone {
		t.Fatalf("期望 done，实际 %s: %s %s", final.State, final.ErrorCode, final.ErrorMsg)
	}

	// 产物断言：视频存在、非 MP4 式微（ftyp box）、尺寸合理。
	fi, err := os.Stat(dst)
	if err != nil || fi.Size() < 8<<10 {
		t.Fatalf("video 缺失或过小: %v (%v)", err, fi)
	}
	head := make([]byte, 12)
	f, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Read(head)
	_ = f.Close()
	if string(head[4:8]) != "ftyp" {
		t.Fatalf("输出不是 MP4（无 ftyp box）: %q", head)
	}
	ci, err := os.Stat(cover)
	if err != nil || ci.Size() == 0 {
		t.Fatalf("cover 缺失或为空: %v", err)
	}

	// result_json 契约断言。
	var result struct {
		VideoPath   string  `json:"video_path"`
		CoverPath   string  `json:"cover_path"`
		DurationSec float64 `json:"duration_sec"`
	}
	if err := json.Unmarshal([]byte(final.ResultJSON), &result); err != nil {
		t.Fatalf("result_json 解析失败: %v", err)
	}
	if result.VideoPath != dst || result.CoverPath != cover {
		t.Fatalf("result 路径不符: %+v", result)
	}
	if result.DurationSec < 3 || result.DurationSec > 7 {
		t.Fatalf("duration_sec 异常: %.2f（期望 ~6s）", result.DurationSec)
	}
}

// TestMMERenderBadSpec 无 DSN/ffmpeg 依赖的纯 handler 校验门。
func TestMMERenderBadSpec(t *testing.T) {
	h := jobrun.MMERenderHandler(jobrun.MMERenderDeps{FFmpegPath: "definitely-not-exists-ffmpeg-x"})
	emit := func(float64) {}
	job := jobstore.Job{ID: "job_test_badspec"}

	// 缺 aspect.filter → BAD_SPEC（在 ffmpeg 探测之前）。
	_, err := h(t.Context(), job, json.RawMessage(`{"src_path":"a","dst_path":"b","start_sec":0,"end_sec":5,"crf":23}`), emit)
	assertCode(t, err, "MEDIA_RENDER_BAD_SPEC")

	// 规格合法但 ffmpeg 缺失 → NO_FFMPEG。
	_, err = h(t.Context(), job, json.RawMessage(`{"src_path":"a","dst_path":"b","aspect":{"filter":"crop=1:1"},"start_sec":0,"end_sec":5,"crf":23}`), emit)
	assertCode(t, err, "MEDIA_RENDER_NO_FFMPEG")

	// EDL 起止倒置 → BAD_SPEC。
	_, err = h(t.Context(), job, json.RawMessage(`{"src_path":"a","dst_path":"b","aspect":{"filter":"crop=1:1"},"start_sec":5,"end_sec":5,"crf":23}`), emit)
	assertCode(t, err, "MEDIA_RENDER_BAD_SPEC")
}

func assertCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误码 %s，实际无错误", want)
	}
	var ce interface{ ErrorCode() string }
	if !errors.As(err, &ce) || ce.ErrorCode() != want {
		t.Fatalf("期望错误码 %s，实际 %v", want, err)
	}
}

// TestMMERenderCancel 取消 → ffmpeg 进程树杀 → cancelled 终态。
func TestMMERenderCancel(t *testing.T) {
	ffmpeg := ffmpegBin(t)
	if ffmpeg == "" {
		t.Skip("ffmpeg 未就位，跳过取消集成测试")
	}
	ex := wireStack(t, ffmpeg)

	src, _ := mkSource(t, ffmpeg, 25)
	dir := t.TempDir()
	dst := filepath.Join(dir, "long.mp4")
	payload := map[string]any{
		"variant_id": "v_test_cancel",
		"src_path":   src,
		"dst_path":   dst,
		"aspect":     map[string]any{"filter": "crop=ih*9/16:ih,scale=1080:1920"},
		"start_sec":  0,
		"end_sec":    20, // 足够长，确保 cancel 落在编码中段
		"preset":     "ultrafast",
		"crf":        32,
	}
	raw, _ := json.Marshal(payload)
	j, _, err := ex.Submit(t.Context(), "render", "mme.render", raw, "mme-render-test-cancel")
	if err != nil {
		t.Fatal(err)
	}

	// 等进入 running 再取消。
	dsn := os.Getenv("MEDIA_TEST_DSN")
	s := storeOf(t, ex, dsn)
	deadline := time.Now().Add(30 * time.Second)
	for {
		cur, err := s.Get(t.Context(), j.ID)
		if err != nil {
			t.Fatal(err)
		}
		if cur.State == jobstore.StateRunning || cur.State.Terminal() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("任务未在 30s 内进入 running")
		}
		time.Sleep(100 * time.Millisecond)
	}

	if _, err := ex.Cancel(t.Context(), j.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	final := waitByStore(t, s, j.ID, 30*time.Second)
	if final.State != jobstore.StateCancelled {
		t.Fatalf("期望 cancelled，实际 %s: %s", final.State, final.ErrorMsg)
	}
	if final.ErrorCode != "MEDIA_JOB_CANCELLED" {
		t.Fatalf("错误码应为 MEDIA_JOB_CANCELLED: %s", final.ErrorCode)
	}
}
