package sseapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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

// wireTestStack DSN 门控的 store+executor 最小栈。
func wireTestStack(t *testing.T) (*jobstore.Store, *jobrun.Executor, func()) {
	t.Helper()
	dsn := os.Getenv("MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("MEDIA_TEST_DSN 未设置，跳过 SSE 集成测试")
	}
	s, err := jobstore.Open(dsn, 16)
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
	return s, ex, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ex.Stop(ctx)
		_, _ = s.Ping.Close
		_ = s.Close()
	}
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

func TestSSEReplayLiveTerminal(t *testing.T) {
	_, ex, cleanup := wireTestStack(t)
	defer cleanup()

	const key = "test-jwt-key"
	h := sseapi.Handler(sseapi.Deps{
		Verifier:         authnutil.NewVerifier(key),
		Executor:         ex,
		KeepaliveSeconds: 1,
	})
	srv := httptest.NewServer(h)
	defer srv.Close()

	j, _, err := ex.Submit(t.Context(), "io", "test.tick", json.RawMessage(`{}`), "sse-e2e-1")
	if err != nil {
		t.Fatal(err)
	}

	// 完整流：队首连接到收尾。
	url := fmt.Sprintf("%s/api/v1/media/jobs/%s/events?token=%s", srv.URL, j.ID, testToken(key))
	req, _ := http.WithContext(t.Context())
	_ = req
	resp, err := rawClient.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("状态码: %d", resp.StatusCode)
	}

	var (
		kinds []string
		last  int64
	)
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	deadline := time.After(10 * time.Second)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		event := ""
		data := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, ":"):
			case strings.HasPrefix(line, "id: "):
				fmt.Sscanf(line, "id: %d", &last)
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "":
				if event != "" {
					kinds = append(kinds, event)
					event, data = "", ""
				}
			}
			if event == "" && len(kinds) > 0 && kinds[len(kinds)-1] == string(jobstore.EvDone) {
				return
			}
		}
	}()

	for {
		select {
		case <-readDone:
			goto FIN
		case <-deadline:
			t.Fatalf("读流超时，已收到事件: %v", kinds)
		case <-time.After(50 * time.Millisecond):
			// Scanner 阻塞读时周期 tick，靠 readDone 唤醒。
		}
		goto FIN_GUARD
	FIN_GUARD:
	}
FIN:
	seq := strings.Join(kinds, ",")
	for _, want := range []string{"job.queued", "job.running", "job.done"} {
		if !strings.Contains(seq, want) {
			t.Fatalf("事件序列缺 %s: %s", want, seq)
		}
	}
	if last == 0 {
		t.Fatal("last seq 不应为 0")
	}

	// 断线回放：after_seq=last 应零事件；after_seq=0 应全量。
	url2 := fmt.Sprintf("%s/api/v1/media/jobs/%s/events?token=%s&after_seq=%d",
		srv.URL, j.ID, testToken(key), 0)
	resp2, err := rawClient.Get(url2)
	if err != nil {
		t.Fatal(err)
	}
	sc2 := bufio.NewScanner(resp2.Body)
	sc2.Buffer(make([]byte, 64*1024), 1<<20)
	count := 0
	done := time.After(5 * time.Second)
READ2:
	for {
		select {
		case <-done:
			t.Fatalf("回放读取超时（count=%d）", count)
		default:
		}
		if !sc2.Scan() {
			break
		}
		if strings.HasPrefix(sc2.Text(), "id: ") {
			count++
		}
		if count >= len(kinds) {
			break READ2
		}
	}
	resp2.Body.Close()
	if count != len(kinds) {
		t.Fatalf("全量回放条数: %d ≠ %d", count, len(kinds))
	}
}

var rawClient = http2.Client()
