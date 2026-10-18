package wsapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/golang-jwt/jwt/v5"

	"framework/app/media/internal/authnutil"
	"framework/app/media/internal/jobrun"
	"framework/app/media/internal/jobstore"
	"framework/app/media/internal/wsapi"
)

// testSchema 本包专属 PG schema（并行包二进制互不误伤；纪律同 jobstore 测试）。
const testSchema = "wsapi_test"

// wireTestStack DSN 门控的 store+executor+hub 最小栈。
func wireTestStack(t *testing.T) (*jobrun.Executor, *wsapi.Hub, string) {
	t.Helper()
	dsn := os.Getenv("MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("MEDIA_TEST_DSN 未设置，跳过 WS 集成测试")
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
		Lanes:            map[string]int32{"io": 2},
		ProgressMinGapMs: 1,
	}, "ws-test", nil)
	ex.RegisterHandler("test.tick", func(ctx context.Context, job jobstore.Job, payload json.RawMessage, emit func(float64)) (json.RawMessage, error) {
		for i := 1; i <= 3; i++ {
			time.Sleep(50 * time.Millisecond)
			emit(float64(i) / 4)
		}
		return json.RawMessage(`{"ok":true}`), nil
	})
	if err := ex.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	const key = "test-ws-key"
	hub := wsapi.NewHub(wsapi.Deps{
		Verifier: authnutil.NewVerifier(key),
		Executor: ex,
	})
	if err := hub.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = hub.Stop(context.Background())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ex.Stop(ctx)
		_ = s.Close()
		if admin, err := sql.Open("pgx", dsn); err == nil {
			_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+testSchema+` CASCADE`)
			_ = admin.Close()
		}
	})
	return ex, hub, key
}

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

type wsFrame struct {
	Kind     string  `json:"kind"`
	JobID    string  `json:"job_id"`
	Lane     string  `json:"lane"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
	At       string  `json:"at"`
}

func TestWSInvalidationFrames(t *testing.T) {
	ex, hub, key := wireTestStack(t)

	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) +
		"/api/v1/media/ws/events?token=" + testToken(key)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") }()

	// 连接建立后再提交任务（广播只推在线连接）。
	j, _, err := ex.Submit(ctx, "io", "test.tick", json.RawMessage(`{}`), "")
	if err != nil {
		t.Fatal(err)
	}

	// 期望 3 帧且仅此 3 帧：created → updated(running) → finished(done)。
	// 进度事件是 OpEvent（详情级），按纪律不上 WS 总线。
	want := []struct{ kind, state string }{
		{"job.created", "queued"},
		{"job.updated", "running"},
		{"job.finished", "done"},
	}
	for i, w := range want {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("读第 %d 帧: %v（想要 %s）", i, err, w.kind)
		}
		var f wsFrame
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("帧不是 JSON: %v: %s", err, data)
		}
		if f.Kind != w.kind || f.State != w.state {
			t.Fatalf("第 %d 帧: got %s/%s want %s/%s", i, f.Kind, f.State, w.kind, w.state)
		}
		if f.JobID != j.ID {
			t.Fatalf("帧 job_id 不符: %s ≠ %s", f.JobID, j.ID)
		}
		if f.Lane != "io" || f.At == "" {
			t.Fatalf("帧字段残缺: %+v", f)
		}
	}
}

// TestWSUnauthorized 无令牌 → 401（不升级）。
func TestWSUnauthorized(t *testing.T) {
	// hub 需要 executor 仅用于 Bus()；这里验签在 Bus 之前，nil 也不应被触达——
	// 但保守起见走完整栈，避免实现漂移后 panic。
	_, hub, _ := wireTestStack(t)

	srv := httptest.NewServer(hub.Handler())
	defer srv.Close()
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/api/v1/media/ws/events"

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("无令牌连接不应成功")
	}
	if resp == nil || resp.StatusCode != 401 {
		t.Fatalf("应为 401: %+v", resp)
	}
}
