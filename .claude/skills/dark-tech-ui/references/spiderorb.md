# 风格 D：SPIDERORB —— 赛博朋克终端仪表盘（FUI / HUD / 终端美学）

> 适用场景：舆情/信息流监控、研究型 Bot 驾驶舱、概念演示大屏、"机器在替你工作"的高压实时掌控感界面。
> 核心气质：Cyberpunk FUI——近黑海军蓝底、零圆角方格面板、全大写等宽像素字、◆■ 符号系统、密度本身即风格（单屏 10+ 面板 / 50+ 数据点）。

## 1. 设计思路

1. **密度本身即风格**：极高信息密度（50+ 数据点/屏）+ 严格模块化栅格（面板零圆角、1px 细边、无投影）。
2. **色彩配比 85:15**：近黑底占 ~85% 面积，霓虹强调色只占 ~15%——"颜色即语义"。
3. **颜色直接编码动作类型**：绿=SAVE/通过、红=BLOCK/拦截、金=SKIP/量化、蓝紫=链接/MAP、品红=热点/SPDR 分——全界面零例外。
4. **"文档即装饰"**：日志流、终端命令行（`spdr@Lab:~$`）、ARXIV 编号等文本内容本身承担装饰职责。
5. **一个绝对主角**：主视觉 THE ORB（放射爆发网络）占屏 38%、位于纵向 47% 处，上密下疏，其余全部让位。
6. **符号系统造终端感**：`◆`（面板前缀）、`■`（次级）、`·`（分隔）、`//`（品牌分隔）、`▲`（上升）、`✓`（成功行）是全局视觉签名。

## 2. 配色（全部实测值）

```css
/* 中性色 */
--bg-page:   #0D111A;              /* 近黑海军蓝，全场底色 */
--bg-panel:  #12151E;              /* 面板（比底色亮约 5%） */
--bg-panel-2:#161922;              /* 二级面板 */
--border:    #24222B;              /* 1px 细线 */
--border-2:  #2A2E3A;
--text-1:    #E5E3E7;              /* 主文字：冷灰白（实测主色，不是近白青！） */
--text-2:    #8B868D;
--text-3:    #555966;              /* 弱文字/微标签 */

/* 语义强调色（主色 / 深色变体） */
--green:   #53CD90;  --green-dim: #33AE6A;   /* SAVE/计数器/SIGNAL/JDG/正向 */
--red:     #AC364E;  --red-dim:   #8D3E44;   /* BLOCK/STALE/GRD/LIVE FEED 徽章 */
--magenta: #D8425D;  --pink:      #B85888;   /* 网络 hub/REACH/SPDR/危险 */
--gold:    #FFDD83;  --gold-dim:  #DEA248;   /* 星尘/QNT/REPO/HYPE/行底进度条 */
--blue:    #A2B3DF;  --blue-dim:  #8C90C0;   /* LOGO/链接/MAP/徽章描边 */
--cyan:    #D5FFFB;                            /* 仅图表高亮光点，不作正文色 */
```

- **全局质感层四件套（"视频感"最后一公里，全部 ≤5% 透明度、若有若无）**：
  ```css
  /* 噪点：feTurbulence data-URI 平铺，opacity .018 */
  /* 扫描线 */ background: repeating-linear-gradient(0deg, transparent 0 3px, rgba(0,0,0,.12) 3px 4px); opacity: .25;
  /* 暗角   */ background: radial-gradient(ellipse at center, transparent 55%, rgba(0,0,0,.38) 100%);
  /* 文字辉光：标题级亮字 */ text-shadow: 0 0 6px currentColor;
  /* 三层都是 position:fixed; inset:0; pointer-events:none; z-index:99 */
  ```

## 3. 字体排版

```css
/* 单一等宽家族走全场，像素/位图风 */
标题(像素体):   "Silkscreen","Pixelify Sans",monospace        /* ~30px, ls .02em */
正文/数据:      "Share Tech Mono","JetBrains Mono","IBM Plex Mono",ui-monospace,monospace
大计数器(LED):  "DotGothic16","Silkscreen",monospace
/* 或用 Silkscreen + CSS 点阵纹理模拟 LED：
   background: radial-gradient(circle, currentColor 38%, transparent 40%) 0 0/3px 3px;
   -webkit-background-clip: text; color: transparent; */
```

- 面板标题 ≈11px/600/ls .08em；面板元信息 9–10px `--text-2`；正文 10–11px；标签 9px；统计大数字 20–24px；主计数器 64–72px。
- **微文本行"散焦"技巧**：9px `--text-3` + `letter-spacing: .2em` 以上。
- 全部标签/标题/按钮**一律大写**；数字 `tabular-nums`。
- **⚠ 字体子集陷阱**：Google Fonts latin 子集不含 `◆ ■ ▲ ★ ✓`——这些符号一律用 CSS 图形绘制（菱形=旋转方块、箭头=border 三角、勾=伪元素），否则渲染成豆腐块。

## 4. 布局

- 设计基准 **1320×2868 竖屏**（移动竖屏优先），三栏 ≈440px/栏；`max-width:1320px` 居中。
- **自上而下 11 区块**（按内容高度百分比定节奏）：状态栏 → 微文本行(◆ SIGNALS…) → 主标题行(LOGO+像素题+三组统计+UTC 钟+SPIDER-ON 红徽) → LIVE FEED 静态标签行 → 三栏面板 → 计数器+日志流 → 统计列+曲线图 → ORB 标题行 → **ORB 主视觉行（38% 高度）** → 底部行(SIX AGENTS + STATE BUILDER) → 页脚。
- 参数：面板间距 10–12px、内边距 10–14px、标题行高 ~30px、流式行高 ~110px/行、智能体图标盒 60×60px。
- **面板通用规范**：背景 `#12151E` + `1px solid #24222B` + **圆角 0（至多 2px）** + 标题行"◆ + 大写标题 + 右侧元信息" + 标题下 1px 分隔线。
- 响应式：<1024px 三栏转两栏、<640px 单栏（顺序：标题→计数器→ORB→其余）；细丝/星尘数量按屏宽缩放。

## 5. 核心组件配方

- **◆ 面板**：见上；标题前缀 ◆，次级 ■。
- **boxed chip 代号系统**：`[JDG]`、`[QNT]`、`[GRD]`、`[WRT]`、`[MAP]`、`[SCT]`——3 字母代号 + `border:1px solid 代号色` + 同色文字，8px 字号 padding 2px 4px。
- **徽章三态**：实心红底黑字（LIVE FEED / SPIDER-ON，`background:var(--red); color:#0D111A`）/ 描边（金/品红）/ 实心绿（GUARD PASS）。
- **数码管计数器（Odometer）**：逐位翻滚——每位一个 `overflow:hidden` 窗口、内部 0–9 竖排 translateY 滚动；**千分位逗号是独立窄格**（建格时就处理，否则 999→1,000 布局跳动）；绿 `#53CD90` + `text-shadow:0 0 12px rgba(83,205,144,.8)` + 点阵扫描纹理；更新节奏每秒 +0~1 用泊松随机比线性更真实。
- **日志流**：容器定高 `overflow:hidden`，每 600–1400ms 随机顶部推入新行、整体 `translateY(-lineHeight)`；行首代号着色 + 穿插无代号绿色 `✓` 行；底部常驻终端行 `spdr@Lab:~$ orb trace …`；数据池预置 15–20 条模板循环。
- **流式行 StreamRow ×5**（左列灵魂组件）：左 3px 彩竖条 + 白粗体主标签 + 暗灰副标签 + 右对齐彩色数值 + boxed 代号 chip + 横贯整行波浪 sparkline（同色 stroke + 淡填充）+ 末端白色光点沿线流动；**五条流线右端统一上扬、汇入中央 ORB 网络**。
- **THE ORB 放射爆发网络**（主视觉，Canvas）：爆发核（中心偏上、粉白 radial-gradient 强光晕）+ 200–400 条白色细丝极坐标辐射、丝上按弧长布 2–4 微粒 + 6–8 个品红发光卫星 hub（各带 2–3 圈白色虚线椭圆轨道 `setLineDash([2,4])` + 绕行微粒）+ 1–2 个金色大虚线环 + 左侧 80–150 个金色星尘（**不连线**）+ 浮动金色斜角贴纸标签。
- **决策栈盒子行**：完整边框盒（非扁平行）——彩色行动词+代号 (`SAVE·JDG` 绿 / `SKIP·QNT` 金 / `BLOCK·GRD` 红) + 粗体白标题 + 右侧品红 `SPDR 0.83` + 暗灰副标题 + **行底金色细进度条**。
- **SIGNALS→OUTCOMES 发散箭头面板**：`▶▶▶ +1.84` 绿右箭头 ×N vs `◀◀ -0.31` 红左箭头，正反对冲可视化。
- **SIX AGENTS 卡**：60×60 彩边图标盒（`box-shadow: inset 0 0 12px 白×.03, 0 0 10px 自色×25%`）+ 彩色代号 + 白名称 + 18px 数值 + 7px 暗灰小标签；卡上方横线总线（`OBSERVATIONS IN / DECISIONS OUT`）+ 每卡 1px 竖线接上。
- **三元相图**：等边三角形虚线边，顶点 SIGNAL/STALE/NOISE 带辉光点，内部光点由三元组插值定位、带十字线。
- **曲线图**：SVG path 蓝紫渐变 stroke（`#8C90C0→#A2B3DF`）+ 青色发光光点 `getPointAtLength` 循环 + 3–5 个递减透明度小拖尾圆 + TRENDING 位置垂直金色虚线 + 方形 marker chip。

## 6. 动效

**全部动效都是"活数据微更新"级：位移 ≤ 数像素、透明度变化 ≤0.2、无弹跳、无旋转、无转场**（源视频 356 帧无任何转场动画）。更新周期 ≥600ms 避免抖动。

| 动效 | 参数 |
|---|---|
| 计数器递增 | 逐位翻滚，每秒 +0~1（泊松），translateY 0.35s ease-out |
| UTC 钟/头部统计 | 真实走秒/递增 |
| 日志流 | 600–1400ms 随机推入，transform 位移 |
| ORB | 细丝 ±3% 正弦呼吸（相位错开）、微粒沿丝/轨道缓动（0.05–0.15 rad/s）；每帧清屏 + `globalCompositeOperation:"lighter"` 叠加发光；**不做 O(n²) 距离连线** |
| 流线/曲线光点 | `getPointAtLength` 匀速，曲线光点带拖尾 |
| 数值微更新 | 每 1–2s ±0.1–0.5%，300ms 补间 |

- **实测为静态、勿做动画**：LIVE FEED 标签行（相位相关实测位移≈0，**不是跑马灯**）、SPIDER-ON 徽章（30 采样点亮度恒定）。
- reduced-motion：冻结所有定时器，Canvas 保留一帧静态画面。
- 工程化：定时器统一 Scheduler 管理；`document.hidden` 暂停 rAF 省电。

## 7. 特色技法

1. **点阵 LED 数字**：点阵字体或 CSS `radial-gradient` 点阵纹理 `-webkit-background-clip:text`，是"电子感"锚点。
2. **85:15 配色纪律**：霓虹色严格限 15% 面积。
3. **代号系统**：`[JDG]` 三字母 boxed chip + 专属色——让界面有"黑话"风味、像真实内部工具。
4. **放射爆发 ≠ 均匀粒子网络**：密度分层（星尘不连线 < 细丝辐射 < 虚线轨道 < 品红 hub 强辉光），绘序即层级。
5. **质感四件套**：噪点 .018 / 扫描线 .25 / 暗角 .38 / 文字辉光 6px——"若有若无"，过度即廉价滤镜。
6. **静态的真实性**：有些元素（标签行、徽章）实测就是静止的，克制比炫技更还原。

## 8. 落地陷阱（实踩）

1. 定时器"注册即执行"会放大初始化异常 → 部分区块静默空白但页面"看起来正常"。对策：初始化与定时更新分离 + **断言脚本逐项数元素**（`querySelectorAll('.agent').length===6`），不能只靠截图目视。
2. Canvas `canvas.width=` 清空画面 + ResizeObserver 异步 → resize 后主动 `draw()` 一帧。
3. 同步路径（如 reduced-motion 立即 draw）引用 `const` 注意 TDZ。
4. `◆ ▲ ★ ✓` 用 CSS 绘制，不依赖字体符号子集。
5. 计数器逗号建格处理，防千分位跳动。

## 9. 可复用模式

◆ 符号前缀 + 大写等宽标签 = 终端感三秒成型 · 色号即动作词典（SAVE/BLOCK/SKIP 上色即编码）· boxed chip 代号系统造"黑话"风味 · 密度分层绘序（尘<丝<轨<hub）· 一个 38% 的绝对主角 + 上密下疏节奏 · 质感四件套 ≤5% · 计数器泊松随机递增比线性更真实 · 无转场无弹跳——微动态全是数据驱动。
