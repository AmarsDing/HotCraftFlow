import fs from 'fs';

const bak = 'E:/project/TwinMonitor/FrameWork/apps/web/src/services/index.ts.biz.bak';
const outPath = 'E:/project/TwinMonitor/FrameWork/apps/web/src/services/index.ts';
const all = fs.readFileSync(bak, 'utf8').split(/\r?\n/);

const idx = (pred) => all.findIndex(pred);
const helperStart = idx((l) => l.includes('类型定义') || l.includes('json_name'));
const clientStart = idx((l) => l.includes('实例化已生成') || l.includes('实例化'));
const authStart = idx((l) => l.startsWith('// ===================== Auth'));
const assetIOStart = idx((l) => l.includes('Asset IO'));
const roleStart = idx((l) => l.startsWith('// ===================== Role'));
const airportStart = idx((l) => l.includes('Airport (') || l.includes('Airport （'));
const systemConfigStart = idx((l) => l.startsWith('// ===================== SystemConfig'));
const assetModuleStart = idx((l) => l.includes('Asset Module') || l.includes('设备资产'));

console.log({ helperStart, clientStart, authStart, assetIOStart, roleStart, airportStart, systemConfigStart, assetModuleStart });
if ([helperStart, clientStart, authStart, assetIOStart, roleStart, airportStart, systemConfigStart, assetModuleStart].some((n) => n < 0)) {
  throw new Error('marker missing');
}

const header = `// 平台骨架 API 封装：仅保留 authentication / user / admin / file。
import {
  requestHandler,
  request,
  buildQueryParams,
  buildUpdateMask,
  type ListResponse,
  type PagingRequest,
} from './request';
import {
  createAdminLoginLogServiceClient,
  createAdminLoginRestrictionServiceClient,
  createAdminOperationLogServiceClient,
  createApiResourceServiceClient,
  createAuthenticationServiceClient,
  createDataPermissionServiceClient,
  createDepartmentServiceClient,
  createDictServiceClient,
  createFileServiceClient,
  createMenuServiceClient,
  createNotificationMessageServiceClient,
  createNotificationMessageCategoryServiceClient,
  createNotificationMessageRecipientServiceClient,
  createOrganizationServiceClient,
  createPositionServiceClient,
  createPrivateMessageServiceClient,
  createRoleDeptServiceClient,
  createRoleServiceClient,
  createRouterServiceClient,
  createSessionServiceClient,
  createSystemConfigServiceClient,
  createTaskServiceClient,
  createTenantServiceClient,
  createToolLinkServiceClient,
  createToolLinkCategoryServiceClient,
  createUserServiceClient,
  createUserProfileServiceClient,
  type pagination_PagingRequest,
  type SyncApiResourcesResponse,
} from './kratos/admin/service/v1';
`;

const helpers = all.slice(helperStart, clientStart).join('\n');
const clients = `
const authenticationClient = createAuthenticationServiceClient(requestHandler);
const userClient = createUserServiceClient(requestHandler);
const userProfileClient = createUserProfileServiceClient(requestHandler);
const menuClient = createMenuServiceClient(requestHandler);
const routerClient = createRouterServiceClient(requestHandler);
const organizationClient = createOrganizationServiceClient(requestHandler);
const departmentClient = createDepartmentServiceClient(requestHandler);
const positionClient = createPositionServiceClient(requestHandler);
const roleClient = createRoleServiceClient(requestHandler);
const dictClient = createDictServiceClient(requestHandler);
const tenantClient = createTenantServiceClient(requestHandler);
const taskClient = createTaskServiceClient(requestHandler);
const apiResourceClient = createApiResourceServiceClient(requestHandler);
const loginLogClient = createAdminLoginLogServiceClient(requestHandler);
const operationLogClient = createAdminOperationLogServiceClient(requestHandler);
const loginRestrictionClient = createAdminLoginRestrictionServiceClient(requestHandler);
const notificationMessageClient = createNotificationMessageServiceClient(requestHandler);
const notificationMessageCategoryClient = createNotificationMessageCategoryServiceClient(requestHandler);
const notificationMessageRecipientClient = createNotificationMessageRecipientServiceClient(requestHandler);
const privateMessageClient = createPrivateMessageServiceClient(requestHandler);
const fileClient = createFileServiceClient(requestHandler);
const systemConfigClient = createSystemConfigServiceClient(requestHandler);
const sessionClient = createSessionServiceClient(requestHandler);
const dataPermissionClient = createDataPermissionServiceClient(requestHandler);
const roleDeptClient = createRoleDeptServiceClient(requestHandler);
const toolLinkClient = createToolLinkServiceClient(requestHandler);
const toolLinkCategoryClient = createToolLinkCategoryServiceClient(requestHandler);
`;

const authToUserIO = all.slice(authStart, assetIOStart).join('\n');
const roleToRouter = all.slice(roleStart, airportStart).join('\n');
const systemToRoleDept = all.slice(systemConfigStart, assetModuleStart).join('\n');

const out = [header, helpers, clients, authToUserIO, roleToRouter, systemToRoleDept, ''].join('\n\n');
fs.writeFileSync(outPath, out);
console.log({
  bytes: Buffer.byteLength(out, 'utf8'),
  lines: out.split(/\n/).length,
  collector: out.includes('kratos/collector'),
  airport: /airportService|createAirport/.test(out),
  authService: out.includes('export const authService'),
  userService: out.includes('export const userService'),
});
