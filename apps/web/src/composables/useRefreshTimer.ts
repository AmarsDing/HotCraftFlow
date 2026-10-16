// 刷新三档（30s/60s/关）档位管理（IT 监控 06 方案 W1）。
// 设计：
//  - 档位记忆 localStorage（默认全局键，跨页一致；也可按页面传 storageKey 隔离）；
//  - 档位切换即重建定时器；keep-alive 失活停表、激活恢复；页面隐藏时跳过 tick（不堆请求）；
//  - 防重入：上一次 tick 未完成时跳过本次（慢请求下不堆叠）。
// 不复用 usePolling：其内部生命周期钩子随实例累积，档位切换重建实例会泄漏监听器。
import { computed, onActivated, onDeactivated, onUnmounted, ref, watch, type ComputedRef, type Ref } from 'vue';

export type RefreshLevel = 'off' | '30' | '60';

export interface RefreshTimerOptions {
  /** localStorage 键（默认全局：it-monitor:refresh-level） */
  storageKey?: string;
  /** 默认档位（默认 60s；大屏模式调用方传 30） */
  defaultLevel?: RefreshLevel;
  /**
   * 锁定档位（如大屏模式锁 30s）：传入响应式值后，生效档位=锁定值，
   * setLevel 仅改用户偏好不立即生效（解锁后恢复偏好档位），UI 应配合 locked 态禁切换。
   */
  lockedLevel?: Ref<RefreshLevel | null>;
}

export function useRefreshTimer(
  fn: () => void | Promise<void>,
  opts?: RefreshTimerOptions,
): { level: ComputedRef<RefreshLevel>; setLevel: (v: RefreshLevel) => void; locked: ComputedRef<boolean> } {
  const storageKey = opts?.storageKey ?? 'it-monitor:refresh-level';
  const saved = localStorage.getItem(storageKey);
  const initial: RefreshLevel =
    saved === 'off' || saved === '30' || saved === '60' ? saved : (opts?.defaultLevel ?? '60');
  /** 用户偏好档位（localStorage 记忆） */
  const pref = ref<RefreshLevel>(initial);
  const locked = computed(() => opts?.lockedLevel?.value != null);
  /** 生效档位：锁定值优先，否则用户偏好 */
  const level = computed<RefreshLevel>(() => opts?.lockedLevel?.value ?? pref.value);

  let timer: ReturnType<typeof setInterval> | null = null;
  let running = false;
  let active = true; // keep-alive 激活态

  async function tick() {
    if (running) return;
    running = true;
    try {
      await fn();
    } finally {
      running = false;
    }
  }

  function disarm() {
    if (timer !== null) {
      clearInterval(timer);
      timer = null;
    }
  }

  function arm() {
    disarm();
    if (level.value === 'off' || !active) return;
    timer = setInterval(() => {
      if (!document.hidden) void tick();
    }, Number(level.value) * 1000);
  }

  function setLevel(v: RefreshLevel) {
    if (pref.value === v) return;
    pref.value = v;
    localStorage.setItem(storageKey, v);
  }

  // flush: 'sync' 保证切档立即重建（锁定源变化同样即时生效）
  watch(level, arm, { flush: 'sync' });
  onActivated(() => {
    active = true;
    arm();
  });
  onDeactivated(() => {
    active = false;
    disarm();
  });
  onUnmounted(disarm);
  arm();

  return { level, setLevel, locked };
}
