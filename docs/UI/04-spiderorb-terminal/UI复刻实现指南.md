# SPIDERORB // RESEARCH BOT — 赛博朋克终端仪表盘 UI 复刻实现指南 v2

> 本文档基于对录屏视频的逐帧分析 + **复刻 Demo 的并排放大比对**（见《深度对比分析-视频vsDemo.md》），目标是让 AI（或前端工程师）用纯前端技术 1:1 复刻该 UI 界面，包括布局、样式与全部动效。
>
> **视频文件**：`f91c610795b96f116298522e0227f525_raw.mp4`（微信接收）>   
> **视频规格**：5.9 秒 · 1320×2868（≈19.5:9 竖屏）· 60fps · 356 帧 · 无场景转场（单屏 Live Dashboard）>   
> **分析素材**：`frames/`（关键帧）、`crops/`（分区放大图）、`verify/deep/`（原图 vs Demo 逐区块并排对比图）>   
> **验证记录**：《复刻验证报告.md》（布局/色彩/动效三重机器比对结果）
>
> **v2 修订**（依据深度对比）：区块 8 左列"径向仪表"更正为"流式 sparkline 行 ×5"；THE ORB 更正为"放射爆发 + 卫星 hub"结构；决策栈更正为盒子行；SIX AGENTS 图标盒结构；新增 4 个漏记的子面板；LIVE FEED 实测为静态；新增全局质感层与落地陷阱章节。

---

## 目录

1. [软件来源考证（GitHub 调研结论）](#1-软件来源考证)
2. [设计语言总览](#2-设计语言总览)
3. [配色系统（实测色值）](#3-配色系统)
4. [字体排印](#4-字体排印)
5. [布局结构详解](#5-布局结构详解)
6. [动效系统详解（逐项实测参数）](#6-动效系统详解)
7. [组件级实现指南（代码级）](#7-组件级实现指南)
8. [完整技术方案与落地陷阱](#8-完整技术方案与落地陷阱)
9. [可直接使用的 AI 生成提示词](#9-可直接使用的-ai-生成提示词)
10. [附录](#10-附录)

---

## 1. 软件来源考证

### 1.1 调研过程与结论

**结论：未找到该软件的 GitHub 开源地址。这并非一个已开源的项目，而是一段 UI 概念设计演示视频（设计者水印为右下角 `@KRAVN`）。**

已执行的检索：

| 检索方式                    | 关键词                                                               | 结果                                       |
| ----------------------- | ----------------------------------------------------------------- | ---------------------------------------- |
| GitHub Repositories API | `spiderorb`                                                       | **0 个仓库**                                |
| GitHub Repositories API | `kraven dashboard / terminal`                                     | 仅返回无关项目（KravenHD 等 Kodi 插件、Docker 管理面板等） |
| Google                  | `"spiderorb" dashboard UI`                                        | 无相关结果                                    |
| Google                  | `"ONE SPIDER" "1,400 STATES" / "MEASURED BEHAVIOR" / "SPIDERORB"` | 无相关结果                                    |
| Google                  | `KRAVEN "research bot" cyberpunk dashboard concept`               | 无相关结果                                    |

### 1.2 判断依据

1. 界面中所有数据均为**虚构演示数据**：`RUN-004228`、`ARXIV 2510.04412`、`spdr@Lab:~$ orb trace`、`AGENT-SDK REPO`、`GLAB_INSIDER`、`ALPHA_BOT_773` 等，是典型的概念设计占位内容。
2. 界面完成度是"演示级"而非"产品级"：信息密度极高但无可交互元素，符合设计演示视频特征。
3. 水印 `@KRAVN` 是设计师署名，常见于 X(Twitter)/Dribbble/Behance 的概念作品分享。
4. 视频为手机竖屏录屏，带 iOS 状态栏（22:46）与底部黑边，符合"设计师录制概念稿后分享"的传播形态。

### 1.3 建议

- 若要追踪原作者：拿视频首帧到 **Google Lens** 以图搜图，或在 X(Twitter) 搜 `@KRAVN dashboard` / `spiderorb`。
- 若你有该视频的原始分享链接，可以提供给我做进一步溯源。
- **对本项目的影响**：无开源代码可参考，本文档即为完整实现依据——所有视觉与动效参数均已从视频中实测提取，并经一轮复刻验证修正。

---

## 2. 设计语言总览

### 2.1 风格定位

| 维度   | 描述                                            |
| ---- | --------------------------------------------- |
| 风格   | Cyberpunk / FUI（Fictional UI）/ HUD 仪表盘 / 终端美学 |
| 情绪   | 高压、密集、实时、"机器在替你工作"的掌控感                        |
| 信息密度 | **极高**（单屏 10+ 面板、50+ 数据点）——密度本身即风格            |
| 构图   | 严格的模块化栅格，面板几乎零圆角、1px 细边框、无投影                  |
| 色彩策略 | 近黑底（占 ~85% 面积）+ 霓虹强调色（占 ~15%），"颜色即语义"         |
| 文字策略 | 全大写等宽字体、小字号、`◆`/`·`/`//` 符号系统营造终端感            |

### 2.2 五个标志性设计手法

1. **`◆` 菱形面板前缀**：每个面板标题前都有 `◆`（部分为 `■`），是全局视觉签名。
2. **彩色语义标签**：`SAVE`=绿、`BLOCK`=红、`SKIP`=金、`REPO`=金、`LIVE FEED`=红底徽章——颜色直接编码动作类型，全界面严格遵守（详见 §3.3）。
3. **方括号代号系统**：`[JDG]`、`[QNT]`、`[GRD]`、`[WRT]`、`[ORB]`、`[MEASURED]`、`[SCT]`、`[MAP]`——用 3 字母代号指代"六个智能体"，配合专属颜色。
4. **点阵/LED 段码大数字**：LIVE COUNT 用绿色点阵数码风格字体呈现（非实心像素体），带辉光，是全场视觉锚点。
5. **"文档即装饰"**：日志流、终端命令行（`spdr@Lab:~$`）、ARXIV 编号等"文本内容"本身承担装饰职责。

### 2.3 面板（Panel）通用规范

```
┌─────────────────────────────────┐
│ ◆ PANEL TITLE        [META]     │  ← 标题行：◆ + 大写标题 + 右侧元信息/徽章
│─────────────────────────────────│  ← 1px 分隔线 #24222B
│ 内容区                           │
└─────────────────────────────────┘
```

- 背景：`#12151E`（比页面背景 `#0D111A` 亮约 5%）
- 边框：`1px solid #24222B`
- 圆角：**0**（或至多 2px）
- 内边距：约 10-14px
- 标题字号 ≈11px，字重 600，letter-spacing ≈0.08em，颜色 `#E5E3E7`
- 右侧元信息 ≈9-10px，颜色 `#8B868D`

### 2.4 全局质感层（v2 新增 —— "视频感"的最后一公里）

原视频相对干净网页渲染多出四层微妙质感，叠加后才能"看起来像视频里的东西"。全部用 ≤5% 透明度，做到"若有若无"：

| 层           | 参数                       | 实现                                                                                 |
| ----------- | ------------------------ | ---------------------------------------------------------------------------------- |
| 噪点颗粒        | 全屏细颗粒，透明度 1.5-2%         | SVG `feTurbulence` 生成 data-URI 平铺，或 128px 噪点 PNG `repeat`                          |
| 扫描线         | 水平 1px 线，间隔 3-4px，透明度 3% | `repeating-linear-gradient(0deg, transparent 0 3px, rgba(0,0,0,.12) 3px 4px)` 全屏罩层 |
| 暗角 vignette | 四角压暗 8-12%               | `radial-gradient(ellipse at center, transparent 60%, rgba(0,0,0,.35) 100%)` 罩层     |
| 文字辉光        | 亮白/彩色标题级文字 2-6px 柔光      | `text-shadow: 0 0 6px currentColor`（透明度 0.3-0.5）                                   |

```css
.texture-noise { position: fixed; inset: 0; pointer-events: none; z-index: 99;
  opacity: .018; background-image: url("data:image/svg+xml,...");
  /* feTurbulence baseFrequency=.9, numOctaves=2, 128x128 tile */ }
.texture-scan { position: fixed; inset: 0; pointer-events: none; z-index: 99;
  background: repeating-linear-gradient(0deg, transparent 0 3px, rgba(0,0,0,.12) 3px 4px); opacity: .25; }
.texture-vig  { position: fixed; inset: 0; pointer-events: none; z-index: 99;
  background: radial-gradient(ellipse at center, transparent 55%, rgba(0,0,0,.38) 100%); }
```

---

## 3. 配色系统

### 3.1 中性色（实测）

| 用途      | 色值                    | 说明                            |
| ------- | --------------------- | ----------------------------- |
| 页面背景    | `#0D111A`             | 近黑海军蓝，全场底色                    |
| 面板背景    | `#12151E` / `#161922` | 两级面板层次                        |
| 边框/分隔线  | `#24222B` / `#2A2E3A` | 1px 细线                        |
| 主文字     | `#E5E3E7`             | 近白（大面积正文实测主色，**非** `#D5FFFB`） |
| 次文字     | `#8B868D`             | 灰                             |
| 弱文字/微标签 | `#555966`             | 暗灰（如顶部微文本行）                   |

### 3.2 语义强调色（实测）

| 语义      | 主色                    | 深色变体                  | 用途                           |
| ------- | --------------------- | --------------------- | ---------------------------- |
| 绿 GREEN | `#53CD90`             | `#33AE6A` / `#008040` | SAVE、计数器、SIGNAL、JDG、正向数据     |
| 红 RED   | `#AC364E`             | `#8D3E44`             | BLOCK、STALE、GRD、LIVE FEED 徽章 |
| 亮红/品红   | `#D8425D` / `#B85888` | —                     | 网络高亮 hub、REACH、SPDR 标签、危险数据  |
| 金 AMBER | `#FFDD83`             | `#DEA248`             | 星尘粒子、QNT、REPO、HYPE、行底进度条     |
| 蓝紫 BLUE | `#A2B3DF` / `#8C90C0` | `#8379BE`             | LOGO、链接、MAP、徽章描边             |
| 青 CYAN  | `#D5FFFB`             | `#4F8470`             | 图表高亮光点、CURVE（仅用于高亮，不作正文色）    |

> 视频压缩会使实测色值略灰，实现时可在保持明度关系的前提下适度提高饱和度。

### 3.3 颜色→语义映射表（必须严格遵守）

| 颜色 | 含义                  | 出现位置                                                           |
| -- | ------------------- | -------------------------------------------------------------- |
| 绿  | SAVE / 保存 / 信号 / 通过 | 决策栈 SAVE 行、JDG 代号、GUARD PASS、STATE SIGNAL 条、REINFORCE 箭头       |
| 红  | BLOCK / 拦截 / 停滞     | 决策栈 BLOCK 行、GRD 代号、STALE 条、LIVE FEED 徽章、SPIDER-ON 徽章、CANCEL 箭头 |
| 金  | SKIP / 跳过 / 量化      | 决策栈 SKIP 行、QNT 代号、REPO 标签、星尘粒子、PAPER INFLOW、决策行底进度条            |
| 蓝紫 | 链接 / 映射 / 智能体 MAP   | LOGO、RESEARCH 按钮、MAP 代号、1,400 STATES 徽章                        |
| 品红 | 热点 / 传播 / SPDR 分    | REACH 51.5K、网络高亮 hub、"SPIDER CAUGHT IT" 徽章、决策行 SPDR 标签         |
| 青  | 排名 / 曲线光点           | CURVE HN #1、曲线/流线上的运动光点                                        |

---

## 4. 字体排印

### 4.1 字体家族

全场**单一等宽字体家族**（像素/位图风格），通过字号/字重/颜色分层：

- **标题**：`SPIDERORB // RESEARCH BOT`——粗体像素字体，约 30px，`letter-spacing: 0.02em`
- **面板标题/正文/数据**：等宽，10-13px，**全大写**，`letter-spacing: 0.06~0.12em`
- **微文本行**：9px，`#555966`，字距拉到 `0.2em` 以上（"散焦"效果）
- **大计数器**：**点阵/LED 段码风格**（非实心像素体），约 64-72px，绿色，带辉光

### 4.2 推荐字体栈

```css
/* 标题（像素体） */
font-family: "Silkscreen", "Pixelify Sans", monospace;
/* 数据/正文（技术等宽） */
font-family: "Share Tech Mono", "JetBrains Mono", "IBM Plex Mono", ui-monospace, monospace;
/* 大计数器（点阵 LED 感，按可得性三选一） */
font-family: "DotGothic16", "Silkscreen", monospace;
/* 或继续用 Silkscreen + CSS 点阵纹理叠加：
   background: radial-gradient(circle, currentColor 38%, transparent 40%) 0 0/3px 3px;
   -webkit-background-clip: text; color: transparent; */
```

- Google Fonts 可直接引入 `Silkscreen` + `Share Tech Mono` + `DotGothic16`；国内环境可换 `Zpix` / `Fusion Pixel`。
- **⚠ 字体子集陷阱（v2 新增）**：Google Fonts 的 latin 子集**不含** `◆ ■ ▲ ★ ✓ ·` 等符号，直接引用会回退成系统字体甚至豆腐块。两种对策：① 这些符号一律用 CSS 图形/伪元素绘制（推荐，渲染最一致）；② 引入字体的 `latin` + `symbols` 子集并逐字符验证。

### 4.3 排版规则

- 所有标签、面板标题、按钮文字**一律大写**；
- 数字使用 `font-variant-numeric: tabular-nums`（等宽数字，防止计数动画时抖动）；
- 大量使用符号系统：`◆`（面板）、`■`（次级）、`·`（分隔）、`//`（品牌分隔）、`▲`（上升）、`+`（增量）、`★`（星标）、`✓`（日志成功行前缀）；
- 中英混排不出现——纯英文终端语境。

---

## 5. 布局结构详解

### 5.1 坐标系说明

原视频 1320×2868，内容实际占据 **y ≈ 450 → 2150**（上方 y150-450 与下方 y2150-2868 为黑边，系录屏 letterbox）。以下坐标均为**原图像素坐标**，括号内为**相对内容区高度（1700px）的百分比**，复刻时用百分比适配。

参考图：`crops/33_grid_full.png`（带标尺全图）、`frames/t000s.jpg`（首帧原图）、`verify/deep/`（与 Demo 并排对比）。


### 5.2 自上而下 11 个区块

#### 区块 0 · iOS 状态栏（y 60-120）
- 左：`22:46`；右：信号 / Wi-Fi / 充电电池图标。
- 复刻：移动端复刻时保留系统状态栏即可；Web 复刻可省略或做成装饰性顶栏。

#### 区块 1 · 微文本行（y 450-470，0-1%）
- 左：`◆ SIGNALS · PROBABILITY STATES · COLLAPSE · SIX AGENTS`，9px `#555966`
- 右：`[1,400 STATES]` 蓝色描边徽章
- **作用**：氛围层，制造"散焦远景"。

#### 区块 2 · 主标题行（y 470-510，1-4%）

```
[SO]  SPIDERORB // RESEARCH BOT        SOURCES   24H BLOCKED   INSIGHTS
                                      10,663    7,169         58      19:55:03 UTC  [SPIDER-ON]  SLOT
```

- `SO` LOGO：蓝紫渐变圆角方块（约 44×44px，圆角 8px），内白色 `SO`，**外带明显蓝紫辉光**（`box-shadow: 0 0 18px rgba(131,121,190,.5)`）
- 标题：像素粗体 ~30px `#E5E3E7`，带轻微文字辉光
- 右侧三组统计：标签 9px 灰 + 数值 ~22px 白，竖排对齐
- `19:55:03 UTC`：数码风格白字，**实时走秒**
- `[SPIDER-ON]`：红底 `#AC364E` 黑字徽章（实测无闪烁，静态）
- `SLOT`：暗灰小字

#### 区块 3 · LIVE FEED 标签行（y 510-560，4-6%）—— 静态信息条

> **v2 实测修正**：相位相关检测 t0→t5 水平位移 ≈0（响应 0.88/0.94），该行**完全静止，不是跑马灯**，勿加滚动动画。

横向 5 个标签，混排徽章与文本：

| 标签 | 样式 |
|---|---|
| `◆ LIVE FEED` `-28.9%` | **红底徽章** + 白字负值 |
| `REPO` `AGENT-SDK` `+412★` | 金字 |
| `SAVE EVAL REPORT` `· net value +8.3% · 0.12 MIN` | 白字 + 灰字 |
| `BLOCK FREE GPU AIRDROP` | 红字 |
| `CURVE HN #1 81%` | 青字 |

#### 区块 4 · 三栏面板行（y 560-660，6-12%）

| 左栏（0-33%） | 中栏（33-66%） | 右栏（66-100%） |
|---|---|---|
| **◆ CORE MODEL — IN TRAINING**<br>`SPDR-LIGHTBOT v0.0.4` + `[RESEARCH]` 蓝描边按钮<br>`LOBEL-MEASURED TODAY` | **◆ LIFE OF A DECISION — ORB TRACE**<br>标题区右侧：**蓝描边 boxed chip** `RUNS 54 · SAVE 23 · BLOCK 4 · SKIP 27`<br>日志行：`[JDG] +256ms followed account · P(edge) 0.87 · 4 in this topic`<br>`[ORB] +356ms state · SIGNAL 0.54 (±0.08) · noise 0.22 · stale 0.25`<br>`[WRT] +?4?ms net value +3.9% · read 0 min · 3.1K words` | **◆ ENTANGLEMENT MAP**<br>`13 SOURCES · 368 ACCOUNTS` · 右侧 meta **`12 LINKED`（品红）**<br>雷达式散点图：节点 `#LAB_IN`、`ARXIV`、`HB`，白/品红点 + 细连线 |

#### 区块 5 · 计数器 + 日志流行（y 660-760，12-18%）

- **左（0-35%）**：LIVE COUNT 大计数器 `1,329`（绿色**点阵 LED** 风格，约 64px，带辉光），下方 `▲ SIGNAL 25% · NOISE 66% · STALE 25%`，底部三联小统计 `COUNTER 40 · VETO 71% · OPEN 2/6`
- **中（35-70%）**：实时日志流（见 §6.4）：
  ```
  [QNT] +4?1ms net value +13.6M · read 0 min · 2.1K words
  [GRD] +641ms PASS · 0 red flags · bots 3%
  [WRT] +1182ms note written · thread draft +3 · source linked
  ✓ insight saved · decision record written        ← 绿 ✓ 前缀行（无代号）
  [MEASURED] SIGNAL · +60% first at 14 min — training set
  spdr@Lab:~$ orb trace RUN-004228 --from-post
  # trigger: followed account posted 1.8k words on an unnamed model · topic 52%
  ```
- **右（70-100%）**：小型散点/网络图

#### 区块 6 · 统计列 + 曲线图 + CHANGELOG（y 760-1150，18-40%）

- **左统计列（0-22%）**：三组小统计卡
  - `40` `71%` `IN PLAY 2/6`
  - `EVERY 1,000 SOURCES SCANNED · LIVE`：`1,000` `992` `10` `2`（四联数）
  - `FOLLOWED FLOW`（绿色流线 + 运动光点 + `[JDG]` 节点标签）、`SOURCE RATE 13.0/M`、**`PAPER INFLOW 154 [OCT]`（金色区域 sparkline + 流动光点，v2 补记）**
- **中（22-70%）**：`◆ WHERE A STORY LANDS ON THE CURVE`
  - 徽章 `SPIDER CAUGHT IT`（品红实心）、金描边 `35 MIN · TRENDING · ON EVERY FEED`、红描边 `HYPE 6S`
  - 主曲线：蓝紫渐变线 + **沿线运动的发光圆点**（带拖尾），X 轴 40→80 刻度
  - **曲线右端有垂直金色虚线标记 TRENDING 位置；曲线中部有方形 marker chip（如 `READ HERE` 绿标签）（v2 补记）**
  - 右侧 REACH 面板：`51.5K` 大数字 + 三行 `AT 5 MIN 56 POSTS +13.4%` / `AT 30 MIN 1.2K POSTS +13.5%` / `AT 60 MIN 8.6K POSTS +5.1%`
  - 底部注释行：`OUR NOTE 12 MIN IN · 2.3K VIEWS` / `RESULT +47.2% FIRST TO POST` / `SPIDER · 0.6 SEC TO READ · 1 BITE`
- **右（70-100%）**：`◆ LAB CHANGELOG`
  - `GUARD PASS` 绿徽章；`ORIGINAL · SOURCE LINKED · 0 RED FLAGS`
  - 图例：`SOURCE · ACCOUNT · FOLLOWED · BOT FARM · SPIDER`（五色点）
  - 三行带概率条的变更：`SAVE 0.58` / `FREE GPU AIRDROP 0.76` / `#1 649 PTS 0.79`

#### 区块 7 · THE ORB 标题行（y 1150-1250，41-47%）

```
◆ THE ORB   OBSERVABLES      1,400 STATES · EACH ONE A SOURCE · AMPLITUDE · MODEL SCORE · 398 NEAR COLLAPSE
                                      ◆ DECISION STACK   [/agent_spider — ON THE ORB · SITES 12] ← 品红描边 chip
```

#### 区块 8 · 主视觉行（y 1250-1900，47-85%）—— 全场视觉中心

> **⚠ v2 重大修正**：本区块三个子组件的初版描述均有误判，以下列为准。对比图：`verify/deep/08_orb_gauge.png` / `08_orb_net.png` / `08_orb_decision.png`、原图放大 `verify/deep/x_orig_streams.png` / `x_orig_orb.png` / `x_orig_sigout.png`。

- **左列（0-27%）：流式 sparkline 行 ×5（Stream Rows）**——不是径向仪表！

  每行结构（高约 110px）：
  ```
  ▎LABEL(白粗体)          VALUE(彩色)  [TAG]  ┐
  ▎副标签(暗灰小字)                          ├ 波浪 sparkline(同色+淡填充)
  ▎~~~~~~~~~~~~~~~~~~~~~~~●                ┘ 末端白色光点沿线流动
  ```
  左侧 3px 彩色竖条；boxed 代号 chip；**五条流线右端向上扬起、汇入中央 ORB 网络**。

  | 行 | 标签 / 副标签 | 值（色） | chip | 流线色 |
  |---|---|---|---|---|
  | 1 | FOLLOWED FLOW / FOLLOWED POSTS · LA | 5（绿） | `[JDG]` | `#53CD90` |
  | 2 | BOT SHARE / LIKES FROM BOT FARMS | 28.6%（品红） | `[MAP]` | `#D8425D` |
  | 3 | HYPE RISK / FROM REPLY HYPERS | 44%（金） | `[MAP]` | `#FFDD83` |
  | 4 | REPLY DEPTH / REAL REPLIES IN | 7.14%（蓝紫） | `[BOT]` | `#A2B3DF` |
  | 5 | TOPIC PHASE / ABOVE OPEN 24H | 10.0%（白） | `[SCT]` | `#E5E3E7` |

- **中（27-70%）：THE ORB 放射爆发网络**（详见 §6.5 / §7.7）
  - 中央爆发核：数百条白色细丝辐射成蒲公英状，丝上串微粒
  - 6-8 个品红发光卫星 hub，各带 2-3 圈白色虚线轨道环
  - 左下 1-2 个大型金色虚线圆轨道
  - 左侧散布金色星尘（不连线）
  - 核心上方金色斜角贴纸 `ad for a course`
- **右列（70-100%）：`◆ DECISION STACK` 七行盒子行**——每行带完整边框：

  ```
  ┌──────────────────────────────────────────┐
  │ SAVE · JDG   OPEN-WEIGHTS EVAL  SPDR 0.83│  ← 彩色行动词+代号 / 粗体标题 / 品红 SPDR+值
  │              ·· 64 DAYS LEFT ··          │  ← 暗灰副标题
  │ ▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔          │  ← 行底金色细进度条
  └──────────────────────────────────────────┘
  ```

  | 行动词(色) | 标题 | 副标题 | SPDR |
  |---|---|---|---|
  | SAVE·JDG（绿） | OPEN-WEIGHTS EVAL | 64 DAYS LEFT | 0.83 |
  | SKIP·QNT（金） | "AGI IS HERE" POST | 3 HYPE | 0.38 |
  | SAVE·MAP（绿） | AGENT-SDK REPO | +412 STARS TODAY | 0.74 |
  | BLOCK·GRD（红） | @ALPHA_BOT_773 | BOTS FROM ONE FARM | 0.21 |
  | SAVE·SCT（绿） | ARXIV 2510.04412 | 4 IN THIS TOPIC | 0.77 |
  | SKIP·QNT（金） | MODEL LEAK RUMOR | UNNAMED MODEL | 0.64 |
  | SAVE·JDG（绿） | @LAB_INSIDER | ON THE CURVE | 0.81 |

  （初版 `JPG`/`QMT` 系误读，已修正。）

- **右列下方：SIGNALS → OUTCOMES 发散箭头面板（v2 补记）**：
  ```
  7 SIGNALS  3 OUTCOMES                     [STRONG FLOW]绿chip
  OF +SIGNAL FIRST    +1.84 REINFORCE   -0.74 CANCEL
                ▶▶▶▶▶▶ +0.30    ← 绿色右箭头 ×3（+0.30/+0.30/+0.27）
                ◀◀◀ -0.31       ← 红色左箭头 ×2（-0.31/-0.19）
                ▶▶▶ +0.15
  ```

#### 区块 9 · 底部行（y 1900-2100，85-97%）

- **左（0-46%）**：`◆ SIX AGENTS · OBSERVATIONS IN · DECISIONS OUT`
  - **总线装饰（v2 补记）**：卡片上方一条水平总线（标 `OBSERVATIONS IN` / `DECISIONS OUT`），每个智能体盒以竖线接上总线——表达"六智能体汇入观测/决策流"
  - 六个智能体横排。卡片结构（v2 修正，**无迷你柱状图**）：

    ```
    ┌─────┐
    │ ICON │   ← 大方盒(~60px高)，彩色边框 + 内嵌图标
    └─────┘
    SCT           ← 彩色代号
    SCOUT         ← 白色名称
    42            ← 大数值
    MOVED FAST    ← 暗灰小标签
    ```

    | 代号 | 名称 | 值 | 图标 | 盒框色 |
    |---|---|---|---|---|
    | SCT | SCOUT | 42 | 雷达/扇形 | 蓝紫 |
    | MAP | CARTOGRAPHER | 1,250 | 柱状图 | 蓝 |
    | JDG | JUDGE | 33 | 三角/天平 | 绿 |
    | QNT | QUANT | 90 | λ/分布曲线 | 金 |
    | GRD | GUARD | 76% | 盾形 | 红 |
    | WRT | WRITER | 2.4% | 文档 | 金 |

  - 下方 `LAST ACTIONS` 日志（5-6 行带时间戳 `19:55:0X` + 彩色代号 + 白字；其中 `saved LAB CHANGELOG · new agent api · shipped today` 一行带**绿底高亮**）
- **右（46-100%）**：`◆ STATE BUILDER`
  - **顶部 stats strip（v2 补记）**：`NEW REPLIES FROM ONE SOURCE 29% · BOT ALOT BUNDLE 0% · SCT TOPIC POSITION 40%` + 绿色 `+0.19`
  - 副标题：`INTERFERENCE · EACH SIGNAL PUSHES THE ODDS`
  - 信号来源列表（带彩色代号）：`JDG FOLLOWED ACCOUNTS IN 2` / `SCT NEW REPLIES / MIN +0#` / `SCT QUOTES · REPOSTS 2.0+` / `MAP FRESH ACCOUNTS 34%` / `MAP REPLIES FROM ONE SOURCE 29%` / `MAP BOT ALOT BUNDLE 0%`
  - `STATE · SUMS TO 1`：三条水平概率条（**条上有白色刻度竖线标记**）
    - `SIGNAL · +68% FIRST` 0.36（绿）
    - `NOISE · NEITHER IN 60 MIN` 0.13（金）
    - `STALE · -30% FIRST` 0.50（红）
  - 按钮：`GUARD PASS`（绿实心）、`1 CLOSE TO A LINE`（金描边）
  - `STATE SPACE`：三元相图（三角形**虚线边**，三顶点 SIGNAL/STALE/NOISE 各带光点，内部一个运动光点带十字线，底部 `CROSSED +4.2% · COSTS 3.5% · NET +0.3%`）
  - `MEASURED · LAST 80`：三条横条 `SIGNAL +68% FIRST 30·0.38` / `NOISE · NEITHER 14·0.18` / `STALE · -30% FIRST 28·0.36` + `SAVE · 0.66`

#### 区块 10 · 页脚（y 2100-2150，97-100%）

- 左：`SPIDERORB v0.0.4 | RESEARCH MODE | ONE SPIDER, 1,400 STATES, ZERO TABS OPEN`（9px 灰）
- 右：`@KRAVN`（设计者水印，**约 16-18px 亮灰，比想象中大**）

### 5.3 栅格与间距参数

| 参数 | 值 |
|---|---|
| 画布宽 | 1320px（3 栏时 ≈ 440/栏） |
| 面板间距 | 10-12px |
| 面板内边距 | 10-14px |
| 标题行高 | ~30px |
| 正文字号 | 10-11px |
| 标签字号 | 9px |
| 统计大数字 | 20-24px |
| 主计数器 | 64-72px |
| 流式行高 | ~110px/行 |
| 智能体图标盒 | ~60×60px |

---

## 6. 动效系统详解

> 以下参数全部由逐帧 diff 分析实测。视频共 356 帧 / 5.9s，**无任何转场动画**——所有动效都是"活数据"驱动的微动态，这是本品 UI 的生命力来源。

### 6.1 动效总清单

| # | 动效 | 位置 | 实测参数 | 实现方式 |
|---|---|---|---|---|
| 1 | 大计数器递增 | LIVE COUNT | 1,329→1,333 / 5.9s（≈+0.68/s），逐位翻滚 | CSS 位移滚动 |
| 2 | UTC 实时钟 | 标题行右侧 | 19:55:03→19:55:09，真实走秒 | `setInterval` + 文本替换 |
| 3 | 头部统计递增 | SOURCES/BLOCKED/INSIGHTS | 10,663→10,688、7,169→7,184、58→60 | 同 #1 |
| 4 | 日志流滚动 | 中栏日志区 | 持续向上滚动，新行推入，行内数值实时变（如 views 377,147→375,556） | 定时 push + transform 位移 |
| 5 | 放射爆发网络 | THE ORB | 细丝呼吸摆动 + 微粒沿丝/轨道缓慢移动 | Canvas rAF |
| 6 | 流线光点 | 五条流式行末端 + FOLLOWED FLOW | 光点沿波形匀速流动 | SVG `getPointAtLength` |
| 7 | 曲线图光点 | WHERE A STORY LANDS | 发光圆点沿曲线移动 + 拖尾 | SVG + `getPointAtLength` |
| 8 | 流式行数值更新 | 左列五行 | 数值与波形末端微更新（秒级） | 数据驱动 + 补间 |
| 9 | 三元图光点 | STATE SPACE | 内部光点缓慢移动 | rAF 插值 |
| 10 | LIVE FEED 标签行 | 区块 3 | **静态**（相位相关实测位移 ≈0） | — |
| 11 | SPIDER-ON 徽章 | 标题行 | **静态**（实测 30 个采样点亮度恒定） | — |

### 6.2 关键动效的实现要点

**① 大计数器（Odometer）**
- 每个数字位一个固定宽度窗口（`overflow:hidden`），内部 0-9 竖排，通过 `translateY` 滚到目标位；
- **千分位逗号是独立格**：按字符建格时遇 `,` 生成窄格，勿在 set 时才处理（复刻时踩过的坑）；
- 数字样式：绿色 `#53CD90` + `text-shadow: 0 0 12px rgba(83,205,144,.8)` 辉光 + 点阵/扫描线纹理叠加；
- 更新节奏：每秒 +0~1，用随机泊松过程模拟"偶发事件"比线性递增更真实。

**② 日志流（LogStream）**
- 容器定高，`overflow:hidden`；每 600-1200ms（随机）从顶部插入新行，整体 `transform: translateY(-lineHeight)`，旧行溢出隐藏；
- 行首代号着色（`[JDG]` 蓝 / `[QNT]` 金 / `[GRD]` 红 / `[WRT]` 白 / `[ORB]` 紫 / `[MEASURED]` 绿），另有**无代号的绿色 `✓` 前缀行**穿插；
- 新行入场可加 150ms 的 `opacity 0→1` + 左侧 2px 亮条闪入；
- 数据池：预置 15-20 条模板（带随机数值插槽），循环取用。

**③ 放射爆发网络（THE ORB）** —— v2 重写
- Canvas 2D，DPR 适配；视觉构成 = **爆发核 + 细丝 + 微粒 + 卫星 hub + 虚线轨道 + 星尘**（不是均匀随机点阵！）；
- 爆发核：中心偏上，radial-gradient 粉白强光晕；
- 细丝：200-400 条，从核心向外辐射，极坐标参数化（角度 + 长度），丝上按弧长布 2-4 个微粒；
- 卫星 hub：6-8 个品红大节点（半径 3-4px，强辉光），各带 2-3 圈**白色虚线椭圆轨道**（`ctx.setLineDash([2,4])`），轨道上 2-3 个微粒绕行（角速度 0.05-0.15 rad/s）；
- 金色大虚线环：1-2 个，半径大、透明度低；
- 星尘：左侧 80-150 个金色小点，静止或极缓漂移，**不连线**；
- 动效：细丝长度/角度做 ±3% 正弦呼吸（相位错开），微粒沿丝缓慢滑动；**不做** O(n²) 距离连线；
- 性能：每帧清屏 + `globalCompositeOperation = "lighter"` 叠加发光。

**④ 曲线图（LineChart）**
- SVG `path` 画曲线，`stroke` 用蓝紫渐变（`#8C90C0 → #A2B3DF`）；
- 光点：`circle` + `filter: blur` 辉光，用 `path.getPointAtLength(t)` 求坐标，t 随时间 0→1 循环；
- 拖尾：在光点后方再画 3-5 个递减透明度的小圆；
- **垂直金色虚线**：在 TRENDING 位置画一条 `stroke-dasharray` 竖线贯穿图表区；曲线中部放 1-2 个方形 marker chip。

**⑤ 流式行（StreamRow）** —— v2 新增，替代原"径向仪表"
- 每条波形是一段预生成的随机游走平滑曲线（SVG path，同色 stroke + 低透明度面积填充）；
- 末端白色光点：`getPointAtLength` 让光点在波形末段往复/循环流动；
- 数值每 1-2s 随机 ±0.1-0.5%，300ms 补间；
- 五条流线的 SVG viewBox 右端向上弯曲，视觉上指向 ORB 核心区。

**⑥ 三元图（TernaryPlot）**
- 等边三角形**虚线边**，三顶点标注 SIGNAL（上）/ STALE（左下）/ NOISE（右下），各带辉光圆点；
- 内部光点位置由 (s, n, st) 三元组决定，三值每秒微调，光点用 rAF 平滑移动，带十字线；
- 底部一行汇总文字 `CROSSED +4.2% · COSTS 3.5% · NET +0.3%`。

### 6.3 动效尺度原则（复刻时必须遵守）

1. **全部动效都是"数据微更新"级别**：位移 ≤ 数像素、透明度变化 ≤ 0.2、无弹跳、无旋转；
2. **禁止出现**：转场、视差、鼠标悬停大动画（概念稿无交互）、**LIVE FEED 跑马灯**（实测静态）；
3. **节奏**：所有更新周期 ≥ 600ms，避免"抖动感"；
4. **可访问性**：加 `prefers-reduced-motion` 媒体查询，命中时冻结所有动画（Canvas 保留一帧静态画面）。

---

## 7. 组件级实现指南

### 7.1 项目骨架建议

```
spiderorb-ui/
├── index.html          # 单文件演示（推荐起步）
├── styles.css          # 或 Tailwind + 自定义 CSS 变量
├── app.js              # 数据模拟 + 动效调度
└── assets/
    ├── orb.js          # THE ORB 放射爆发网络
    └── charts.js       # 曲线/流式行/三元图
```

### 7.2 CSS 变量（设计令牌）

```css
:root {
  /* 中性色 */
  --bg-page: #0D111A;
  --bg-panel: #12151E;
  --bg-panel-2: #161922;
  --border: #24222B;
  --border-2: #2A2E3A;
  --text-1: #E5E3E7;
  --text-2: #8B868D;
  --text-3: #555966;
  /* 语义色 */
  --green: #53CD90;  --green-dim: #33AE6A;
  --red: #AC364E;    --red-dim: #8D3E44;
  --magenta: #D8425D; --pink: #B85888;
  --gold: #FFDD83;   --gold-dim: #DEA248;
  --blue: #A2B3DF;   --blue-dim: #8C90C0;
  --cyan: #D5FFFB;
  /* 字体 */
  --font-pixel: "Silkscreen", "Pixelify Sans", monospace;
  --font-mono: "Share Tech Mono", "JetBrains Mono", ui-monospace, monospace;
  /* 间距 */
  --gap: 10px;
  --panel-pad: 12px;
}
```

### 7.3 面板组件

```html
<section class="panel">
  <header class="panel-hd">
    <span class="diamond"></span> CORE MODEL
    <span class="panel-meta">IN TRAINING</span>
  </header>
  <div class="panel-bd"><!-- 内容 --></div>
</section>
```

```css
.panel { background: var(--bg-panel); border: 1px solid var(--border); }
.panel-hd {
  display: flex; align-items: center; gap: 8px;
  padding: 8px var(--panel-pad);
  font: 600 11px/1 var(--font-mono);
  letter-spacing: .08em; text-transform: uppercase;
  color: var(--text-1);
  border-bottom: 1px solid var(--border);
}
/* ◆ 用 CSS 画, 避免字体子集缺字 */
.diamond { width: 7px; height: 7px; background: var(--text-2); transform: rotate(45deg); }
.panel-meta { margin-left: auto; color: var(--text-2); font-size: 9px; letter-spacing: .12em; }
.panel-bd { padding: var(--panel-pad); }
```

### 7.4 徽章组件

```css
.badge { font: 600 9px/1 var(--font-mono); letter-spacing: .1em; padding: 3px 6px; text-transform: uppercase; }
.badge--solid-red  { background: var(--red); color: #0D111A; }       /* LIVE FEED / SPIDER-ON */
.badge--outline-blue { border: 1px solid var(--blue-dim); color: var(--blue); } /* 1,400 STATES / RUNS 54... */
.badge--magenta    { background: var(--pink); color: #0D111A; }      /* SPIDER CAUGHT IT */
.badge--outline-magenta { border: 1px solid var(--magenta); color: var(--magenta); } /* /agent_spider */
.badge--outline-gold { border: 1px solid var(--gold-dim); color: var(--gold); }  /* 1 CLOSE TO A LINE */
.badge--solid-green { background: var(--green); color: #0D111A; }    /* GUARD PASS / STRONG FLOW */
```

### 7.5 数码管计数器

```html
<div class="odometer" id="liveCount"><!-- JS 按字符建格 --></div>
```

```css
.odometer { display: flex; font: 64px/1 var(--font-pixel); color: var(--green);
  text-shadow: 0 0 14px rgba(83,205,144,.75); position: relative; }
.odometer::after { /* 扫描线 */ content: ""; position: absolute; inset: 0;
  background: repeating-linear-gradient(0deg, transparent 0 3px, rgba(13,17,26,.35) 3px 4px);
  pointer-events: none; }
.od-cell { width: .62em; height: 1em; overflow: hidden; }
.od-cell i { display: block; font-style: normal; transition: transform .35s cubic-bezier(.22,1,.36,1); }
.od-comma { width: .3em; }
```

```js
// build(): 按字符建格——遇 ',' 生成 .od-comma 窄格, 遇数字生成内含 0-9 竖排的 .od-cell
// set():    数字格 translateY(-n em) 滚动
function buildOdometer(el, value) {
  el.innerHTML = "";
  for (const ch of String(value)) {
    if (ch === ",") { const c = document.createElement("span");
      c.className = "od-comma"; c.textContent = ","; el.appendChild(c); continue; }
    const cell = document.createElement("span"); cell.className = "od-cell";
    const inner = document.createElement("i");
    for (let d = 0; d < 10; d++) { const s = document.createElement("s"); s.textContent = d; inner.appendChild(s); }
    cell.appendChild(inner); el.appendChild(cell);
  }
}
```

### 7.6 日志流

```js
const LOG_TPL = [
  { tag: "QNT", color: "var(--gold)",  text: () => `+${rnd(100,900)}ms net value +${(Math.random()*20).toFixed(1)}M · read 0 min · ${(Math.random()*3).toFixed(1)}K words` },
  { tag: "GRD", color: "var(--red)",   text: () => `+${rnd(300,900)}ms PASS · 0 red flags · bots ${rnd(1,5)}%` },
  { tag: "WRT", color: "var(--text-1)",text: () => `+${rnd(900,1400)}ms note written · thread draft +${rnd(1,5)} · source linked` },
  { tag: "ORB", color: "var(--blue-dim)", text: () => `+${rnd(250,450)}ms insight saved · decision record written` },
  { tag: "MEASURED", color: "var(--green)", text: () => `SIGNAL · +${rnd(40,80)}% first at ${rnd(8,20)} min — training set` },
  { tag: "✓",  color: "var(--green)",  text: () => `insight saved · decision record written` },  // 无代号绿勾行
];
```

```css
.log-stream { height: 96px; overflow: hidden; font: 10px/1.9 var(--font-mono); }
.log-row { white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  animation: logIn .25s ease both; }
@keyframes logIn { from { opacity: 0; transform: translateX(-6px); } }
```

### 7.7 THE ORB 放射爆发网络 —— v2 重写

```js
// 结构: 爆发核 + N条细丝(极坐标) + 丝上微粒 + M个卫星hub(虚线轨道+绕行微粒) + 金色星尘
const canvas = document.getElementById("orb");
const ctx = canvas.getContext("2d");
const DPR = Math.min(devicePixelRatio, 2);
let W, H, filaments = [], hubs = [], dust = [], t = 0;

function build() {
  const cx = W * .5, cy = H * .42;                       // 核心偏上
  // 细丝: 角度扇形分布 + 长度随机, 丝上 2-4 个微粒(弧长参数 u)
  filaments = Array.from({ length: 320 }, () => {
    const ang = rnd(-Math.PI * .95, Math.PI * .05);      // 主要向下/两侧扇出
    return { ang, len: rnd(H * .18, H * .52), wob: rnd(0, Math.PI * 2),
             beads: Array.from({ length: rnd(2, 4) }, () => ({ u: Math.random(), v: rnd(.02, .08) })) };
  });
  // 卫星 hub: 品红大节点 + 2-3 圈虚线椭圆轨道 + 绕行微粒
  hubs = Array.from({ length: 7 }, () => {
    const ang = rnd(0, Math.PI * 2), dist = rnd(H * .18, H * .42);
    return { x: cx + Math.cos(ang) * dist, y: cy + Math.sin(ang) * dist * .8,
             r: rnd(3, 4.5), orbits: [rnd(14, 22), rnd(26, 38)],
             sats: Array.from({ length: rnd(2, 3) }, (_, i) => ({ o: i % 2, a: rnd(0, 6.28), w: rnd(.05, .15) })) };
  });
  // 金色星尘(左侧, 静止)
  dust = Array.from({ length: 120 }, () => ({ x: rnd(0, W * .45), y: rnd(0, H), r: rnd(.5, 1.4) }));
}

function frame() {
  t += .016;
  ctx.clearRect(0, 0, W, H);
  ctx.globalCompositeOperation = "lighter";
  const cx = W * .5, cy = H * .42;
  // 核心光晕
  const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, 80);
  g.addColorStop(0, "rgba(255,240,246,.85)"); g.addColorStop(.35, "rgba(216,66,93,.35)");
  g.addColorStop(1, "transparent");
  ctx.fillStyle = g; ctx.fillRect(cx - 90, cy - 90, 180, 180);
  // 星尘
  ctx.fillStyle = "rgba(255,221,131,.5)";
  for (const d of dust) { ctx.beginPath(); ctx.arc(d.x, d.y, d.r, 0, 7); ctx.fill(); }
  // 细丝 + 丝上微粒
  ctx.lineWidth = .5;
  for (const f of filaments) {
    const breathe = 1 + Math.sin(t * .8 + f.wob) * .03;  // 相位错开的呼吸
    const ex = cx + Math.cos(f.ang) * f.len * breathe, ey = cy + Math.sin(f.ang) * f.len * breathe;
    ctx.strokeStyle = "rgba(162,179,223,.14)";
    ctx.beginPath(); ctx.moveTo(cx, cy); ctx.lineTo(ex, ey); ctx.stroke();
    for (const b of f.beads) {
      b.u = (b.u + b.v * .016) % 1;                      // 沿丝滑动
      ctx.fillStyle = "rgba(229,227,231,.8)";
      ctx.beginPath(); ctx.arc(cx + (ex - cx) * b.u, cy + (ey - cy) * b.u, 1, 0, 7); ctx.fill();
    }
  }
  // 卫星 hub + 虚线轨道 + 绕行微粒
  for (const h of hubs) {
    ctx.strokeStyle = "rgba(229,227,231,.28)"; ctx.setLineDash([2, 4]);
    for (const r of h.orbits) { ctx.beginPath(); ctx.ellipse(h.x, h.y, r, r * .45, -.5, 0, 7); ctx.stroke(); }
    ctx.setLineDash([]);
    ctx.fillStyle = "#D8425D"; ctx.shadowColor = "#D8425D"; ctx.shadowBlur = 12;
    ctx.beginPath(); ctx.arc(h.x, h.y, h.r, 0, 7); ctx.fill(); ctx.shadowBlur = 0;
    for (const s of h.sats) {
      s.a += s.w * .016;
      const r = h.orbits[s.o];
      ctx.fillStyle = "rgba(255,221,131,.9)";
      ctx.beginPath(); ctx.arc(h.x + Math.cos(s.a) * r, h.y + Math.sin(s.a) * r * .45, 1.2, 0, 7); ctx.fill();
    }
  }
  requestAnimationFrame(frame);
}
```

> 与初版"均匀粒子 + 距离连线"的本质区别：**无 O(n²) 连线**、微粒位置由"丝/轨道"参数化驱动、星尘不连线。视觉密度靠细丝数量（320+）而非粒子数。

### 7.8 曲线图 + 运动光点

```js
const path = document.querySelector("#curvePath");
const dot = document.querySelector("#curveDot");
const len = path.getTotalLength();
let prog = 0;
(function move() {
  prog = (prog + .0016) % 1;                       // ~10s 走完一圈
  const pt = path.getPointAtLength(prog * len);
  dot.setAttribute("cx", pt.x); dot.setAttribute("cy", pt.y);
  // 拖尾: 3 个小圆, 取 prog-0.01/-0.02/-0.03
  requestAnimationFrame(move);
})();
```

```html
<svg viewBox="0 0 480 220">
  <path id="curvePath" d="M20,180 C120,170 180,60 260,55 S400,40 460,18"
        fill="none" stroke="url(#curveGrad)" stroke-width="2"/>
  <defs><linearGradient id="curveGrad" x1="0" x2="1">
    <stop offset="0" stop-color="#8C90C0"/><stop offset="1" stop-color="#A2B3DF"/>
  </linearGradient></defs>
  <!-- TRENDING 垂直金色虚线 -->
  <line x1="400" y1="10" x2="400" y2="190" stroke="#FFDD83" stroke-width="1" stroke-dasharray="3 4" opacity=".6"/>
  <circle id="curveDot" r="5" fill="#D5FFFB" filter="url(#glow)"/>
  <!-- 曲线中部方形 marker chip -->
  <rect x="180" y="90" width="10" height="10" fill="none" stroke="#53CD90"/>
</svg>
```

### 7.9 流式行（StreamRow）—— v2 新增，替代原"径向仪表"

```html
<div class="stream-row" style="--sc: var(--magenta)">
  <i class="edge"></i>
  <div class="sr-head">
    <div><b>BOT SHARE</b><small>LIKES FROM BOT FARMS</small></div>
    <span class="sr-val">28.6%</span>
    <span class="sr-chip">[MAP]</span>
  </div>
  <svg class="sr-line" viewBox="0 0 300 28" preserveAspectRatio="none">
    <path class="sr-area" d="M0,22 C40,20 80,24 120,18 S220,14 268,10 L300,4 L300,28 L0,28 Z"/>
    <path class="sr-wave" d="M0,22 C40,20 80,24 120,18 S220,14 268,10 L300,4" id="wave2"/>
    <circle class="sr-dot" r="3" fill="#fff"/>
  </svg>
</div>
```

```css
.stream-row { position: relative; padding: 10px 12px 8px 16px; border-bottom: 1px solid var(--border); }
.stream-row .edge { position: absolute; left: 0; top: 10px; bottom: 10px; width: 3px; background: var(--sc); }
.sr-head { display: flex; align-items: baseline; gap: 10px; }
.sr-head b { font: 700 12px/1.2 var(--font-mono); letter-spacing: .1em; color: var(--text-1); display: block; }
.sr-head small { font: 8px/1 var(--font-mono); color: var(--text-3); letter-spacing: .14em; }
.sr-val { margin-left: auto; color: var(--sc); font-size: 13px; }
.sr-chip { border: 1px solid var(--sc); color: var(--sc); font-size: 8px; padding: 2px 4px; letter-spacing: .1em; }
.sr-line { width: 100%; height: 28px; display: block; margin-top: 6px; }
.sr-wave { fill: none; stroke: var(--sc); stroke-width: 1.5; }
.sr-area { fill: var(--sc); opacity: .08; }
```

```js
// 光点在波形末段往复流动
const wave = document.querySelector("#wave2"), dot2 = document.querySelector(".sr-dot");
const L = wave.getTotalLength(); let u = .7, dir = 1;
(function flow() {
  u += dir * .0012; if (u > .99 || u < .7) dir *= -1;
  const p = wave.getPointAtLength(u * L);
  dot2.setAttribute("cx", p.x); dot2.setAttribute("cy", p.y);
  requestAnimationFrame(flow);
})();
```

### 7.10 三元图

```js
// 顶点: 上 (0.5,0.06) 左下 (0.08,0.9) 右下 (0.92,0.9), 三角形边用 stroke-dasharray 虚线
// 光点坐标 = 重心坐标: x = a*0.5 + b*0.08 + c*0.92, y = a*0.06 + b*0.9 + c*0.9
// (a,b,c) 为 SIGNAL/STALE/NOISE 且 a+b+c=1, 每秒微调后 rAF 补间; 光点带十字线
```

### 7.11 决策行（DECISION STACK）—— v2 重写为盒子行

```html
<div class="decision decision--save">
  <div class="dc-main">
    <span class="dc-act">SAVE · JDG</span>
    <span class="dc-name">OPEN-WEIGHTS EVAL</span>
    <span class="dc-spdr">SPDR <b>0.83</b></span>
  </div>
  <div class="dc-sub">·· 64 DAYS LEFT ··</div>
  <div class="dc-bar"><i style="width:83%"></i></div>
</div>
```

```css
.decision { border: 1px solid var(--border); padding: 7px 10px 0; margin-bottom: 6px;
  font: 10px/1.4 var(--font-mono); }
.dc-main { display: flex; align-items: baseline; gap: 10px; }
.dc-act { font-weight: 700; letter-spacing: .06em; }
.decision--save  .dc-act { color: var(--green); }
.decision--skip  .dc-act { color: var(--gold); }
.decision--block .dc-act { color: #E0607C; }
.dc-name { color: var(--text-1); font-weight: 700; letter-spacing: .04em; }
.dc-spdr { margin-left: auto; color: var(--magenta); font-size: 9px; }
.dc-spdr b { color: var(--text-1); font-weight: 400; }
.dc-sub { color: var(--text-3); font-size: 8.5px; letter-spacing: .14em; margin: 2px 0 5px 92px; }
.dc-bar { height: 2px; background: var(--border-2); margin: 0 -10px; }
.dc-bar i { display: block; height: 100%; background: var(--gold); opacity: .8; transition: width .3s; }
```

### 7.12 发散箭头面板（SIGNALS → OUTCOMES）—— v2 新增

```html
<div class="diverge">
  <div class="dv-row dv--up"><i style="width:60%"></i><span>+0.30</span></div>  <!-- 绿右箭头 -->
  <div class="dv-row dv--up"><i style="width:52%"></i><span>+0.30</span></div>
  <div class="dv-row dv--dn"><i style="width:34%"></i><span>-0.31</span></div>  <!-- 红左箭头 -->
</div>
```

```css
.dv-row { display: flex; align-items: center; gap: 8px; font: 9px/2 var(--font-mono); }
.dv-row i { height: 4px; position: relative; }
.dv--up i { background: var(--green); }
.dv--up i::after { content: ""; position: absolute; right: -6px; top: -3px;
  border: 5px solid transparent; border-left-color: var(--green); }  /* CSS 三角箭头 */
.dv--dn i { background: var(--red); }
.dv--dn i::before { content: ""; position: absolute; left: -6px; top: -3px;
  border: 5px solid transparent; border-right-color: var(--red); }
.dv--up span { color: var(--green); } .dv--dn span { color: #E0607C; }
```

### 7.13 智能体图标盒（SIX AGENTS）—— v2 修正

```html
<div class="agent" style="--ac: var(--green)">
  <div class="agent-box"><!-- SVG 图标, 盒内居中 --></div>
  <span class="agent-cd">JDG</span>
  <span class="agent-nm">JUDGE</span>
  <span class="agent-vl">33</span>
  <span class="agent-sub">SOURCE LIST</span>
</div>
```

```css
.agent { display: flex; flex-direction: column; align-items: center; gap: 4px; text-align: center; }
.agent-box { width: 60px; height: 60px; border: 1px solid var(--ac);
  display: flex; align-items: center; justify-content: center;
  box-shadow: inset 0 0 12px rgba(255,255,255,.03), 0 0 10px color-mix(in srgb, var(--ac) 25%, transparent); }
.agent-cd { color: var(--ac); font: 700 9px/1 var(--font-mono); letter-spacing: .12em; }
.agent-nm { color: var(--text-1); font-size: 9px; letter-spacing: .08em; }
.agent-vl { color: var(--text-1); font-size: 18px; font-variant-numeric: tabular-nums; }
.agent-sub { color: var(--text-3); font-size: 7px; letter-spacing: .1em; }
```

卡片上方总线装饰：一条 `border-top` 横线（标 `OBSERVATIONS IN` / `DECISIONS OUT`），每个 `.agent-box` 顶部用 1px 竖线（伪元素）接上横线。

---

## 8. 完整技术方案与落地陷阱

### 8.1 推荐技术栈（两档）

| 方案 | 适用 | 组成 |
|---|---|---|
| **A · 单文件 HTML**（推荐入门） | 快速复刻、演示 | 原生 HTML + CSS 变量 + 原生 JS + Canvas + SVG，零依赖，`index.html` 一个文件跑通 |
| **B · React 工程** | 生产化 | Vite + React + Tailwind（配 CSS 变量）+ Framer Motion（补间）+ 自研 Canvas 组件 |

### 8.2 数据模拟层

```js
// 中央数据 store: 每 600-1500ms 随机更新各面板数值, 触发对应组件补间
const store = {
  liveCount: 1329, sources: 10663, blocked: 7169, insights: 58,
  streams: { followedFlow: 5, botShare: .286, hypeRisk: .44, replyDepth: .0714, topicPhase: .10 },
  state: { signal: .36, noise: .13, stale: .50 },
  decisions: [ /* 7 行 */ ],
};
setInterval(tick, 1000);   // tick() 内对所有数值做微小随机扰动并通知订阅者
```

### 8.3 动效调度

- 所有定时器统一由一个 `Scheduler` 管理（避免 setInterval 漂移）；
- 日志流 600-1400ms 随机、计数器 1000ms、流式行数值 1000-2000ms、三元图 rAF；
- `document.hidden` 时暂停 rAF 与定时器（省电）。

### 8.4 响应式

- 设计基准宽 1320px；Web 端用 `max-width: 1320px` 居中；
- 三栏 → 两栏（< 1024px）→ 单栏（< 640px，纵向堆叠，顺序：标题行 → 计数器 → ORB → 其余）；
- 细丝/星尘数量按屏宽动态缩放。

### 8.5 性能预算

| 项 | 预算 |
|---|---|
| 首屏 | < 1s（零依赖方案） |
| ORB 帧率 | ≥ 55fps（320 细丝 + 7 hub / DPR≤2） |
| 内存 | < 150MB |

### 8.6 落地陷阱（v2 新增 —— 复刻验证中真实踩过的坑）

1. **定时器"立即执行一次"会放大初始化异常**：`every(ms, fn)` 若设计成注册即执行，`fn` 内任何 null 引用都会**静默中断后续全部初始化**（症状：部分区块整块空白但页面"看起来正常"）。对策：初始化与定时更新分离；并写**断言脚本逐项数元素**（如 `document.querySelectorAll('.agent').length === 6`），不能只靠截图目视验收。
2. **Canvas + ResizeObserver 的清空竞态**：给 `canvas.width` 赋值会清空画面；ResizeObserver 回调是异步的，若在 `prefers-reduced-motion` 模式（无 rAF 循环）下发生，画面会被清空后无人重绘。对策：`resize()` 后主动 `draw()` 一帧。
3. **TDZ 时序**：同步路径（如 reduced-motion 下的立即 `draw()`）中引用的 `const` 必须先声明后调用。
4. **Google Fonts latin 子集不含 `◆ ▲ ★ ✓`**：这些符号用 CSS 图形绘制（菱形=旋转方块、箭头=border 三角、勾=伪元素），保证渲染一致。
5. **计数器千分位逗号**：建格时就要把 `,` 作为独立窄格处理，否则数值从 999→1,000 时位数变化导致布局跳动或逗号丢失。

---

## 9. 可直接使用的 AI 生成提示词

将以下 prompt 直接投喂给 v0 / Cursor / Claude / 文心快码等，可生成高还原度初版：

```
请创建一个单文件 HTML 页面，复刻一个名为 "SPIDERORB // RESEARCH BOT" 的赛博朋克终端仪表盘 UI（移动竖屏优先，设计基准宽 1320px）。

【整体风格】
- 深色 FUI/HUD 终端美学，近黑背景 #0D111A，面板 #12151E，1px 边框 #24222B，零圆角
- 全场等宽字体（Google Fonts: Silkscreen 标题 + Share Tech Mono 正文），全部大写，letter-spacing 0.06-0.12em
- 高信息密度：单屏 10+ 模块，像"机器在工作"的实时监控台
- 语义色：绿 #53CD90=SAVE/信号，红 #AC364E=BLOCK/停滞，金 #FFDD83=SKIP/QUANT，蓝紫 #A2B3DF=链接/MAP，品红 #D8425D=热点/SPDR
- 全局质感层（透明度≤5%）：细噪点 + 水平扫描线 + 四角暗角 + 标题级文字 2-6px 辉光
- ◆■▲★✓ 等符号一律用 CSS 图形绘制，不依赖字体

【布局（自上而下）】
1. 顶部微文本行：◆ SIGNALS · PROBABILITY STATES · COLLAPSE · SIX AGENTS + [1,400 STATES] 蓝描边徽章
2. 主标题行：蓝紫渐变圆角 LOGO(SO, 带辉光) + "SPIDERORB // RESEARCH BOT" 像素粗体标题；右侧 SOURCES 10,663 / 24H BLOCKED 7,169 / INSIGHTS 58 三组统计、UTC 实时钟、红底 [SPIDER-ON] 徽章
3. LIVE FEED 静态标签行（不要滚动）：◆LIVE FEED -28.9%(红徽章) | REPO AGENT-SDK +412★(金) | SAVE EVAL REPORT | BLOCK FREE GPU AIRDROP(红) | CURVE HN #1 81%(青)
4. 三栏面板：CORE MODEL/IN TRAINING | LIFE OF A DECISION(标题区右侧蓝描边chip "RUNS 54 · SAVE 23 · BLOCK 4 · SKIP 27" + 彩色日志行) | ENTANGLEMENT MAP(散点, 右meta "12 LINKED" 品红)
5. 计数器+日志行：左侧 64px 绿色点阵LED风格大计数器(辉光+扫描线, 逐位翻滚递增, 千分位逗号独立格)，下方 ▲ SIGNAL 25% · NOISE 66% · STALE 25% 和 COUNTER 40 · VETO 71% · OPEN 2/6；中间实时日志流(0.6-1.4s 随机推入, 代号着色, 穿插绿✓行, 底部终端行 spdr@Lab:~$ orb trace RUN-004228 --from-post)；右侧小散点图
6. 统计+曲线行：左统计列(40/71%/IN PLAY 2/6; EVERY 1,000 SOURCES SCANNED 1,000/992/10/2; FOLLOWED FLOW 流线; SOURCE RATE 13.0/M; PAPER INFLOW 154 金色sparkline) | 中 "WHERE A STORY LANDS ON THE CURVE"(蓝紫渐变曲线+运动光点拖尾+右端金色垂直虚线+方形marker chip+SPIDER CAUGHT IT 品红徽章+REACH 51.5K+AT 5/30/60 MIN 三行) | 右 LAB CHANGELOG(GUARD PASS 绿实心徽章+图例+三行概率条)
7. THE ORB 标题行(右侧品红描边chip "/agent_spider — ON THE ORB · SITES 12") + 主视觉行：
   - 左列：五条流式sparkline行(StreamRow)：左侧3px彩色竖条+白粗体标签+暗灰副标签+彩色数值+boxed代号chip+横贯波浪线(同色+淡填充)+末端白色光点沿线流动；五行为 FOLLOWED FLOW 5 绿/BOT SHARE 28.6% 品红/HYPE RISK 44% 金/REPLY DEPTH 7.14% 蓝紫/TOPIC PHASE 10.0% 白；流线右端上扬汇入中央网络
   - 中间：Canvas 放射爆发网络(中心偏上粉白强光晕爆发核+320条白色细丝极坐标辐射+丝上微粒滑动+7个品红发光卫星hub(各带2-3圈白色虚线椭圆轨道+绕行微粒)+左下金色大虚线环+左侧金色星尘不连线+核心上方金色斜角贴纸"ad for a course"；细丝相位错开呼吸±3%；不做距离连线)
   - 右列：DECISION STACK 七行盒子行(完整边框+彩色行动词SAVE绿/SKIP金/BLOCK红+代号+粗体标题+品红SPDR值+暗灰副标题+行底金色细进度条)；下方 SIGNALS→OUTCOMES 发散箭头面板(+1.84 REINFORCE 绿右箭头×3 / -0.74 CANCEL 红左箭头×2 / STRONG FLOW 绿chip)
8. 底部行：左 SIX AGENTS(上方 OBSERVATIONS IN/DECISIONS OUT 总线+竖线连接; 六张卡: 60px彩色边框图标盒+彩色代号+白名称+大数值+暗灰小标签; SCT/MAP/JDG/QNT/GRD/WRT) + LAST ACTIONS 带时间戳日志(一行绿底高亮)；右 STATE BUILDER(顶部 stats strip + INTERFERENCE 副标题 + 带彩色代号的信号来源列表 + STATE SUMS TO 1 三条带刻度概率条 + GUARD PASS/1 CLOSE TO A LINE 按钮 + STATE SPACE 虚线三元相图带运动光点十字线 + MEASURED LAST 80 横条)
9. 页脚：SPIDERORB v0.0.4 | RESEARCH MODE | ONE SPIDER, 1,400 STATES, ZERO TABS OPEN + @KRAVN(16px亮灰水印)

【动效要求】
- 大计数器每秒 +0~1 递增，逐位翻滚(translateY, 0.35s ease-out)
- UTC 钟实时走秒；头部统计递增
- 日志流持续滚动推入
- ORB 细丝呼吸 + 微粒沿丝/轨道移动（无连线动画）
- 流式行末端光点流动；曲线光点沿路径循环带拖尾
- 所有数据每 1-2s 微小随机更新并补间过渡
- 无转场无弹跳无跑马灯，全部是"活数据"级微动态
- prefers-reduced-motion：冻结动画但 Canvas 保留静态一帧

【技术约束】
- 单 HTML 文件，原生 JS + Canvas + SVG，零外部依赖(除 Google Fonts)
- CSS 变量管理设计令牌
- 移动竖屏 1320px 基准，窄屏降级为单栏
- resize 后 Canvas 主动重绘一帧；初始化与定时器分离
```

---

## 10. 附录

### 10.1 参考图索引

| 文件 | 内容 |
|---|---|
| `frames/t000s.jpg` ~ `t005s.jpg` | 0s/1s/2s/3s/4s/5s 全帧原图 |
| `frames/contact_sheet.jpg` | 60 帧序列缩略总览（观察动效） |
| `crops/33_grid_full.png` | 带 y 轴标尺的半分辨率全图（布局定位） |
| `verify/deep/01~10_*.png` | **原图 vs Demo 逐区块并排对比（上原下仿）** |
| `verify/deep/x_orig_streams.png` | 流式行 ×5 全分辨率放大（v2 关键证据） |
| `verify/deep/x_orig_orb.png` | THE ORB 爆发结构全分辨率放大（v2 关键证据） |
| `verify/deep/x_orig_agents.png` | SIX AGENTS 图标盒放大 |
| `verify/deep/x_orig_sigout.png` | 决策栈行 + SIGNALS→OUTCOMES 放大 |
| `verify/side_by_side.png` | 整页并排对比 |
| `verify/motion_diff_t0_t3.png` | Demo 动效差异热力图 |
| `crops/02_toprow.png` / `20_p1` `21_p2` `22_p3` | 标题行/标签行/三栏面板 |
| `crops/03_ticker.png` / `31_log_t000s.png` | 计数器 + 日志流 |
| `crops/04_orb_top.png` / `50_curve_f0000.png` | 统计列 + 曲线图 |
| `crops/10_bottom_row.png` / `24_state.png` | SIX AGENTS + STATE BUILDER |
| `crops/41_log_stack.png` / `46_counter_stack.png` | 日志流/计数器 4 时间点对比栈 |

### 10.2 界面文本清单（演示数据填充用）

```
SPIDERORB // RESEARCH BOT · x/ARKIV · GITHUB · +SIGNAL FIRST · +NOISE IN 60 MIN
SIGNALS · PROBABILITY STATES · COLLAPSE · SIX AGENTS · 1,400 STATES
SPDR-LIGHTBOT v0.0.4 · LOBEL-MEASURED TODAY · RESEARCH MODE
RUNS 54 · SAVE 23 · BLOCK 4 · SKIP 27
SPIDER CAUGHT IT · 35 MIN · TRENDING · ON EVERY FEED · HYPE 6S
OUR NOTE 12 MIN IN · 2.3K VIEWS · RESULT +47.2% FIRST TO POST
SPIDER · 0.6 SEC TO READ · 1 BITE · ad for a course
FOLLOWED FLOW · FOLLOWED POSTS · LA | BOT SHARE · LIKES FROM BOT FARMS
HYPE RISK · FROM REPLY HYPERS | REPLY DEPTH · REAL REPLIES IN | TOPIC PHASE · ABOVE OPEN 24H
MEASURED OUTCOMES · 1,329 LABELS · OBSERVATIONS IN · DECISIONS OUT 4
INTERFERENCE · EACH SIGNAL PUSHES THE ODDS
NEW REPLIES FROM ONE SOURCE 29% · BOT ALOT BUNDLE 0% · SCT TOPIC POSITION 40%
+1.84 REINFORCE · -0.74 CANCEL · STRONG FLOW · 7 SIGNALS 3 OUTCOMES
ONE SPIDER, 1,400 STATES, ZERO TABS OPEN · @KRAVN
```

### 10.3 复刻验收清单（v2）

- [ ] 深色底 + 1px 细边框面板 + 零圆角，`◆` 面板前缀（CSS 绘制）
- [ ] 等宽字体、全大写、letter-spacing 到位
- [ ] 语义色严格遵守映射表（SAVE 绿 / BLOCK 红 / SKIP 金 / SPDR 品红）
- [ ] 全局质感层（噪点/扫描线/暗角/文字辉光，透明度 ≤5%）
- [ ] 11 个区块齐全，三栏栅格比例正确（ORB 区占内容高 ~38%）
- [ ] 大计数器逐位翻滚递增 + 辉光 + 扫描线 + 千分位逗号独立格
- [ ] UTC 实时钟、日志流滚动（含绿 ✓ 行）、头部统计递增
- [ ] LIVE FEED 行静止（无跑马灯）
- [ ] THE ORB：爆发核 + 320 细丝 + 丝上微粒 + 7 卫星 hub 虚线轨道 + 金色星尘（无距离连线）
- [ ] 五条流式 sparkline 行（竖条/副标签/boxed chip/末端光点/上扬汇入 ORB）
- [ ] 曲线图运动光点 + 拖尾 + TRENDING 垂直虚线 + 方形 marker
- [ ] 决策栈七行盒子行（边框 + 副标题 + SPDR 品红 + 行底金进度条）
- [ ] SIGNALS→OUTCOMES 发散箭头面板（绿右红左）
- [ ] SIX AGENTS 图标盒 + 总线装饰；LAST ACTIONS 一行绿底高亮
- [ ] STATE BUILDER 顶部 stats strip + 刻度概率条 + 虚线三元图
- [ ] 全部动效为"数据微更新"级，无转场
- [ ] `prefers-reduced-motion` 降级（Canvas 保留静态帧）
- [ ] **断言脚本验收**：逐区块元素计数 + 双时间点差分，不只靠目视

---

*v2 修订说明：初版基于 356 帧逐帧 diff 与分区放大比对；v2 依据复刻 Demo 与原视频的逐区块并排放大对比（《深度对比分析-视频vsDemo.md》）修正 3 处组件级误判（流式行/ORB 结构/决策栈行）、补记 6 处细节缺失、更正代号笔误与水印写法（@KRAVN）、新增质感层与落地陷阱章节。色值为视频帧实测（压缩会致轻微偏差，实现时按明度关系微调饱和度即可）。*
