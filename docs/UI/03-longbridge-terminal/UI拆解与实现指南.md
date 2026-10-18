# 金融行情 Web 终端 UI 拆解与前端实现指南

> 本文档基于一段 136 秒的实机录屏（1320×2868、60fps）逐帧分析整理而成。
> 录屏内容为**桌面端 Web 应用**（投放在手机上显示的桌面窗口，窗口实测约 1135×690，16:10 比例，带 macOS 风格交通灯与浏览器导航控件）。
> 参考产品为 **Longbridge（长桥）桌面端 Web 交易终端**：一个集「市场总览 / 热力图 / 自选 / K 线 / 财务 / 期权 / AI 助手 / 量化回测」于一体的深色专业终端。
>
> **文档目标**：不只看懂，更要能复刻。所有配色、尺寸、动效参数均标注实测值或近似值，并给出前端技术选型、组件拆解、核心算法与代码骨架，可直接作为 AI 编码的规格说明书。
>
> **配套材料**：`reference-frames/` 目录下有 29 张从录屏中抽取的原分辨率关键帧与局部放大图（编号 01–19 为整屏，z01–z10 为局部细节），正文中以 `[帧NN]`、`[放大zNN]` 引用。

---

## 目录

1. [总体特征与信息架构](#1-总体特征与信息架构)
2. [设计令牌 Design Tokens](#2-设计令牌-design-tokens)
3. [应用骨架布局](#3-应用骨架布局)
4. [视图逐个拆解](#4-视图逐个拆解)
5. [动效系统](#5-动效系统)
6. [图表与可视化实现](#6-图表与可视化实现)
7. [数据、实时与性能](#7-数据实时与性能)
8. [工程结构与分阶段路线](#8-工程结构与分阶段路线)
9. [验收清单](#9-验收清单)

---

## 1. 总体特征与信息架构

### 1.1 产品形态判断

| 维度 | 结论 |
|---|---|
| 载体 | 桌面 Web 应用（浏览器窗口内运行，无系统标题栏遮挡，自绘顶部窗口栏） |
| 市场 | 多市场：US / HK / CN / SG（顶部区域 Tab 切换），美股为主 |
| 用户 | 专业/半专业投资者（信息密度极高，一屏 ≈ 8–12 个信息模块） |
| 语言 | 英文为主，混排中文（AI 会话历史、部分文案），需内置 i18n |
| 账户态 | 演示账户 `Demo A/C(1757)`，支持模拟交易（`Simulated Buy`） |
| AI | 深度集成：右侧抽屉 + 全屏两种形态，可调用站内工具（选股器）并以富文本回执 |

### 1.2 信息架构（导航地图）

```
顶部窗口栏（自绘）
├─ 窗口控制行：交通灯 / 侧栏开关 / 历史 / 前进后退 ｜ 全局搜索「Press / to search」｜ 账户菜单 ｜ ✚AI 入口
├─ 品牌行：Logo + 市场 Tab（US HK CN SG）
└─ 视图 Tab 行：随市场而变
   US: Market | Sector | Heatmap
   个股: Chart | Options | Financial | Forecast | Valuation | Shareholder | Wiki

左侧图标栏（约 40px，图标导航）
├─ 图表(Market) / ❤自选 / 🧭发现 / 🕘历史 / ▦网格 / </> 量化(带角标×6) / ⚗ 筛选
└─ 底部：头像（设置/主题/登出入口）

主内容区（随视图切换）
├─ Market 市场总览：指数卡 + Monitor 监控 + Industry 行业列表 + Quote List
├─ Heatmap 热力图：Treemap 全屏 + 筛选胶囊 + 阈值色阶 chips
├─ Watchlist 自选网格：迷你 K 线卡片宫格 + 右栏个股详情
├─ 个股详情：K 线主图 + 左自选列 + 右报价/订单簿/成交明细 + 底部交易面板
├─ Financial 财务：AI 财报解读 + 收入结构桑基图 + 组合图 + 指标表
├─ Options 期权：到期日条 + Call/Put 压力条 + 双边期权链 + P&L 损益图
├─ Quant 量化：DSL 代码编辑器 + 回测日志 + 回测结果/历史
└─ LongbridgeAI：右侧抽屉 / 全屏（历史会话 + 流式对话 + 富文本卡片）

底部状态栏（约 30px）
└─ 头像 + 市场切换 + DOW / NASDAQ / S&P 500 实时值 + 时间戳 + 连接状态点
```

### 1.3 设计语言关键词

**深色、高密度、扁平、无衬线、等宽数字、青/玫红语义色、圆角胶囊、微动效**。
整体气质是「Bloomberg 终端的现代化 Web 版」：去掉边框喧嚣，用 1px 分隔线 + 色块语义 + 留白分层；交互反馈集中在 hover 卡片、十字光标、数字闪烁三类。

---

## 2. 设计令牌 Design Tokens

> 颜色值说明：标注 `实测` 的来自录屏像素采样；标注 `≈` 的为目视近似（视频压缩 + 抗锯齿导致边缘色偏移）。实现时建议以 `实测` 为准，`≈` 作为兜底。

### 2.1 色彩令牌

#### 中性色（骨架）

```css
:root {
  /* 背景 */
  --bg-app:        #18191D;   /* 实测：应用主背景/顶栏/底栏 */
  --bg-panel:      #1E2026;   /* ≈ 卡片/面板（比 app 背景亮约 4%） */
  --bg-panel-2:    #22242A;   /* ≈ 悬浮卡片/下拉/Toast 背景 */
  --bg-input:      #141517;   /* ≈ 输入框、代码编辑器背景 */
  --bg-hover:      rgba(255,255,255,.04);   /* 行 hover */
  --bg-active:     rgba(255,255,255,.07);   /* 行选中/按下 */
  --bg-rail:       #18191D;   /* 左图标栏同 app 背景，仅右侧 1px 分隔线 */

  /* 边框/分隔线 */
  --border:        #26282E;   /* ≈ 常规边框 */
  --border-soft:   rgba(255,255,255,.06);   /* 分隔线 */
  --border-strong: #34363D;   /* ≈ 输入框描边 */

  /* 文本 */
  --text-primary:   #E6E8EB;  /* ≈ 标题/价格（实测亮字 #B4B4C0~#E6E8EB） */
  --text-secondary: #9AA0A6;  /* ≈ 次要信息（实测 #9C9CA8） */
  --text-tertiary:  #62666E;  /* ≈ 弱标签（实测 #60606C~#6C6C78） */
  --text-disabled:  #4A4D54;
  --text-inverse:   #0D0E10;  /* 深色按钮上的字 */
}
```

#### 语义色（涨跌）——本应用采用「青涨 / 玫红跌」（欧美惯例）

```css
:root {
  /* 涨（青绿系） */
  --up-text:    #35C8A8;   /* ≈ 涨跌文字（+1.23% 实测偏亮青） */
  --up-btn:     #0CA890;   /* 实测：Buy 按钮 / 活动胶囊填充 */
  --up-deep:    #0C6C60;   /* 实测：热力图深青档 / 分组标签条 #0A826E */
  --up-mid:     #0C846C;   /* 实测 */
  --up-bright:  #0DB397;   /* 实测：大涨幅档 */
  --up-glow:    #2BC4A8;   /* ≈ 最亮档 / 图表涨蜡烛高光 */

  /* 跌（玫红系） */
  --down-text:  #F0568C;   /* ≈ 涨跌文字（-0.22% 实测偏亮玫红） */
  --down-btn:   #C42B6B;   /* ≈ Sell 按钮填充 */
  --down-deep:  #78183C;   /* 实测：热力图深玫红档（NVDA -0.63% = #781942） */
  --down-mid:   #901848;   /* 实测 */
  --down-bright:#B11955;   /* 实测：中大跌幅档 */
  --down-glow:  #D7165E;   /* 实测：大跌幅档 / <-3% 阈值 chip */

  /* 零值/中性 */
  --flat:       #6B7280;   /* ≈ 0% 档灰 */

  /* 蓝色（最新价线/标签，独立于涨跌语义） */
  --price-line: #3E7BFA;   /* ≈ K 线最新价虚线 + 价格标签底色 */
}
```

**重要约定**：本应用中「涨跌语义色」与「交互主色」是两套——
- 涨跌色只表达**数据方向**（价格、涨跌幅、K 线、热力图、订单簿、P&L）；
- **交互主色是青绿 `#0CA890`**（Buy 按钮、活动 Tab 胶囊、AI 入口 ✚、链接）；
- 跌方向的操作（Sell 按钮）才用玫红填充。

> 若面向 A 股用户，只需把 `--up-*` / `--down-*` 两组变量互换（红涨绿跌），骨架不变——这也是把语义色抽成 CSS 变量的原因。

#### 功能色（图表/状态）

```css
:root {
  --c-sankey-income:  #5060F0;  /* 实测：收入节点/流（蓝） */
  --c-sankey-profit:  #38C850;  /* 实测：利润节点/流（绿） */
  --c-sankey-expense: #F09818;  /* 实测：成本费用节点/流（橙） */
  --c-chart-bar:      #3B82F6;  /* ≈ 组合图柱（蓝） */
  --c-chart-line:     #F59E0B;  /* ≈ 组合图折线（琥珀，QoQ 线） */
  --c-ai-gradient-a:  #EC4899;  /* AI 标题渐变：粉 */
  --c-ai-gradient-b:  #F59E0B;  /* AI 标题渐变：橙 */
  --c-ai-gradient-c:  #2DD4BF;  /* AI 标题渐变：青 */
  --c-success:        #22C55E;  /* Toast「Filled」徽章 */
}
```

### 2.2 热力图色阶（Treemap 专用）

实测规律：**按 |涨跌幅| 分档，绝对值越大颜色越亮越饱和**；正负各自一条色带；0% 附近为灰。顶部筛选条把这 9 档直接做成了可点击的阈值 chip `[帧02]`：

```
跌：  <-3%    -3%    -2%    -1%   |  0%  |  +1%   +2%   +3%   >+3%   涨
     #D7165E #C2265B #A82050 #901848  #6B7280  #0C7A6A #0C9C82 #0DB397 #2BC4A8
     (亮玫红 → 深玫红)                  (灰)    (深青 → 亮青)
```

实测样本对照（S&P 500 热力图 `[帧02][放大z03]`）：

| 标的 | 涨跌幅 | 采样色 | 档位 |
|---|---|---|---|
| NVDA | -0.63% | `#781942` | 负 1 档（深） |
| AVGO | -0.41% | `#771A42` | 负 1 档（深） |
| AAPL | +1.37% | `#10886E` | 正 2 档 |
| META | +3.64% | `#0DB397` | 正 4 档（亮） |
| 小市值 | -2~-4% | `#B11955`~`#D7165E` | 负 3–4 档（亮） |

实现：一个纯函数 `heatColor(pct)`，9 个离散档（或档内线性插值），**不要**用连续渐变——离散档才能和筛选 chip 一一对应。

```ts
const SCALE = [
  { max: -3,   color: '#D7165E' }, { max: -2, color: '#A82050' },
  { max: -1,   color: '#901848' }, { max: -0.001, color: '#78183C' },
  { max: 0.001, color: '#6B7280' },
  { min: 0.001, color: '#0C7A6A' }, { min: 1, color: '#0C9C82' },
  { min: 2, color: '#0DB397' },     { min: 3, color: '#2BC4A8' },
];
export const heatColor = (pct: number) =>
  SCALE.find(s => (s.max !== undefined ? pct <= s.max : pct >= s.min!)!.color ?? '#6B7280';
```

### 2.3 字体排印

```css
:root {
  --font-sans: "Inter", "HarmonyOS Sans SC", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif;
  --font-mono: "JetBrains Mono", "SF Mono", "Cascadia Code", Consolas, monospace;

  /* 字号阶（录屏实测） */
  --fs-micro:  10px;  /* 角标、轴标签、时间戳 */
  --fs-caption:11px;  /* 分组标签、chip、表头 */
  --fs-body:   12px;  /* 表格主体、列表、正文（终端主力字号） */
  --fs-body-lg:13px;  /* 表单标签、卡片标题 */
  --fs-title:  15px;  /* 视图标题 */
  --fs-price:  22px;  /* 个股大价格 */
  --fs-index:  20px;  /* 指数数值 */
  --fs-kpi:    28px;  /* AI 欢迎语等 hero 文本 */

  /* 行高 */
  --lh-tight: 1.2; --lh-normal: 1.45; --lh-table: 1.6;
}
```

三条铁律：

1. **所有数字必须 `font-variant-numeric: tabular-nums`**（等宽数字）。行情每 100ms 刷新一次，比例数字会左右抖动，这是专业终端与普通后台的分水岭。
2. 价格/涨跌幅用 600–700 字重；标签用 400–500； ticker 用 700。
3. 中文不单独加大字号，跟随所在组件。

### 2.4 圆角 / 阴影 / 间距

```css
:root {
  --r-pill: 999px;   /* Tab 胶囊、chip、按钮（Buy/Sell 也是全圆角） */
  --r-chip: 6px;     /* 下拉筛选、小标签 */
  --r-card: 10px;    /* 面板/卡片 */
  --r-input: 6px;    /* 输入框 */
  --r-toast: 10px;   /* Toast / 悬浮卡片 */
  --r-tile: 2px;     /* 热力图小瓦片（瓦片间 2–3px 缝隙） */

  --shadow-card:  0 8px 24px rgba(0,0,0,.45);          /* 悬浮卡片（Treemap hover） */
  --shadow-pop:   0 12px 32px rgba(0,0,0,.55);         /* 下拉/命令面板/Toast */
  --shadow-panel: 0 0 0 1px var(--border-soft), 0 16px 48px rgba(0,0,0,.5); /* AI 抽屉 */

  --sp-1: 4px;  --sp-2: 8px;  --sp-3: 12px; --sp-4: 16px;
  --sp-5: 20px; --sp-6: 24px;
  /* 终端密度：表格行高 28–32px；面板内边距 12–16px；模块间距 8–12px */
}
```

### 2.5 动效令牌

```css
:root {
  --ease-out-expo:  cubic-bezier(0.22, 1, 0.36, 1);    /* 全场主曲线（实测先快后慢） */
  --ease-in-out:    cubic-bezier(0.4, 0, 0.2, 1);
  --dur-view:       900ms;   /* 视图形变切换（实测 ~0.9–1.0s） */
  --dur-panel:      800ms;   /* AI 抽屉滑入/退出（实测 ~0.8s） */
  --dur-pop:        200ms;   /* 下拉/菜单/命令面板 */
  --dur-toast:      300ms;   /* Toast */
  --dur-flash:      600ms;   /* 数字闪烁 */
  --dur-hover:      120ms;   /* hover 过渡 */
}
```

---

## 3. 应用骨架布局

### 3.1 实测几何（录屏窗口 1135×690 坐标系）

```
┌──────────────────────────────────────────────────────────────┐  y=0
│ ●●●  ▤  🕘  < >        🔍 Press / to search   Demo A/C(1757) ▾  ✚ │  窗口控制行 ~30px
│ [logo]  US HK CN SG                                            │  品牌行 ~25px
│ Chart Options Financial Forecast Valuation Shareholder Wiki    │  视图Tab行 ~25px
├────┬───────────────────────────────────────┬──────────────────┤
│图标│  左自选列 ~310px（个股页）             │  右报价栏 ~290px  │
│栏  │  ┌───────────────────────────────────┐│  ┌───────────────│
│ ~40│  │ K线主图 + 成交量                   ││  │ 报价/订单簿    │
│ px │  │                                   ││  │ /Time&Sales   │
│    │  └───────────────────────────────────┘│  │               │
│    │  交易面板（Trade/Today's Orders/…）~220px                │
├────┴───────────────────────────────────────┴──────────────────┤
│ ◉头像 US▾  DOW 51729.590 +74.640 +0.14%  NASDAQ …  S&P 500 …  🕐2026/10/12 21:50:14 ● │  底部状态栏 ~30px
└──────────────────────────────────────────────────────────────┘
```

- **左图标栏**：40px 宽，图标 18–20px，垂直居中排布，hover 显示 tooltip（如 `</>` 图标悬停显示 `Quant ×6` 角标 `[帧17]`）；当前项左侧 2px 青色指示条。
- **右报价栏**：固定 290px，不随窗口缩放消失（<1100px 时才折叠为 Tab）。
- **底部状态栏**：全局唯一「时间锚」——服务器时间 + 连接状态绿点，让用户建立对实时性的信任。
- **顶栏三行**合计约 80px，是 macOS 浏览器窗口与 Web 应用的混合体：第一行是**操作系统级控件**（交通灯、前进后退），搜索和账户是应用级。

### 3.2 响应式策略（桌面优先）

| 断点 | 行为 |
|---|---|
| ≥1440 | 标准三栏（自选列 + 主图 + 报价栏） |
| 1135–1440 | 录屏即此档：三栏紧凑 |
| 900–1135 | 报价栏折叠为底部 Tab；自选列收窄 |
| <900 | 自选列隐藏（滑出抽屉）；图表全宽 |

---

## 4. 视图逐个拆解

### 4.1 Market 市场总览 `[帧01]`

**布局**：CSS Grid，`grid-template-columns: 260px 1fr; gap: 12px`，下方通栏 Industry + Quote List。

| 模块 | 结构 | 细节 |
|---|---|---|
| 指数卡 | 左列，3 个 Tab（DOW/NASDAQ/S&P 500）+ 大数值 +  Sparkline | 数值 20px/600，涨跌值同色；迷你折线 60×24，Canvas 绘制，末端渐变填充 |
| Monitor 监控 | 右列大表，US/Watchlist 切换 chip | 行：时间(10px灰) + 方向三角(▲青/▼玫红) + 名称 + 事件徽章(52-week low 灰 / Large buy 青 / Large sell 玫红) + 数值右对齐； zebra 行 + 1px 分隔线；右上角 `…` 溢出菜单 |
| Industry 行业 | 下方通栏，4–5 列行业榜单 | 列头 = 行业名 + 行业涨跌幅（着色）；行 = 股票 + 涨跌幅；Industry/Concept 切换胶囊 |
| Quote List | 底部 chip 横排 | All / Chinese Concept / Popular Stocks / All US ETFs / IPO / Trending / Mention / Followers / Pre Market / Post Market，横向滚动 |

**实现要点**：
- Monitor 表是**实时追加流**：新行从顶部滑入（`height: 0 → auto` + opacity，200ms），超过 50 行出队。用 `requestAnimationFrame` 合批。
- 方向三角用小号 `▲▼` 字符即可，颜色跟随语义变量。
- Sparkline：Canvas + `devicePixelRatio` 缩放，数据超过宽度时抽稀（LTTB 或首尾采样）。

### 4.2 Heatmap 热力图 `[帧02][帧03][放大z03]`

**这是全套 UI 的视觉招牌**，实现质量直接决定「像不像」。

#### 布局结构

```
筛选行： [Heatmap 标题] [S&P 500 Index▾ | Nasdaq Composite Index | All US companies]   …   [Market cap▾] [Change 1D▾] [Sector▾] [<-3%][-3%][-2%][-1%][0%][+1%][+2%][+3%][>+3%] [⟳]
Treemap 区：分组成员（行业）作为背景区域，组内为个股瓦片
```

- 筛选胶囊：**选中态 = 青色描边 + 青字**（非填充）；未选中 = 灰字。
- 右侧三个下拉（Market cap / Change 1D / Sector）控制 Treemap 的 **size 字段、颜色字段、分组字段**——这是可配置的热力图，不是硬编码。
- 阈值 chips 即色阶图例，点击=筛选出该档位的股票（多选）， chip 的填充色就是对应档位色。
- 右上角 `⟳` 手动刷新。

#### Treemap 算法（squarified）

分组行业 → 个股，两层 Treemap：

```ts
// 依赖 d3-hierarchy（或自己实现 squarified，~80 行）
import { hierarchy, treemap, treemapSquarify } from 'd3-hierarchy';

type Node = { name: string; value: number; pct: number; logo?: string; children?: Node[] };

function layoutTreemap(groups: Node[], w: number, h: number) {
  const root = hierarchy({ name: 'root', children: groups })
    .sum(d => d.children ? 0 : d.value)          // 只有叶子占面积
    .sort((a, b) => b.value! - a.value!);
  treemap<Node>().tile(treemapSquarify.ratio(1.3))  // 接近方形的长宽比
    .size([w, h]).paddingOuter(3).paddingTop(19)    // 组内留出标签条高度
    .paddingInner(2)(root);                          // 瓦片间 2px 缝隙
  return root;
}
```

- `paddingTop(19)`：每个行业分组区顶部留 19px 标签条，显示行业名 + 行业涨跌幅，标签条底色 = 该组加权涨跌幅对应的热力色（实测 `#0A826E` 青 / 深玫红）。
- 瓦片渲染：面积 > 12000px² 显示 logo+ticker+%；> 3000 显示 ticker+%；更小只显示 logo；极小只显示色块。
- 颜色 = `heatColor(pct)`（§2.2），与分组标签条同函数。

#### 交互

- **Hover 瓦片 → 悬浮卡片** `[放大z03]`：深色浮卡（`--bg-panel-2` + `--shadow-card` + 10px 圆角），带指向瓦片的小三角；内容 = logo + `AVGO` + `US Broadcom` + 四行指标（Price / Change 1D 着色 / Change YTD 着色 / Market cap）。卡片出现 120ms fade+translateY(4px)，鼠标移出立即消失（终端场景不用退场动画，避免遮挡）。
- 点击瓦片 → 跳转个股详情（触发视图形变动画，见 §5.1）。
- 悬停时其余瓦片**不**变暗（保持信息密度），仅浮卡本身抬升。

#### 性能

- 300+ 瓦片用 **Canvas** 批量绘制（颜色填充 + 文字）， hover 命中用四叉树或简单的网格索引；DOM 方案在 500+ 节点时掉帧。
- 数据更新（价格跳动）时**只重绘变色瓦片**：维护 `Map<tileId, color>`，diff 后局部 `clearRect`。

### 4.3 Watchlist 自选网格 `[帧04]`

- 顶部：`All Symbols | US | HK | Holdings | A-Shares | SG | Indices` Tab + 周期选择 `Intraday 1D W M Y` + 粒度 `5min 15min 30min 1h`。
- 主体：**响应式宫格**（`repeat(auto-fill, minmax(280px, 1fr))`），每卡一张迷你 K 线：
  - 卡头：名称 + 价格 + 涨跌幅（着色，12px/600）
  - 卡体：Canvas K 线（高 ~110px）+ 下方成交量（高 24px，颜色随蜡烛）
  - 卡脚：`Candlestick only` 标签 + `Split-Adj.` 复权切换
- 右栏：当前选中个股的完整详情（与个股页同组件，复用）。
- 迷你图同样支持十字光标 OHLC（低配版：只显示竖直准线）。

### 4.4 个股详情（Chart 视图）`[帧06][放大z05][放大z06][放大z08]`

整套 UI 的最高完成度模块，拆成 5 个子组件。

#### 4.4.1 K 线主图

```
┌ NVDA 228.035 (-0.54%)        ✏️(画线) ⤢(全屏) ☰(指标) Trading(交易) ⋯ ┐
│ 2026.06.10 ET Wed -3.73%(-7.761) O:204.201 H:206.988 L:199.696 C:200.196 Vol:161.75M T:32.83B │ ← 十字光标 OHLC 读数条
│  251.340 ┤                                        ╭─╮                   │
│          │      ╭╮  ╭─╮                          ╭─╯ ╰─╮   243.370       │
│  228.035 ┤ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ [228.035] ─ ─ ─ ─ ─ ─  │ ← 最新价：蓝色虚线 + 蓝底白字标签
│          │  ╰╯╰╮╰─╯╰─╮╰╮                                          │
│  163.669 ┤ ╰─╯  ╰────╯╰─╯          ⓔ    ⓓ    ⓔ ⓓ    ⓔ    ⓓ          │ ← 事件标记：ⓔ财报(青) ⓓ分红(橙)
│          └────┴────┴────┴────┴────┴────┴────┴────┴────┴────┴──       │
│  VOL VOLUME:161.75M  ▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮▮  │ ← 成交量（颜色随对应蜡烛）
│  [Intraday 5D D W M Q Y 1min 5min 15min 30min 1h ▾]        Split-Adj. ⇄ │
└──────────────────────────────────────────────────────────────────────┘
```

实现规格：

| 元素 | 规格 |
|---|---|
| 蜡烛 | 涨=`--up-glow` 青、跌=`--down-deep~glow` 玫红；**空心蜡烛**（涨=青边青芯、跌=玫红边玫红芯，实测为实心+细影线）；影线 1px |
| 最新价 | 水平虚线 `--price-line` + 右端圆角标签（蓝底白字 11px） `[放大z08]` |
| 网格 | 水平虚线 `rgba(255,255,255,.05)`，垂直网格更淡；Y 轴右侧 4–5 个价格刻度 |
| 十字光标 | 竖直+水平 1px 实线（白 30% 透明），跟随鼠标；左上角 OHLC 读数条（日期+涨跌+O/H/L/C/Vol/Turnover，等宽数字） |
| 事件标记 | 轴下方圆形徽章：`E`(earnings, 青底) / `D`(dividend, 橙底)，hover 显示详情 tooltip |
| 成交量 | 独立子图（高度比 1:4），每根柱颜色 = 对应蜡烛方向色 |
| 周期/粒度 | 两级选择：周期（Intraday/5D/D/W/M/Q/Y）+ 粒度（1min…1h），当前项白字+下划线 |
| 复权 | `Split-Adj. ⇄` 开关，切换时**重算序列并保持视口**（FLIP） |

技术：Canvas 2D 自绘（理由：十字光标、OHLC 浮层、事件标记、复权都是高频交互，自绘最可控； Lightweight Charts 需大量覆写）。**必须**：
- `devicePixelRatio` 适配，避免高清屏发虚；
- 视口 = `[startIdx, endIdx]` 状态，缩放/平移只改状态不重取数；
- 数据抽稀：屏宽 800px 时最多画 400 根，超出按 OHLC 聚合；
- 动画：周期切换时 300ms 补间旧新序列（可选）。

#### 4.4.2 左自选列 `[放大z07]`

- 宽 310px，行高 34px：旗帜 emoji + 名称/代码 + 价格（右对齐）+ 涨跌幅（着色，右对齐）。
- 当前行：左侧 2px 青色竖条 + 背景 `--bg-active`。
- 排序：点击 `Price ↕` / `%Chg ↕` 表头切换。
- **实时跳动**：价格变化时整行背景闪一下方向色（600ms 淡出），数字用 tabular-nums 不抖。
- 虚拟滚动（TanStack Virtual），支持 1000+ 行。

#### 4.4.3 右报价栏（Quote Panel）

```
NVIDIA NVDA
228.035 -1.245 -0.54%          ← 大价格 22px/700 + 涨跌额/幅（着色）
Trading 10/12 09:50:24 EDT     ← 10px 灰 + 市场状态
🇺🇸 💱 🛒 ⚙️(4个图标按钮)
──────────────────────────────
Open 749.689    Volume 3.15M shares
High 749.790    Turnover 2.36B
Low  747.100    Volume Ratio 1.69
Prev.Close 751.270  Bid/Ask Price -14.29%
Amplitude 0.36%   NAV 751.230
Turnover Ratio 0.47%  NAV Date 2026/10/09
Average Price 748.938  Total Issu... 675.65M shares
52 wk High 762.860   Bid/Ask Ratio 505.34B
52 wk Low 554.412    Div/Preferred Rate 0.01%
Float Cap 505.34B    Dividend Yield(TTM) 0.41%
Lot Size 1           Dividend(TTM) 3.092
──────────────────────────────
● Large sell 1.71K shares   09:49:35 EDT   ← 实时事件流（方向点着色）
▲ December hike still up in the air
  Warn: Speaks at IMF Annual Meeting — Is a Dec Rate Hike...
──────────────────────────────
[Quotes] [News] [Wiki]      ← 内容 Tab
Bid 57.14%                  ← 买卖压力条：左青右玫红的分裂条
BBO 747.890 120  747.920 160
──────────────────────────────
Order Book ○
Nasdaq TotalView    [Subscribe]
──────────────────────────────
Time & Sales (Intraday)
09:50:36  228.035  10  NASDAQ  09:50:36
…                        ← 实时成交流，行高 22px，虚拟滚动
```

实现要点：
- 指标区用两列 `label(11px灰) value(12px白/着色)` 网格，行高 20px；
- **买卖压力条**：横向 100% 条，左段青色（Bid 42.86%）右段玫红（Ask 57.14%），百分比数字置中于条上（白字 10px）——比纯数字更直观，是终端的标志性小组件；
- Time & Sales：WebSocket 推送，新行顶部插入 + 行首时间戳；限制 200 行 + 虚拟滚动；
- `Subscribe` 按钮表示 Level2 行情订阅态（未订阅置灰 tooltip 引导）。

#### 4.4.4 底部交易面板 `[放大z06]`

```
[Trade][Today's Orders][Past Orders][Grid Trading][Holdings]
Symbol [NVDA▾]   Typeⓘ [Limit▾]                    B/S  [Buy][Sell]
Price  [− 227.58 +] 🎯   Quantity [− Min.0.0001 +] 🗂   Time in force [Valid for the day▾]
Buy on Cash ⓘ 0 shares        Buy on Margin ⓘ 199 shares
Session ○ [Market + extended hours▾]        Attached order ○────
[Simulated Buy]                      0.00 USD  Est. cost after trade 0.00 USD
```

- 表单控件：深底输入框（`--bg-input` + 1px `--border-strong`）+ **独立步进按钮**（−/＋，28px 宽，点击 ±1 tick，长按加速）；
- 价格输入旁的 🎯 图标 = 「取当前价」；数量旁 🗂 = 常用手数快捷档；
- `Buy` = 青绿填充胶囊（白字），`Sell` = 玫红填充胶囊——**全应用唯一允许大面积语义色填充的按钮**；
- 下单成功后：Toast（§4.7）+ 按钮进入 loading → `Filled` 状态；
- 演示账户显示 `Simulated Buy`（描边样式），与真实账户区分。

### 4.5 Financial 财务页 `[帧07][帧08][帧09][放大z04]`

页面 = 一列可滚动的**卡片流**，卡片间距 12px。二级 Tab：`Earnings | Revenue Breakdown | Financial Score | Key Indicators | Income Statement | Balance Sheet | Cash Flow Statement`。

#### 4.5.1 Earnings（AI 财报解读）

- 卡片头：`Q2 FY2027 Earnings` + `Analysis, Aug 27, 2027` + `Quarterly▾ / Q2 2027▾`。
- **AI 生成摘要**：正文流式输出，关键数据加粗/着色（`+105.85%` 青、`▲` 前缀）。
- **指标表**：列 = `Metric | Actual YoY | Estimate YoY | Vs estimate`；行 = Revenue / EBIT / EPS；超预期行右侧 `▲` 绿标。
- 底部 `Full analysis ›` 链接（青色）。

#### 4.5.2 Revenue Breakdown（桑基图）`[放大z04]`

**整套 UI 技术含量最高的图表**，用 ECharts `series.type: 'sankey'` 或 `d3-sankey` 自绘：

```
[收入侧 蓝 #5060F0]                [利润侧 绿 #38C850]         [费用侧 橙 #F09818]
Compute & Networking 88.30B ▲113.64% ┐
                                      ├→ Total Revenue 96.22B ▲105.85% → Gross Profit 72.14B ▲113.10% ┐
Graphics 7.92B ▲46.36% ──────────────┘                                    │                            │
                                                       Cost Of Revenues 24.08B ▲86.60% ←───────────────┤
                                                                                                        │
                              Gross Profit → Operating Income 63.73B ▲124.10% ─┬→ Net Income 59.69B ▲125.90%
                                                      Operating Exp. 8.41B ▲55.33% ─┘   Income Tax Exp. 11.82B ▲147.05%
                                                      ├→ SG&A Exp. 1.35B ▲20.68%
                                                      └→ R&D Exp. 7.05B ▲64.39%
```

规格：
- 节点 = 6px 圆角竖条，颜色按语义（收入蓝/利润绿/费用橙）；节点标签 = `数值(12px白) + 名称(11px灰) + ▲同比(11px绿)`，置于节点右侧；
- 流 = 两端节点色的**线性渐变带**（蓝→绿、绿→绿、橙→橙），宽度 ∝ 金额，透明度 ~0.45；
- 图例：`● Income ● Profit ● Expense  Currency: USD`；
- 卡片右上：`Zoom In` 按钮 + 全屏 ⤢；**点击 Zoom In 进入沉浸模式**：桑基图放大至全屏、其余内容淡出（§5.1 同款形变动画）`[帧09→26.5s]`；
- 右上 `Quarterly▾ / Q2 2027▾` 切换报告期，切换时桑基**重播生长动画**（流宽从 0 → 目标，600ms）。

#### 4.5.3 Revenue Structure（组合图 + 表）

- 分段控件 `Chart View | Percentage View`（选中 = 青底白字胶囊）；
- 组合图：蓝色柱（分部收入）+ 琥珀色折线（QoQ %，右轴 -50%~+100%），双 Y 轴；
- Hover：竖直准线 + 深色 tooltip（`Q3 2025 / ● Compute & Networking 31.04B / ● Graphics 4.06B`）`[帧09]`；
- 下方表格：`Name | Revenue | QoQ | Percentage`，行 = 业务分部；
- 数据源 chip：`QoQ Currency: USD` + `Industry▾ Quarterly▾`。

#### 4.5.4 Operating Metrics（多 Tab 卡片流）

Tab：`Revenue by Business Segment | Data Center Revenue Breakdown | Revenue by Geography | Expense ratio | Operating Indicators | Revenue Guidance | Gross Margin Guidance`——同一卡片容器内切换图表，图表区高度固定 220px，切换时 fade+translateY(8px) 200ms。表格式数据用密集小表（行高 22px）。

### 4.6 Options 期权页 `[帧14][帧15][放大z09][放大z10]`

```
Options AAPL                                    🔍 ⤢ ⋯
[Option Chain][Unusual Activity][Option Analysis]        All Call Put ⚙
Apple 340.770 +4.130 +1.23%        ← 正股报价条（着色）
████████████████ teal ████████│████████ pink ████████████████   ← Call/Put 持仓压力条
Call 124.20K              71:29              Put 50.65K          ← 两端标签
10/12* 10/14* 10/16 10/19* … 12/18 27/01/1▾   ← 到期日横向条（* = 周度，当前项高亮）
┌─ Calls ─────────────────────────┬─ Puts ──────────────────────┐
│ Ask Bid Last  Vol OI            │ Strike │ Vol OI Last Bid Ask │
│ …                               │ 350    │ …                    │
└─────────────────────────────────┴─────────────────────────────┘
```

- **到期日条**：横向滚动 chip 列表，当前到期日 = 白字 + 底部 2px 青条；带 `w` 上标 = 周度期权。
- **压力条**：全宽 8px 分裂条，左段青（Call 持仓量）右段玫红（Put 持仓量），中间显示距到期时间（`71:29`）。
- **双边链**：一个表格两种语义——左半 Call（卖价在前）、右半 Put；中间 Strike 列固定对齐，滚动时 Strike 列**吸顶**（`position: sticky`）；ATM 行（最接近现价）高亮 + 左侧箭头标记；所有价格按买卖方向着色。
- 点击某行 → 下方展开**合约详情**：合约名（`AAPL 261012 337.5 Call`）+ 价格 + 迷你走势图 + **P&L 损益图**：
  - SVG 折线：到期损益（kink 线，盈亏以 0 轴分界，盈利段青、亏损段玫红，渐变填充）；
  - 标注：`Max profit: Unlimited` / `Max loss: -370.00` / `Break-Even 341.20` / `Delta 0.8153`；
  - 交互：拖动到期价格竖线，实时重算损益数值 `[帧15]`。
- 下单：数量输入（`1 unit of contracts` 下拉切手/张）、`Simulated Buy`；成交通知 Toast `[放大z10]`。

### 4.7 LongbridgeAI 助手 `[帧10][帧11][帧12][帧13][放大z10]`

两种形态 + 一套对话引擎。

#### 4.7.1 形态 A：右侧抽屉

- 入口：顶栏右侧 **✚ 青色图标按钮**（带轻微脉冲光晕吸引点击）。
- 行为：宽 380px 的右侧抽屉**滑入**（translateX 100%→0，800ms `--ease-out-expo`），同时**主内容区缩放至 0.97 + 压暗**（`filter: brightness(.85)`）——制造「主界面退后」的纵深感 `[帧10]`。
- 结构：头部（`LongbridgeAI` + ➕新会话 + ⋯更多）→ 消息流 → 底部输入区（附件 📎、@提及、`Flash▾` 模型选择、发送 ⬆）。

#### 4.7.2 形态 B：全屏

- 从抽屉**形变放大**到全屏（Framer Motion `layout` / View Transitions，800ms）`[帧12]`。
- 左侧 260px 会话历史栏：`New chat` 按钮 + `Search` + 按 `Today / Earlier` 分组的会话标题（自动取首条消息摘要，支持中英混排）。
- 中央：`Hello, may I help you?` hero 标题（28px/700），标题下是**三色渐变下划线**（粉→橙→青，宽 200px，3px 高，圆角）——AI 品牌的视觉签名。
- 建议问题卡：4–5 张深色圆角卡（`--bg-panel` + 1px 边框），文案为真实用户问过的问题（部分带 `$` 代码图标）；点击即发送；底部 `⟳ Shuffle` 换一批（400ms 交叉淡入）。
- 输入框：居中大输入框（高 56px，`--bg-input`，圆角 12px），placeholder 闪烁光标；右侧 `Flash▾` + 圆形发送钮。

#### 4.7.3 对话与富文本渲染 `[帧11][帧13]`

- **流式输出**：逐 token 追加，光标处闪烁 caret（`▍` 1px 宽，530ms 步进闪烁）；Markdown 增量解析（标题/加粗/列表/代码块）。
- **思考态**：`Nebulizing… 0s`（文案随机化：Galloping… / Nebulizing…），带省略号呼吸动画 + 秒表计时——把等待游戏化。
- **Agent 步骤条**：调用站内工具时显示步骤（`Screen for US stocks over $100B that bea ›` + `Screener indicators · 1s` + 青色脉冲点），让用户感知「AI 在干活」`[帧13]`。
- **富文本卡片**：回复内嵌组件——
  - 行情卡：logo + 名称 + 大价格 + 迷你图 + Metric/Value 表；
  - 数据表：完整表格（`# | Ticker | Company | Mkt Cap | 1M% | YTD% | Price`，45+ 行，行内着色）；
  - 要点列表：方向标签着色（`Regulatory headline (mild negative):` 玫红 / `Offsetting positives:` 青）。
- **免责声明**：每次 AI 回复底部固定灰字：`This content is AI-generated and provided for reference only…`。
- 追问建议：回复尾部 3–4 个相关问题 chip（`↳` 前缀）。

### 4.8 Quant 量化 `[帧17][帧18]`

- 入口：左栏 `</>` 图标，tooltip 显示 `Quant ×6`（6 个打开的编辑器 Tab，证明是**多文档编辑器**）。
- **DSL 代码编辑器**：类 TradingView Pine 的脚本语言，语法高亮（关键字/装饰器/字符串/数字/函数/注释分色），左侧行号列，顶部 Tab 栏。技术选型：**CodeMirror 6**（体积小、可定制语言高亮）或 Monaco（功能全但重）。
- 语言特性（从代码可见）：`@locale def({...})` 装饰器做**界面文案多语言**（zh-CN/zh-HK 内联）、`input()` 声明参数、`ta.*` 技术指标库、`strategy.entry()` 下单原语、`overlay: true` 主图叠加。
- **回测运行**：点击运行 → 日志面板流式打印（`backtest task accepted…` / `loading strategy runtime…` / `validating backtest parameters…` / `loading historical quotes…`），带 spinner。
- **回测结果**：Tab = `Performance | Trades | Strategy info` + `Backtest Summary` 浮层（权益曲线数值）；主图叠加策略买卖点标记（`341.160` 青 / `345.340` 玫红 价格标签）；权益曲线 = 青色面积图 + hover tooltip（`2026/03/05 13:00 / Return -0.40% / Daily P/L -4220.88`）。
- **回测历史**：顶部横排 recent runs（`RSI Strategy -85.30% Just now / 6 minutes ago / 14 minutes ago`）+ 提示 `Server keeps the latest 3 runs`。

### 4.9 命令面板与主题系统 `[帧19]`

- 入口：头像菜单 → `Theme:…`，或快捷键（Cmd/Ctrl+K）。
- **居中偏上的命令面板**：搜索框 `Type a command or search…` + 主题列表（Default Dark / Adventure / Adventure Time / Alduin / Ascelinea / Ayu Dark / Catppuccin Mocha / Catppuccin Frappe / Catppuccin Macchiato / Everforest Dark），每行右侧 `Dark` 徽章；当前项高亮，键盘上下选择 `[帧19]`。
- 实现：**全站 CSS 变量主题**——所有颜色令牌挂在 `:root`，切换 = 换 `data-theme` 属性 + 一整套变量值；图表颜色从 CSS 变量读取（`getComputedStyle`），切主题时图表重绘。这也是唯一需要「设计 10 套配色」的地方，令牌化后成本极低。

---

## 5. 动效系统

> 全部时长/曲线来自录屏的帧间差分实测（60fps 下每 100ms 采样）。核心特征：**先快后慢的 ease-out、0.8–1.0s 的从容时长、带运动模糊的缩放形变**——这是「高级感」的来源，切忌 300ms 的急促过渡。

### 5.1 视图切换（形变动画）`[帧05]`

- 实测：15.7s→16.6s 持续约 0.9s，帧差先增后减（4.3→4.9→…→1.4→0.7），中间帧**重度运动模糊**——判定为 `scale + opacity + filter: blur()` 组合的形变，而非简单淡入淡出。
- 实现（二选一）：
  1. **View Transitions API**（现代浏览器，最省事）：
     ```ts
     document.startViewTransition(() => setView(next));  // ::view-transition-old/new 自动 crossfade
     ```
     ```css
     ::view-transition-old(root) { animation: vt-out 900ms var(--ease-out-expo) both; }
     ::view-transition-new(root) { animation: vt-in  900ms var(--ease-out-expo) both; }
     @keyframes vt-out { to { transform: scale(0.96); opacity: 0; filter: blur(6px); } }
     @keyframes vt-in  { from { transform: scale(1.04); opacity: 0; filter: blur(6px); } }
     ```
  2. **Framer Motion** `layout` + `AnimatePresence`（兼容性更好，可做共享元素）：
     ```tsx
     <AnimatePresence mode="popLayout">
       <motion.div key={view}
         initial={{ scale: 1.03, opacity: 0, filter: "blur(8px)" }}
         animate={{ scale: 1, opacity: 1, filter: "blur(0px)" }}
         exit={{ scale: 0.97, opacity: 0, filter: "blur(8px)" }}
         transition={{ duration: 0.9, ease: [0.22, 1, 0.36, 1] }}>
     ```
- 降级：`prefers-reduced-motion` 时改为 150ms 纯淡入。

### 5.2 AI 抽屉 / 全屏

- 抽屉：`x: 100% → 0`，800ms `--ease-out-expo`；同时主内容 `scale: 1 → 0.97` + `brightness(1 → .85)`（CSS `transition` 同步 800ms）——**双层动画制造深度**。
- 全屏：抽屉 → 全屏用 `layout` 共享布局动画（宽度/位置/圆角补间），消息流保持位置连续。
- 关闭：反向，主内容弹回（可加 4% 回弹 overshoot：`ease: [0.34, 1.56, 0.64, 1]`）。

### 5.3 微交互清单

| 交互 | 规格 |
|---|---|
| Treemap 瓦片 hover | 浮卡 120ms fade + translateY(4px)→0，`--shadow-card` |
| 表格行 hover | 背景 120ms → `--bg-hover` |
| Tab 切换 | 活动胶囊 200ms 位移 + 颜色（Framer Motion `layoutId` 共享胶囊） |
| 下拉/菜单/命令面板 | 200ms fade + scale(0.96→1) + translateY(-4px→0) |
| Toast（成交通知） | 从顶栏下方滑入（translateY(-12px)→0 + fade），300ms；3s 后淡出；可手动关闭 `[放大z10]` |
| 数字跳动（tick flash） | 价格变化时背景闪方向色 600ms 淡出；**只闪背景不闪文字**（文字保持可读） |
| 流式 caret | 1px 宽、530ms 步进 `step-end` 闪烁 |
| 思考省略号 | 三个点依次呼吸（opacity 0.3→1，300ms 间隔） |
| 图表 tooltip | 跟随鼠标，偏移 12px，贴边翻转；150ms fade |
| 桑基图重播 | 流宽 0→目标 600ms `--ease-out-expo`，节点延迟 100ms |

### 5.4 动效实现原则

1. **只动 transform / opacity / filter**（合成器属性），绝不碰 width/top/left。
2. 列表插入/删除用 FLIP（First-Last-Invert-Play），保持其余行稳定。
3. 实时数据（tick）**不做进出场动画**，只做状态闪烁——动画会让眼睛追不上价格。
4. 所有动效统一从令牌取时长/曲线，禁止散落硬编码。

---

## 6. 图表与可视化实现

### 6.1 技术选型总表

| 图表 | 推荐方案 | 理由 |
|---|---|---|
| K 线 + 成交量 + 十字光标 | **自绘 Canvas 2D** | 交互密度最高（OHLC 浮层/事件标记/复权/画线），封装库限制多 |
| 迷你图宫格 | Canvas（复用主图引擎的低配模式） | 一屏 9–12 张，必须轻 |
| 指数 Sparkline | Canvas 30 行 | — |
| Treemap 热力图 | `d3-hierarchy` 布局 + **Canvas 渲染** | squarified 算法现成；300+ 节点 Canvas 才稳 |
| 桑基图 | **ECharts sankey**（渐变 flow 需 `itemStyle.color` 用渐变对象或 `d3-sankey` 自绘） | ECharts 开箱即用的层级/标签/主题；渐变流需自定义 `links.itemStyle.color: linearGradient` |
| 组合图（柱+双轴线） | ECharts | 双轴/图例/zoom 现成 |
| P&L 损益图 | 手写 SVG（<100 行） | 图形简单、需拖动交互 |
| 买卖压力条 | 纯 CSS flex + transition | — |
| 代码编辑器 | CodeMirror 6 | 语言高亮可定制（DSL），体积 ~200KB |
| 表格 | TanStack Table + TanStack Virtual | 排序/列/虚拟滚动全都要 |

### 6.2 K 线图核心骨架（TypeScript）

```tsx
// 关键点：视口状态机 + DPR 适配 + 分层 canvas（静态层/交互层）
export function useCandlestick(data: Candle[], container: RefObject<HTMLDivElement>) {
  const [view, setView] = useState({ start: 0, end: data.length }); // 视口 = 索引区间
  const dpr = window.devicePixelRatio || 1;

  // 1. 数据抽稀：屏宽内最多 N 根
  const visible = useMemo(() => {
    const step = Math.max(1, Math.ceil((view.end - view.start) / 400));
    return aggregate(data.slice(view.start, view.end), step); // OHLC 聚合
  }, [data, view]);

  // 2. 分层：grid/price 静态层（数据变才重绘）+ crosshair 交互层（鼠标动才重绘）
  // 3. 十字光标：mousemove → 索引 = floor((x - padLeft) / barW) → 更新 OHLC 读数条（React state）
  // 4. 缩放：wheel → 以光标位置为锚点缩放 view；拖拽 pan → 平移 view
  // 5. 最新价：最后一根 close → 水平虚线 + 标签（--price-line）
  // 6. 事件标记：earnings/dividends 数组 → 轴下圆形徽章
}
```

### 6.3 Treemap 渲染要点

```tsx
// 布局（d3）→ 渲染（Canvas）
// 颜色：heatColor(pct)  §2.2
// 文字规则（按面积降级）：
//   >12000px²: logo(24px圆) + ticker(16px/700) + pct(12px)
//   >3000px²: ticker + pct
//   >800px² : logo + ticker(11px)
//   else    : 纯色块
// hover：网格索引（把画布按 40px 分格，记录每格 tileId）→ O(1) 命中 → 浮卡（DOM，避免 Canvas 内画文字模糊）
```

### 6.4 桑基图（ECharts 渐变流）

```ts
{
  series: [{
    type: 'sankey',
    left: 8, right: 120, top: 10, bottom: 8,
    nodeWidth: 6, nodeGap: 12,
    data: [
      { name: 'Total Revenue', itemStyle: { color: '#5060F0' } },
      { name: 'Gross Profit',   itemStyle: { color: '#38C850' } },
      { name: 'Cost Of Revenues', itemStyle: { color: '#F09818' } },
      // ...
    ],
    links: [
      { source: 'Total Revenue', target: 'Gross Profit', value: 72.14,
        lineStyle: { color: 'linearGradient(90deg, #5060F0, #38C850)', opacity: 0.45, curveness: 0.5 } },
      // ...
    ],
    label: { formatter: (p) => `${p.data.value}B\n${p.data.name}`, /* 右侧两行：数值+名称 */ },
    emphasis: { focus: 'adjacency' },
  }]
}
```

---

## 7. 数据、实时与性能

### 7.1 实时架构

```
WebSocket（行情网关）
  ├─ tick 批量帧（100ms 内合批）→ store（Zustand，selector 精确订阅）
  │    → 订阅者：自选列/报价栏/T&S/热力图瓦片/K 线最新价
  ├─ 快照（开盘/重连）→ 全量替换 + 视图形变重绘
  └─ 事件流（大单/财报/分红）→ Monitor 表插入 + Toast

规则：
1. 所有 push 进入 store 后由 rAF 统一 flush，禁止每条消息 setState；
2. 价格跳动只触发「变色 + 数字替换」，DOM 节点 key 稳定（ticker 为 key）；
3. 断线：底部状态栏绿点 → 黄点（重连中）→ 红点（断开）+ 顶部 banner；
4. 时间锚：底部状态栏服务器时间 + 各面板 `Trading 10/12 09:50:24 EDT` 本地化时区标注。
```

### 7.2 性能清单

| 场景 | 手段 |
|---|---|
| 自选列 1000 行 / T&S 无限追加 | TanStack Virtual，overscan 10 行 |
| 热力图 300+ 瓦片 | Canvas + 局部重绘 + 网格索引命中 |
| K 线 10 万根 | 视口切片 + 抽稀聚合；历史按需 fetch |
| 多图表同屏（Watchlist 宫格） | 共享引擎 + `IntersectionObserver` 离屏暂停渲染 |
| 主题切换 | CSS 变量（零重排）+ 图表读变量重绘（debounce 100ms） |
| 首屏 | 视图级代码分割（Financial/Options/Quant/AI 全部 lazy） |
| 数字防抖 | `tabular-nums` + 固定列宽（`min-width` + `ch` 单位） |

---

## 8. 工程结构与分阶段路线

### 8.1 推荐技术栈

```
Vite + React 18 + TypeScript（严格模式）
├─ 样式：Tailwind CSS（或 CSS Modules）+ CSS 变量主题层（tokens.css）
├─ 状态：Zustand（行情/会话/UI）+ TanStack Query（REST 快照）
├─ 图表：自绘 Canvas 引擎 + ECharts（桑基/组合图）+ d3-hierarchy（Treemap 布局）
├─ 表格：TanStack Table + Virtual
├─ 动效：Framer Motion（layout/AnimatePresence）+ View Transitions API（渐进增强）
├─ 编辑器：CodeMirror 6 + 自定义 DSL 语言包
├─ 实时：WebSocket + rAF 合批
└─ i18n：react-i18next（中/英/繁）
```

### 8.2 目录骨架

```
src/
├─ tokens/            # design tokens（css 变量 + ts 常量，唯一色彩来源）
├─ components/        # 通用：Button/Chip/Input/Segmented/Tabs/Table(Virtual)/Toast/Tooltip
├─ charts/            # candlestick/ treemap/ sankey/ combo/ sparkline/ pnl/ pressure-bar
├─ views/
│  ├─ market/ heatmap/ watchlist/ stock/ financial/ options/ quant/ ai/
├─ ai/                # 流式客户端、Markdown 渲染、富文本卡片、agent 步骤
├─ store/             # quote store / ui store / chat store
├─ hooks/             # useWebSocket / useRAF / useVirtualGrid / useTheme
└─ pages/             # Shell（顶栏/左栏/底栏/视图路由）
```

### 8.3 实施阶段（建议顺序）

| 阶段 | 内容 | 验收 |
|---|---|---|
| P0 骨架 | Shell + tokens + 主题切换 + 命令面板 | 顶栏三行/左栏/底栏像素级还原；10 套主题可切 |
| P1 热力图 | d3 treemap + Canvas 渲染 + heatColor + hover 卡 + 阈值 chips | 300 瓦片 60fps；色阶与 chip 一一对应 |
| P2 Market | 指数卡 + Monitor 流 + Industry + Quote List | WS 推送插入流畅；行 hover/闪烁正确 |
| P3 个股 | K 线引擎（十字光标/OHLC/复权/事件）+ 自选列 + 报价栏 + 交易表单 | 缩放平移跟手；tick 只闪不抖 |
| P4 财务 | Earnings 卡 + 桑基 + 组合图 + Zoom In 沉浸模式 | 桑基渐变流/重播动画；卡片流滚动流畅 |
| P5 期权 | 到期日条 + 压力条 + 双边链 + P&L 拖动 | Sticky Strike 列；ATM 行高亮 |
| P6 AI | 抽屉/全屏/历史/流式/富文本卡片/agent 步骤 | 流式 caret；工具调用步骤可见 |
| P7 量化 | CodeMirror DSL + 回测日志 + 结果图 | 语法高亮；多 Tab |
| P8 打磨 | 动效统一走令牌、a11y、i18n、性能审计 | Lighthouse ≥ 90；动效全部来自令牌 |

---

## 9. 验收清单（对着录屏逐项打勾）

**视觉**
- [ ] 主背景 `#18191D`，面板/浮卡/输入框三层亮度可辨（4% / 8% / -4%）
- [ ] 涨跌语义色全套（文字/按钮/热力图/K 线/订单簿）一致，无第四种红绿
- [ ] 交互主色青 `#0CA890` 与涨跌色不混用（Buy 除外）
- [ ] 所有数字 tabular-nums，刷新不抖动
- [ ] 圆角令牌：胶囊全圆角 / 卡 10px / 输入 6px / 瓦片 2px

**布局**
- [ ] 顶栏三行（窗口控制/品牌+区域/视图 Tab）≈80px
- [ ] 左图标栏 40px + 当前项 2px 青条 + tooltip
- [ ] 个股页三栏（310 / 自适应 / 290）+ 底部交易面板 220px
- [ ] 底栏：头像 + 市场 + 三大指数 + 服务器时间 + 状态点

**热力图**
- [ ] squarified 布局，瓦片缝 2px，组标签条 19px
- [ ] 9 档色阶 ↔ 9 个阈值 chip 互相对应
- [ ] hover 浮卡带指向三角 + Price/1D/YTD/MktCap 四行

**K 线**
- [ ] 十字光标 + OHLC 读数条 + 最新价蓝线蓝签
- [ ] 成交量子图颜色随蜡烛；E/D 事件徽章
- [ ] 周期/粒度两级选择；Split-Adj. 保持视口

**动效**
- [ ] 视图切换 ~900ms scale+blur ease-out
- [ ] AI 抽屉 800ms 滑入 + 主内容 0.97 缩放压暗
- [ ] Toast 300ms 滑入；tick 闪烁 600ms
- [ ] `prefers-reduced-motion` 降级

**AI**
- [ ] 三色渐变下划线；流式 caret；思考态计时
- [ ] agent 步骤条；富文本行情卡/数据表；免责声明

---

## 附录 A：参考帧索引（`reference-frames/`）

| 文件 | 内容 |
|---|---|
| 01_market_overview | Market 总览（指数/Monitor/Industry/Quote List） |
| 02_heatmap_sp500 | 热力图（S&P 500，含阈值 chips、悬停卡） |
| 03_heatmap_nasdaq | 热力图（Nasdaq，分组密集态） |
| 04_watchlist_grid | 自选迷你 K 线宫格 |
| 05_view_transition_blur | 视图切换中的运动模糊帧 |
| 06_stock_chart_nvda | 个股详情全貌（K 线+自选列+报价栏+交易面板） |
| 07_financial_earnings_ai | AI 财报解读 + 指标表 |
| 08_financial_sankey | 收入结构桑基图 |
| 09_revenue_structure | 组合图 + tooltip + Operating Metrics |
| 10_ai_side_panel | AI 抽屉形态 + 建议问题 |
| 11_ai_chat_rich_content | 流式回复 + 行情卡 + 免责声明 |
| 12_ai_fullscreen_greeting | AI 全屏欢迎态 |
| 13_ai_screener_table | AI 选股器结果表 |
| 14_options_chain | 期权链 + 到期日条 + 压力条 |
| 15_option_pnl_detail | 合约 P&L 损益图 + Greeks |
| 16_trade_filled_toast | 成交通知 Toast |
| 17_quant_code_editor | DSL 代码编辑器（多 Tab） |
| 18_backtest_result | 回测结果 + 权益曲线 |
| 19_theme_command_palette | 命令面板 + 主题列表 |
| z01–z10 | 局部放大：窗口顶部两行/底部指数条/热力图瓦片与悬停卡/桑基细节/K 线细节/交易面板/自选列行/价格标签/期权顶部/Toast |

## 附录 B：从录屏中提炼的「专业感」细节（易被忽略）

1. **方向三角**（▲▼）比 +/- 号更省空间，Monitor/自选/期权链通用。
2. **事件徽章**（E/D/52-week low/Large buy）用文本而非图标，密度更高。
3. **买卖压力分裂条**把百分比可视化，比纯数字快 3 倍识别。
4. **服务器时间戳 + 连接状态点**建立实时信任。
5. **「 thinking 文案随机化」**（Nebulizing/Galloping）把 AI 等待游戏化。
6. **回测历史限 3 条**并明示——管理预期，避免无限列表。
7. **复权开关、时区标注（EDT）、币种标注（Currency: USD）**——专业终端的地毯式标注习惯。
8. **表格斑马纹 + 1px 分隔线**而非卡片阴影——高密度下的可读性优先。

---

## 10. 实现验证与踩坑记录（对照复刻产物 `lb-terminal/`）

> 依据本指南已完成一套零依赖（原生 HTML/CSS/JS + Canvas/SVG）的复刻实现，位于 `docs/UI/03-longbridge-terminal/demo/`，
> 并用 Playwright 无头截图（`lb-terminal/shots/*.png`）与录屏参考帧逐张比对。
> 以下为**实测后对指南的修正与补充**，实现者应优先参考本节。

### 10.1 复刻产物 ↔ 录屏帧对照表

| 实现截图（shots/） | 录屏参考帧 | 覆盖视图 | 还原度结论 |
|---|---|---|---|
| 01_market.png | [帧01] | Market 总览 | 布局/配色一致 |
| 02_heatmap.png / 02b | [帧02/03][放大z03] | 热力图 + 阈值筛选 + 悬停卡 | 色阶经 10.2 修正后一致 |
| 03_watchlist.png | [帧04] | 自选迷你 K 线宫格 | 一致 |
| 04_stock.png / 04b | [帧06][放大z05/z06/z07/z08] | 个股详情全要素 | 一致 |
| 05_financial.png | [帧07/08/09] | 财报 AI 卡 + 桑基 + 组合图 + 指标表 | 一致 |
| 06_options.png | [帧14/15] | 期权链 + P&L | 一致 |
| 07_quant.png | [帧17/18] | 代码编辑器 + 回测日志 | 一致 |
| 08/09/10_ai_*.png | [帧10/11/12] | AI 抽屉/对话/全屏 | 一致 |
| 11_cmdk.png | [帧19] | 命令面板 + 主题列表 | 一致 |
| 12_cn_theme.png | — | A股红涨绿跌主题（令牌互换验证） | 通过 |

### 10.2 实测修正（覆盖正文相应章节）

1. **热力图色阶：线性插值优于离散 9 档。** 正文 §4.2 的 9 档色阶在实现中发现色块间有可见跳变；改为
   `k = min(1, |chg|/6)` 在「深档 ↔ 亮档」之间线性插值（涨 `#0C6C60→#0DB397`，跌 `#78183C→#D7165E`），
   平盘灰区间收窄为 **±0.3%**（原 ±0.5% 会把 -0.41% 的 AVGO 误染成灰色，录屏中它是暗粉）。
   阈值 chips（≥2/4/6/8%）依然保留，仅作筛选不作着色档位。
2. **Treemap 分组必须先聚合 size。** `squarify(groups)` 入参的组对象需先计算
   `group.size = Σ items.size`，否则权重为 NaN 导致整图空白（实现时实际踩到）。
3. **组合图次坐标轴必须自适应数据范围。** 固定 55–70% 右轴会把 73.4% 的毛利率线画到画布外；
   应为 `lLo = floor(min)-2, lHi = ceil(max)+2`。
4. **桑基图节点缩放因子需全局统一且受标签行高约束。**
   `factor = min((H-24-(n-1)·30)/Σval, 2.6)`——分三行标签（名称/数值/同比箭头）需要每节点至少 ~44px 垂直空间，
   否则小节点（所得税 4.0B）标签互相叠字。

### 10.3 通用前端陷阱（与产品无关，但必踩）

1. **SVG 图标必须显式约束尺寸。** 在 flex 容器（输入框、命令面板）里放无 width/height 的内联 SVG，
   会被拉伸成巨型图形。统一 `.container svg { width:15px; height:15px; flex:none }`。
2. **正则做语法高亮必须用占位符法。** 先提取注释/字符串为占位符再替换关键字/数字，最后回填；
   顺序嵌套替换会污染已生成的 `<span>` 标签属性（实现时实际踩到，出现 `class="tok-str"` 残片）。
   占位符不能用数字（会被 `\d+` 规则二次命中），用字母序列。
3. **绝对定位的视图容器会让 grid `1fr` 行塌陷。** 视图切换用 `.view { position:absolute }` 时，
   父容器高度不能依赖内容——终端外壳必须用显式高度（如 `height:100vh`）而非 `min-height`，
   否则整个 1fr 主区域塌为 0，Canvas 全黑。
4. **Canvas 绘制要避开入场动画的 transform 期。** 视图入场 900ms 的 scale 动画期间
   `getBoundingClientRect()` 返回值被放大 ~3.5%，首帧绘制应延迟到动画结束或用 ResizeObserver 重绘。
5. **顶栏信息密度高，所有文本控件需 `white-space:nowrap; flex:none`**，否则 1320px 宽度下
   账户名/AI 按钮会被挤成竖排。

### 10.4 运行方式

```bash
# 零构建，直接打开
docs/UI/03-longbridge-terminal/demo/index.html
# 或本地服务
npx serve docs/UI/03-longbridge-terminal/demo
# 重新生成对比截图（需本机 Playwright + Chromium）
node docs/UI/03-longbridge-terminal/demo/shoot.js
```

