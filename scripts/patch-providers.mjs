import fs from 'fs';
import path from 'path';

const admin = process.argv[2];
const write = (rel, s) => {
  const p = path.join(admin, rel);
  fs.writeFileSync(p, s);
  console.log('write', rel);
};

// 1) service ProviderSet
write('internal/service/service.go', `//go:build wireinject
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
)
`);

// 2) biz ProviderSet
write('internal/biz/biz.go', `//go:build wireinject
// +build wireinject

package biz

import "github.com/google/wire"

// ProviderSet is biz providers (platform-only skeleton).
var ProviderSet = wire.NewSet(
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
`);

console.log('providers written');
