// Package observability 提供 5 个微服务统一的可观测性能力：
//   - /metrics Prometheus 指标暴露（promhttp）
//   - /ready   就绪探针（聚合命名依赖检查，任一失败返回 503）
//   - HTTP 请求指标中间件（请求总数 + 时延直方图，供 Prometheus 抓取）
//
// 对应整体实施计划 Phase 1-1.10：5 服务统一接入 Prometheus metrics + /ready 依赖检查。
package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ---------- /metrics ----------

// MetricsHandler 返回 Prometheus 抓取端点 handler（默认注册表）。
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// ---------- /ready ----------

// CheckFunc 依赖就绪检查函数，返回 nil 表示就绪。
type CheckFunc func(ctx context.Context) error

// ReadyHandler 聚合命名依赖检查：全部通过返回 200，任一失败返回 503 并附明细。
// 每项检查带独立超时，避免慢依赖拖垮探针。
func ReadyHandler(checks map[string]CheckFunc, timeout time.Duration) http.HandlerFunc {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return func(w http.ResponseWriter, r *http.Request) {
		type result struct {
			name string
			err  error
		}
		results := make([]result, 0, len(checks))
		var mu sync.Mutex
		var wg sync.WaitGroup

		for name, check := range checks {
			if check == nil {
				continue
			}
			wg.Add(1)
			go func(n string, c CheckFunc) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(r.Context(), timeout)
				defer cancel()
				mu.Lock()
				results = append(results, result{name: n, err: c(ctx)})
				mu.Unlock()
			}(name, check)
		}
		wg.Wait()

		status := http.StatusOK
		detail := make(map[string]string, len(results))
		for _, res := range results {
			if res.err != nil {
				status = http.StatusServiceUnavailable
				detail[res.name] = res.err.Error()
			} else {
				detail[res.name] = "ok"
			}
		}

		statusText := "ready"
		if status != http.StatusOK {
			statusText = "not_ready"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": statusText,
			"checks": detail,
		})
	}
}

// ---------- HTTP 请求指标中间件 ----------

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "framework",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed.",
		},
		[]string{"service", "method", "path", "code"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "framework",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"service", "method", "path"},
	)
	registerOnce sync.Once
)

// MetricsMiddleware 返回 kratos HTTP 请求指标中间件。
// serviceName 作为 `service` 标签区分 5 个微服务。
// path 标签优先取路由模板（如 /api/v1/monitor/assets/{id}），避免 ID 类路径造成 label 高基数。
func MetricsMiddleware(serviceName string) middleware.Middleware {
	registerOnce.Do(func() {
		prometheus.MustRegister(httpRequestsTotal, httpRequestDuration)
	})
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			start := time.Now()
			reply, err := handler(ctx, req)

			if tr, ok := transport.FromServerContext(ctx); ok && tr.Kind() == transport.KindHTTP {
				if ht, ok2 := tr.(*kratoshttp.Transport); ok2 {
					method := ht.Request().Method
					path := ht.PathTemplate()
					if path == "" {
						path = ht.Request().URL.Path
					}
					code := strconv.Itoa(httpStatusFromError(err))
					httpRequestsTotal.WithLabelValues(serviceName, method, path, code).Inc()
					httpRequestDuration.WithLabelValues(serviceName, method, path).Observe(time.Since(start).Seconds())
				}
			}
			return reply, err
		}
	}
}

// httpStatusFromError 从 kratos 错误推导 HTTP 状态码；无错误返回 200。
func httpStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if se := kratoserrors.FromError(err); se != nil {
		return int(se.Code)
	}
	return http.StatusInternalServerError
}
