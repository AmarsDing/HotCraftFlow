package sseapi_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"framework/app/media/internal/authnutil"
	"framework/app/media/internal/jobrun"
	"framework/app/media/internal/jobstore"
	"framework/app/media/internal/sseapi"
)

// testSchema 本包测试专属 PG schema：go test 默认并行跑不同包的二进制，
// 共享 public schema 会被彼此的 FailOrphaned/RecoverQueued 误伤（真实根因，
// 见 jobstore/jobstore_test.go 的同名注释）。
const testSchema = "sseapi_test"

// wireTestStack DSN 门控的 store+executor 最小栈。
func wireTestStack(t *testing.T) (*jobstore.Store, *jobrun.Executor) {
	t.Helper()
	dsn := os.Getenv("MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("MEDIA_TEST_DSN 未设置，跳过 SSE 集成测试")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+testSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	_ = admin.Close()

	s, err := jobstore.Open(dsn+" search_path="+testSchema, 16)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	ex := jobrun.New(s, jobrun.Config{
		Lanes:            map[string]int32{"io": 2, "infer": 1},
		ProgressMinGapMs: 1, // 测试内感知每个进度事件
	}, "sse-test", nil)
	ex.RegisterHandler("test.tick", func(ctx context.Context, job jobstore.Job, payload json.RawMessage, emit func(float64)) (json.RawMessage, error) {
		for i := 1; i <= 3; i++ {
			time.Sleep(60 * time.Millisecond)
			emit(float64(i) / 4)
		}
		return json.RawMessage(`{"ok":true}`), nil
	})
	if err := ex.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ex.Stop(ctx)
		_ = s.Close()
		if admin, err := sql.Open("pgx", dsn); err == nil {
			_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+testSchema+` CASCADE`)
			_ = admin.Close()
		}
	})
	return s, ex
}

// testToken HMAC 测签（与 Verifier 同钥）。
func testToken(key string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": 1, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(key))
	if err != nil {
		panic(err)
	}
	return s
}

// sseEvent 解析出的一帧事件。
type sseEvent struct {
	Seq  int64
	Kind string
}

// readEvents 读到 terminal（或 EOF/超时）为止，返回事件帧序列。
func readEvents(t *testing.T, resp *http.Response) []sseEvent {
	t.Helper()
	var out []sseEvent
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		var seq int64
		var kind string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, ":"):
				// keepalive / connected 注释
			case strings.HasPrefix(line, "id: "):
				_, _ = fmt.Sscanf(line, "id: %d", &seq)
			case strings.HasPrefix(line, "event: "):
				kind = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				// payload 本体不校验（序列连续性由 seq 覆盖）
			case line == "":
				if kind != "" {
					out = append(out, sseEvent{Seq: seq, Kind: kind})
					kind = ""
					if strings.HasPrefix(out[len(out)-1].Kind, "job.done") ||
						out[len(out)-1].Kind == string(jobstore.EvDone) ||
						out[len(out)-1].Kind == string(jobstore.EvFailed) ||
						out[len(out)-1].Kind == string(jobstore.EvCancelled) {
						return
					}
				}
			default:
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = resp.Body.Close()
		t.Fatalf("读流超时，已收到: %+v", out)
	}
	return out
}

func kindsOf(evs []sseEvent) string {
	var b strings.Builder
	for i, e := range evs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(e.Kind)
	}
	return b.String()
}

func TestSSEReplayLiveTerminal(t *testing.T) {
	_, ex := wireTestStack(t)

	const key = "test-jwt-key"
	h := sseapi.Handler(sseapi.Deps{
		Verifier:         authnutil.NewVerifier(key),
		Executor:         ex,
		KeepaliveSeconds: 1,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	// 不用幂等键：每次跑都是新 job，避免复跑拿到旧终态。
	j, _, err := ex.Submit(t.Context(), "io", "test.tick", json.RawMessage(`{}`), "")
	if err != nil {
		t.Fatal(err)
	}

	// 完整流：queued → running(+progress) → terminal。
	url := fmt.Sprintf("%s/api/v1/media/jobs/%s/events?token=%s", srv.URL, j.ID, testToken(key))
	resp, err := http.Get(url) //nolint
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("状态码: %d", resp.StatusCode)
	}
	evs := readEvents(t, resp)
	_ = resp.Body.Close()

	seq := kindsOf(evs)
	for _, want := range []string{
		string(jobstore.EvQueued),
		string(jobstore.EvRunning),
		string(jobstore.EvDone),
	} {
		if !strings.Contains(seq, want) {
			t.Fatalf("事件序列缺 %s: %s", want, seq)
		}
	}
	// seq 单调递增。
	for i := 1; i < len(evs); i++ {
		if evs[i].Seq <= evs[i-1].Seq {
			t.Fatalf("seq 不单调: %v", evs)
		}
	}
	if evs[0].Seq != 1 {
		t.Fatalf("首 seq 应为 1: %d", evs[0].Seq)
	}

	// 断线全量回放：after_seq=0 重连应收到同量事件（任务已终态，回放后即关流）。
	url2 := fmt.Sprintf("%s/api/v1/media/jobs/%s/events?token=%s&after_seq=0", srv.URL, j.ID, testToken(key))
	resp2, err := http.Get(url2) //nolint
	if err != nil {
		t.Fatal(err)
	}
	evs2 := readEvents(t, resp2)
	_ = resp2.Body.Close()
	if len(evs2) != len(evs) {
		t.Fatalf("全量回放条数: %d ≠ %d（seq=%s）", len(evs2), len(evs), kindsOf(evs2))
	}

	// 增量回放：after_seq=头一条 → 应少一条。
	url3 := fmt.Sprintf("%s/api/v1/media/jobs/%s/events?token=%s&after_seq=%d",
		srv.URL, j.ID, testToken(key), evs[0].Seq)
	resp3, err := http.Get(url3) //nolint
	if err != nil {
		t.Fatal(err)
	}
	evs3 := readEvents(t, resp3)
	_ = resp3.Body.Close()
	if len(evs3) != len(evs)-1 {
		t.Fatalf("增量回放条数: %d ≠ %d", len(evs3), len(evs)-1)
	}
}

// TestSSEUnauthorized 无令牌 → 401（不发事件流）。
func TestSSEUnauthorized(t *testing.T) {
	// 不依赖 DSN：Verifier 先于 Executor 命中，构造一个 nil executor 的安全深依赖即可。
	h := sseapi.Handler(sseapi.Deps{
		Verifier: authnutil.NewVerifier("test-jwt-key"),
		Executor: nil,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Executor 为 nil 时若验签通过会 panic——handler 有 recover 兜底，
		// 但本用例只走验签失败支路，不应到达 executor。
		h(w, r)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/media/jobs/job_x/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("无令牌应为 401: %d", resp.StatusCode)
	}
}
