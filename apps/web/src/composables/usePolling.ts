import { ref, unref, onUnmounted, onActivated, onDeactivated, type Ref } from 'vue';

export function usePolling(fn: () => Promise<void>, interval: number | Ref<number>) {
  const isPolling = ref(false);
  let timerId: ReturnType<typeof setInterval> | null = null;
  // 防重叠：上一次执行未完成时跳过本次 tick（避免慢请求下并发堆叠）
  let running = false;
  // keep-alive 失活标记：deactivated 期间页面隐藏恢复也不复活轮询
  let deactivated = false;
  // deactivated 前是否处于轮询中（activated 时据此恢复）
  let wasPolling = false;

  async function run() {
    if (running) return;
    running = true;
    try {
      await fn();
    } finally {
      running = false;
    }
  }

  function armTimer() {
    if (timerId === null) {
      timerId = setInterval(() => void run(), unref(interval));
    }
  }

  function disarmTimer() {
    if (timerId !== null) {
      clearInterval(timerId);
      timerId = null;
    }
  }

  function start() {
    if (isPolling.value) return;
    isPolling.value = true;
    void run();
    armTimer();
  }

  function stop() {
    isPolling.value = false;
    disarmTimer();
  }

  function handleVisibilityChange() {
    if (document.hidden) {
      disarmTimer();
    } else if (isPolling.value && !deactivated) {
      void run();
      armTimer();
    }
  }

  document.addEventListener('visibilitychange', handleVisibilityChange);

  // keep-alive 感知：切走停 timer 但保留 isPolling 语义，切回立即补一轮并重排 timer
  onDeactivated(() => {
    deactivated = true;
    wasPolling = isPolling.value;
    disarmTimer();
  });

  onActivated(() => {
    deactivated = false;
    if (wasPolling && isPolling.value && !document.hidden) {
      void run();
      armTimer();
    }
    wasPolling = false;
  });

  onUnmounted(() => {
    stop();
    document.removeEventListener('visibilitychange', handleVisibilityChange);
  });

  return {
    isPolling,
    start,
    stop,
  };
}
