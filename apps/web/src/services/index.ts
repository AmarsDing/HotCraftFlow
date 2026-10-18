// 平台骨架 API 封装：仅保留 authentication / user / admin / file。
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

import {
  createTrendingServiceClient,
  createWatchServiceClient,
  createRadarNotifyServiceClient,
  createRadarConfigServiceClient,
  createRepoServiceClient,
  type RepoItem as RadarRepoItem,
  type WatchItem as RadarWatchItem,
  type WatchEvent as RadarWatchEvent,
  type RadarNotification,
  type ListTrendingResponse,
  type SearchRisingResponse,
  type GetRadarConfigResponse,
  type GetRepoResponse,
  type GetRepoTreeResponse,
  type GetRepoContentResponse,
  type CloneRepoResponse,
  type GenerateInsightResponse,
  type AIInsight as RadarAIInsight,
} from './kratos/radar/service/v1';


// ===================== 类型定义（与后端 proto json_name 对齐） =====================
// 保留手写类型以保证 stores/pages 的兼容性；生成的客户端返回值会被 cast 到这些类型。
export interface LoginRequest {
  grant_type: string;
  username: string;
  password: string;
  refresh_token?: string;
  mfaCode?: string;
}
export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in?: number;
  mfaToken?: string;
  mfaRequired?: boolean;
}
export interface UserProfile {
  id: string;
  username: string;
  nickname: string;
  realname?: string;
  avatar?: string;
  email?: string;
  mobile?: string;
  telephone?: string;
  gender?: string;
  address?: string;
  region?: string;
  description?: string;
  authority?: string;
  tenantId?: string;
  orgId?: string;
  positionId?: string;
  workId?: string;
  status?: string;
  lastLoginTime?: string;
  lastLoginIp?: string;
  mfaEnabled?: boolean; // MFA 启用状态（B7：User DTO 下发 mfa_enabled）
  roles?: string[]; // 角色码列表（super/gly/xx/yb/gc/jw），getMe 返回时后端已填充
}

export interface CrudEntity {
  id?: string | number;
  createTime?: string;
  updateTime?: string;
  createBy?: string;
  updateBy?: string;
}

// ===================== 通用 CRUD 适配器 =====================
// 将生成的 PascalCase 客户端适配为 stores 期望的 lowercase 方法签名。
// 修复旧代码 bug：stores/modules.ts 的 makeCrudStore 调用 service.delete(id)，
// 但旧 makeCrudService 返回的是 remove()，导致删除功能不可用。此处统一为 delete。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
interface CrudClient {
  List: (req: any) => Promise<any>;
  Get: (req: any) => Promise<any>;
  Create: (req: any) => Promise<any>;
  Update: (req: any) => Promise<any>;
  Delete: (req: any) => Promise<any>;
}

function makeCrudServiceFromClient<T extends CrudEntity>(client: CrudClient) {
  return {
    list(opts: PagingRequest & { formValues?: Record<string, unknown> } = {}) {
      const params = buildQueryParams(opts.formValues, opts);
      // C6：`or` 查询参数已由生成客户端承载（pagination_PagingRequest.or，proto or_query json_name="or"），不再绕行 axios。
      return client.List({
        page: params.page ? Number(params.page) : undefined,
        pageSize: params.pageSize ? Number(params.pageSize) : undefined,
        noPaging: params.noPaging === 'true' ? true : undefined,
        query: params.query,
        or: opts.orQuery,
        orderBy: params.orderBy,
        fieldMask: params.fieldMask,
      }) as Promise<ListResponse<T>>;
    },
    get(id: string | number) {
      return client.Get({ id: Number(id) }) as Promise<T>;
    },
    create(data: Partial<T>) {
      return client.Create({ data }) as Promise<unknown>;
    },
    update(id: string | number, data: Partial<T>) {
      // id 作为顶层字段（URL 路径参数 {id}），不再注入 data，避免与 body 中的 data.id 冲突。
      const updateMask = buildUpdateMask(data as Record<string, unknown>);
      return client.Update({ id: Number(id), data: data as never, updateMask }) as Promise<unknown>;
    },
    delete(id: string | number) {
      return client.Delete({ id: Number(id) }) as Promise<unknown>;
    },
  };
}

// 无 Get rpc 的服务客户端（如 AssetTagService/KnowledgeTagService）专用变体
interface ListWriteClient {
  List: (req: any) => Promise<any>;
  Create: (req: any) => Promise<any>;
  Update: (req: any) => Promise<any>;
  Delete: (req: any) => Promise<any>;
}

function makeListWriteServiceFromClient<T extends CrudEntity>(client: ListWriteClient) {
  return {
    list(opts: PagingRequest & { formValues?: Record<string, unknown> } = {}) {
      const params = buildQueryParams(opts.formValues, opts);
      return client.List({
        page: params.page ? Number(params.page) : undefined,
        pageSize: params.pageSize ? Number(params.pageSize) : undefined,
        noPaging: params.noPaging === 'true' ? true : undefined,
        query: params.query,
        orderBy: params.orderBy,
        fieldMask: params.fieldMask,
      }) as Promise<ListResponse<T>>;
    },
    create(data: Partial<T>) {
      return client.Create({ data }) as Promise<unknown>;
    },
    update(id: string | number, data: Partial<T>) {
      const updateMask = buildUpdateMask(data as Record<string, unknown>);
      return client.Update({ id: Number(id), data: data as never, updateMask }) as Promise<unknown>;
    },
    delete(id: string | number) {
      return client.Delete({ id: Number(id) }) as Promise<unknown>;
    },
  };
}



const authClient = createAuthenticationServiceClient(requestHandler);
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
const adminLoginLogClient = createAdminLoginLogServiceClient(requestHandler);
const adminOperationLogClient = createAdminOperationLogServiceClient(requestHandler);
const adminLoginRestrictionClient = createAdminLoginRestrictionServiceClient(requestHandler);
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

const radarTrendingClient = createTrendingServiceClient(requestHandler);
const radarWatchClient = createWatchServiceClient(requestHandler);
const radarNotifyClient = createRadarNotifyServiceClient(requestHandler);
const radarConfigClient = createRadarConfigServiceClient(requestHandler);
const radarRepoClient = createRepoServiceClient(requestHandler);


// ===================== Auth =====================
export const authService = {
  login: (data: LoginRequest) =>
    authClient.Login({
      grant_type: data.grant_type,
      username: data.username,
      password: data.password,
      refresh_token: data.refresh_token,
      mfaCode: data.mfaCode,
    }) as Promise<LoginResponse>,
  logout: () => authClient.Logout({}) as Promise<unknown>,
  refreshToken: (refresh_token: string) =>
    authClient.RefreshToken({ grant_type: 'refresh_token', refresh_token }) as Promise<LoginResponse>,
  changePassword: (data: { oldPassword?: string; newPassword: string; userId?: number }) =>
    authClient.ChangePassword({
      username: undefined,
      oldPassword: data.oldPassword,
      newPassword: data.newPassword,
      userId: data.userId,
    }) as Promise<unknown>,
  verifyMFA: (mfaToken: string, mfaCode: string) =>
    authClient.VerifyMFA({ mfaToken, mfaCode }) as Promise<LoginResponse>,
  enableMFA: () => authClient.EnableMFA({}) as Promise<{ secret: string; otpauthUrl: string }>,
  // 确认 MFA 绑定（B7）：扫码后校验首个 TOTP 验证码，通过才正式启用
  confirmMFA: (mfaCode: string) => authClient.ConfirmMFA({ mfaCode }) as Promise<unknown>,
  disableMFA: (mfaCode: string) => authClient.DisableMFA({ mfaCode }) as Promise<unknown>,
  // 生成类型 User.id 为 number，而手写 UserProfile.id 为 string，需经 unknown 中转
  getMe: () => userProfileClient.GetUser({}) as unknown as Promise<UserProfile>,
  updateMe: (data: Partial<UserProfile>) => userProfileClient.UpdateUser({ data } as never) as Promise<unknown>,
  // 设置独立操作密码（§2.6.6，6 位数字；已设置过时 oldOperationPassword 必填）。
  // 生成的客户端未覆盖该端点，直接使用 axios 实例，与 credentialService.getSecret 同模式。
  setOperationPassword: (data: { operationPassword: string; oldOperationPassword?: string }) =>
    request.post('/api/v1/monitor/users/me/operation-password', data),
};

// ===================== User =====================
export interface UserEntity extends CrudEntity {
  username: string;
  password?: string;
  nickname: string;
  realname?: string;
  avatar?: string;
  email?: string;
  mobile?: string;
  telephone?: string;
  gender?: string;
  address?: string;
  region?: string;
  authority?: string;
  orgId?: string;
  positionId?: string;
  tenantId?: string;
  workId?: string;
  roles?: string[];
  status?: string;
  remark?: string;
  lastLoginTime?: string;
  lastLoginIp?: string;
  // 岗位ID列表（关联 airport_posts.id，ap_org_user_post 关联表持久化）
  postIds?: number[];
}
// CreateUserRequest/UpdateUserRequest 中 password 为顶层字段（非 data 内），生成的客户端已处理
export const userService = {
  ...makeCrudServiceFromClient<UserEntity>(userClient),
  // 上传头像到 avatars 桶，返回存储相对路径（存入 User.avatar 字段；展示走 fileService.fetchBlob 鉴权拉取）
  uploadAvatar(file: File): Promise<string> {
    const ext = (file.name.split('.').pop() || 'png').toLowerCase();
    return uploadToBucket(file, `${crypto.randomUUID()}.${ext}`, undefined, 'avatars');
  },
  create(data: Partial<UserEntity> & { password?: string }) {
    const { password, ...userData } = data;
    return userClient.Create({ data: userData as never, password }) as Promise<unknown>;
  },
  update(id: string | number, data: Partial<UserEntity> & { password?: string }) {
    const { password, ...userData } = data;
    const updateMask = buildUpdateMask(userData as Record<string, unknown>);
    // id 作为顶层字段（URL 路径参数 {id}），不再注入 data，避免与 body 中的 data.id 冲突。
    return userClient.Update({ id: Number(id), data: userData as never, password, updateMask }) as Promise<unknown>;
  },
};

// ===================== User IO（1.6 用户导入导出 + 审计日志导出） =====================
// 手写路由（非 proto 生成）：Excel 二进制下载 + multipart 上传。
export interface ImportRowError {
  row: number;
  username: string;
  message: string;
}
export interface ImportUsersResult {
  total: number;
  successCount: number;
  failedCount: number;
  errors?: ImportRowError[];
}

function excelFileName(prefix: string): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${prefix}_${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}_${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}.xlsx`;
}

function downloadBlob(data: Blob, filename: string) {
  const url = URL.createObjectURL(data);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

async function downloadExcel(url: string, filename: string, query?: string, orQuery?: string) {
  const params: Record<string, string> = {};
  if (query) params.query = query;
  if (orQuery) params.or = orQuery;
  const blob = await request.get<Blob>(url, {
    params: Object.keys(params).length > 0 ? params : undefined,
    responseType: 'blob',
    timeout: 120000,
  });
  downloadBlob(blob, filename);
}

export const userIOService = {
  exportUsers(query?: string) {
    return downloadExcel('/api/v1/monitor/users:export', excelFileName('users'), query);
  },
  downloadImportTemplate() {
    return downloadExcel('/api/v1/monitor/users:import-template', 'user_import_template.xlsx');
  },
  importUsers(file: File) {
    const fd = new FormData();
    fd.append('file', file);
    return request.post<ImportUsersResult>('/api/v1/monitor/users:import', fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 120000,
    });
  },
  exportLoginLogs(query?: string) {
    return downloadExcel('/api/v1/monitor/admin-login-logs:export', excelFileName('login_logs'), query);
  },
  exportOperationLogs(query?: string) {
    return downloadExcel('/api/v1/monitor/admin-operation-logs:export', excelFileName('operation_logs'), query);
  },
};


// ===================== Role =====================
export interface RoleEntity extends CrudEntity {
  name: string;
  code?: string;
  sortId?: number;
  status?: string;
  remark?: string;
  parentId?: string;
  menus?: number[];
  apis?: number[];
}
export const roleService = makeCrudServiceFromClient<RoleEntity>(roleClient);

// ===================== Menu =====================
export interface MenuMeta {
  title?: string;
  icon?: string;
  activeIcon?: string;
  hideInMenu?: boolean;
  hideInTab?: boolean;
  hideInBreadcrumb?: boolean;
  hideChildrenInMenu?: boolean;
  keepAlive?: boolean;
  affixTab?: boolean;
  affixTabOrder?: number;
  openInNewWindow?: boolean;
  order?: number;
  authority?: string[];
  link?: string;
  iframeSrc?: string;
  ignoreAccess?: boolean;
  divider?: boolean;
}
export interface MenuEntity extends CrudEntity {
  name: string;
  path?: string;
  component?: string;
  redirect?: string;
  alias?: string;
  parentId?: string;
  type?: string;
  status?: string;
  meta?: MenuMeta;
  children?: MenuEntity[];
}
export interface MenuImpact {
  roleCount?: number;
  userCount?: number;
}
export interface MenuRoleRef {
  menuId?: number;
  roleIds?: number[];
}
export const menuService = {
  ...makeCrudServiceFromClient<MenuEntity>(menuClient),
  // 查询菜单影响面（删除/禁用前评估：引用角色数与波及用户数）
  getImpact(id: string | number): Promise<MenuImpact> {
    return menuClient.GetImpact({ id: Number(id) }) as Promise<MenuImpact>;
  },
  // 查询全部菜单的角色引用关系（列表徽标 + 角色清单抽屉）
  listRoleRefs(): Promise<{ items?: MenuRoleRef[] }> {
    return menuClient.ListRoleRefs({}) as Promise<{ items?: MenuRoleRef[] }>;
  },
};

// ===================== Organization =====================
export interface OrganizationEntity extends CrudEntity {
  name: string;
  parentId?: string;
  sortId?: number;
  status?: string;
  remark?: string;
}
export const organizationService = makeCrudServiceFromClient<OrganizationEntity>(organizationClient);

// ===================== Department =====================
export interface DepartmentEntity extends CrudEntity {
  name: string;
  parentId?: string;
  organizationId?: string;
  organizationName?: string;
  sortId?: number;
  status?: string;
  remark?: string;
  /** 后端 List 返回嵌套树时的子部门节点 */
  children?: DepartmentEntity[];
  /** 本部门直接管理的资产数（后端 fillAssetCounts 填充，不向父部门聚合） */
  assetCount?: number;
  /** 部门领导用户ID（显式指定，中心值班页值班领导识别主口径） */
  leaderId?: string;
  /** 部门领导姓名（后端 fillLeaderNames 联表只读） */
  leaderName?: string;
}
export const departmentService = makeCrudServiceFromClient<DepartmentEntity>(departmentClient);

// ===================== Position =====================
export interface PositionEntity extends CrudEntity {
  name: string;
  code?: string;
  parentId?: string;
  sortId?: number;
  status?: string;
  remark?: string;
}
export const positionService = makeCrudServiceFromClient<PositionEntity>(positionClient);

// ===================== Dict =====================
export interface DictEntity extends CrudEntity {
  key: string;
  category?: string;
  categoryDesc?: string;
  value?: string;
  valueDesc?: string;
  valueDataType?: string | null;
  sortId?: number;
  status?: string;
  remark?: string;
}

// DictCategory 字典分类实体（P2 分类实体化）
// 分类由条目冗余字符串升级为独立实体；kind/builtin/consumers 由服务端注册表权威声明（只读）。
export interface DictCategoryEntity {
  id?: number;
  code: string;
  name?: string;
  kind?: string; // enum 枚举选项 / param 系统参数
  builtin?: boolean;
  consumers?: string[];
  sortId?: number;
  remark?: string;
  entryCount?: number;
  createTime?: string;
  updateTime?: string;
}

export const dictService = {
  ...makeCrudServiceFromClient<DictEntity>(dictClient),
  // 分类实体 CRUD（分类量级小，不分页；kind 过滤由前端处理）
  listCategories(): Promise<{ items?: DictCategoryEntity[]; total?: number }> {
    return dictClient.ListCategories({}) as Promise<{ items?: DictCategoryEntity[]; total?: number }>;
  },
  createCategory(data: Partial<DictCategoryEntity>) {
    return dictClient.CreateCategory({ data: data as never });
  },
  updateCategory(code: string, data: Partial<DictCategoryEntity>) {
    return dictClient.UpdateCategory({ code, data: data as never });
  },
  deleteCategory(code: string) {
    return dictClient.DeleteCategory({ code });
  },
};

// ===================== Tenant =====================
export interface TenantEntity extends CrudEntity {
  name: string;
  code?: string;
  memberCount?: number;
  subscriptionAt?: string;
  unsubscribeAt?: string;
  status?: string;
  remark?: string;
}
export const tenantService = makeCrudServiceFromClient<TenantEntity>(tenantClient);

// ===================== ToolLink（工具链接门户） =====================
export interface ToolLinkEntity extends CrudEntity {
  name?: string;
  url?: string;
  iconType?: 'LUCIDE' | 'IMAGE';
  iconValue?: string; // LUCIDE=lucide 图标名；IMAGE=本地存储相对路径
  category?: string;
  description?: string;
  sort?: number;
  external?: boolean;
  status?: string;
  categoryId?: number; // 分类ID（引用 tool_link_categories）
  healthStatus?: 'UNKNOWN' | 'UP' | 'DOWN'; // 健康状态（健康检测写回）
  lastCheckAt?: string; // 最近一次健康检测时间
  clickCount?: number; // 累计点击次数（全用户）
  visibleRoles?: string[]; // 可见角色码列表（空=全员可见）
  linkType?: 'NORMAL' | 'SSO'; // 链接类型（SSO=免密跳转签票）
  nameI18n?: Record<string, string>; // 名称多语言（locale->名称）
  descriptionI18n?: Record<string, string>; // 描述多语言
  favorite?: boolean; // 当前用户是否已收藏（响应计算字段）
  myClickCount?: number; // 当前用户点击次数（响应计算字段）
}
export const toolLinkService = {
  ...makeCrudServiceFromClient<ToolLinkEntity>(toolLinkClient),
  move: (id: string | number, direction: 'UP' | 'DOWN') =>
    toolLinkClient.Move({ id: Number(id), direction }) as Promise<unknown>,
  // 批量重排（拖拽排序，ids 按目标展示顺序）
  reorder: (ids: Array<string | number>) => toolLinkClient.Reorder({ ids: ids.map(Number) }) as Promise<unknown>,
  // 记录一次点击（点击统计 + 常用工具）
  click: (id: string | number) => toolLinkClient.Click({ id: Number(id) }) as Promise<unknown>,
  // 收藏/取消收藏
  favorite: (id: string | number, favorite: boolean) =>
    toolLinkClient.Favorite({ id: Number(id), favorite }) as Promise<unknown>,
  // 手动触发健康检测，返回更新后的记录
  checkHealth: (id: string | number) => toolLinkClient.CheckHealth({ id: Number(id) }) as Promise<ToolLinkEntity>,
  // 抓取目标站点 favicon，返回本地存储相对路径（写入 iconValue，iconType=IMAGE）
  fetchIcon: (url: string): Promise<string> =>
    toolLinkClient.FetchIcon({ url }).then((resp) => {
      const iconValue = resp?.iconValue || '';
      if (!iconValue) return Promise.reject(new Error('抓取图标返回为空'));
      return iconValue;
    }),
  // 导出工具链接（json/yaml）
  exportLinks: (format: 'json' | 'yaml') =>
    toolLinkClient.Export({ format }) as Promise<{ content?: string; format?: string; count?: number }>,
  // 导入工具链接（json/yaml=导出格式，html=浏览器书签）
  importLinks: (format: 'json' | 'yaml' | 'html', content: string) =>
    toolLinkClient.Import({ format, content }) as Promise<{
      created?: number;
      skipped?: number;
      failed?: number;
    }>,
  // 签发 SSO 跳转票据，返回拼接票据后的最终跳转 URL
  issueSsoTicket: (id: string | number) =>
    toolLinkClient.IssueSsoTicket({ id: Number(id) }) as Promise<{
      ticket?: string;
      url?: string;
      expireAt?: string;
    }>,
  // 上传自定义图标（multipart 手写路由），返回本地存储相对路径（存入 iconValue）
  uploadIcon(file: File): Promise<string> {
    const fd = new FormData();
    fd.append('file', file);
    return request
      .post<{ url?: string }>('/api/v1/monitor/file:upload', fd, {
        params: { bucketName: 'images', objectName: `tool-links/${Date.now()}_${file.name}` },
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      .then((resp) => {
        // 后端返回 { url: "/api/v1/monitor/file:download?path=<rel>" }，提取 path 作为存储值
        const url = resp?.url || '';
        const idx = url.indexOf('path=');
        const rel = idx >= 0 ? decodeURIComponent(url.slice(idx + 5)) : url;
        if (!rel) return Promise.reject(new Error('上传返回缺少文件路径'));
        return rel;
      });
  },
  // 拉取图标 Blob（行内预览用，Authorization 头由 request 拦截器注入）
  fetchIconBlob(relPath: string): Promise<Blob> {
    return request.get<Blob>('/api/v1/monitor/file:download', {
      params: { path: relPath },
      responseType: 'blob',
    });
  },
};

// ===================== ToolLinkCategory（工具链接分类） =====================
export interface ToolLinkCategoryEntity extends CrudEntity {
  name?: string;
  icon?: string; // Lucide 图标名
  description?: string;
  sort?: number;
  status?: string;
}
export const toolLinkCategoryService = makeCrudServiceFromClient<ToolLinkCategoryEntity>(toolLinkCategoryClient);

// ===================== Task =====================
// 任务执行记录（sys_task_executions；任务删除后记录保留，typeName 冗余可辨识）
export interface TaskExecutionEntity {
  id?: number;
  taskId?: number;
  typeName?: string;
  status?: 'ExecutionStatus_Success' | 'ExecutionStatus_Failed' | string;
  error?: string;
  startedAt?: string;
  finishedAt?: string;
  durationMs?: number;
  createTime?: string;
}
/** 任务选项（sys_tasks.task_options JSON 列；周期任务仅暴露 retryCount/timeout，其余为一次性任务语义） */
export interface TaskOptionEntity {
  retryCount?: number;
  /** protobuf Duration JSON 字符串（如 "300s"） */
  timeout?: string;
}
export interface TaskEntity extends CrudEntity {
  type?: string;
  typeName?: string;
  /** 任务显示名（人类可读；留空时前端回退显示 typeName） */
  name?: string;
  taskPayload?: string;
  cronSpec?: string;
  taskOptions?: TaskOptionEntity;
  enable?: boolean;
  remark?: string;
  /** 是否内置任务（后端按内置名单计算填充，不落库；内置任务禁止删除） */
  builtin?: boolean;
  /** 最近一次执行记录（列表页计算字段） */
  lastExecution?: TaskExecutionEntity;
}
export const taskService = {
  ...makeCrudServiceFromClient<TaskEntity>(taskClient),
  restart: () => taskClient.RestartAllTask({}) as Promise<unknown>,
  stop: () => taskClient.StopAllTask({}) as Promise<unknown>,
  control: (data: { id: string; action: string }) =>
    taskClient.ControlTask({ controlType: data.action as never, typeName: data.id }) as Promise<unknown>,
  listHandlers: () =>
    taskClient.ListTaskHandlers({}) as Promise<{
      items?: {
        typeName?: string;
        description?: string;
        /** 载荷参数填写规则说明（无参数时为"无需参数"类说明） */
        payloadDescription?: string;
        /** 载荷示例（JSON，可一键填入） */
        payloadExample?: string;
      }[];
    }>,
  /** 立即执行一次任务（一次性入队，不影响既有周期调度；禁用状态也可手动触发） */
  run: (id: string | number) => taskClient.RunTask({ id: Number(id) }) as Promise<unknown>,
  /** 查询任务最近执行记录（最近 50 条，按执行时间倒序） */
  listExecutions: (id: string | number) =>
    taskClient.ListTaskExecutions({ id: Number(id) }) as Promise<{
      items?: TaskExecutionEntity[];
      total?: number;
    }>,
};

// ===================== API Resource =====================
export interface ApiResourceEntity extends CrudEntity {
  description?: string;
  module?: string;
  moduleDescription?: string;
  path?: string;
  method?: string;
  operation?: string;
  source?: string;
  status?: string;
  delisted?: boolean;
  inOpenapi?: boolean;
  roleCount?: number;
}
export const apiResourceService = {
  ...makeCrudServiceFromClient<ApiResourceEntity>(apiResourceClient),
  sync: () => apiResourceClient.SyncApiResources({}) as Promise<SyncApiResourcesResponse>,
};

// ===================== Login Log =====================
export interface LoginLogEntity extends CrudEntity {
  username?: string;
  success?: boolean;
  loginTime?: string;
  location?: string;
  osName?: string;
  browserName?: string;
  loginIp?: string;
  reason?: string;
}
export const loginLogService = {
  list(opts: PagingRequest & { formValues?: Record<string, unknown> } = {}) {
    const params = buildQueryParams(opts.formValues, opts);
    return adminLoginLogClient.List({
      page: params.page ? Number(params.page) : undefined,
      pageSize: params.pageSize ? Number(params.pageSize) : undefined,
      noPaging: params.noPaging === 'true' ? true : undefined,
      query: params.query,
      orderBy: params.orderBy,
      fieldMask: params.fieldMask,
    } as never) as Promise<ListResponse<LoginLogEntity>>;
  },
  get(id: string | number) {
    return adminLoginLogClient.Get({ id: Number(id) }) as Promise<LoginLogEntity>;
  },
  /** 手动清理登录日志：物理删除 keepDays 天前的记录，返回删除行数 */
  clean(keepDays: number) {
    return adminLoginLogClient.Clean({ keepDays }) as Promise<{ deleted?: number }>;
  },
};

// ===================== Operation Log =====================
export interface OperationLogEntity extends CrudEntity {
  username?: string;
  success?: boolean;
  method?: string;
  path?: string;
  operation?: string;
  location?: string;
  osName?: string;
  browserName?: string;
  clientIp?: string;
  costTime?: string;
  reason?: string;
  requestId?: string;
  requestBody?: string;
  response?: string;
}
export const operationLogService = {
  list(opts: PagingRequest & { formValues?: Record<string, unknown> } = {}) {
    const params = buildQueryParams(opts.formValues, opts);
    return adminOperationLogClient.List({
      page: params.page ? Number(params.page) : undefined,
      pageSize: params.pageSize ? Number(params.pageSize) : undefined,
      noPaging: params.noPaging === 'true' ? true : undefined,
      query: params.query,
      orderBy: params.orderBy,
      fieldMask: params.fieldMask,
    } as never) as Promise<ListResponse<OperationLogEntity>>;
  },
  get(id: string | number) {
    return adminOperationLogClient.Get({ id: Number(id) }) as Promise<OperationLogEntity>;
  },
  /** 手动清理操作日志：物理删除 keepDays 天前的记录，返回删除行数 */
  clean(keepDays: number) {
    return adminOperationLogClient.Clean({ keepDays }) as Promise<{ deleted?: number }>;
  },
};

// ===================== Login Restriction =====================
export interface LoginRestrictionEntity extends CrudEntity {
  targetType?: string;
  targetValue?: string;
  action?: string;
  reason?: string;
  status?: string;
  // null = 显式清空（永不过期），走 updateMask null → SetNull 链路
  expireTime?: string | null;
  hitCount?: number;
  lastHitTime?: string;
}
export interface LoginRestrictionConflict {
  type?: string;
  withRuleId?: number;
  detail?: string;
}
export interface LoginRestrictionSimulateResult {
  operatorAllowed?: boolean;
  operatorVerdict?: string;
  matchedRuleId?: number;
  operatorIp?: string;
  operatorUsername?: string;
  affectedUserIds?: number[];
  affectedOnlineSessions?: number;
  conflicts?: LoginRestrictionConflict[];
}
export const loginRestrictionService = {
  ...makeCrudServiceFromClient<LoginRestrictionEntity>(adminLoginRestrictionClient),
  // 自锁护栏被拦截（409 LOGIN_RESTRICTION_SELF_LOCK）后由用户确认强制保存/删除
  createForce(data: Partial<LoginRestrictionEntity>) {
    return adminLoginRestrictionClient.Create({ data: data as never, force: true }) as Promise<unknown>;
  },
  updateForce(id: string | number, data: Partial<LoginRestrictionEntity>) {
    const updateMask = buildUpdateMask(data as Record<string, unknown>);
    return adminLoginRestrictionClient.Update({
      id: Number(id),
      data: data as never,
      updateMask,
      force: true,
    }) as Promise<unknown>;
  },
  deleteForce(id: string | number) {
    return adminLoginRestrictionClient.Delete({ id: Number(id), force: true }) as Promise<unknown>;
  },
  // 预演变更影响报告（不落库）：操作者自锁判定/受影响在线会话/规则冲突
  simulate(
    data: Partial<LoginRestrictionEntity>,
    changeType: 'CHANGE_TYPE_CREATE' | 'CHANGE_TYPE_UPDATE' | 'CHANGE_TYPE_DELETE',
  ) {
    return adminLoginRestrictionClient.Simulate({
      data: data as never,
      changeType,
    }) as Promise<LoginRestrictionSimulateResult>;
  },
};

// ===================== Notification =====================
export interface NotificationEntity extends CrudEntity {
  subject?: string;
  content?: string;
  categoryId?: number;
  categoryName?: string;
  status?: string;
}
export const notificationService = makeCrudServiceFromClient<NotificationEntity>(notificationMessageClient);

// 站内信接收记录（notification_message_recipients）
// 生成的 TS 客户端方法为空（protoc-gen-typescript-http 未生成），按 licenseService 约定手工封装 REST。
export interface NotificationRecipientEntity extends CrudEntity {
  messageId?: number;
  recipientId?: number;
  status?: string; // RECEIVED-已接收 READ-已读
}

export const notificationRecipientService = {
  list(opts: PagingRequest & { formValues?: Record<string, unknown> } = {}) {
    return request.get<ListResponse<NotificationRecipientEntity>>('/api/v1/monitor/notification-recipients', {
      params: buildQueryParams(opts.formValues, opts),
    });
  },
  update(id: string | number, data: Partial<NotificationRecipientEntity>) {
    const updateMask = buildUpdateMask(data as Record<string, unknown>);
    return request.put(`/api/v1/monitor/notification-recipients/${Number(id)}`, { data, updateMask });
  },
};

// ===================== Private Message =====================
export interface PrivateMessageEntity extends CrudEntity {
  subject?: string;
  content?: string;
  senderId?: number;
  senderName?: string;
  senderAvatar?: string;
  receiverId?: number;
  receiverName?: string;
  receiverAvatar?: string;
}
export const privateMessageService = makeCrudServiceFromClient<PrivateMessageEntity>(privateMessageClient);

// ===================== File =====================
export interface FileEntity extends CrudEntity {
  provider?: string;
  fileName?: string;
  saveFileName?: string;
  extension?: string;
  size?: number;
  bucketName?: string;
  fileDirectory?: string;
  linkUrl?: string;
}
export const fileService = {
  ...makeCrudServiceFromClient<FileEntity>(fileClient),
  // 鉴权拉取文件 Blob（行内图片预览用，如头像；Authorization 头由 request 拦截器注入）
  fetchBlob(relPath: string): Promise<Blob> {
    return request.get<Blob>('/api/v1/monitor/file:download', {
      params: { path: relPath },
      responseType: 'blob',
    });
  },
  // 鉴权下载：GET /api/v1/monitor/file:download?path=<bucket>/<dir>/<saveFileName>（JWT 走 middleware 链）
  async download(entity: FileEntity) {
    const dir = (entity.fileDirectory || '').replace(/^\/+|\/+$/g, '');
    const rel = [entity.bucketName, dir, entity.saveFileName || entity.fileName].filter(Boolean).join('/');
    const blob = await request.get<Blob>('/api/v1/monitor/file:download', {
      params: { path: rel },
      responseType: 'blob',
    });
    downloadBlob(blob as unknown as Blob, entity.fileName || 'download');
  },
};

// ===================== Router (dynamic routes + perm codes) =====================
export interface RouteItem {
  id?: string;
  name?: string;
  path?: string;
  redirect?: string;
  component?: string;
  meta?: MenuMeta;
  children?: RouteItem[];
}
export interface ListRouteResponse {
  items: RouteItem[];
}
export interface ListPermCodeResponse {
  codes: string[];
}
export const routerService = {
  getRoutes: () => routerClient.ListRoute({}) as Promise<ListRouteResponse>,
  getPermCodes: () => routerClient.ListPermissionCode({}) as Promise<ListPermCodeResponse>,
};


// ===================== SystemConfig (系统配置) =====================
export interface SystemConfigEntity extends CrudEntity {
  configKey: string;
  configValue: string;
  configType?: string;
  description?: string;
}
export const systemConfigService = makeCrudServiceFromClient<SystemConfigEntity>(systemConfigClient);

// ===================== Session (在线会话) =====================
export interface OnlineUserEntity {
  userId?: number;
  username?: string;
  nickname?: string;
  deptId?: number;
  token?: string;
  clientId?: string;
  loginIp?: string;
  loginLocation?: string;
  userAgent?: string;
  loginTime?: string;
  expiresAt?: string;
}
export const sessionService = {
  listOnline() {
    return sessionClient.ListOnline({}) as unknown as Promise<{ items: OnlineUserEntity[]; total: number }>;
  },
  forceOffline(userId: number) {
    return sessionClient.ForceOffline({ userId }) as Promise<unknown>;
  },
};

// ===================== DataPermission (数据权限规则) =====================
export interface DataPermissionEntity extends CrudEntity {
  roleId: number;
  name: string;
  field?: string;
  scope?: string;
  ruleJson?: string;
  status?: string;
  tenantId?: string;
}
export const dataPermissionService = makeCrudServiceFromClient<DataPermissionEntity>(dataPermissionClient);

// ===================== RoleDept (角色部门绑定) =====================
export interface RoleDeptEntity {
  id?: number;
  roleId?: number;
  deptId?: number;
  tenantId?: number;
  createTime?: string;
}
export const roleDeptService = {
  listByRole(roleId: number) {
    return roleDeptClient.ListByRole({ roleId }) as unknown as Promise<{ items: RoleDeptEntity[]; total: number }>;
  },
  batchBind(roleId: number, deptIds: number[]) {
    return roleDeptClient.BatchBind({ roleId, deptIds }) as Promise<unknown>;
  },
  unbindByRole(roleId: number) {
    return roleDeptClient.UnbindByRole({ roleId }) as Promise<unknown>;
  },
};

// ===================== My Notice (FrameWork stub) =====================
export interface MyNoticeItem {
  id?: number | string;
  recipientId?: number;
  title?: string;
  content?: string;
  status?: string;
  createdAt?: string;
}

export const myNoticeService = {
  async countMyUnread(): Promise<{ count: number }> {
    return { count: 0 };
  },
  async listMy(_query?: { page?: number; pageSize?: number }): Promise<{ items: MyNoticeItem[]; unreadCount: number }> {
    return { items: [], unreadCount: 0 };
  },
  async markMyRead(_ids?: number[]): Promise<void> {
    return;
  },
};


// ===================== GitHub Radar =====================
export type { RadarRepoItem, RadarWatchItem, RadarWatchEvent, RadarNotification, ListTrendingResponse, SearchRisingResponse, GetRadarConfigResponse };
export type { GetRepoResponse, GetRepoTreeResponse, GetRepoContentResponse, CloneRepoResponse, GenerateInsightResponse, RadarAIInsight };

export const radarTrendingService = {
  listTrending(query: {
    since?: string;
    language?: string;
    topic?: string;
    onlyNew?: boolean;
    page?: number;
    pageSize?: number;
    sort?: string;
    query?: string;
    license?: string;
  } = {}) {
    return radarTrendingClient.ListTrending({
      since: query.since,
      language: query.language,
      topic: query.topic,
      onlyNew: query.onlyNew,
      page: query.page,
      pageSize: query.pageSize,
      sort: query.sort,
      query: query.query,
      license: query.license,
    }) as Promise<ListTrendingResponse>;
  },
  searchRising(query: { query?: string; language?: string; page?: number; pageSize?: number } = {}) {
    return radarTrendingClient.SearchRising({
      query: query.query,
      language: query.language,
      page: query.page,
      pageSize: query.pageSize,
    }) as Promise<SearchRisingResponse>;
  },
};

export const radarRepoService = {
  getRepo(fullName: string, opts: { includeReadme?: boolean; insightLevel?: string; cacheOnly?: boolean } = {}) {
    return radarRepoClient.GetRepo({
      fullName,
      includeReadme: opts.includeReadme ?? false,
      insightLevel: opts.insightLevel || undefined,
      insightCacheOnly: opts.cacheOnly ?? false,
    }) as Promise<GetRepoResponse>;
  },
  getTree(fullName: string, branch?: string) {
    return radarRepoClient.GetRepoTree({ fullName, branch: branch || undefined }) as Promise<GetRepoTreeResponse>;
  },
  getContent(fullName: string, path: string, ref?: string) {
    return radarRepoClient.GetRepoContent({ fullName, path, ref: ref || undefined }) as Promise<GetRepoContentResponse>;
  },
  clone(fullName: string, toServer = false) {
    return radarRepoClient.CloneRepo({ fullName, toServer }) as Promise<CloneRepoResponse>;
  },
  generateInsight(fullName: string, level: string) {
    return radarRepoClient.GenerateInsight({ fullName, level }) as Promise<GenerateInsightResponse>;
  },
};

export const radarWatchService = {
  list() {
    return radarWatchClient.ListWatches({}) as Promise<{ items?: RadarWatchItem[] }>;
  },
  add(fullName: string) {
    return radarWatchClient.AddWatch({ fullName }) as Promise<{ item?: RadarWatchItem }>;
  },
  remove(id: string) {
    return radarWatchClient.RemoveWatch({ id }) as Promise<unknown>;
  },
  listEvents(unreadOnly?: boolean) {
    return radarWatchClient.ListWatchEvents({ unreadOnly }) as Promise<{ items?: RadarWatchEvent[] }>;
  },
  markEventsRead(ids?: string[]) {
    return radarWatchClient.MarkWatchEventsRead({ ids }) as Promise<unknown>;
  },
};

export const radarNotifyService = {
  list(unreadOnly?: boolean) {
    return radarNotifyClient.ListNotifications({ unreadOnly }) as Promise<{ items?: RadarNotification[] }>;
  },
  markRead(ids?: string[]) {
    return radarNotifyClient.MarkNotificationsRead({ ids }) as Promise<unknown>;
  },
};

export const radarConfigService = {
  get() {
    return radarConfigClient.GetConfig({}) as Promise<GetRadarConfigResponse>;
  },
  set(cfg: {
    githubToken?: string;
    llmEnabled?: boolean;
    llmBaseUrl?: string;
    llmApiKey?: string;
    llmModel?: string;
    cloneEnabled?: boolean;
    cloneDir?: string;
  }) {
    return radarConfigClient.SetConfig({
      githubToken: cfg.githubToken,
      llmEnabled: cfg.llmEnabled,
      llmBaseUrl: cfg.llmBaseUrl,
      llmApiKey: cfg.llmApiKey,
      llmModel: cfg.llmModel,
      cloneEnabled: cfg.cloneEnabled,
      cloneDir: cfg.cloneDir,
    }) as Promise<GetRadarConfigResponse>;
  },
};

// ===================== Airport stubs (removed domain) =====================
export interface AirportEntity extends CrudEntity {
  name?: string;
  code?: string;
}
export interface AirportPostEntity extends CrudEntity {
  name?: string;
  airportId?: string | number;
  status?: string;
}
export const airportService = {
  async list(_q?: Record<string, unknown>): Promise<ListResponse<AirportEntity>> {
    return { items: [], total: 0 };
  },
  async get(_id: string | number): Promise<AirportEntity> {
    return { id: 0 };
  },
};
export const airportPostService = {
  async list(_q?: Record<string, unknown>): Promise<ListResponse<AirportPostEntity>> {
    return { items: [], total: 0 };
  },
  async get(_id: string | number): Promise<AirportPostEntity> {
    return { id: 0 };
  },
  async create(data: Partial<AirportPostEntity>): Promise<AirportPostEntity> {
    return { id: Date.now(), ...data };
  },
  async update(id: string | number, data: Partial<AirportPostEntity>): Promise<AirportPostEntity> {
    return { id, ...data };
  },
  async delete(_id: string | number): Promise<void> {},
};
