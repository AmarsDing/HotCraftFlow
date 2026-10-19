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

	// ② TT 热点追踪（M2 权重配置页：查看 + 变更，A-15 断点口径）
	NewTtWeightService,

	// ② TT M3 话题人工管理（合并/拆分/复核；Step5 merge_lock 写入路径）
	NewTtTopicAdminService,

	// ① IC 情报收集（信源/规则 + 情报治理 + 采集器管理三服务；02 §6.2 路由面）
	NewIcSourceService,
	NewIcIntelService,
	NewIcCollectorService,

	// ⑤ MAP 媒体发布矩阵（FR-PUB-01~07；审批门/账号池/分发/回执全链）
	NewMapService,

	// ④ MME 短视频矩阵混剪（任务/变体簇/渲染队列三路 REST）
	NewMmeTaskService,
	NewMmeVariantService,
	NewMmeRenderService,

	// RadarConfigProvider 的 biz 接口由 service 的 RadarConfigService 实现，
	// 供 biz 的 ProvideRadarGitHubClient/NewInsightService/NewRadarRepoUsecase 注入。
	wire.Bind(new(biz.RadarConfigProvider), new(*RadarConfigService)),
)
