// 大屏模式（06 方案 §4.4）：?screen=1 驱动的全局只读展示态。
// 行为：
//  - MainLayout 监听 query：隐侧栏 + 根元素加 .app-screen-mode（字号放大/对比度增强）；
//  - localStorage 记忆（默认键 it-monitor:screen-mode）：IT 监控页挂载时若记忆为开且 query 未显式指定，
//    自动补 screen=1，跨页保持大屏态；
//  - Esc 退出（清 query 与记忆）；
//  - 大屏态下自动刷新锁 30s（useRefreshTimer 的 lockedLevel 消费 screenMode）。
import { computed, onBeforeUnmount, onMounted, type ComputedRef } from 'vue';
import { useRoute, useRouter } from 'vue-router';

const STORAGE_KEY = 'it-monitor:screen-mode';

export interface ScreenMode {
  /** 当前是否大屏态（query 生效） */
  active: ComputedRef<boolean>;
  /** 进入大屏（写 query + 记忆） */
  enter: () => void;
  /** 退出大屏（清 query + 记忆） */
  exit: () => void;
  toggle: () => void;
}

export function useScreenMode(): ScreenMode {
  const route = useRoute();
  const router = useRouter();

  const active = computed(() => route.query.screen === '1');

  function enter() {
    localStorage.setItem(STORAGE_KEY, '1');
    if (!active.value) {
      void router.replace({ query: { ...route.query, screen: '1' } });
    }
  }

  function exit() {
    localStorage.removeItem(STORAGE_KEY);
    if (active.value) {
      const q = { ...route.query };
      delete q.screen;
      void router.replace({ query: q });
    }
  }

  function toggle() {
    if (active.value) exit();
    else enter();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && active.value) exit();
  }

  // 挂载时：记忆为开且 query 未显式指定 screen → 补 query（跨页保持）
  onMounted(() => {
    if (route.query.screen == null && localStorage.getItem(STORAGE_KEY) === '1') {
      void router.replace({ query: { ...route.query, screen: '1' } });
    }
    window.addEventListener('keydown', onKeydown);
  });
  onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown));

  return { active, enter, exit, toggle };
}
