# GitHub 热门追踪（Radar）深化开发文档

> 版本：v1 · 2026-10-17 · 作者：WorkBuddy
> 上游依赖：`doc/media-matrix-system-prd-v1.md`（TT 热点追踪子系统，FR-TRK-01~13）
> 代码基线：`apps/server/app/admin/internal/{biz,service}/radar_*`、`apps/web/src/pages/github-radar/*`、`apps/server/api/radar/v1/*.proto`
> 本文档是 radar 模块本轮深化的**唯一实施依据**；实施完成后按 §10 验收清单 Review。

---

## 1. 背景与目标

### 1.1 现状一句话
GitHub Radar 模块已有「热门/新星双榜 + 语言/Topic 过滤 + 盯盘提醒」的**演示级骨架**（前后端全链路可跑，但数据源是内存 Demo，信息密度低，无 AI 能力，无源码/克隆能力）。

### 1.2 本轮目标（按用户需求归纳）

| # | 目标 | 对应章节 |
|---|---|---|
| G1 | 榜单支持**日 / 周 / 月**多时间维度，按热度（窗口增量）从高到低排列 | §4.1 |
| G2 | 卡片达到 GitHub 官网级信息密度：语言技术栈、总星、协议（License）、窗口增量、Fork、Issue、更新时间、Topics | §4.2 |
| G3 | **AI 总结分析**项目，分三个等级（一句话 / 速览 / 深度） | §4.4 |
| G4 | **一键克隆**项目 + 应用内**查看源码**（文件树 + 代码阅读） | §4.5 / §4.6 |
| G5 | 多维检索：时间维度 × 语言 × Topic × License × 关键词 × 仅新晋，排序可切换 | §4.1 |
| G6 | 与平台设计体系（玻璃卡 / token 配色）保持一致的专业视觉 | §7 |

### 1.3 非目标（明确不做）
- 不做全仓库 DeepWiki 式永久索引（依赖外部服务或重量级管线，超出本轮范围；用「按需三级 LLM 总结」覆盖 80% 场景）。
- 不引入 ent 持久化（沿用内存 + TTL 缓存；持久化留待 TODO(ent) 演进，接口已按可替换设计）。
- 不动盯盘/通知/设置的既有交互模型，仅增强配置项（LLM / 克隆目录）。

---

## 2. 现状盘点（代码级）

### 2.1 已有资产

| 层 | 文件 | 现状 |
|---|---|---|
| proto | `api/radar/v1/{trending,watch,notify,config}.proto` | 4 个 Service；RepoItem 13 字段；`since` 仅 today/week |
| 生成 | `api/gen/go/radar/v1/*` + `apps/web/src/services/kratos/radar/*` | buf 全链路生成，勿手改 |
| biz | `biz/radar_trending.go` | `RadarRepoView` + **DemoRadarTrendingReader**（6 条硬编码内存数据，TODO(ent)） |
| service | `service/radar_{trending,watch,notify,config}_service.go` | trending 走 biz；watch/notify/config 均为**内存 Demo**；token 明文存内存（TODO 加密） |
| web 页面 | `pages/github-radar/RadarPage.vue`（687 行） | 筛选栏（热门/新星 × 今日/本周 × 搜索 × 语言 × Topic × 仅新晋）、KPI 卡 ×4、卡片网格、盯盘侧栏、详情弹窗、通知、设置 |
| web 组件 | `components/{RepoCard,Sparkline,WatchRail}.vue` | 卡片仅：排名/新晋/盯盘/名称/描述(2行)/语言点/★/Fork/增量/sparkline/3 个 topic |
| web 封装 | `services/index.ts` radar* 四个 | snake/camel 双读适配 |

### 2.2 差距分析（vs 目标）

| 能力 | 现状 | 差距 |
|---|---|---|
| 时间维度 | today / week | **缺 month**；无排序切换 |
| 数据源 | 6 条硬编码 Demo | **无真实 GitHub 数据通道**（这是最核心缺口） |
| 卡片信息 | 语言/★/Fork/增量 | **缺 License、Issue、更新时间、贡献者、精确星数** |
| AI 能力 | 无 | **缺**（三级总结） |
| 源码查看 | 无 | **缺**（文件树/内容） |
| 克隆 | 仅跳 GitHub 外链 | **缺**一键克隆命令/ZIP/服务端克隆 |
| 详情 | 弱弹窗（星/Fork/语言/Topic） | 信息量与 GitHub 仓库页差距大 |
| LLM 配置 | 无 | 需新增（provider/model/key 脱敏回显） |

---

## 3. 同类项目调研（2026-10-17）

### 3.1 对标清单

| 项目 / 产品 | 形态 | 关键做法（可借鉴） | 对我方启示 |
|---|---|---|---|
| **GitHub 官方 Trending** | 网页 | today/week/month × 语言过滤；按**窗口内星增量**排序；卡片含 owner/repo、描述、语言色点、总星、"N stars today"、built-by 贡献者头像；另有 Trending Developers 榜 | 卡片信息密度基准；**无官方 API** |
| **doforce/github-trending**（65★，MIT） | FastAPI+Scrapy | 爬 trending 页 → JSON；支持 daily/weekly/monthly + lang；返回 desc/stars/forks/build_by/change | 采集层可直接对标其字段集 |
| **tomowang/github-trending** | Docker 服务 | trending HTML 解析（h2>a、p、span[itemprop]、a[href*=stargazers] 选择器）；REST+CLI 双接口 | HTML 选择器策略参考 |
| **anmoanmo/GitHubTrendsProject** | Flask+cron | **双引擎**：爬虫优先拿 "stars today" 精确增量，失败自动降级 GitHub Search API；日/周/月历史归档可回溯 | **双引擎 + 快照归档**直接采纳 |
| **dev.to 零成本聚合器** | 200 行 Python + Actions | 纯 stdlib 正则解析服务端渲染 HTML；6h cron；同时产出 HTML+trending.json；保留上一个好快照兜底 | 「缓存上一个好快照 + stale 标记」策略 |
| **OSS Insight**（pingcap，开源） | 数据分析平台 | 100 亿+ GitHub events → TiDB；Trending 按综合分+语言过滤；Repo Analytics（stars/forks/issues/commits/contributors 历史趋势、star 地理/公司分布）；Collections 精选集；Compare 双仓对比；自然语言→SQL | 详情页指标集（issues/commits/contributors/趋势）参照；本模块做轻量子集 |
| **star-history.com** | 可视化 | Star 历史曲线；周榜（近期增量 Top20，带 ▲▼ 排名升降与 N 新入榜标记） | **排名升降 + 新入榜标记**增强盯盘/榜单体感 |
| **HelloGitHub**（17.3万★，CC BY-NC-ND） | 中文月刊 | 人工精选「有趣、入门级」；条目=项目名+链接+一句话简介+语言/技术标签+入门难度；分类归档；社区推荐 | **中文解读**是差异化点 → AI 总结默认中文输出 |
| **DeepWiki**（Cognition） | AI Wiki | URL 替换即得；分层解析（系统→页面→串联）；Fast 秒级 / Deep Research 20–60s 双模式问答；5 万+ 热门仓预索引 | **分级深度**理念 → 我方 brief/summary/deep 三级 |

### 3.2 调研结论（设计决策）

1. **数据层双引擎**（采纳 anmoanmo 模式）：trending HTML 为第一数据源（唯一能拿到官方口径 "stars today/week/month" 增量的途径），GitHub Search API（`created:>date sort:stars`）兜底。两者产物归一为同一 `RepoItem`。
2. **快照 + 缓存**：榜单快照进内存环Buffer（每窗口 key 一条，默认保留 30 天，每日容量有上限）；对外 TTL 缓存（trending 10min / detail 30min / readme 60min / tree 10min / content 10min）；抓取失败时返回**上一个好快照 + stale 标记**（proto 已有 `stale/stale_reason` 字段，沿用）。
3. **排序双轨**：`hot`=窗口增量降序（默认，GitHub 口径）；`stars`=总星降序；`updated`=push 时间降序；`new`=新晋优先。用户要求的「明星项目按热度最高到最低排列」= `hot`。
4. **AI 分级**（DeepWiki 分级理念轻量化）：L1 brief 一句话（<50字）→ L2 summary 速览（是什么/技术栈/亮点/适用场景/热度信号，结构化 Markdown）→ L3 deep 深度（架构与设计推测/优劣/社区健康度/上手建议/风险）。输入=repo 元数据 + README 截断（~8k 字符）+ 语言构成；输出默认中文，缓存 24h。
5. **卡片密度对齐 GitHub 官网并略超**：语言色点（GitHub 官方语言色表，前端 TS map 承载，规避 sass hex 门禁）+ 总星（title 显示精确值）+ 窗口增量 + Fork + License 徽章 + 相对更新时间 + Topics。
6. **合规红线不变**：不 vendor 任何 GPL/AGPL 代码；trending HTML 解析仅作数据采集参考实现（自写 Go 解析器，不抄第三方源码）；GitHub ToS 遵守（UA 标识、限流退避、token 服务端脱敏）。

---

## 4. 功能设计

### 4.1 多维榜单（热门榜）

**筛选维度**（筛选栏一行，允许换行）：

| 维度 | 控件 | 值域 |
|---|---|---|
| 榜单 | toggle | 热门 trending / 新星 rising |
| 时间窗口 | toggle | 今日 today / 本周 week / **本月 month**（新增） |
| 排序 | select（新增） | 热度 hot（默认）/ 总星 stars / 最近更新 updated / 新晋 new |
| 关键词 | input | 匹配 name/description/topics |
| 语言 | select | 动态语言 Top + 全量常见语言 |
| Topic | input | 精确匹配（逗号分隔多个，或关系由后端决定：任一命中） |
| License | select（新增） | MIT / Apache-2.0 / GPL-3.0 / BSD-3-Clause / AGPL-3.0 / 无 |
| 仅新晋 | toggle | 复用现有 |

**KPI 条**（保留并增强）：当前榜单数 / 新晋仓库数（可点击过滤）/ 有增量数 / 语言 Top；新增第 5 卡「License Top」与「更新时间」。

**空态/过期态**：沿用现有 empty-state 与 `stale` banner（文案补充「展示的是最近一次成功快照」）。

### 4.2 卡片（RepoCard v2）信息密度

单卡信息架构（自下而上）：

```
┌─────────────────────────────────────┐
│ #12  [新晋]  ●TypeScript      [🔖]  │  排名 · 新晋徽章 · 语言色点 · 盯盘
│ facebook / react                    │  owner 弱化 / name 加粗
│ The library for web and native UIs  │  描述 2 行 clamp
│ ★ 232k  +1.2k 本周  ⑂ 45.1k  🐛2.1k│  总星(title精确) · 窗口增量 · Fork · OpenIssues
│ 📜 MIT · 更新于 3 小时前             │  License 徽章 · 相对时间
│ ▁▃▅▇ sparkline   [ai] [clone] [源码]│  迷你趋势 · 快捷操作
│ #react #javascript #frontend        │  Topics ≤4
└─────────────────────────────────────┘
```

- 数字格式：≥1000 → `232k`（一位小数），≥1e6 → `1.2M`；title 显示精确值。
- 相对时间：<1min 刚刚 / <60min N 分钟前 / <24h N 小时前 / <30d N 天前 / 否则日期。
- 语言色点：GitHub 官方语言色表（TS map + inline style，不进 sass，过 color-gate）。
- 快捷操作（hover 显示）：AI 总结（打开详情并定位 AI 面板）、克隆（弹克隆对话框）、源码（打开源码浏览器）。

### 4.3 仓库详情面板（RepoDetailPanel）

替代现有弱弹窗，双栏 Dialog（宽屏 980px）：

- **头部**：owner/name、描述、外链 GitHub、盯盘、关闭。
- **左栏（元数据）**：License、语言、Stars、Forks、Open Issues、Watchers、创建/最近推送时间、默认分支、仓库体积、Clone/SSH/ZIP 链接、Topics 全量、语言构成条。
- **右栏（Tab）**：
  - `AI 分析`：三级入口卡（一句话/速览/深度）+ 生成按钮 + Markdown 渲染 + 模型/耗时/缓存标记。
  - `源码`：SourceBrowser（文件树 + 代码查看器）。
  - `克隆`：CloneDialog 内容（git clone 命令复制 / SSH / 下载 ZIP / 服务端克隆）。
- 数据入口：`GetRepo` 一次拉全（含可选 readme + 指定 level insight）。

### 4.4 AI 总结分析（三级）

| 等级 | level | 输出 | 长度 | 用途 |
|---|---|---|---|---|
| L1 一句话 | `brief` | 一句中文 TL;DR | ≤50 字 | 卡片浮层 / 快速扫榜 |
| L2 速览 | `summary` | 结构化 Markdown：**是什么 / 技术栈 / 核心亮点 / 适用场景 / 热度信号**（结合 stars/增量/新晋） | 200-400 字 | 详情页默认 |
| L3 深度 | `deep` | **架构与设计推测 / 优势与风险 / 社区健康度**（issues 响应、贡献者数、release 节奏推断）/ **上手建议** | 500-900 字 | 选型评估 |

实现要点：
- Provider：**OpenAI 兼容** chat/completions（`base_url` + `api_key` + `model` 可配，默认关）。未配置时 UI 提示前往设置，**不报错崩溃**。
- Prompt：system 固定为「资深开源生态分析师，输出简体中文 Markdown」；user 消息 = 元数据 + README 截断（去徽章/HTML 噪声，8k 字符上限）。
- 输出：Markdown（前端轻量渲染：标题/加粗/列表/代码块/段落）。
- 缓存：`(full_name, level)` 为 key，内存 TTL 24h；响应带 `cached/model/generated_at`。
- 成本护栏：单仓库单 level 每日最多生成 3 次（超限返回缓存或 429 语义错误码）；README 缺失时降级为仅元数据生成并在输出中标注「无 README，基于元数据推断」。

### 4.5 源码查看（SourceBrowser）

- 文件树：`GET /repos/{owner}/{repo}/git/trees/{default_branch}?recursive=1` 代理，前端折叠树（目录优先、名称排序）；>1000 条截断并提示。
- 文件内容：`GET /repos/{owner}/{repo}/contents/{path}` 代理；文本文件 base64 解码渲染（>512KB 截断提示）；二进制（含 NUL 或解码失败）提示「二进制文件，请下载」。
- 代码视图：等宽 + 行号 + 轻量语法着色（TS 侧正则高亮：注释/字符串/数字/关键字，覆盖主流语言近似即可）+ 路径面包屑 + 文件大小 + 「在 GitHub 打开」外链。
- 安全：仅代理公开仓库；路径校验（禁 `..`）；经平台 JWT。

### 4.6 一键克隆（CloneDialog）

| 方式 | 行为 |
|---|---|
| 复制 HTTPS 命令 | `git clone https://github.com/{full_name}.git` → 剪贴板 |
| 复制 SSH 命令 | `git clone git@github.com:{full_name}.git` |
| 下载 ZIP | 跳 `zipball_url`（GitHub 302 直链，新窗口） |
| 服务端克隆（可选，默认关） | 配置 `radar.clone_enabled` + `clone_dir` 后，服务端 `git clone --depth 1` 到该目录，返回本地路径与日志；用于后续源码分析链路 |

限流：服务端克隆同步执行有超时（60s）与并发上限（同时 1 个），失败返回可读原因。

### 4.7 配置项扩展（RadarConfig）

| 字段 | 说明 |
|---|---|
| `github_token` | 已有（脱敏回显不变） |
| `llm_enabled` / `llm_base_url` / `llm_api_key` / `llm_model` | 新增；api_key 脱敏回显 |
| `clone_enabled` / `clone_dir` | 新增；目录仅服务端可见（前端只显示是否开启） |

---

## 5. 接口设计（proto 变更）

### 5.1 `trending.proto`（扩展）

- `RepoItem` 新增字段（编号 13 起）：
  `license`(13) `license_spdx`(14) `homepage`(15) `open_issues`(16) `watchers`(17) `contributors`(18, repeated string) `created_at`(19) `updated_at`(20) `pushed_at`(21) `clone_url`(22) `ssh_url`(23) `zip_url`(24) `default_branch`(25) `languages`(26, repeated LanguageStat) `readme_excerpt`(27)
- `LanguageStat { string name=1; int64 bytes=2; double percent=3; }` 新增 message
- `ListTrendingRequest` 新增：`sort`(7)（hot|stars|updated|new）、`query`(8)、`license`(9)；`since` 值域扩展 `today|week|month`
- `ListTrendingResponse` 新增：`license_top`(8, map)、`window`(9, 回显实际窗口)
- `SearchRisingRequest/Response` 保持（rising 走搜索发现）

### 5.2 `repo.proto`（新增文件，同包 `radar.service.v1`）

```proto
service RepoService {
  rpc GetRepo(GetRepoRequest) returns (GetRepoResponse)         get  /api/v1/radar/repo
  rpc GetRepoTree(GetRepoTreeRequest) returns (GetRepoTreeResponse)   get  /api/v1/radar/repo/tree
  rpc GetRepoContent(GetRepoContentRequest) returns (GetRepoContentResponse) get /api/v1/radar/repo/content
  rpc CloneRepo(CloneRepoRequest) returns (CloneRepoResponse)   post /api/v1/radar/repo/clone
  rpc GenerateInsight(GenerateInsightRequest) returns (GenerateInsightResponse) post /api/v1/radar/insight
}
```

消息要点：
- `GetRepoRequest { full_name; include_readme; insight_level }`；`GetRepoResponse { RepoItem repo; string readme; AIInsight insight; bool stale; string stale_reason }`
- `GetRepoTreeRequest { full_name; branch }` → `{ repeated TreeNode nodes; string default_branch; bool truncated }`，`TreeNode { path; type(blob|tree); size }`
- `GetRepoContentRequest { full_name; path; ref }` → `{ path; name; content; encoding; size; truncated; html_url; download_url }`
- `CloneRepoRequest { full_name; to_server }` → `{ clone_url; ssh_url; zip_url; command; local_path; cloned; message }`
- `GenerateInsightRequest { full_name; level }` → `{ AIInsight insight }`
- `AIInsight { full_name; level; content_markdown; model; generated_at; cached; duration_ms; error }`

### 5.3 `config.proto`（扩展）

`SetRadarConfigRequest` 增加 `llm_enabled/llm_base_url/llm_api_key/llm_model/clone_enabled/clone_dir`；两个 Response 同步增加脱敏/状态回显字段。

### 5.4 生成与注册

```bash
cd apps/server/api && PATH="$PATH:/c/Users/Administrator/go/bin" buf generate
```

- Go：`api/gen/go/radar/v1/repo.pb.go` 等；TS：`apps/web/src/services/kratos/radar/service/v1/index.ts`
- `server/rest.go`：新增 `radarRepoSvc` 参数 + `radarV1.RegisterRepoServiceHTTPServer(srv, radarRepoSvc)`
- `cmd/wire_gen.go`：新增 `radarGitHubClient` → `radarRepoService` → 传入 NewRESTServer（与现有 radar 四件套同模式）

---

## 6. 后端设计

### 6.1 新增/改动文件

| 文件 | 职责 |
|---|---|
| `biz/radar_github_client.go` | GitHub REST 客户端：`FetchTrending(ctx, since, language)` 双引擎、`GetRepo`、`GetReadme`、`GetTree`、`GetContent`、`CloneRepo`；TTL 内存缓存；UA/限流退避；stale 快照回退 |
| `biz/radar_trending.go`（改） | `RadarRepoView` 补全新字段；新增 `GitHubRadarTrendingReader` 实现 `RadarTrendingReader`；Demo reader 保留为 `RADAR_DEMO=1` 开关 |
| `biz/radar_insight.go`（新） | OpenAI 兼容客户端 + 三级 Prompt 构建 + 缓存 + 频控 |
| `biz/radar_repo.go`（新） | RepoUsecase：编排 client + insight（detail/tree/content/clone/insight） |
| `service/radar_repo_service.go`（新） | `RepoService` 实现（DTO 映射） |
| `service/radar_trending_service.go`（改） | 透传 sort/query/license/window |
| `service/radar_config_service.go`（改） | LLM/克隆配置存取与脱敏 |
| `server/rest.go`、`cmd/wire_gen.go`（改） | 注册与装配 |

### 6.2 GitHub 客户端要点

- BaseURL `https://api.github.com`；trending HTML `https://github.com/trending?since=&language=`（UA：`FrameWork-Radar/1.0 (+https://github.com)`）。
- **双引擎**：`FetchTrending` 先试 HTML（解析 `article.Box-row`：h2>a=full_name、p=desc、span[itemprop=programmingLanguage]、stargazers 链接、`span.float-sm-right`="N stars today"）；HTML 失败/空 → Search API（`q=created:>={windowStart} sort=stars order=desc per_page=50`），此时 `stars_delta` 由快照差计算（无快照记 0 并标注 `delta_source=snapshot`）。
- **缓存**：key=`trending:{since}:{lang}` TTL 10min；detail key=`repo:{full}` TTL 30min；readme 60min；tree/content 10min。缓存命中仍可正常返回。
- **stale**：抓取失败 → 返回上一份好快照（内存保留）+ `stale=true, stale_reason`；无任何快照 → 返回空 + stale。
- **限流**：读 `X-RateLimit-Remaining`；低于阈值或 403/429 → 指数退避（1s/2s/4s，最多 3 次）后标 stale。无 token 时 60 次/小时的限额通过缓存 + 降级缓解。
- **并发**： trending 抓取用 `errgroup` 限并发 4；内容请求单次。

### 6.3 AI Insight 要点

- 客户端：POST `{base_url}/chat/completions`（OpenAI 兼容），`Authorization: Bearer {key}`，body `{model, messages:[{system},{user}], temperature:0.3, max_tokens}`；30s 超时。
- 三级 Prompt 模板（简体中文输出，Markdown），输入预算：元数据 JSON + README 截断 8000 字符。
- 缓存：`insight:{full}:{level}` TTL 24h；频控：每 (repo,level) 每日 3 次生成（计数内存，重启清零，可接受）。
- 失败：上游错误 → `AIInsight.error` 可读文案，HTTP 200（业务错误在 payload），前端 toast 提示。

### 6.4 克隆要点

- `CloneRepo(to_server=false)`：仅返回命令与链接（纯计算）。
- `to_server=true` 且 `clone_enabled`：`exec.Command("git", "clone", "--depth", "1", cloneURL, dir)`，60s 超时；目录校验（必须在 clone_dir 下、禁路径穿越）；返回 local_path + message。git 不可用 → 可读错误。

---

## 7. 前端设计

### 7.1 组件树

```
pages/github-radar/
├── RadarPage.vue            （改：筛选栏增强/KPI 增强/详情面板替换/快捷入口）
├── types.ts                 （改：RepoItem 新字段 + RepoDetail/Insight/Tree/Content/Clone 类型）
└── components/
    ├── RepoCard.vue         （改：密度对齐 §4.2 + 快捷操作 + 语言色点）
    ├── Sparkline.vue        （不变）
    ├── WatchRail.vue        （小改：排名升降/新入榜标记）
    ├── RepoDetailPanel.vue  （新：§4.3 双栏详情 + AI Tab + 克隆 Tab）
    ├── RepoInsight.vue      （新：三级 AI 卡片 + Markdown 渲染 + 生成状态）
    ├── MarkdownView.vue     （新：轻量 MD 渲染：标题/加粗/行内代码/列表/代码块/引用）
    ├── SourceBrowser.vue    （新：文件树 + 代码查看 + 轻量高亮）
    └── CloneDialog.vue      （新：§4.6）
services/index.ts            （改：radarRepoService 封装 + radarConfigService 新字段）
```

### 7.2 状态与数据流

- `RadarPage`：筛选状态（mode/since/sort/query/language/topic/license/onlyNew）→ watch 触发 `loadBoard()`；`loadBoard` 按 mode 分流 `listTrending/searchRising`，新增 `sort/license/query` 透传。
- 详情：点卡片 → `radarRepoService.getRepo(full_name, {includeReadme:true, insightLevel:''})` → 打开 `RepoDetailPanel`；AI Tab 内按 level 调 `generateInsight`（或首次随详情带出 L2）。
- 源码 Tab：`getRepoTree` → 树；点文件 → `getRepoContent` → 渲染。
- 克隆 Tab：`cloneRepo`（toServer=false）拿命令；可选服务端克隆（loading + 结果）。
- 错误：沿用 `Notify.create` 负向 toast；AI 未配置时面板内联提示 + 「去设置」按钮。

### 7.3 样式约束（强制）

- 只用既有 token：`--color-accent/success/warning/info`、`--glass-*`、`--space-*`、`--text-*`；**禁止 hex/命名色**（color-gate + stylelint 双门禁）。
- 卡片/面板：`.dash-card` 体系；Dialog：`.app-dialog-card` + `.app-glass-dialog`；筛选栏：`.app-filter-bar`；KPI：`.app-stat-strip/.app-stat-card`。
- 语言色点：TS map（GitHub 官方色）→ inline `style` 绑定，不落 sass。
- 密度：与平台控件密度 token 一致；暗色主题（当前 IDE dark）可读。

---

## 8. 实施计划（批次与验收）

| 批次 | 内容 | 验收标准 |
|---|---|---|
| B1 文档 | 本文档 | 用户确认范围 |
| B2 proto+codegen | §5 全部 + buf generate | `git status` 见 gen 更新；`go build ./...` 过 |
| B3 后端采集与详情 | GitHub 客户端双引擎、trending 扩展、RepoService 详情/树/内容/克隆、配置扩展、装配 | `go build ./...` 过；curl（带 token）`/api/v1/radar/trending?since=month&sort=hot` 返回真实数据或明确 stale；`/repo?full_name=vercel/next.js` 返回 license/open_issues 等 |
| B4 后端 AI | Insight 三级 + 配置 | 配置 LLM 后 `generateInsight` 三级均返回中文 Markdown；未配置时返回可读 error 且不 500 |
| B5 前端 | §7 全部组件 + service 封装 | `npm run type-check`、`npm run lint`、`npm run color:gate`、`stylelint` 全过；页面交互走查（暗色） |
| B6 Review | 按 §10 清单逐条核对 + 构建验证 + 报告 | 清单全过或遗留项明确记录 |

---

## 9. 风险与合规

| 风险 | 缓解 |
|---|---|
| GitHub trending HTML 结构变化 | 双引擎自动降级 Search API；解析失败保留旧快照 + stale 文案；解析器集中单文件便于修 |
| 无 token 限流（60/h） | 缓存 TTL + 设置页引导填 token；响应带 stale 语义 |
| GitHub ToS / 爬取合规 | 仅采集公开榜单页与公开 API；UA 标识；低频（TTL 10min）；不突破 robots 与频率限制 |
| LLM 成本/密钥安全 | key 服务端存储脱敏回显（沿用既有 maskToken 模式，TODO 加密）；频控每日 3 次/级；temperature 低 |
| 服务端克隆安全 | 默认关；目录限定 clone_dir；禁路径穿越；60s 超时；并发 1 |
| 大体量仓库 tree 截断 | recursive=1 >1000 条截断提示；content >512KB 截断 |
| 演示数据残留 | Demo reader 仅 `RADAR_DEMO=1` 启用，默认真实客户端 |

---

## 10. 验收清单（Review 用）

**功能**
- [ ] 榜单支持 今日/本周/本月；默认按热度（窗口增量）降序
- [ ] 排序可切换 热度/总星/最近更新/新晋
- [ ] 过滤：关键词/语言/Topic/License/仅新晋 均生效且可重置
- [ ] 卡片显示：排名、新晋、语言色点、owner/name、描述、总星(title精确)、窗口增量、Fork、Open Issues、License、相对更新时间、sparkline、Topics
- [ ] 详情面板：完整元数据 + AI 分析 Tab + 源码 Tab + 克隆 Tab
- [ ] AI 三级：brief/summary/deep 均可生成，中文 Markdown，带模型/缓存标记；未配置 LLM 时友好提示
- [ ] 源码浏览器：文件树可展开、代码可读（行号+着色）、大文件/二进制有提示、可跳 GitHub
- [ ] 克隆：HTTPS/SSH 命令一键复制、ZIP 下载、服务端克隆（开启后）
- [ ] 盯盘/通知/设置既有功能不回退；设置页可配 LLM 与克隆
- [ ] stale/空态/错误态文案可读

**工程质量**
- [ ] `go build ./...` 通过
- [ ] `buf generate` 产物已提交且未手改 gen
- [ ] 前端 type-check / eslint / stylelint / color:gate 通过
- [ ] 无 hex/命名色新增（sass 区域）
- [ ] 暗色主题可读，玻璃卡/筛选栏/KPI 与平台同肤
- [ ] 接口均经平台 JWT；token/key 不明文回显

---

*附：术语 — 「窗口增量」= trending 页口径的 stars today/week/month；「快照」= 一次成功抓取的全量榜单结果。*
