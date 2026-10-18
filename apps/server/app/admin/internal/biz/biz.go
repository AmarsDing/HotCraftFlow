//go:build wireinject
// +build wireinject

package biz

import "github.com/google/wire"

// ProviderSet is biz providers (platform-only skeleton).
var ProviderSet = wire.NewSet(
	NewRadarTrendingUsecase,
	// demo/真实选择内聚到 ProvideRadarTrendingReader（radar_wire.go），
	// NewGitHubRadarTrendingReader/ProvideDemoRadarTrendingReader 不再直挂集合，
	// 避免两个构造器同时提供同一接口值造成 wire 歧义。
	ProvideRadarTrendingReader,
	ProvideRadarGitHubClient,
	NewInsightService,
	NewRadarRepoUsecase,
	NewRadarWatchUsecase,
	NewRadarNotifyUsecase,
	NewRadarConfigUsecase,
	NewDictUsecase,
	NewTaskUsecase,
	NewFileUsecase,
	NewSystemConfigUsecase,
	NewDataPermissionUsecase,
	NewRoleDeptUsecase,
	NewUserUsecase,
	NewUserCredentialUsecase,
	NewRoleUsecase,
	NewDepartmentUsecase,
	NewPositionUsecase,
	NewOrganizationUsecase,
	NewTenantUsecase,
	NewMenuUsecase,
	NewRouterUsecase,
	NewApiResourceUsecase,
	NewNotificationMessageUsecase,
	NewNotificationMessageCategoryUsecase,
	NewNotificationMessageRecipientUsecase,
	NewPrivateMessageUsecase,
	NewAdminLoginLogUsecase,
	NewAdminOperationLogUsecase,
	NewAdminLoginRestrictionUsecase,
	NewSessionUsecase,
	NewAuthenticationUsecase,
	NewUserIOUsecase,
	NewLogExportUsecase,
	NewResourceGroupUsecase,
	NewToolLinkUsecase,
	NewToolLinkCategoryUsecase,
	// ---- ① IC 情报收集（端口实现由 ic 适配器批次补齐后方可 wire）----
	NewIcConfig,
	NewGovernanceGate,
	NewIcSourceUsecase,
	NewIcExtractUsecase,
	NewIcDedupUsecase,
	NewIcNormalizeUsecase,
	NewIcCollectUsecase,
)
