import fs from 'fs';
import path from 'path';

const admin = process.argv[2];
const restPath = path.join(admin, 'internal/server/rest.go');
let s = fs.readFileSync(restPath, 'utf8');
const start = s.indexOf('func NewRESTServer(');
const endMarker = '\n// newRestServer ';
const end = s.indexOf(endMarker, start);
if (start < 0 || end < 0) throw new Error('markers not found ' + start + ' ' + end);

const newFn = `func NewRESTServer(
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

	userV1.RegisterUserServiceHTTPServer(srv, userSvc)
	userV1.RegisterOrganizationServiceHTTPServer(srv, orgSvc)
	userV1.RegisterDepartmentServiceHTTPServer(srv, deptSvc)
	userV1.RegisterPositionServiceHTTPServer(srv, positionSvc)
	userV1.RegisterRoleServiceHTTPServer(srv, roleSvc)
	userV1.RegisterTenantServiceHTTPServer(srv, tenantService)

	adminV1.RegisterAdminLoginLogServiceHTTPServer(srv, adminLoginLogSvc)
	adminV1.RegisterAdminOperationLogServiceHTTPServer(srv, adminOperationLogSvc)
	adminV1.RegisterFileServiceHTTPServer(srv, fileService)
	adminV1.RegisterNotificationMessageServiceHTTPServer(srv, notificationMessageService)
	adminV1.RegisterNotificationMessageCategoryServiceHTTPServer(srv, notificationMessageCategoryService)
	adminV1.RegisterNotificationMessageRecipientServiceHTTPServer(srv, notificationMessageRecipientService)
	adminV1.RegisterPrivateMessageServiceHTTPServer(srv, privateMessageService)

	_ = seedSvc

	registerOssUploadHandler(srv, ossSvc)
	registerUEditorUploadHandler(srv, ueditorSvc)
	registerUserIOHandlers(srv, userIOSvc)
	registerFileDownloadHandler(srv, store)

	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(\`{"status":"ok"}\`))
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
`;

s = s.slice(0, start) + newFn + s.slice(end);
// ensure userV1 import
if (!s.includes('userV1 "twinserver/api/gen/go/user/v1"')) {
  s = s.replace(
    'adminV1 "twinserver/api/gen/go/admin/v1"',
    'adminV1 "twinserver/api/gen/go/admin/v1"\n\tuserV1 "twinserver/api/gen/go/user/v1"'
  );
}
fs.writeFileSync(restPath, s);
console.log('rest.go NewRESTServer replaced');
