# 风格 A：GrokBot —— AI Agent 实时仪表盘（"夜里发光的机房"）

> 适用场景：AI Agent 集群监控、实时任务看板、运营驾驶舱、窄屏/移动端密集型 Dashboard。
> 核心气质：近黑底 + 粉紫氛围雾光 + 高饱和语义色指示灯；动态 100% 来自数据更新，没有一帧完全静止。

## 1. 设计思路（为什么这么设计）

1. **近黑底 + 单一光源感**：底色不用纯黑（纯黑"立刻变廉价"），用带青灰倾向的近黑，靠 8% 白色描边分层。
2. **颜色即身份**：一个实体/角色一种颜色（SCOUT=青、QUANT=紫、RISK=粉、YOU=黄），一色只服务一种语义，绝不混用。
3. **固定视口、零滚动**：所有动态感来自数据更新而非滚动，22 次位移检测全为 0px。
4. **事件驱动剧情编排**：每次事件 = 弹窗 → 列表新增 → 日志追加 → 数字跳变，四者同步演出。
5. **极高密度但有序**：小字号 + 大行高 + 等宽数字维持可读性，592px 窄屏仍塞三栏。

## 2. 配色（完整 token）

```css
/* 背景层级 */
--bg-page:  #0A0E10;   /* 页面底：近黑带青灰 */
--bg-card:  #0C0C18;   /* 主卡片 */
--bg-elev:  #12161F;   /* 浮起层 */
--bg-inset: #15181F;   /* 内嵌块 */
--bg-hover: rgba(255,255,255,.04);
--bg-chip:  rgba(255,255,255,.05);

/* 描边三档 */
--line:   rgba(255,255,255,.08);
--line-2: rgba(255,255,255,.14);
--line-3: rgba(255,255,255,.22);

/* 文字四级 */
--text-1: #F5F3FF;             /* 主文字（实测近 #FBF8FF） */
--text-2: #A9AEBC;
--text-3: #61667A;
--text-4: #4A5060;             /* 最暗：日志时间戳 */

/* 语义强调色（一色一语义） */
--green:  #52D89D;  /* 收入/健康 */
--cyan:   #41D2F3;
--pink:   #F25E97;  /* 支出/风险 */
--purple: #A489FA;
--amber:  #F6C157;  /* 待你拍板 */
--yellow: #E3CB72;  /* 人类行为 */
--red:    #F05959;
--blue:   #4C86F9;

/* 强调色派生：12% alpha 背景态 / 35–45% alpha 描边态 / 100% 文字 */
--green-bg:  rgba(82,216,157,.12);
--pink-bg:   rgba(242,94,151,.12);
--amber-bg:  rgba(246,193,87,.10);
--green-line: rgba(82,216,157,.35);
--amber-line: rgba(246,193,87,.45);
/* 推荐写法：color-mix(in srgb, var(--green) 12%, transparent) */

/* 辉光三件套 */
--glow-amber: 0 0 0 1px rgba(246,193,87,.38), 0 16px 44px -12px rgba(246,193,87,.30);
--glow-green: 0 0 22px -6px rgba(82,216,157,.55);
--glow-pink:  0 0 22px -6px rgba(242,94,151,.45);
--shadow-card: 0 24px 60px -24px rgba(0,0,0,.9), 0 0 0 1px rgba(255,255,255,.07);
```

**页面氛围雾光（少了这层页面会"死黑"）**：body 叠三层 radial-gradient——
```css
body {
  background:
    radial-gradient(620px 420px at 8% 22%,  rgba(242,94,151,.10), transparent 65%),   /* 左上粉 */
    radial-gradient(560px 380px at 96% 26%, rgba(190,110,240,.09), transparent 65%),  /* 右上紫 */
    radial-gradient(1200px 600px at 20% -10%, rgba(65,210,243,.05), transparent 60%), /* 顶部青 */
    var(--bg-page);
}
```

**铁律**：负值统一 `-` 前缀 + `--red`，与正值右对齐；一个色只服务一种语义。

## 3. 字体排版

```css
--font-ui:   'Inter','Space Grotesk',system-ui,'PingFang SC','Microsoft YaHei',sans-serif;
--font-mono: 'JetBrains Mono','IBM Plex Mono',Menlo,Consolas,monospace;

--fs-display: 32px;  --fw-display: 700;  --ls-display: -0.02em;  /* Space Grotesk 标题 */
--fs-bignum:  34px;  --fw-bignum:  700;                          /* KPI 大数字 mono */
--fs-label:   9.5px; --fw-label: 600; --ls-label: 0.14em;        /* 区块标签，uppercase */
--fs-row:    13px;   --fw-row:  500;
--fs-value:  15px;   --fw-value: 700;                            /* mono */
--fs-meta:   10px;
--fs-log:    10px;   /* line-height 1.55 */
--fs-chip:   11px;
```

- 统计区**数值在上、标签在下**。
- **必须开 `font-variant-numeric: tabular-nums`**——"数字在跳但不抖"的唯一原因。全局工具类 `.num, .mono, .tabular`。
- KPI 大数字发光：`text-shadow: 0 0 22px rgba(82,216,157,.35)`（同色 .35 alpha）。

## 4. 布局

```css
.wrap { max-width: 960px; }
.dashboard { display:grid; grid-template-rows: auto auto auto auto 1fr auto; gap:14px; }
.workspace { display:grid; grid-template-columns: 0.9fr 1.75fr 1.35fr; gap:16px; } /* ≈1:1.95:1.5，非三等分 */
.money-row { display:grid; grid-template-columns: 1fr 180px; gap:16px; }

--sp-1:4px; --sp-2:8px; --sp-3:12px; --sp-4:16px; --sp-5:24px;
--row-h: 54px;
--r-card:18px; --r-panel:12px; --r-row:8px; --r-btn:8px; --r-chip:999px;
```

- 卡片顶部加窗口装饰条（三色交通灯 `#FF5F57/#FEBC2E/#28C840` + 11px 标题），增强"真实产品"感。
- **卡片区隔不用间距，用行间 1px 分隔线**：`.row + .row { border-top: 1px solid rgba(255,255,255,.05); }`（日志行用 .04）。
- 响应式：断点压到 **380px 以下**才塌单列（视频原型就是 592px 窄屏三栏）；左栏窄至 ~120px，meta 必须 `nowrap + ellipsis`。

## 5. 核心组件配方

- **角色/实体图标**：每个实体一个几何形状（胶囊/圆/水滴 `border-radius:0 50% 50% 50%; rotate(45deg)`/clip-path 星花），复用同一母题；活跃态 `box-shadow: 0 0 0 6px rgba(色,.10~.18)`。
- **列表行**：方角行 + 行间 1px 线；**左侧 3px 分类色竖条** `border-left:3px solid var(--accent)`；右值 mono 700 且颜色=accent。
- **暗态预置行**：未来事件行从初始就 `opacity:.32` 占位，事件触发只点亮——列表高度零跳动。
- **Chip 胶囊**：高 26px、`padding:0 10px`、r=999px；四态（常规/待办=粉色系+`> YOU` 黄字/告警=琥珀系/开关）。
- **待审批态**：`border:1px dotted rgba(246,193,87,.55)`（dotted 非 dashed）+ `--amber-bg` + 琥珀小字 `pending your OK`。
- **非模态审批弹窗**：~320×150px，`background:rgba(12,12,24,.96); backdrop-filter:blur(8px)` + `--glow-amber`；**无遮罩不锁滚动**；按钮默认幽灵态（approve 描边更亮），**仅 hover 才翻成浅色实心深字** `bg:rgba(255,255,255,.92); color:#0C0C18`；`active{transform:scale(.97)}`。
- **进度/负载条**：高 4px、r=2px、底 `rgba(255,255,255,.07)`、max-width:78px（**不撑满**）；用 `transform:scaleX()` + `transition:.5s`（禁动 width）。
- **结算卡高亮行**：`--bg-inset` 内嵌块 + r=8px + 淡绿底 + 绿色 700 数值——整卡视觉落点。
- **日志控制台**：固定高 112px、开场即满、从底部插入、旧行 150ms 淡出移除；时间戳 `--text-4`；金额/数字用 `<b>` 提亮 `--text-1`。

## 6. 动效

```css
--ease-out:  cubic-bezier(.2,.9,.2,1);   /* 进出场首选，果断 */
--ease-soft: cubic-bezier(.4,0,.2,1);
--dur-micro:120ms; --dur-fast:180ms; --dur-base:260ms; --dur-slow:500ms;
```

- 数字插值：`cur += (target - cur) * 0.12` 每 100ms 更新（lerp 指数平滑），全局单 rAF。边界：lerp 只用于连续量（金额/比率/吞吐）；离散状态计数（待办数/在线数）若两处语义强绑定同显，应 snap 直写或把系数提到 0.35+，把收敛压进 300ms，避免收敛窗内两处不同值。
- 弹窗入场 180ms `opacity 0→1 + scale .96→1`（出现要 <66ms 感知，**不要做慢速放大**）。
- 行入场 `opacity 0→1 + translateY(-6px)→0` 260ms；chip 220ms。
- 状态点呼吸 `opacity .55↔1` 2s ease-in-out alternate；虚线环 `spin 14s linear infinite`。
- 告警脉冲 `pulse-ring: 0 0 0 0 → 0 0 0 8px transparent` 600ms×2。
- hover 提亮 `brightness(1.08)` 或背景 +.04 alpha。
- `prefers-reduced-motion` 全量降级。

## 7. 特色技法

1. **彩色点链代替实线**（本界面灵魂）：连线 = 沿贝塞尔路径排列的小圆点 r≈1.8–2px、间距 6px、颜色=数据流实体色，整链匀速流动、两端 10% 淡出；底轨 `stroke:rgba(255,255,255,.05)`。性能写法：路径静态 → `getTotalLength()` 预采样一次（SAMPLE_STEP=3），运行时只做数组索引 + 共用 drift，不每帧调 getPointAtLength。
2. **12% 色彩背景公式**：所有强调色派生 `.12` 背景 + `.35–.45` 描边 + 原色文字——暗底上彩色块不发脏的关键。
3. **SVG 端点圆用 HTML 绝对定位元素**（防 SVG 非等比拉伸成椭圆）：`box-shadow:0 0 12px rgba(82,216,157,.95)`。
4. **双折线同系叙事**：绿上粉下，两线垂直间距即衍生指标（利润），自带叙事省图例；**直段折线带折角，不平滑**。
5. **底部浅色窄条暗示下一屏**：卡片下方浅色区块 `#EFF0F3` 上叠 `blur(80px) opacity:.55` 光斑，只露 30–50px。

## 8. 易错点

底色勿纯黑 · 弹窗勿加遮罩 · 数字勿 setInterval 直写（要 lerp+rAF；离散计数 snap 或 0.35+ 快系数） · 粒子勿用 left/top 动画 · 连接线是彩色点链非灰实线 · 断点压 380px 以下 · SVG viewBox 比例贴近容器 · SVG 呈现属性不支持 `var()` 须内联 · 异步移除节点用快照数组防 while 死循环 · 容器别写 `overflow:hidden`（弹窗要向外溢出）。
