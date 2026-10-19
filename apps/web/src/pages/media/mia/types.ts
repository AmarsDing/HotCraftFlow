/** ③ MIA 页面共享类型与工具（后端 DTO 直接复用 services 导出类型）。 */

export type {
  EventCardItem,
  ImpactCardListItem,
  GetImpactCardResponse,
  ImpactCard,
  AffectedItem,
  ImpactFactors,
  Verified,
  CalibrationReport,
  CalibrationBucket,
  TransmissionRuleItem,
  GetQuoteResponse,
  OhlcvBar,
  EntityAssetMappingItem,
  MiaImportCsvResponse,
  NarrativeCoinMappingItem,
} from '../../../services/index';

/** 默认免责声明的后端契约文案（pkg/mediacontract.ImpactDisclaimer；前端仅作占位兜底，真值以卡片字段为准）。 */
export const MIA_IMPACT_DISCLAIMER_FALLBACK = '研究/监测辅助，非投资建议；模型输出须人工复核。';

/** 错误消息抽取（kratos status error → message 字段；兜底层级化）。 */
export function errMessage(e: unknown): string {
  if (e == null) return '未知错误';
  if (typeof e === 'string') return e;
  const anyErr = e as Record<string, unknown>;
  if (typeof anyErr.message === 'string' && anyErr.message) return anyErr.message;
  if (typeof anyErr.reason === 'string' && anyErr.reason) return anyErr.reason;
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

/** 方向徽标（bullish/bearish/neutral；unknown 不猜——置灰"无方向"）。 */
export function directionMeta(d?: string): { color: string; icon: string; label: string } {
  switch (d) {
    case 'bullish':
      return { color: 'negative', icon: 'trending_up', label: '看涨' }; // 红涨（A股习惯）
    case 'bearish':
      return { color: 'positive', icon: 'trending_down', label: '看跌' }; // 绿跌
    case 'neutral':
      return { color: 'grey-7', icon: 'remove', label: '中性' };
    default:
      // 🔴 unknown / engine=none：不产出方向结论（不猜）
      return { color: 'grey-5', icon: 'help_outline', label: '无方向（不猜）' };
  }
}

/** 情感标签徽标（sentiment.unknown 显式可视，绝不伪装成中性）。 */
export function sentimentMeta(label?: string): { color: string; label: string } {
  switch (label) {
    case 'positive':
      return { color: 'negative', label: '积极' };
    case 'negative':
      return { color: 'positive', label: '消极' };
    case 'neutral':
      return { color: 'grey-7', label: '中性' };
    default:
      return { color: 'grey-5', label: '未知（不猜）' };
  }
}

/** 情感引擎展示名（契约枚举 EngineFinBERT/EngineLLM/EngineNone）。 */
export function engineLabel(engine?: string): string {
  switch (engine) {
    case 'finbert':
      return 'FinBERT';
    case 'llm':
      return 'LLM';
    case 'none':
      return '引擎全挂';
    default:
      return engine || '-';
  }
}

/** 资产类别中文标签。 */
export function assetClassLabel(c?: string): string {
  switch (c) {
    case 'equity':
      return '股票';
    case 'index':
      return '指数';
    case 'fx':
      return '外汇';
    case 'commodity':
      return '商品';
    case 'crypto':
      return '加密';
    default:
      return c || '-';
  }
}

/** 实体类型中文标签。 */
export function entityTypeLabel(t?: string): string {
  switch (t) {
    case 'person':
      return '人物';
    case 'org':
      return '机构';
    case 'policy':
      return '政策';
    case 'tech':
      return '技术';
    case 'place':
      return '地区';
    case 'topic':
      return '主题';
    default:
      return t || '-';
  }
}

/** TTL 剩余（expires_at - now；过期显式标出，供"默认隐藏过期卡"认知一致）。 */
export function ttlRemaining(expiresAt?: string): { text: string; expired: boolean } {
  if (!expiresAt) return { text: '-', expired: false };
  const ms = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(ms)) return { text: '-', expired: false };
  if (ms <= 0) return { text: '已过期', expired: true };
  const h = Math.floor(ms / 3600000);
  const m = Math.floor((ms % 3600000) / 60000);
  if (h >= 1) return { text: `${h}h${m}m`, expired: false };
  return { text: `${Math.max(m, 1)}m`, expired: false };
}

/** 分数展示（0–100 影响分；undefined 占位 -）。 */
export function fmtScore(s?: number | null): string {
  if (s == null) return '-';
  return s.toFixed(0);
}

/** 覆盖率：校准桶样本数 <30 → 不具统计意义（详细设计 §8 固定提示纪律）。 */
export const CALIBRATION_MIN_SAMPLES = 30;
