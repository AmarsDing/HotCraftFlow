import type { QTableColumn } from "quasar";

/** FrameWork stub: keep column widths as declared */
export function withAutoColWidths<T extends QTableColumn>(columns: T[]): T[] {
  return columns.map((col) => {
    if (col.style || col.headerStyle || col.width) return col;
    return { ...col, style: "max-width: 280px", headerStyle: "max-width: 280px" };
  });
}

/** FrameWork stub: stringify cell value for default body slot */
export function qTableCellText(slotData: { value?: unknown; row?: Record<string, unknown>; col?: { name?: string; field?: string | ((row: Record<string, unknown>) => unknown) } }, truncate = false): string {
  let v: unknown = slotData?.value;
  if (v === undefined && slotData?.row && slotData.col) {
    const field = slotData.col.field ?? slotData.col.name;
    if (typeof field === "function") v = field(slotData.row);
    else if (typeof field === "string") v = slotData.row[field];
  }
  if (v == null) return "";
  const s = typeof v === "object" ? JSON.stringify(v) : String(v);
  if (truncate && s.length > 64) return s.slice(0, 64) + "...";
  return s;
}