import { defineStore } from 'pinia';
import {
  userService,
  roleService,
  menuService,
  organizationService,
  departmentService,
  positionService,
  dictService,
  tenantService,
  toolLinkService,
  toolLinkCategoryService,
  taskService,
  apiResourceService,
  loginLogService,
  operationLogService,
  loginRestrictionService,
  notificationService,
  privateMessageService,
  fileService,
  systemConfigService,
  dataPermissionService,  type UserEntity,
  type RoleEntity,
  type MenuEntity,
  type OrganizationEntity,
  type DepartmentEntity,
  type PositionEntity,
  type DictEntity,
  type TenantEntity,
  type ToolLinkEntity,
  type ToolLinkCategoryEntity,
  type TaskEntity,
  type ApiResourceEntity,
  type LoginLogEntity,
  type OperationLogEntity,
  type LoginRestrictionEntity,
  type NotificationEntity,
  type PrivateMessageEntity,
  type FileEntity,
  type SystemConfigEntity,
  type DataPermissionEntity,} from '../services';

interface ListParams {
  noPaging?: boolean;
  page?: number;
  pageSize?: number;
  formValues?: Record<string, unknown>;
  orderBy?: string;
  fieldMask?: string;
}

/** 完整 CRUD service 形状（list/get/create/update/delete） */
export interface CrudService<T> {
  list(params: ListParams): Promise<{ items: T[]; total: number }>;
  get(id: string | number): Promise<T>;
  create(values: Partial<T>): Promise<unknown>;
  update(id: string | number, values: Partial<T>): Promise<unknown>;
  delete(id: string | number): Promise<unknown>;
}

/** 只读 service 形状（仅 list/get，用于日志类只读实体） */
export interface ReadOnlyService<T> {
  list(params: ListParams): Promise<{ items: T[]; total: number }>;
  get(id: string | number): Promise<T>;
}

function makeCrudStore<T extends { id?: string | number }>(name: string, service: CrudService<T>) {
  return defineStore(name, {
    state: () => ({
      list: [] as T[],
      total: 0,
      loading: false,
      detail: null as T | null,
    }),
    actions: {
      async fetchList(params: ListParams = {}) {
        this.loading = true;
        try {
          const resp = await service.list(params);
          // Pinia options API 对泛型 T 的 state 解包存在已知类型限制（UnwrapRefSimple<T>），
          // 此处断言绕过，读取时仍按声明的 T[] 类型使用
          this.list = resp.items as never;
          this.total = resp.total;
          return resp;
        } finally {
          this.loading = false;
        }
      },
      async fetchDetail(id: string | number) {
        const resp = await service.get(id);
        this.detail = resp as never;
        return resp;
      },
      async create(values: Partial<T>) {
        await service.create(values);
      },
      async update(id: string | number, values: Partial<T>) {
        await service.update(id, values);
      },
      async remove(id: string | number) {
        await service.delete(id);
      },
      $reset() {
        this.list = [];
        this.total = 0;
        this.detail = null;
      },
    },
  });
}

/** 只读 store 工厂：仅暴露 fetchList/fetchDetail，避免误调用不存在的 create/update/delete */
function makeReadOnlyStore<T extends { id?: string | number }>(name: string, service: ReadOnlyService<T>) {
  return defineStore(name, {
    state: () => ({
      list: [] as T[],
      total: 0,
      loading: false,
      detail: null as T | null,
    }),
    actions: {
      async fetchList(params: ListParams = {}) {
        this.loading = true;
        try {
          const resp = await service.list(params);
          // Pinia options API 对泛型 T 的 state 解包存在已知类型限制（UnwrapRefSimple<T>），
          // 此处断言绕过，读取时仍按声明的 T[] 类型使用
          this.list = resp.items as never;
          this.total = resp.total;
          return resp;
        } finally {
          this.loading = false;
        }
      },
      async fetchDetail(id: string | number) {
        const resp = await service.get(id);
        this.detail = resp as never;
        return resp;
      },
      $reset() {
        this.list = [];
        this.total = 0;
        this.detail = null;
      },
    },
  });
}

export const useUserStore = makeCrudStore<UserEntity>('user', userService);
export const useRoleStore = makeCrudStore<RoleEntity>('role', roleService);
export const useMenuStore = makeCrudStore<MenuEntity>('menu', menuService);
export const useOrganizationStore = makeCrudStore<OrganizationEntity>('organization', organizationService);
export const useDepartmentStore = makeCrudStore<DepartmentEntity>('department', departmentService);
export const usePositionStore = makeCrudStore<PositionEntity>('position', positionService);
export const useDictStore = makeCrudStore<DictEntity>('dict', dictService);
export const useTenantStore = makeCrudStore<TenantEntity>('tenant', tenantService);
export const useToolLinkStore = makeCrudStore<ToolLinkEntity>('toolLink', toolLinkService);
export const useToolLinkCategoryStore = makeCrudStore<ToolLinkCategoryEntity>(
  'toolLinkCategory',
  toolLinkCategoryService,
);
export const useTaskStore = makeCrudStore<TaskEntity>('task', taskService);
export const useApiResourceStore = makeCrudStore<ApiResourceEntity>('apiResource', apiResourceService);
export const useLoginLogStore = makeReadOnlyStore<LoginLogEntity>('loginLog', loginLogService);
export const useOperationLogStore = makeReadOnlyStore<OperationLogEntity>('operationLog', operationLogService);
export const useLoginRestrictionStore = makeCrudStore<LoginRestrictionEntity>(
  'loginRestriction',
  loginRestrictionService,
);
export const useNotificationStore = makeCrudStore<NotificationEntity>('notification', notificationService);
export const usePrivateMessageStore = makeCrudStore<PrivateMessageEntity>('privateMessage', privateMessageService);
export const useFileStore = makeCrudStore<FileEntity>('file', fileService);
export const useSystemConfigStore = makeCrudStore<SystemConfigEntity>('systemConfig', systemConfigService);
export const useDataPermissionStore = makeCrudStore<DataPermissionEntity>('dataPermission', dataPermissionService);

// Generic tree builder
export type TreeNode<T> = T & { children: TreeNode<T>[] };

export function buildTree<T extends { id?: string | number; parentId?: string | number }>(items: T[]): TreeNode<T>[] {
  // 入参可能是后端已嵌套的树（department/organization 等 repo 直接返回父子结构，children 内联在父节点里）。
  // 先深度拍平收集全部后代、再按 parentId 统一重建，避免内层 children 被静默丢弃
  // （曾致部门页只显示 11 个根部门、迁移来的 23 个子部门任何一页都不可见）；map 按 id 去重。
  const flat: T[] = [];
  const walk = (list: T[]) => {
    for (const item of list) {
      flat.push(item);
      const children = (item as { children?: T[] }).children;
      if (Array.isArray(children) && children.length > 0) {
        walk(children);
      }
    }
  };
  walk(items);

  const map = new Map<string, TreeNode<T>>();
  const roots: TreeNode<T>[] = [];
  flat.forEach((item) => {
    if (item.id != null) map.set(String(item.id), { ...item, children: [] });
  });
  map.forEach((item) => {
    const parentId = item.parentId;
    if (parentId != null && parentId !== '' && map.has(String(parentId))) {
      map.get(String(parentId))?.children.push(item);
    } else {
      roots.push(item);
    }
  });
  return roots;
}

// Menu tree builder utility（buildTree 的 MenuEntity 特化别名，保持导出名兼容）
export function buildMenuTree(menus: MenuEntity[]): MenuEntity[] {
  return buildTree(menus) as unknown as MenuEntity[];
}

