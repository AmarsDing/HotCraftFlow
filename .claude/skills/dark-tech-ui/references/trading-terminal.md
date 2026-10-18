# 风格 C：金融行情 Web 终端（"Bloomberg 的现代 Web 版"）

> 适用场景：交易终端、行情看板、多市场数据平台、量化回测界面——一切以**实时数字可信度**为核心的产品。
> 核心气质：深色 / 高密度 / 扁平 / 等宽数字 / 青涨玫红跌 / 圆角胶囊 / 微动效；用 1px 分隔线 + 色块语义分层。

## 1. 设计思路（金融 UI 的特殊约束）

1. **动画会让眼睛追不上价格**：实时数据**不做进出场动画**，只做 600ms 背景闪烁。装饰动效全部让给视图切换。
2. **主力字号仅 12px、行高 28–32px**：密度优先于呼吸感。
3. **实时信任基础设施**：底部状态栏常驻服务器时间 + 连接状态点；时区（EDT）/币种/复权状态**地毯式标注**——用户敢据此下单，全靠这些。
4. **明度分层替代投影**：背景四档明度递进，几乎不用 box-shadow。
5. **双轨制色彩**：涨跌色只表数据方向，**交互主色另设**（青 #0CA890），互不越界。

## 2. 配色

```css
/* 背景四档（明度分层，不用投影） */
#18191D  app 底      #1E2026  panel(+4%)     #22242A  浮卡      #141517  input(更暗)
hover rgba(255,255,255,.04)    active rgba(255,255,255,.07)
边框 #26282E / rgba(255,255,255,.06) / #34363D

/* 文字四级 */
#E6E8EB → #9AA0A6 → #62666E → #4A4D54      反色(亮底深字) #0D0E10

/* 涨跌（欧美惯例：青涨玫红跌） */
up:   #35C8A8 / #0CA890 / #0C6C60 / #0C846C / #0DB397 / #2BC4A8
down: #F0568C / #C42B6B / #78183C / #901848 / #B11955 / #D7165E
flat: #6B7280
最新价线独立蓝: #3E7BFA

/* A 股红涨绿跌：只需互换 up/down 变量 */
up: #D9342B  down: #16A34A   /* data-theme="cn-red-up" 已验证 */

/* 功能色 */
桑基: 收入 #5060F0 / 利润 #38C850 / 费用 #F09818
组合图: 柱 #3B82F6 / 线 #F59E0B
AI 渐变: #EC4899 → #F59E0B → #2DD4BF
成功: #22C55E
```

- **交互主色青 `#0CA890`**：选中、链接、Tab 下划线、按钮全用它；**仅 Buy/Sell 按钮允许大面积语义填充**。
- **热力图线性插值**（勿用 9 离散档）：`k = min(1, |chg|/6)`，涨 `#0C6C60→#0DB397`、跌 `#78183C→#D7165E`，平盘灰收窄至 ±0.3%（`#2A2C31`）。
- **闪烁只闪背景，不闪文字**：
  ```css
  @keyframes flashUp   { from { background: rgba(53,200,168,.28); } to { background: transparent; } }
  @keyframes flashDown { from { background: rgba(240,86,140,.28); } to { background: transparent; } }
  /* 行上应用，600ms 一次 */
  ```
- 主题切换：`data-theme` 换 CSS 变量，Canvas 图表 `getComputedStyle` 读回变量重绘。

## 3. 字体排版

```css
--font-sans: "Inter","HarmonyOS Sans SC","PingFang SC","Microsoft YaHei",system-ui,sans-serif;
--font-mono: "JetBrains Mono","SF Mono","Cascadia Code",Consolas,monospace;

--fs-micro:10px; --fs-caption:11px; --fs-body:12px;  /* 12px = 终端主力 */
--fs-body-lg:13px; --fs-title:15px; --fs-index:20px; --fs-price:22px; --fs-kpi:28px;
行高 1.2 / 1.45 / 1.6
```

- **数字铁律**：所有数字 `font-variant-numeric: tabular-nums` + `font-feature-settings:"tnum" 1`；另靠 `min-width:52px` 定宽列防抖。
- 价格/涨跌幅 600–700 字重、ticker 700、标签 400–500。
- 千分位 `toLocaleString('en-US',{minimumFractionDigits:2})`。
- 方向用 `▲▼` 三角字符，省空间且比箭头图标更"终端"。

## 4. 布局

```css
--h-winrow:30px; --h-brand:26px; --h-tabs:26px;   /* 顶栏三行 ≈ 80px */
--h-status:30px;                                   /* 底部状态栏 */
--w-rail:40px; --w-watch:310px; --w-quote:290px; --h-trade:220px;

个股页:  grid-template-columns: 310px 1fr 290px;  rows: 1fr 220px;
Market:  260px 1fr; gap:12px;
自选宫格: repeat(auto-fill, minmax(280px, 1fr));

--sp-1..6: 4 8 12 16 20 24px;   面板 padding 12–16px; 模块间距 8–12px;
分隔线 1px rgba(255,255,255,.03–.06)
```

- 响应式：900–1135px 报价栏折为底部 Tab；<900px 自选列变抽屉。
- 终端骨架 = 顶栏三行（窗控/logo+全球市场指数/主 Tab）+ 左侧 40px 图标 rail + 三栏主区 + 底部状态栏。

## 5. 核心组件配方

- **行情列表**：行 grid `46px 14px 1fr auto auto`（logo/状态点/名称/价格/涨跌）；新行 `rowIn 200ms translateY(-6px)`；当前行 `--bg-active` + 左侧 2px 青竖条。
- **K 线**：蜡烛体宽 = 档位宽×0.62、影线 1px；成交量子图高 16%、alpha .75 随蜡烛色；OHLC 读数条 10px；复权切换保视口。
- **热力图**：两层 squarified treemap，`paddingOuter(3) paddingTop(19) paddingInner(2)`，瓦片 r=2px，面积分级显字（>12000px² 全要素 />3000px² ticker+%）；hover 浮卡 min-width 190px + r=10px + `::after` 45° 小三角。
- **报价栏**：大价格 22px/700；KV 双列（label 11px 灰 / value tabular）。
- **分裂压力条**（多空对抗）：`height:8px; border-radius:999px`，左青右玫红，正中白字 9px 带 text-shadow——**可套一切对抗型双指标**。
- **期权链**：Strike 列 sticky 居中吸顶；ATM 行 `rgba(12,168,144,.07)` 淡青底；当前到期日 chip 底部 2px 青条。
- **交易面板**：输入框高 26px、深底 + 1px 描边 + 内嵌 26px 步进钮；Buy/Sell 全圆角实心胶囊（买入青 #0CA890 / 卖出玫红）。
- **选中态三套路**（全局统一）：① 描边胶囊 `color/border: #0CA890 + rgba(12,168,144,.08) 底`；② 下划线 2px 青条；③ 侧竖条 2px。

## 6. 动效

```css
--ease-out-expo: cubic-bezier(0.22,1,0.36,1);
--ease-in-out:   cubic-bezier(0.4,0,0.2,1);
--dur-view:900ms; --dur-panel:800ms; --dur-pop:200ms;
--dur-toast:300ms; --dur-flash:600ms; --dur-hover:120ms;
```

- **视图形变**：enter `scale(1.035) + blur(7px) + opacity 0→1`，exit `scale(0.965) + blur(7px)`，各 900ms（60% 处 opacity 即归位）——统一复用到所有视图切换、桑基 Zoom In 沉浸模式。
- **AI 抽屉**：800ms 滑入 + 主内容 `scale(.97) + brightness(.85)` 同步退后。
- Toast `translateY(-12px)` 300ms 入，3s 淡出；浮卡 120ms fade + translateY(4px)。
- 输入光标 1px 530ms step-end；加载省略号三点呼吸间隔 .2s。
- 原则：只动 transform/opacity/filter；reduced-motion 统一降到 150ms。

## 7. 特色技法

1. **tick 闪烁只闪背景**（§2），600ms 一次，不做颜色渐变。
2. **最新价线**：画布上 `setLineDash([3,3])` + `#3E7BFA` 虚线 + 右端 16px 高蓝底白字 r=2 标签；十字光标 `rgba(255,255,255,.35)` 虚线 + 灰底 `#3A3D44` 标签。
3. **Sparkline 末端渐变淡出**：填充 `col+'40' → col+'00'`（hex alpha 拼接），主线 1.3px。
4. **热力图分组 size 先 Σ 聚合**再画，否则 NaN 空白（实踩坑）。
5. **网格** `rgba(255,255,255,.05)` + `setLineDash([2,3])`；zebra 行 `rgba(255,255,255,.022)`。
6. **桑基图**：渐变流 `stop-opacity .35→.5`；纵向缩放 `factor = min((H-24-(n-1)*30)/Σval, 2.6)`。
7. **AI 签名三件套**：3px 三色渐变下划线 + ✚按钮 `aiPulse 2.6s` 光环 `box-shadow:0 0 0 3px rgba(12,168,144,.12)` + 思考文案随机化。
8. **组合图右轴自适应**：`lLo=floor(min)-2, lHi=ceil(max)+2`。
9. **E/D 财报事件徽章** 6px 圆点 + 连接状态点 `box-shadow:0 0 0 3px rgba(34,197,94,.15)`。

## 8. 易错点（实踩修正）

- 容器高度陷阱：absolute 定位视图在 `height:100vh` 下 grid 1fr 会塌——给显式高度。
- scale 动画期间 `getBoundingClientRect` 膨胀 ~3.5%——用 ResizeObserver 延迟重绘 Canvas。
- DPR 适配用 `setTransform(dpr,0,0,dpr,0,0)` 而非 scale。
- 行内 SVG 必须显式尺寸 + `flex:none`，否则被 flex 压没。
- 顶栏指数条 `nowrap + flex:none` 防换行塌行。
- 正则高亮先替换字母占位符再高亮，防关键字二次命中 HTML 标签。

## 9. 可复用模式

语义色双轨 + CSS 变量主题 · 等宽数字铁律 + 定宽列 · 四档背景明度分层替代投影 · 闪烁只闪背景 600ms · 选中态三套路 · 分裂压力条套一切对抗指标 · 视图形变 scale+blur 900ms 统一复用 · 主内容 scale(.97)+brightness(.85) 退后 · 底部时间锚 + 连接点 = 实时信任基础设施。
