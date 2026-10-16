<template>
  <q-layout view="hHh LpR fFf" class="app-layout-root" :class="{ 'app-screen-mode': screenMode }">
    <q-header :elevated="false" :class="isDark ? 'dark-header' : 'cream-header'">
      <q-toolbar class="q-pl-sm">
        <q-btn flat dense round icon="menu" aria-label="Menu" @click="drawerOpen = !drawerOpen" />
        <q-btn
          flat
          dense
          round
          :icon="drawerMini ? 'chevron_right' : 'chevron_left'"
          :aria-label="t('common.expandSidebar')"
          @click="drawerMini = !drawerMini"
        />
        <q-toolbar-title class="q-pl-sm app-toolbar-title text-weight-bold">
          {{ displayName }}
        </q-toolbar-title>
        <q-space />
        <div class="row items-center q-gutter-sm q-mr-sm app-header-actions">
          <header-duty-clock />
          <notification-bell class="app-header-icon-btn" />
          <q-btn round flat dense class="cursor-pointer">
            <UserAvatar :avatar="auth.user?.avatar" :name="auth.displayLabel" :size="30" class="app-header-avatar" />
            <q-menu anchor="bottom right" self="top right">
              <q-list dense style="min-width: 180px">
                <q-item>
                  <q-item-section class="text-caption text-grey">{{ auth.displayLabel }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item v-close-popup clickable to="/profile">
                  <q-item-section avatar>
                    <q-icon name="person" />
                  </q-item-section>
                  <q-item-section>{{ t('menu.profile') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="avatarDialog = true">
                  <q-item-section avatar>
                    <q-icon name="photo_camera" />
                  </q-item-section>
                  <q-item-section>{{ t('profile.editAvatar') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable :disable="logoutLoading" @click="onLogout">
                  <q-item-section v-if="logoutLoading" avatar>
                    <q-spinner size="16px" />
                  </q-item-section>
                  <q-item-section>{{ t('auth.logout') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
        <q-btn flat round class="app-header-icon-btn" :icon="themeIcon" :aria-label="t('common.autoMode')">
          <q-menu anchor="bottom right" self="top right" auto-close>
            <q-list dense style="min-width: 160px">
              <q-item clickable :active="themeMode === 'auto'" active-class="text-primary" @click="setTheme('auto')">
                <q-item-section avatar>
                  <q-icon name="brightness_auto" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.autoMode') }}</q-item-section>
              </q-item>
              <q-item clickable :active="themeMode === 'light'" active-class="text-primary" @click="setTheme('light')">
                <q-item-section avatar>
                  <q-icon name="light_mode" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.lightMode') }}</q-item-section>
              </q-item>
              <q-item clickable :active="themeMode === 'dark'" active-class="text-primary" @click="setTheme('dark')">
                <q-item-section avatar>
                  <q-icon name="dark_mode" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.darkMode') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
        <q-select
          v-model="locale"
          :options="localeOptions"
          emit-value
          map-options
          dense
          :dark="isDark"
          borderless
          options-dense
          class="app-header-locale q-ml-sm"
          style="min-width: 96px"
        />
      </q-toolbar>
      <!-- 许可证受限模式横幅（§2.6.8/§3.7）：X-License-Status: restricted 驱动，点击跳转许可证管理页 -->
      <div v-if="license.restricted" class="license-restricted-banner" role="alert" @click="goLicensePage">
        <q-icon name="gpp_maybe" size="18px" class="license-restricted-banner__icon" />
        <span class="license-restricted-banner__text">{{ t('license.restrictedBanner') }}</span>
        <q-icon name="chevron_right" size="18px" />
      </div>
    </q-header>

    <q-drawer
      v-model="drawerOpen"
      show-if-above
      :width="232"
      :mini="drawerMini"
      :mini-width="80"
      :breakpoint="0"
      :class="['app-sidebar', { 'q-drawer--design-mini': drawerMini }]"
    >
      <div class="app-sidebar-card">
        <!-- 品牌区：logo + 产品名/副标题（mini 态仅 logo） -->
        <div class="app-sidebar-brand">
          <div class="app-sidebar-brand__logo">
            <q-icon name="view_in_ar" size="20px" />
          </div>
          <div v-show="!drawerMini" class="app-sidebar-brand__text">
            <div class="app-sidebar-brand__title">{{ displayName }}</div>
            <div class="app-sidebar-brand__subtitle">{{ t('common.appSubtitle') }}</div>
          </div>
        </div>
        <!-- 菜单可见性=角色权限树全量（2026-08-21 评审修订：三视图切换器下线，不再按业务面二次过滤） -->
        <div ref="navScrollRef" class="app-sidebar__scroll fit">
          <q-list class="app-sidebar-nav" :dense="drawerMini">
            <template v-for="(group, gi) in navGroups" :key="`g-${gi}`">
              <button
                v-show="!drawerMini"
                type="button"
                class="app-sidebar-section-label"
                :class="{
                  'is-collapsed': isGroupCollapsed(group),
                  'is-current': groupHasActive(group),
                }"
                :aria-expanded="!isGroupCollapsed(group)"
                :aria-label="
                  isGroupCollapsed(group)
                    ? t('common.expandGroup', { name: safeT(group.labelKey) })
                    : t('common.collapseGroup', { name: safeT(group.labelKey) })
                "
                @click="toggleGroup(group)"
              >
                <span class="app-sidebar-section-label__text">{{ safeT(group.labelKey) }}</span>
                <q-icon
                  class="app-sidebar-section-label__chevron"
                  :name="isGroupCollapsed(group) ? 'chevron_right' : 'expand_more'"
                  size="16px"
                />
              </button>
              <div v-show="drawerMini || !isGroupCollapsed(group)" class="app-sidebar-group__items">
                <template v-for="(item, ii) in group.items" :key="`g-${gi}-i-${ii}`">
                  <q-separator v-if="item.divider" v-show="!drawerMini" class="app-sidebar-subdivider" />
                  <q-item
                    v-ripple
                    clickable
                    class="app-sidebar-nav-item"
                    :active="isNavItemActive(item)"
                    active-class="app-sidebar-item--active"
                    @click="navigateTo(item.to)"
                  >
                    <q-tooltip v-if="drawerMini" anchor="center right" self="center left" :offset="[8, 0]">
                      {{ safeT(item.labelKey) }}
                    </q-tooltip>
                    <q-item-section avatar>
                      <q-icon :name="item.icon" />
                    </q-item-section>
                    <q-item-section v-show="!drawerMini">{{ safeT(item.labelKey) }}</q-item-section>
                  </q-item>
                </template>
              </div>
              <q-separator v-if="gi < sideNavGroups.length - 1" v-show="!drawerMini" class="app-sidebar-divider" />
            </template>
          </q-list>
        </div>
      </div>
    </q-drawer>

    <q-page-container class="app-page-container">
      <!-- 页级 keep-alive：OpsCenterPage / Topology3DPage 保活（App 层 keep-alive 下本 Layout 停用时 router-view 仍按 matched 渲染对方子页面，
           故按 meta.layout 判定非本 Layout 路由渲空；keep-alive 对 null 不缓存）；
           Topology3DPage 保活后重复进入免重建 WebGL 场景（页面 onDeactivated 已停 RAF/轮询，无后台空转）；
           DiscoveryTopologyPage 已退役摘除（拓扑体系统一：运行态唯一视图 = Topology3DPage） -->
      <router-view v-slot="{ Component, route }">
        <!-- ErrorBoundary 兜底：动态路由页（菜单注册的异步组件）加载失败/渲染抛错时给出错误态+重试，
             而非静默空白（2026-09-12 报表中心「报表模板不显示」实证）。
             用 reset-key（而非 :key）在换路由时清错误态：:key 会重建整棵子树、击穿内层
             keep-alive 对 OpsCenterPage/Topology3DPage 的保活；reset-key 只复位边界不动缓存 -->
        <ErrorBoundary :reset-key="route.fullPath">
          <keep-alive :include="['OpsCenterPage', 'Topology3DPage']">
            <component :is="(!route.meta.layout || route.meta.layout === 'main') ? Component : null" />
          </keep-alive>
        </ErrorBoundary>
      </router-view>
    </q-page-container>

    <!-- 关联上下文条（信息关联可视化 D1）：全 MainLayout 页可见，BlankLayout 页不显示 -->
    <CorrelationContextBar />

    <!-- 设备全域星图（信息关联可视化 D2）：全局单例，v-if 内部控制 -->
    <DeviceUniversePanel />

    <!-- 设备 360° 全局抽屉宿主（可视化信息关联重组 P1-T2）：useDevice360().open() 一行接入 -->
    <Device360Host />

    <!-- 顶栏更换头像弹窗（类微信：不跳页直接换，内部完成上传与 store 同步） -->
    <AvatarChangeDialog
      v-model="avatarDialog"
      :avatar="auth.user?.avatar"
      :name="auth.displayLabel"
      :username="auth.user?.username"
    />
  </q-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onActivated, onMounted, onUnmounted } from 'vue';
import { useRoute, useRouter, type LocationQuery } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { setQuasarLangFor } from '../i18n/quasar-lang';
import { sideNavGroups, buildSideNavFromRoutes, injectWindShearItem, type SideNavGroup, type SideNavItem } from '../config/sideNav';
import { filterVisibleWorkbenchTabs, collectRoutePaths } from '../config/lineWorkbenchTabs';
import { injectWeatherDomainItems } from '../config/weatherDomains';
import { useAuthStore } from '../stores/auth';
import { useLicenseStore } from '../stores/license';
import { useMyNoticeStore } from '../stores/my-notice';
import { useTheme } from '../composables/useTheme';
import { useSystemName } from '../composables/useSystemName';
import NotificationBell from '../components/NotificationBell.vue';
import HeaderDutyClock from '../components/HeaderDutyClock.vue';
import UserAvatar from '../components/UserAvatar.vue';
import AvatarChangeDialog from '../components/AvatarChangeDialog.vue';
import CorrelationContextBar from '../components/correlation/CorrelationContextBar.vue';
import DeviceUniversePanel from '../components/device-universe/DeviceUniversePanel.vue';
import Device360Host from '../components/device-360/Device360Host.vue';
import ErrorBoundary from '../components/ErrorBoundary.vue';

const { t, te, locale } = useI18n();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const license = useLicenseStore();
const myNotice = useMyNoticeStore();
// 系统名称（DB system_configs.system_name 优先，i18n 兜底），同步浏览器页签标题
const { displayName } = useSystemName({ syncDocumentTitle: true });
const drawerOpen = ref(true);
// 2026-08-24：默认展开全宽菜单（232px 图标+文字），顶栏 chevron 可收回 mini 态
const drawerMini = ref(false);

// ── 大屏模式（IT 监控 06 方案 §4.4，query 驱动全局能力）：?screen=1 时隐侧栏 + 根元素挂
//    .app-screen-mode（字号/对比度见 app-global.sass）；退出后恢复侧栏先前状态。 ──
const screenMode = computed(() => route.query.screen === '1');
let drawerBeforeScreen: boolean | null = null;
watch(
  screenMode,
  (on) => {
    if (on) {
      if (drawerBeforeScreen == null) drawerBeforeScreen = drawerOpen.value;
      drawerOpen.value = false;
    } else {
      if (drawerBeforeScreen != null) {
        drawerOpen.value = drawerBeforeScreen;
        drawerBeforeScreen = null;
      }
    }
  },
  { immediate: true },
);
// 分组折叠：只藏子菜单，根目录标题留着。刷新后沿用。
const COLLAPSED_GROUPS_KEY = 'framework_sidebar_group_collapsed';
const collapsedGroups = ref(loadCollapsedGroups());
// 顶栏「更换头像」弹窗开关
const avatarDialog = ref(false);

// App 层 keep-alive include 匹配名（跨 Layout 往返时保留 MainLayout 子树 → 内层拓扑页缓存生效）
defineOptions({ name: 'MainLayout' });

onMounted(() => {
  myNotice.startPolling();
  // 刷新后首次定位：菜单滚到当前页面对应项（动态菜单异步下发时由 navGroups watch 补定位）
  void nextTick(() => syncNavScroll(false));
  // 空闲预取 3D 拓扑页 chunk（three.js 场景栈体积大；预取后首次点击菜单免去 chunk 下载等待）
  const prefetchTopo3d = () => void import('../pages/visualization/Topology3DPage.vue');
  if ('requestIdleCallback' in window) {
    window.requestIdleCallback(prefetchTopo3d, { timeout: 8000 });
  } else {
    setTimeout(prefetchTopo3d, 5000);
  }
});
onActivated(() => {
  // keep-alive 恢复时补启动（onMounted 仅首次触发；logout reset 后重进需恢复轮询，startPolling 幂等）
  myNotice.startPolling();
  // 从其他 Layout 返回时重定位（DOM 重挂后滚动位置可能丢失）
  void nextTick(() => syncNavScroll(false));
});
onUnmounted(() => {
  myNotice.stopPolling();
});

// 后端动态路由下发的 meta.title 是中文原文（如 "实时告警"），不是 i18n key。
// 直接用 t() 会触发 "Not found key" 警告；先用 te() 探测，未命中则原样返回。
function safeT(key: string): string {
  return te(key) ? t(key) : key;
}

function loadCollapsedGroups(): Set<string> {
  try {
    const raw = localStorage.getItem(COLLAPSED_GROUPS_KEY);
    if (!raw) return new Set();
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((key): key is string => typeof key === 'string' && key.length > 0));
  } catch {
    return new Set();
  }
}

function saveCollapsedGroups(keys: Set<string>) {
  try {
    localStorage.setItem(COLLAPSED_GROUPS_KEY, JSON.stringify([...keys]));
  } catch {
    /* 隐私模式等写入失败：仅本次会话生效 */
  }
}

function isGroupCollapsed(group: SideNavGroup): boolean {
  return collapsedGroups.value.has(group.labelKey);
}

function groupHasActive(group: SideNavGroup): boolean {
  return group.items.some((item) => matchNavItem(item.to, item.exact, route.path, route.query));
}

function toggleGroup(group: SideNavGroup) {
  const next = new Set(collapsedGroups.value);
  if (next.has(group.labelKey)) next.delete(group.labelKey);
  else next.add(group.labelKey);
  collapsedGroups.value = next;
  saveCollapsedGroups(next);
}

// 菜单渲染：后端下发的动态路由优先；为空（后端无菜单数据）时降级静态侧栏。
// 2026-08-21 评审修订：菜单可见性=角色权限树全量显示（admin 全显），不再按三视图二次过滤
const navGroups = computed<SideNavGroup[]>(() => {
  const base = auth.hasDynamicRoutes ? buildSideNavFromRoutes(auth.dynamicRoutes) : sideNavGroups;
  return injectWindShearItem(injectWeatherDomainItems(injectWorkbenchTabItems(base)));
});

// 监控工作台页内功能 Tab 提升为侧栏菜单项（深链 ?tab=xx 切换），
// 插入「监控工作台」项之后；权限口径与 WorkbenchPage.visibleTabs 一致（paths 命中 dynamicRoutes）
function injectWorkbenchTabItems(groups: SideNavGroup[]): SideNavGroup[] {
  const permitted = new Set<string>();
  collectRoutePaths(auth.dynamicRoutes, permitted);
  const extra: SideNavItem[] = filterVisibleWorkbenchTabs(permitted)
    .filter((d) => d.key !== 'monitor')
    .map((d) => ({ to: `/linemonitor/workbench?tab=${d.key}`, icon: d.icon, labelKey: d.labelKey, exact: true }));
  if (extra.length === 0) return groups;
  return groups.map((g) => {
    const idx = g.items.findIndex((it) => it.to === '/linemonitor/workbench');
    if (idx < 0) return g;
    return { ...g, items: [...g.items.slice(0, idx + 1), ...extra, ...g.items.slice(idx + 1)] };
  });
}

const { themeMode, isDark, setTheme } = useTheme();

const themeIcon = computed(() => {
  if (themeMode.value === 'auto') return 'brightness_auto';
  return isDark.value ? 'dark_mode' : 'light_mode';
});

const localeOptions = [
  { label: t('common.localeZhCN'), value: 'zh-CN' as const },
  { label: t('common.localeEnUS'), value: 'en-US' as const },
];

watch(locale, (v) => {
  setQuasarLangFor(String(v));
  if (v === 'zh-CN' || v === 'en-US') {
    localStorage.setItem('locale', v);
  }
});

function isNavItemActive(item: { to: string; exact?: boolean }) {
  return matchNavItem(item.to, item.exact, route.path, route.query);
}

// 菜单项与路由的匹配规则（isNavItemActive 与滚动跟随共用同一口径）
function matchNavItem(itemTo: string, exact: boolean | undefined, path: string, query: LocationQuery): boolean {
  // 深链项（itemTo 带 query，如 ?tab=xx / ?domain=xx）：path 匹配且携带的每个 query 键值均一致
  const qIdx = itemTo.indexOf('?');
  if (qIdx >= 0) {
    if (path !== itemTo.slice(0, qIdx)) return false;
    let matched = true;
    new URLSearchParams(itemTo.slice(qIdx + 1)).forEach((value, key) => {
      if (query[key] !== value) matched = false;
    });
    return matched;
  }
  if (itemTo === '/linemonitor/workbench') {
    const tab = query.tab;
    return path === itemTo && (!tab || tab === 'monitor');
  }
  if (exact === true) return path === itemTo;
  return path === itemTo || path.startsWith(itemTo + '/');
}

async function navigateTo(path: string) {
  // 比对 fullPath：同 path 不同 query 的深链项（?tab=xx）也必须触发导航
  if (route.fullPath === path) return;
  await router.push(path);
}

// ── 左侧菜单主动跟随右侧页面 ──────────────────────────────────────────────
// 刷新/路由变化后自动滚动侧栏，使当前页面对应的菜单项进入可视区。
// 三层触发：beforeEach 导航开始即跟随（不等懒加载页面 chunk，点击即时响应）；
// route watch 在导航确认后校正（重定向/取消场景）；navGroups watch 兜底动态菜单异步下发。
const navScrollRef = ref<HTMLElement | null>(null);

// 与侧栏 DOM 渲染顺序一致的扁平菜单项（组顺序渲染、组内 item 顺序渲染，divider/q-separator 不占 item 类名）
const flatNavItems = computed<SideNavItem[]>(() => navGroups.value.flatMap((g) => g.items));

function scrollNavElIntoView(el: HTMLElement, smooth: boolean) {
  const container = navScrollRef.value;
  if (!container) return;
  const cRect = container.getBoundingClientRect();
  const iRect = el.getBoundingClientRect();
  // 已完整可见（含 8px 余量）则不滚动，避免用户浏览菜单时被无谓拽动
  const margin = 8;
  if (iRect.top >= cRect.top + margin && iRect.bottom <= cRect.bottom - margin) return;
  // 目标位置：菜单项垂直居中（只滚侧栏容器，不影响页面滚动）
  const target = container.scrollTop + (iRect.top - cRect.top) - (container.clientHeight - iRect.height) / 2;
  container.scrollTo({ top: Math.max(target, 0), behavior: smooth ? 'smooth' : 'auto' });
}

// 按路由定位菜单项并滚动（导航开始时即可调用，不依赖 active class 已落地）
function scrollNavToRoute(path: string, query: LocationQuery, smooth: boolean) {
  const container = navScrollRef.value;
  if (!container) return;
  const idx = flatNavItems.value.findIndex((it) => matchNavItem(it.to, it.exact, path, query));
  if (idx < 0) return;
  const el = container.querySelectorAll<HTMLElement>('.app-sidebar-nav-item')[idx];
  // 分组收起后子项仍在 DOM 里，但高度为 0，滚它会把菜单拽偏
  if (el && getComputedStyle(el).display !== 'none') scrollNavElIntoView(el, smooth);
}

// 按当前已渲染的 active 项滚动（刷新/keep-alive 恢复/菜单异步下发后兜底）
function syncNavScroll(smooth: boolean) {
  const activeEl = navScrollRef.value?.querySelector<HTMLElement>('.app-sidebar-item--active');
  if (activeEl) scrollNavElIntoView(activeEl, smooth);
}

// 导航开始即跟随：点击菜单/页内跳转的瞬间侧栏就滚动就位，不等页面 chunk 加载确认
const removeNavFollowGuard = router.beforeEach((to) => {
  scrollNavToRoute(to.path, to.query, true);
});
// 导航确认后校正（重定向/取消等场景；已可见时 no-op）
watch(
  () => route.fullPath,
  () => void nextTick(() => syncNavScroll(true)),
);
// 动态菜单异步下发后补一次定位（覆盖刷新时菜单数据尚未就绪的场景）
watch(navGroups, () => void nextTick(() => syncNavScroll(false)));

// 退出登录防重入：进行中再次点击直接忽略（菜单项同步置 loading/disable），
// 避免并发触发多次登出请求导致的撤销冲突与界面卡顿
const logoutLoading = ref(false);

async function onLogout() {
  if (logoutLoading.value) return;
  logoutLoading.value = true;
  try {
    myNotice.reset();
    await auth.logout();
    await router.push('/login');
  } finally {
    logoutLoading.value = false;
  }
}

async function goLicensePage() {
  if (route.path !== '/business/license') {
    await router.push('/business/license');
  }
}
</script>

<style lang="sass">
.license-restricted-banner
  display: flex
  align-items: center
  justify-content: center
  gap: 8px
  padding: 8px 16px
  cursor: pointer
  font-size: 13px
  font-weight: 500
  color: var(--color-on-accent)
  background: color-mix(in srgb, var(--color-warning) 88%, var(--canvas-base))
  transition: filter 0.2s

  &:hover
    filter: brightness(1.05)

  &__text
    line-height: 1.4

body.body--dark
  .license-restricted-banner
    background: color-mix(in srgb, var(--color-warning) 32%, var(--canvas-base))
    color: var(--color-text-heading)
    border-top: 1px solid color-mix(in srgb, var(--color-warning) 45%, transparent)
</style>
