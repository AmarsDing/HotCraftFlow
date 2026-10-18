# 00 · 技术选型总表与许可证红线

> **版本**：v1.0 · **日期**：2026-10-17
> **上游**：PRD §9（技术选型总表）
> **证据口径**：★ 数与 SPDX 许可证均为 **2026-10-17 GitHub API 实测快照**。元数据会漂移，**任何依赖引入前必须复核**。
> **红线地位**：本文的「禁 vendor」清单是 **NFR 级强制约束**，违反者不得合入。

---

## 1. 姿态图例

| 姿态 | 含义 | 代码可进主仓？ |
|------|------|--------------|
| **Depend** | 直接依赖库或自部署独立服务（HTTP 调用） | ✅ 库可以；独立服务以进程外方式接 |
| **Reimpl** | 只学契约/思路，自行实现 | ❌ 不引入，参考实现留注释与文档链接 |
| **UX** | 只学交互与产品形态 | ❌ 严禁拷代码 |
| **禁 vendor** | GPL/AGPL/无 SPDX/商用不明 | ❌ **绝对禁止**源码进主仓或 fork |

---

## 2. 选型总表

| 能力层 | 子系统 | 推荐 | ★ | 许可证 | 姿态 | 备注 |
|--------|--------|------|---|--------|------|------|
| **多平台热榜聚合** | IC / TT | **newsnow** | 22,062 | **MIT** ✅ | **Depend / 自部署** | 覆盖微博/百度/知乎/头条等；作者公开呼吁勿高频打公共实例 → **必须自部署** |
| **长尾 RSS 订阅** | IC | **RSSHub** | 46,549 | **AGPL-3.0** ⚠️ | **独立服务 + HTTP** | 禁止 fork/嵌入；不得对外 SaaS 化而不开放其源码 |
| 自托管 RSS 聚合 | IC | ~~FreshRSS~~ | 16,298 | **AGPL-3.0** ❌ | **禁 vendor** | ⚠️ **原调研误列为主力，此处纠正**：以 RSSHub 独立服务 + 自研订阅表替代 |
| 舆情/生命周期/爆火检测 | TT | ~~TrendRadar~~ | 62,957 | **GPL-3.0** ❌ | **禁 vendor**（学权重模型与阶段定义） | 功能最贴合但许可证致命 |
| **网页 → 干净 Markdown** | IC | **Crawl4AI** | — | Apache-2.0 ✅ | **Depend 默认** | 须保留署名、NOTICE 与 badge（NFR-14） |
| 新闻正文抽取 | IC / MIA | news-please | 2,494 | **Apache-2.0** ✅ | 可 Depend | 合法授权范围内使用 |
| **财经情感分类** | MIA | **FinBERT**（`ProsusAI/finBERT`） | 2,253 | **Apache-2.0** ✅ | **Depend**（自托管推理） | 英文强，中文弱 → 中文走 LLM |
| **加密行情** | MIA | **ccxt** | 44,396 | **MIT** ✅ | **Depend（仅只读）** | 100+ 交易所；**交易能力一律不启用** |
| 股票/宏观行情 | MIA | OpenBB | — | **[待核实]** | 待定 | ⚠️ 主仓库名与许可证本轮未能确认，引入前**必须**复核 |
| 金融 LLM（可选） | MIA | FinGPT | 21,412 | MIT | 可选 | 偏研究，算力成本高 |
| 量化回测（可选） | MIA | qlib | 49,408 | MIT | 可选 | 用于新闻因子收益检验 |
| 中美财经源采集（可选） | MIA / IC | FinNLP | 1,491 | MIT | 可选 | ⚠️ 末次 push 2024-07，**活跃度低** |
| 中文直播采集 | IC | DouyinLiveRecorder / rust-srec | — | MIT | 侧车 Depend | **须授权录制** |
| **时序存储** | TT | **InfluxDB 2.x** | — | 开源 | 依赖 | 本机已有 `D:/influxdb2-2.9.1-windows_amd64` |
| **消息总线** | 全部 | **NATS / JetStream** | — | Apache-2.0 ✅ | 依赖 | 本机已有 `D:/nats-server`；仓库已有 `pkg/natsx` |
| 切片流水线 | MME | **OpenClip** | — | MIT | 工程模板 / 思路 Depend | |
| 切片 UX | MME | ~~HotClip / ViralMint~~ | — | **AGPL-3.0** ❌ | **UX-only** | |
| **本地 ASR** | MME | **FunASR** / Whisper | — | MIT ✅ | **Depend** | 禁止锁定云 ASR（NFR） |
| Ask / EDL / 自检 | MME | video-use | — | MIT | Reimpl | |
| 深度双报告 | MME | VideoDevour | — | MIT | 学 UX + 契约 | |
| 报告 → 解说片 | MME | html-video | — | Apache-2.0 | 主路径可学 | |
| **剪映交接** | MME | **pyJianYingDraft** | — | Apache-2.0 ✅ | **Depend** | |
| **发布调度 (A) 层** | MAP | Open-Dispatch | — | MIT | 抄契约自研，作 dry-run 参照 | ⚠️ **必须改默认 `public` → `private`**（NFR-17） |
| **审批门 (B) 层** | MAP | **pendpost** | — | MIT | 学范式 / 旁路 | (A) 不能替代 (B) |
| 厚调度 UX | MAP | CogSend / YominPost | — | MIT | UX | YominPost 的 MCP **故意无 publish**，正合审批门思路 |
| 社媒日历画布 | MAP | ~~Postiz / OpenPost~~ | — | **AGPL** ❌ | **UX-only**（NFR-12） | |
| NLE 交接 | MME | premiere-xml / fcp-mcp / Mark Player | — | MIT | 可审 / 可选 / UX | |
| 加密新闻（可选） | MIA | cryptopanic-mcp-server | 74 | MIT | 可选 | 依赖第三方配额与 ToS |

---

## 3. 许可证红线清单（强制）

### 3.1 主仓许可证

自研代码一律 **MIT 或 Apache-2.0**。

### 3.2 GPL / AGPL 禁令

以下项目**源码禁止进主仓、禁止 fork、禁止 vendor**；仅允许「独立进程部署 + HTTP 调用」或「UX 参考」：

| 项目 | 许可证 | 允许的用法 |
|------|--------|-----------|
| **TrendRadar** | GPL-3.0 | 仅学权重模型与生命周期阶段定义，**不引代码** |
| **RSSHub** | AGPL-3.0 | 独立容器部署，主仓仅 HTTP 调用 |
| **FreshRSS** | AGPL-3.0 | **不用**（见 §2 纠正项） |
| **HotClip / ViralMint** | AGPL-3.0 | UX-only |
| **ArcReel / OpenMontage / podcli / KnowClip / PullMD** | AGPL（族） | UX-only |
| **Postiz / OpenPost** | AGPL | UX-only（NFR-12） |

> **AGPL 网络服务边界说明**：RSSHub 以独立进程部署、主仓仅通过 HTTP 调用，属程序间通信，不触发主仓的 AGPL 传染。**但**：① 不得 fork 或嵌入主仓代码；② 若对外提供 SaaS，**必须**开放该服务的源码。

### 3.3 无 SPDX 禁令

| 项目 | 状态 | 处置 |
|------|------|------|
| `premiere-agent` | 无 LICENSE | 澄清前**禁 vendor** |
| `pyCapCut` | 无 LICENSE | 澄清前**禁 vendor** |
| `trending-hub` | 无 LICENSE | 澄清前**禁 vendor** |

### 3.4 Attribution 义务

| 依赖 | 义务 |
|------|------|
| **Crawl4AI** | 保留 Apache-2.0 署名、NOTICE 文件与 badge（NFR-14） |
| **newsnow / RSSHub** | 在 NOTICE 中登记来源与许可证 |
| 全部第三方 | 随仓库维护 `NOTICE` 文件，列明名称 / 许可证 / 用途 / 获取地址 |

---

## 4. NFR-19：财经/市场类第三方引入复核（本轮新增）

MIA 子系统的第三方引入**额外**要求四项复核，缺一不可：

| # | 复核项 | 具体要求 | 责任人 |
|---|--------|---------|--------|
| ① | **SPDX 实测** | 通过 GitHub API 读取 `license.spdx_id`，**不得**凭 README 徽章或描述推断 | 架构 |
| ② | **数据源 ToS 与配额** | 书面确认数据源的服务条款允许本项目用法，并记录配额上限 | 法务 + 架构 |
| ③ | **只读约束** | 行情类依赖**只启用只读**；交易、下单、撤单能力**一律不启用**，并在配置层做硬开关 | 开发 |
| ④ | **免责声明** | 所有对外输出强制带免责声明字段（契约层 `disclaimer` 非空校验） | 开发 |

> **理由**：财经类输出一旦被误读为投资建议，风险量级远高于普通内容分发。NFR-19 是**风控前置**而非流程负担。

---

## 5. 本仓库既有技术基线（不重新选型）

以下为 `apps/server` + `apps/web` 已确定的骨架，五子系统**必须**复用，不另起炉灶：

| 层 | 选型 | 版本 | 备注 |
|----|------|------|------|
| 语言 / 模块 | Go · `module framework` | 1.26.1 | |
| 框架 | **Kratos v2** | v2.9.2 | 分层 `server → service → biz → data` |
| 契约 | **Proto-first + buf** | — | Go 产物 `api/gen/go/**`，TS 产物 `apps/web/src/services/kratos/**`，**均禁手改** |
| ORM / 迁移 | **Ent** + Atlas | Ent v0.14.6 | schema 手写于 `app/admin/internal/data/ent/schema/` |
| 依赖注入 | **Wire** | v0.7.0 | |
| 关系库 | **PostgreSQL** | 本机 `:5434` | 主库 `framework` / 日志库 `framework_log` |
| 缓存 | **Redis** | 本机 `:6380` | |
| 异步任务 | **asynq** | v0.26.0 | 目录 `app/admin/internal/worker` |
| 消息总线 | **NATS / JetStream** | — | `pkg/nats` · `pkg/natsx` |
| 时序库 | **InfluxDB 2.x** | 2.9.1 | 仅 TT 使用；本机 `D:/influxdb2-2.9.1-windows_amd64` |
| 前端 | **Quasar** | — | `apps/web` |
| 观测 | `pkg/observability` · `pkg/log` | — | 指标 / 链路 / 结构化日志 |

### 5.1 后端落地强制纪律

沿用 [`docs/guides/backend-architecture-guide.md`](../../guides/backend-architecture-guide.md)：

- **禁手改生成物**：`api/gen/go/**`、`data/ent/**`（`schema/` 除外）、`apps/web/src/services/kratos/**`
- 改 proto 后**必须** `buf generate`，再编译 server 与 web
- 新增业务走**方案 A**：独立 proto package（如 `api/media/v1/ic/*.proto`），注册进 admin 进程
- 新增菜单需在 `seed_service.go` 的 `platformMenuSeedRecords` 追加，并同步 API Resource + Casbin
- 定时任务放 `worker`，cron 可配置开关（`off` 表示关闭）
- 敏感凭据（平台 token、账号 Cookie）**不入库明文、不打日志**

---

## 6. 依赖引入检查清单

新增任何第三方依赖前，逐条确认：

- [ ] SPDX 许可证经 **API 实测**（非 README 推断），且不在 §3.2 / §3.3 禁令清单
- [ ] 若是财经/市场类，NFR-19 四项复核全部通过
- [ ] 是否已加入 `NOTICE`（Attribution 义务）
- [ ] 上游活跃度可接受（末次 push 在 12 个月内；低于此须记录降级预案）
- [ ] 上游失效时的**回退路径**已设计（如 newsnow 挂了 → 直采各平台热榜）
- [ ] 引入形态明确：库依赖 / 独立服务 / 仅参考
- [ ] 若引入的是服务：部署形态、资源占用、升级方式已写进对应子系统的概要设计

---

## 7. 诚实边界

1. **元数据会漂移**：★ 数与许可证为 2026-10-17 快照，引入前须复核。
2. **OpenBB 未核实**：主仓库名与许可证本轮未能确认，标 `[待核实]`；引入前**必须**复核。
3. **未实机验证**：多数深挖以 README + 源码契约为准，未在本机端到端跑通模型/发布/采集。
4. **TrendRadar 是能力最贴合却不可用的典型案例**：62,957★ 但 GPL-3.0。许可证审查必须**先于**能力评估，否则会做无用选型。

---

*修改本文等于修改项目的法律边界，须经架构 + 法务共同确认。*
