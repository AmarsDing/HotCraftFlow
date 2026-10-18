# DOTS + GPT-6.1 UI 复刻指南

> 基于对视频 `4e2288642875dde64a6f9921a93b7023_raw.mp4`（26.38s / 1320×2868 / 59.66fps / 手机竖屏录屏）的逐帧分析。
> 本文档深入拆解视频中两类核心界面的布局、样式、动效与设计语言，并给出可直接指导 AI 编码的前端实现方案。
>
> **v2（2026-10-17）**：已按本文档完成前端复刻并逐区域对比验证（16/16 项通过），验证中发现的偏差已回写至对应章节（修正处标注 **（v2）**），新增[第 10 章 复刻验证记录](#10-复刻验证记录v2)。复刻工程见 `replica/`，对比材料见 `compare/`。
>
> **v3（2026-10-17，本轮）**：把复刻结果与原视频帧做**逐区域放大对比**（原帧与复刻同坐标切片叠加），发现并修正 11 处**结构性误读**（不只参数微调）——其中 5 处是初版"看错结构"：作战室整体是**红色描边容器**、Agent 卡是**一套 4×2 网格**而非两行两类卡、顶部参数区是**填充条内嵌标签**而非独立芯片卡、ACCOUNT 面板是**星座网络**而非折线、X 帖子头部是**叠压**在容器之上。另有 `NODES 20`（非 220）等关键数据更正。修正处标注 **（v3）**，详见[第 11 章 细节对比修订记录](#11-细节对比修订记录v3)。

---

## 目录

1. [软件识别与 GitHub 地址](#1-软件识别与-github-地址)
2. [视频内容结构](#2-视频内容结构)
3. [设计语言总览](#3-设计语言总览)
4. [视图 A：Agent 执行图谱（graph 视图）拆解](#4-视图-aagent-执行图谱graph-视图拆解)
5. [视图 B：DOTS + GPT-6.1 作战室拆解](#5-视图-bdots--gpt-61-作战室拆解)
6. [动效系统全解](#6-动效系统全解)
7. [前端实现指南（供 AI 编码）](#7-前端实现指南供-ai-编码)
8. [设计 Token 速查表](#8-设计-token-速查表)
9. [复刻验收清单](#9-复刻验收清单)
10. [复刻验证记录（v2）](#10-复刻验证记录v2)
11. [细节对比修订记录（v3）](#11-细节对比修订记录v3)

---

## 1. 软件识别与 GitHub 地址

### 1.1 视频里是什么软件

视频是手机录屏，内容是 X（Twitter）用户 **Logics（@immortalhowwl，蓝 V 认证）** 发布的一条帖子，帖子文案：

> 天啊，我给我的终端加了个网络爬虫，结果它一夜之间收盘在 $8,980...

帖子中展示的软件是 **OpenAI Dots** —— OpenAI 于 2026-09-29 DevDay 发布的"常驻型 AI 同事"（always-on agent）：

- 每个 Dot 运行在独立的云端 Linux 虚拟机 + 无头浏览器中，7×24 小时后台执行长期目标；
- 由 **GPT-6 Astra / GPT-6.1 Sol** 模型驱动（视频标题栏即 "DOTS + GPT-6.1"）；
- 每个 Dot 有名字和卡通形象（视频中为青蛙/小熊等吉祥物），可组成"Agent 团队"；
- 可连接 4000+ 应用（Gmail / Google Drive / GitHub / Calendar / Slack 等）；
- 敏感操作有"人工审批门"（human-in-the-loop gate），视频中多次出现 `AWAIT USER`、`human owns the post`、`APPROVAL · YOU` 等字样，正是这一机制的可视化。

视频 0–18.5s 展示的另一个界面（左上角标注 `kiosa · @thegreatest_sv`）是同一生态下的 **Agent 执行轨迹图谱工具**（agent execution graph / observability kiosk），呈现 `jev-cascade` 级联执行过程中的节点、边、成本与门禁事件。

### 1.2 GitHub 地址

| 项目 | 地址 | 说明 |
|---|---|---|
| **OpenDots（开源复刻版，重点参考）** | **https://github.com/CopilotKit/OpenDots** | CopilotKit 在 OpenAI 发布 Dots 后两天内开源的同构实现。约 5.5k Stars，TypeScript。定位是"构建常驻 AI 同事工作区的开源模板" |
| CopilotKit（底层框架） | https://github.com/CopilotKit/CopilotKit | OpenDots 依赖的 Agent 前端框架（React SDK） |
| AG-UI（协议） | https://docs.ag-ui.com/introduction | Agent 与前端之间的流式事件协议 |
| OpenBot | https://github.com/CopilotKit/OpenBot | Agent 云电脑的容器主管 |
| OpenMuse / OpenTag | https://github.com/CopilotKit/OpenMuse · https://github.com/CopilotKit/OpenTag | 同系列开源项目 |

**OpenDots 技术栈**（与本文复刻方案高度相关）：

- 前端：React + TypeScript + Vite + CopilotKit React SDK + TanStack AI
- 协议：AG-UI（消息/工具调用/Agent 状态流式传输）
- 后端：CopilotKit Runtime + Node.js 24 + Docker（应用容器 + 独立浏览器容器）
- 存储：SQLite；集成：Composio（Gmail/Calendar 等）、Parallel（网页研究）、Channels SDK（Slack）、WebRTC（语音）
- UI 特性：Specialist Dots 导航、Spaces 文档工作区（网格/列表视图、斜杠命令编辑器）、Dot Computers 面板（浏览器/文件/终端/活动日志）、人机协同审批卡片（Approve & save / Decline）、通话界面（实时计时器、字幕、静音、悬浮窗）、System/Light/Dark 三主题

> **注意**：官方 Dots 是 OpenAI 的闭源产品，没有官方仓库；视频中的 "kiosa" 图谱工具和 "作战室" 仪表盘属于第三方设计概念/定制面板，并非 OpenDots 官方界面。本文复刻的是**视频中呈现的视觉与交互设计**，可参照 OpenDots 的工程结构落地。

---

## 2. 视频内容结构

整段录屏是一个"三幕式"结构（硬切转场，无过渡动画）：

| 时间 | 内容 | 主题色 |
|---|---|---|
| 00.00 – 18.5s | **kiosa 执行图谱**：节点图 + 底部 dock（代码/日志/成本），视图在 GRAPH→FLOW→HEAT 间切换，阶段字幕 03→06 递进，成本计数器 $0.156→$0.42 | 深绿/青绿 + 琥珀 + 珊瑚红 |
| 18.5 – 25.5s | **X 帖子详情页**：内嵌视频播放 "DOTS + GPT-6.1" 作战室仪表盘（Agent chips、房间场景、账户/复盘面板、余额计数器 $63→$2,810） | 深红 #a81024 + 青 + 金 |
| 25.5 – 26.4s | 手机控制中心下拉（录屏结束） | — |

手机状态栏显示 22:56，录制时间约 22:56（作战室内时钟显示 22:46 UTC）。

---

## 3. 设计语言总览

### 3.1 两套暗色主题

**主题 1 —— 图谱工具（kiosa）"深绿仪表盘"**：

- 背景：近黑带绿调 `#000000 → #0c1514 → #182220`（三层纵深）
- 面板/节点卡：`#182220`，1px 发丝边框 `#303a36`
- 主文本：`#ccd8ca`（灰绿）；高亮文本：`#dee3d4`
- 强调色：薄荷青 `#afe0d9`（激活态/视图标签）、深青 `#225d6e`（边/图表）
- 语义色：琥珀 `#c09d70`（等待/门禁）、珊瑚红 `#ab3d42` / `#e36e59`（阻塞/高负载/字符串）、鼠尾草绿 `#aec86a`（正常/代码键名）、靛蓝 `#4747b9`（节点头）

**主题 2 —— 作战室（DOTS + GPT-6.1）"深红驾驶舱"**：

- 背景：纯黑 `#000000` / `#080d13`
- 面板：`#16181d` / `#2c3339`，深红阴影 `#5e212a` / `#902939`
- 品牌红：`#a81024` / `#b4152b`（标题栏、面板描边、关注按钮）
- 强调色：金 `#bf9442`（AWAIT USER 按钮）、青 `#3593b4` / `#51b4a6`（网络图/图表）、亮文本 `#f2f0f1` / `#e1dee3`

### 3.2 字体系统

| 用途 | 字体 | 特征 |
|---|---|---|
| 所有数字、指标、代码、日志 | 等宽字体（JetBrains Mono / IBM Plex Mono / Berkeley Mono 风格） | `font-variant-numeric: tabular-nums`，数字等宽不跳动 |
| 微标签 | 等宽大写 | 8–10px，`letter-spacing: 0.08–0.12em`，如 `SOURCES KEPT`、`GRAPH COST` |
| 大计数器 | 等宽特粗 | 40–64px，如 `$0.29`、`$2,810.00` |
| 作战室标题 | 粗体压缩无衬线（如 D-DIN / Oswald 风格） | `DOTS + GPT-6.1`，白色，字重 800 |
| 阶段字幕 | 等宽大写 + 彩色竖条 | `05 ESCALATE` + 黄色副标题 |

### 3.3 布局密度与质感

- 信息密度极高：一屏内同时呈现 20+ 数据点，但通过分组、留白（面板内 8–12px padding）和视觉层级保持可读；
- 圆角小（2–6px），边框细（1px），大量使用**进度条 / 迷你柱状图 / sparkline** 作为"数据的微缩图形"；
- 几乎所有面板都有：大写微标签（左上）+ 状态标签（右上，如 `live`）+ 主数值区 + 底部脚注的四段式结构；
- 语义色全局一致：**绿=正常/低成本/进行中，红=门禁/阻塞/高负载，琥珀=等待/人工介入，青=流动/激活**。

---

## 4. 视图 A：Agent 执行图谱（graph 视图）拆解

![图谱全貌](images/ref_graph_full.jpg)

### 4.1 整体布局（1320×2868 录屏中的内容区 y≈480–2130）

```
┌──────────────────────────────────────────────────────────┐
│ ① 指标行  y 480–526（46px，单行等宽）                       │
│   GRAPH jev-cascade-02  NODES 20  EDGES 350  VIEW graph  │
│   DECISIONS 7 024  COST $0.295  T 20.65  FRAME 1239      │
│   右侧红色: kiosa · @thegreatest_sv                        │
├──────────────────────────────────────────────────────────┤
│ ①b 参数区  y 526–676（150px，4 列 × 3 行）                 │
│   每行 = 青绿填充条（内嵌标签，条长∝数值）+ 右对齐数值        │
├──────────────────────────────────────────────────────────┤
│ ①c 时间轴  y 680–706（贴在画布顶缘内侧，02…48 刻度）          │
├──────────────────────────────────────────────────────────┤
│ ② 画布区  y 706–1610（约 904px）                          │
│   节点卡片 + 贝塞尔曲线边（毛球）+ 顶/底两行泳道芯片          │
├──────────────────────────────────────────────────────────┤
│ ③ 阶段字幕条  y 1610–1745                                  │
│   ▌05 ESCALATE / human owns the post                     │
├──────────────────────────────────────────────────────────┤
│ ④ 底部 Dock  y 1745–2130（三栏）                          │
│   ┌───────────┬───────────────┬─────────────┐            │
│   │ questions │ RUN LOG       │ GRAPH COST  │            │
│   │ .py 代码   │ + NODE LOAD   │ 计数器面板   │            │
│   └───────────┴───────────────┴─────────────┘            │
└──────────────────────────────────────────────────────────┘
```

### 4.2 ① 顶部指标条

- 单行等宽字体指标流，格式为 `标签 值`：`NODES 20`、`EDGES 350`、`DECISIONS 7 024`（千分位空格分隔）、`COST $0.295`（绿色）、`T 20.65`（运行秒数）、`FRAME 1239`（帧号）；
- `VIEW graph` 中当前视图名用薄荷青高亮；`DECISIONS` 数值部分字符用红色（增量位）；
- 右上角应用标识 `kiosa · @thegreatest_sv` 用品牌红，等宽小字。
- **修正（v3，放大原帧 y498–580 逐字复核）**：`NODES` 的值是 **20**，不是 220 —— 20 个节点 / 350 条边。这一点直接解释了画布为何是"毛球"：**节点极少、边极多**（平均每对节点约 17 条边），而不是"节点多"。

**参数区（v3 重写）**：初版把这里描述成"一枚枚独立小芯片卡"。放大原帧后确认结构完全不同——

- 是**一整块暗面板**，内部 **4 列 × 3 行**，列间有 1px 竖分隔线；没有独立卡片边框、没有迷你进度条；
- 每行 = **一条青绿渐变填充条，标签文字写在条内**（`input` / `steps` / `weight` / `notes` / `fresh` / `open` / `strict` / `retries` / `window` / `seeded` / `budget` / `risk`），右侧跟一个右对齐的白色等宽数值；
- **填充条的长度与数值正相关** —— 它同时兼任"条形图"，这是初版误读为"标签 + 独立进度条"的根源；
- 部分行是裸文本标签（条长归零，如 `notes`）；`Missing` 一类异常项用琥珀底；
- 数值约每 0.4s 抖动一次，条长随之伸缩。

**时间轴**：`02 04 06 … 48` 刻度，位于参数区下方、**贴在画布顶缘内侧**（不是独立占位一行，也不是画布底部）。刻度线 `#42564f`，数字灰绿，当前时刻加重加亮。

### 4.3 ② 画布区：节点卡片设计

![节点图](images/crop_graph_canvas.jpg)

**单张节点卡**（以 `jev-1.13.0 · system_one` 为例）：

- 尺寸约 220×90px，圆角 6px，背景 `#182220`，1px 边框；
- **彩色头部条**（高 4–6px）：靛蓝 `#4747b9`（system 节点）/ 青 / 琥珀 / 红 / 绿，按节点类型着色；
- 标题行：等宽 11px，`jev-1.13.0 · system_one`，主色 `#dee3d4`；
- 数据行：`state_tokens ………… 0.164`，标签灰色、值右对齐，值旁可带 2px 迷你进度条；
- 卡片四边有**连接埠**（小圆点），边从埠口出入；
- 动效：新节点**弹入**（scale 0.9→1 + opacity，约 200ms，`cubic-bezier(0.34,1.56,0.64,1)`）；被选中的节点边框增亮 + 外发光；
- 视频中出现的节点族：`jev-1.13.0 · system_one`（system）、`Noul · safe_to_run`、`Score · urgency`、`write`、`review`、`gate ≥ 0.85`、`choices`、`result.scores`、`result.nouls`、`queue/…`、`human · publish` 等。

**边（edges）**：

- 三次贝塞尔曲线（水平方向出入，控制点对称），默认 1px、`rgba(200,220,210,0.25)` 灰绿；
- **流动光带**：激活边上有沿路径运动的光点/光段——琥珀色 `#c09d70`（choice 路径）、青色（flow 视图）、红粉色 `#e36e59`（escalate 路径）；
- 边的粗细随流量变化（bundle 效果），形成"粗股流"；
- 画布有极淡的网格底纹（`rgba(255,255,255,0.02~0.03)`）。

**底部泳道芯片**：画布下缘一排彩色胶囊（`completed_work` 青 / `sources` 青 / `gaps` 橄榄），每个胶囊上沿有彩色小圆点，标注该泳道的事件。

**泳道芯片修正（v2）**：泳道芯片实际有**两行**——画布顶部一行（`completed_work` / `sources` / `gaps`，位于分段控件下方）和画布底部一行（`result.scores` / `result.nouls` / `queue/…`，右侧另有灰色 `state bus · JSON` 胶囊）。初版指南只记录了下缘一行。

**画布密度修正（v3，本轮最大改进项）**：原版画布 = **20 个节点 / 350 条边**的「毛球」——节点聚在上半部成簇，细白灰长曲线密集交叉织成网，另有 3–5 股极粗的彩色流束（琥珀黄、青、粉、珊瑚红）以大弧度横扫到右下。复刻验证结论：**稀疏 DAG + 单线边是全场最大差距**，必须叠加四层处理（v2 的三层法 + v3 新增的密集线束），缺一层都"不像"：

1. **流束（bundle）**：同一对节点间画 4–12 条平行贝塞尔曲线（spread ±6px 递进），而非一条；
2. **幽灵边**：15–20 组长距离边，每组 3 条、spread 随机 ±110px、透明度仅 0.06，垫在底层制造雾感；
3. **密集线束（v3 新增，关键）**：**随机节点对**再画 **约 400 条**长距离贝塞尔（`spread` 随机 ±190px，`stroke rgba(216,234,226,.10–.34)`，线宽 0.8–1.3px）——注意是"随机配对"而不是"沿拓扑相邻"，原版的毛球感来自大量拓扑上无关的长边交叠；
4. **彩色粗束**：约 **12–14 组**随机长边，每组 3 条、线宽 3px、`opacity .72` + `drop-shadow(0 0 7px)` 辉光，配色只用 4 色（琥珀黄 `#c9d94a`、青 `#3fae9c`、粉 `#f07aa0`、珊瑚 `#e0705f`）。**彩色束不能多** —— v3 实测 30 组会让画面变成"彩色面条"，14 组才接近原版的"灰网 + 少数彩束"。

> 实测数据：`WIRE_N = 420` 条随机长边 + 14 组彩束 ≈ 视觉密度接近原版；这是纯 DOM/SVG 方案下可接受的量级（约 470 个 `<path>`，Chromium 下稳定 60fps）。

### 4.4 ② 视图切换：GRAPH / FLOW / HEAT

居中分段控件，激活项 = 薄荷青填充圆角底 + 深色文字；未激活 = 透明底灰字。切换时画布约 150ms 交叉淡变。

**位置修正（v2）**：分段控件位于**时间轴下方、画布顶部居中悬浮**（绝对定位 `top:14px; left:50%`），并非独立占位一行。复刻验证时鼠标悬停 GRAPH 标签的帧（t≈15.55s）正落在此位置。

- **GRAPH**（默认）：完整拓扑，灰绿边 + 琥珀流动；
- **FLOW**（t≈11.67s，字幕 `VIEW · FLOW / where decisions travel`）：边上的流动光带增强，突出"决策的流动"；
- **HEAT**（t≈13.61s，字幕 `VIEW · HEAT / load per node`）：节点卡整体按负载染色，卡中央显示超大百分比徽标（99%/3%/65%/42%…），红色=高负载、绿色=低负载，呈现热力图效果。

![FLOW 视图](images/ref_graph_flow.jpg)
![HEAT 视图](images/ref_graph_heat.jpg)

### 4.5 ③ 阶段字幕条

左侧 4px 彩色竖条 + 大字幕 + 黄色小字副标题，随执行阶段切换（交叉淡变 + 竖条颜色过渡）：

| 阶段 | 字幕 | 副标题 | 竖条色 |
|---|---|---|---|
| 03 | `CHOICE → WRITE` | `confidence 0.93` | 绿 |
| 04 | `GATE HOLDS` | `0.71 < 0.85 → review` | 红 |
| 05 | `ESCALATE` | `human owns the post` | 琥珀 |
| — | `VIEW · FLOW` | `where decisions travel` | 青 |
| — | `VIEW · HEAT` | `load per node` | 琥珀 |
| 06 | `CODE ACTS` | `10 000 decisions = $0.42` | 绿 |

### 4.6 ④ 底部 Dock 三栏

**栏 1：代码面板 `questions.py`**（右上角 `12 l/s` = 每秒 12 行的"打字"速度）

![代码面板](images/crop_graph_code.jpg)

- 等宽 10px Python 源码，语法高亮：键名青绿 `#7dd3fc` 系、字符串琥珀/珊瑚 `#e36e59`、数字与布尔值 sage 绿 `#aec86a`、注释灰 `#899186`；
- **逐字打字动画** + 块状光标闪烁（1s steps 无限次）；
- 行号槽、当前行高亮；
- 代码内容随阶段演进（criteria 字典 → `FLOOR = 0.85` → `def gate(answer)` 门禁函数 → `# three answers, one cheap de…` 注释行）。

**栏 2：RUN LOG + NODE LOAD**

- RUN LOG：日志流，每行 = 彩色状态点（绿 ✓ / 红 ✗ / 琥珀 hold）+ 等宽文本（`jev`、`ask`、`gate`、`queue`、`node.drag`、`graph.load`、`bus.attach`），右上角 `10 l/s`；新行从顶部滑入淡入，自动滚动；
- NODE LOAD（右上角 `live` + 脉冲小圆点）：横向条形图，行标签 + 红→橙渐变条 + 右对齐百分比（`jev-1.13.0 100%`、`review 99%`、`result.choices 96%`、`gate ≥ 0.85 95%`…）；下方还有一列迷你竖向柱状图（绿/琥珀/红混合）。

**栏 3：GRAPH COST 面板**

![成本面板](images/crop_graph_cost.jpg)

- 头部 `GRAPH COST` + 右上 `live`（琥珀，带脉冲圆点）；
- 指标行：`edges 350`、`bus pins 48`、`decisions 7 026`（红）、`input / M tokens $0.042`（绿）、`output free`、`median 256 ms`（青）、`held at gate 26`（红）；
- **核心大计数器**：上方微标签 `10 000 decisions`，下方 48px+ 特粗等宽绿色 `$0.29`，约 $0.01/秒 匀速递增（18s 内 $0.156→$0.42）；
- 两条对比条：`jev`（短绿条）vs `LLM`（长红条）——成本归因；
- 脚注：`LLM writes · Jev decides · code acts`。

### 4.7 一个真实的细节

t≈15.55s 时画面上**鼠标指针**悬停在 GRAPH 标签上——说明源录屏是桌面端（kiosk 展示屏）。复刻演示时可保留光标元素以增强真实感。

---

## 5. 视图 B：DOTS + GPT-6.1 作战室拆解

![作战室全貌](images/ref_warroom_full.jpg)

### 5.1 整体布局（v3 重写：这是被初版误读最严重的一节）

> **v3 关键更正**：初版把作战室描述成"导航条 + 两行 chips + 场景 + 面板"的自上而下堆叠。逐帧取色后确认：**整个作战室媒体区是一个红色描边容器** —— 原帧在 y407–2212 区间存在色值 `#b81027` 的矩形描边（左右竖条各 **16px**、底部横条 **18px**），顶部那条"导航条"其实就是容器的红色标题带（**62px**）。这个红框是整块 UI 最强的识别特征，初版完全遗漏。

容器内尺寸（总高 1805px，内容宽 1288px）：

```
   ┌═══ 红色描边 16px ═══════════════════════════════════════┐
   ║ ① 红色标题带  h=62px  #a81024→#c01330 渐变              ║
   ║    ▪▪▪ DOTS + GPT-6.1 │ CRAWLER / NIGHT 03 │          ║
   ║                        X - immortalhowwl │ [3-NIGHT RUN]║
   ├─────────────────────────────────────────────────────────┤
   ║ ② 8 张 Agent 卡（4×2 统一网格）  h=244px                 ║
   ║   SCOUT ANALYST SKEPTIC QUANT                           ║
   ║   RISK  JOURNAL CRAWLER APPROVAL                        ║
   ├─────────────────────────────────────────────────────────┤
   ║ ③ 主场景：驾驶舱房间 + 大图表面板   h=740px               ║
   ║   左下 THREE NIGHTS / STARTED WITH $63                   ║
   ║   右下 POST REPLAY / SITES · UPDATES · POSTS             ║
   ├─────────────────────────────────────────────────────────┤
   ║ ④ 面板行 1：ACCOUNT / THREE NIGHTS   h=222px             ║
   ║   左：星座网络 + 底部 mini 柱状图   右：旅程行 + 大计数器   ║
   ├─────────────────────────────────────────────────────────┤
   ║ ⑤ 面板行 2：POSITION REPLAY + EVIDENCE ROUTER  h≈500px    ║
   └═══ 红色底边 18px ═════════════════════════════════════════┘
   （X 帖子 chrome 叠压在容器下缘之上，正文延伸到底边之下）
```

### 5.2 ① 红色标题带（初版误称"导航条"）

- 高 **62px**，横向渐变 `#c01330 → #a81024`，左端带三个白色小方点组成的品牌 motif（3 列 × 2 行）；
- 左：`DOTS + GPT-6.1`（白、粗、压缩无衬线，字号约 26px）；
- 中左 `CRAWLER / NIGHT 03`（粉白小字，约 x=405）；
- 中右 `X - immortalhowwl`（粉白小字，约 x=790）；
- 右：`3-NIGHT RUN` 徽章（近黑胶囊 `#1c0509` + 白字 + `#d44055` 1px 描边）；
- **容器边框**：左右各 16px 实心 `#b81027`（实测 BGR `(39,16,184)` → RGB `(184,16,39)`），底部 18px 同色横条，形成"红框包住整块媒体"的观感；
- 右上角小字时钟 `22:46 UTC`（**已删除**：v2/v3 两轮在全部参考帧中均未找到该元素，初版系误读，不再实现）。

### 5.3 ② 8 张 Agent 卡（v3 重写：是**一套** 4×2 网格，不是两套不同卡片）

![Agent 卡网格](images/crop_war_chips.jpg)

**v3 更正**：初版把它拆成"第一行角色卡 + 第二行功能 chips"两套不同组件。放大原帧后确认**8 张卡结构完全一致**，只是一个 4 列 × 2 行的统一网格（`gap 8px`，区内边距 12/14px，整区高 244px ⇒ 单卡约 118px）。

八张卡的完整内容（`角色 / 编号 / 状态行 / 主数值 / 脚注`）：

| # | 角色 | 编号 | 状态行 | 主数值 | 脚注 |
|---|---|---|---|---|---|
| 1 | SCOUT（绿） | `DOT 01` | `SOURCES KEPT` | `3 / 76` | `SOURCE LINK ATTACHED` |
| 2 | ANALYST（金） | `DOT 02` | `EVIDENCE + CONTEXT` | `GPT-6.1` | `THESIS + OBJECTION` |
| 3 | SKEPTIC（粉） | `DOT 03` | `RECYCLED / REJECTED` | `CHECKING` | `CHECK ORIGINAL DATE` |
| 4 | QUANT（蓝） | `DOT 04` | `ENTRY RECALC` | `WAIT` | `FEES / EXIT / INVALIDATION` |
| 5 | RISK（青） | `DOT 05` | `SIZE LIMIT` | `REVIEW` | `STOP LIMITS + SIZE` |
| 6 | JOURNAL（橙） | `DOT 06` | `CLOSED P&L` | `TRACKING` | `AFTER ACTION LOG` |
| 7 | CRAWLER（青绿） | `INPUT` | `LAUNCHES SCANNED` | `64` | `SITES · UPDATES · POSTS` |
| 8 | APPROVAL（红） | `INPUT` | `ENTRY AUTHORITY` | `YOU` | `ENTRIES REQUIRE YOUR APPROVAL` |

**单卡结构（自上而下，5 行，`justify-content:space-between`）**：

1. 头部行：**彩色圆角方块图标**（22×22，圆角 7px，填充 = 角色色，带 `box-shadow:0 0 12px` 同色辉光，内含一个角色 emoji/图形）+ 角色名（15.5px 特粗、角色色）+ 右对齐 `DOT 0N`（9.5px 灰）；
2. 状态行（10px 灰、大写、`letter-spacing .06em`）；
3. 主数值（21px 特粗、角色色、右对齐、`tabular-nums`）；
4. 底部细条（3px，`#1c222a` 轨道 + 角色色填充，宽度 38%↔86% 往复动画 2.4s）；
5. 脚注（8.5px、`#565e66`、大写）。

> 注意数值行的字体明显大于状态行，且颜色与角色同色 —— 这是原版"一眼扫出谁在干什么"的关键，初版把主数值做得偏小是错的。

### 5.4 ④ 主场景：驾驶舱房间

![场景](images/crop_war_scene.jpg)

- **等距视角的暗色房间**（2.5D 场景，SVG/CSS 3D 或预渲染素材皆可）：深色地板 + 背景墙带红色辉光；
- 5–6 个 **Dot 吉祥物**（青蛙/团子形象，绿/橙/蓝/粉/黄）"坐"在工位前，带** idle 呼吸动画**（translateY ±2px，3s ease-in-out 无限交替）；
- 每张工位一台显示器，屏上跑**迷你条形图**（各角色各司其职）；
- 环境道具：盆栽（2 盆）、服务器机柜、卫星碟、火箭模型、DOTS 品牌墙（三个圆点）；
- 场景上部叠加大图表面板：`SELECTED TICKER / RESEARCH REPLAY`，右上 `GPT-6.1` 标签，绿/粉双色折线（价格走势），带辉光端点；下方一行小字 `SOURCES → OBJECTION → SIZE · YOUR APPROVAL`；
- 场景下部：左侧 `THREE NIGHTS / STARTED WITH $63` 标注（绿字 `THREE NIGHTS` + 灰字副行），右侧 `POST REPLAY` / `SITES · UPDATES · POSTS`；每个 Dot 的名字（`SCOUT`/`ANALYST`/…）以小标签贴在各自桌面上；
- **尺寸（v3 实测）**：场景区高 **740px**（初版假设 480px 偏小约 35%），是整个容器里占比最大的一块；整块占位 721–1470。

> **v3 补充**：角色（blob 生物）坐在**桌子与显示器之间**——纵向上是"显示器在后、角色居中、桌子在前"。复刻时若把角色画在桌子下方，会立刻"不像"。

### 5.5 ④ 面板行 1：账户与星座网络

![账户面板](images/crop_war_replay.jpg)

- 左 panel `ACCOUNT / THREE NIGHTS`（面板标题就是这两个词，v2 曾误写成 `THREE NIGHTS / STARTED WITH $63` —— 那是**场景内**的叠加标注，两者都在，别混）：
  - **星座式网络图**：15 个左右的小圆圈节点（青 `#5aa7e8` / 粉 `#f07aa0` 两色，半径 2.2–7px 递减），节点带同色柔光晕；节点间用细线（1px / `opacity .6`）随机连成网；中心节点最大最亮；
  - **循环重绘**：约 4.2s 一轮，节点按 `i × 0.07s` 依次弹出（`pop` 关键帧：`scale .6 → 1.08 → 1`），边比节点晚 0.5s 起；
  - **下方 mini 柱状图**：横贯面板整宽的细柱条（约 128 根，宽 4px 左右），主体青绿 `#2f7d6a`，每 7 根插一根粉色 `#f07aa0` 作为"事件"标记。
- 右 panel（余额区）：顶部旅程行 `$63 → $740 → $2,810 → $8,980`（灰白小字 + 箭头）；中央 **约 64px 特粗等宽绿色大计数器**；下方微标签 `USD BALANCE`（左）与 `NIGHT 03 / $63 START`（右）；数字变化时带轻微缩放脉冲；
- 计数器时序：$74.37（t=19.4）→ $279.01（20.4）→ $620.86（21.4）→ $1,896.84（22.4）→ $2,810.00（23.3）→ 保持——**先快后慢的 ease-out 加速叙事**。

**构成修正（v3）**：`THREE NIGHTS` 面板内**不是折线图，而是星座网络 + 底部柱条**（v2 复刻把它做成了双色折线，本轮已改正）。真正的**双色折线**只出现在**主场景中央那块大屏**上（`SELECTED TICKER / RESEARCH REPLAY`，绿涨粉跌）。三处网络/曲线各归其位，别搞混：

| 位置 | 图形 | 颜色 |
|---|---|---|
| 主场景中央大屏 | 双色折线（价格走势，带辉光端点） | 绿 `#3ddc84` / 粉 `#f07aa0` |
| ACCOUNT 面板左 | 星座网络 + 底部 mini 柱条 | 青 `#5aa7e8` / 粉 `#f07aa0` |
| EVIDENCE ROUTER | 辐射网络（椭圆轨道 + 中心矩形框） | 6 色（见 5.6） |

### 5.6 ⑤ 面板行 2：复盘与路由器

- **左 panel `NIGHT 03 / POSITION REPLAY`**（高约 500px）：
  - 头部行：标题 + 右上 `2 EXITS / 1 STOP`（红）；
  - 第二行：`AWAIT USER` 按钮（**金底 `#bf9442` + 深字**、圆角、带呼吸微光）+ `EXIT LIQUIDITY` / `deep · 2 412 fills` + 右侧 `PRICE PATH / SCHEMATIC`；
  - 主体：**K 线图占左侧约 4/5 宽度、高度撑满**（26 根，绿涨红跌 + 1px 影线 + 4 条水平网格）；
  - **右侧 1/5 是 4 条纵向排列的 mini sparkline**（约 112px 宽 × 46px 高）：`SIZE`（青）/ `EXPOSURE`（金）/ `HEDGE`（蓝）/ `DRAWDOWN`（粉），各带一条基线，逐条 `draw-line` 描边动画。**v3 新增项** —— 初版完全漏掉了这 4 条小图，是复盘面板最明显的缺口；
  - 底部：`MOMENTUM` 迷你走势（300×90，带 100/80 虚线刻度）+ 右侧 `NET P&L +$4,320`（绿）。
- **右 panel `EVIDENCE ROUTER / DOTS`**：
  - 头部行：标题 + 右上 `GPT-6.1` 标签；
  - 三枚状态胶囊：`76 LAUNCHES`（青底青字）/ `3 SOURCES`（绿底绿字）/ `2 REJECTED`（红底红字），各带 1px 同色描边；
  - **辐射网络（v3 更正为 6 个卫星节点）**：
    - **中心不是圆，是圆角矩形框**（140×58，`rx 12`，青描边 2.2px，内含两行：`GPT-6.1` / `CHECK + REASON`），外圈再套一层虚线圆角框做呼吸辉光；
    - 背后有 **2 圈椭圆轨道**（实线 180×86 + 虚线 250×120，`#2c4a52` / `#22404a`）——初版漏了轨道；
    - 6 个卫星按方位排布：`SOURCE`（正上，青）、`SCOUT`（右上，绿）、`YOU`（左，金）、`SKEPTIC`（右下，粉）、`RISK`（左下，橙）、`QUANT`（正下，蓝）；卫星是 26px 圆圈 + 33px 脉冲光晕，名字写在圈外上方、语义脚注写在圈外下方；
    - 中心→卫星用二次贝塞尔（控制点抬高 34px 形成弧线），`draw-line` 描边动画 + 沿线运动光点（`getPointAtLength()`，周期 2.2s，逐个延迟 0.18s 启动）；
  - 底部：`RECYCLED ANNOUNCEMENTS`（左灰）/ `AWAIT USER`（右金）+ `SOURCE → OBJECTION → SIZE` 流水行。
- 面板行上方右侧可见盈亏流水 `+$4,320`（绿）/ `-$285`（红）/ `+$2,135`（绿）与 `05:05 SECOND EXIT`。

### 5.7 外围：X 帖子 chrome（v3 更正为"叠压"关系）

![帖子](images/crop_post.jpg)

**v3 更正**：初版把帖子 UI 描述成"在作战室下方"。实际是 —— 作战室媒体区是**帖子的媒体预览**，X 的帖子 chrome **叠压在容器下缘之上**：

- **帖子头部行**（头像 + 昵称）**压在红色容器的下半部之上**，与 `POSITION REPLAY` / `EVIDENCE ROUTER` 面板重叠约 130px；实现上用 `margin-top: -126px; z-index: 6`（放在 `.wr-frame` 之后）；
- 头部本身带一层近黑半透明底 `rgba(10,10,12,.90)` + 圆角 18px，让底下的面板内容"隐约透出"；
- 头像约 **132px** 直径（圆形，暖橙渐变底 + 表情图形，4px 深色描边）；
- `Logics`（44px 特粗）+ 蓝色认证徽章（40px 圆形 `#1d9bf0`）+ 换行 `@immortalhowwl`（30px 灰）；
- 右侧 **`关注` 按钮：白底 `#e8e6ea` + 深字**（这是"未关注"态，**不是红色** —— 初版用了品牌红，错）、圆角 34px、字号 26px；
- 容器底边之下：`翻译自英语 显示原文`（灰 + 蓝色可点）→ 正文（27px 白，两行截断，中文）→ 互动栏（评论 51 / 转发 36 / 点赞 328 / 收藏 / 分享）→ 播放控制条（进度条、`-0:19`、`1x`、音量、全屏、画中画）。

---

## 6. 动效系统全解

### 6.1 图谱视图动效时间线（0–18.5s）

| 时间 | 事件 | 参数 |
|---|---|---|
| 0.0s | 阶段字幕 `03 CHOICE → WRITE`（绿） | 交叉淡变 150ms |
| 全程 | 成本计数器递增 | $0.156 → $0.42，约 $0.01/s，数字位跳动 |
| 全程 | 边流动光带 | 循环 2–4s，线性 |
| 全程 | 节点弹入 / 数值刷新 | 200ms，回弹缓动 |
| 全程 | 代码打字 `questions.py` | 12 行/s，光标闪烁 1s steps |
| 全程 | RUN LOG 流 | 新行 ~100ms 一条，滑入 + 淡入 |
| 1.94s | 字幕切 `04 GATE HOLDS`（红） | — |
| 5.83s | 字幕切 `05 ESCALATE`（琥珀） | — |
| 11.67s | 视图切 FLOW，字幕 `VIEW · FLOW`（青） | 150ms 淡变 |
| 13.61s | 视图切 HEAT，节点显示负载 % 徽标 | 颜色按负载插值 |
| 15.55s | 鼠标悬停 GRAPH 标签，切回 GRAPH | — |
| 17.50s | 字幕切 `06 CODE ACTS`（绿） | — |

### 6.2 作战室动效时间线（18.5–25.5s）

| 时间 | 事件 |
|---|---|
| 18.5s | 硬切入 X 帖子页（无过渡） |
| 19.4→23.3s | 余额计数器 $74 → $2,810（ease-out 递增，约每 0.5s 一个数量级），23.3s 后定格 |
| 20–23s | 星座网络图逐节点绘制 → 完整 → 循环重绘 |
| 全程 | 折线图沿时间绘制、K 线逐根reveal |
| 全程 | Dot 吉祥物 idle 上下浮动（±2px / 3s） |
| 全程 | `live` 圆点脉冲（opacity 1→0.4，1.6s） |
| 全程 | AWAIT USER 按钮呼吸微光 |
| 25.5s | 下拉控制中心，录屏结束 |

### 6.3 动效实现要点速查

| 动效 | 推荐实现 |
|---|---|
| 大数字计数器 | `requestAnimationFrame` + easeOutExpo；`font-variant-numeric: tabular-nums`；变化时 `scale(1.04)` 脉冲 120ms |
| 边流动光带 | SVG `stroke-dasharray` + 动画 `stroke-dashoffset`；或沿 path 运动的 `<circle>`（`getPointAtLength`） |
| 节点弹入 | Framer Motion `initial={{scale:0.9, opacity:0}}` + `transition: spring` |
| 代码打字 | 按 token 数组 setInterval 揭示（10–14 行/s），块光标 `::after` 闪烁 |
| 日志流 | 列表 push + `AnimatePresence` 滑入；`scrollToBottom` 平滑滚动 |
| 视图切换 | 双图层 opacity 交叉 150ms |
| 热力着色 | 负载 % → HSL 插值（0% 绿 → 50% 琥珀 → 100% 红） |
| 星座/辐射网络 | 力导向或预设坐标；边用 `pathLength` 动画绘制，节点延迟 80ms 依次弹出 |
| idle 浮动 | CSS keyframes `translateY(±2px)` alternate infinite 3s |
| live 脉冲 | keyframes opacity 1→0.4 alternate infinite 1.6s |

---

## 7. 前端实现指南（供 AI 编码）

### 7.1 技术选型

```
框架：      React 18 + TypeScript + Vite
样式：      Tailwind CSS（或 CSS Modules + CSS 变量）
动画：      Framer Motion（组件级）+ CSS keyframes（循环类）
图谱：      SVG（边）+ 绝对定位卡片（节点）；布局可用 d3-force 或预设坐标
图表：      自绘 SVG（折线/K线/柱状）或 Recharts（需深度定制样式时建议自绘）
字体：      @fontsource/jetbrains-mono（等宽）+ 系统压缩无衬线做标题
状态：      Zustand（图谱/计数器/日志的模拟数据流）
```

### 7.2 全局 CSS 变量（主题 Token）

```css
:root {
  /* 图谱主题 */
  --bg-0: #000000; --bg-1: #0c1514; --bg-2: #182220;
  --panel: #182220; --border: #303a36;
  --text: #ccd8ca; --text-hi: #dee3d4;
  --mint: #afe0d9; --teal: #225d6e;
  --amber: #c09d70; --coral: #e36e59; --red: #ab3d42;
  --sage: #aec86a; --indigo: #4747b9;

  /* 作战室主题 */
  --wr-bg: #080d13; --wr-panel: #16181d; --wr-panel-2: #2c3339;
  --wr-red: #a81024; --wr-red-hi: #b4152b;
  --wr-gold: #bf9442; --wr-teal: #3593b4; --wr-jade: #51b4a6;
  --wr-text: #f2f0f1;

  /* 通用 */
  --font-mono: "JetBrains Mono", ui-monospace, monospace;
  --radius: 4px;
  --hairline: 1px solid var(--border);
}
```

### 7.3 布局骨架（CSS Grid）

```css
.graph-app {
  display: grid;
  grid-template-rows: 220px 910px 135px 385px; /* 指标条/画布/字幕/dock */
  height: 100vh;
  background: var(--bg-1);
}
.dock {
  display: grid;
  grid-template-columns: 520px 490px 310px; /* 代码/日志/成本 */
  gap: 1px; /* 发丝分割线 */
}
.war-room {
  display: grid;
  grid-template-rows: 70px 180px 64px 920px 400px 330px;
  background: var(--wr-bg);
}
```

### 7.4 节点卡片组件

```tsx
function GraphNode({ title, rows, color }: NodeProps) {
  return (
    <motion.div
      initial={{ scale: 0.9, opacity: 0 }}
      animate={{ scale: 1, opacity: 1 }}
      transition={{ type: "spring", stiffness: 320, damping: 22 }}
      className="node"
      style={{ borderTopColor: color }}
    >
      <div className="node-head" style={{ background: color }} />
      <div className="node-title">{title}</div>
      {rows.map(([k, v, p]) => (
        <div className="node-row" key={k}>
          <span className="k">{k}</span>
          <span className="v">{v}</span>
          <i className="bar" style={{ width: `${p * 100}%` }} />
        </div>
      ))}
    </motion.div>
  );
}
/* CSS: .node { width:220px; background:var(--panel); border:1px solid var(--border);
   border-radius:6px; border-top:4px solid; padding:8px 10px; } */
```

### 7.5 流动边组件

```tsx
function FlowEdge({ d, active }: { d: string; active: boolean }) {
  return (
    <g>
      <path d={d} className="edge-base" />
      {active && (
        <path d={d} className="edge-flow"
          style={{ animationDuration: "2.4s" }} />
      )}
    </g>
  );
}
/* CSS */
.edge-base { fill: none; stroke: rgba(200,220,210,.25); stroke-width: 1; }
.edge-flow {
  fill: none; stroke: var(--amber); stroke-width: 2;
  stroke-dasharray: 6 220;
  animation: flow linear infinite;
  filter: drop-shadow(0 0 4px var(--amber));
}
@keyframes flow { to { stroke-dashoffset: -226; } }
```

> **v2 验证补充**：`stroke-dashoffset` 的位移量必须与 `stroke-dasharray` 总和一致（6+220=226 → `-226`），否则光带循环会跳帧。另外**不要**用固定大值 dasharray（如 `1200`）做「路径绘制」动画——短路径会在动画前 15% 就画完；必须动态取 `path.getTotalLength()` 后赋值（见 7.9.1）。

### 7.6 大数字计数器 Hook

```ts
function useCountUp(target: number, duration = 4000) {
  const [n, setN] = useState(0);
  useEffect(() => {
    const t0 = performance.now();
    const ease = (t: number) => 1 - Math.pow(1 - t, 4); // easeOutQuart
    const tick = (now: number) => {
      const p = Math.min((now - t0) / duration, 1);
      setN(target * ease(p));
      if (p < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }, [target, duration]);
  return n;
}
// 用法：<span className="big-num">${n.toLocaleString("en-US",{minimumFractionDigits:2})}</span>
// .big-num { font: 800 48px/1 var(--font-mono); color: #3ddc84;
//            font-variant-numeric: tabular-nums; }
```

> **v2 验证补充（踩坑）**：如果计数器是「关键帧序列」（如余额 $63→$74→$279→…），务必用**代数令牌（generation token）**中断整个动画链——只 `cancelAnimationFrame` 不够，`setTimeout` 排队的后续关键帧仍会启动新动画，导致外部 `setState` 无法定格数值（复刻验证中实测复现）：

```ts
let raf = 0, gen = 0;
function play() {
  const my = ++gen;                       // 每次播放拿新令牌
  KEYS.forEach((k, i) => setTimeout(() => {
    if (my !== gen) return;               // 已被 setState 中断
    animateTo(k.v, 1000);
  }, i * 1000));
}
function stop(v: number) { gen++; cancelAnimationFrame(raf); render(v); }
```

### 7.7 代码打字机组件

```tsx
const LINES = [
  [{ t: "key", v: "criteria" }, { t: "punct", v: "={" }],
  [{ t: "str", v: '"research"' }, { t: "punct", v: ": " },
   { t: "str", v: '"Missing evidence."' }, { t: "punct", v: "," }],
  // ...
];
// 每 1000/12 ms 揭示一个 token；token 类型 → class（key/str/num/punct/comment）
// 光标：.caret::after { content:"▍"; animation: blink 1s steps(2) infinite; }
```

### 7.8 日志流与 NODE LOAD

```tsx
// 日志：useInterval 100ms push { dot: 'ok'|'fail'|'hold', text }，最多保留 40 行
// NODE LOAD：<div class="load-row"><span>{name}</span>
//   <i style={{width: pct+'%', background: heatColor(pct)}}/><b>{pct}%</b></div>
// heatColor(p): p>=.9 ? '#e36e59' : p>=.6 ? '#f59e0b' : '#aec86a'
```

### 7.9 作战室场景要点

- 房间用 SVG 绘制（多边形地板 + 矩形工位 + 显示器），吉祥物用 emoji 或简笔 SVG，外层包 `animation: bob 3s ease-in-out infinite alternate`；
- 大图表面板：SVG polyline + `stroke-dasharray` 沿路径绘制动画，端点 `<circle>` 带 `filter: drop-shadow` 辉光；
- K 线：逐根 `scaleY` 0→1 reveal，涨绿跌红（遵循 A 股/中文习惯）；
- 星座网络：预设节点坐标数组，边后画（每条边延迟 80ms），完成后 600ms 整体淡出重绘；
- 辐射路由器：中心节点 + 4 个角点节点，节点外圈 `<circle>` 做 `r` 脉冲动画。

#### 7.9.1 路径绘制动画的正确做法（v2 验证补充）

```ts
function animateDraw(root: SVGElement) {
  root.querySelectorAll(".draw-line").forEach(p => {
    const L = (p as SVGPathElement).getTotalLength();   // 必须动态取长度
    if (!L || !isFinite(L)) return;
    const delay = parseFloat(p.style.animationDelay || "0") * 1000;
    p.style.strokeDasharray = String(L);
    p.style.strokeDashoffset = String(L);
    p.animate([{ strokeDashoffset: L }, { strokeDashoffset: 0 }],
      { duration: 1300, delay, easing: "ease-in-out", fill: "forwards" });
  });
}
```

沿线运动光点（辐射网络）：`path.getPointAtLength(k * L)` + `requestAnimationFrame`，比纯 CSS 更可控，且可在重绘前统一 `cancelAnimationFrame`。

#### 7.9.2 图谱「毛球」质感四层法（v3 更新）

复刻验证表明：仅按字面描述（单线边 + 少量节点）实现，与原版差距最大的就是画布密度。必须先建立正确认知 —— **原版是 20 节点 / 350 条边**，即"节点极少、边极多"，然后叠加四层（详见 4.3 修正）：

1. **幽灵边**：15–20 组 × 3 条，`spread` 随机 ±110px，`rgba(200,220,210,.06)`，垫底层；
2. **密集线束（v3 新增，决定成败）**：**随机**节点对 × 约 400 条长贝塞尔，`spread` 随机 ±190px，`rgba(216,234,226,.10–.34)`，线宽 0.8/1.3px。**必须随机配对**，不要只连拓扑相邻节点；
3. **主干 bundle**：同源同目标 4–12 条平行曲线（spread ±6px 递进）+ 流动光点；
4. **彩色粗束**：12–14 组 × 3 条，线宽 3px、`opacity .72`、`drop-shadow(0 0 7px)`，只用 4 色（`#c9d94a` 黄 / `#3fae9c` 青 / `#f07aa0` 粉 / `#e0705f` 珊瑚）。**宁少勿多**：超过 ~20 组会变成"彩色面条"。

```js
// 第 2 层：密集线束（最影响"像不像"的一层）
const ids = NODES.map(n => n.id);
const pick = () => NODES[(Math.random() * NODES.length) | 0];
for (let i = 0; i < 420; i++) {
  const a = pick(), b = pick();
  if (a === b) continue;
  el("path", { d: edgePath(a, b, (Math.random() - .5) * 190), fill: "none",
    stroke: `rgba(216,234,226,${(.10 + Math.random() * .24).toFixed(2)})`,
    "stroke-width": Math.random() < .18 ? 1.3 : .8 }, gEdges);
}
// 第 4 层：少量彩色粗束
const COLS = ["#c9d94a","#3fae9c","#f07aa0","#e0705f","#c9d94a","#3fae9c"];
for (let i = 0; i < 14; i++) {
  const a = pick(), b = pick(); if (a === b) continue;
  const col = COLS[i % COLS.length];
  for (let k = 0; k < 3; k++)
    el("path", { d: edgePath(a, b, (k - 1) * 11), fill: "none", stroke: col,
      "stroke-width": 3, "stroke-opacity": .72,
      style: `filter:drop-shadow(0 0 7px ${col})` }, gEdges);
}
```

> SVG 元素创建顺序决定层级：幽灵边 → 密集线束 → 主干 bundle → 彩色粗束 → 节点。全部约 470 个 `<path>`，Chromium 下 60fps 无压力。

### 7.10 性能与质感清单

- 所有循环动画只用 `transform` / `opacity`；
- 画布元素加 `will-change: transform`；
- 等宽数字全局 `font-variant-numeric: tabular-nums`；
- 发丝分割线用 `gap: 1px + 背景色` 实现，比 border 更锐利；
- 面板统一四段式：微标签（左上 8px 大写）/ 状态（右上，如 `live`）/ 主体 / 脚注；
- 深色下文字最小 10px，对比度 ≥ 4.5:1。

---

## 8. 设计 Token 速查表

| Token | 值 | 用途 |
|---|---|---|
| 图谱背景 | `#000` / `#0c1514` / `#182220` | 三层纵深 |
| 图谱面板边框 | `#303a36` | 1px 发丝边 |
| 图谱主文字 | `#ccd8ca` / 高亮 `#dee3d4` | 标签 / 数值 |
| 薄荷青（激活） | `#afe0d9` | 视图标签、流带 |
| 深青 | `#225d6e` | 边、图表线 |
| 琥珀（等待） | `#c09d70` | 门禁、字符串 |
| 珊瑚红（阻塞） | `#e36e59` / `#ab3d42` | 高负载、错误 |
| 鼠尾草绿（正常） | `#aec86a` | 代码键名、低成本 |
| 靛蓝（节点头） | `#4747b9` | system 节点 |
| 作战室品牌红 | `#a81024` / `#b4152b` | 标题栏、面板描边 |
| 作战室金 | `#bf9442` | AWAIT USER 按钮 |
| 作战室青 | `#3593b4` / `#51b4a6` | 网络图、图表 |
| 涨/跌 | 绿 `#3ddc84` 系 / 红 `#e36e59` 系 | K线、盈亏、计数器 |
| 大计数器字号 | 48–64px 等宽 800 | `$0.29` / `$2,810.00` |
| 微标签 | 8–10px 大写 + 0.1em 字距 | 全局面板 |
| 数值 | 10–13px 等宽 tabular-nums | 全局 |

---

## 9. 复刻验收清单

- [ ] 图谱视图：顶部指标条（含千分位、COST 绿色、VIEW 高亮）+ 参数芯片行
- [ ] 节点卡：彩头 + kv 行 + 迷你条 + 弹入动画
- [ ] 边：贝塞尔曲线 + 流动光带（琥珀/青/红三色语义）
- [ ] GRAPH / FLOW / HEAT 三视图切换（150ms 淡变；HEAT 显示 % 徽标）
- [ ] 阶段字幕条（彩竖条 + 六阶段递进）
- [ ] 代码面板打字机（12 行/s）+ 语法高亮 + 闪烁光标
- [ ] RUN LOG 流式日志（100ms/条，彩色状态点）
- [ ] NODE LOAD 横向条形图 + 迷你柱状图 + `live` 脉冲
- [ ] GRAPH COST 面板：指标行 + 48px 绿色大计数器（$0.01/s 递增）+ jev/LLM 对比条
- [ ] 作战室：红色导航条 + DOTS + GPT-6.1 标题 + 3-NIGHT RUN 徽章
- [ ] Agent chips（头像/角色名/状态标签/大数值/DOT 编号/进度条/激活描边）
- [ ] 驾驶舱房间（工位/显示器图表/吉祥物 idle 浮动/盆栽/服务器/火箭）
- [ ] ACCOUNT 面板：星座网络绘制动画 + 余额大计数器（ease-out 递增至定格）
- [ ] POSITION REPLAY：AWAIT USER 按钮 + K 线（涨绿跌红）+ MOMENTUM 图
- [ ] EVIDENCE ROUTER：中心辐射网络 + 节点脉冲
- [ ] 全局：等宽 tabular-nums、发丝边框、四段式面板、语义色一致

**v3 追加验收项**（本轮精细对比后新增）：

- [ ] 图谱：`NODES` 显示 **20**；参数区为 **4 列 × 3 行的填充条内嵌标签**（非独立芯片卡）；时间轴贴在画布顶缘内侧
- [ ] 图谱画布：**≥ 400 条随机长贝塞尔** + 12–14 组彩色粗束（灰网为主、彩束为点缀）
- [ ] 作战室：整块媒体区有 **16px 红色描边容器**（左右竖条 + 底部 18px 横条），顶部为 **62px 红色标题带**
- [ ] 作战室：Agent 卡为 **8 张同构卡片的 4×2 网格**（不是两行两类卡）
- [ ] 场景区高度 **740px**；角色位于"显示器之下的桌子之前"
- [ ] ACCOUNT 面板左：**星座网络**（约 15 节点 + 青粉双色 + 底部 128 根 mini 柱条）
- [ ] POSITION REPLAY 右侧：**4 条 mini sparkline**（SIZE/EXPOSURE/HEDGE/DRAWDOWN）
- [ ] EVIDENCE ROUTER：**6 个卫星节点** + 2 圈椭圆轨道 + 中心**圆角矩形框**（非圆形）
- [ ] X 帖子头部：`margin-top:-126px` **叠压**在容器之上；`关注` 按钮为**白底深字**

---

## 10. 复刻验证记录（v2）

> 本章由「按本文档实际施工 → 截图 → 与原视频逐区域对比 → 修订文档」的闭环产生。
> 复刻工程与对比材料见文末「附录：参考素材」。

### 10.1 验证方法

- **复刻实现**：严格按本文档 v1 规格（第 3–8 章）编写单文件前端实现（`replica/index.html`，原生 JS + SVG，无框架依赖，手机竖屏 1320×2868 等比还原），覆盖第 9 章验收清单全部 16 项；
- **截图对比**：用 Playwright 对复刻界面按 8 个状态截图（图谱：03/05/FLOW/HEAT/06；作战室：起跑/中途/定格），与原视频抽帧（`images/ref_*.jpg`）并排对比（`compare/cmp_*.png`）；
- **修订闭环**：把对比中发现的偏差回写到本文档对应章节（修正处以 **（v2）** 标注）。

**图谱视图对比**（左：原视频 t=9.72s；右：复刻 stage05）：

![图谱对比](compare/cmp_graph.png)

**HEAT 视图对比**（左：原视频 t=13.61s；右：复刻 heat）：

![HEAT 对比](compare/cmp_heat.png)

**作战室对比**（左：原视频 t=22.36s；右：复刻 war-final）：

![作战室对比](compare/cmp_war.png)

### 10.2 验收清单逐项结果

| # | 验收项 | 结果 | 说明 |
|---|---|---|---|
| 1 | 顶部指标条（千分位/COST 绿/VIEW 高亮） | ✅ 通过 | v2 修正：DECISIONS 仅尾段红色 |
| 2 | 参数芯片行 | ✅ 通过 | 数值 0.4s 抖动；芯片需 flex 自适应宽度 |
| 3 | 节点卡（彩头/kv 行/迷你条/弹入） | ✅ 通过 | 弹入用 WAAPI `cubic-bezier(.34,1.56,.64,1)` |
| 4 | 边：贝塞尔 + 流动光带三色语义 | ✅ 通过 | 需 bundle 多层 + 幽灵边才够密度（见 4.3/7.9.2） |
| 5 | GRAPH/FLOW/HEAT 三视图切换 | ✅ 通过 | 分段控件悬浮于画布顶部居中（见 4.4 修正） |
| 6 | 阶段字幕条（彩竖条 + 六阶段） | ✅ 通过 | 4.2s 自动循环 + 点击定格 |
| 7 | 代码打字机 + 语法高亮 + 光标 | ✅ 通过 | 12 行/s，token 级揭示，当前行高亮 |
| 8 | RUN LOG 流（彩色状态点） | ✅ 通过 | 380ms/条滑入，保留 7 行 |
| 9 | NODE LOAD 条形图 + 迷你柱 + live 脉冲 | ✅ 通过 | heatColor 三段插值正确 |
| 10 | GRAPH COST 大计数器 $0.01/s + 对比条 | ✅ 通过 | 计数器与指标行联动 |
| 11 | 红色导航条 + 标题 + 徽章 | ✅ 通过 | 压缩无衬线标题需 `transform:scaleY(1.12)` 逼近 D-DIN |
| 12 | Agent chips（七要素） | ✅ 通过 | 激活卡琥珀描边；底部进度条循环伸缩 |
| 13 | 驾驶舱房间（工位/图表/浮动/道具） | ✅ 通过 | 扁平 2D SVG；道具六类齐全 |
| 14 | 星座网络绘制 + 余额计数器 | ✅ 通过 | **修复**：需代数令牌中断动画链（见 7.6 补充） |
| 15 | POSITION REPLAY（AWAIT USER + K 线 + MOMENTUM） | ✅ 通过 | K 线逐根 scaleY reveal，涨绿跌红 |
| 16 | EVIDENCE ROUTER 辐射网络 | ✅ 通过 | 边绘制 + 沿线光点 + 节点 halo 脉冲，6.5s 循环 |
| — | 全局：tabular-nums/发丝边/四段面板/语义色 | ✅ 通过 | — |

**结论：16/16 项均可按本文档实现，指南整体可复刻。**

### 10.3 对比发现的差距与文档修订

| 差距 | 原版表现 | 复刻初版差距原因 | 文档修订 |
|---|---|---|---|
| 画布密度 | ~50 节点、数百条边的毛球，右侧 2–3 股粗流束 | v1 只描述单线边，实现出来是稀疏 DAG | 4.3 新增「毛球质感三层法」；7.9.2 补充代码级做法 |
| 泳道芯片 | 顶部、底部各一行 | v1 只记了底部一行 | 4.3 修正：补顶部一行 |
| 分段控件位置 | 时间轴下方、画布内居中悬浮 | v1 描述为独立控件行 | 4.4 修正：明确绝对定位悬浮 |
| 参数芯片密度 | 单行紧凑卡（icon+label+value 同行） | v1 描述易被实现为两行卡 | 4.2 修正：明确单行更贴近 |
| DECISIONS 红色 | 仅尾段（增量位）红色 | 易整段染红 | 4.2 已述，v2 在 10.2 再次强调 |
| 余额计数器定格 | t=23.3 后保持 $2,810.00 | `setState` 无法中断 setTimeout 动画链 | 7.6 补充代数令牌方案 |
| 路径绘制动画 | 折线/网络边沿路径生长 | 固定 dasharray 使短路径提前画完 | 7.5、7.9.1 补充 `getTotalLength()` 方案 |
| 「星座网络」构成 | 双色旅程折线 + 辐射路由器两部分 | v1 描述偏模糊 | 5.5 修正构成 |
| 导航条时钟 | 参考帧中未出现 | v1 记录 `22:46 UTC` | 5.2 标注「存疑」 |

### 10.4 已知未复刻项（不影响验收）

- **鼠标指针**（t≈15.55s 悬停 GRAPH 标签）：源录屏为桌面端展示屏；复刻版为手机竖屏构图，指针元素省略（指南 4.7 已说明为可选）；
- **场景纵深**：原版房间有等距透视、地面倒影、沙发；复刻版为扁平 2D SVG，元素（工位/显示器/吉祥物/盆栽/服务器/卫星碟/火箭/DOTS 品牌墙）齐全，纵深以渐变与辉光暗示；
- **显示器图表多样性**：原版各角色屏幕图表不同（柱状/折线/仪表）；复刻版统一为迷你柱状图。

### 10.5 复刻工程使用说明

- 入口：`replica/index.html`（双击即可打开，无需构建）；
- 演示控制条：底部可切换视图与状态（`?chrome=0` 隐藏控制条出干净截图）；
- 状态直达：`index.html?view=graph&state=stage05|flow|heat|stage06|stage03`，`?view=war&state=war-start|war-mid|war-final`；
- 截图脚本：`replica/shot.js`（需 Playwright + Chromium），输出至 `compare/`。

---

## 附录：参考素材

本目录下的图片素材（`images/`）与关键帧（`dots-frames/`）均来自原视频，可直接作为复刻的视觉对照：

| 文件 | 内容 |
|---|---|
| `images/ref_graph_full.jpg` | 图谱视图全貌（t=9.72s） |
| `images/ref_graph_flow.jpg` | FLOW 视图（t=11.67s） |
| `images/ref_graph_heat.jpg` | HEAT 视图（t=13.61s） |
| `images/ref_warroom_full.jpg` | 作战室全貌（t=22.36s） |
| `images/ref_post_full.jpg` | X 帖子 + 作战室（t=19.44s） |
| `images/crop_graph_canvas.jpg` | 图谱画布（y470–1750） |
| `images/crop_graph_code.jpg` / `crop_graph_cost.jpg` | 代码面板 / 成本面板 |
| `images/crop_war_chips.jpg` | 作战室 8 卡网格（y469–725） |
| `images/crop_war_scene.jpg` | 驾驶舱场景（y721–1470） |
| `images/crop_war_replay.jpg` | 面板行（y1490–2212） |
| `images/crop_post.jpg` | X 帖子区（y2230–2620） |
| `dots-frames/` | 关键帧原图（t = 9.72 / 11.67 / 13.61 / 19.44 / 20.41 / 22.36 / 24.30 / 26.25，1320×2868 PNG） |

### v2 复刻验证产物

| 文件 | 内容 |
|---|---|
| `replica/index.html` | 复刻实现（单文件，原生 JS + SVG，无构建依赖），覆盖验收清单 16/16 |
| `replica/shot.js` | Playwright 截图脚本（8 个状态） |
| `compare/rep_*.png` | 复刻界面各状态截图（1320×2868） |
| `compare/cmp_graph.png` | 图谱视图：原版 vs 复刻并排对比 |
| `compare/cmp_heat.png` | HEAT 视图：原版 vs 复刻并排对比 |
| `compare/cmp_war.png` | 作战室：原版 vs 复刻并排对比 |
| `compare/rep_topbar/rep_dock/rep_canvas/rep_scene.png` | 复刻界面分区细节图 |
| `dots-compare/` | v3 复刻截图（`rep_*.png`）与全幅对比（`cmp_graph/cmp_heat/cmp_war/cmp_canvas.png`） |
| `dots-frames/` | v3 重新抽取的关键帧（t = 9.72 / 11.67 / 13.61 / 19.44 / 20.41 / 22.36 / 24.30 / 26.25） |

---

## 11. 细节对比修订记录（v3）

> 本章由「把复刻结果与原始视频帧按**同一坐标切片、叠加对比**」产生。
> 方法：对每个区域取同 `y` 区间（如顶栏 y470–720、卡片区 y469–725、面板区 y1450–2300）分别裁出原帧与复刻，上下堆叠成一图放大目视 → 逐条记录差异 → 改实现 → 重新截图复验。
> 相比 v2 的"整幅并排"，同坐标切片能暴露**结构级**误读（v2 只看整幅时这些都"看起来差不多"）。

### 11.1 本轮修正清单

| # | 位置 | 初版（v1/v2）写法 | 实测真相 | 修正章节 |
|---|---|---|---|---|
| 1 | 作战室整体 | 自上而下堆叠：导航条 → 两行 chips → 场景 → 面板 | **整块媒体区是红色描边容器**：左右各 16px、底部 18px 实心 `#b81027`（实测 RGB 184,16,39），顶部红色标题带 62px | 5.1 / 5.2 |
| 2 | 图谱指标 | `NODES 220` | **`NODES 20`**（20 节点 / 350 边 —— 这才是"毛球"的成因） | 4.2 / 4.3 |
| 3 | 图谱参数区 | 一枚枚"参数芯片卡"（icon + 标签 + 数值 + 迷你进度条） | **一整块暗面板**，4 列 × 3 行；每行是**青绿填充条内嵌标签**（条长∝数值，兼任条形图）+ 右对齐数值 | 4.2 |
| 4 | 图谱时间轴 | 位于"最下方"（易实现为画布底部） | 贴在**画布顶缘内侧**（参数区之下、分段控件之上） | 4.2 |
| 5 | 图谱画布密度 | 单线边 + 少量节点 | 原版 20 节点 / **350 条边**；需四层：幽灵边 + **约 400 条随机长贝塞尔** + 主干 bundle + 12–14 组彩色粗束 | 4.3 / 7.9.2 |
| 6 | Agent 卡 | "第一行角色卡 + 第二行功能 chips"两套组件 | **8 张同构卡片的 4×2 统一网格**（含 `DOT 05/06` 与 `INPUT` 共 8 个角色） | 5.3 |
| 7 | 场景区高度 | 约 480px（占容器 27%） | **740px**（占容器 41%），是容器内最大一块 | 5.4 |
| 8 | ACCOUNT 面板 | 双色旅程折线 | **星座网络**（约 15 个青/粉节点 + 细连线，4.2s 循环重绘）+ 底部 **128 根 mini 柱条** | 5.5 |
| 9 | POSITION REPLAY | K 线 + MOMENTUM | 另有 **4 条纵向 mini sparkline**（`SIZE`/`EXPOSURE`/`HEDGE`/`DRAWDOWN`），K 线占左侧约 4/5 宽 | 5.6 |
| 10 | EVIDENCE ROUTER | 中心**圆形**节点 + 4 个卫星 | 中心是**圆角矩形框**（140×58，两行文字），**6 个**卫星节点，背后 **2 圈椭圆轨道** | 5.6 |
| 11 | X 帖子 | 位于作战室**下方**；`关注` 按钮品牌红 | 帖子头部 **叠压**在容器下缘之上（`margin-top:-126px`）；`关注` 为**白底深字**（未关注态） | 5.7 |

### 11.2 复核用图

图谱视图（左原版 t=9.72s / 右复刻）——注意红色 `NODES 20`、参数填充条、时间轴位置、画布密度：

![图谱对比 v3](compare/cmp_graph.png)

图谱画布局部（同 y 区间切片）——密度层次是最直观的对比：

![画布密度对比](dots-compare/cmp_canvas.png)

作战室（左原版 t=22.36s / 右复刻）——红框、8 卡网格、场景高度、面板结构：

![作战室对比 v3](compare/cmp_war.png)

### 11.3 v3 复刻实现改动的量化结果

| 指标 | v2 | v3 | 说明 |
|---|---|---|---|
| 图谱边数量（DOM `<path>`） | ~110 | **~470** | 420 条随机长边 + 14 组 × 3 彩色粗束 + 主干 bundle + 幽灵边 |
| 图谱 `NODES` 显示 | 220 | **20** | 与原版一致 |
| 作战室容器 | 无红框 | **红框 16/18px + 62px 标题带** | 最强识别特征 |
| 作战室卡片数 | 4 + 4（两套样式） | **8（同构网格）** | 单卡 ~118px 高 |
| 场景高度 | 480px | **740px** | 占容器 41% |
| ACCOUNT 左图 | 折线 | **星座网络 + 128 根柱条** | — |
| REPLAY 迷你图 | 0 | **4 条 sparkline** | — |
| ROUTER 卫星节点 | 5（含中心圆形） | **6 + 2 圈轨道 + 矩形中心** | — |
| 单文件体积 | 62 KB | **80 KB** | 仍无任何构建依赖 |

### 11.4 仍未复刻 / 有意简化（v3 更新）

- **场景等距透视**：原版是真 2.5D（地面倒影、沙发、地毯、纵深透视）；复刻用扁平 2D SVG + 径向渐变 + 辉光暗示纵深。元素齐全，但"空间感"仍是最大的一条观感差距；
- **显示器图表多样性**：原版各工位屏幕图表不同（柱状/折线/仪表盘）；复刻统一为迷你柱状图；
- **鼠标指针**（t≈15.55s）：桌面端录屏特有，手机竖屏构图下省略；
- **X 帖子头部与面板的重叠区**：原版重叠处底层面板内容仍部分可辨（半透明），复刻用了 `rgba(10,10,12,.90)`，比原版略"实"一些；
- **红框内左上/右上的小屏与监控窗**：原版场景墙面上还有 2 块小屏跑 sparkline，复刻已实现但在当前构图中不够显眼。
