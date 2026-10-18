/** ⑤ MAP 页面共享类型与工具（后端 DTO 直接复用 services 导出类型）。 */

export type {
  MapChannel,
  MapAccount,
  MapDispatchUnitItem,
  MapDispatchUnitDetail,
  MapApprovalRecordItem,
  MapReceipt,
  MapCalendarItem,
  MapAnalyticsView,
} from '../../../services/index';

/** 错误消息抽取（kratos status error → message 字段；兜底层级化）。 */
export function errMessage(e: unknown): string {
  if (e == null) return '未知错误';
  if (typeof e === 'string') return e;
  const anyErr = e as Record<string, unknown>;
  // requestHandler 抛出的通常是 { code, message, reason } 或 Error
  if (typeof anyErr.message === 'string' && anyErr.message) return anyErr.message;
  if (typeof anyErr.reason === 'string') return anyErr.reason;
  return String(e);
}

/** 审批状态徽标配色。 */
export function approvalStateColor(state?: string): string {
  switch (state) {
    case 'approved':
      return 'positive';
    case 'rejected':
      return 'negative';
    case 'draft':
    default:
      return 'warning';
  }
}

/** 发布状态徽标配色。 */
export function publishStateColor(state?: string): string {
  switch (state) {
    case 'published':
      return 'positive';
    case 'publishing':
      return 'info';
    case 'failed':
      return 'negative';
    case 'pending':
    default:
      return 'grey';
  }
}

/** 账号健康徽标（健康状态可视：healthy ✅ / degraded ⚠️ / paused ⏸ / credential_invalid 🔑）。 */
export function healthMeta(health?: string): { color: string; icon: string; label: string } {
  switch (health) {
    case 'healthy':
      return { color: 'positive', icon: 'check_circle', label: '正常' };
    case 'degraded':
      return { color: 'warning', icon: 'warning', label: '降级' };
    case 'paused':
      return { color: 'grey', icon: 'pause_circle', label: '熔断暂停' };
    case 'credential_invalid':
      return { color: 'negative', icon: 'key_off', label: '凭据失效' };
    default:
      return { color: 'grey', icon: 'help', label: health || '未知' };
  }
}

/** 可见性徽标（非 private 一律醒目警示色）。 */
export function visibilityMeta(v?: string): { color: string; icon: string; label: string } {
  switch (v) {
    case 'public':
      return { color: 'negative', icon: 'public', label: '公开' };
    case 'unlisted':
      return { color: 'warning', icon: 'link_off', label: '不公开列出' };
    case 'private':
    default:
      return { color: 'positive', icon: 'lock', label: '私密' };
  }
}

/** 时间戳格式化（ISO → 本地短格式；空值占位 -）。 */
export function fmtTime(ts?: string | null): string {
  if (!ts) return '-';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/**
 * 分页全量取数：服务端对 pageSize 有上限（建议 100），
 * 列表型页面（账号/渠道/回执导出）须循环取全，避免被静默截断。
 */
export async function fetchAllPages<T>(
  fetchPage: (page: number, pageSize: number) => Promise<{ items?: T[]; total?: number }>,
  pageSize = 100,
): Promise<{ items: T[]; total: number }> {
  const first = await fetchPage(1, pageSize);
  const items = [...(first.items ?? [])];
  const total = first.total ?? items.length;
  const pages = Math.ceil(total / pageSize);
  for (let p = 2; p <= pages; p += 1) {
    // 串行翻页：矩阵数据量小，避免并发打服务端
    const res = await fetchPage(p, pageSize);
    items.push(...(res.items ?? []));
  }
  return { items, total };
}
