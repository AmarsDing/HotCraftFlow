# 全媒体内容矩阵系统 · 开发文档体系

> **体系版本**：v1.0 · **建立日期**：2026-10-17（Asia/Shanghai）
> **上游权威文档**：[`doc/media-matrix-system-prd-v1.md`](../../doc/media-matrix-system-prd-v1.md)
> **关系说明**：PRD 是**唯一权威需求源**；本目录是 PRD 的**工程化展开**，按「需求 → 概要设计 → 详细设计 → 开发进度」四段式落地到五个子系统。
> **冲突处理**：若本文与 PRD 不一致，**以 PRD 为准**并回来修正本文；若 PRD 未覆盖而本文已细化，以本文为准并回写 PRD。

---

## 1. 子系统速览

| # | 目录 | 子系统 | 全称 | 一句话职责 | PRD 章节 |
|---|------|--------|------|-----------|---------|
| ① | [`01-IC`](./01-IC/) | **IC** | Intelligence Collection 情报收集 | 从国内外媒体平台持续采集合规情报，产出带证据链的 `IntelItem` | §3 |
| ② | [`02-TT`](./02-TT/) | **TT** | Trend Tracking 热点追踪 | 把情报时间序列化，判定热度/生命周期/突发，输出选题建议 | §4 |
| ③ | [`03-MIA`](./03-MIA/) | **MIA** | Market Impact Analysis 市场影响分析 | 判断消息对股市/汇市/数字货币的影响面，产出 `ImpactCard` | §5 |
| ④ | [`04-MME`](./04-MME/) | **MME** | Matrix Montage Engine 短视频矩阵混剪 | 一源 N 版 × 多画幅 × 多变体批量成片 | §6 |
| ⑤ | [`05-MAP`](./05-MAP/) | **MAP** | Media Auto-Publisher 媒体自动化发布矩阵 | 适配器矩阵 + 审批门 + 排期发布 + 数据回流 | §7 |
| ⑥ | [`06-RADAR`](./06-RADAR/) | **RADAR** | GitHub Hotspot Analysis GitHub 热点分析 | GitHub 热门榜/新星/AI 解读/源码速览/克隆 + 个人盯盘与变更通知（菜单「github热门追踪」） | —（独立迁入模块，文档按 as-built 口径） |

**数据流（单向主干 + 回流旁路）**：

```
① IC ──IntelItem──► ② TT ──TopicCandidate──┬──► ③ MIA ──ImpactCard──► 简报/推送
                                            └──► ④ MME ─VariantSet─► ⑤ MAP ──► 各平台
                                                                        │
                                                              表现数据回流 ─► ② TT / ① IC
```

> **纪律**：MIA 产出**分析结论**，MME 产出**可发布内容**，二者目标不同，**不得混为同一按钮**。
> **⑥ RADAR 不在主干**：它是独立 GitHub 开源情报工具（迁入自独立项目 github-radar），**不产出/消费七份契约消息**，仅复用平台 JWT、菜单、加密与 retention 基建。

---

## 2. 每个子系统的四份文档

| 文档 | 回答什么问题 | 主要读者 |
|------|-------------|---------|
| **01-需求规格.md** | 做什么、不做什么、做到什么算完成 | 产品、法务、测试 |
| **02-概要设计.md** | 拆成哪些模块、模块间怎么流、依赖谁、为什么这么选 | 架构、tech lead |
| **03-详细设计.md** | 表怎么建、算法怎么写、接口怎么定、出错怎么办 | 开发工程师 |
| **04-开发进度.md** | 谁在什么时候做完什么、砍单顺序是什么 | 项目管理、全员 |

---

## 3. 共享文档（读子系统前先看）

| 文档 | 内容 |
|------|------|
| [`00-shared/01-data-contract.md`](./00-shared/01-data-contract.md) | **七份跨系统 JSON 契约**：`IntelItem` / `TrendSeries` / `ImpactCard` / `VariantSet` / `DispatchUnit` / `TopicCandidate` / `Alert`。这是五子系统唯一的衔接语言，**任何跨系统调用只允许传这些结构** |
| [`00-shared/02-tech-selection.md`](./00-shared/02-tech-selection.md) | 开源选型总表、许可证实测快照、**GPL/AGPL 红线清单**、NFR-19 财经类引入复核规则 |
| [`00-shared/03-compliance.md`](./00-shared/03-compliance.md) | 著作权、新闻版权、投资建议、平台 ToS、PIPL、抓取合规、风控熔断 |

---

## 4. 全系统统一技术基线

本仓库既有骨架（`apps/server` + `apps/web`）的**强制**基线，五子系统的详细设计均基于此，不另起炉灶。

| 层 | 选型 | 版本/路径 |
|----|------|-----------|
| 语言 / 模块 | Go · `module framework` | Go 1.26.1 |
| 框架 | **Kratos v2** | v2.9.2 |
| 契约 | **Proto-first + buf** | `apps/server/api/**/*.proto` → `api/gen/go/**`（Go）与 `apps/web/src/services/kratos/**`（TS） |
| ORM / 迁移 | **Ent** + Atlas | Ent v0.14.6；schema 手写于 `app/admin/internal/data/ent/schema/` |
| 依赖注入 | **Wire** | `providers` / `wire.go` |
| 关系库 | **PostgreSQL** | 本机 `localhost:5434`（主 `framework` / 日志 `framework_log`） |
| 缓存 | **Redis** | 本机 `127.0.0.1:6380` |
| 异步任务 | **asynq** | v0.26.0；目录 `app/admin/internal/worker` |
| 消息总线 | **NATS / JetStream** | `pkg/nats` · `pkg/natsx`；本机 `D:/nats-server` |
| 时序库 | **InfluxDB 2.x** | 本机 `D:/influxdb2-2.9.1-windows_amd64`；仅 TT 使用 |
| 前端 | **Quasar** | `apps/web`；页面 `src/pages/**`，TS 客户端由 buf 生成 |
| 观测 | `pkg/observability` · `pkg/log` | 指标/链路/结构化日志 |

**Proto 分包约定**（沿用后端指南 §6 方案 A，独立 package 注册进 admin 进程）：

```
apps/server/api/media/v1/
  ic/     source.proto  collector.proto  intel.proto
  tt/     topic.proto   trend.proto      alert.proto
  mia/    mapping.proto event.proto      impact.proto
  mme/    task.proto    variant.proto    render.proto
  map/    channel.proto dispatch.proto   approval.proto
```

`option go_package` 统一前缀：`framework/api/gen/go/media/v1/<pkg>;<pkgalias>`

**HTTP 前缀**：`/api/v1/media/{ic|tt|mia|mme|map}/...`；⑥ RADAR 独立前缀 `/api/v1/radar/...`（proto package `radar.service.v1`，见 `06-RADAR/02-概要设计.md` §6）

---

## 5. 文档状态总览

| # | 子系统 | 需求 | 概要设计 | 详细设计 | 开发进度 | 开发状态 |
|---|--------|------|---------|---------|---------|---------|
| ① | IC | ✅ | ✅ | ✅ | ✅ | **未启动**（Phase 1） |
| ② | TT | ✅ | ✅ | ✅ | ✅ | **未启动**（Phase 1） |
| ③ | MIA | ✅ | ✅ | ✅ | ✅ | **未启动**（Phase 2） |
| ④ | MME | ✅ | ✅ | ✅ | ✅ | **MVP 0.8 执行中**（2026-10-20 → 12-12） |
| ⑤ | MAP | ✅ | ✅ | ✅ | ✅ | **未启动**（Phase 2） |
| ⑥ | RADAR | ✅ | ✅ | ✅ | ✅ | **已嵌入投产**（v2 功能全集；API Resource 挂载等待办见 `06-RADAR/04` §4） |

> **状态口径**：截至 2026-10-17，①~⑤子系统均无生产代码。`04-MME` 的 MVP 0.8 排期已冻结并即将启动，是本体系的**地基**，其余子系统在其后接续。⑥ RADAR 是例外——代码先行（迁入 + v2 迭代已完成），文档为 2026-10-18 补建的 as-built 描述。

---

## 6. 阅读顺序建议

| 你是 | 建议路径 |
|------|---------|
| **新加入的后端开发** | `README` §4 → `00-shared/01-data-contract.md` → 目标子系统 `01 需求` → `02 概要` → `03 详细` |
| **前端开发** | `README` §4 → `00-shared/01-data-contract.md` → 各子系统 `02 概要` 的「前端交互」小节 |
| **产品/法务** | `00-shared/03-compliance.md` → 各子系统 `01 需求规格` → PRD §12 待决策问题 |
| **项目管理** | `README` §5 → 各子系统 `04 开发进度` → PRD §11 路线图 |
| **架构评审** | `README` → 全部 `02 概要设计` → `00-shared/02-tech-selection.md` |

---

## 7. 命名与写作纪律

1. **FR 编号**沿用 PRD：`FR-INT-*` / `FR-TRK-*` / `FR-MIA-*` / `FR-MIX-*` / `FR-PUB-*`，本文**不新造编号**；确需新增时在子系统内以 `FR-XXX-1x` 追加并回写 PRD §14.2。
2. **命名对齐**：目录用 ASCII（受安全策略限制），文档正文一律用子系统缩写（IC/TT/MIA/MME/MAP）+ 中文全称。
3. **不编造**：涉及开源项目的 ★ 数与许可证一律标注实测日期；未实机验证的写 `[待核实]` 或 `[未实机验证]`。
4. **诚实边界**：每个子系统的「需求规格」末节必须写「明确不做」与「诚实声明」，禁止把目标态写成已实现态。
5. **禁手改生成物**：`api/gen/go/**`、`data/ent/**`（除 `schema/`）、`apps/web/src/services/kratos/**` 三者一律禁止手改——沿用后端架构指南 §3.3。

---

## 8. 待决策问题（阻塞开发，需拍板）

完整清单见 PRD §12。**阻塞 Phase 1 启动的三条**：

| # | 问题 | 阻塞什么 | 建议 |
|---|------|---------|------|
| **Q1** | 是否做「他人热点内容切片搬运」 | MME 素材来源与法务风险量级 | **V1 不做**，热点仅作选题灵感 |
| **Q4** | 是否自部署 newsnow | IC 采集可靠性 | **必须自部署**，勿打公共实例 |
| **Q6** | FreshRSS 处置（实测 AGPL-3.0 撞红线） | IC 订阅方案 | **不用**，以 RSSHub 独立服务 + 自研订阅表替代 |

---

*文档体系建立于 2026-10-17。上游 PRD：`doc/media-matrix-system-prd-v1.md`；归档原文：`doc-archive-20261017/`。*
