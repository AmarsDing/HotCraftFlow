// CSV 导出工具（utf-8-sig，Excel 直开不乱码）。
// 单源口径：IT 监控报表页/设备列表批量导出共用，禁止各页再手写私有 csvCell。

/** 单元格转义：含引号/逗号/换行时整格加引号并双写内部引号 */
export function csvCell(v: unknown): string {
  const s = String(v ?? '');
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/** 组装 CSV 文本（带 BOM + CRLF 行尾） */
export function buildCsv(header: unknown[], rows: unknown[][]): string {
  return '\uFEFF' + [header, ...rows].map((r) => r.map(csvCell).join(',')).join('\r\n');
}

/** 触发浏览器下载 */
export function downloadCsv(filename: string, header: unknown[], rows: unknown[][]): void {
  const blob = new Blob([buildCsv(header, rows)], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}
