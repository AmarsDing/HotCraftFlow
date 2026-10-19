package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/conf"
)

// fakeMedia 以 httptest 伪造 media 服务的 REST+SSE 面（契约取自
// api/media/v1/studio + sseapi 写帧格式）。assertAuth=true 时校验
// 每个请求都带 Authorization Bearer。
type fakeMedia struct {
	t          *testing.T
	assertAuth bool

	job          map[string]any   // GET /jobs/{id} 应答（可变：轮询用例逐拍改）
	sseStatus    int              // 非 200 时模拟 SSE 建连失败（触发轮询兜底）
	sseFrames    []string         // 顺序写出的 SSE 帧（原始文本，空行分隔）
	submitStatus int              // 非 200 时模拟提交失败
	gotPayload   map[string]any   // 捕获 submit 请求体
	gotCancel    *atomic.Int32    // :cancel 计数
}

func newFakeMedia(t *testing.T) *fakeMedia {
	return &fakeMedia{t: t, assertAuth: true, sseStatus: 200, submitStatus: 200, gotCancel: &atomic.Int32{}}
}

func (f *fakeMedia) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/media/studio/jobs", func(w http.ResponseWriter, r *http.Request) {
		if f.checkAuth(w, r) {
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/media/studio/jobs" {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &f.gotPayload)
			if f.submitStatus != 200 {
				w.WriteHeader(f.submitStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"job_fake","lane":"render","type":"mme.render","state":"queued"}`)
		}
	})
	mux.HandleFunc("/api/v1/media/studio/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if f.checkAuth(w, r) {
			return
		}
		if strings.HasSuffix(r.URL.Path, ":cancel") {
			f.gotCancel.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"job_fake","state":"cancelled"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		raw, _ := json.Marshal(f.job)
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/api/v1/media/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if f.checkAuth(w, r) {
			return
		}
		if f.sseStatus != 200 {
			w.WriteHeader(f.sseStatus)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for _, fr := range f.sseFrames {
			_, _ = fmt.Fprint(w, fr+"\n\n")
			if fl != nil {
				fl.Flush()
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
	return mux
}

func (f *fakeMedia) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if !f.assertAuth {
		return false
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
		f.t.Errorf("请求缺 JWT: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
		return true
	}
	return false
}

func testRenderer(t *testing.T, fm *fakeMedia) (bizmme.Renderer, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fm.handler())
	t.Cleanup(srv.Close)
	r := NewMediaServiceRenderer(srv.URL, "test-secret-key", log.NewStdLogger(io.Discard))
	return r, srv
}

func testSpec() bizmme.RenderSpec {
	sp := bizmme.RenderSpec{
		VariantID: "v_ut_1",
		SrcPath:   "E:/tmp/src.mp4",
		DstPath:   "E:/tmp/dst.mp4",
		CoverPath: "E:/tmp/cover.jpg",
		SrtPath:   "E:/tmp/burn.srt",
		StartSec:  0,
		EndSec:    6,
		Preset:    "medium",
		CRF:       23,
	}
	sp.Aspect = bizmme.AspectSpec{Key: "9:16", Width: 1080, Height: 1920, Filter: "crop=ih*9/16:ih,scale=1080:1920"}
	sp.Margin = bizmme.SafetyMargin{Bottom: 380}
	return sp
}

func TestMediaServiceRendererHappy(t *testing.T) {
	fm := newFakeMedia(t)
	fm.sseFrames = []string{
		": keepalive",
		"event: job.progress\ndata: {\"progress\":0.4}",
		"event: job.progress\ndata: {\"progress\":0.8}",
		"event: job.done\ndata: {\"progress\":1.0}",
	}
	fm.job = map[string]any{
		"id": "job_fake", "state": "done", "progress": 1.0,
		"resultJson": `{"video_path":"E:/tmp/dst.mp4","cover_path":"E:/tmp/cover.jpg","srt_path":"E:/tmp/burn.srt","duration_sec":6.02}`,
	}
	r, _ := testRenderer(t, fm)

	var prog []float64
	res, err := r.Render(context.Background(), testSpec(), func(p float64) { prog = append(prog, p) })
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if res.VideoPath != "E:/tmp/dst.mp4" || res.DurationSec != 6.02 {
		t.Fatalf("结果不符: %+v", res)
	}
	if len(prog) < 3 || prog[0] != 0.4 || prog[1] != 0.8 {
		t.Fatalf("进度序列异常: %v", prog)
	}

	// submit 载荷契约断言（protojson 命名 + 幂等键 + lane）。
	if fm.gotPayload["type"] != "mme.render" || fm.gotPayload["lane"] != "render" {
		t.Fatalf("submit 载荷缺 type/lane: %v", fm.gotPayload)
	}
	// M2：幂等键 = mme_render_<variant>_<fnv64hex 16位>（载荷指纹：参数一变即换新
	// job；同参重试同键幂等去重——两性质见 TestMediaRendererIdempotencyKey）。
	key, _ := fm.gotPayload["idempotencyKey"].(string)
	if !strings.HasPrefix(key, "mme_render_v_ut_1_") || len(key) != len("mme_render_v_ut_1_")+16 {
		t.Fatalf("幂等键形态不符（应为 mme_render_<variant>_<16hex>）: %q", key)
	}
	pl, _ := fm.gotPayload["payloadJson"].(string)
	var inner map[string]any
	if json.Unmarshal([]byte(pl), &inner) != nil {
		t.Fatalf("payloadJson 不是 JSON 串: %q", pl)
	}
	if inner["src_path"] != "E:/tmp/src.mp4" {
		t.Fatalf("内层载荷 src_path 不符: %v", inner)
	}
	asp, _ := inner["aspect"].(map[string]any)
	if asp["filter"] != "crop=ih*9/16:ih,scale=1080:1920" {
		t.Fatalf("aspect.filter 丢失: %v", asp)
	}
}

// TestMediaRendererIdempotencyKey M2 幂等键语义矩阵：
//   - 同参重试 → 同键（media 侧 jobstore 幂等去重，不重复烧机器）；
//   - 任何渲染参数变化 → 新键（QC 编辑后重渲不吃旧 done job 的 stale 结果）。
func TestMediaRendererIdempotencyKey(t *testing.T) {
	// fake 直接置 done，观察三次 submit 抓到的键。
	mkDone := func() (*bizmme.Renderer, *fakeMedia) {
		fm := newFakeMedia(t)
		fm.sseFrames = []string{"event: job.done\ndata: {\"progress\":1.0}"}
		fm.job = map[string]any{
			"id": "job_fake", "state": "done", "progress": 1.0,
			"resultJson": `{"video_path":"E:/tmp/dst.mp4","cover_path":"E:/tmp/cover.jpg","srt_path":"E:/tmp/burn.srt","duration_sec":6.0}`,
		}
		r, _ := testRenderer(t, fm)
		return &r, fm
	}
	submitKey := func(r bizmme.Renderer, fm *fakeMedia, spec bizmme.RenderSpec) string {
		fm.gotPayload = nil
		if _, err := r.Render(context.Background(), spec, func(float64) {}); err != nil {
			t.Fatalf("render: %v", err)
		}
		key, _ := fm.gotPayload["idempotencyKey"].(string)
		if key == "" {
			t.Fatal("submit 未落幂等键")
		}
		return key
	}

	r, fm := mkDone()
	rr := *r
	spec := testSpec()
	k1 := submitKey(rr, fm, spec)
	k2 := submitKey(rr, fm, spec)
	if k1 != k2 {
		t.Fatalf("同参重试应同键（幂等去重）: %q vs %q", k1, k2)
	}

	// 逐参数扰动：任一变都必须换键（矩阵采样 Cut/画幅/源路径三个高频编辑面）。
	for _, perturb := range []struct {
		name string
		edit func(*bizmme.RenderSpec)
	}{
		{"切点变化", func(s *bizmme.RenderSpec) { s.StartSec++ }},
		{"画幅变化", func(s *bizmme.RenderSpec) { s.Aspect.Filter = "crop=iw:iw,scale=1080:1080" }},
		{"源路径变化", func(s *bizmme.RenderSpec) { s.SrcPath = "E:/tmp/src_v2.mp4" }},
	} {
		s2 := spec
		perturb.edit(&s2)
		if got := submitKey(rr, fm, s2); got == k1 {
			t.Fatalf("%s：参数变化必须换新键（不吃 stale job），仍为 %q", perturb.name, got)
		}
	}
}

func TestMediaServiceRendererFail(t *testing.T) {
	fm := newFakeMedia(t)
	fm.sseFrames = []string{"event: job.failed\ndata: {}"}
	fm.job = map[string]any{
		"id": "job_fake", "state": "failed",
		"errorCode": "MEDIA_RENDER_FAIL", "errorMsg": "ffmpeg exit 1",
	}
	r, _ := testRenderer(t, fm)

	_, err := r.Render(context.Background(), testSpec(), nil)
	if !errors.Is(err, bizmme.ErrRenderFail) {
		t.Fatalf("应包裹 ErrRenderFail: %v", err)
	}
	if !strings.Contains(err.Error(), "MEDIA_RENDER_FAIL") {
		t.Fatalf("应透传上游错误码: %v", err)
	}
}

func TestMediaServiceRendererSubmitFail(t *testing.T) {
	fm := newFakeMedia(t)
	fm.submitStatus = 500
	r, _ := testRenderer(t, fm)

	_, err := r.Render(context.Background(), testSpec(), nil)
	if !errors.Is(err, bizmme.ErrRenderFail) {
		t.Fatalf("提交失败应包裹 ErrRenderFail: %v", err)
	}
}

// TestMediaServiceRendererPollFallback SSE 建连 404 → 降级轮询 → done。
func TestMediaServiceRendererPollFallback(t *testing.T) {
	fm := newFakeMedia(t)
	fm.sseStatus = 404
	fm.job = map[string]any{"id": "job_fake", "state": "running", "progress": 0.5}
	r, _ := testRenderer(t, fm)

	// 第二拍起轮询应答变为 done。
	go func() {
		time.Sleep(1800 * time.Millisecond)
		fm.job = map[string]any{
			"id": "job_fake", "state": "done", "progress": 1.0,
			"resultJson": `{"video_path":"E:/tmp/dst.mp4","cover_path":"","srt_path":"E:/tmp/burn.srt","duration_sec":5.9}`,
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := r.Render(ctx, testSpec(), nil)
	if err != nil {
		t.Fatalf("轮询兜底应成功: %v", err)
	}
	if res.VideoPath != "E:/tmp/dst.mp4" {
		t.Fatalf("结果不符: %+v", res)
	}
}

// TestMediaServiceRendererCancel 上游 ctx 取消 → 远端 :cancel 被调用。
func TestMediaServiceRendererCancel(t *testing.T) {
	fm := newFakeMedia(t)
	// SSE 永不给终态（阻塞到 ctx 取消）。
	fm.sseFrames = []string{}
	fm.assertAuth = false // 取消请求的令牌在后台 goroutine 里取，简单起见跳过断言
	r, srv := testRenderer(t, fm)
	_ = srv

	// 让 SSE handler 挂住：替换为不返回的服务
	srv.Close()
	block := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
		switch {
		case rq.URL.Path == "/api/v1/media/studio/jobs" && rq.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":"job_fake","state":"queued"}`)
		case strings.HasSuffix(rq.URL.Path, ":cancel"):
			fm.gotCancel.Add(1)
			_, _ = fmt.Fprint(w, `{"id":"job_fake","state":"cancelled"}`)
		case strings.Contains(rq.URL.Path, "/events"):
			w.Header().Set("Content-Type", "text/event-stream")
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			<-rq.Context().Done() // 挂住直到客户端断
		default:
			_, _ = fmt.Fprint(w, `{"id":"job_fake","state":"cancelled"}`)
		}
	}))
	t.Cleanup(block.Close)
	r = NewMediaServiceRenderer(block.URL, "k", log.NewStdLogger(io.Discard))

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	_, err := r.Render(ctx, testSpec(), nil)
	if err == nil {
		t.Fatal("ctx 取消后 Render 应返回错误")
	}
	// AfterFunc 异步触发：稍等远端取消落账。
	time.Sleep(300 * time.Millisecond)
	if fm.gotCancel.Load() == 0 {
		t.Fatal("ctx 取消未触发远端 :cancel")
	}
}

// TestNewMMERendererSwitch env 开关：缺省 local，media 时换实现。
func TestNewMMERendererSwitch(t *testing.T) {
	l := log.NewStdLogger(io.Discard)
	t.Setenv("MME_RENDER_EXECUTOR", "")
	if _, ok := NewMMERenderer(bizmme.Config{FFmpegPath: "ffmpeg"}, &conf.Bootstrap{}, l).(*FFmpegRenderer); !ok {
		t.Fatal("缺省应为本地 FFmpegRenderer")
	}
	t.Setenv("MME_RENDER_EXECUTOR", "media")
	r := NewMMERenderer(bizmme.Config{}, bootstrapWithJWT("k"), l)
	if _, ok := r.(*MediaServiceRenderer); !ok {
		t.Fatal("executor=media 应为 MediaServiceRenderer")
	}
}

func bootstrapWithJWT(key string) *conf.Bootstrap {
	return &conf.Bootstrap{
		Auth: &conf.Auth{
			Authn: &conf.Auth_Authn{
				Jwt: &conf.Auth_JWT{Method: "HS256", Key: key},
			},
		},
	}
}
