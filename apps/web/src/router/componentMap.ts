import NotFoundPage from '../pages/NotFoundPage.vue';

// 骨架仅保留平台页映射（键与 seed Menu.component 对齐）。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const componentMap: Record<string, () => Promise<any>> = {
  DashboardPage: () => import('../pages/DashboardPage.vue'),

  'opm/OrganizationPage': () => import('../pages/opm/OrganizationPage.vue'),
  'opm/DepartmentPage': () => import('../pages/opm/DepartmentPage.vue'),
  'opm/PositionPage': () => import('../pages/opm/PositionPage.vue'),
  'opm/UserPage': () => import('../pages/opm/UserPage.vue'),
  'opm/ProfilePage': () => import('../pages/opm/ProfilePage.vue'),
  'opm/PostPage': () => import('../pages/opm/PostPage.vue'),

  'permission/RolePage': () => import('../pages/permission/RolePage.vue'),
  'permission/MenuPage': () => import('../pages/permission/MenuPage.vue'),

  'system/DictPage': () => import('../pages/system/DictPage.vue'),
  'system/FilePage': () => import('../pages/system/FilePage.vue'),
  'system/TaskPage': () => import('../pages/system/TaskPage.vue'),
  'system/ApiResourcePage': () => import('../pages/system/ApiResourcePage.vue'),
  'system/LoginRestrictionPage': () => import('../pages/system/LoginRestrictionPage.vue'),
  'system/SystemConfigPage': () => import('../pages/system/SystemConfigPage.vue'),
  'system/OnlineUserPage': () => import('../pages/system/OnlineUserPage.vue'),

  'tenant/TenantPage': () => import('../pages/tenant/TenantPage.vue'),

  'log/LoginLogPage': () => import('../pages/log/LoginLogPage.vue'),
  'log/OperationLogPage': () => import('../pages/log/OperationLogPage.vue'),

  'github-radar/RadarPage': () => import('../pages/github-radar/RadarPage.vue'),

  // ⑤ MAP 媒体发布矩阵（键与 seed Menu.component 对齐；分发详情页不进菜单，走静态路由）
  'media/map/MapApprovalsPage': () => import('../pages/media/map/MapApprovalsPage.vue'),
  'media/map/MapAccountsPage': () => import('../pages/media/map/MapAccountsPage.vue'),
  'media/map/MapCalendarPage': () => import('../pages/media/map/MapCalendarPage.vue'),
  'media/map/MapReceiptsPage': () => import('../pages/media/map/MapReceiptsPage.vue'),

  // ④ MME 矩阵混剪（键与 seed Menu.component 对齐；详情/矩阵/审核台不进菜单，走静态路由）
  'media/mme/TaskListPage': () => import('../pages/media/mme/TaskListPage.vue'),
  'media/mme/RenderQueuePage': () => import('../pages/media/mme/RenderQueuePage.vue'),

  // ③ MIA 市场影响分析（键与 seed Menu.component 对齐；影响卡详情不进菜单，走静态路由）
  'media/mia/ImpactListPage': () => import('../pages/media/mia/ImpactListPage.vue'),
  'media/mia/EventListPage': () => import('../pages/media/mia/EventListPage.vue'),
  'media/mia/MappingPage': () => import('../pages/media/mia/MappingPage.vue'),
  'media/mia/CalibrationPage': () => import('../pages/media/mia/CalibrationPage.vue'),
};

export function resolveMenuComponent(name?: string | null) {
  if (!name) return NotFoundPage;
  const loader = componentMap[name];
  if (!loader) return NotFoundPage;
  return loader;
}

export function getComponentMap() {
  return componentMap;
}

export default componentMap;

/** Alias for auth dynamic menu wiring */
export const resolveComponent = resolveMenuComponent;

export function getComponentMapOptions(): { label: string; value: string }[] {
  return Object.keys(componentMap).map((k) => ({ label: k, value: k }));
}
