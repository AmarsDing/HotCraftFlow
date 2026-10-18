# docs/UI —— 四套顶级深色数据 UI 的复刻素材库

四套从真实产品视频逐帧拆解的 UI 复刻档案。每套含：**源视频 → 逐帧复刻指南（参数手册）→ 可运行 Demo → 验证对比 → 设计思想学习文档（Why 层）**。

> 想**学设计思想** → 先读 `design-thinking/`（为什么这样设计）
> 想**动手复刻** → 读各目录下的复刻指南（全部实测参数），配合 demo.html 打开对照
> 想**直接开工做新页面** → 用 `.claude/skills/dark-tech-ui/`（已蒸馏成 skill，含十条铁律 + token 模板）
> 想**验证文档论断** → 打开 `design-thinking/demo/index.html`（原创第五套界面，内嵌 13 项"论断→落点"锚点面板，加 `?check=open` 直接展开），结论见 [design-thinking/demo-verify.md](design-thinking/demo-verify.md)

## 学习路径（推荐阅读顺序）

| 顺序 | 文档 | 学什么 |
|---|---|---|
| 0 | [design-thinking/00-总论.md](design-thinking/00-总论.md) | 四套设计共享的世界观与方法骨架 |
| 1 | [design-thinking/01-grokbot.md](design-thinking/01-grokbot.md) | 实时仪表盘：视线动线、语义色、事件编排 |
| 2 | [design-thinking/04-spiderorb.md](design-thinking/04-spiderorb.md) | FUI 终端美学：密度信仰、符号系统、微动态 |
| 3 | [design-thinking/02-dots.md](design-thinking/02-dots.md) | 图谱/作战室：人机权责叙事、叠压真实感 |
| 4 | [design-thinking/03-longbridge.md](design-thinking/03-longbridge.md) | 金融终端：信任工程、极端密度的秩序 |

## 目录结构

```
docs/UI/
├── README.md                        ← 你在这里
│
├── videos/                          源视频（竖屏手机录屏 1320×2868，内容在中部横带）
│   ├── grokbot-dashboard.mp4        11.1s · GrokBot 仪表盘
│   ├── spiderorb-terminal.mp4       5.9s  · SPIDERORB FUI 终端
│   ├── dots-graph-kiosa.mp4         26.4s · DOTS kiosa 执行图谱（深绿毛球）
│   ├── dots-nodemap-launch.mp4      15.8s · DOTS Node Map（Five dots launch team）
│   ├── longbridge-terminal-full.mp4 136s  · 长桥金融终端（macOS 桌面窗口完整版）
│   └── longbridge-terminal-douyin.mp4 20s · 长桥抖音剪辑版（有水印，仅存档参考）
│
├── design-thinking/                 ★ 设计思想学习文档（Why：设计师在想什么）
│   ├── 00-总论.md
│   ├── 01-grokbot.md
│   ├── 02-dots.md
│   ├── 03-longbridge.md
│   ├── 04-spiderorb.md
│   ├── demo-verify.md               ★ 检验报告：13/13 论断 PASS + 3 处措辞精修建议
│   └── demo/                        ★ 哲学检验台（原创第五套界面，单页零依赖）
│       ├── index.html / style.css / app.js
│       └── _shot6.png / _check-open.png  验收截图（网络渲染 / 检验面板展开）
│
├── 01-grokbot-dashboard/            GrokBot · AI Agent 通宵值班仪表盘
│   ├── UI复刻分析报告.md            参数手册（token/布局/动效全量实测）
│   ├── design-tokens.css            可直接引用的 CSS 变量
│   ├── demo.html                    复刻 Demo（需同目录 design-tokens.css）
│   └── frames/{video,demo}/         视频关键帧 + 复刻截图
│
├── 02-dots-mission-control/         DOTS · 图谱 / Node Map / 作战室 三态
│   ├── UI复刻指南.md / .html        参数手册（.html 为排版版，10.8MB 内嵌图）
│   ├── demo-nodemap.html            Node Map 复刻 Demo（单文件零依赖）
│   ├── replica/                     作战室复刻尝试（index.html 单文件）
│   ├── frames/                      作战室关键帧 f_*.png
│   │   ├── video-graph/             图谱视频新提取帧
│   │   └── video-nodemap/           Node Map 视频新提取帧
│   ├── ref-images/                  图谱/作战室/帖子参考图与局部放大
│   ├── compare-old/                 早期复刻对比图
│   └── compare-v3/                  v3 复刻对比图
│
├── 03-longbridge-terminal/          长桥 · 金融行情 Web 终端
│   ├── UI拆解与实现指南.md          参数手册（136 秒录屏逐帧拆解）
│   ├── demo/                        零依赖复刻（open index.html 即跑）
│   │   └── shots/                   复刻验收截图
│   └── frames/                      29 张录屏帧（01-19 整屏 + z01-10 局部）
│       └── video/                   新提取帧
│
└── 04-spiderorb-terminal/           SPIDERORB · 赛博朋克 FUI 研究机器人
    ├── UI复刻实现指南.md            参数手册（含 AI 生成提示词 §9）
    ├── 深度对比分析.md              视频 vs Demo 逐区块修正记录
    ├── 复刻验证报告.md              机器比对验证（色差/动效断言）
    ├── demo.html                    复刻 Demo（单文件零依赖，字体内嵌）
    └── frames/video/                视频新提取帧
```

## 备注

- **作战室源视频不在本目录**——仅存截图（`02-dots-mission-control/ref-images/ref_warroom_full.jpg`）与 8 张关键帧；指南中作战室内容基于这些静态材料。
- `02-*/replica/shot.js` 与 `03-*/demo/shoot.js` 内含旧机器的绝对路径（WorkBuddy/FrameWork 时期），移动后已失效，仅作实现参考。
- 长桥抖音版视频带第三方水印与字幕条，**不要**作为取色/布局依据，仅存档。
- 全部视频已加入 `.gitignore`，不会进 git。
- 蒸馏好的工程级设计 skill：`.claude/skills/dark-tech-ui/`（SKILL.md + 4 份 references + tokens-base.css）。
