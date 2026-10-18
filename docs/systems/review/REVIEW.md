# 全媒体内容矩阵系统 · 文档深度评审报告

> **评审日期**：2026-10-17 · **评审范围**：`docs/systems/` 全部 24 份文档 + 上游 PRD + 仓库代码基线交叉核验
> **评审方式**：逐字段、逐算法、逐契约比对；对核心算法做数学推导验证（非仅文字审阅）
> **详细评审附录**：[`IC-深度评审.md`](./IC-深度评审.md)（26 阻塞 + 35 严重 + 17 契约违规 + 25 缺失）、[`TT-深度评审.md`](./TT-深度评审.md)（11 阻塞 + 25 严重 + 15 契约违规 + 19 算法歧义 + 42 缺失）

---

## 0. 评审结论（一句话）

**文档骨架完整、诚实声明质量高，但核心算法层与契约层未达到「可照着写代码」的标准。** 在进入开发前，必须先闭环下述阻塞级问题，否则会在 M1–M3 大量返工。**本报告所附的修复已全部落地到契约层与各系统核心算法（见 §6 修复清单）。**

### 阻塞级问题分布

| 系统 | 阻塞 | 严重 | 最致命的一条 |
|------|:---:|:---:|------------|
| ① IC | 26 | 35 | `IntelItem` proto 缺失 + 时区规则零覆盖 + `url_key` 唯一约束错误（会导致 TT 断粮） |
| ② TT | 11 | 25 | **velocity 与 residual 线性相关 ρ=1，三方法投票在数学上失效** + 聚类零算法 + 热度因子不落盘不可重现 |
| ③ MIA | 4 | 9 | proto 用 enum（违契约小写枚举）+ impact 乘积公式分数普遍过小且丢失方向性 |
| ④ MME | 2 | 6 | 设计最扎实（MVP 地基）；proto/契约对齐与时区问题同其余系统 |
| ⑤ MAP | 3 | 8 | 审计记录先于发布（状态一致性）+ 限流/熔断的并发原子性未定义 |

---

## 1. 契约层问题（`00-shared/01-data-contract.md`）—— 最高优先级

这是五个系统的共同语言，问题会级联到所有子系统。**已全部修复**（见 §6.1）。

| # | 问题 | 影响 |
|---|------|------|
| C1 | **时区自相矛盾**：§0.2 规定「存储一律 UTC 带 Z」，但 §1/§2/§7 的示例全部写成 `+08:00` | 照抄示例即违约，跨市场对齐失效 |
| C2 | `IntelItem.dedup.cluster_id` 示例写 `tp_01H...`，与 §0.3 修正后的 `cl_` 前缀冲突 | TT 话题与 IC 去重簇 ID 命名空间污染 |
| C3 | **`PerformanceSnapshot` 未登记**：§8 NATS 表引用它作为 `mm.map.feedback.v1` 载荷，但 §1–§7 七契约里没有它，§0.3 也无前缀 | MAP 回流契约无定义，TT/IC 无法消费 |
| C4 | **缺幂等键**：所有 NATS 事件消息无 `event_id`/`dedup_key`，而 JetStream 是至少一次投递 | 重复投递必然产生重复条目 |
| C5 | **缺链路追踪**：无 `trace_id`/`correlation_id`，与 `pkg/observability` 脱节 | 跨系统排障无抓手 |
| C6 | ID 前缀表未登记 `src_`(IntelSource)、`ts_`(TrendSubscription)、`run_`、`acc_`、`ps_` 等 | 前缀冲突风险 |
| C7 | `source.platform`/`region` 取值域未定义；IC/TT/MAP 各自表述 | 跨系统平台标识不一致 |
| C8 | `series` 数组「≤200 点」未定义如何截取（多平台下可能被单平台占满） | 跨平台对比失效 |
| C9 | `IntelItem` 缺 `published_at`（上游发布时间）→ NFR-IC-02 延迟指标不可度量 | 时延 SLO 无法计算 |

---

## 2. ① IC 情报收集 —— 阻塞级精选（26 条全录于附录）

按「必须先修」排序：

1. **【B-1】`IntelItem` proto 完全没定义**，只写「与契约一一对应」。proto-first 基线下没有 message 定义 = 无法生成、无法映射、无法校验。→ 已补完整 message 定义。
2. **【B-15】时区规则四份文档零覆盖**。IC 是时间生产方，不写 = 全系统时区漂移。→ 强制 UTC + timestamptz + RFC3339 带 Z。
3. **【B-6】`unique(url_key, collected_at_date)` 按天唯一**：热榜 30 分钟一轮，同 URL 一天 48 次，按天唯一会导致只有第一条入库，rank 时序全丢 → **② TT 直接断粮**。→ 改为 `dedup_key = sha256(url_key|source_id|cycle_bucket)`。
4. **【B-8】证据快照写入时 `intel_id` 尚未生成** → 取证链从根上断裂。→ 调整顺序：先生成 `intel_id`（ULID 本地生成），再写证据。
5. **【B-9】SimHash 只有 5 行步骤**，哈希函数/权重公式/边界处理全缺，不同人写出来指纹不可比。→ 补完整可执行规格。
6. **【B-17】防篡改只写 SHA-256**——只能校验完整性不能防篡改。→ 升级为哈希链 `prev_hash`/`chain_hash` + 只追加存储。
7. **【B-16】合规闸门 `Evaluate` 无 error 返回** → 规则加载失败时 fail-open，违规内容直通下游，违反「100% 经闸门、无旁路」。→ 改 fail-closed。
8. **【B-26】`fingerprint uint64` 在 PostgreSQL 无法安全存储**（无无符号整型，最高位 50% 概率为 1 → 一半记录写入失败）。→ 拆 4 段 smallint + hex 字符串。

---

## 3. ② TT 热点追踪 —— 阻塞级精选（11 条全录于附录）

**这是评审中发现的最严重的算法缺陷，有数学证明：**

### 【B-5】三方法投票在数学上失效（★ 最严重）

文档定义：`residual(t) = heat(t) - h_ewma(t)`、`velocity(t) = (h_ewma(t) - h_ewma(t-1))/Δt`。推导：

```
h_ewma(t)  = α·heat(t) + (1-α)·h_ewma(t-1)     α=0.4
residual(t) = (1-α)·X = 0.6·X        （X = heat(t) - h_ewma(t-1)）
velocity(t) = α·X/Δt = 0.8·X          （Δt=0.5h）
⟹ residual(t) = 0.75 × velocity(t)    —— 确定性线性关系，相关系数 ρ=1
```

**即：方法一（zscore 残差）与方法二（velocity）是同一个统计量的两个阈值，方法二命中者方法一必命中，「≥2 方法投票」实际等于单方法双阈值，投票机制完全失效。** 修复：**通道化投票**——heat 通道（zscore+velocity 合计最多 1 票）必须再搭配 rank 通道（rank_jump）或 engagement 通道（新增 eng_spike）的票。

### 其余阻塞级

- **【B-1】话题聚类零算法**：FR-TRK-03 是 P0、10 人日、验收「归并正确率≥85%」，但对聚类的全部描述只有一句话。→ 补六步可编码方案（硬键→倒排+SimHash 召回→三路加权打分→三段阈值→merge_lock→label 重选）。
- **【B-3/B-4】热度四因子不落盘且不可重现**：`freshness` 用 `now` 而非样本时间，`eng_pct` 用当前滚动窗口 → 历史点今天重算和昨天不一样 → 击穿热点档案与回流校正。→ 全量改样本时间口径 + 因子取值与生效权重全部落盘。
- **【B-6】状态机有洞**：五条判据不覆盖状态空间（`heat=50, velocity=+2, n=5` 时无匹配阶段），新话题初始态未定义。→ 有序求值 + default 保持 + 新增 dormant。
- **【B-7】min_dwell 单位错配**：定义为「采样点数」，检查式却用墙钟时长；30 分钟档与 2 分钟档防抖强度差 15 倍。→ 统一为点数计数。

---

## 4. ③ MIA 市场影响分析 —— 核心发现

MIA 核心算法（实体链接/情感/事件聚类/影响面评分/传导/校验）已写到接口签名 + 公式级别，质量高于 IC/TT。但仍有问题：

### 阻塞级

1. **proto 用 `enum Direction`**：protojson 序列化为 `DIRECTION_BULLISH`，违反契约「枚举一律小写下划线字符串」。`bullish/bearish` 无法用 proto enum 表达。→ 契约字段一律 `string` + validate `in:`。
2. **impact 乘积公式分数普遍过小**：`100 × |sentiment| × source_weight × relevance × decay`，四个都 <1 的数相乘，典型值 0.7×0.55×0.7×0.6 ≈ 16 分，排行榜区分度差。且 `|sentiment|` 丢失方向性（极好/极坏同分）。→ 见 §6.4 修复。
3. **FinBERT 仅英文，中文财经文本无方案**：需求明确含中文时政/财经。→ 引擎选择已分语言（zh→LLM），但需在契约强制 `sentiment.engine` 标注且**禁止跨引擎比分**。
4. **「影响面分析」本质上是对弱信号做预测**：有效市场假说下消息→价格的边际信息有限。文档需诚实声明预期准确率上限，并设计防过度依赖机制（强制免责声明已做，但需补「样本量 <30 不具统计意义」的运行时门禁）。

### 金融方法论缺陷（需在详细设计补强）

- 实体→标的映射表的**数据来源与维护流程**未定义（谁维护？映射错误如何发现？）。
- `verified.hit` 的事后校验缺基准定义（相对什么？多久后？）与样本偏差控制。

---

## 5. ④ MME / ⑤ MAP —— 核心发现

### MME（设计最扎实，MVP 地基）

矩阵展开（笛卡尔积+规模保护 60）、缓存 Key（含 prompt_version+intent 哈希）、四级差异化检查（视觉/Hook/文案/综合）、QC 硬门禁、FFmpeg 命令基线均已到可编码程度。**诚实标注到位**（无 GPU 只能规则裁切，不是智能主体追踪）。问题：

- proto/契约对齐、枚举小写、时区问题与其余系统相同。
- `copy.<platform>` 固定 4 平台键，限制了 MAP 的 12 平台扩展 → 改 `map<string, PlatformCopy>`。
- 差异化检查的 pHash/音频指纹依赖外部库，需登记许可。

### MAP（审批门设计到位）

三条铁律（无 approve 不得 publish / 禁自批 / 默认私密）+ 应用层校验与数据库约束**双保险**，凭据安全（vault + 内存清零 + 不进契约）、熔断（账号级+平台级）设计完整。问题：

1. **审计一致性问题**：`Gate.Publish` 的 `approvalRepo.Record(Action:"publish")` 先于实际发布执行，若发布失败，审计记录显示 published 但实际未发布。→ 审计应记「意图」与「结果」两条，或在发布完成后补 `PublishReceipt` 关联。
2. **限流/熔断的并发原子性未定义**：`ConsecutiveFailures++` 是读-改-写，多实例/多协程下竞态。→ 用 Redis Lua 或 DB 行锁。
3. **proto 枚举/时区问题**同上。
4. **速率治理的「拟人化 ±3 分钟偏移」**与排期精确性冲突需明确优先级。

---

## 6. 修复清单（本次评审后已落地的修改）

### 6.1 契约层（`00-shared/01-data-contract.md`）—— 已修复

- [x] C1：三处 `+08:00` 示例改为 UTC 带 `Z`
- [x] C2：`cluster_id` 示例 `tp_` → `cl_`
- [x] C3：补登 `PerformanceSnapshot` 为第八份契约（§8）+ 前缀 `ps_`
- [x] C4：补「事件信封 `EventEnvelope`」——`event_id`（幂等键）+ `trace_id` + `occurred_at` + `producer`，所有 NATS 消息强制包裹
- [x] C6：前缀表补登 `src_`/`ts_`/`run_`/`acc_`/`ps_`/`rule_`/`task_`/`job_`
- [x] C7：补「平台标识枚举表」（IC/TT/MAP 共用）+ `region ∈ {cn, intl, global}`
- [x] C8：`series` 截取规则（平台均分 + 全局降采样两段式）
- [x] C9：`IntelItem.source` 补 `published_at`（可空）
- [x] 枚举跨系统传输一律小写字符串（禁用 proto enum 承载契约字段）写入 §0.2

### 6.2 ② TT 核心算法 —— 已修复（落地为代码 + 文档回写）

- [x] B-5：突发检测改**通道化投票**（heat/rank/engagement 三通道，≥2 票且来自 ≥2 通道）
- [x] B-4：热度因子改**样本时间口径**（freshness/eng_pct/breadth 均为 t 的函数，禁 now()）
- [x] B-3：四因子取值 + 生效权重 + missing 全部落盘
- [x] B-6：生命周期改**有序求值 + default 保持 + dormant**
- [x] B-7：min_dwell 统一为**点数计数**
- [x] A-6：因子缺失按有效权重重新归一化，全缺失 → heat=null 强制 latent
- [x] A-9：zscore 改 MAD 稳健 σ + 窗口不含当前点 + 单尾
- [x] A-11：rank_jump 改相对阈值 `max(3, ceil(0.2×rank_total))`，缺失记 unavailable

### 6.3 ① IC 核心算法 —— 已修复（落地为代码 + 文档回写）

- [x] B-9：SimHash 完整规格（NFKC 归一化 + CJK 2-gram + tf-idf + FNV-1a + 4 段 smallint 存储）
- [x] B-6：`dedup_key = sha256(url_key|source_id|cycle_bucket)` 幂等键
- [x] B-17：证据哈希链 `prev_hash`/`chain_hash`
- [x] B-15：时区纪律（全部 UTC）

### 6.4 ③ MIA 核心算法 —— 已修复（落地为代码）

- [x] 影响面评分保留方向性（direction 独立于 score）+ 分数分布校准说明
- [x] 传导规则 confidence ≤0.6 强制校验
- [x] 情感引擎标注 + 禁止跨引擎比分

### 6.5 ⑤ MAP 核心算法 —— 已修复（落地为代码）

- [x] 审批门状态机（draft→approved/rejected→published）+ 三条铁律 + 双保险
- [x] 熔断（账号级 + 平台级）
- [x] 审计「意图 + 结果」双记录

---

## 7. 不修复 / 推迟项（诚实声明）

| 项 | 原因 |
|----|------|
| newsnow/RSSHub/Crawl4AI 自部署 | 运维任务，M0 前置，非代码 |
| Redis Stack VSS / pgvector 实测 | 需运维核实，已标 `[待核实]` |
| 全量 proto/Ent/Worker/前端 5 子系统实施 | 100+ 人日工程，本次落地**契约层 + 核心算法包**地基（可编译可测试），详见实施报告 |
| Q1/Q4/Q6 待拍板问题 | 需用户决策，不代为拍板 |

---

*评审完成于 2026-10-17。详细问题清单见附录。修复后的代码见 `apps/server/pkg/mediacontract/` 与 `apps/server/app/admin/internal/algo/`。*
