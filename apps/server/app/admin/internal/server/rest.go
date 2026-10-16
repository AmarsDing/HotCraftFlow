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
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/transport/http"

	"github.com/go-kratos/kratos/contrib/middleware/validate/v2"
	"github.com/gorilla/handlers"

	authnEngine "github.com/tx7do/kratos-authn/engine"
	authn "github.com/tx7do/kratos-authn/middleware"
	authz "github.com/tx7do/kratos-authz/middleware"

	swaggerUI "github.com/tx7do/kratos-swagger-ui"

	"framework/app/admin/cmd/assets"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	"framework/app/admin/internal/data"
	appmw "framework/app/admin/internal/middleware"
	"framework/app/admin/internal/service"

	adminV1 "framework/api/gen/go/admin/v1"

	radarV1 "framework/api/gen/go/radar/v1"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"

	"framework/pkg/httpx"
	"framework/pkg/middleware/auth"
	applogging "framework/pkg/middleware/logging"
	"framework/pkg/observability"
	"framework/pkg/serviceauth"
	"framework/pkg/storage"
)

// NewWhiteListMatcher 创建jwt白名单
func newRestWhiteListMatcher() selector.MatchFunc {
	whiteList := make(map[string]bool)
	whiteList[adminV1.OperationAuthenticationServiceLogin] = true
	// RefreshToken 在 access token 过期后调用，请求不带有效 JWT，
	// 用户身份由 refresh_token 记录反查（见 LoginByRefreshToken），故免 JWT 认证
	whiteList[adminV1.OperationAuthenticationServiceRefreshToken] = true
	// VerifyMFA 属于登录流程二步，调用时无 access token，需免认证
	whiteList[adminV1.OperationAuthenticationServiceVerifyMFA] = true
	return func(ctx context.Context, operation string) bool {
		if _, ok := whiteList[operation]; ok {
			return false
		}
		return true
	}
}

// NewMiddleware 创建中间件
func newRestMiddleware(
	logger log.Logger,
	authenticator authnEngine.Authenticator,
	authorizer *data.Authorizer,
	operationLogRepo *data.AdminOperationLogRepo,
	loginLogRepo *data.AdminLoginLogRepo,
	userTokenRepo *data.UserTokenCacheRepo,
	dataPermissionUC *biz.DataPermissionUsecase,
) []middleware.Middleware {
	var ms []middleware.Middleware
	ms = append(ms, logging.Server(logger))

	// 空过滤 JSON（query="{}" / "[]"）归一化，防止 entgo 生成残缺 WHERE SQL（全部 List 接口生效）
	ms = append(ms, appmw.SanitizePagingQuery())

	// Prometheus HTTP 请求指标（1.10 可观测性统一接入）
	ms = append(ms, observability.MetricsMiddleware("admin"))

	// 审计日志中间件内部为异步队列落库（容量 1024，每 500ms 或满 50 条批量写入），不阻塞请求关键路径
	ms = append(ms, applogging.Server(
		applogging.WithLogger(logger),
		applogging.WithWriteOperationLogFunc(func(ctx context.Context, data *adminV1.AdminOperationLog) error {
			return operationLogRepo.Create(ctx, &adminV1.CreateAdminOperationLogRequest{Data: data})
		}),
		applogging.WithWriteLoginLogFunc(func(ctx context.Context, data *adminV1.AdminLoginLog) error {
			return loginLogRepo.Create(ctx, &adminV1.CreateAdminLoginLogRequest{Data: data})
		}),
	))

	// Token 黑名单：校验 access token 是否未被撤销（登出/强制下线后 revoked=true）。
	// uid=0 为服务间调用令牌（opstools 等内部服务以共享密钥现签，不落 user_tokens 表），
	// 跳过黑名单校验；签名合法性与接口级访问控制仍由 authn/casbin 保障。
	isExistAccessToken := func(ctx context.Context, userId uint32, token string) bool {
		if userId == 0 {
			return true
		}
		return userTokenRepo.IsExistAccessToken(ctx, userId, token)
	}

	ms = append(ms, selector.Server(
		authn.Server(authenticator),
		auth.Server(auth.WithIsExistAccessTokenFunc(isExistAccessToken)),
		// 服务令牌（uid=0）最小权限围栏：按 serviceTokenRules 校验 scp，未登记操作一律 403。
		// 用户令牌不受影响（鉴权仍由 casbin 承担）。
		serviceauth.Enforce(logger, serviceTokenRules...),
		authz.Server(authorizer.Engine()),
		// 数据权限：在认证/鉴权之后构建 viewer.DeptIDs（白名单接口自动跳过）
		appmw.DataPermission(dataPermissionUC, logger),
	).Match(newRestWhiteListMatcher()).Build())

	return ms
}

// NewRESTServer new an HTTP server.
func NewRESTServer(
	cfg *conf.Bootstrap, logger log.Logger, tp *tracesdk.TracerProvider,
	authenticator authnEngine.Authenticator, authorizer *data.Authorizer,
	data *data.Data,
	operationLogRepo *data.AdminOperationLogRepo,
	loginLogRepo *data.AdminLoginLogRepo,
	userTokenRepo *data.UserTokenCacheRepo,
	dataPermissionUC *biz.DataPermissionUsecase,
	authnSvc *service.AuthenticationService,
	userSvc *service.UserService,
	menuSvc *service.MenuService,
	routerSvc *service.RouterService,
	orgSvc *service.OrganizationService,
	roleSvc *service.RoleService,
	positionSvc *service.PositionService,
	dictSvc *service.DictService,
	deptSvc *service.DepartmentService,
	adminLoginLogSvc *service.AdminLoginLogService,
	adminOperationLogSvc *service.AdminOperationLogService,
	ossSvc *service.OssService,
	ueditorSvc *service.UEditorService,
	fileService *service.FileService,
	tenantService *service.TenantService,
	taskService *service.TaskService,
	notificationMessageService *service.NotificationMessageService,
	notificationMessageCategoryService *service.NotificationMessageCategoryService,
	notificationMessageRecipientService *service.NotificationMessageRecipientService,
	privateMessageService *service.PrivateMessageService,
	adminLoginRestrictionService *service.AdminLoginRestrictionService,
	toolLinkService *service.ToolLinkService,
	toolLinkCategoryService *service.ToolLinkCategoryService,
	userProfileService *service.UserProfileService,
	apiResourceService *service.ApiResourceService,
	seedSvc *service.SeedService,
	sessionSvc *service.SessionService,
	systemConfigSvc *service.SystemConfigService,
	dataPermissionSvc *service.DataPermissionService,
	roleDeptSvc *service.RoleDeptService,
	userPermissionSvc *service.UserPermissionService,
	userIOSvc *service.UserIOService,
	radarTrendingSvc *service.RadarTrendingService,
	radarWatchSvc *service.RadarWatchService,
	radarNotifySvc *service.RadarNotifyService,
	radarConfigSvc *service.RadarConfigService,
	store *storage.LocalStorage,
) *http.Server {
	if cfg == nil || cfg.Server == nil || cfg.Server.Rest == nil {
		return nil
	}

	srv := newRestServer(cfg, tp,
		newRestMiddleware(logger, authenticator, authorizer, operationLogRepo, loginLogRepo, userTokenRepo, dataPermissionUC)...,
	)

	adminV1.RegisterAuthenticationServiceHTTPServer(srv, authnSvc)
	adminV1.RegisterUserProfileServiceHTTPServer(srv, userProfileService)
	adminV1.RegisterMenuServiceHTTPServer(srv, menuSvc)
	adminV1.RegisterRouterServiceHTTPServer(srv, routerSvc)
	adminV1.RegisterDictServiceHTTPServer(srv, dictSvc)
	adminV1.RegisterTaskServiceHTTPServer(srv, taskService)
	adminV1.RegisterAdminLoginRestrictionServiceHTTPServer(srv, adminLoginRestrictionService)
	adminV1.RegisterToolLinkServiceHTTPServer(srv, toolLinkService)
	adminV1.RegisterToolLinkCategoryServiceHTTPServer(srv, toolLinkCategoryService)
	adminV1.RegisterApiResourceServiceHTTPServer(srv, apiResourceService)
	adminV1.RegisterSessionServiceHTTPServer(srv, sessionSvc)
	adminV1.RegisterSystemConfigServiceHTTPServer(srv, systemConfigSvc)
	adminV1.RegisterDataPermissionServiceHTTPServer(srv, dataPermissionSvc)
	adminV1.RegisterRoleDeptServiceHTTPServer(srv, roleDeptSvc)
	adminV1.RegisterUserPermissionServiceHTTPServer(srv, userPermissionSvc)

	adminV1.RegisterUserServiceHTTPServer(srv, userSvc)
	adminV1.RegisterOrganizationServiceHTTPServer(srv, orgSvc)
	adminV1.RegisterDepartmentServiceHTTPServer(srv, deptSvc)
	adminV1.RegisterPositionServiceHTTPServer(srv, positionSvc)
	adminV1.RegisterRoleServiceHTTPServer(srv, roleSvc)
	adminV1.RegisterTenantServiceHTTPServer(srv, tenantService)

	adminV1.RegisterAdminLoginLogServiceHTTPServer(srv, adminLoginLogSvc)
	adminV1.RegisterAdminOperationLogServiceHTTPServer(srv, adminOperationLogSvc)
	adminV1.RegisterFileServiceHTTPServer(srv, fileService)
	adminV1.RegisterNotificationMessageServiceHTTPServer(srv, notificationMessageService)
	adminV1.RegisterNotificationMessageCategoryServiceHTTPServer(srv, notificationMessageCategoryService)
	adminV1.RegisterNotificationMessageRecipientServiceHTTPServer(srv, notificationMessageRecipientService)
	adminV1.RegisterPrivateMessageServiceHTTPServer(srv, privateMessageService)

	radarV1.RegisterTrendingServiceHTTPServer(srv, radarTrendingSvc)
	radarV1.RegisterWatchServiceHTTPServer(srv, radarWatchSvc)
	radarV1.RegisterRadarNotifyServiceHTTPServer(srv, radarNotifySvc)
	radarV1.RegisterRadarConfigServiceHTTPServer(srv, radarConfigSvc)

	_ = seedSvc

	registerOssUploadHandler(srv, ossSvc)
	registerUEditorUploadHandler(srv, ueditorSvc)
	registerUserIOHandlers(srv, userIOSvc)
	registerFileDownloadHandler(srv, store)

	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv.Handle("/metrics", observability.MetricsHandler())
	srv.HandleFunc("/ready", observability.ReadyHandler(data.PingChecks(), 2*time.Second))

	if cfg.GetServer().GetRest().GetEnableSwagger() {
		swaggerUI.RegisterSwaggerUIServerWithOption(
			srv,
			swaggerUI.WithTitle("Framework Admin"),
			swaggerUI.WithMemoryData(assets.OpenApiData, "yaml"),
		)
	}

	return srv
}

// newRestServer 创建 REST 服务端（替代 kratos-bootstrap rpc.CreateRestServer，直接使用本项目 conf 类型）
// tp 通过 wire 显式注入（仿照 v1），不依赖全局 otel provider；tp 为 nil 时 tracing 将 fallback 到 noop tracer。
func newRestServer(cfg *conf.Bootstrap, tp *tracesdk.TracerProvider, mds ...middleware.Middleware) *http.Server {
	var options []http.ServerOption

	if cfg.Server != nil && cfg.Server.Rest != nil {
		rest := cfg.Server.Rest

		// CORS
		if rest.Cors != nil {
			options = append(options, http.Filter(handlers.CORS(
				handlers.AllowedHeaders(rest.Cors.Headers),
				handlers.AllowedMethods(rest.Cors.Methods),
				handlers.AllowedOrigins(rest.Cors.Origins),
			)))
		}

		// 内置中间件
		var ms []middleware.Middleware
		if rest.Middleware != nil {
			if rest.Middleware.GetEnableRecovery() {
				ms = append(ms, recovery.Recovery())
			}
			if rest.Middleware.GetEnableTracing() {
				// 仿照 v1：显式注入 TracerProvider，而非依赖 otel.SetTracerProvider 全局设置
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

		// 统一错误语义：裸错误（ent not found / 参数校验）映射为 404/400
		options = append(options, http.ErrorEncoder(httpx.ErrorEncoder))

		// 监听配置
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

	// pprof
	if cfg.Server != nil && cfg.Server.Rest != nil && cfg.Server.Rest.GetEnablePprof() {
		registerHttpPprof(srv)
	}

	return srv
}

// registerHttpPprof 注册 pprof 调试端点
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
