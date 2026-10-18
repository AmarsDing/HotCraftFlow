// Package server media 服务的传输层装配。
package server

import (
	"context"
	"net/http/pprof"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/http"

	"github.com/go-kratos/kratos/contrib/middleware/validate/v2"
	"github.com/gorilla/handlers"

	authnEngine "github.com/tx7do/kratos-authn/engine"
	authnjwt "github.com/tx7do/kratos-authn/engine/jwt"
	authn "github.com/tx7do/kratos-authn/middleware"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"

	"framework/app/media/internal/artifactapi"
	"framework/app/media/internal/authnutil"
	"framework/app/media/internal/conf"
	"framework/app/media/internal/service"
	"framework/app/media/internal/sseapi"
	"framework/app/media/internal/wsapi"

	studiov1 "framework/api/gen/go/media/v1/studio"

	"framework/pkg/httpx"
	"framework/pkg/observability"
)

// NewAuthenticator 创建 JWT 认证器（与 admin 同钥即令牌互通；空配置返回 nil 由启动期判死）。
func NewAuthenticator(cfg *conf.Bootstrap) authnEngine.Authenticator {
	if cfg.GetAuth() == nil || cfg.GetAuth().GetAuthn() == nil {
		return nil
	}
	if cfg.GetAuth().GetAuthn().GetType() != "jwt" {
		return nil
	}
	authenticator, err := authnjwt.NewAuthenticator(
		authnjwt.WithKey([]byte(cfg.GetAuth().GetAuthn().GetJwt().GetKey())),
		authnjwt.WithSigningMethod(cfg.GetAuth().GetAuthn().GetJwt().GetMethod()),
	)
	if err != nil {
		return nil
	}
	return authenticator
}

// RawDeps 裸端点（SSE/WS/artifacts）的装配依赖。
type RawDeps struct {
	Verifier     *authnutil.Verifier
	SSEHandler   sseapi.Deps
	WSHub        *wsapi.Hub
	ArtifactBase string
}

// NewRESTServer media REST 服务：
//   - proto 路由（jobs/engines/doctor）全部挂 authn 中间件（media 无登录面，不设白名单）；
//   - 裸端点手工验签：SSE（/jobs/{id}/events）、WS（/ws/events）、artifacts 下载。
func NewRESTServer(
	cfg *conf.Bootstrap, logger log.Logger, tp *tracesdk.TracerProvider,
	authenticator authnEngine.Authenticator,
	jobsSvc *service.JobsService,
	enginesSvc *service.EnginesService,
	doctorSvc *service.DoctorService,
	raw RawDeps,
	pingFn func(ctx context.Context) error,
) *http.Server {
	if cfg == nil || cfg.Server == nil || cfg.Server.Rest == nil {
		return nil
	}

	srv := newRestServer(cfg, tp, newRestMiddleware(logger, authenticator)...)

	studiov1.RegisterMediaJobsServiceHTTPServer(srv, jobsSvc)
	studiov1.RegisterMediaEnginesServiceHTTPServer(srv, enginesSvc)
	studiov1.RegisterMediaDoctorServiceHTTPServer(srv, doctorSvc)

	// ---- 裸端点（手工验签；proto 注释与 README 文档化约定）----
	sseHandler := sseapi.Handler(raw.SSEHandler)
	srv.HandleFunc("/api/v1/media/jobs/{id}/events", sseHandler)
	srv.HandleFunc("/api/v1/media/ws/events", raw.WSHub.Handler())
	srv.HandleFunc("/api/v1/media/artifacts/{job_id}/{name}", artifactapi.Handler(artifactapi.Deps{
		Verifier: raw.Verifier,
		BaseDir:  raw.ArtifactBase,
	}))

	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv.Handle("/metrics", observability.MetricsHandler())
	if pingFn != nil {
		srv.HandleFunc("/ready", observability.ReadyHandler(
			map[string]observability.CheckFunc{"postgres": pingFn}, 2*time.Second))
	}

	return srv
}

// newRestMiddleware media 中间件链（对齐 admin 的精简子集）。
func newRestMiddleware(logger log.Logger, authenticator authnEngine.Authenticator) []middleware.Middleware {
	var ms []middleware.Middleware
	ms = append(ms, logging.Server(logger))
	ms = append(ms, observability.MetricsMiddleware("media"))
	if authenticator != nil {
		ms = append(ms, authn.Server(authenticator))
	}
	return ms
}

// newRestServer 创建 REST 服务端（结构对齐 admin，配置取 media 自己的 conf 类型）。
func newRestServer(cfg *conf.Bootstrap, tp *tracesdk.TracerProvider, mds ...middleware.Middleware) *http.Server {
	var options []http.ServerOption

	if cfg.Server != nil && cfg.Server.Rest != nil {
		rest := cfg.Server.Rest

		if rest.Cors != nil {
			options = append(options, http.Filter(handlers.CORS(
				handlers.AllowedHeaders(rest.Cors.Headers),
				handlers.AllowedMethods(rest.Cors.Methods),
				handlers.AllowedOrigins(rest.Cors.Origins),
			)))
		}

		var ms []middleware.Middleware
		if rest.Middleware != nil {
			if rest.Middleware.GetEnableRecovery() {
				ms = append(ms, recovery.Recovery())
			}
			if rest.Middleware.GetEnableTracing() {
				ms = append(ms, tracing.Server(tracing.WithTracerProvider(tp)))
			}
			if rest.Middleware.GetEnableValidate() {
				ms = append(ms, validate.ProtoValidate())
			}
			if rest.Middleware.GetEnableMetadata() {
				ms = append(ms, metadata.Server())
			}
		}
		ms = append(ms, mds...)
		options = append(options, http.Middleware(ms...))

		// 统一错误语义（裸错误归一化 404/400）
		options = append(options, http.ErrorEncoder(httpx.ErrorEncoder))

		if rest.Network != "" {
			options = append(options, http.Network(rest.Network))
		}
		if rest.Addr != "" {
			options = append(options, http.Address(rest.Addr))
		}
		if rest.Timeout != nil {
			options = append(options, http.Timeout(rest.Timeout.AsDuration()))
		}
	}

	srv := http.NewServer(options...)

	if cfg.Server != nil && cfg.Server.Rest != nil && cfg.Server.Rest.GetEnablePprof() {
		registerHttpPprof(srv)
	}

	return srv
}

// registerHttpPprof 注册 pprof 调试端点。
func registerHttpPprof(s *http.Server) {
	s.HandleFunc("/debug/pprof", pprof.Index)
	s.HandleFunc("/debug/cmdline", pprof.Cmdline)
	s.HandleFunc("/debug/profile", pprof.Profile)
	s.HandleFunc("/debug/symbol", pprof.Symbol)
	s.HandleFunc("/debug/trace", pprof.Trace)

	s.HandleFunc("/debug/allocs", pprof.Handler("allocs").ServeHTTP)
	s.HandleFunc("/debug/block", pprof.Handler("block").ServeHTTP)
	s.HandleFunc("/debug/goroutine", pprof.Handler("goroutine").ServeHTTP)
	s.HandleFunc("/debug/heap", pprof.Handler("heap").ServeHTTP)
	s.HandleFunc("/debug/mutex", pprof.Handler("mutex").ServeHTTP)
	s.HandleFunc("/debug/threadcreate", pprof.Handler("threadcreate").ServeHTTP)
}
