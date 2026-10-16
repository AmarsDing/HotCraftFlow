// 轻量五段式 cron 解析与预览（无第三方依赖）。
// 支持语法：*、*/n、a、a-b、a-b/n、a/n、逗号列表、英文月/周名（JAN-DEC、SUN-SAT，大小写不敏感）。
// 语义对齐 robfig/cron 标准解析器：dom 与 dow 同时受限时为 OR 关系。

export interface CronField {
  values: number[];
  wildcard: boolean;
}

export interface CronSchedule {
  minute: CronField;
  hour: CronField;
  dom: CronField;
  month: CronField;
  dow: CronField;
}

const MONTH_NAMES: Record<string, number> = {
  JAN: 1, FEB: 2, MAR: 3, APR: 4, MAY: 5, JUN: 6,
  JUL: 7, AUG: 8, SEP: 9, OCT: 10, NOV: 11, DEC: 12,
};
const DOW_NAMES: Record<string, number> = {
  SUN: 0, MON: 1, TUE: 2, WED: 3, THU: 4, FRI: 5, SAT: 6,
};

function parseField(
  field: string,
  min: number,
  max: number,
  names?: Record<string, number>,
): CronField | null {
  const src = field.trim().toUpperCase();
  if (!src) return null;

  const resolve = (tok: string): number => {
    if (names && names[tok] != null) return names[tok];
    const n = Number(tok);
    return Number.isInteger(n) ? n : NaN;
  };

  const values = new Set<number>();
  let wildcard = false;

  for (const part of src.split(',')) {
    const p = part.trim();
    if (!p) return null;

    let range = p;
    let step = 1;
    const slashIdx = p.indexOf('/');
    if (slashIdx >= 0) {
      range = p.slice(0, slashIdx);
      step = Number(p.slice(slashIdx + 1));
      if (!Number.isInteger(step) || step <= 0) return null;
    }

    let lo: number;
    let hi: number;
    if (range === '*' || range === '') {
      lo = min;
      hi = max;
      if (slashIdx < 0) wildcard = true;
    } else if (range.includes('-')) {
      const [a, b] = range.split('-', 2);
      lo = resolve(a);
      hi = resolve(b);
      if (Number.isNaN(lo) || Number.isNaN(hi) || lo > hi) return null;
    } else {
      lo = resolve(range);
      if (Number.isNaN(lo)) return null;
      // a/n 等价 a-max/n；单值无步长时仅取该值
      hi = slashIdx >= 0 ? max : lo;
    }

    if (lo < min || hi > max) return null;
    for (let v = lo; v <= hi; v += step) values.add(v);
  }

  if (values.size === 0) return null;
  return { values: [...values].sort((a, b) => a - b), wildcard };
}

/** 解析五段式 cron；非法返回 null。 */
export function parseCron(spec: string): CronSchedule | null {
  const fields = (spec || '').trim().split(/\s+/);
  if (fields.length !== 5) return null;
  const minute = parseField(fields[0], 0, 59);
  const hour = parseField(fields[1], 0, 23);
  const dom = parseField(fields[2], 1, 31);
  const month = parseField(fields[3], 1, 12, MONTH_NAMES);
  const dow = parseField(fields[4], 0, 6, DOW_NAMES);
  if (!minute || !hour || !dom || !month || !dow) return null;
  return { minute, hour, dom, month, dow };
}

/** 计算从 from 起的下 count 次执行时间；解析失败或 4 年内无匹配返回已收集的部分（可能为空）。 */
export function nextRuns(spec: string, count = 3, from: Date = new Date()): Date[] {
  const cron = parseCron(spec);
  if (!cron || count <= 0) return [];

  const result: Date[] = [];
  const start = new Date(from.getTime());
  start.setSeconds(0, 0);
  start.setMinutes(start.getMinutes() + 1); // 从下一分钟起

  const domAny = cron.dom.wildcard;
  const dowAny = cron.dow.wildcard;

  const MAX_DAYS = 366 * 4;
  const cursor = new Date(start.getFullYear(), start.getMonth(), start.getDate());

  for (let d = 0; d < MAX_DAYS && result.length < count; d++) {
    const month = cursor.getMonth() + 1;
    if (!cron.month.values.includes(month)) {
      cursor.setDate(cursor.getDate() + 1);
      continue;
    }
    const day = cursor.getDate();
    const weekday = cursor.getDay();
    const domHit = cron.dom.values.includes(day);
    const dowHit = cron.dow.values.includes(weekday);
    // Vixie/robfig 语义：两者同时受限取 OR；一方通配取另一方
    const dayMatch = domAny && dowAny ? true : domAny ? dowHit : dowAny ? domHit : domHit || dowHit;
    if (dayMatch) {
      for (const h of cron.hour.values) {
        for (const m of cron.minute.values) {
          const t = new Date(cursor.getFullYear(), cursor.getMonth(), day, h, m);
          if (t >= start) {
            result.push(t);
            if (result.length >= count) return result;
          }
        }
      }
    }
    cursor.setDate(cursor.getDate() + 1);
  }
  return result;
}

const pad2 = (n: number) => String(n).padStart(2, '0');

/** 人性化描述（尽力而为：命中常见模式返回自然语言，否则返回空串由调用方回退显示原文）。 */
export function describeCron(spec: string, locale: 'zh' | 'en' = 'zh'): string {
  const cron = parseCron(spec);
  if (!cron) return '';
  const zh = locale === 'zh';
  const { minute, hour, dom, month, dow } = cron;
  const single = (f: CronField) => (f.values.length === 1 ? f.values[0] : null);

  const mSingle = single(minute);
  const hSingle = single(hour);
  const timeText =
    mSingle != null && hSingle != null ? `${pad2(hSingle)}:${pad2(mSingle)}` : '';

  // 每分钟
  if (minute.wildcard && hour.wildcard && dom.wildcard && month.wildcard && dow.wildcard) {
    return zh ? '每分钟' : 'Every minute';
  }
  // */n 分钟
  const mStep = stepOf(spec, 0);
  if (mStep > 1 && hour.wildcard && dom.wildcard && month.wildcard && dow.wildcard) {
    return zh ? `每 ${mStep} 分钟` : `Every ${mStep} minutes`;
  }
  // 每小时（指定分）
  if (mSingle != null && hour.wildcard && dom.wildcard && month.wildcard && dow.wildcard) {
    return zh ? `每小时第 ${mSingle} 分` : `At minute ${mSingle} every hour`;
  }
  // */n 小时
  const hStep = stepOf(spec, 1);
  if (mSingle != null && hStep > 1 && dom.wildcard && month.wildcard && dow.wildcard) {
    return zh ? `每 ${hStep} 小时（第 ${mSingle} 分）` : `Every ${hStep} hours at minute ${mSingle}`;
  }

  const weekNames = zh
    ? ['日', '一', '二', '三', '四', '五', '六']
    : ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

  // 每天 HH:mm
  if (timeText && dom.wildcard && month.wildcard && dow.wildcard) {
    return zh ? `每天 ${timeText}` : `Daily at ${timeText}`;
  }
  // 每周
  if (timeText && dom.wildcard && month.wildcard && !dow.wildcard) {
    const days = dow.values.map((d) => weekNames[d]).join(zh ? '、' : ', ');
    return zh ? `每周${days} ${timeText}` : `Weekly on ${days} at ${timeText}`;
  }
  // 每月
  if (timeText && !dom.wildcard && month.wildcard && dow.wildcard) {
    const days = dom.values.join(zh ? '、' : ', ');
    return zh ? `每月 ${days} 日 ${timeText}` : `Monthly on day ${days} at ${timeText}`;
  }
  // 每年某月
  if (timeText && !dom.wildcard && !month.wildcard && dow.wildcard) {
    const months = month.values.join(zh ? '、' : ', ');
    const days = dom.values.join(zh ? '、' : ', ');
    return zh ? `每年 ${months} 月 ${days} 日 ${timeText}` : `Yearly on month ${months} day ${days} at ${timeText}`;
  }
  return '';
}

/** 提取某字段的步长（仅含 "/n" 形式时返回 n，否则返回 1）。 */
function stepOf(spec: string, fieldIdx: number): number {
  const fields = (spec || '').trim().split(/\s+/);
  const f = fields[fieldIdx] || '';
  const slashIdx = f.indexOf('/');
  if (slashIdx < 0) return 1;
  const n = Number(f.slice(slashIdx + 1));
  return Number.isInteger(n) && n > 0 ? n : 1;
}
