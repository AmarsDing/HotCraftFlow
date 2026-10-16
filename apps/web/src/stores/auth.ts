import { defineStore } from 'pinia';
import type { RouteRecordRaw } from 'vue-router';
import { authService, routerService, type UserProfile, type LoginRequest, type RouteItem } from '../services';
import { setLoggingOut } from '../services/request';
import { resolveComponent } from '../router/componentMap';

// 跟踪已注册的动态路由 name，logout 时按 name 移除，避免切换账号后路由残留
const addedRouteNames: string[] = [];

// 登出撤销令牌的等待上限：正常 <1s 即返回；网关/后端拥塞时不再阻塞跳转登录页
const LOGOUT_REVOKE_TIMEOUT_MS = 1500;

// registerDynamicRoute 将后端 RouteItem 树扁平化注册为 main-layout 的子路由。
// folder（有 children）只注册 redirect（无 component），leaf 注册 component。
// /dashboard 跳过 —— routes.ts 已有静态兜底。
function registerDynamicRoute(router: { addRoute: (name: string, route: RouteRecordRaw) => void }, item: RouteItem) {
  if (item.path === '/dashboard') return;
  if (item.children && item.children.length > 0) {
    if (item.redirect) {
      router.addRoute('main-layout', {
        path: item.path ?? '',
        redirect: item.redirect,
        meta: item.meta as never,
      } as RouteRecordRaw);
    }
    for (const child of item.children) {
      registerDynamicRoute(router, child);
    }
  } else {
    if (item.name) addedRouteNames.push(item.name);
    router.addRoute('main-layout', {
      path: item.path ?? '',
      component: resolveComponent(item.component),
      name: item.name,
      // 叶子已有组件时忽略 redirect：容器目录（气象数据）若 redirect 到自身 query 会循环。
      ...(item.redirect && !item.component ? { redirect: item.redirect } : {}),
      meta: item.meta as never,
    } as RouteRecordRaw);
  }
}

export interface AdminSession {
  id: string;
  name: string;
  username: string;
  email?: string;
  avatar?: string;
  authority?: string;
  access?: string;
}

export type AuthIdentityMode = 'username' | 'email';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as AdminSession | null,
    profile: null as UserProfile | null,
    sessionChecked: false,
    loginLoading: false,
    accessToken: localStorage.getItem('framework_access_token') || '',
    refreshToken: localStorage.getItem('framework_refresh_token') || '',
    permCodes: [] as string[],
    dynamicRoutes: [] as RouteItem[],
    dynamicRoutesLoaded: false,
    // 标记动态路由刚刚加载完毕，router 守卫据此触发一次重定向以重新解析 to
    justLoadedDynamicRoutes: false,
    // 登录安全增强：MFA 临时状态
    mfaToken: '' as string,
    mfaSetupData: null as { secret: string; otpauthUrl: string } | null,
  }),
  getters: {
    displayLabel(state): string {
      if (!state.user) return '';
      const n = state.user.name?.trim();
      if (n) return n;
      const e = state.user.email?.trim();
      return e || `id:${state.user.id}`;
    },
    avatarLetter(state): string {
      const s = (state.user?.name || state.user?.email || '?').trim();
      return s ? s.slice(0, 1).toUpperCase() : '?';
    },
    isLoggedIn(state): boolean {
      return !!state.accessToken && !!state.user;
    },
    // 是否有后端下发的动态菜单数据；为空时前端使用静态侧栏兜底
    hasDynamicRoutes(state): boolean {
      return state.dynamicRoutes.length > 0;
    },
  },
  actions: {
    async ensureSession() {
      if (this.sessionChecked) return;
      if (this.accessToken) {
        try {
          const profile = await authService.getMe();
          this.profile = profile;
          this.user = {
            id: String(profile.id),
            name: profile.nickname || profile.username,
            username: profile.username,
            email: profile.email,
            avatar: profile.avatar,
            authority: profile.authority,
            access: profile.authority,
          };
          localStorage.setItem('framework_session', JSON.stringify(this.user));
          // 拉取动态路由与权限码（失败不阻塞，降级静态菜单）
          await this.loadDynamicRoutes();
        } catch {
          this.clearTokens();
          localStorage.removeItem('framework_session');
          this.user = null;
          this.profile = null;
          this.dynamicRoutes = [];
          this.dynamicRoutesLoaded = false;
          this.justLoadedDynamicRoutes = false;
          this.permCodes = [];
        }
      } else {
        // 无 token 不再从 localStorage 残留恢复 user（避免守卫放行但 API 全 401）
        this.user = null;
        this.profile = null;
        this.dynamicRoutes = [];
        this.dynamicRoutesLoaded = false;
        this.justLoadedDynamicRoutes = false;
        this.permCodes = [];
      }
      this.sessionChecked = true;
    },

    forceRecheckSession() {
      this.sessionChecked = false;
    },

    // 个人资料保存后重新拉取，同步顶栏会话（昵称/头像）与本地缓存
    async refreshProfile() {
      const profile = await authService.getMe();
      this.profile = profile;
      this.user = {
        id: String(profile.id),
        name: profile.nickname || profile.username,
        username: profile.username,
        email: profile.email,
        avatar: profile.avatar,
        authority: profile.authority,
        access: profile.authority,
      };
      localStorage.setItem('framework_session', JSON.stringify(this.user));
    },

    // 拉取后端下发的动态路由树与权限码；各自独立失败，互不影响。
    // 路由树通过 router.addRoute('main-layout', ...) 注册为 MainLayout 子路由。
    // 用动态 import('../router') 打破 auth.ts ↔ router/index.ts 的循环依赖。
    async loadDynamicRoutes() {
      if (this.dynamicRoutesLoaded) return;
      const [routesResult, codesResult] = await Promise.allSettled([
        routerService.getRoutes(),
        routerService.getPermCodes(),
      ]);
      this.dynamicRoutes = routesResult.status === 'fulfilled' ? (routesResult.value?.items ?? []) : [];
      this.permCodes = codesResult.status === 'fulfilled' ? (codesResult.value?.codes ?? []) : [];

      // 注册动态路由为 main-layout 子路由
      const { default: router } = await import('../router');
      for (const item of this.dynamicRoutes) {
        registerDynamicRoute(router, item);
      }

      // 菜单拉取失败（超时/被中止）不置 loaded：允许后续导航守卫重试，
      // 否则深链在本会话永久命中 404 兜底（2026-08-21 深链掉 404 事故根因）
      this.dynamicRoutesLoaded = routesResult.status === 'fulfilled';
      this.justLoadedDynamicRoutes = this.dynamicRoutesLoaded;
    },

    async login(mode: AuthIdentityMode, identity: string, password: string, mfaCode?: string) {
      const id = identity.trim();
      if (!id || !password) {
        throw new Error('identity and password required');
      }
      this.loginLoading = true;
      try {
        const req: LoginRequest = {
          grant_type: 'password',
          username: id,
          password,
          mfaCode,
        };
        const resp = await authService.login(req);
        // MFA 二步：服务端返回 mfaRequired=true 时缓存 mfaToken，抛出特定信号由 LoginPage 切换步骤
        if (resp.mfaRequired) {
          this.mfaToken = resp.mfaToken || '';
          const err = new Error('MFA_REQUIRED') as Error & { mfaRequired: boolean };
          err.mfaRequired = true;
          throw err;
        }
        await this.applyLoginSuccess(resp);
      } finally {
        this.loginLoading = false;
      }
    },

    // 登录成功后统一应用 token 与会话加载逻辑（login 与 verifyMFA 共用）
    async applyLoginSuccess(resp: { access_token: string; refresh_token: string }) {
      // 新会话开始，解除登出期 401 静默（残留旧轮询此时已全部失效）
      setLoggingOut(false);
      this.accessToken = resp.access_token;
      this.refreshToken = resp.refresh_token;
      localStorage.setItem('framework_access_token', resp.access_token);
      localStorage.setItem('framework_refresh_token', resp.refresh_token);

      const profile = await authService.getMe();
      this.profile = profile;
      this.user = {
        id: String(profile.id),
        name: profile.nickname || profile.username,
        username: profile.username,
        email: profile.email,
        avatar: profile.avatar,
        authority: profile.authority,
        access: profile.authority,
      };
      localStorage.setItem('framework_session', JSON.stringify(this.user));
      this.sessionChecked = true;
      this.mfaToken = '';
      await this.loadDynamicRoutes();
    },

    // MFA 二步登录验证
    async verifyMFA(mfaCode: string) {
      if (!this.mfaToken) {
        throw new Error('mfa token missing');
      }
      this.loginLoading = true;
      try {
        const resp = await authService.verifyMFA(this.mfaToken, mfaCode);
        await this.applyLoginSuccess(resp);
      } finally {
        this.loginLoading = false;
      }
    },

    // 启用 MFA：返回 secret + otpauthUrl，前端渲染 QR 供用户扫描
    async enableMFA() {
      const resp = await authService.enableMFA();
      this.mfaSetupData = resp;
      return resp;
    },

    // 禁用 MFA
    async disableMFA(mfaCode: string) {
      await authService.disableMFA(mfaCode);
      this.mfaSetupData = null;
    },

    // 登出：先同步撤销服务端令牌（带 Authorization 头），再以本地清理收尾。
    // 撤销等待设上限（网关/后端拥塞时不再让用户干等 30s 超时），超时后跳转登录页、
    // 撤销请求在后台继续完成（request.ts 已将其排除在路由切换取消之外）。
    // 全程 setLoggingOut(true)：残留轮询的 401 不再触发 refresh/全量刷新/错误 toast。
    async logout() {
      setLoggingOut(true);
      try {
        await Promise.race([
          authService.logout(),
          new Promise<void>((resolve) => setTimeout(resolve, LOGOUT_REVOKE_TIMEOUT_MS)),
        ]);
      } catch {
        // 撤销失败/超时静默降级：本地会话照常清理，令牌随 exp 自然过期
      }
      // 移除动态路由，避免切换账号后路由残留
      const { default: router } = await import('../router');
      for (const name of addedRouteNames) {
        router.removeRoute(name);
      }
      addedRouteNames.length = 0;
      this.clearTokens();
      localStorage.removeItem('framework_session');
      this.user = null;
      this.profile = null;
      this.dynamicRoutes = [];
      this.dynamicRoutesLoaded = false;
      this.justLoadedDynamicRoutes = false;
      this.permCodes = [];
      this.sessionChecked = true;
    },

    clearTokens() {
      this.accessToken = '';
      this.refreshToken = '';
      localStorage.removeItem('framework_access_token');
      localStorage.removeItem('framework_refresh_token');
    },
  },
});
