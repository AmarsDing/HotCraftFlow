import { boot } from 'quasar/wrappers';

// 全站表格长文本悬停提示：
// 单元格被 CSS ellipsis 截断（scrollWidth > clientWidth）时，向 td/th 注入原生 title 显示全文；
// 未截断时移除残留 title。采用 document 级事件委托，所有 q-table / q-markup-table 零改造生效。
// 含交互控件（按钮/复选框/弹出编辑）的单元格跳过，避免遮挡控件自身的 q-tooltip。
export default boot(() => {
  document.addEventListener(
    'mouseover',
    (ev) => {
      const target = ev.target as HTMLElement | null;
      const cell = target?.closest?.('.q-table td, .q-table th, .q-markup-table td, .q-markup-table th') as
        | HTMLElement
        | null;
      if (!cell) return;
      if (cell.querySelector('.q-btn, .q-checkbox, .q-popup-edit, .q-toggle')) return;
      if (cell.scrollWidth > cell.clientWidth + 1) {
        const text = cell.textContent?.trim() ?? '';
        if (text) cell.title = text;
      } else if (cell.title) {
        cell.removeAttribute('title');
      }
    },
    { passive: true },
  );
});
