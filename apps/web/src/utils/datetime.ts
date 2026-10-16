// 日期时间显示工具

// 时间值 → Date：RFC3339/ISO 字符串或 epoch 数字（F1.3 时区统一：DataFrame time 列输出
// epoch 秒；>1e12 视为毫秒，兼容既有推送）。无法解析返回 null。
export function toDate(v: unknown): Date | null {
  if (v == null || v === '') return null;
  if (typeof v === 'number') {
    if (!Number.isFinite(v) || v <= 0) return null;
    const d = new Date(v > 1e12 ? v : v * 1000);
    return Number.isNaN(d.getTime()) ? null : d;
  }
  if (typeof v === 'string') {
    const d = new Date(v);
    return Number.isNaN(d.getTime()) ? null : d;
  }
  return null;
}

// RFC3339/ISO 时间戳或 epoch 秒 → 本地 "YYYY-MM-DD HH:mm:ss"；空值显示 '-'，无法解析时原样返回
export function formatDateTime(v?: string | number | null): string {
  if (v == null || v === '') return '-';
  const d = toDate(v);
  if (!d) return String(v);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

// 紧凑时间（卡片标签等窄位展示）：当年 "MM-DD HH:mm"，跨年 "YY-MM-DD"；空值 '-'
// 完整时间与语义说明由调用方以 tooltip 提供
export function formatCompactTime(v?: string | null): string {
  if (!v) return '-';
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return v;
  const pad = (n: number) => String(n).padStart(2, '0');
  if (d.getFullYear() === new Date().getFullYear()) {
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }
  return `${String(d.getFullYear()).slice(2)}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// "YYYY-MM" 月份 → [月初, 月末] 日期串；月末按当月实际天数计算，
// 禁止硬编码 31——PG date 字面量校验会拒绝 "2026-09-31" 抛 22008（考勤列表 500 根因）
export function monthRange(month: string): [string, string] {
  const [y, m] = month.split('-').map(Number);
  const last = new Date(y, m, 0).getDate();
  const pad = (n: number) => String(n).padStart(2, '0');
  return [`${y}-${pad(m)}-01`, `${y}-${pad(m)}-${pad(last)}`];
}

// 匹配 ISO/RFC3339 时间戳字符串（如 2026-08-09T07:11:58.098826Z），供表格默认单元格自动格式化
const ISO_DATETIME_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}/;
export function isIsoDateTime(v: unknown): v is string {
  return typeof v === 'string' && ISO_DATETIME_RE.test(v);
}

// 纯日期字段（ent time.Time 存 UTC 零点，如 2026-08-07T00:00:00Z）→ "YYYY-MM-DD"
// 直接截 UTC 前缀，不做时区换算——与编辑表单 slice(0,10) 回填语义一致，避免 +8 时区日期漂移
export function formatDate(v?: string | null): string {
  if (!v) return '-';
  return v.slice(0, 10) || '-';
}

// 秒数时长人性化（统计类表格展示用，语言中性紧凑格式）：86400 → "1d"，3661 → "1h 1m"，45 → "45s"；空值 '-'
export function formatDuration(seconds?: number | null): string {
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '-';
  const s = Math.round(seconds);
  if (s < 60) return `${s}s`;
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return h > 0 ? `${d}d ${h}h` : `${d}d`;
  if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`;
  return s % 60 > 0 ? `${m}m ${s % 60}s` : `${m}m`;
}
