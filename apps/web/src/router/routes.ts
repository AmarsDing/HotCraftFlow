import type { RouteRecordRaw } from 'vue-router';
import BlankLayout from '../layouts/BlankLayout.vue';
import MainLayout from '../layouts/MainLayout.vue';
import LoginPage from '../pages/LoginPage.vue';
import NotFoundPage from '../pages/NotFoundPage.vue';

/**
 * FrameWork skeleton routes — platform pages only.
 * Business TwinMonitor routes were removed so the app can boot for inspection.
 */
export const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: BlankLayout,
    meta: { public: true, layout: 'blank' },
    children: [{ path: '', name: 'login', component: LoginPage, meta: { public: true, layout: 'blank' } }],
  },
  {
    path: '/visual-twin',
    component: BlankLayout,
    children: [
      {
        path: '',
        name: 'visual-twin',
        component: () => import('../pages/visual-twin/VisualTwinPage.vue'),
      },
    ],
  },
  {
    path: '/',
    component: MainLayout,
    meta: { layout: 'main' },
    children: [
      { path: '', redirect: '/dashboard' },
      {
        path: 'dashboard',
        name: 'dashboard',
        component: () => import('../pages/DashboardPage.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'opm/organizations',
        name: 'opm-organizations',
        component: () => import('../pages/opm/OrganizationPage.vue'),
        meta: { title: '组织' },
      },
      {
        path: 'opm/departments',
        name: 'opm-departments',
        component: () => import('../pages/opm/DepartmentPage.vue'),
        meta: { title: '部门' },
      },
      {
        path: 'opm/positions',
        name: 'opm-positions',
        component: () => import('../pages/opm/PositionPage.vue'),
        meta: { title: '岗位' },
      },
      {
        path: 'opm/users',
        name: 'opm-users',
        component: () => import('../pages/opm/UserPage.vue'),
        meta: { title: '用户' },
      },
      {
        path: 'profile',
        name: 'profile',
        component: () => import('../pages/opm/ProfilePage.vue'),
        meta: { title: '个人中心' },
      },
      {
        path: 'permission/roles',
        name: 'permission-roles',
        component: () => import('../pages/permission/RolePage.vue'),
        meta: { title: '角色' },
      },
      {
        path: 'permission/menus',
        name: 'permission-menus',
        component: () => import('../pages/permission/MenuPage.vue'),
        meta: { title: '菜单' },
      },
      {
        path: 'system/dicts',
        name: 'system-dicts',
        component: () => import('../pages/system/DictPage.vue'),
        meta: { title: '字典' },
      },
      {
        path: 'system/files',
        name: 'system-files',
        component: () => import('../pages/system/FilePage.vue'),
        meta: { title: '文件' },
      },
      {
        path: 'system/tasks',
        name: 'system-tasks',
        component: () => import('../pages/system/TaskPage.vue'),
        meta: { title: '任务' },
      },
      {
        path: 'system/api-resources',
        name: 'system-api-resources',
        component: () => import('../pages/system/ApiResourcePage.vue'),
        meta: { title: 'API 资源' },
      },
      {
        path: 'system/configs',
        name: 'system-configs',
        component: () => import('../pages/system/SystemConfigPage.vue'),
        meta: { title: '系统配置' },
      },
      {
        path: 'system/login-restrictions',
        name: 'system-login-restrictions',
        component: () => import('../pages/system/LoginRestrictionPage.vue'),
        meta: { title: '登录限制' },
      },
      {
        path: 'system/online-users',
        name: 'system-online-users',
        component: () => import('../pages/system/OnlineUserPage.vue'),
        meta: { title: '在线用户' },
      },
      {
        path: 'tenant',
        name: 'tenant',
        component: () => import('../pages/tenant/TenantPage.vue'),
        meta: { title: '租户' },
      },
      {
        path: 'log/login',
        name: 'log-login',
        component: () => import('../pages/log/LoginLogPage.vue'),
        meta: { title: '登录日志' },
      },
      {
        path: 'log/operation',
        name: 'log-operation',
        component: () => import('../pages/log/OperationLogPage.vue'),
        meta: { title: '操作日志' },
      },
      {
        path: 'notice/center',
        name: 'notice-center',
        component: () => import('../pages/notice/NoticeCenterPage.vue'),
        meta: { title: '通知中心' },
      },
      {
        path: 'visualization/topology-3d',
        name: 'topology-3d',
        component: () => import('../pages/visualization/Topology3DPage.vue'),
        meta: { title: '3D 拓扑' },
      },
      { path: ':pathMatch(.*)*', name: 'not-found', component: NotFoundPage },
    ],
  },
];
