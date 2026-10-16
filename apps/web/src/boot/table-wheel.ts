import { boot } from 'quasar/wrappers';

// 全站表格滚轮横向滚动：
// 鼠标悬停任意 q-table 表体（.q-table__middle）时，滚轮驱动横向滚动条；
// 横向已到边界（或无横向溢出）时放行，保留表内/页面纵向滚动——兼容资产中心这类横纵双滚动条表格。
// 采用 document 级捕获委托，所有 q-table（含裸用页面）零改造生效。
export default boot(() => {
  document.addEventListener(
    'wheel',
    (ev) => {
      const target = ev.target as HTMLElement | null;
      const middle = target?.closest?.('.q-table__middle') as HTMLElement | null;
      if (!middle) return;
      if (middle.scrollWidth <= middle.clientWidth) return;
      const delta = Math.abs(ev.deltaX) > Math.abs(ev.deltaY) ? ev.deltaX : ev.deltaY;
      if (delta === 0) return;
      const atStart = middle.scrollLeft <= 0 && delta < 0;
      const atEnd = middle.scrollLeft + middle.clientWidth >= middle.scrollWidth - 1 && delta > 0;
      if (atStart || atEnd) return;
      ev.preventDefault();
      middle.scrollLeft += delta;
    },
    { passive: false, capture: true },
  );
});
