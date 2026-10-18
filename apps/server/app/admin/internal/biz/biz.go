//go:build wireinject
// +build wireinject

package biz

import "github.com/google/wire"

// ProviderSet is biz providers (platform-only skeleton).
var ProviderSet = wire.NewSet(
	NewRadarTrendingUsecase,
	NewGitHubRadarTrendingReader,
	ProvideDemoRadarTrendingReader,
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
)
