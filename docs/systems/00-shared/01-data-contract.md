# 00 · 共享数据契约（跨五子系统的唯一衔接语言）

> **版本**：v1.0 · **日期**：2026-10-17
> **上游**：PRD §8（统一数据契约）
> **地位**：**强制**。五子系统之间**只允许**通过本文定义的七份结构交换数据；任何跨系统调用若出现本文之外的字段，视为契约违规，必须在本文登记后再实现。

---

## 0. 总则

### 0.1 为什么需要这一层

PRD §8 的定位是「贯穿五子系统」。没有它，IC 的输出直接被 TT 消费，任何字段改名都会造成跨子系统级联返工。本文把这七份结构固化为**版本化契约**，并规定它们与 Go 结构体、Ent schema、Proto message 的映射规则。

### 0.2 通用规则

| 规则 | 约定 |
|------|------|
| **版本字段** | 每份契约顶层必须带 `api_version`，当前统一 `"1.0"` |
| **ID 规范** | `{前缀}_{ULID}`。ULID 26 字符，字典序≈时间序，可排序、可本地生成、无中心化分配 |
| **时区** | **存储一律 UTC（RFC3339 带 `Z`）；展示层转 Asia/Shanghai**。财经跨市场对齐的硬要求 |
| **枚举** | 一律小写下划线字符串（`rising`、`bullish`），**禁止**用整型魔法数字跨系统传。**proto 侧禁用 enum 承载契约字段**（protojson 会序列化成 `DIRECTION_BULLISH`），契约枚举字段一律 `string` + 校验 `in:[...]` |
| **幂等** | 所有 NATS 事件消息必须包裹 `EventEnvelope`（§8.1），携带全局唯一 `event_id` 作幂等键；消费者必须按 `event_id` 幂等去重 |
| **追踪** | 所有事件消息携带 `trace_id`，贯穿 IC→TT→MIA/MME→MAP 全链路，接入 `pkg/observability` |
| **空值** | 可选字段允许 `null`；`0` 与 `null` **语义不同**（如 `rank_total=0` 表示未知，不表示榜长为 0） |
| **扩展** | 禁止在契约里塞任意 `map[string]any`；确需扩展走 `extensions` 显式字段并登记 |
| **不可变** | 已发布字段**只增不改语义**；语义变更必须升 `api_version` |

### 0.3 ID 前缀分配表

| 前缀 | 全称 | 归属 | 示例 |
|------|------|------|------|
| `it_` | IntelItem | ① IC | `it_01J9Z...` |
| `cl_` | Cluster（去重簇） | ① IC | `cl_01J9Z...` |
| `ev_` | EvidenceSnapshot | ① IC | `ev_01J9Z...` |
| `tp_` | Topic（话题） | ② TT | `tp_01J9Z...` |
| `tc_` | TopicCandidate（选题） | ② TT | `tc_01J9Z...` |
| `al_` | Alert（告警） | ② TT | `al_01J9Z...` |
| `im_` | ImpactCard | ③ MIA | `im_01J9Z...` |
| `ec_` | EventCard（事件卡） | ③ MIA | `ec_01J9Z...` |
| `as_` | Asset（素材） | ④ MME | `as_01J9Z...` |
| `vs_` | VariantSet | ④ MME | `vs_01J9Z...` |
| `v_` | Variant | ④ MME | `v_01J9Z...` |
| `du_` | DispatchUnit | ⑤ MAP | `du_01J9Z...` |
| `pa_` | PublishAccount | ⑤ MAP | `pa_01J9Z...` |
| `pr_` | PublishReceipt | ⑤ MAP | `pr_01J9Z...` |
| `ps_` | PerformanceSnapshot | ⑤ MAP | `ps_01J9Z...` |
| `src_` | IntelSource | ① IC（内部实体，不进契约） | `src_01J9Z...` |
| `ts_` | TrendSubscription | ② TT（内部实体） | `ts_01J9Z...` |
| `rule_` | TransmissionRule / ComplianceRule | ③ MIA / ① IC | `rule_01J9Z...` |
| `task_` | MontageTask | ④ MME | `task_01J9Z...` |
| `job_` | RenderJob | ④ MME | `job_01J9Z...` |
| `run_` | CollectRun（采集运行记录） | ① IC（内部实体） | `run_01J9Z...` |
| `acc_` | CredentialAccount | ① IC / ⑤ MAP（内部实体） | `acc_01J9Z...` |

> **跨系统事件统一前缀**：`EventEnvelope.event_id` 用 `evt_` 前缀（见 §8.1），与业务实体 ID 区分。

### 0.4 契约与代码的映射规则

| 契约 | Proto message | Ent schema | 存储 |
|------|--------------|-----------|------|
| `IntelItem` | `media.v1.ic.IntelItem` | `IntelItem` | PostgreSQL（`intel_items`） |
| `TrendSeries` | `media.v1.tt.TrendSeries` | 元数据 `Topic` + 时序 `topic_heat` | PG（元数据）+ **InfluxDB 2.x**（时序） |
| `ImpactCard` | `media.v1.mia.ImpactCard` | `ImpactCard` | PostgreSQL（`impact_cards`） |
| `VariantSet` | `media.v1.mme.VariantSet` | `VariantSet` + `Variant` | PG（元数据）+ 对象存储（媒体文件） |
| `DispatchUnit` | `media.v1.map.DispatchUnit` | `DispatchUnit` | PostgreSQL |
| `TopicCandidate` | `media.v1.tt.TopicCandidate` | `TopicCandidate` | PostgreSQL |
| `Alert` | `media.v1.tt.Alert` | `Alert` | PostgreSQL |

> **规则**：契约是**外部交换视图**（跨系统/跨进程/落 NATS）；Ent schema 是**内部持久化视图**（可含索引、外键、审计字段）。二者字段名一致，但 Ent 允许额外内部字段（如 `created_by`、`tenant_id`），这些字段**不进**契约。

---

## 1. `IntelItem` —— ① IC 的输出

**生产者** ① IC 归一化层 · **消费者** ② TT（全量）、③ MIA（仅 `category=C_market`）、④ MME（仅 `category=B_material`）

```json
{
  "api_version": "1.0",
  "intel_id": "it_01H...",
  "source": {
    "platform": "weibo",
    "region": "cn",
    "collect_method": "hotlist | api | rss | crawl | sidecar",
    "url": "https://...",
    "author": "...",
    "collected_at": "2026-10-17T01:00:00Z",
    "published_at": "2026-10-17T00:58:00Z",
    "evidence": { "snapshot_path": "...", "raw_ref": "...", "http_headers": {} }
  },
  "payload": { "title": "...", "text": "...", "media_refs": ["asset://..."], "lang": "zh-CN" },
  "entities": [{ "type": "person|org|policy|tech|place|topic", "value": "...", "confidence": 0.0 }],
  "category": "A_hot | B_material | C_market | D_compliance",
  "metrics": { "rank": 3, "rank_total": 50, "views": 0, "likes": 0, "comments": 0, "_as_of": "2026-10-17T01:00:00Z" },
  "dedup": { "url_key": "...", "fingerprint": "...", "cluster_id": "cl_01H..." },
  "compliance": { "gate": "pass|review|block", "reasons": [] },
  "workability": { "score": 0.0, "reason": "..." }
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `source.collected_at` | **本地接收时间（UTC 带 Z）**，必填。由采集端 `clock.Now().UTC()` 生成，禁止用上游时间兜底 |
| `source.published_at` | **上游发布时间（UTC 带 Z）**，可空。用于计算 NFR-IC-02 时延（`collected_at - published_at`）；缺失或为未来时间时不计该指标，不伪造 |
| `source.collect_method` | 五路采集器之一；决定该条目的**取证强度与可信度权重**，下游必须可见 |
| `source.evidence` | 取证链指针。**不允许**把原始 HTML 塞进消息体，只存路径/引用 |
| `payload.media_refs` | `asset://` 指向 MME 素材库；`https://` 为外链。二者**必须区分**（前者可加工，后者须先下载） |
| `entities[].type` | 六类枚举；`confidence` ∈ [0,1]，低于阈值（默认 0.6）的实体**不进** MIA 实体链接 |
| `category` | **决定下游路由**，见下表 |
| `metrics._as_of` | 指标快照时间。互动数是**时变**的，下游必须知道这个数是「什么时候的数」 |
| `dedup.cluster_id` | 前缀在初版 PRD 中写作 `tp_`，**本文统一修正为 `cl_`**（`tp_` 归 TT 话题），见 §0.3 |
| `compliance.gate` | `block` 条目**不得**进入 MME/MIA 自动流程，只能进人工队列 |

**`source.platform` / `source.region` 取值域（IC/TT/MAP 共用，禁止各自造词）**：

| 字段 | 合法值 |
|------|--------|
| `platform` | `weibo` `zhihu` `baidu` `toutiao` `douyin` `bilibili` `xiaohongshu` `kuaishou` `weixin_channels` `youtube` `youtube_shorts` `reddit` `x` `tiktok` `facebook` `instagram` `telegram` `hackernews`（新增须先登记本表） |
| `region` | `cn`（国内）· `intl`（国外单一市场）· `global`（跨市场/全球话题） |

**category 路由表**：

| 值 | 含义 | 下游 |
|----|------|------|
| `A_hot` | 正在上升的话题/事件 | → ② TT |
| `B_material` | 可直接加工的长视频/文章/播客 | → ④ MME 建任务 |
| `C_market` | 可能影响市场的消息 | → ③ MIA |
| `D_compliance` | 平台规则/版权/政策信号 | → 人工 + 告警，**阻断自动流程** |

**Go 结构体**（`pkg/mediacontract/intel.go`，跨服务共享，非 ent 生成物）：

```go
type IntelItem struct {
    APIVersion  string       `json:"api_version"`
    IntelID     string       `json:"intel_id"`
    Source      IntelSource  `json:"source"`
    Payload     IntelPayload `json:"payload"`
    Entities    []Entity     `json:"entities"`
    Category    Category     `json:"category"`
    Metrics     Metrics      `json:"metrics"`
    Dedup       Dedup        `json:"dedup"`
    Compliance  Compliance   `json:"compliance"`
    Workability Workability  `json:"workability"`
}
```

---

## 2. `TrendSeries` —— ② TT 的核心

**生产者** ② TT · **消费者** ② TT 看板、④ MME（选题）、③ MIA（热度佐证）

```json
{
  "api_version": "1.0",
  "topic_id": "tp_01H...",
  "label": "...", "category": "...",
  "stage": "latent | rising | peak | decaying | long_tail",
  "stage_evidence": { "velocity": 0.0, "accel": 0.0, "duration_points": 0 },
  "heat": { "score": 0.0, "rank_pct": 0.0, "eng_rate": 0.0, "velocity": 0.0, "accel": 0.0,
            "breadth": { "platform_count": 0, "platforms": ["..."] },
            "factors": { "w_rank": 0.0, "w_eng": 0.0, "w_fresh": 0.0, "w_breadth": 0.0 } },
  "series": [{ "t": "2026-10-17T00:30:00Z", "platform": "weibo", "rank": 3, "heat": 0.0 }],
  "burst": { "is_burst": true, "methods_hit": ["zscore", "velocity"], "evidence": {} },
  "origin": { "first_seen_at": "...", "first_platform": "...", "first_url": "..." }
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `heat.score` | **相对可比分数** 0–100，**不是**绝对热度。UI 必须明示这一点 |
| `heat.factors` | 四个权重因子的**实际取值**。可解释性靠这个字段，缺失即视为违约 |
| `stage_evidence` | 生命周期判据的**数值**。缺数值的阶段标签不可信 |
| `series` | 时序点数组。**大量点不随契约传输**——契约只带窗口化摘要，完整时序查 InfluxDB |
| `burst.methods_hit` | 至少 2 个方法命中才为 `true`（PRD §4.5）。单方法命中**不得**置 `is_burst=true` |

> **传输约束**：`series` 数组在 NATS 消息中**不得超过 200 点**；完整时序通过 `GET /api/v1/media/tt/topics/{id}/series` 分页查询。

**`series` 200 点截取规则**（防止被单平台占满，保证跨平台可比）：

```
平台数 P，预算 B=200
第一段（窗口均分）：每平台分 ⌊200/P⌋ 点，取最近窗口；
  若导致窗口 < 6h（P 较大时），进入第二段
第二段（全局降采样）：取最近 7d，按等时间间隔抽样到恰好 200 点，
  跨平台交替取点；响应头带 X-Series-Downsampled: true
硬规则：同一消息内所有平台点数之差 ≤ 1
```

---

## 3. `ImpactCard` —— ③ MIA 的输出

**生产者** ③ MIA · **消费者** 简报、推送、人工决策。**默认不对外发布**（PRD §12 Q2）

```json
{
  "api_version": "1.0",
  "impact_id": "im_01H...",
  "topic_id": "tp_01H...",
  "event": { "title": "...", "summary": "...", "sources": ["it_01H..."], "occurred_at": "..." },
  "sentiment": { "label": "positive | neutral | negative", "score": 0.0,
                 "engine": "finbert | llm", "lang": "en" },
  "impact": { "score": 0.0, "ttl_hours": 12,
              "factors": { "sentiment": 0.0, "source_weight": 0.0, "relevance": 0.0, "decay": 0.0 } },
  "affected": [{
    "symbol": "SPY | ^GSPC | EURUSD | BTC/USDT",
    "asset_class": "equity | index | fx | commodity | crypto",
    "direction": "bullish | bearish | neutral",
    "confidence": 0.0,
    "path": ["人物表态", "→ 行业 ETF", "→ 避险资产"],
    "rule_id": "..."
  }],
  "disclaimer": "研究/监测辅助，非投资建议；模型输出须人工复核。",
  "verified": { "checked_at": null, "actual_move": null, "hit": null }
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `sentiment.engine` | 必须标明引擎（FinBERT / LLM），**不同引擎的分数不可混比** |
| `impact.ttl_hours` | 6–24 可配。过期卡**不得**继续占据排行榜 |
| `affected[].path` | 影响路径文本链。**每条结论必须有 path**，无 path 的 affected 项视为违约 |
| `affected[].confidence` | 跨资产传导是**假设不是因果**；置信度必显（PRD §5.5 FR-MIA-08） |
| `disclaimer` | **强制非空**。为空的 ImpactCard 不允许落库 |
| `verified` | 事后校验回填；`hit` 为 `null` 表示未校验，**不得**默认解释为「不准」 |

---

## 4. `VariantSet` —— ④ MME 的输出

**生产者** ④ MME · **消费者** ⑤ MAP

```json
{
  "api_version": "1.0",
  "variant_set_id": "vs_01H...",
  "origin": { "candidate_id": "tc_01H...", "asset_id": "as_01H...", "transcript_ref": "..." },
  "variants": [{
    "variant_id": "v_01H...",
    "aspect": "9:16", "duration_sec": 58,
    "content_version": "v1", "hook_id": "h1",
    "video_path": "...", "cover_path": "...", "edl_ref": "...",
    "captions": { "style": "simple | dynamic", "srt_path": "..." },
    "copy": {
      "douyin":         { "title": "...", "desc": "...", "tags": ["..."] },
      "xiaohongshu":    { "title": "...", "desc": "...", "tags": ["..."] },
      "bilibili":       { "title": "...", "desc": "...", "tags": ["..."] },
      "youtube_shorts": { "title": "...", "desc": "...", "tags": ["..."] }
    },
    "manifest_path": "...",
    "qc": { "boundary_ok": true, "subtitle_ok": true, "score": 0.0 }
  }],
  "diff_check": { "min_pair_similarity": 0.42, "warn": false }
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `origin.candidate_id` | 回指 ② TT 的选题，形成闭环。**允许为 null**（用户手动上传素材的场景） |
| `variants[].video_path` | 对象存储路径，**不是** HTTP URL。MAP 取件时再签临时 URL |
| `copy.<platform>` | 平台键命名与 ⑤ MAP 适配器注册表的 `platform_key` **必须一致** |
| `qc.boundary_ok` / `subtitle_ok` | 渲染自检结果。任一为 `false` 的变体**不得**进入审批门 |
| `diff_check.min_pair_similarity` | 变体簇内两两最小相似度。**过高**说明变体不够差异化（平台查重风险），需告警 |

---

## 5. `DispatchUnit` —— ⑤ MAP 的输入

**生产者** ⑤ MAP（由 VariantSet 派生）· **消费者** ⑤ MAP 调度/审批/发布

```json
{
  "api_version": "1.0",
  "unit_id": "du_01H...",
  "variant_set_id": "vs_01H...",
  "targets": ["youtube:main", "bilibili:main", "douyin:a1"],
  "scheduled_for": "2026-10-20T19:00:00Z",
  "visibility": { "default": "private", "override_reason": null },
  "approval": { "state": "draft | approved | rejected", "approver": null, "at": null },
  "publish": { "post_id": null, "url": null, "at": null, "operator": null },
  "feedback_ref": null
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `targets` | `平台:账号槽位` 列表。槽位指向 `PublishAccount`，**凭据不进契约** |
| `visibility.default` | **恒为 `private`**。改公开必须填 `override_reason` 且经审批（NFR-17） |
| `approval.state` | 状态机见 MAP 详细设计。`approved` **且** `approver != 提交者` 才允许 `publish` |
| `publish.operator` | 谁点的发布。自动化身份必须有独立标识，**不得**与提交者相同（禁自批） |

---

## 6. `TopicCandidate` —— ② TT → ④ MME / ③ MIA 的交接

**生产者** ② TT 选题评分 · **消费者** ④ MME、③ MIA

```json
{
  "api_version": "1.0",
  "candidate_id": "tc_01H...",
  "topic_id": "tp_01H...",
  "route": "mme | mia | both",
  "score": { "hot": 0.0, "freshness": 0.0, "workability": 0.0, "compliance": 0.0, "total": 0.0 },
  "suggested_angles": ["..."],
  "material_refs": ["asset://...", "https://..."],
  "state": "pending | adopted | rejected | snoozed",
  "reject_reason": null
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `route` | 决定走 MME（加工）、MIA（分析）还是两者。这是 PRD §1.2「不得混为同一按钮」的**技术实现点** |
| `score.total` | 加权总分。**仅用于排序**，不作为自动采纳依据——`state` 必须经人工或显式规则流转 |
| `material_refs` | 素材指针。若 Q1 拍板「不做切片搬运」，此处只保留**自有/授权/CC** 素材 |

---

## 7. `Alert` —— 突发告警

**生产者** ② TT 突发检测 · **消费者** 推送渠道、看板

```json
{
  "api_version": "1.0",
  "alert_id": "al_01H...",
  "topic_id": "tp_01H...",
  "triggered_at": "2026-10-17T01:20:00Z",
  "methods_hit": ["zscore", "rank_jump"],
  "evidence": { "zscore": 4.2, "velocity": 0.31, "rank_jump": 22 },
  "channels": ["webhook", "inapp"],
  "state": "new | ack | muted"
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `methods_hit` | 命中的检测方法名，≥2 个才产生 Alert（PRD §4.5） |
| `evidence` | 每个命中方法的**具体数值**。无数值的告警不可解释，不允许发出 |
| `state=muted` | 人工静音后**不再重复推送同一 topic**，但仍在看板显示（避免信息黑洞） |

---

## 8. NATS 主题与载荷对照

### 8.1 `EventEnvelope` —— 所有事件消息的强制信封（幂等 + 追踪）

**所有**经 NATS 传输的契约消息**必须**包裹在 `EventEnvelope` 中，不得裸传业务载荷。这是幂等与链路追踪的唯一载体。

```json
{
  "event_id": "evt_01J9Z...",
  "trace_id": "01J9Z...",
  "producer": "ic | tt | mia | mme | map",
  "occurred_at": "2026-10-17T01:00:00Z",
  "subject": "mm.intel.normalized.v1",
  "payload": { "...": "七份契约之一（含 api_version）" }
}
```

| 字段 | 约束 |
|------|------|
| `event_id` | `evt_` + ULID，**全局唯一**，是幂等键。消费者用 `SET NX event_id` 去重；NATS 侧同时以它为 `Nats-Msg-Id`，配合 Stream `duplicate_window` 服务端去重 |
| `trace_id` | 贯穿一次业务链路（如「一条情报从采集到发布」）；跨系统传递时**透传不重建**；接入 `pkg/observability` 做链路串联 |
| `producer` | 生产者子系统标识，用于路由与审计 |
| `occurred_at` | 事件发生时间（UTC 带 Z），**不是**投递时间 |

### 8.2 主题对照表

| Subject | 载荷契约 | 生产者 | 消费者 | 持久化 |
|---------|---------|-------|-------|--------|
| `mm.intel.raw` | 原始片段（内部结构，非七契约之一） | IC 采集器 | IC 归一化 | Stream `MM_INTEL` |
| `mm.intel.normalized.v1` | `IntelItem` | IC 归一化 | TT / MIA / MME | Stream `MM_INTEL` |
| `mm.intel.blocked.v1` | `IntelItem`（`gate=block`） | IC 合规闸门 | 人工队列 | Stream `MM_INTEL` |
| `mm.trend.snapshot.v1` | 热度快照（内部结构） | TT 采集 | TT 时序写入 | Stream `MM_TREND` |
| `mm.trend.burst.v1` | `Alert` | TT 突发检测 | 推送服务 | Stream `MM_TREND` |
| `mm.trend.candidate.v1` | `TopicCandidate` | TT 选题评分 | MME / MIA | Stream `MM_TREND` |
| `mm.impact.card.v1` | `ImpactCard` | MIA 评分 | 简报 / 推送 | Stream `MM_IMPACT` |
| `mm.mme.variant.ready.v1` | `VariantSet` | MME 渲染 | MAP | Stream `MM_MEDIA` |
| `mm.map.dispatch.v1` | `DispatchUnit` | MAP 编排 | MAP 调度 | Stream `MM_PUB` |
| `mm.map.feedback.v1` | `PerformanceSnapshot` | MAP 回流 | TT / IC | Stream `MM_PUB` |

### 8.3 `PerformanceSnapshot` —— ⑤ MAP 表现回流契约（第八份契约）

**生产者** ⑤ MAP 数据回流 · **消费者** ② TT（选题反馈）、① IC（信源选题权重）、④ MME（变体效果）

```json
{
  "api_version": "1.0",
  "snapshot_id": "ps_01H...",
  "dispatch_unit_id": "du_01H...",
  "variant_id": "v_01H...",
  "platform": "douyin",
  "account_slot": "main",
  "post_url": "https://...",
  "metrics": { "views": 0, "likes": 0, "comments": 0, "shares": 0, "favorites": 0 },
  "captured_at": "2026-10-17T01:00:00Z",
  "lag_hours": 24
}
```

| 字段块 | 说明与约束 |
|--------|-----------|
| `dispatch_unit_id` / `variant_id` | 回指 MAP 发布单元与 MME 变体，形成「发布 → 表现 → 反哺选题」闭环 |
| `metrics` | 平台回流的真实表现数。`shares`/`favorites` 在本契约补登（V1 `IntelItem.metrics` 暂无，列入 V1.1） |
| `captured_at` | 抓取该快照的时间（UTC 带 Z）；表现数是时变的，必须带时间 |
| `lag_hours` | 发布后多久抓的（平台 analytics 通常 T+24h 才出数），下游据此判断数据成熟度 |

**命名规则**：`mm.<子系统>.<事件>.v<契约主版本>`。版本在 subject 里，便于灰度切换与并存消费。

**JetStream 配置基线**：

| Stream | Subjects | 保留策略 | 最大时长 | 副本 |
|--------|----------|---------|---------|------|
| `MM_INTEL` | `mm.intel.>` | limits（按条数） | 90d | 1（单机） |
| `MM_TREND` | `mm.trend.>` | limits | 30d | 1 |
| `MM_IMPACT` | `mm.impact.>` | limits | 30d | 1 |
| `MM_MEDIA` | `mm.mme.>` | workQueue（消费即删） | — | 1 |
| `MM_PUB` | `mm.map.>` | limits | 90d（审计需要） | 1 |

---

## 9. 版本演进规则

1. **补丁**（加可选字段）：`api_version` 不变，消费者必须容忍未知字段。
2. **次版本**（加必填字段 / 改语义）：升 `api_version` 至 `1.1`，同时发布新 subject（`...v1_1`），旧版本并行 ≥1 个迭代周期。
3. **主版本**（结构重构）：升 `2.0`，走双写 + 影子消费验证 ≥2 周。
4. **禁止**：直接修改已发布字段的语义而不升版本——这是跨系统级联故障的头号来源。

---

## 10. 契约测试（强制）

每个子系统提测时必须提供：

| 测试 | 内容 |
|------|------|
| **生产者契约测试** | 用 JSON Schema 校验产出的每条消息，**未知字段与缺失必填字段都失败** |
| **消费者兼容测试** | 注入含未知字段的旧版消息，消费者必须正常处理（向后兼容） |
| **往返测试** | 契约 → Proto → 契约，字段无损（在 `pkg/mediacontract` 下维护） |

JSON Schema 文件位置：`docs/systems/00-shared/schema/*.json`（随实现补充）。

---

*本文是五子系统的共同语言。修改本文等价于修改五个系统的接口，必须走评审。*
