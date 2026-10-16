// 在线远程会话徽标共享状态（远程运维能力增强 F4，概要设计 §9）。
// 数据源：opstools ListActiveSessions（status=active 的 remote_sessions）。
// 刷新策略：30s 轮询（document.hidden 跳过）+ 本地会话生命周期即时刷新——
// VncSession/TerminalSession 等通道 connect/disconnect 时调 refreshActiveSessions()，
// 本机发起的开/关即时反映；他机会话变化在下一轮轮询收敛（概要设计：30s 轮询 + 事件刷新）。
import { computed, onScopeDispose, ref, type ComputedRef } from 'vue';
import { opstoolsSessionService } from '../services';
import type { RemoteSession } from '../services/kratos/opstools/service/v1';

const POLL_INTERVAL_MS = 30_000;

// ---- 模块级单例（多页面共享一份轮询与状态） ----
const activeSessions = ref<RemoteSession[]>([]);
const loading = ref(false);
let pollTimer: ReturnType<typeof setInterval> | null = null;
let subscriberCount = 0;

// activeAssetIds 当前有活跃远程会话的资产 ID 集合（徽标判定数据源）
const activeAssetIds: ComputedRef<Set<number>> = computed(() => {
  const ids = new Set<number>();
  for (const s of activeSessions.value) {
    const id = Number(s.assetId);
    if (Number.isFinite(id) && id > 0) ids.add(id);
  }
  return ids;
});

// activeCountMap 各资产活跃会话数（tooltip 展示「N 路活跃会话」）
const activeCountMap: ComputedRef<Map<number, number>> = computed(() => {
  const map = new Map<number, number>();
  for (const s of activeSessions.value) {
    const id = Number(s.assetId);
    if (Number.isFinite(id) && id > 0) map.set(id, (map.get(id) ?? 0) + 1);
  }
  return map;
});

async function refreshActiveSessions(): Promise<void> {
  loading.value = true;
  try {
    const resp = await opstoolsSessionService.listActiveSessions();
    activeSessions.value = resp.items ?? [];
  } catch {
    // 拉取失败保留旧值（拦截器已提示），下一轮轮询自愈
  } finally {
    loading.value = false;
  }
}

function startPolling() {
  if (pollTimer !== null) return;
  void refreshActiveSessions();
  pollTimer = setInterval(() => {
    if (document.hidden) return;
    void refreshActiveSessions();
  }, POLL_INTERVAL_MS);
}

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

// useRemoteActiveSessions 订阅共享活跃会话状态（引用计数：首个订阅启动轮询，最后一个退订停止）。
export function useRemoteActiveSessions() {
  subscriberCount += 1;
  startPolling();
  onScopeDispose(() => {
    subscriberCount -= 1;
    if (subscriberCount <= 0) {
      subscriberCount = 0;
      stopPolling();
    }
  });
  return {
    activeSessions,
    activeAssetIds,
    activeCountMap,
    loading,
    refresh: refreshActiveSessions,
  };
}
