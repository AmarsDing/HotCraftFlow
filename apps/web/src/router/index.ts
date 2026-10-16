import { createRouter, createWebHistory } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { collectRoutePaths } from '../config/lineWorkbenchTabs';
import { abortRoutePendingRequests } from '../services/request';
import { routes } from './routes';
import type { RouteItem } from '../services';

const router = createRouter({
  history: createWebHistory(),
  routes,
});

// 页面跳转后取消上一页在途请求（仅 path 变化；query/hash 变化视为同页内导航），
// 避免上一页的慢请求继续占用后端连接拖慢新页面首屏
router.afterEach((to, from) => {
  if (to.path !== from.path) abortRoutePendingRequests();
});

// 懒加载 chunk 失败兜底：发版后旧页面持有过期 chunk hash，动态 import 404 会白屏。
// 一次性强制刷新拉取新资源；sessionStorage 时间戳防刷新死循环（新 chunk 仍失败则不再 reload）。
const CHUNK_RELOAD_KEY = 'framework_chunk_reload_ts';
const CHUNK_RELOAD_COOLDOWN_MS = 10_000;
router.onError((error) => {
  const msg = error?.message || '';
  const isChunkLoadError =
    error.name === 'ChunkLoadError' ||
    msg.includes('Failed to fetch dynamically imported module') ||
    msg.includes('error loading dynamically imported module');
  if (!isChunkLoadError) return;
  const last = Number(sessionStorage.getItem(CHUNK_RELOAD_KEY) || 0);
  if (Date.now() - last < CHUNK_RELOAD_COOLDOWN_MS) return;
  sessionStorage.setItem(CHUNK_RELOAD_KEY, String(Date.now()));
  location.reload();
});

// 角色→首页显式映射（《00-角色用户体系与登录页面规划》§6，2026-09-16 v1.1）：
// 多角色并存时取优先级最高角色的首页；该首页未获授权（如 cnseng 的 /commnav 未上线）
// 自动降级到下一优先级；全部落空回退"业务平台→监控→运维"旧推导。
const ROLE_HOME_PRIORITY: Array<{ roles: string[]; home: string }> = [
  { roles: ['gc', 'yb', 'jw'], home: '/integrated-ops' },
  { roles: ['cnseng'], home: '/commnav' },
  { roles: ['keeper'], home: '/inventory' },
  { roles: ['assetadm'], home: '/asset/center' },
  { roles: ['xx'], home: '/ops-center' },
  { roles: ['deptmgr', 'viewer'], home: '/stats-center' },
  { roles: ['auditor'], home: '/log/login' },
  { roles: ['dutylead', 'operator', 'gly', 'super'], home: '/dashboard' },
];

function roleHome(items: RouteItem[], roles: string[] = []): string {
  const permitted = new Set<string>();
  collectRoutePaths(items, permitted);
  const owned = new Set(roles);
  for (const rule of ROLE_HOME_PRIORITY) {
    if (!rule.roles.some((r) => owned.has(r))) continue;
    if (permitted.has(rule.home)) return rule.home;
  }
  if (permitted.has('/integrated-ops')) return '/integrated-ops';
  if (permitted.has('/dashboard')) return '/dashboard';
  if (permitted.has('/aiops')) return '/aiops';
  return '/dashboard';
}

router.beforeEach(async (to) => {
  // 公开路由（如 /login）直接放行
  const isPublic = to.matched.some((r) => r.meta.public === true);
  if (isPublic) return true;

  // 非公开路由需要登录态
  const auth = useAuthStore();
  await auth.ensureSession();
  if (!auth.user) {
    return { path: '/login', query: { redirect: to.fullPath } };
  }

  // 动态菜单未就绪（上次拉取失败/被中止）且目标命中 404 兜底 → 重试拉取；
  // 成功则 justLoadedDynamicRoutes 置位，落入下方既有分支重解析（保 query/hash），
  // 深链不再因一次性菜单拉取失败而永久掉 404
  if (!auth.dynamicRoutesLoaded && to.matched.some((r) => r.name === 'not-found')) {
    await auth.loadDynamicRoutes();
  }

  // 动态路由刚加载完 → 重定向一次让新路由生效。
  // 因为 to 在守卫开始前就已解析（可能命中 404 兜底），addRoute 后需重新解析。
  // 注意：对象形式的 path 不会解析其中的 query（fullPath 里的 ? 会被静默丢弃），
  // 必须显式传递 query/hash 才能保留 ?tab=xxx 这类参数。
  if (auth.justLoadedDynamicRoutes) {
    auth.justLoadedDynamicRoutes = false;
    // 根落地（'/' → redirect /dashboard）时按权限推导首页，与重解析合并为一次重定向
    const target = to.redirectedFrom?.path === '/' ? roleHome(auth.dynamicRoutes, auth.profile?.roles) : to.path;
    return { path: target, query: to.query, hash: to.hash, replace: true };
  }

  // 同会话后续根落地（如点品牌区回首页）：动态路由早已就绪，直接按权限推导
  if (to.redirectedFrom?.path === '/') {
    const home = roleHome(auth.dynamicRoutes, auth.profile?.roles);
    if (home !== to.path) return { path: home, replace: true };
  }

  return true;
});

export default router;
