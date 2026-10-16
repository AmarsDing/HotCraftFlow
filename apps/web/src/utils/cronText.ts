// 5 字段标准 Cron（分 时 日 月 周）→ 人类可读描述。
// 用于报表定时任务：表格 cron 列辅助小字 + 表单输入实时提示，降低 cron 阅读成本。
// 仅覆盖企业报表高频模式（每天/每周/每月定点、每 N 分钟/小时间隔），
// 无法识别的复杂表达式回退空串（调用方继续显示原始表达式，不误导）。

export interface CronT {
  (key: string, params?: Record<string, unknown>): string;
}

function pad2(n: number): string {
  return String(n).padStart(2, '0');
}

/**
 * 描述 cron 表达式；无法识别返回 ''。
 * @param expr 5 字段 cron（分 时 日 月 周）
 * @param t vue-i18n 的 t 函数
 */
export function describeCron(expr: string, t: CronT): string {
  const parts = (expr || '').trim().split(/\s+/);
  if (parts.length !== 5) return '';
  const [min, hour, dom, mon, dow] = parts;

  // 每 N 分钟：*/n * * * *
  const everyMin = /^\*\/(\d{1,2})$/.exec(min);
  if (everyMin && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return t('report.cronEveryMinutes', { n: Number(everyMin[1]) });
  }
  // 每 N 小时（整点 m 分）：m */n * * *
  const everyHour = /^\*\/(\d{1,2})$/.exec(hour);
  if (/^\d{1,2}$/.test(min) && everyHour && dom === '*' && mon === '*' && dow === '*') {
    return t('report.cronEveryHours', { n: Number(everyHour[1]) });
  }
  // 每小时定点：m * * * *
  if (/^\d{1,2}$/.test(min) && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return t('report.cronHourlyAt', { minute: pad2(Number(min)) });
  }

  // 定点模式：分、时均为数字
  if (!/^\d{1,2}$/.test(min) || !/^\d{1,2}$/.test(hour)) return '';
  const time = `${pad2(Number(hour))}:${pad2(Number(min))}`;

  // 每天：m h * * *
  if (dom === '*' && mon === '*' && dow === '*') {
    return t('report.cronDailyAt', { time });
  }
  // 每周：m h * * d（0/7=周日）
  if (dom === '*' && mon === '*' && /^\d$/.test(dow)) {
    const d = Number(dow) % 7;
    return t('report.cronWeeklyAt', { dow: t(`report.cronDow${d}`), time });
  }
  // 每月：m h D * *
  if (/^\d{1,2}$/.test(dom) && mon === '*' && dow === '*') {
    return t('report.cronMonthlyAt', { day: Number(dom), time });
  }
  return '';
}

/**
 * 计算未来 N 次执行时间；无法识别的表达式返回 []。
 * 与 describeCron 同口径（仅覆盖企业报表高频模式），本地时区。
 */
export function nextRunTimes(expr: string, count = 3, from: Date = new Date()): Date[] {
  const parts = (expr || '').trim().split(/\s+/);
  if (parts.length !== 5) return [];
  const [min, hour, dom, , dow] = parts;
  const out: Date[] = [];
  const push = (d: Date) => {
    if (d.getTime() > from.getTime() && out.length < count) out.push(d);
  };

  // 每 N 分钟：*/n * * * *（对齐分钟边界，秒清零）
  const everyMin = /^\*\/(\d{1,2})$/.exec(min);
  if (everyMin && hour === '*' && dom === '*' && dow === '*') {
    const step = Number(everyMin[1]);
    if (step < 1 || step > 59) return [];
    const base = new Date(from);
    base.setSeconds(0, 0);
    const nextMin = (Math.floor(base.getMinutes() / step) + 1) * step;
    const cursor = new Date(base);
    cursor.setMinutes(nextMin, 0, 0);
    while (out.length < count) {
      push(new Date(cursor));
      cursor.setMinutes(cursor.getMinutes() + step);
    }
    return out;
  }

  // 每 N 小时（整点 m 分）：m */n * * *
  const everyHour = /^\*\/(\d{1,2})$/.exec(hour);
  if (/^\d{1,2}$/.test(min) && everyHour && dom === '*' && dow === '*') {
    const step = Number(everyHour[1]);
    const m = Number(min);
    if (step < 1 || step > 23) return [];
    const cursor = new Date(from);
    cursor.setMinutes(m, 0, 0);
    if (cursor.getTime() <= from.getTime()) cursor.setHours(cursor.getHours() + step);
    while (out.length < count) {
      push(new Date(cursor));
      cursor.setHours(cursor.getHours() + step);
    }
    return out;
  }

  // 每小时定点：m * * * *
  if (/^\d{1,2}$/.test(min) && hour === '*' && dom === '*' && dow === '*') {
    const m = Number(min);
    if (m > 59) return [];
    const cursor = new Date(from);
    cursor.setMinutes(m, 0, 0);
    if (cursor.getTime() <= from.getTime()) cursor.setHours(cursor.getHours() + 1);
    while (out.length < count) {
      push(new Date(cursor));
      cursor.setHours(cursor.getHours() + 1);
    }
    return out;
  }

  // 定点模式：分、时均为数字
  if (!/^\d{1,2}$/.test(min) || !/^\d{1,2}$/.test(hour)) return [];
  const m = Number(min);
  const h = Number(hour);

  // 每天：m h * * *
  if (dom === '*' && dow === '*') {
    const cursor = new Date(from);
    cursor.setHours(h, m, 0, 0);
    if (cursor.getTime() <= from.getTime()) cursor.setDate(cursor.getDate() + 1);
    while (out.length < count) {
      push(new Date(cursor));
      cursor.setDate(cursor.getDate() + 1);
    }
    return out;
  }

  // 每周：m h * * d（0/7=周日）
  if (dom === '*' && /^\d$/.test(dow)) {
    const target = Number(dow) % 7;
    const cursor = new Date(from);
    cursor.setHours(h, m, 0, 0);
    let delta = (target - cursor.getDay() + 7) % 7;
    if (delta === 0 && cursor.getTime() <= from.getTime()) delta = 7;
    cursor.setDate(cursor.getDate() + delta);
    while (out.length < count) {
      push(new Date(cursor));
      cursor.setDate(cursor.getDate() + 7);
    }
    return out;
  }

  // 每月：m h D * *
  if (/^\d{1,2}$/.test(dom) && dow === '*') {
    const day = Number(dom);
    let y = from.getFullYear();
    let mo = from.getMonth();
    // 逐月显式推进 year/month 并全新构造 Date：避免"2 月 31 日"进位污染游标日序
    let guard = 0;
    while (out.length < count && guard < 36) {
      const d = new Date(y, mo, day, h, m, 0, 0);
      if (d.getDate() === day && d.getTime() > from.getTime()) out.push(d);
      mo += 1;
      if (mo > 11) {
        mo = 0;
        y += 1;
      }
      guard += 1;
    }
    return out;
  }
  return [];
}

/** 紧凑执行时间：当天 HH:mm，否则 MM-DD HH:mm */
export function formatRunTime(d: Date, now: Date = new Date()): string {
  const sameDay = d.toDateString() === now.toDateString();
  const hm = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
  return sameDay ? hm : `${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${hm}`;
}
