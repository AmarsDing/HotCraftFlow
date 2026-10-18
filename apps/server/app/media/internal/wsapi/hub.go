// Package wsapi WS 失效信号总线。
//
// 端点：GET /api/v1/media/ws/events
//
// 语义纪律：
//   - v1 只推三类轻量失效信号：job.created | job.updated | job.finished；
//     客户端收到后重查 jobs 清单取权威态（本端点不做 after_seq——
//     权威回放走 SSE after_seq，这里丢帧无害）；
//   - 25s 服务端 ping，读循环只为处理 pong/close；
//   - 鉴权同 SSE：Authorization 或 ?token=（浏览器 WS API 无法自定义 header）。
package wsapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"framework/app/media/internal/authnutil"
	"framework/app/media/internal/jobrun"
)

// Deps 构造依赖。
type Deps struct {
	Verifier *authnutil.Verifier
	Executor *jobrun.Executor
	Log      *slog.Logger
	// PingSeconds 覆盖默认 25s 心跳（测试可缩短）。
	PingSeconds int
}

// frame 出站帧（v1 词表见包注释）。
type frame struct {
	Kind     string  `json:"kind"` // job.created | job.updated | job.finished
	JobID    string  `json:"job_id"`
	Lane     string  `json:"lane"`
	State    string  `json:"state"`
	Progress float64 `json:"progress"`
	At       string  `json:"at"`
}

// Hub 广播集线器（transport.Server 挂接：Start/Stop 由 kratos 生命周期驱动）。
type Hub struct {
	dep  Deps
	ping time.Duration

	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}

	cancel context.CancelFunc
	done   chan struct{}
}

// NewHub 构造。
func NewHub(dep Deps) *Hub {
	ping := 25 * time.Second
	if dep.PingSeconds > 0 {
		ping = time.Duration(dep.PingSeconds) * time.Second
	}
	if dep.Log == nil {
		dep.Log = slog.Default()
	}
	return &Hub{dep: dep, ping: ping, clients: make(map[*websocket.Conn]struct{})}
}

// Name kratos transport.Server 语义标识。
func (h *Hub) Name() string { return "media-ws-hub" }

// Start 广播循环（kratos transport.Server 接口）。
func (h *Hub) Start(ctx context.Context) error {
	ctx, h.cancel = context.WithCancel(ctx)
	h.done = make(chan struct{})
	go h.run(ctx)
	return nil
}

// Stop 收拢全部连接并退出循环。
func (h *Hub) Stop(context.Context) error {
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Lock()
	for c := range h.clients {
		_ = c.Close(websocket.StatusGoingAway, "server shutdown")
	}
	h.mu.Unlock()
	if h.done != nil {
		<-h.done
	}
	return nil
}

// run 消费执行器总线并扇出。
func (h *Hub) run(ctx context.Context) {
	defer close(h.done)
	bus := h.dep.Executor.Bus()
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-bus:
			b, err := json.Marshal(frame{
				Kind:     string(n.Op),
				JobID:    n.JobID,
				Lane:     n.Lane,
				State:    string(n.State),
				Progress: n.Progress,
				At:       n.At.UTC().Format(time.RFC3339),
			})
			if err != nil {
				continue
			}
			h.broadcast(ctx, b)
		}
	}
}

// broadcast 非阻塞扇出（慢/死连接立即剔除）。
func (h *Hub) broadcast(ctx context.Context, data []byte) {
	h.mu.Lock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := c.Write(wctx, websocket.MessageText, data)
		cancel()
		if err != nil {
			h.drop(c)
		}
	}
}

// drop 剔除连接。
func (h *Hub) drop(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	_ = c.Close(websocket.StatusNormalClosure, "dropped")
}

// Handler WS 升级端点。
func (h *Hub) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := h.dep.Verifier.Verify(r); err != nil {
			http.Error(w, `{"code":401,"reason":"UNAUTHORIZED","message":"unauthorized"}`+"\n", http.StatusUnauthorized)
			return
		}
		// 开发期跨源放开；生产经同源 nginx，无跨域问题。
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			h.dep.Log.Warn("ws accept failed", "err", err)
			return
		}
		h.mu.Lock()
		h.clients[conn] = struct{}{}
		h.mu.Unlock()

		ctx := conn.CloseRead(r.Context())

		// ping 循环。
		stopPing := make(chan struct{})
		go func() {
			t := time.NewTicker(h.ping)
			defer t.Stop()
			for {
				select {
				case <-stopPing:
					return
				case <-ctx.Done():
					return
				case <-t.C:
					pctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					err := conn.Ping(pctx)
					cancel()
					if err != nil {
						return
					}
				}
			}
		}()

		// 读循环：只处理 close/ping/pong（CloseRead 自动回 pong）。
		<-ctx.Done()
		close(stopPing)
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}
}
