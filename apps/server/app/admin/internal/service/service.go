//go:build wireinject
// +build wireinject

package service

import (
	"github.com/google/wire"
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
	NewRadarWatchService,
	NewRadarNotifyService,
	NewRadarConfigService,
)
