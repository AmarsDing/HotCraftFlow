/* ==========================================================================
   data.js — Mock 数据（数值取自录屏实测帧）
   ========================================================================== */

const DB = {
  /* 三大指数（底部状态栏） */
  indices: [
    { name: '道指', val: 46253.31, chg: 17.05, pct: 0.04 },
    { name: '标普', val: 6715.79, chg: -8.95, pct: -0.13 },
    { name: '纳指', val: 22670.08, chg: -148.38, pct: -0.65 },
  ],

  /* 自选列表（来自 [帧06] 放大） */
  watchlist: [
    { sym: 'NVDA', name: '英伟达', px: 186.25, chg: -0.22, market: 'US' },
    { sym: 'QQQ', name: '纳指100ETF', px: 598.42, chg: -0.65, market: 'US' },
    { sym: 'TSLA', name: '特斯拉', px: 439.31, chg: 1.31, market: 'US' },
    { sym: 'AAPL', name: '苹果', px: 255.46, chg: 1.37, market: 'US' },
    { sym: 'MSFT', name: '微软', px: 523.61, chg: 0.63, market: 'US' },
    { sym: 'AMZN', name: '亚马逊', px: 221.31, chg: -0.41, market: 'US' },
    { sym: 'META', name: 'Meta', px: 705.20, chg: 3.84, market: 'US' },
    { sym: 'GOOG', name: '谷歌A', px: 251.34, chg: 0.59, market: 'US' },
    { sym: 'AVGO', name: '博通', px: 337.85, chg: -0.41, market: 'US' },
    { sym: 'WMT', name: '沃尔玛', px: 103.43, chg: 1.06, market: 'US' },
    { sym: 'JPM', name: '摩根大通', px: 311.25, chg: 0.84, market: 'US' },
    { sym: 'V', name: 'Visa', px: 348.20, chg: 0.31, market: 'US' },
  ],

  /* 热力图（按行业分组；size = 市值权重；来自 [帧02/03]） */
  heatmap: [
    {
      name: 'Technology', items: [
        { name: 'NVDA', size: 4400, chg: -0.63, logoBg: '#76B900' },
        { name: 'AAPL', size: 3800, chg: 1.37, logoBg: '#333333' },
        { name: 'MSFT', size: 3900, chg: 0.63, logoBg: '#0067B8' },
        { name: 'AVGO', size: 1600, chg: -0.41, logoBg: '#CC0000' },
        { name: 'ORCL', size: 820, chg: 2.11, logoBg: '#C74634' },
        { name: 'AMD', size: 380, chg: -3.12, logoBg: '#ED1C24' },
        { name: 'MU', size: 210, chg: -4.05, logoBg: '#1E6FAE' },
        { name: 'CRM', size: 260, chg: 0.42, logoBg: '#00A1E0' },
      ]
    },
    {
      name: 'Communication', items: [
        { name: 'META', size: 1780, chg: 3.84, logoBg: '#0081FB' },
        { name: 'GOOG', size: 3050, chg: 0.59, logoBg: '#4285F4' },
        { name: 'NFLX', size: 520, chg: -1.20, logoBg: '#E50914' },
        { name: 'DIS', size: 200, chg: 0.28, logoBg: '#113CCF' },
      ]
    },
    {
      name: 'Consumer Cyclical', items: [
        { name: 'AMZN', size: 2350, chg: -0.41, logoBg: '#FF9900' },
        { name: 'TSLA', size: 1400, chg: 1.31, logoBg: '#CC0000' },
        { name: 'HD', size: 380, chg: 0.52, logoBg: '#F96302' },
        { name: 'MCD', size: 215, chg: -0.15, logoBg: '#FFC72C' },
        { name: 'NKE', size: 105, chg: -1.86, logoBg: '#111111' },
      ]
    },
    {
      name: 'Healthcare', items: [
        { name: 'LLY', size: 780, chg: 2.24, logoBg: '#D52B1E' },
        { name: 'UNH', size: 330, chg: -0.72, logoBg: '#002677' },
        { name: 'JNJ', size: 460, chg: 0.35, logoBg: '#D51900' },
        { name: 'ABBV', size: 390, chg: 1.02, logoBg: '#071D49' },
      ]
    },
    {
      name: 'Financial', items: [
        { name: 'JPM', size: 860, chg: 0.84, logoBg: '#005EB8' },
        { name: 'V', size: 660, chg: 0.31, logoBg: '#1A1F71' },
        { name: 'MA', size: 520, chg: -0.22, logoBg: '#EB001B' },
        { name: 'BAC', size: 360, chg: -1.05, logoBg: '#E31837' },
        { name: 'WFC', size: 260, chg: 0.63, logoBg: '#D71E28' },
      ]
    },
    {
      name: 'Consumer Defensive', items: [
        { name: 'WMT', size: 830, chg: 1.06, logoBg: '#0071CE' },
        { name: 'COST', size: 410, chg: 0.55, logoBg: '#E31837' },
        { name: 'PG', size: 380, chg: -0.31, logoBg: '#003DA5' },
        { name: 'KO', size: 300, chg: 0.12, logoBg: '#F40009' },
      ]
    },
    {
      name: 'Energy', items: [
        { name: 'XOM', size: 480, chg: -2.35, logoBg: '#D01F2E' },
        { name: 'CVX', size: 290, chg: -1.74, logoBg: '#00539B' },
        { name: 'COP', size: 140, chg: -3.02, logoBg: '#BE2B22' },
      ]
    },
  ],

  /* Monitor 实时流 */
  monitorSeeds: [
    { sym: 'NVDA', name: '英伟达', dir: 'up', v: '+0.84%' },
    { sym: 'TSLA', name: '特斯拉', dir: 'up', v: '+1.31%' },
    { sym: 'AMD', name: '超威半导体', dir: 'down', v: '-3.12%' },
    { sym: 'AAPL', name: '苹果', dir: 'up', v: '+1.37%' },
    { sym: 'META', name: 'Meta', dir: 'up', v: '+3.84%' },
    { sym: 'XOM', name: '埃克森美孚', dir: 'down', v: '-2.35%' },
    { sym: 'MSFT', name: '微软', dir: 'up', v: '+0.63%' },
    { sym: 'MU', name: '美光科技', dir: 'down', v: '-4.05%' },
  ],

  /* 行业涨幅榜（三列） */
  industries: [
    { name: '信息技术', chg: 1.24, items: [['软件', 1.86], ['半导体', 0.94], ['硬件', 0.61], ['IT服务', 0.33]] },
    { name: '通信服务', chg: 0.98, items: [['互动媒体', 1.45], ['娱乐', 0.62], ['电信', 0.21], ['媒体', -0.14]] },
    { name: '可选消费', chg: 0.42, items: [['汽车', 1.12], ['电商', 0.38], ['家居', 0.19], ['餐饮', -0.28]] },
  ],

  /* 报价列表（Quote List） */
  quoteList: [
    { sym: 'NVDA', name: '英伟达', px: 186.25, chg: -0.22, vol: '1.82亿', amt: '339.0亿' },
    { sym: 'TSLA', name: '特斯拉', px: 439.31, chg: 1.31, vol: '9240万', amt: '406.0亿' },
    { sym: 'AAPL', name: '苹果', px: 255.46, chg: 1.37, vol: '4520万', amt: '115.5亿' },
    { sym: 'META', name: 'Meta', px: 705.20, chg: 3.84, vol: '2210万', amt: '155.9亿' },
    { sym: 'AMD', name: '超威半导体', px: 218.50, chg: -3.12, vol: '6880万', amt: '150.3亿' },
    { sym: 'MSFT', name: '微软', px: 523.61, chg: 0.63, vol: '1890万', amt: '99.0亿' },
    { sym: 'LLY', name: '礼来', px: 812.40, chg: 2.24, vol: '320万', amt: '26.0亿' },
    { sym: 'AVGO', name: '博通', px: 337.85, chg: -0.41, vol: '1740万', amt: '58.8亿' },
  ],

  /* Time & Sales */
  timeSales: [
    ['22:11:03', 186.25, 1200, 'up'], ['22:11:02', 186.26, 500, 'up'], ['22:11:01', 186.24, 340, 'up'],
    ['22:10:58', 186.22, 800, 'down'], ['22:10:55', 186.22, 210, 'flat'], ['22:10:52', 186.18, 1500, 'down'],
    ['22:10:49', 186.20, 420, 'up'], ['22:10:44', 186.15, 980, 'down'], ['22:10:40', 186.16, 310, 'flat'],
  ],

  /* NVDA 财务（[帧07/08]） */
  fin: {
    headline: '英伟达 2026 财年 Q3 财报',
    summary: '营收 <b class="kpi">$570.1亿</b>，同比增长 <b class="up kpi">+62.5%</b>；净利润 <b class="kpi">$319.1亿</b>，毛利率 <b class="kpi">73.4%</b>。数据中心业务贡献主要增量，游戏业务环比持平。自由现金流 <b class="kpi">$266.4亿</b>，回购计划加码至 <b class="kpi">$600亿</b>。',
    sankey: {
      nodes: [
        { id: 'rev', label: '营收', val: 57.0, x: 0, color: '#5060F0', yoy: 62.5 },
        { id: 'gp', label: '毛利润', val: 41.8, x: 1, color: '#38C850', yoy: 58.1 },
        { id: 'cost', label: '营业成本', val: 15.2, x: 1, color: '#F09818', yoy: 71.9 },
        { id: 'ni', label: '净利润', val: 31.9, x: 2, color: '#38C850', yoy: 65.3 },
        { id: 'opex', label: '运营费用', val: 5.9, x: 2, color: '#F09818', yoy: 31.2 },
        { id: 'tax', label: '所得税及其他', val: 4.0, x: 2, color: '#F09818', yoy: 24.6 },
      ],
      links: [
        { s: 'rev', t: 'gp', val: 41.8, color: '#38C850' },
        { s: 'rev', t: 'cost', val: 15.2, color: '#F09818' },
        { s: 'gp', t: 'ni', val: 31.9, color: '#38C850' },
        { s: 'gp', t: 'opex', val: 5.9, color: '#F09818' },
        { s: 'gp', t: 'tax', val: 4.0, color: '#F09818' },
      ]
    },
    combo: [
      { x: '25Q1', bar: 44.1, line: 71.3 }, { x: '25Q2', bar: 46.7, line: 72.4 },
      { x: '25Q3', bar: 35.1, line: 70.1 }, { x: '25Q4', bar: 39.3, line: 73.0 },
      { x: '26Q1', bar: 44.1, line: 71.0 }, { x: '26Q2', bar: 46.7, line: 72.7 },
      { x: '26Q3', bar: 57.0, line: 73.4 },
    ],
    ratios: [
      ['营收', '$570.1亿', '+62.5%', 'up'], ['毛利润', '$418.3亿', '+58.1%', 'up'],
      ['净利润', '$319.1亿', '+65.3%', 'up'], ['毛利率', '73.4%', '+0.7pct', 'up'],
      ['净利率', '56.0%', '+1.0pct', 'up'], ['EPS', '$1.31', '+65.8%', 'up'],
      ['研发费用', '$46.2亿', '+38.9%', 'up'], ['自由现金流', '$266.4亿', '+52.1%', 'up'],
    ]
  },

  /* 期权链 */
  options: {
    under: 'NVDA', spot: 186.25,
    exps: ['2026-10-16', '2026-10-23', '2026-10-30', '2026-11-06', '2026-11-20', '2026-12-18', '2027-01-15'],
    chain: [
      { strike: 170, call: { bid: 17.85, ask: 18.10, vol: 3214, oi: 18200 }, put: { bid: 1.62, ask: 1.70, vol: 8902, oi: 24500 } },
      { strike: 175, call: { bid: 13.60, ask: 13.85, vol: 4521, oi: 21400 }, put: { bid: 2.58, ask: 2.68, vol: 12310, oi: 31200 } },
      { strike: 180, call: { bid: 9.95, ask: 10.15, vol: 8920, oi: 35800 }, put: { bid: 4.02, ask: 4.14, vol: 18540, oi: 42100 } },
      { strike: 185, call: { bid: 6.85, ask: 7.00, vol: 15230, oi: 48600 }, put: { bid: 5.88, ask: 6.02, vol: 16980, oi: 38900 }, atm: true },
      { strike: 190, call: { bid: 4.40, ask: 4.52, vol: 20450, oi: 56300 }, put: { bid: 8.44, ask: 8.60, vol: 9870, oi: 25800 } },
      { strike: 195, call: { bid: 2.62, ask: 2.72, vol: 24180, oi: 61200 }, put: { bid: 11.70, ask: 11.90, vol: 4320, oi: 15400 } },
      { strike: 200, call: { bid: 1.46, ask: 1.53, vol: 28940, oi: 74500 }, put: { bid: 15.55, ask: 15.80, vol: 2110, oi: 9800 } },
    ]
  },

  /* AI 建议问题 */
  aiSuggests: [
    { q: '英伟达最新财报的核心看点？', s: '财报速读' },
    { q: '对比 NVDA 与 AMD 的毛利率趋势', s: '同业对比' },
    { q: '帮我筛选高股息率的港股', s: '智能选股' },
    { q: '分析一下特斯拉期权链的持仓分布', s: '期权分析' },
  ],

  /* 量化代码（[帧17]） */
  quantCode: `# 双均线动量策略 · NVDA 日线回测
import pandas as pd
import numpy as np

@strategy(name="DualMomentum", market="US")
class DualMomentumStrategy:

    def on_init(self, ctx):
        ctx.fast = ctx.sma(close, window=20)
        ctx.slow = ctx.sma(close, window=60)
        ctx.rsi  = ctx.rsi(close, window=14)
        ctx.pos  = 0

    def on_bar(self, ctx, bar):
        if ctx.fast[-1] > ctx.slow[-1] and ctx.rsi[-1] < 70:
            if ctx.pos == 0:
                ctx.buy(symbol="NVDA", weight=0.95)
                ctx.pos = 1
        elif ctx.fast[-1] < ctx.slow[-1]:
            if ctx.pos == 1:
                ctx.sell(symbol="NVDA", all=True)
                ctx.pos = 0

    def on_stop(self, ctx):
        ctx.report(metrics=["sharpe", "maxdd", "winrate"])`,

  btLog: [
    ['22:14:01', 'INFO', '加载行情数据 NVDA 1D × 504 bars'],
    ['22:14:02', 'INFO', '初始化指标 SMA(20) / SMA(60) / RSI(14)'],
    ['22:14:02', 'INFO', '回测区间 2024-10-15 → 2026-10-16'],
    ['22:14:03', 'TRADE', 'BUY  NVDA × 520 @ 131.20'],
    ['22:14:03', 'TRADE', 'SELL NVDA × 520 @ 168.44  P&L +28.4%'],
    ['22:14:04', 'TRADE', 'BUY  NVDA × 480 @ 142.10'],
    ['22:14:04', 'TRADE', 'SELL NVDA × 480 @ 186.25  P&L +31.1%'],
    ['22:14:05', 'DONE', '回测完成 · 总收益 +52.8% · 夏普 1.84 · 最大回撤 -11.2% · 胜率 66.7%'],
  ],

  /* 命令面板主题项 */
  themes: [
    { id: 'default-dark', name: 'Default Dark', sw: '#18191D', tag: '默认' },
    { id: 'catppuccin-mocha', name: 'Catppuccin Mocha', sw: '#1E1E2E', tag: '社区' },
    { id: 'everforest-dark', name: 'Everforest Dark', sw: '#2D353B', tag: '社区' },
    { id: 'ayu-dark', name: 'Ayu Dark', sw: '#0F1419', tag: '社区' },
    { id: 'adventure', name: 'Adventure', sw: '#1A1625', tag: '社区' },
    { id: 'cn-red-up', name: 'A股配色 · 红涨绿跌', sw: '#D9342B', tag: '市场惯例' },
  ],
};

/* ---------- K 线生成器 ---------- */
function genCandles(seed = 42, n = 160, base = 120, vol = 0.016) {
  let s = seed;
  const rnd = () => (s = (s * 9301 + 49297) % 233280) / 233280;
  const out = [];
  let c = base;
  for (let i = 0; i < n; i++) {
    const o = c;
    const drift = (rnd() - 0.48) * 2 * vol * c + (i > n - 40 ? 0.004 * c : 0);
    c = Math.max(base * 0.4, o + drift);
    const h = Math.max(o, c) + rnd() * vol * c * 0.6;
    const l = Math.min(o, c) - rnd() * vol * c * 0.6;
    const mm = i % 5;
    out.push({
      t: `10:${String(30 + Math.floor(i / 6)).padStart(2, '0')}`,
      o: +o.toFixed(2), c: +c.toFixed(2), h: +h.toFixed(2), l: +l.toFixed(2),
      v: Math.round(400 + rnd() * 2400),
    });
  }
  return out;
}
function genSpark(seed, n = 48, base = 100) {
  let s = seed;
  const rnd = () => (s = (s * 9301 + 49297) % 233280) / 233280;
  const arr = [base];
  for (let i = 1; i < n; i++) arr.push(arr[i - 1] + (rnd() - 0.47) * 1.8);
  return arr;
}
