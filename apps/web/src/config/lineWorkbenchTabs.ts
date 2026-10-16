/** FrameWork stub: no line-monitor workbench tabs */
export type WorkbenchTabDef = { key: string; icon: string; labelKey: string };

export function filterVisibleWorkbenchTabs(_permitted: Set<string>): WorkbenchTabDef[] {
  return [];
}

export function collectRoutePaths(items: unknown, out: Set<string>): void {
  if (!Array.isArray(items)) return;
  for (const raw of items) {
    const it = raw as { path?: string; children?: unknown[] };
    if (typeof it?.path === "string" && it.path) out.add(it.path);
    if (Array.isArray(it?.children)) collectRoutePaths(it.children, out);
  }
}
