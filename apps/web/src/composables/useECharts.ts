import { shallowRef, onMounted, onUnmounted, type Ref } from 'vue';
import * as echarts from 'echarts';

export function useECharts(el: Ref<HTMLElement | null>) {
  // shallowRef：ref() 的 UnwrapRef 映射会丢 EChartsType 的类私有成员导致类型不兼容
  const chart = shallowRef<echarts.EChartsType | null>(null);

  function isDarkTheme(): boolean {
    return document.body.classList.contains('body--dark');
  }

  function init() {
    if (!el.value) return;
    chart.value = echarts.init(el.value, undefined, { renderer: 'canvas' });
  }

  // ensureChart 自愈取实例：宿主元素被 v-if/v-else 重建后，旧实例仍绑定已卸载的游离 DOM，
  // setOption 会静默渲染进不可见元素（页面空白）；此处检测 getDom 漂移后弃旧建新。
  function ensureChart(): echarts.EChartsType | null {
    const dom = el.value;
    if (!dom || !dom.isConnected) return null;
    if (chart.value && (chart.value.isDisposed() || chart.value.getDom() !== dom)) {
      if (!chart.value.isDisposed()) chart.value.dispose();
      chart.value = null;
    }
    if (!chart.value) {
      chart.value = echarts.init(dom, undefined, { renderer: 'canvas' });
    }
    return chart.value;
  }

  // 减少动态效果偏好（prefers-reduced-motion）：在 setOption 出口统一关闭图表动画，
  // 覆盖所有经 ChartPanel/useECharts 渲染的图表；不注册 change 监听——
  // 偏好变更随下一次 setOption 自然生效，不干扰主题切换重渲机制。
  function prefersReducedMotion(): boolean {
    return (
      typeof window !== 'undefined' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
    );
  }

  function setOption(option: echarts.EChartsOption) {
    const finalOption = prefersReducedMotion() ? { ...option, animation: false } : option;
    ensureChart()?.setOption(finalOption, { notMerge: true });
  }

  function resize() {
    const dom = el.value;
    if (!chart.value || !dom || chart.value.isDisposed() || chart.value.getDom() !== dom) return;
    chart.value.resize();
  }

  function dispose() {
    chart.value?.dispose();
    chart.value = null;
  }

  function handleResize() {
    resize();
  }

  // 导出当前画布为 PNG（2x 像素密度），供 ChartPanel 导出按钮调用
  function exportImage(filename = 'chart.png') {
    const inst = ensureChart();
    if (!inst) return;
    const url = inst.getDataURL({ type: 'png', pixelRatio: 2, backgroundColor: 'transparent' });
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
  }

  // ── 主题翻转自动重渲 ─────────────────────────────────────────────
  // 图表颜色在 option 构建期取定（chartTheme.ts 读 CSS 变量），切换昼夜主题后
  // 必须重新构建 option 才能变色。页面把自身的 renderChart 注册进来，
  // body.body--dark class 翻转时此处统一触发重渲（防抖合并多次 class 变化）。
  const renderers = new Set<() => void>();
  let themeObserver: MutationObserver | null = null;
  let rerenderTimer: ReturnType<typeof setTimeout> | null = null;

  function registerRenderer(fn: () => void) {
    renderers.add(fn);
  }

  function scheduleRerender() {
    if (rerenderTimer) clearTimeout(rerenderTimer);
    rerenderTimer = setTimeout(() => {
      rerenderTimer = null;
      renderers.forEach((fn) => fn());
    }, 120);
  }

  // 容器尺寸观察：图表宿主常位于 Tab/keep-alive/弹窗内，挂载时可能 display:none（0 宽）。
  // 此时 init/setOption 计算的布局不可用，且 0→非0 变化不触发 window resize，
  // 导致绘图区被压缩成竖线（实证 2026-08-11 数据查询历史页）。ResizeObserver 在容器
  // 获得真实尺寸时主动 resize 重排，覆盖窗口 resize 盲区。
  // 注意：回调内需用 rAF 打破同步链，否则 chart.resize() 可能再次改变容器尺寸，
  // 触发 "ResizeObserver loop completed with undelivered notifications" 警告。
  let resizeObserver: ResizeObserver | null = null;
  let resizeRafId: number | null = null;

  onMounted(() => {
    init();
    window.addEventListener('resize', handleResize);
    themeObserver = new MutationObserver(scheduleRerender);
    themeObserver.observe(document.body, { attributes: true, attributeFilter: ['class'] });
    if (el.value && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => {
        const dom = el.value;
        if (!dom || dom.clientWidth === 0 || dom.clientHeight === 0) return;
        if (resizeRafId !== null) cancelAnimationFrame(resizeRafId);
        resizeRafId = requestAnimationFrame(() => {
          resizeRafId = null;
          // 0→非0 时旧实例可能仍绑定错误尺寸，先校验实例健康再 resize
          ensureChart()?.resize();
        });
      });
      resizeObserver.observe(el.value);
    }
  });

  onUnmounted(() => {
    window.removeEventListener('resize', handleResize);
    themeObserver?.disconnect();
    themeObserver = null;
    if (rerenderTimer) clearTimeout(rerenderTimer);
    renderers.clear();
    resizeObserver?.disconnect();
    resizeObserver = null;
    if (resizeRafId !== null) cancelAnimationFrame(resizeRafId);
    resizeRafId = null;
    dispose();
  });

  return {
    chart,
    setOption,
    resize,
    dispose,
    isDarkTheme,
    exportImage,
    registerRenderer,
  };
}
