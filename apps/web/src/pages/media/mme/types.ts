/** ④ MME 页面共享类型与工具（后端 DTO 直接复用 services 导出类型）。 */

export type {
  MontageTask,
  AssetInfo,
  HighlightCandidate,
  DoctorReport,
  DoctorItem,
  MmeSettings,
  VariantSetView,
  VariantItem,
  DiffResult,
  PairSimilarity,
  HookSpec,
  RenderResponse,
  RenderJob,
  SetRenderProgress,
  MontageTemplate,
  PlatformCopy,
  QC,
  MmeContractVariantSet,
} from '../../../services/index';

import { request } from '../../../services/request';

/** 错误消息抽取：优先 kratos status body（message/reason），兜底 axios 原文。 */
export function errMessage(e: unknown): string {
  if (e == null) return '未知错误';
  if (typeof e === 'string') return e;
  const anyErr = e as Record<string, unknown>;
  const resp = anyErr.response as Record<string, unknown> | undefined;
  const data = resp?.data as Record<string, unknown> | undefined;
  if (typeof data?.message === 'string' && data.message) return data.message;
  if (typeof data?.reason === 'string' && data.reason) return data.reason;
  if (typeof anyErr.reason === 'string' && anyErr.reason) return anyErr.reason;
  if (typeof anyErr.message === 'string' && anyErr.message) return anyErr.message;
  return String(e);
}

/** 时间戳格式化（ISO → 本地短格式；空值占位 -）。 */
export function fmtTime(ts?: string | null): string {
  if (!ts) return '-';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** 字节数人性化（素材大小）。 */
export function fmtBytes(n?: number | null): string {
  if (n == null || Number.isNaN(n)) return '-';
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = n;
  let i = -1;
  do {
    v = v / 1024;
    i += 1;
  } while (v >= 1024 && i < units.length - 1);
  return `${v.toFixed(1)} ${units[i]}`;
}

/** 秒数 → mm:ss。 */
export function fmtDuration(sec?: number | null): string {
  if (sec == null || Number.isNaN(sec)) return '-';
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

/** ETA 秒数人性化（-1 = 无样本，如实显示“估算中”）。 */
export function fmtEta(sec?: number | null): string {
  if (sec == null || sec < 0) return '估算中';
  if (sec < 60) return `约 ${sec}s`;
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return `约 ${m}m${s > 0 ? `${s}s` : ''}`;
}

/** 短 id 展示（ULID 太占版面，保留前 8+后 4）。 */
export function shortId(id?: string | null): string {
  if (!id) return '-';
  if (id.length <= 14) return id;
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

/** 任务状态徽标（created→done/failed 全链路）。 */
export function taskStateMeta(state?: string): { color: string; icon: string; label: string } {
  switch (state) {
    case 'done':
      return { color: 'positive', icon: 'check_circle', label: '完成' };
    case 'failed':
      return { color: 'negative', icon: 'error', label: '失败' };
    case 'downloading':
      return { color: 'info', icon: 'download', label: '下载中' };
    case 'transcribing':
      return { color: 'info', icon: 'graphic_eq', label: '转写中' };
    case 'analyzing':
      return { color: 'info', icon: 'psychology', label: '分析中' };
    case 'expanded':
      return { color: 'primary', icon: 'apps', label: '已展开' };
    case 'rendering':
      return { color: 'warning', icon: 'precision_manufacturing', label: '渲染中' };
    case 'reviewing':
      return { color: 'warning', icon: 'rate_review', label: '待审核' };
    case 'created':
    default:
      return { color: 'grey', icon: 'fiber_new', label: state || '已创建' };
  }
}

/** 变体簇状态徽标。 */
export function setStateMeta(state?: string): { color: string; label: string } {
  switch (state) {
    case 'approved':
      return { color: 'positive', label: '已批准' };
    case 'rejected':
      return { color: 'negative', label: '已驳回' };
    case 'rendering':
      return { color: 'warning', label: '渲染中' };
    case 'review':
      return { color: 'info', label: '待审核' };
    case 'planned':
    default:
      return { color: 'grey', label: state || '已规划' };
  }
}

/** 渲染任务状态徽标。 */
export function renderStateMeta(state?: string): { color: string; icon: string; label: string } {
  switch (state) {
    case 'done':
      return { color: 'positive', icon: 'check_circle', label: '完成' };
    case 'failed':
      return { color: 'negative', icon: 'error', label: '失败' };
    case 'cancelled':
      return { color: 'grey', icon: 'cancel', label: '已取消' };
    case 'running':
      return { color: 'info', icon: 'sync', label: '运行中' };
    case 'queued':
    default:
      return { color: 'warning', icon: 'schedule', label: state || '排队中' };
  }
}

/** 画幅选项（A 维度；展开对话框默认全选）。 */
export const ASPECT_OPTIONS = [
  { label: '9:16 竖屏', value: '9:16' },
  { label: '16:9 横屏', value: '16:9' },
  { label: '1:1 方屏', value: '1:1' },
];

/** QC 硬门禁判定：boundary/subtitle 任一 false → 禁止批准。未渲染（qc 为空）≠ 未过。 */
export function qcBlocked(qc?: { boundary_ok?: boolean; subtitle_ok?: boolean } | null): boolean {
  if (!qc) return false;
  return qc.boundary_ok === false || qc.subtitle_ok === false;
}

/** MME 媒体流式拉取（JWT 走 axios 拦截器；<video> 标签无法带 Authorization 头 → blob URL）。 */
export async function fetchMmeMediaUrl(relPath: string, timeout = 180000): Promise<string> {
  const blob = await request.get<Blob>('/api/v1/media/mme/media:get', {
    params: { path: relPath },
    responseType: 'blob',
    timeout,
  });
  return URL.createObjectURL(blob as unknown as Blob);
}
