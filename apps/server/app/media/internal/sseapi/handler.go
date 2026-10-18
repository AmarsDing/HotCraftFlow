// Package sseapi 任务事件的 SSE 裸端点。
//
// 端点：GET /api/v1/media/jobs/{id}/events?after_seq=N
//
// 语义纪律：
//   - 权威源永远是 media_job_events 表；内存 Notice 只作"有新事件"唤醒信号；
//   - 断线重连带 after_seq=上次最后 seq，缺口一次性回放后进入实时段；
//   - 每 15s 写 ": keepalive" 注释穿透代理空闲超时；
//   - 写出 terminal 事件（job.done/failed/cancelled）即关闭流。
package sseapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"framework/app/media/internal/authnutil"
	"framework/app/media/internal/jobrun"
	"framework/app/media/internal/jobstore"
)

// Deps 构造依赖。
type Deps struct {
	Verifier *authnutil.Verifier
	Executor *jobrun.Executor
	Log      *slog.Logger
	// KeepaliveSeconds 覆盖默认 15s 心跳间隔（测试可缩短）。
	KeepaliveSeconds int
}

const (
	pathPrefix  = "/api/v1/media/jobs/"
	pathSuffix  = "/events"
	replayBatch = 500
	// maxStreamAge 单连接寿命兜底（防泄漏端点长挂）。
	maxStreamAge = 6 * time.Hour
)

// Handler 返回 http.HandlerFunc；path 形如前缀+{id}+后缀。
func Handler(dep Deps) http.HandlerFunc {
	if dep.Log == nil {
		dep.Log = slog.Default()
	}
	keepalive := time.Duration(dep.KeepaliveSeconds) * time.Second
	if keepalive <= 0 {
		keepalive = 15 * time.Second
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// panic 兜底：SSE 中途崩不得拖垮 server。
		defer func() {
			if rec := recover(); rec != nil {
				dep.Log.Error("sse handler panic", "panic", rec)
			}
		}()

		id, ok := parseJobID(r.URL.Path)
		if !ok {
			http.Error(w, `{"code":400,"reason":"MEDIA_BAD_REQUEST","message":"bad job path"}`+"\n", http.StatusBadRequest)
			return
		}
		if _, err := dep.Verifier.Verify(r); err != nil {
			http.Error(w, `{"code":401,"reason":"UNAUTHORIZED","message":"unauthorized"}`+"\n", http.StatusUnauthorized)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, `{"code":500,"reason":"MEDIA_INTERNAL","message":"streaming unsupported"}`+"\n", http.StatusInternalServerError)
			return
		}

		var afterSeq int64
		if v := r.URL.Query().Get("after_seq"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				afterSeq = n
			}
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // 穿透 nginx
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		streamCtx, cancel := context.WithTimeout(r.Context(), maxStreamAge)
		defer cancel()

		// 回放段 + 实时段统一循环：每次被唤醒就 drain 库中 seq > cursor 的事件。
		pingCh, unsubscribe := dep.Executor.Subscribe(id)
		defer unsubscribe()

		cursor := afterSeq
		beat := time.NewTimer(keepalive)
		defer beat.Stop()

		for {
			terminal := false
			evs, err := dep.Executor.EventsSince(streamCtx, id, cursor, replayBatch)
			if err != nil {
				dep.Log.Warn("sse events query failed", "job", id, "err", err)
				return
			}
			for _, e := range evs {
				cursor = e.Seq
				if !writeEvent(w, e) {
					return // 客户端已断
				}
				if isTerminalKind(e.Kind) {
					terminal = true
				}
			}
			if len(evs) > 0 {
				flusher.Flush()
			}
			if terminal {
				return
			}

			// 无新事件：等唤醒或心跳。
			beat.Reset(keepalive)
			select {
			case <-streamCtx.Done():
				return
			case <-beat.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case <-pingCh:
				// 有新事件信号 → 回到 drain
			}
		}
	}
}

// parseJobID 从路径提取 job id（前缀/后缀双锚，防形如 /jobs/x/events/y）。
func parseJobID(path string) (string, bool) {
	if !strings.HasPrefix(path, pathPrefix) || !strings.HasSuffix(path, pathSuffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, pathPrefix), pathSuffix)
	if id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", false
	}
	return id, true
}

// writeEvent 写一帧 SSE；返回 false 表示连接已断。
func writeEvent(w http.ResponseWriter, e jobstore.Event) bool {
	var b strings.Builder
	b.WriteString("id: ")
	b.WriteString(strconv.FormatInt(e.Seq, 10))
	b.WriteString("\nevent: ")
	b.WriteString(string(e.Kind))
	b.WriteString("\ndata: ")
	payload := strings.ReplaceAll(string(e.Payload), "\n", "")
	if payload == "" {
		payload = "{}"
	}
	b.WriteString(payload)
	b.WriteString("\n\n")
	_, err := fmt.Fprint(w, b.String())
	return err == nil
}

// isTerminalKind terminal 事件判据（写出后关流）。
func isTerminalKind(k jobstore.EventKind) bool {
	switch k {
	case jobstore.EvDone, jobstore.EvFailed, jobstore.EvCancelled:
		return true
	}
	return false
}
