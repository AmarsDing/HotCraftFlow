import { watch } from 'vue';
import { defineBoot } from '#q-app/wrappers';
import { Dark } from 'quasar';

/**
 * Theme boot — supports 'auto' | 'dark' | 'light' modes.
 *
 * - 'auto' (default): follows system `prefers-color-scheme`.
 * - 'dark' / 'light': explicit user preference, overrides system.
 */
export default defineBoot(() => {
  if (import.meta.env.SSR) return;
  const raw = localStorage.getItem('theme');
  if (raw === 'dark') {
    Dark.set(true);
  } else if (raw === 'light') {
    Dark.set(false);
  } else {
    const prefersDark =
      typeof window !== 'undefined' && window.matchMedia
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
        : false;
    Dark.set(prefersDark);
  }
  // Quasar Dark 只标记 body.body--dark；视口滚动条属于 <html>（body 之上），
  // 需要 html 级的主题钩子才能命中暗色 CSS 变量。所有切换路径（boot/useTheme/系统监听）
  // 都经过 Dark.set，故单点 watch 即可全覆盖。
  watch(
    () => Dark.isActive,
    (val) => document.documentElement.classList.toggle('theme-dark', val),
    { immediate: true },
  );
});
