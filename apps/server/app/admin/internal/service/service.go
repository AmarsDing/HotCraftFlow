//go:build wireinject
// +build wireinject

package service

import (
	"github.com/google/wire"

	"framework/app/admin/internal/biz"
)

// ProviderSet is service providers (platform-only skeleton).
var ProviderSet = wire.NewSet(
	NewAuthenticationService,
	NewUserService,
	NewMenuService,
	NewRouterService,
	NewTaskService,
	NewRoleService,
	NewOrganizationService,
	NewDepartmentService,
	NewPositionService,
	NewDictService,
	NewAdminLoginLogService,
	NewAdminOperationLogService,
	NewOssService,
	NewUEditorService,
	NewFileService,
	NewTenantService,
	NewNotificationMessageService,
	NewNotificationMessageCategoryService,
	NewNotificationMessageRecipientService,
	NewPrivateMessageService,
	NewAdminLoginRestrictionService,
	NewToolLinkService,
	NewToolLinkCategoryService,
	NewUserProfileService,
	NewUserCredentialService,
	NewApiResourceService,
	NewSeedService,
	NewSessionService,
	NewSystemConfigService,
	NewDataPermissionService,
	NewRoleDeptService,
	NewUserPermissionService,
	NewUserIOService,
	NewRadarTrendingService,
	NewRadarRepoService,
	NewRadarWatchService,
	NewRadarNotifyService,
	NewRadarConfigService,

	// ③ MIA 市场影响（FR-MIA-05~08 查询服务）
	NewMiaMappingService,
	NewMiaEventService,
	NewMiaImpactService,

	// ⑤ MAP 媒体发布矩阵（FR-PUB-01~07；审批门/账号池/分发/回执全链）
	NewMapService,

	// RadarConfigProvider 的 biz 接口由 service 的 RadarConfigService 实现，
	// 供 biz 的 ProvideRadarGitHubClient/NewInsightService/NewRadarRepoUsecase 注入。
	wire.Bind(new(biz.RadarConfigProvider), new(*RadarConfigService)),
)
