# 视频 UI 深度拆解与前端复刻指南
### 案例：Grok Bot — AI Agent 集群「通宵值班」实时仪表盘

> 本文档基于对参考视频（592×1280 / 30fps / 11.10s / 333 帧）逐帧取样、像素取色、帧间差分与动画时序测量得出。
> 目标：**任何前端工程师或 AI 编码代理读完本文件后，可以独立还原出同等级别的界面与动效。**

**修订记录**

| 版本 | 说明 |
|---|---|
| v1 | 基于视频逐帧分析的首版规格 |
| **v2** | **按 `grokbot-demo.html` 的实际渲染结果与视频做逐块对照，修正 21 项偏差**（见 [第 10 章](#10-与视频的逐项校验记录v2-校正)）。其中影响最大的是：网络图连线实为**彩色点链**（非灰曲线）、响应式断点必须在 380px 以下、统计行顺序与角色图标形状。**阅读时若发现正文与第 10 章冲突，以第 10 章为准。** |

---

## 0. 一句话结论

这是一个 **「深色 + 高饱和强调色」的实时数据仪表盘**，核心观感由 5 类手法支撑：

1. **近黑底色 + 单一光源感**：页面底色不是纯黑，而是带青灰倾向的 `#0A0E10`，卡片 `#0C0C18`，靠 8% 白色描边分层；
2. **语义化强调色**：一个角色一种颜色（侦察=青、量化=紫、风控=粉、路由=粉、人=黄），颜色即身份；
3. **连续微动效**：数字滚动、**彩色点链沿贝塞尔曲线流动**、状态点呼吸、折线生长——**没有一帧是完全静止的**；
4. **事件驱动的「剧情」**：弹窗审批 → 列表新增行 → 日志追加 → 数字跳变，四者同步构成一个完整事件；
5. **信息密度极高但有序**：全部依赖「小字号 + 大间距行高 + 等宽数字」维持可读性。

---

## 1. 视频取证信息（measurable facts）

| 项目 | 实测值 | 说明 |
|---|---|---|
| 分辨率 | 592 × 1280 | 竖屏，手机录屏（顶部为 iOS 状态栏 `22:07`） |
| 帧率 / 时长 | 30 fps / 11.10 s / 333 帧 | — |
| 是否滚动 | **否**（逐帧位移检测 22 次，shift 全为 0px） | 视口固定，靠数据更新制造"动"的错觉 |
| 弹窗出现耗时 | **< 66 ms**（逐帧确认 2.433s 无 → 2.500s 满显） | 接近瞬时，实质是「状态切换」而非「入场动画」 |
| 高频变化区 | 顶部数字区（帧间像素变化率 11%~21%）、网络图区（4%~6%） | 低频区：图表、日志、右侧卡片（每 1~2s 才更新一次） |

**关键结论：不要给这个界面加"滚动叙事"。** 它是一块**固定视口的实时看板**，动态感 100% 来自数据更新与粒子动效。

---

## 2. 整体结构（从外到内 5 层）

```
┌ iOS 状态栏 22:07 ─────────────────────────┐  ← 录屏外壳，非 UI
│ 页面背景（近黑 + 极淡渐变/噪点）            │
│ ┌ 主卡片 rounded-18 border-1px ─────────┐ │
│ │ ① 窗口装饰条：三色交通灯 + 标题文字      │ │  ← 模拟 App 窗口，增强"真实产品"感
│ │ ② 页头：头像 + Grok Bot + 副标题        │ │
│ │        右侧：巨大绿色利润数字 + 说明     │ │
│ │ ③ 统计行（4 项：Tokens / tok-s / 主机 / 告警）│
│ │ ④ 动态胶囊 Chip 行（状态 + 待办 + 告警）  │ │
│ │ ⑤ 五列角色头（SCOUT QUANT RISK ROUTER YOU）│
│ │ ⑥ 主工作区 三栏：                       │ │
│ │    左 CLIENTS 列表 │ 中 网络图+弹窗 │ 右 GPU NODES │
│ │ ⑦ MONEY 区：折线图 + 图例               │ │
│ │ ⑧ 日志控制台（等宽彩色标签） + 右侧结算卡 │ │
│ └────────────────────────────────────────┘ │
│ 下方：浅色区块（大模糊彩色光斑，露出顶部一截）│
└────────────────────────────────────────────┘
```

**布局骨架（推荐实现）**

```css
.dashboard{
  display:grid;
  grid-template-rows: auto auto auto auto 1fr auto;   /* 头/统计/chip/角色头/工作区/钱与日志 */
  gap:14px;
}
.workspace{
  display:grid;
  /* 实测比例 ≈ 1 : 1.95 : 1.5 —— 左栏最窄，中栏最宽，右栏居中（不是对称三等分） */
  grid-template-columns: 0.9fr 1.75fr 1.35fr;
  gap:16px;
  align-items:stretch;
}
.workspace > section{ display:flex; flex-direction:column; min-width:0; }
.money-row{
  display:grid;
  grid-template-columns: 1fr 180px;          /* 图表 : 结算卡 */
  gap:16px;
}
```

> 设计稿宽度建议 **1200px（桌面）** 或 **592px（移动，即视频原尺寸）**；视频中为移动宽度下仍塞进三栏，靠 9~10px 字号压缩信息。

---

## 3. 设计令牌 Design Tokens（像素实测取色）

### 3.1 颜色

> 页面级还有一层**粉紫氛围光**：`body` 上叠两个巨大的 `radial-gradient`
> （左上 `rgba(242,94,151,.10)`、右上 `rgba(190,110,240,.09)`，各约 600×420px，`transparent 65%`）。
> 少了这层，页面会显得"死黑"，与视频的那种"夜里发光的机房"气质差很远。

| 令牌 | 实测 HEX | 用途 |
|---|---|---|
| `--bg-page` | `#0A0E10`（取样 `#101A1B`，含渐变） | 页面底色，带青绿倾向的近黑 |
| `--bg-card` | `#0C0C18` | 主卡片底 |
| `--bg-elev` | `#12161F` | 行/卡片悬浮层 |
| `--bg-inset` | `#15181F` | 内嵌块（如利润行底） |
| `--line` | `rgba(255,255,255,.08)` | 常规分隔线 |
| `--line-2` | `rgba(255,255,255,.14)` | 强分隔线 |
| `--text-1` | `#F5F3FF` | 主文字（实测近白 `#FBF8FF`） |
| `--text-2` | `#A9AEBC` | 次要文字 |
| `--text-3` | `#61667A` | 时间戳 / 单位 / 弱化 |
| `--green` | **`#52D89D`** | 利润、通过、在线、scout |
| `--cyan` | **`#41D2F3`** | 侦察、均价、chat 类客户端 |
| `--pink` | **`#F25E97`** | 风控、路由、GPU 支出、告警 |
| `--purple` | **`#A489FA`** | 量化 QUANT |
| `--amber` | **`#F6C157`** | 待审批（边框/标题/告警 chip） |
| `--yellow` | `#E3CB72` | YOU（人类决策）、doc.search |
| `--red` | `#F05959` | 拒绝 / 罚金 / 负值 |
| `--blue` | `#4C86F9` | GPU 负载条（主色） |

**颜色使用铁律**
- 一个色只服务一种语义，**绝不混用**。绿=收入/健康，粉=支出/风险，琥珀=待你决策，黄=人类行为。
- 所有强调色在深色底上必须 **降饱和后用作背景**（`color-mix(in srgb, var(--green) 12%, transparent)`），文字则保持高亮原色。
- 负值统一带 `-` 前缀并用 `--red`，且**与正值右对齐**（`font-variant-numeric: tabular-nums`）。

### 3.2 字体

| 层级 | 推荐 | 尺寸 / 字重 / 字距 |
|---|---|---|
| 显示标题 | `Space Grotesk`（或 `Inter Tight`） | 32px / 700 / -0.02em |
| 大数字 | `Space Grotesk` + `tabular-nums` | 34px / 700 |
| 区块标签 | `Inter` | 9.5px / 600 / **+0.14em** / `uppercase` |
| 行标题 | `Inter` | 13px / 500 |
| 行数值 | `JetBrains Mono`（或 `IBM Plex Mono`） | 15px / 700 |
| 元信息 | `Inter` | 10px / 400 / `--text-3` |
| 日志 | `JetBrains Mono` | 10px / 400 / line-height 1.55 |
| Chip | `Inter` | 11px / 500 |

**必须开启 `font-variant-numeric: tabular-nums`** —— 这是"数字在跳但不抖"的唯一原因。

### 3.3 圆角 / 间距 / 阴影

```css
--r-card:18px; --r-panel:12px; --r-chip:999px; --r-btn:8px; --r-row:8px;
--sp-1:4px; --sp-2:8px; --sp-3:12px; --sp-4:16px; --sp-5:24px;
--row-h:54px;
--shadow-card: 0 24px 60px -24px rgba(0,0,0,.9), 0 0 0 1px rgba(255,255,255,.07);
--glow-amber: 0 0 0 1px rgba(246,193,87,.38), 0 16px 44px -12px rgba(246,193,87,.30);
--glow-green: 0 0 22px -6px rgba(82,216,157,.55);
```

---

## 4. 组件解剖（逐组件给出规格）

### 4.1 页头（Header）

```
[头像]  Grok Bot                    +$567.70
        ● night shift · 03:26       profit tonight · of the $1,000 budget
```

- **头像**：白色圆形 `#FBF8FF`（直径 ~44px），内含 2 条深色圆角竖条（"眼睛"），是整套图标系统的**母题**——所有角色图标都是「彩色圆形 + 两条深色竖条」的变体，形成强烈的家族感。
- **副标题**：`● night shift`（绿色，`--green`）+ ` · 03:26`（白色等宽）。绿点带 2s 呼吸动画。
- **利润数字**：34px / 700 / `--green` / 带 `text-shadow: 0 0 22px rgba(82,216,157,.35)` 发光。
- **说明行**：10px `--text-3`。
- **统计行**：**与利润块同处右侧一栏**（不是独立的整宽一行），4 项等宽分布，且**【数值在上、标签在下】**：
  数值 `20~22px mono 700 --text-1`，标签 `9.5px uppercase --text-3`。
  内容：`196.2M` / `14.7k` / `5/8` / `0`，对应标签 `tokens served` / `tok/s now` / `nodes on` / `overnight`
  （视频末尾数值变为 `377.7M` / `17.7k` / `8/8`）。

### 4.2 Chip 胶囊行

四类形态：

| 类型 | 样式 | 示例 |
|---|---|---|
| 常规状态 | `bg rgba(255,255,255,.05)` + `text-2` | `scanning 14 clusters`、`blended $1.87 / 1M`、`watching 5 hosts` |
| 待办（需你决策） | `bg rgba(242,94,151,.12)` + `border 1px rgba(242,94,151,.35)` + 文字 `--pink` + 末尾 `> YOU` | `need Dallas + ask YOU` |
| 告警 | `bg rgba(246,193,87,.12)` + 边框 `.4` + 文字 `--amber` | `Kill - price spiked 2x` |
| 开关（弱化） | 两个相邻暗胶囊，选中项提亮 | `asleep` / `on call` |

- 高 26px，圆角 999px，`padding: 0 10px`，`gap: 8px`，**可横向换行**（`flex-wrap`）。
- 出现动画：`opacity 0→1` + `translateY(-4px)→0`，220ms `cubic-bezier(.2,.9,.2,1)`。

### 4.3 角色列头（5 列）

```
 ①       ②       ③       ④        ⑤
SCOUT   QUANT    RISK    ROUTER    YOU
 蓝      紫       粉      粉星      白
```

- 图标**五个形状各不相同**（这是"像不像"的分水岭，极易做错），但都复用「两条深色圆角竖条」的眼睛母题：

| 角色 | 形状 | 底色 | 备注 |
|---|---|---|---|
| SCOUT | **横向胶囊（pill，约 38×26）** | `#4AD8F5` **青色** | 不是圆，也不要写成蓝色 |
| QUANT | 正圆（30px） | `--purple` | |
| RISK | **水滴形，尖朝上**（`border-radius:0 50% 50% 50%` + `rotate(45deg)`） | `#F0516B` 粉红 | 不是圆 |
| ROUTER | **粉红星花/齿轮形**（`clip-path` 16 点多边形） | `--pink` | **带粉色外发光** `drop-shadow` |
| YOU | 正圆 | `#FBF8FF` 白 | 内嵌深色 `you` 字样 |

- 图标 30px（SCOUT 为 38×26），标签 9.5px uppercase。
- 标签：9.5px / 600 / `+0.14em` / uppercase / `--text-3`；**当前活跃角色时标签与图标提亮 + 加一圈 `box-shadow: 0 0 0 6px rgba(色,.10)`**。
- 左侧写 `CLIENTS`、右侧写 `GPU NODES` 作为列区标签（同为 9.5px uppercase）。

### 4.4 客户端列表行（左栏）

```
┃ chat.app              3.2k        ← 左侧 3px 彩色竖条（分类色）
  11.3k tok/s · $1.99 / 1M           ← 元信息 10px --text-3
```

- 行高 ~52px，`padding: 9px`；**实测是"方角行 + 行间 1px 分隔线"，不是卡片式圆角块**：
  `.row + .row{ border-top:1px solid rgba(255,255,255,.05) }`，hover 时 `background: rgba(255,255,255,.04)`。
- 左栏在窄屏下只有 ~120px 可用宽，**meta 必须单行 + 省略号**（`white-space:nowrap; text-overflow:ellipsis`），否则会折行破坏节奏。
- **左侧 3px 竖条**是本组件的灵魂：`border-left: 3px solid var(--accent)`，颜色由客户端类型决定（chat=青、copilot=蓝、support=绿、doc.search=黄、game NPCs=粉、translator=紫）。
- 右侧数值 15px mono 700，颜色 = 该行 accent。
- 副行元信息 10px `--text-3`，用 `·` 分隔。
- **新增行动画**：`height 0→54px` + `opacity 0→1` + `translateY(-6px)→0`，260ms。
- **审批后徽章**：右侧出现 `✓ approved`（绿）或 `declined 99.99% SLA`（红/灰）。
- **暗态占位行（关键）**：列表从一开始就包含"未来事件"的行位（如 `translator`、`voice agent`），以 `opacity:.32`
  的暗态存在，事件发生后才**逐条点亮**——不是等事件来了才 push 新行。给每行一个 `st:'ok' | 'dim'` 状态位，
  事件触发时改状态重渲染即可。

### 4.5 GPU 节点列表行（右栏）

```
● N1 · Oslo
  llama-70b · $12.40/h      [████▌] 2.9k
```

- 状态点 6px：`--green`（在线）/ `--amber`（待批）/ `--red`（已下线），带 2s 呼吸（`opacity .55→1`）。
- **行内结构（实测）**：第一行 = `状态点 + 名称` → `负载条` → `数值`（三者同一行，条居中）；第二行 = `meta`。
  不是"名称+数值一行、条另起一行"。
- 负载条：高 4px，`border-radius 2px`，底 `rgba(255,255,255,.07)`，填充色 `--blue` / `--amber` 交替；
  **长度有上限（约 70~78px）**，不要撑满整行；用 `transform: scaleX()` 变化并配
  `transition: transform .5s cubic-bezier(.2,.8,.2,1)`（**不要动 width，避免重排**）。
- **待审批态**：整块 `border: 1px dotted rgba(246,193,87,.55)`（**是点线，不是虚线**）+ `background: rgba(246,193,87,.08)`，
  方块圆角、略微缩进；名称后跟琥珀小字 `pending your OK`，状态点转琥珀色。
  批准后用 250ms 过渡"转正"为普通实线行（实测就是同一条行位改状态，不是插入新行）。

### 4.6 中央网络图（本界面的灵魂）

> ⚠️ **这里是初版分析最严重的误判，务必按下面的实测结论做。**
> 连线 **不是**"灰色曲线 + 上面跑几个彩色粒子"，而是 **整条线本身就是一串彩色小圆点（点链 / dotted chain）**，
> 点链整体沿路径流动。放大 4 倍看视频可以清楚看到"珠链"结构，而不是实线。

- 画在一块 `1fr` 宽的 SVG 上，`viewBox` 比例要**贴近容器比例**（推荐 `0 0 400 300`，实际容器约 1.3~1.5:1）。
  若比例差太多 + `preserveAspectRatio="none"`，曲线会被纵向压平，弧度全丢。
- **连线（5 条入边 + 5 条出边）**：
  - 路径为三次贝塞尔 `M x1 y1 C x1+dx y1, x2-dx y2, x2 y2`，控制点水平外扩 **~50%** 距离（实测弧度较大，呈扇形收拢）。
  - 路径本身只画一条极淡底轨 `stroke: rgba(255,255,255,.05)`。
  - **真正的"线"是沿路径排列的一串小圆点**：`r ≈ 1.8~2px`，点间距 **≈ 6px**，颜色 = 该数据流实体的语义色
    （与左右列表的 accent 一致：chat.app 青、doc.search 黄、game NPCs 粉、GPU 节点蓝/琥珀…）。
  - 点链**整体沿路径匀速流动**，走完全程约 **1.2~1.6s**；靠近曲线两端做透明度淡出（fade 区间约两端各 10%）。
- **中心节点**：圆形 `r ≈ 27`，`fill: #12161F`，**`stroke: var(--pink)` 粉色描边 + 粉色外发光**
  （`filter: drop-shadow(0 0 10px rgba(242,94,151,.75))`）；外圈还有一道**粉色点线环** `stroke-dasharray:3 5`
  并 `animation: spin 14s linear infinite`；中央是数值 `13~15.1k`（mono 700 `--text-1`），**下方还有一行 `TOK/S` 小字**。
  节点位置为区域**垂直居中略偏上**，审批弹窗压在它下方一点点、两者轻微重叠。

**点链的性能写法（推荐）**：路径是静态的，所以**预采样一次**、运行时只做数组索引——
不要在每帧里调 `getPointAtLength()`：

```js
// ① 初始化：预采样路径点
const len = path.getTotalLength();
const SAMPLE_STEP = 3, DOT_GAP = 6;           // DOT_GAP 取 SAMPLE_STEP 的整数倍 → 点距严格均匀
const samples = [];
for (let d = 0; d <= len; d += SAMPLE_STEP) samples.push(path.getPointAtLength(d));
const N = samples.length, perDot = DOT_GAP / SAMPLE_STEP;

// ② 撒点：每个点记录自己在采样表里的基准下标
const dots = [];
for (let k = 0; k < Math.floor(N / perDot); k++){
  const c = document.createElementNS(SVGNS, 'circle');
  c.setAttribute('r', '1.8');
  c.style.fill = col; c.style.color = col;                       // SVG 呈现属性不支持 var()
  c.style.filter = 'drop-shadow(0 0 2px currentColor)';
  g.appendChild(c);
  dots.push({ el: c, base: Math.round(k * perDot) });
}

// ③ 每帧：所有点共用一个漂移量 → 整条链一起流动
let drift = 0;
function step(dt){
  drift += dt * 0.030;                        // 采样点/毫秒
  const off = Math.floor(drift);
  for (const d of dots){
    const idx = ((d.base + off) % N + N) % N;
    const pt = samples[idx];
    d.el.style.transform = `translate(${pt.x.toFixed(1)}px,${pt.y.toFixed(1)}px)`;
    d.el.style.opacity = fade[idx];           // 端点淡出表，预先算好
  }
}
```

> 备选：给每个点设 `offset-path: path("...")` + 统一动画、再用 `animation-delay: -k*Δ` 错开相位，
> 可做到零 JS 每帧开销；但 SVG 子元素上的 `offset-path` 行为在各内核不完全一致，JS 采样表更稳。

### 4.7 审批弹窗（Modal / Action Card）

```
╭───────────────────────────────╮   ← 1px 琥珀边框 + 外发光，圆角 ~14px
│ RENT 8×H100 · Paris           │   ← 14px 700 **白色**（琥珀只用于边框与状态词）
│ $12.72/h · till 07:00         │   ← 12px text-1
│ queue 2.4k tok/s · RISK: PASS │   ← 10px text-3，RISK 用 amber 提亮
│                               │
│  ┌ approve ┐  ┌ deny ┐        │
╰───────────────────────────────┘
```

- **非模态**：不加遮罩、不锁滚动，浮在网络图中央偏下，`z-index` 高于连线。
- 尺寸 ~320×150px（**比中栏还宽，会轻微溢出到左右栏上方**，所以容器 `overflow` 不能是 `hidden`），
  `--r-panel`，`background: rgba(12,12,24,.96)` + `backdrop-filter: blur(8px)`，`--glow-amber`。
- **按钮三态（易错）**：
  - **默认态**：`approve` 与 `deny` **看起来是同一种幽灵按钮** —— 暗底 + 描边 + 白/灰字。
    `approve` 的描边更亮：`bg rgba(255,255,255,.06)` / `border rgba(255,255,255,.42)` / `color --text-1`。
  - **`approve:hover` 才翻成浅色实心深字**：`bg rgba(255,255,255,.92)` / `color #0C0C18`。
    （初版把 hover 态误当成默认态了 —— 视频里 1.0s 那一帧之所以"白得发亮"，正是因为鼠标已经悬停在上面。）
  - `active`：`transform: scale(.97)`（120ms）
- **点击后**：`approve` → 变为 `✓ approved`（`--green`，淡入 200ms）；整体 300~400ms 后 `opacity→0 + scale(.98)` 移除。
- **实测时序**：点击 → approved 状态 ≤ 130ms；弹窗消失在 approved 后约 300ms；**弹窗出现几乎是瞬时的（<66ms）**，因此入场动画建议控制在 150~200ms，不要做慢速放大。

### 4.8 MONEY 区（折线图）

- **图例是竖排两行**（`display:flex; flex-direction:column`），紧贴 `MONEY` 标签下方、图表左上角：
  `● token revenue`（`--green`）/ `● GPU rent`（`--pink`），10px。**不是横排**。
- **两条折线共用同一坐标系**，绿线在上、粉线在下，**两线之间的垂直间距就是"利润"** —— 设计巧思，无需额外图例。
  实测 GPU rent 约为 revenue 的 **30% 左右**，所以粉线压在下部约 70% 高度处。
- **折线是直段连接（有明显折角），不要做贝塞尔平滑** —— 视频里就是一段段斜线。
- **末端有当前值发光圆点**：绿线末端一个 8px 圆点 + `box-shadow: 0 0 12px rgba(82,216,157,.95)`。
  实现技巧：**这个点不要放在等比例拉伸的 SVG 里**（会被拉成椭圆），用 HTML 绝对定位元素、按百分比定位即可。
- 面积填充：`linearGradient` 从上到下 `rgba(82,216,157,.18) → transparent`（很淡）。
- 数据每 1s 追加一点，超过 ~40 点后左侧移出（滚动窗口）。

### 4.9 日志控制台（Log Console）

```
01:04  you      denied voice agent — $500 penalty
01:22  risk     PASS Dallas host · uptime 99.9%
01:40  scout    found H100×8 @ $11.20/h
02:03  router   routing 15.3k tok/s → 5 hosts
02:31  you      approved N6 · Dallas
03:00  scribe   checkpoint · 191.3M tokens · +$553.88 so far
```

- 等宽字体 10px / line-height 1.55；固定高度容器（约 5~7 行，**开场就是满的**，不是空容器），
  **新行从底部插入**，超出后最老一行先 `opacity→0` 再移除（150ms）。
- **行之间也有 1px 细分隔线**（`.lrow + .lrow{ border-top:1px solid rgba(255,255,255,.04) }`）。
- 新行动画：`opacity 0→1` + `translateY(6px)→0`，260ms `ease-out`。
- 时间戳 `--text-4`（最暗，约 `#4A5060`），**标签按角色着色（实测，注意 router 的颜色）**：
  `you=--yellow`、`risk=--pink`、**`router=#C08BF0`（粉紫，不是青色）**、`scout=#5BD8B0`、`scribe=#5BD8B0`、`quant=--purple`。
- 消息文本 `--text-2`，其中**金额/数字片段用 `--text-1` 提亮**（用 `<span class="num">` 包裹）。

### 4.10 右侧结算卡

```
token revenue     $794.56     ← text-1
GPU rent         -$226.66     ← --red
avg $ / 1M         $4.95      ← --cyan
profit            $567.70     ← --green，位于 --bg-inset 内嵌块中，圆角 8px
```

- 两列布局 `label` / `value`，`justify-content: space-between`，行高 22px。
- 卡片标题实测为 **`TONIGHT`**（不是 MONEY），标签依次为 `token revenue`（白）/
  `GPU rent`（或 `GPU sent`，视频中两种措辞都出现过）/ `avg $ / 1M`（青）/ `profit`。
- **最后一行（profit）用内嵌块 `--bg-inset` + 圆角 8px + 淡绿底 + 绿色数值**，是整卡的视觉落点。

### 4.11 卡片下方的浅色区块

深色卡片结束后，下方是一段**浅色背景（≈#EFEFF3）的区块**，上面漂浮着 **超大、强模糊（blur ~80px）的彩色光斑**（黄、青、粉），
**只露出顶部约 30~50px 的一窄条**（不是一大块！初版按 150px 做会明显偏大）。
这是落地页的「下一屏」——作用是**制造"页面还在继续"的暗示**，并给纯黑底部一个柔和的过渡。实现：`filter: blur(80px)` 的绝对定位圆形 div + `background: radial-gradient(...)`，`opacity: .55`。

---

## 5. 动效总表（Animation Spec）

| # | 动效 | 元素 | 关键参数 | 实现方式 |
|---|---|---|---|---|
| A1 | 数字滚动 | 利润 / Tokens / tok-s | 每 100ms 更新，`lerp` 到目标值；`tabular-nums` | rAF + 插值函数 |
| A2 | **点链流动** | 网络图连线 | 每条边约 30~40 个 2px 点、间距 6px，整链沿路径流动，全程 1.2~1.6s | 预采样 `getPointAtLength` + 数组索引 |
| A3 | 虚线环旋转 | 中心节点 | `14s linear infinite` | CSS `@keyframes spin` |
| A4 | 状态点呼吸 | 在线圆点 | `opacity .55→1`，`2s ease-in-out infinite alternate` | CSS |
| A5 | 弹窗入场 | Action Card | `opacity 0→1` + `scale .96→1`，**180ms** `cubic-bezier(.2,.9,.2,1)` | CSS transition |
| A6 | 按钮反馈 | approve/deny | `active{scale:.97}` 120ms；`hover{brightness:1.08}` | CSS |
| A7 | 审批完成 | 按钮 → 徽章 | 文字淡出 → `✓ approved` 绿色淡入，200ms | JS 类切换 |
| A8 | 弹窗退场 | Action Card | 300ms 后 `opacity→0 + scale .98`，220ms | JS + transitionend 移除 |
| A9 | 日志追加 | 控制台 | `opacity 0→1 + translateY(6px)→0`，260ms ease-out | JS 插入节点 |
| A10 | 列表新增 | 客户端/节点行 | `height 0→54px` + `translateY(-6px)→0`，260ms | JS + CSS |
| A11 | 待办转正式 | 虚线块 | `border/background` 250ms 过渡 | CSS transition |
| A12 | 负载条变化 | GPU 行 | `transform: scaleX()`，`.5s cubic-bezier(.2,.8,.2,1)` | CSS transform |
| A13 | 告警脉冲 | RISK 图标 / 告警 chip | `box-shadow 0 0 0 0 → 0 0 0 8px transparent`，600ms ×2 | CSS keyframes |
| A14 | 图表生长 | 折线 | 直段折线（不平滑），每 1s 追加一点；末端 8px 发光圆点跟随 | SVG `path` + HTML 定位点 |
| A15 | Chip 出现 | 胶囊行 | `opacity 0→1 + translateY(-4px)→0`，220ms | CSS |

**统一缓动**：`--ease-out: cubic-bezier(.2,.9,.2,1)`（果断、快出慢收）；`--ease-soft: cubic-bezier(.4,0,.2,1)`（常规过渡）。
**统一时长**：微交互 120~180ms；内容增删 220~300ms；装饰循环 2~14s。

---

## 6. 事件编排（Choreography）—— 视频 11 秒剧本

这是让界面"像活的一样"的核心：**每次事件都是一次多组件同步演出**。

| 时刻 | 事件 | 联动组件 |
|---|---|---|
| 0.0s | 初始态 | 5 客户端 / 5 GPU 节点(N1-N5) / Dallas 租赁弹窗已悬浮 |
| ~1.0s | 光标移到 `approve`，按钮进入 hover→pressed | A6 |
| 1.2~1.6s | `approved N6 · Dallas` | A7→A8→A9（日志）→A10（GPU 列表新增实线行）→A11→A1（数字跳变） |
| 2.45s | `SIGN – translator` 弹窗出现 | A5 |
| ~3.4s | 点击 approve → `approved` | 同上链路；左栏新增 `translator` 行（紫色 accent） |
| 4.3s | `Kill - price spiked 2x` 告警 chip 出现，RISK 图标脉冲 | A13 + A9（日志 `Kill Warsaw - price spiked 2.1x in 1h`） |
| 7.5s | `RENT 8×H100 · Reno` 弹窗 → 8.5s approved → `N7 · Reno` | 完整链路 |
| 9.5s | `RENT 8×H100 · Paris` 弹窗 → 10.5s approved → `N8 · Paris` | 完整链路 |
| 全程 | 数字持续爬升：`196.2M → 377.7M` tokens、`+567.70 → +1,041.71` 利润、`15.1k → 17.7k` tok/s、`5/8 → 8/8` hosts | A1 + A14 |
| 全程 | 粒子沿曲线持续流动、虚线环旋转、状态点呼吸 | A2/A3/A4 |

**实现要点**：写一个 `EventBus` + `scheduler`，每个事件是一个对象：
```js
{ at: 1500, type: 'rent', payload: { host:'Dallas', gpus:'6×H100', rate:'$11.28/h', queue:'2.4k tok/s' } }
```
播放器 `setTimeout` 触发 → 派发 `modal:show` → 用户点击（或 `autoApprove` 演示模式在 400ms 后自动点击）→ 派发 `modal:approve` → 各订阅者（列表/日志/数字/图表）**同步响应**。这是唯一能保证"多组件演出同步"的架构。

---

## 7. 技术实现方案

### 7.1 技术选型建议

| 方案 | 适用 | 说明 |
|---|---|---|
| **原生 HTML + CSS + 少量 JS** | ✅ 推荐 | 本界面无需框架；SVG 承担图形与点链，CSS 承担大部分动效 |
| React / Vue | 团队协作、数据驱动 | 列表与日志天然适合响应式渲染；动效用 CSS 类切换，不要引入动画库 |
| Canvas | 点+路径元素总数 > 1500 或需要尾迹 | 本例 10 条边 × ~35 点 ≈ 350 个 `<circle>`，SVG 完全够用且更好调试 |
| 动画库 | 谨慎 | GSAP 可做复杂编排，但本例所有动效 CSS 都能覆盖；**不要引入 framer-motion 做位移动画**（会显著降低 60fps 稳定性） |

### 7.2 网络图：SVG 路径 + 彩色点链

```html
<svg class="net" viewBox="0 0 400 260">
  <defs>
    <filter id="glow"><feGaussianBlur stdDeviation="2.2" result="b"/>
      <feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
  </defs>
  <g id="edges"></g>
  <circle class="hub-ring" cx="200" cy="130" r="34"/>
  <circle class="hub" cx="200" cy="130" r="26"/>
  <text class="hub-val" x="200" y="134">15.1k</text>
</svg>
```

```js
// 生成贝塞尔路径：控制点水平外扩，得到 S 形
const edge = (x1,y1,x2,y2) => {
  const dx = Math.abs(x2-x1) * 0.42;
  return `M${x1},${y1} C${x1+dx},${y1} ${x2-dx},${y2} ${x2},${y2}`;
};
```

**粒子方案 A（推荐，GPU 合成，零 JS 每帧计算）**
```css
.particle{
  offset-path: path("M60,60 C150,60 140,130 200,130"); /* 与连线 d 保持一致 */
  animation: flow 1.5s linear infinite;
  animation-delay: var(--delay);        /* 相位错开 */
}
@keyframes flow{
  from{ offset-distance: 0%;  opacity:0; }
  8%  { opacity:1; }
  92% { opacity:1; }
  to  { offset-distance: 100%; opacity:0; }
}
```
> `offset-path` 在现代 Chrome/Safari/FF 均支持；若需兼容旧内核，退回方案 B。

**粒子方案 B（JS 采样，可控性强）**
```js
const path = document.querySelector('#edge-3');       // SVGPathElement
const len  = path.getTotalLength();
const dot  = document.querySelector('#dot-3');
let t = 0;
function tick(dt){
  t = (t + dt/1500) % 1;                              // 1.5s 走完全程
  const p = path.getPointAtLength(len * t);
  dot.setAttribute('transform', `translate(${p.x},${p.y})`);
  requestAnimationFrame(tick);
}
```

### 7.3 数字滚动（Odometer）

```js
function createCounter(el, {format}){
  let cur = 0, target = 0;
  const fmt = format || (v => v.toFixed(0));
  return {
    set(v){ target = v; },
    tick(){
      // 指数平滑：无论目标值多大，都是"快速逼近后缓收"
      cur += (target - cur) * 0.12;
      if (Math.abs(target-cur) < 0.01) cur = target;
      el.textContent = fmt(cur);
    }
  };
}
// 全局一个 rAF 循环驱动所有 counter（不要每个数字起一个 rAF）
const counters = [profitCounter, tokenCounter, rateCounter];
(function loop(){ counters.forEach(c=>c.tick()); requestAnimationFrame(loop); })();
setInterval(()=>{ /* 每 100ms 从模拟引擎取新目标值并 set() */ }, 100);
```
> **坑**：不要用 `setInterval` 直接写 DOM 数字，会出现"阶梯跳变"；必须用插值。
> **坑2**：货币必须 `tabular-nums`，否则每次数字变化整行宽度抖动。

### 7.4 折线图（纯 SVG，无图表库）

```js
function toSmoothPath(pts){                 // Catmull-Rom → Cubic Bezier
  if (pts.length < 2) return '';
  let d = `M${pts[0][0]},${pts[0][1]}`;
  for (let i = 0; i < pts.length-1; i++){
    const p0 = pts[i-1] || pts[i], p1 = pts[i], p2 = pts[i+1], p3 = pts[i+2] || p2;
    const c1x = p1[0] + (p2[0]-p0[0])/6, c1y = p1[1] + (p2[1]-p0[1])/6;
    const c2x = p2[0] - (p3[0]-p1[0])/6, c2y = p2[1] - (p3[1]-p1[1])/6;
    d += ` C${c1x},${c1y} ${c2x},${c2y} ${p2[0]},${p2[1]}`;
  }
  return d;
}
```
面积图 = 折线路径 + 闭合到底边：`d + ` L${last.x},${H} L${first.x},${H} Z``，填充 `url(#gradGreen)`。

### 7.5 日志追加

```js
function pushLog({time, tag, msg}){
  const row = document.createElement('div');
  row.className = 'log-row enter';
  row.innerHTML = `<span class="log-t">${time}</span>`
                + `<span class="log-tag" data-tag="${tag}">${tag}</span>`
                + `<span class="log-msg">${highlight(msg)}</span>`;
  box.appendChild(row);
  requestAnimationFrame(()=> row.classList.remove('enter'));   // 触发过渡
  while (box.children.length > 7) {                            // 保留 7 行
    const old = box.firstElementChild;
    old.classList.add('leave');
    setTimeout(()=> old.remove(), 160);
  }
}
```
```css
.log-row{ transition: opacity .26s ease-out, transform .26s ease-out; }
.log-row.enter{ opacity:0; transform: translateY(6px); }
.log-row.leave{ opacity:0; transform: translateY(-4px); }
.log-tag[data-tag="you"]{color:var(--yellow)} .log-tag[data-tag="risk"]{color:var(--pink)}
.log-tag[data-tag="router"]{color:var(--cyan)} .log-tag[data-tag="scout"]{color:var(--green)}
```

### 7.6 数据模拟引擎（让界面"自己会动"）

```js
const SCRIPT = [
  {at:1200,  type:'approve'},
  {at:2450,  type:'sign',  who:'translator'},
  {at:4300,  type:'kill',  host:'Warsaw', mult:'2.1x'},
  {at:7500,  type:'rent',  host:'Reno',   gpus:'8×H100', rate:'$11.68/h'},
  {at:9500,  type:'rent',  host:'Paris',  gpus:'8×H100', rate:'$12.72/h'},
];
const HEARTBEAT = setInterval(()=>{           // 每 1s 的"心跳"
  state.tokens += 14_000_000 + rand(-2e6, 2e6);
  state.rate   = clamp(state.rate + rand(-0.8,0.8), 12, 20);
  state.revenue += rand(18, 46);
  state.gpuCost += rand(6, 18);
  state.profit = state.revenue - state.gpuCost;
  render();                                    // 只更新 textContent / style
}, 1000);
```
> 演示模式：弹窗出现后若 3s 未操作则自动 `approve`（录制视频就是这么跑的），真实产品里应等用户点击。

### 7.7 性能与无障碍

- **只用一个 rAF 主循环**驱动所有插值（数字、粒子、图表），避免多循环互相抢占。
- 粒子 ≤ 20 个；超过则改用 Canvas 批量绘制。
- 颜色/尺寸变化优先 `transform / opacity / filter`，禁止动画 `width / height / top`。
- `will-change: transform` 只加在粒子与弹窗上（滥用会爆显存）。
- **必须支持 `prefers-reduced-motion`**：关闭粒子流动、呼吸、旋转，数字改为直接跳变。
```css
@media (prefers-reduced-motion: reduce){
  *{ animation-duration:.001ms !important; animation-iteration-count:1 !important;
     transition-duration:.001ms !important; }
}
```
- 语义化：列表用 `<ul>/<li>`，弹窗加 `role="alertdialog"` + `aria-live="polite"` 的日志区。

---

## 8. 复刻验收清单（Checklist）

**视觉**
- [ ] 底色是带青灰倾向的近黑（不是纯 `#000`），卡片 `#0C0C18`，1px 8% 白描边
- [ ] 5 种强调色各司其职，无混用
- [ ] 所有数字 `tabular-nums`
- [ ] 区块标签 9.5px uppercase +0.14em
- [ ] 角色图标复用「彩色圆 + 两条深色竖条」母题
- [ ] 卡片圆角 18px、弹窗 12px、chip 999px

**布局**
- [ ] 三栏 `168px : 1fr : 168px`，MONEY 区 `1fr : 180px`
- [ ] 视口固定、**无滚动**
- [ ] 客户端行左侧 3px 分类色竖条
- [ ] 利润行为内嵌块（`--bg-inset` + 圆角 + 淡绿底）高亮
- [ ] 五个角色图标形状各异（胶囊/圆/水滴/星花/圆）
- [ ] 页面有粉紫氛围光，底部浅色区块只露一窄条

**动效**
- [ ] 数字插值滚动（非阶梯跳变）
- [ ] 网络图连线是**彩色点链**（不是灰曲线），每条边颜色 = 数据流实体色
- [ ] 点链整体流动，走完全程 1.2~1.6s，两端淡出
- [ ] 中心节点是**粉色**发光环 + 粉色点线外环 + 数值 + `TOK/S` 小字
- [ ] 中心虚线环 14s 匀速旋转
- [ ] 弹窗 180ms 入场 / 220ms 退场，approve 按钮浅色实心深字
- [ ] 日志从底部插入、超 7 行移除最旧
- [ ] 折线是直段（非平滑），绿线末端有发光圆点
- [ ] 折线两线间距 = 利润（GPU rent ≈ revenue 的 30%），绿上粉下，图例竖排
- [ ] 列表行之间、日志行之间都有 1px 分隔线
- [ ] 未来事件的行位以 `opacity:.32` 暗态预置，事件发生后才点亮
- [ ] RISK 告警时图标脉冲 600ms ×2

**编排**
- [ ] 每个「审批」事件同时驱动：弹窗 → 列表新增 → 日志追加 → 数字跳变 → 图表追加
- [ ] 11 秒剧本可完整重放（Dallas → translator → kill → Reno → Paris）

---

## 9. 附：最容易做错的 10 件事

1. **把底色做成纯黑** → 立刻变廉价；请保留 `#0A0E10` 的青灰倾向与极淡渐变。
2. **给弹窗加遮罩** → 破坏"实时看板"的非阻塞感，视频中弹窗是浮层不是模态。
3. **数字用 `setInterval` 直接赋值** → 阶梯跳变，没有"活着"的感觉。
4. **粒子用 `left/top` 动画** → 掉帧；用 `offset-path` 或 `transform: translate()`。
5. **强调色随便挑** → 必须语义固定（绿=赚、粉=花、琥珀=等你拍板、黄=人干的）。
6. **加了滚动叙事** → 视频实测 0 滚动，动态感全部来自数据更新。
7. **把网络图连线画成灰色实线** → 实测是**彩色点链**，这是"像不像"的分水岭。
8. **响应式断点照搬桌面套路** → 视频本身是 592px 窄屏下的三栏密集布局，
   断点必须压到 **380px 以下**；在 768/820 就塌成单列会完全走样。
9. **用空 `<g>` 元素测高度做初始化守卫** → 它此刻没有子节点、高度恒为 0，会让初始化永远推迟；
   要测容器（如 `.netbox`）的高度。
10. **`preserveAspectRatio="none"` + viewBox 比例与容器差太多** → 曲线被纵向压平，弧度全丢；
    让 viewBox 比例贴近容器（如 `0 0 400 300`）。

---

## 10. 与视频的逐项校验记录（v2 校正）

把 `grokbot-demo.html` 渲染出来、与视频关键帧（0.0s / 2.5s / 4.5s / 7.6s / 9.6s）逐块对照后，
共发现并修正 **22 项**偏差。下面按"影响程度"排序，**每一条都是容易踩的坑**。

### 10.1 校验方法

| 步骤 | 手段 |
|---|---|
| 取视频基准帧 | OpenCV 按时间点抽帧，含事件窗口内的 33ms 原始帧 |
| 取复刻截图 | Playwright + Chromium，`viewport: 592×1280`（贴近视频宽度）、`deviceScaleFactor: 2`，在相同时间点截图 |
| 定位区块边界 | 对左侧内容列做**逐行亮度剖面**（找 1px 分隔线与文字带） |
| 定色 | HSV 色相聚类取"最亮 25% 像素"的均值，避免 JPEG 噪点干扰 |
| 定形态 | 区域裁剪后 **4~8 倍**放大目视（点链、图标形状、按钮态只能这样看出来） |

### 10.2 偏差清单

| # | 项目 | 初版（错） | 视频实测（对） | 影响 |
|---|---|---|---|---|
| 1 | **网络图连线** | 灰白实线曲线 + 上面跑 8~14 个彩色粒子 | **整条线就是一串彩色小圆点**（r≈2px、间距≈6px、颜色=数据流实体色），整链沿路径流动 | ★★★★★ 决定"像不像" |
| 2 | **中心节点** | 白灰描边 + 白色虚线环 | **粉色描边 + 粉色外发光**，外圈是**粉色点线环**；节点下方还有 `TOK/S` 小字 | ★★★★ |
| 3 | **响应式断点** | 820px 就塌成单列 | 视频本身就是 **592px 窄屏下的三栏密集布局**，断点应压到 380px 以下 | ★★★★ 布局全错位 |
| 4 | **统计行** | 标签在上、数值在下；独占整宽一行 | **数值在上、标签在下**；且**与利润块同处右侧一栏** | ★★★★ |
| 5 | **角色图标** | 全是圆；ROUTER 用 45° 方块；SCOUT 用蓝色 | SCOUT=**青色横向胶囊**、RISK=**尖朝上的水滴**、ROUTER=**粉红星花+光晕**、QUANT/YOU=圆 | ★★★★ |
| 6 | **approve 按钮** | 默认就是浅色实心深字 | 默认是**暗底 + 亮描边 + 白字**（与 deny 同类），**只有 hover 才翻成浅色实心** | ★★★ |
| 7 | **统计行数值字号** | 15px | 约 **20~22px**（比行内数值明显大一档） | ★★★ |
| 8 | **GPU 行结构** | 名称+数值一行，负载条另起一行且撑满 | `状态点+名称 → 负载条 → 数值` **同一行**，条长上限 ~78px，meta 在下一行 | ★★★ |
| 9 | **折线图图例** | 横排一行 | **竖排两行**，贴 MONEY 标签下方 | ★★★ |
| 10 | **折线形态** | 贝塞尔平滑曲线 | **直段折线**（有明显折角），且**绿线末端有发光圆点** | ★★★ |
| 11 | **列表行样式** | 圆角卡片块 + 行间距 | **方角行 + 行间 1px 分隔线**（日志行同理） | ★★★ |
| 12 | **未来事件行** | 事件发生时才 push 新行 | 行位**一开始就以 `opacity:.32` 暗态预置**，事件后逐条点亮 | ★★★ |
| 13 | **待审批块边框** | `dashed` 虚线 | **`dotted` 点线** | ★★ |
| 14 | **弹窗标题色** | 琥珀色 | **白色**（琥珀只用于边框与 RISK 状态词） | ★★ |
| 15 | **弹窗宽度** | 300px 固定，被中栏裁切 | ~320px，**比中栏还宽、会溢出到左右栏上方**（容器不能 `overflow:hidden`） | ★★ |
| 16 | **日志 router 标签色** | 青色 | **粉紫 `#C08BF0`**（scout/scribe 才是青绿） | ★★ |
| 17 | **结算卡标题** | `MONEY` | **`TONIGHT`**；profit 行是**淡绿底内嵌块** | ★★ |
| 18 | **底部浅色区块** | 150px 高、彩色块很明显 | **只露 30~50px 一窄条**，浅灰白底 + 强模糊光斑 | ★★ |
| 19 | **页面氛围光** | 无 | 左右各一团**粉紫 radial-gradient** 雾光 | ★★ |
| 20 | **网络图区域高度** | 依赖 SVG 固有比例，被压成矮条 | 用 `flex:1 + min-height` 撑满本栏高度（与列表等高） | ★★ |
| 21 | **头像** | 44px + 细长竖条 | **56px + 粗短胶囊**（6×18px、圆角 3px） | ★ |

### 10.3 实现层面踩到的 4 个隐形坑

1. **SVG 呈现属性不支持 `var()`**
   `circle.setAttribute('fill', 'var(--cyan)')` **不生效**，必须写内联样式：`c.style.fill = 'var(--cyan)'`。
   同理 `filter: drop-shadow(... currentColor)` 需要同时设 `c.style.color` 才会解析。
2. **不能用空 `<g>` 测高度做初始化守卫**
   `<g id="edges">` 在还没插入子节点时 `getBoundingClientRect().height === 0`，
   用它做守卫会导致初始化被无限推迟（表现：网络图完全空白、且不报错）。要测**容器**（`.netbox`）的高度。
   同时 `stepChains()` 里要加 `if (!N) continue;` 与 `if (!pt) continue;` 兜底。
3. **日志退场的"异步移除 while 死循环"（最严重，直接卡死主线程）**
   错误写法：
   ```js
   while (logEl.children.length > 6){
     const old = logEl.firstElementChild;
     old.classList.add('leave');
     setTimeout(() => old.remove(), 160);   // 异步移除 → length 永不下降 → 死循环
   }
   ```
   因为子节点是 160ms 后才真正移除的，`children.length` 在循环体内**不会变化**，
   `while` 永远出不来（且不报任何错，只是页面整体冻住）。
   正确写法：**用快照数组驱动**——
   ```js
   const rows = [...logEl.children];
   rows.slice(0, Math.max(0, rows.length - 6)).forEach(old => {
     if (old.classList.contains('leave')) return;   // 已在退出队列的跳过
     old.classList.add('leave');
     setTimeout(() => old.remove(), 160);
   });
   ```
4. **`preserveAspectRatio="none"` 会放大比例失配**
   `viewBox="0 0 400 260"` 放进 480×400 的容器 → 纵向被拉伸 1.46 倍，曲线弧度看起来被"压平"。
   把 viewBox 改成 `0 0 400 300`（贴近容器比例）后弧度立刻恢复正常。

### 10.4 校验结果

修正后在 **592×1280 视口**下与视频逐块比对，以下均一致：
页头结构（标题/利润/统计行位置与顺序）、5 个角色图标形状与颜色、chip 行、三栏比例与宽度分布、
客户端行（色条/分隔线/meta 单行/暗态行）、GPU 行（条位/长度/颜色/待批点线块/暗态行）、
网络图点链与中心粉环、审批弹窗（边框/标题色/按钮默认态与 hover 态）、
MONEY 竖排图例与直段折线 + 末端光点、TONIGHT 结算卡配色、日志（行分隔线 + 5 种标签色）、
底部浅色窄条与页面粉紫氛围光。

无头 Chromium 冒烟：**console 零报错**，事件链路（Dallas → translator → kill → Reno → Paris）完整可重放。

---

*配套文件：`grokbot-design-tokens.css`（设计令牌）、`grokbot-demo.html`（可直接运行的参考实现）。*
