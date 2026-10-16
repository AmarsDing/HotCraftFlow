/** Platform side-nav helpers for FrameWork skeleton */
export interface SideNavItem {
  label?: string;
  labelKey?: string;
  icon?: string;
  to?: string;
  exact?: boolean;
  children?: SideNavItem[];
}

export interface SideNavGroup {
  labelKey: string;
  icon?: string;
  items: SideNavItem[];
}

export const sideNavItems: SideNavItem[] = [
  { label: "仪表盘", icon: "dashboard", to: "/dashboard" },
  {
    label: "组织权限",
    icon: "corporate_fare",
    children: [
      { label: "组织", to: "/opm/organizations" },
      { label: "部门", to: "/opm/departments" },
      { label: "岗位", to: "/opm/positions" },
      { label: "用户", to: "/opm/users" },
    ],
  },
  {
    label: "权限",
    icon: "security",
    children: [
      { label: "角色", to: "/permission/roles" },
      { label: "菜单", to: "/permission/menus" },
    ],
  },
  {
    label: "系统",
    icon: "settings",
    children: [
      { label: "字典", to: "/system/dicts" },
      { label: "文件", to: "/system/files" },
      { label: "任务", to: "/system/tasks" },
      { label: "API 资源", to: "/system/api-resources" },
      { label: "系统配置", to: "/system/configs" },
      { label: "登录限制", to: "/system/login-restrictions" },
      { label: "在线用户", to: "/system/online-users" },
    ],
  },
  { label: "租户", icon: "apartment", to: "/tenant" },
  {
    label: "日志",
    icon: "history",
    children: [
      { label: "登录日志", to: "/log/login" },
      { label: "操作日志", to: "/log/operation" },
    ],
  },
];

export const sideNavGroups: SideNavGroup[] = [
  {
    labelKey: "nav.dashboard",
    icon: "dashboard",
    items: [{ to: "/dashboard", icon: "dashboard", labelKey: "nav.dashboard" }],
  },
  {
    labelKey: "nav.opm",
    icon: "corporate_fare",
    items: [
      { to: "/opm/organizations", icon: "account_tree", labelKey: "nav.organizations" },
      { to: "/opm/departments", icon: "groups", labelKey: "nav.departments" },
      { to: "/opm/positions", icon: "badge", labelKey: "nav.positions" },
      { to: "/opm/users", icon: "person", labelKey: "nav.users" },
    ],
  },
  {
    labelKey: "nav.permission",
    icon: "security",
    items: [
      { to: "/permission/roles", icon: "admin_panel_settings", labelKey: "nav.roles" },
      { to: "/permission/menus", icon: "menu", labelKey: "nav.menus" },
    ],
  },
  {
    labelKey: "nav.system",
    icon: "settings",
    items: [
      { to: "/system/dicts", icon: "book", labelKey: "nav.dicts" },
      { to: "/system/files", icon: "folder", labelKey: "nav.files" },
      { to: "/system/tasks", icon: "schedule", labelKey: "nav.tasks" },
      { to: "/system/api-resources", icon: "api", labelKey: "nav.apiResources" },
      { to: "/system/configs", icon: "tune", labelKey: "nav.configs" },
      { to: "/system/login-restrictions", icon: "lock", labelKey: "nav.loginRestrictions" },
      { to: "/system/online-users", icon: "sensors", labelKey: "nav.onlineUsers" },
    ],
  },
  {
    labelKey: "nav.tenant",
    icon: "apartment",
    items: [{ to: "/tenant", icon: "apartment", labelKey: "nav.tenant" }],
  },
  {
    labelKey: "nav.log",
    icon: "history",
    items: [
      { to: "/log/login", icon: "login", labelKey: "nav.loginLog" },
      { to: "/log/operation", icon: "history_edu", labelKey: "nav.operationLog" },
    ],
  },
];

type RouteLike = { path?: string; meta?: { title?: string; icon?: string }; children?: RouteLike[] };

export function buildSideNavFromRoutes(routes: RouteLike[]): SideNavGroup[] {
  const items: SideNavItem[] = [];
  const walk = (list: RouteLike[]) => {
    for (const r of list || []) {
      if (r.path && !r.children?.length) {
        items.push({
          to: r.path.startsWith("/") ? r.path : `/${r.path}`,
          icon: r.meta?.icon || "chevron_right",
          labelKey: r.meta?.title || r.path,
        });
      }
      if (r.children?.length) walk(r.children);
    }
  };
  walk(routes || []);
  if (items.length === 0) return sideNavGroups;
  return [{ labelKey: "nav.menus", icon: "menu", items }];
}

export function injectWindShearItem(groups: SideNavGroup[]): SideNavGroup[] {
  return groups;
}

export default sideNavItems;
