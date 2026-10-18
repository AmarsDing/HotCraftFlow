评审完毕。以下只输出问题与可执行修改建议。

---

## 一、阻塞级问题（会导致返工或写不出代码）

**[B-1] 03-详细设计.md:110-111 | `IntelItem` 这个最核心的 proto message 根本没有定义，只写了一句"字段与契约 §1 一一对应"** | 这是 IC 的唯一对外产物，proto-first 基线下没有 message 定义 = `buf generate` 无产物 = 后续 Ent 映射、NATS 序列化、TS 客户端、JSON Schema 全部无法开工；且"一一对应"无法约束 `json_name`，必然产出 camelCase 违约 | 在 `intel.proto` 增加如下完整定义（可直接替换"同构/一一对应"那句话）：

```proto
syntax = "proto3";
package media.v1.ic;
import "google/protobuf/timestamp.proto";
import "validate/validate.proto";

message IntelItem {
  string api_version = 1 [json_name = "api_version"];
  string intel_id    = 2 [json_name = "intel_id"];
  IntelItemSource source      = 3 [json_name = "source"];
  IntelPayload    payload     = 4 [json_name = "payload"];
  repeated IntelEntity entities = 5 [json_name = "entities"];
  string category   = 6 [json_name = "category",
    (validate.rules).string.pattern = "^(A_hot|B_material|C_market|D_compliance)$"];
  IntelMetrics     metrics     = 7 [json_name = "metrics"];
  IntelDedup       dedup       = 8 [json_name = "dedup"];
  IntelCompliance  compliance  = 9 [json_name = "compliance"];
  IntelWorkability workability = 10 [json_name = "workability"];
}
message IntelItemSource {
  string platform = 1 [json_name = "platform"];
  string region   = 2 [json_name = "region"];
  string collect_method = 3 [json_name = "collect_method",
    (validate.rules).string = {in: ["hotlist","api","rss","crawl","sidecar"]}];
  string url    = 4 [json_name = "url"];
  string author = 5 [json_name = "author"];
  google.protobuf.Timestamp collected_at = 6 [json_name = "collected_at"];
  google.protobuf.Timestamp published_at = 7 [json_name = "published_at"]; // 新增，须回写契约 §1
  IntelEvidence evidence = 8 [json_name = "evidence"];
}
message IntelEvidence {   // 契约 §1 的三个键，禁止再增
  string snapshot_path = 1 [json_name = "snapshot_path"];
  string raw_ref       = 2 [json_name = "raw_ref"];
  map<string,string> http_headers = 3 [json_name = "http_headers"];
}
message IntelPayload {
  string title = 1 [json_name = "title"];
  string text  = 2 [json_name = "text"];
  repeated string media_refs = 3 [json_name = "media_refs"];
  string lang = 4 [json_name = "lang"];
}
message IntelEntity {
  string type = 1 [json_name = "type",
    (validate.rules).string = {in: ["person","org","policy","tech","place","topic"]}];
  string value = 2 [json_name = "value"];
  double confidence = 3 [json_name = "confidence"];
}
message IntelMetrics {
  int32 rank = 1 [json_name = "rank"];            // 无榜 = 0
  int32 rank_total = 2 [json_name = "rank_total"];// 无榜 = 0（契约 §0.2）
  int64 views = 3 [json_name = "views"];
  int64 likes = 4 [json_name = "likes"];
  int64 comments = 5 [json_name = "comments"];
  google.protobuf.Timestamp as_of = 6 [json_name = "_as_of"];
}
message IntelDedup {
  string url_key     = 1 [json_name = "url_key"];
  string fingerprint = 2 [json_name = "fingerprint"]; // 16 位小写 hex，非数字
  string cluster_id  = 3 [json_name = "cluster_id"];
}
message IntelCompliance {
  string gate = 1 [json_name = "gate", (validate.rules).string = {in: ["pass","review","block"]}];
  repeated string reasons = 2 [json_name = "reasons"];
}
message IntelWorkability {
  double score  = 1 [json_name = "score"];
  string reason = 2 [json_name = "reason"];
}
```

**[B-2] 03-详细设计.md:14-108 | 给出的 `source.proto` 片段**不可编译**：① `DeleteIntelSourceRequest`/`DeleteIntelSourceResponse` 未定义却被引用；② `FetchURL` 返回 `IntelItem` 但未 `import "media/v1/ic/intel.proto"`；③ `import "validate/validate.proto"` 在本仓 `api/buf.yaml` 的 `deps` 中**不存在**（实测只有 `googleapis / kratos/apis / gnostic / tx7do/pagination`），`buf generate` 直接报 import not found；④ 全仓现有 40+ 个 proto **没有任何一个** import 过 validate | 开发第一步 `buf generate` 就失败，且 validate 规则无法生效 | 二选一并写死：(a) 保留 validate → 必须在 `api/buf.yaml` 增加 `deps: - 'buf.build/envoyproxy/protoc-gen-validate'` 并执行 `buf dep update` 生成 `buf.lock`，同时在 `buf.gen.yaml` 的 `managed.disable` 中保留该 module（已有）；(b) 去掉 validate，改用 Kratos `middleware/validate`（go.mod 已有 `kratos/contrib/middleware/validate/v2`）在 service 层校验。另补齐缺失 message：

```proto
import "media/v1/ic/intel.proto";
message DeleteIntelSourceRequest  { string id = 1; }
message DeleteIntelSourceResponse { bool ok = 1; }
```

**[B-3] 03-详细设计.md:24-41、51 | `SourceKind`/`CollectMethod` 用 proto enum，protojson 默认序列化为 `COLLECT_METHOD_HOTLIST`，而契约 §0.2 强制"枚举一律小写下划线字符串"；且契约值 `A_hot`、`pass` 根本无法用 proto enum 表达（buf lint DEFAULT 要求枚举值形如 `CATEGORY_A_HOT`）** | 直接产出契约违约消息，验收第 7 条"100% 通过 JSON Schema 校验"必然失败，且是跨系统级联故障源 | **禁止**用 enum 承载契约字段。改为 `string` + validate `in:`（见 B-1 的 `collect_method` / `category` / `gate` / `entity.type` 写法）。`SourceKind`（信源种类，非契约字段）可保留 enum，但必须显式说明"仅内部管理面使用，不出现在 `IntelItem` 中"。

**[B-4] 03-详细设计.md:111 | 只说"字段名与 JSON tag 必须一致"，但没给实现机制。Kratos HTTP 默认走 `encoding/json`，buf 生成物**禁手改**，无法加 `json` tag，默认生成的是 `json:"apiVersion,omitempty"` | 这是最容易踩且最难排查的坑：REST 响应与 NATS 载荷都会变成 camelCase，全部违约 | 两条都写死：① 所有契约字段在 proto 中显式写 `json_name = "<snake_case>"`（protoc-gen-go 会把它写进生成物的 struct tag，`encoding/json` 与 `protojson` 同时生效）；② NATS 侧统一封装 `pkg/mediacontract` 的编解码：

```go
var IntelItemCodec = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: false}
// 发布：b, _ := IntelItemCodec.Marshal(item)
// 消费：IntelItemCodec.Unmarshal(b, &item)
```
并在契约测试中断言：`json.Marshal(item)` 与 `protojson` 两种路径产出的 key 集合完全相同。

**[B-5] 03-详细设计.md:141-165 | Ent `IntelItem` 与契约之间**没有任何映射定义**，且字段名不一致（`id` vs `intel_id`）、`api_version` 缺失、`published_at` 缺失、`rank_total` 缺失、`fingerprint` 类型冲突 | "契约是外部交换视图、Ent 是内部持久化视图"（契约 §0.4）二者的转换函数是全系统最容易出 bug 的地方，不写就等于让每个开发各写各的 | 在 `03-详细设计.md` §2.2 后追加"Ent → 契约映射表"（逐字段，含 `id → intel_id`、`platform/region/collect_method/url/author/collected_at → source.*`、`title/text/media_refs/lang → payload.*`、`url_key/fingerprint/cluster_id → dedup.*`、`gate/gate_reasons → compliance.*`、`workability_score/reason → workability.*`、`evidence_id → 联查 EvidenceSnapshot 生成 source.evidence`），并强制实现：

```go
// data/ic/intel_repo.go
func toContract(e *ent.IntelItem, ev *ent.EvidenceSnapshot) (*contract.IntelItem, error)
// 反面：禁止在 biz 层手工拼装 map 再序列化
```
同时补字段：`api_version string // 默认 "1.0" 落库`、`published_at time // 上游发布时间，可空`。

**[B-6] 03-详细设计.md:148 | `url_key` 建 `unique(url_key, collected_at_date)`（按天唯一）** | 三重致命：① 与 NFR-IC-03"同一采集周期内重复不产生重复项"语义不符（按天 ≠ 按周期）；② 热榜 30 分钟一轮 = 一天同一 URL 出现 ~48 次，按天唯一会导致**只有第一条入库**，rank 时序彻底丢失，而 ②TT 的 `rank_pct=(rank_total-rank)/rank_total`（02-TT/03:142）与 Influx `topic_heat` 时序点完全依赖每轮快照 → TT 直接断粮；③ 并发插入下唯一约束冲突无处理策略（无 `ON CONFLICT` 说明），会随机抛 23505 错误 | 改为：

```
- 删除 unique(url_key, collected_at_date)
- 新增 idx(url_key)              -- 非唯一，供 L1 查询
- 新增 cycle_bucket int64        -- floor(unix(collected_at_utc) / (interval_minutes*60)) * (interval_minutes*60)
- 新增 dedup_key string          -- sha256(url_key + "|" + source_id + "|" + strconv(cycle_bucket))
- 新增 unique(dedup_key)
- 冲突策略：ON CONFLICT (dedup_key) DO UPDATE SET
      metrics = EXCLUDED.metrics, rank = EXCLUDED.rank,
      collected_at = EXCLUDED.collected_at, updated_at = now()
  （不新建 IntelItem，满足 NFR-IC-03；同一 URL 每轮只更新快照）
- 同时把 (intel_id, platform, rank, rank_total, collected_at) 写入 Influx topic_heat，供 TT 时序
```
Ent 侧需注明 `cycle_bucket`/`dedup_key` 由 `BeforeCreate` Hook 计算，且 Atlas 迁移必须生成对应唯一索引（Ent 原生不支持 `collected_at_date` 生成列，不要走生成列方案）。

**[B-7] 03-详细设计.md:340 vs 02-概要设计.md:49-58 | L3 语义去重的准入条件写的是"且 `category != D_compliance`"，但 `category` 由 **L4 理解层**产出，而三级去重在 **L3 归一化层**（02 §2 架构图与 02 §4.1 主链路顺序均为"去重 → 理解层 → 治理层"） | 执行顺序上 `category` 尚不存在，条件不可求值 → 开发必然自行发挥（有的跳过、有的先分类），导致去重结果不一致 | 删除该条件，改为可在 L1/L2 阶段求值的条件：

```
L3 执行条件（全部满足）：
  ① L1、L2 均未命中
  ② payload.text 有效 token 数 ≥ 12（低于此不参与语义判定）
  ③ 当日 L3 调用量 < ic.dedup.embedding_daily_cap（默认 5000，防成本失控）
硬规则：D_compliance 由治理层闸门处理，与去重层无关；去重层不做类别判断。
```

**[B-8] 03-详细设计.md:179-193 + 02-概要设计.md:124-141 | 主链路顺序是"写证据快照 → 发 `mm.intel.raw` → 归一化生成 `IntelItem`"，但 `EvidenceSnapshot.intel_id` 指向 `IntelItem` | 写证据时 `intel_id` 尚未生成 → 外键悬空、证据与条目无法关联，取证链（🔴 不可后置项）从根上断掉 | 调整顺序并写死：

```
1. 采集器产出 RawFragment
2. 立即生成 intel_id = "it_" + ulid.Make()   （ULID 本地生成，无需 DB）
3. 写证据快照（evidence_id = "ev_" + ulid），落盘 + 计算 raw_hash / chain_hash，回填 evidence.intel_id = intel_id
4. 发 mm.intel.raw（携带 intel_id、evidence_id）
5. 归一化：RawFragment + intel_id + evidence_id → IntelItem（schema 校验）
6. 三级去重 → cluster_id
7. 理解层 → 治理层 → 发布
```
并在 §2.4 注明 `intel_id` 为非空外键、`EvidenceSnapshot` 与 `IntelItem` 同一事务写入。

**[B-9] 03-详细设计.md:318-329 | SimHash 只给了 5 行步骤，关键实现参数全缺：哈希函数未指定、"按位加权累加 → 降维"无公式、IDF 表从哪来未写、无最小文本长度、无空文本/超短文本处理、无多语言处理、无 MinHash 与 SimHash 的选型理由 | 照这一段写不出代码，且不同人写出来指纹不可比（历史数据一次性作废） | 替换为可执行规格：

```
simhash(text, lang) -> uint64:
  1. 归一化：NFKC；去 HTML 标签；URL/邮箱替换为占位符；连续空白折叠
  2. 分词：
     - lang 判定：CJK 字符占比 > 0.3 → 走中文路径
     - CJK：去停用词后做 2-gram（"上海发布" → ["上海","海发","发布"]）
     - 非 CJK：Unicode 词法切分 + lowercase + 去停用词
  3. 权重 tf_idf(w) = tf(w) * idf[w]，idf 表 data/ic/idf.json（周任务 worker/ic/idf_job.go
     从近 30 天语料统计；冷启动用内置默认表，随代码发布）
  4. h = fnv64a(w)（固定，禁止更换；更换等于全量重算）
  5. v[0..63] = 0
     for w in tokens: for i in 0..63: v[i] += ((h>>i)&1) == 1 ? +tf_idf(w) : -tf_idf(w)
  6. f = 0; for i in 0..63: if v[i] > 0 { f |= 1 << i }
边界：
  - tokens < 40 或 text 长度 < 80 字符 → 指纹不稳定，标记 fingerprint_valid=false，
    L2 跳过，直接进 L3
  - text 为空 → 用 title 计算；title 也为空 → fingerprint=0 且 fingerprint_valid=false
  - text > 20000 字符 → 截断到 20000 再计算（防超大文档拖垮权重）
  - 中英混排 → 两段分词结果合并后统一加权
阈值换算（写进注释）：64 位汉明 ≤3 ≈ 相似度 ≥ 0.953，仅对长文本可靠；
  这就是为什么必须有 tokens>=40 的前置条件。不采用 MinHash：单条比对场景
  SimHash O(1) 存储 + 汉明距离更快，MinHash 需维护 k 个 sketch 且需 LSH 分桶，
  V1 日产 3000-8000 条量级收益不抵复杂度。
```

**[B-10] 03-详细设计.md:331-341 | embedding 模型名/维度/部署形态未定；"存 Redis VSS"在本机基线（Redis 6380）**未核实是否带 Stack/VSS 模块**；无降级方案 | 技术基线未列 Redis Stack，若不可用则 L3 完全无法落地，而 L3 支撑验收第 3 条（85% 归并正确率） | 写死为：

```yaml
ic:
  dedup:
    embedding:
      provider: "local"                 # local | cloud
      model: "bge-small-zh-v1.5"        # 固定模型名；更换须重算全量向量
      dim: 512
      endpoint: "http://127.0.0.1:9991" # 本地推理服务（进程外 HTTP）
      timeout: 3s
      daily_cap: 5000
      window_hours: 24
      threshold: 0.86
      store: "redis_vss"                # 前置条件：redis 6380 须为 Redis Stack
      fallback_store: "pgvector"        # 若 redis 无 VSS 模块，启用 pgvector 扩展
```
并在 04 §6 依赖表新增一行"Redis Stack 或 pgvector 扩展 | 阻塞 M3 | 运维 | 待核实"，在 02 §5 失败影响列补充"VSS 不可用 → L3 降级为仅 L1+L2，验收第 3 条判据同步下调并公告"。

**[B-11] 03-详细设计.md:427-460 | `ic.yaml` 缺 9 项必备配置，且**信源级 `interval_minutes`（2-1440，IntelSource 字段）与全局 `cron` 三套周期（hotlist/rss/keyword）谁生效完全没说** | 两套周期并存 = 调度器无法编写；asynq 周期任务到底按信源动态注册还是按固定 cron，是 M2 的核心实现，不定就写不出来 | (1) 明确调度模型：

```
调度模型：以信源级 interval_minutes 为唯一真值。
worker/ic/schedule.go 实现 asynq.PeriodicTaskManager，启动时遍历 enabled 信源
为每个信源注册 PeriodicTask{TaskID: "ic:src:"+src.ID, CronSpec: "*/N * * * *",
Opts: []asynq.Option{asynq.Unique(2*N*time.Minute)}}；
信源 CRUD/启停变更时调用 manager 热更新（监听 source 变更事件）。
ic.cron.* 仅作为"未配置 interval_minutes 时的默认值"，不再独立生效。
```
(2) yaml 补齐：

```yaml
ic:
  nats:
    url: "nats://127.0.0.1:4222"
    stream: "MM_INTEL"
    max_msgs: 2000000          # 契约 §8 limits 按条数，90d 约 100 万条，留 2 倍余量
    max_bytes: 10737418240
    publish_timeout: 5s
    retry_attempts: 3
    ack_wait: 30s
    max_deliver: 5
  asynq:
    queue: "ic"
    concurrency: 8
    max_retry: 3                # 仅对可重试错误
    retryable: ["IC_UPSTREAM_TIMEOUT","IC_RATE_LIMITED"]
    non_retryable: ["IC_ANTI_BOT","IC_ROBOTS_DISALLOWED","IC_CREDENTIAL_INVALID","IC_EMPTY_CONTENT"]
    task_timeout: 300s
    retention: 72h
  robots:
    user_agent: "FrameWorkBot/1.0 (+https://internal/robot)"
    timeout: 5s
    cache_ttl: 24h
    respect_crawl_delay: true
  shortlink:
    enabled: true
    max_redirects: 5
    timeout: 5s
    block_private_ip: true      # SSRF 防护，禁止解析到 10/172.16/192.168/127/169.254
  breaker:
    fail_threshold: 5           # 连续失败 N 次
    cooldown: 30m               # 自动暂停时长
  retention:
    evidence_days: 90
    intel_days: 365
    cluster_idle_close_hours: 48
    sweep_cron: "0 3 * * *"
  embedding: { ... 见 B-10 ... }
```

**[B-12] 03-详细设计.md:466-480 | 9 个错误码是纯字符串常量，未走本仓错误定义规范 | 本仓用 `errors/errors.proto` + `protoc-gen-go-errors`（`buf.gen.yaml` 已配置，见 `api/admin/v1/admin_error.proto`），前端与网关依赖 `reason` 做分支；不定义则前端只能匹配 message 文本，且无 HTTP 状态映射 | 新增 `api/media/v1/ic/ic_error.proto`：

```proto
syntax = "proto3";
package media.v1.ic;
import "errors/errors.proto";
enum IntelCollectionErrorReason {
  option (errors.default_code) = 500;
  IC_SRC_NOT_FOUND      = 0  [(errors.code) = 404];
  IC_SRC_DISABLED       = 1  [(errors.code) = 409];
  IC_ROBOTS_DISALLOWED  = 2  [(errors.code) = 403];
  IC_RATE_LIMITED       = 3  [(errors.code) = 429];
  IC_UPSTREAM_TIMEOUT   = 4  [(errors.code) = 504];
  IC_ANTI_BOT           = 5  [(errors.code) = 422];
  IC_EMPTY_CONTENT      = 6  [(errors.code) = 422];
  IC_SCHEMA_INVALID     = 7  [(errors.code) = 500];
  IC_CREDENTIAL_INVALID = 8  [(errors.code) = 401];
}
```
同时统一重试次数（现 02 §4.3 说"重试 1 次"、03 §8 说"最多 2 次"、yaml `retry: 1`，三处不一致）：改为 `max_attempts`（含首次）语义，timeout/5xx = 3，429 = 按 `Retry-After` 最多 3，ANTI_BOT/4xx = 1（不重试），并把 02 §4.3 的错误码补上 `IC_` 前缀。

**[B-13] 02-概要设计.md:197-210 vs 03-详细设计.md:89-108 | 02 承诺 9 个 REST 端点，03 只定义了 4 个 RPC；且 `RunCollector` 路径两边不一致（02 写 `/api/v1/media/ic/collector/run`，03 写 `/api/v1/media/ic/sources/{source_id}/run`）；`toggle` / `health` / `intels` 检索 / `evidence` 导出 / `supplement` 手贴 / `briefings` / `SplitCluster` / `MergeCluster` 全部无 proto | 前端按 02 的路由表开发，后端按 03 生成，联调必然对不上；`supplement` 手贴续跑是 FR-INT-04 的验收项，没 proto 就没法交付 | 以 03 的 `{source_id}/run` 为准，回改 02 §6.2 表格；并在 03 §1 补齐全部 RPC 签名（含 `ToggleIntelSource`、`GetCollectorHealth`、`ListIntels`、`GetIntel`、`ExportEvidence`、`SupplementIntel`、`SplitCluster`、`MergeCluster`、`ListBriefings`），每个带 `option (google.api.http)` 与 validate 规则。

**[B-14] 03-详细设计.md:117-205 | Ent schema 缺 6 张必需表：`dead_letter`（§8 引用但无定义）、去重调参样本表（§4.4 与 02 §9 均要求"拆分/合并行为记入调参样本"）、采集运行记录 `IntelCollectRun`（审计与"谁在何时采了什么"）、cluster 合并事件表（`merged_by` 与 merge 事件无处落）、账号池表（FR-INT-13 多账号轮询无载体）、授权记录表（闸门 `authorization` 规则无判据来源） | 4 处"提了一句"的落地点全部缺失，其中 `authorization` 规则是 🔴 合规项，无来源表 = 规则永远不命中 = 合规形同虚设 | 在 §2 追加 6 张表的字段级定义，最小集：

```
DeadLetter{ id, task_type, payload jsonb, error_code, error_msg, attempt,
            first_failed_at, last_failed_at, status(enum pending|replayed|discarded),
            replayed_by, replayed_at }            idx(status, last_failed_at)
DedupSample{ id, intel_id, cluster_id, action(enum auto_merge|manual_split|manual_merge),
             level(url|fingerprint|embedding), sim_score, operator, created_at }
IntelCollectRun{ id "run_"+ULID, source_id, trigger(enum cron|manual|retry),
                 started_at, finished_at, fetched, normalized, merged, blocked,
                 status, error_code }             idx(source_id, started_at)
ClusterMergeEvent{ id, from_cluster_id, to_cluster_id, reason, member_count, created_at }
CredentialAccount{ id "acc_"+ULID, platform, account_key, credential_ref, enabled,
                   quota_used, quota_limit, fail_streak, paused_until, last_used_at }
                                                  unique(platform, account_key)
AuthorizationRecord{ id, subject(enum author|domain|asset), subject_key,
                     status(enum granted|unknown|denied), scope, evidence_ref,
                     valid_until, created_at }    idx(subject, subject_key)
```

**[B-15] 四份文档全文 | **时区规则一次都没写** | 契约 §0.2 强制"存储一律 UTC RFC3339 带 Z"，02-TT 已明确 Influx timestamp 为 UTC（NFR-TT-03）。IC 是时间生产方，不写 = `collected_at`/`_as_of`/`retain_until` 全系统时区漂移，跨市场对齐失效，且 NFR-IC-02"发布到入库 ≤ 周期+5 分钟"无法计算 | 在 03 §2 开头加全局约束段，并在 01 §4 增 NFR-IC-10：

```
时区纪律（全部时间字段强制）：
- Go 侧一律 time.Time 且 .UTC()；禁止 time.Now() 直接落库，统一 clock.Now().UTC()
- PostgreSQL 列类型一律 timestamptz，禁止 timestamp
- 序列化一律 RFC3339 带 Z（"2026-10-17T01:00:00Z"），禁止 +08:00
- 展示层（Quasar）转 Asia/Shanghai，转换只在前端做
- 契约 §1 示例中 "2026-10-17T01:00:00+08:00" 应改写成 "2026-10-17T01:00:00Z"（回写契约）
```

**[B-16] 03-详细设计.md:360-370 + 287-298 | `Gate.Evaluate` 签名无 `error` 返回（规则加载/DB 失败时无法表达），且 `robots` 规则判定自相矛盾：§5.1 说 `robots_allowed=false → block`，§3.5 说 robots 抓取失败时"保守放行" | 合规闸门是 🔴 不可后置项。无 error 返回 = 规则加载失败必然 fail-open（违规内容直通下游），违反 NFR-IC-04"100% 条目经闸门、无旁路"；robots 矛盾会让开发在"放行"和"阻断"之间随机选 | 改为：

```go
type Gate interface {
    // err != nil 时调用方必须按 fail-closed 处理：判 GateResult{Decision: GateReview, Reasons: ["GATE_UNAVAILABLE"]}
    Evaluate(ctx context.Context, item *contract.IntelItem) (GateResult, error)
}
```
并把 §3.5 的判定改写为（可直接替换）：

```
robots 三态判定（穷举，无歧义）：
  robots_checked == true  && robots_allowed == true  → 正常，不改 gate
  robots_checked == true  && robots_allowed == false → gate = block（规则 ID: ROBOTS_DISALLOWED）
  robots_checked == false                            → gate = review（不是放行！）
                                                       并且禁止进入 MME 自动流程
"保守"的含义是"不下 block 结论"，不是"允许流通"。
```

**[B-17] 03-详细设计.md:396-419 | "防篡改"只写了 `raw_hash = SHA-256` | SHA-256 只能校验完整性，不能防篡改——同一进程改了原文就能重算哈希，审计方无法自证；验收第 4 条"证据包可导出"与合规 §3 不可后置清单第 4 项（取证链）都建立在此之上 | 升级为哈希链 + 只追加：

```
EvidenceSnapshot 增加两列：
  prev_hash  string  // 上一条证据的 chain_hash（首条为 64 个 0）
  chain_hash string  // sha256(prev_hash || raw_hash || intel_id || collected_at.UTC().UnixNano())
约束：
  1. 证据目录以"只追加"方式写入，文件名 = evidence_id，禁止覆盖与删除（删除仅通过
     pkg/retention 到期清理，且清理写审计日志）
  2. 每次写入后校验：SELECT chain_hash FROM evidence_snapshots ORDER BY id DESC LIMIT 1
     必须等于新记录的 prev_hash；不匹配 → 触发 IC_EVIDENCE_CHAIN_BROKEN critical 告警
  3. 导出 ZIP 内增加 chain.json（含 prev_hash/chain_hash/共 N 条链），并对 ZIP 计算
     sha256 写入 meta.json
  4. 对象存储（MinIO）开启 object versioning + retention lock；本地目录则设为
     只追加挂载并每日异地备份
  5. 导出行为本身记审计日志：谁、何时、导出哪个 intel_id
```

**[B-18] 02-概要设计.md:54-58 vs 03-详细设计.md 全文 | 02 有 L4 理解层（实体抽取/情感/摘要/分类/可加工性），03 **完全没有理解层的任何设计**——无 `extract_usecase.go` 实现、无 ABCD 分类算法、无 `workability.score` 计算公式、无实体抽取实现与中英文案 | 04 §4.5 给 M5 排了 8 人日要交付 FR-INT-08/09/10，但详细设计为零，开发只能现编；且 `entities` 是 MIA 实体链接的唯一输入（confidence ≥0.6），`workability.score` 是 MME 的准入阈值（02 §10），都是下游硬依赖 | 在 03 新增 §12 理解层，至少给到：

```
实体抽取（FR-INT-08）：
  规则优先：人名/机构名词典 + 正则（职务前缀、上市公司名、"XX市/省"）+ 地点词表
  LLM 增强（可降级）：prompt 输出 JSON {type,value,confidence}，temperature=0
  confidence 标定：词典命中 0.9 / 正则 0.7 / LLM 0.6~0.95；<0.6 丢弃（契约要求）
  中文分词：gojieba（须登记依赖，见 S-许可）；英文：空格 + 大写连续词
分类（FR-INT-09）：
  规则版（可砍单降级态）：关键词 → A_hot(有 rank 且 rank/rank_total ≤0.3)、
    D_compliance(命中"规定/处罚/下架/侵权/新规")、C_market(命中政策/公司/央行/SEC/财报)、
    否则 B_material(text 长度 ≥800 或 media_refs 含长视频)
  LLM 版：输出 category + 理由，与规则不一致时以规则为准并记样本
可加工性（FR-INT-10）：
  score = 0.4*has_material + 0.3*has_viewpoint + 0.3*narratable
    has_material  = clamp(len(media_refs)/3, 0, 1)
    has_viewpoint = clamp(len(entities)/5, 0, 1)
    narratable    = clamp(min(len(text),3000)/3000, 0, 1) * (lang 是否支持 ASR ? 1 : 0.5)
  阈值 workability_min_score: 0.5（MME 准入），低于此不自动进 MME
  理由必填：reason = "素材不足" | "无明确观点" | "时长不足" | "语言不支持"
```

**[B-19] 03-详细设计.md:151、154 | `fulltext(title)` 与 `entities gin(jsonb)` 直接写进表设计，但**PostgreSQL 默认配置下中文全文检索不可用**（`to_tsvector('simple', ...)` 对中文整段不切词），gin(jsonb) 也需指定 `jsonb_path_ops` | FR-INT-14"全文 + 实体 + 时间范围检索"无实现路径，验收第 3 条与 M5 判据都会卡在这里 | 明确方案并写入部署依赖：

```
方案（选一，写死）：
  A. 安装 zhparser（需编译）或 pg_bigm 扩展 → tsvector 生成列 + GIN
  B. 引入 Meilisearch（MIT）独立服务 → 索引 title+text，PG 仅存结构化字段
  C. V1 降级：仅 ILIKE '%kw%' + 实体 jsonb 的 jsonb_path_ops GIN（需声明验收降级）
推荐 B。无论选哪个，必须在 02 §5 部署表与 04 §6 依赖表登记该组件（阻塞 M5）。
同时把 Ent 字段定义补齐：
  search_vector 字段类型需标注为 tsvector 生成列（Ent 不支持，走 Atlas 自定义迁移）
  entities 索引需标注 entsql.Annotation{Indexes: ...} 或 Atlas 手写 GIN
```

**[B-20] 03-详细设计.md 全文 | 幂等与并发落地缺失：无 asynq 任务唯一性（同一信源上一轮未结束下一轮又触发）、无 NATS 消费者定义（durable 名 / ack 策略 / max_deliver / 重复投递窗口）、无 DB 冲突处理、无背压 | NFR-IC-03 幂等与 NFR-IC-07 资源占用（≤1.5GB、并发 ≤8）无法验收；NATS 至少一次投递下必然产生重复条目 | 写死三条：

```
1. 任务唯一：每个信源周期性任务注册时带
   asynq.TaskID(fmt.Sprintf("ic:src:%s:%d", src.ID, cycle_bucket)) +
   asynq.Unique(2 * interval)   // 防止跨轮重叠
   进程内另加 singleflight（key=source_id），双保险
2. NATS：
   MM_INTEL 消费者：durable "ic-normalizer"（pull，显式 ack），
     AckWait 30s、MaxDeliver 5、MaxAckPending 1000、AckPolicy explicit
   发布侧：每条消息设置 Nats-Msg-Id = intel_id，Stream 开启
     Duplicate Window = 2m（NATS 服务端去重，抵消至少一次投递）
   归一化消费侧：以 dedup_key 做 DB 幂等，重复消息直接 ack 丢弃
3. 背压：
   采集 → raw 队列 inflight 上限 5000；超限则采集端限速（令牌桶额外扣减）
   并计 ic_backpressure_dropped_total；禁止无界 goroutine：统一
   errgroup.WithContext + SetLimit(ic.concurrency)
```

**[B-21] 03-详细设计.md:225-241 | `RawFragment.Raw []byte` 承载原始 HTML，而 `mm.intel.raw` 要发它 | NATS 默认 `max_payload = 1MB`，单个 HTML 常 200KB-2MB，超限直接发布失败；且 8 并发 × 全量 HTML 常驻内存会击穿 NFR-IC-07 的 1.5GB | 改为：

```
RawFragment 不再携带 Raw []byte 进 NATS：
  - 采集后立即把原始片段落证据存储，得到 evidence_id / raw_path
  - RawFragment 仅保留 RawSize int + RawTruncated bool + RawPath string
  - mm.intel.raw 载荷上限硬编码 256KB：超过则强制截断并置 RawTruncated=true
  - NATS server 配置 max_payload: 1MB（写进 infra 配置基线）
  - 采集器侧单条 Raw 内存上限 4MB，超过丢弃并计 ic_raw_oversized_total
```

**[B-22] 04-开发进度.md:27 vs 72-76 vs 162 | 三个自部署服务的部署时点自相矛盾：M0 判据（2026-12-14）要求"三个实例可用"，§4.2 却把部署列为 M2 任务，§7.1 缓解措施又说"提前到 M0 完成，不等 W2" | M2 是"≥15 源稳定出数"的硬节点，若部署在 M2 才做，M2 判据在 M2 内不可达；排期无法执行 | 删除 §4.2 中三条部署任务，改为在 M0 单列任务组：

```
### 4.0 M0 · 前置就绪（2026-12-14 前，运维主责）
- [ ] 部署 newsnow 自部署实例，锁定版本 <tag/commit>，输出内网 base_url
- [ ] 部署 RSSHub 独立容器（不 fork、不改源码），输出 base_url + 可用路由清单
- [ ] 部署 Crawl4AI 服务，输出 base_url + 超时/并发参数
- [ ] Q4/Q5/Q8 拍板结论写入 01 需求规格 §8
- [ ] 核实 Redis 6380 是否具备 VSS（否则装 pgvector）
- [ ] 核实/安装中文全文检索组件（见 B-19）
- [ ] 建 NATS MM_INTEL stream（subjects mm.intel.>，max_msgs 2000000，max_age 90d）
```
并把 §6 依赖表中三条基础设施的"阻塞 M2"改为"阻塞 M0"，负责人改运维，状态列保留 🔴。

**[B-23] 04-开发进度.md:126-137 | 工时估算不自洽且严重低估 | ① M1-M4 = 34 人日，排期 W1-W4 只有 4 周，2 人满负荷 = 40 人日，零缓冲（不含评审返工、联调、契约变更）；② M2 仅 10 人日却包含"部署三个外部服务 + 五路采集器 + 限流 + robots + asynq 调度 + 健康检查"，其中光 newsnow 多平台适配（≥15 源）+ YouTube/B站/Reddit 三套官方 API 适配就不止 10 人日；③ M5 的 8 人日要交付 FR-INT-08/09/10/14/17 五条，而详细设计对这五条零内容（B-18）；④ 04 全文**没有任何前端（Quasar 4 个页面）工作量**，02 §6.3 却定义了 4 个页面 | 排期不可执行，第一周就会失守，且会倒逼砍掉 🔴 不可砍项 | 重排为：

```
M0 前置就绪        6 人日（运维主导，开发 1 人配合）
M1 契约与骨架      8 人日（+ JSON Schema + ic_error.proto + 映射层）
M2 采集与治理基建 16 人日（hotlist 5 / rss 3 / api 4 / crawl 2 / 限流+robots+asynq 2）
M3 归一化与去重   14 人日（URL 归一 2 / SimHash+IDF 4 / embedding+VSS 4 / 聚类 2 / 证据 2）
M4 合规与治理     10 人日（闸门 3 / 规则 CRUD 2 / 速率+熔断 2 / 凭据 1 / 审计 2）
M5 理解层与补齐   16 人日（实体 5 / 分类 3 / 可加工性 2 / 检索 4 / 简报 2）
前端 4 页面       10 人日（sources 3 / intels 3 / detail 2 / review 2）——新增行
联调·契约·验收·缓冲 12 人日（≥15%，不得为 0）
合计             92 人日 ≈ 2 人 × 9.2 周 → 超出 8 周窗口，必须在 M0 前砍单或加人
```
同时把 §5 那句错误的换算删掉（"42 人日 ÷ 5 工作日 = 8.4 人周单人"，原文写成 4.2 人周是按 10 日/周算的，与"周"混用）。

**[B-24] 01-需求规格.md:71 | "P0 共 8 条"与表格实际不符——标 P0 的是 01/02/03/04/05/06/07/11/12 共 **9 条** | 04 §7.2 的"不可砍"清单据此漏掉了 FR-INT-05（RSS 订阅采集）与 FR-INT-03/04，砍单时会误砍 P0 | 改为"**P0 共 9 条**"，并同步修正 04 §7.2 不可砍清单为：

```
────────── 以下为 🔴 不可砍 ──────────
   FR-INT-12 合规闸门
   FR-INT-11 取证快照
   FR-INT-07 三级去重
   FR-INT-06 归一化
   FR-INT-05 RSS 订阅采集（P0）
   FR-INT-04 正文抽取（P0，与 MME W7 共用）
   FR-INT-03 采集器插件五路（P0）
   FR-INT-02 热榜采集
   FR-INT-01 信源注册表
```

**[B-25] 02-概要设计.md:181 | "NATS 挂 → 本地磁盘队列暂存（≤1h）" | 这是对可用性的显式承诺，但 03 无实现、04 无排期、yaml 无配置、包结构里也没有对应模块 → 承诺了做不到的能力，NATS 故障时实际行为是丢数据 | 要么删除该承诺改为"NATS 不可用时采集暂停并告警（不丢、不采）"，要么补齐设计与排期。建议（改动最小、最诚实）：

```
NATS 不可用 → IC 停止发布但继续采集落库（PG 是可用的），消息侧标记
publish_state=pending，由 worker/ic/republish.go 每 30s 扫描 pending 重试，
重试窗口 ≤1h，超时置 failed 并告警。存储不落在"本地磁盘队列"这种无规范载体上。
```

**[B-26] 03-详细设计.md:157 | `fingerprint uint64` 在 PostgreSQL 上无法安全存储 | PG 无无符号整型，Ent `Uint64` 映射到 `bigint`（int8，上限 2^63-1），而 64 位 SimHash 的最高位有 50% 概率为 1 → 约一半的记录会写入失败或溢出 | 运行期随机报错，且是数据正确性问题（指纹错误 = 去重全错） | 改为拆分存储并建倒排（同时解决 §4.2 的"4 段倒排"落地）：

```
IntelItem 增加 4 列（替换 fingerprint uint64）：
  fp_b0 smallint  // (f >> 48) & 0xFFFF   有符号范围足够（0..65535）
  fp_b1 smallint  // (f >> 32) & 0xFFFF
  fp_b2 smallint  // (f >> 16) & 0xFFFF
  fp_b3 smallint  // f & 0xFFFF
  fingerprint_hex string  // 16 位小写 hex，契约输出用（契约 dedup.fingerprint 是 string）
索引：idx(fp_b0, fp_b1)、idx(fp_b1, fp_b2)、idx(fp_b2, fp_b3)、idx(fp_b0, fp_b3)
      —— pigeonhole：汉明 ≤3 时必有一段完全相同
查询：4 段分别查候选集，并集后再算 popcount(f XOR cand) <= 3
```

---

## 二、严重问题（影响正确性或可维护性）

**[S-1] 03-详细设计.md:148 | `unique(url_key, collected_at_date)` 需要 `collected_at_date` 生成列，Ent 不支持生成列、Atlas 也不会自动生成 | 迁移脚本必然手写，且与 Ent schema 脱节 | 见 B-6，直接改为 `dedup_key` 方案，删除生成列依赖。

**[S-2] 03-详细设计.md:167-177 | `IntelCluster` 无 `region`、无 `category`、无向量/中心点、无关闭条件、无成员列表 | 跨 region 误合并（中日同事件）、cluster 无限增长、无法按类目检索 | 增加 `region`、`category`、`centroid_ref`（Redis key）、`status(open|closed)`、`closed_at`；关闭规则 `last_seen_at < now() - 48h → closed`。

**[S-3] 03-详细设计.md:343-354 | 聚类合并"取成员最多的为主，其余并入"未定义并发控制与事务边界 | 8 并发下两个 cluster 同时被第三条合并 → 环路、member_count 错乱、cluster_id 指向已失效簇 | 写死：按 `cluster_id` 字典序排序后 `SELECT ... FOR UPDATE` 加锁，单事务完成成员迁移 + `member_count` 重算 + 写 `ClusterMergeEvent`；乐观锁失败重试 3 次；`IntelItem.cluster_id` 更新走批量 `UPDATE ... WHERE cluster_id IN (...)` 同一事务。

**[S-4] 03-详细设计.md:331-341 | "与近 24h 内同 region 的条目向量做余弦相似度"——未定义 cluster 自身的向量如何维护 | 若只比"单条条目"，则 cluster 代表条目变化后旧成员无法召回；若每条都要和 24h 内全部条目比，日产 2000 条时是 2000×2000 全量扫描 | 定义增量聚类：cluster 维护加权中心向量 `centroid`（成员数 ≤50 时随新成员增量更新，>50 冻结），新条目先与近 72h 内 `status=open` 的同 region cluster centroid 比对（数量级 ≤2000，centroid 比对可批量），命中后再与代表性成员做二次确认。

**[S-5] 03-详细设计.md:336 | "近 24h"硬窗口导致跨窗事件（持续 2-3 天的热点）无法归并 | 事件跨越窗口边界时会分裂成多个 cluster，直接影响 TT 话题追踪与验收第 3 条 | 窗口与 cluster 生命周期解耦：参与比对的候选 = `status=open` 且 `last_seen_at > now() - 72h` 的 cluster centroid；`mm.intel.raw` 侧仍按 24h 保留原始向量以控成本。

**[S-6] 03-详细设计.md:310、316 | 短链展开无 SSRF 防护与重定向限制 | 展开 `http://169.254.169.254/...` 类的短链会打到云元数据/内网服务，属于安全漏洞 | 见 B-11 yaml：强制 `max_redirects=5`、`block_private_ip=true`，并在解析后对 IP 做私网/回环/链路本地/保留段黑名单校验（含 IPv6 ULA），命中则 `IC_ROBOTS_DISALLOWED` 同级错误 `IC_URL_BLOCKED`。

**[S-7] 03-详细设计.md:287-298 | robots 缓存 24h，但未提取 `Crawl-delay` 指令 | 站点明确要求的抓取间隔被忽略，是抓取合规（03-compliance §1.6）的直接违反 | `RobotsChecker` 接口扩为 `CrawlDelay(ctx, host) (time.Duration, bool)`，令牌桶 key 用 host，速率取 `min(平台默认, crawl_delay)`，并在 `EvidenceSnapshot` 记 `crawl_delay_ms`。

**[S-8] 03-详细设计.md:298 | "api/rss/hotlist 路按平台条款豁免（记录理由）"——豁免理由无处可配、无处可查 | 审计时无法自证为什么没查 robots | `IntelSource` 增列 `robots_exempt bool`、`robots_exempt_reason string`（非空校验，必填）；`ComplianceRule` 增 `rule_type = platform_tos`；前端信源表单强制填写。

**[S-9] 03-详细设计.md:195-205 | `ComplianceRule` 缺审计与生命周期字段 | 合规规则是 🔴 项的判据，谁改的、何时生效、命中多少次必须可审计（03-compliance §5） | 增加 `created_by`、`updated_by`、`created_at`、`updated_at`、`effective_from`、`effective_until`、`hit_count`、`scope(global|platform)`、`platform`；规则变更写 `admin_operation_log`（本仓已有表）。

**[S-10] 03-详细设计.md:275-285 | `RateLimiter` 无多实例原子性说明、无日配额概念、退避状态不持久 | 多实例部署时限流失效；进程重启后退避计数归零，会疯狂重试被封的源 | 明确：令牌桶用 Redis Lua 脚本保证原子（`INCR` + `EXPIRE` 或 `cl.throttle`）；配额用 `INCR` + 日维度 key + TTL；退避状态存 `ic:backoff:{platform}` 带 TTL，重启后继承。

**[S-11] 03-详细设计.md:273-285 vs 01-需求规格.md:65 | FR-INT-13 要求"多账号轮询"，但设计只有限流，无账号池与轮询算法 | FR-INT-13 是 M4 交付项，不可达 | 见 B-14 的 `CredentialAccount` 表；轮询算法写死为：`按 (fail_streak ASC, last_used_at ASC) 选 enabled 且 paused_until < now 的账号`，命中 429/401 则 `fail_streak++`、`paused_until = now + min(2^fail_streak × 5m, 4h)`。

**[S-12] 03-详细设计.md:466-480 | 只定义了错误码，未定义熔断动作（03 §9.3 只有告警） | "连续 3 次失败 → warn"没有自动处置，被封的源会持续重试直到账号封禁，违反 03-compliance §2"异常自动熔断" | `IntelSource` 增列 `status(enum active|paused|disabled)`、`paused_until`、`fail_streak`；熔断规则：`fail_streak ≥ 5 → status=paused, paused_until=now+30m, 告警`；恢复需到期自动或人工。

**[S-13] 03-详细设计.md:480 | `dead_letter` 只提"表保留失败消息，提供重放入口"，表结构与入口 API 都无定义 | 错误可恢复时数据永久丢失 | 见 B-14 表定义；并重放入口写进 proto：`POST /api/v1/media/ic/dead-letters/{id}/replay`、`ListDeadLetters`，重放必须记录 `replayed_by`。

**[S-14] 01-需求规格.md:80-86 vs 03-详细设计.md 全文 | NFR-IC-08"IntelItem 元数据 ≥1 年"无清理策略；NFR-IC-01"7 日滚动成功率"无统计口径 | 元数据表无限增长；成功率无法计算（无滚动窗口实现说明） | 见 B-11 的 `retention.intel_days=365` + `sweep_cron`；成功率口径写死：`success_count_7d / (success_count_7d + fail_count_7d)`，由每日 `worker/ic/health_job.go` 重算（不用计数器累加，避免无限增长导致口径漂移），并同时写入 Influx 供看板。

**[S-15] 01-需求规格.md:81 + 四份文档 | NFR-IC-02"从发布到入库 ≤ 采集周期 + 5 分钟"需要**上游发布时间** | Ent 与契约均无 `published_at`，该 NFR 不可度量 | 见 B-1 增 `published_at`（可空），并定义：`latency = collected_at - published_at`；`published_at` 缺失时不计该指标（不伪造）。

**[S-16] 03-详细设计.md 全文 | 时钟漂移零覆盖 | 上游时间不可信、本地时钟可能漂移，会让 `collected_at` 出现未来时间，进而污染去重窗口与 Influx 时序 | 增加纪律：`collected_at = clock.Now().UTC()`（本地接收时间）；`published_at` 若 > `collected_at + 5min` 则标记 `published_at_suspect=true` 并保留原值；宿主机强制 NTP 同步（写进 infra 检查项）；禁止用 `published_at` 做去重窗口边界。

**[S-17] 03-详细设计.md:43-57 | `config_json string`（proto）与 `config jsonb`（Ent）类型不一致 | 序列化边界不清，运营填的 JSON 可能在 proto/DB 之间往返失真 | proto 改为 `google.protobuf.Struct config` 或保留 string 但明确"string 内是 JSON 原文，写入 Ent 前 `json.Valid()` 校验，读回原样返回"；并在 `IntelSource` 增加 `config_schema_hint`（如 `newsnow.hotlist/v1`）供前端渲染动态表单。

**[S-18] 03-详细设计.md:51 | `interval_minutes` 校验 `gte:2` 与 yaml"不得低于 30（newsnow 缓存）"冲突 | 运营可以配成 2 分钟，疯狂打 newsnow，违反作者呼吁与合规 §1.6 | 校验改为按 method 分级：`hotlist` 最小 30、`rss` 最小 5、`api` 最小 10、`crawl` 最小 60，用 CEL 或多段 validate 在服务层实现，并在前端下拉里只给合法档位。

**[S-19] 03-详细设计.md:55、12（last_run_at）、59-65（last_success_at） | 时间字段在 proto 里用 `string` 和 `int64` 混用 | 格式与时区无约束，前端解析必然出错 | 全部改为 `google.protobuf.Timestamp`；若必须 string，则在注释写死 `RFC3339 UTC，例 2026-10-17T01:00:00Z`。

**[S-20] 02-概要设计.md:38-47 vs 03-详细设计.md:24-41 | `SourceKind`（hotlist/keyword/creator/rss/competitor）与 `CollectMethod`（hotlist/api/rss/crawl/sidecar）两个枚举都有 `hotlist`/`rss`，但二者的**合法组合矩阵**未定义 | 运营能配出 `kind=creator & method=crawl` 这类无效组合，运行时才报错 | 增加矩阵表并写进前端校验：

```
hotlist   → hotlist(+api 回退)          keyword → api|crawl|hotlist
creator   → api|rss|crawl               rss     → rss
competitor→ api|rss|crawl               (sidecar 仅 kind=creator 且须授权)
```

**[S-21] 03-详细设计.md:255-259 | `var registry = map[...]Collector{}` 全局裸 map | 若后续支持运行时注册/注销（如插件化），并发读写 panic | `sync.RWMutex` 包裹，或改为 `sync.Map`；`init()` 自注册说明保留，但注明"运行期只读"。

**[S-22] 03-详细设计.md:213-223 vs 398-404 | `EvidenceStore.Save(ctx, *RawFragment)` 让 data 层依赖 biz 层的结构体 | 依赖方向倒置，biz 与 data 易成环 | `RawFragment` 下沉到 `pkg/mediacontract`（契约包，二者都可依赖），`biz/ic/ports.go` 只声明接口；或改为 `Save(ctx, ev *contract.EvidenceInput)`。

**[S-23] 03-详细设计.md:343-354、02-概要设计.md:258 | `SplitCluster`/`MergeCluster` API 与"调参样本表"被反复提及，但 proto 与 REST 表中都没有 | 语义去重误合并的唯一补救手段（02 §9 风险项）无法使用 | 见 B-13 补齐 RPC，并明确 `POST /api/v1/media/ic/clusters/{id}/split`、`POST /api/v1/media/ic/clusters/merge`，操作写 `DedupSample` 与操作审计。

**[S-24] 03-详细设计.md:126-139、145-165 | Ent `IntelItem` 缺 `created_at`/`updated_at` 审计字段（`IntelSource` 却有），`IntelSource` 缺 `status`/`fail_streak` | 审计不一致；熔断（S-12）无处落地 | `IntelItem` 补 `created_at`/`updated_at`；`IntelSource` 补 `status`、`paused_until`、`fail_streak`、`last_success_at`。

**[S-25] 03-详细设计.md:151-152 | `lang` 取值只写了 "zh-CN / en" | 国外源含日/韩/德/法/西，lang 缺失会让分词走错路径（B-9 的分词分支依赖它） | 改为 BCP-47 全集 + 检测兜底：优先上游声明 → 其次 Unicode 区块统计 → 兜底 `und`；`und` 时分词按 CJK 占比自动判定，并把判定结果写回 `lang`。

**[S-26] 02-概要设计.md:270 | "MME 仅消费 `category=B_material` 且 `workability.score` ≥ 阈值"——阈值未定义、未配置化 | MME 无法编码 | 见 B-18 的 `workability_min_score: 0.5`，写入 `ic.yaml` 并在 02 §10 明确"该阈值由 IC 配置，MME 只读，不各自硬编码"。

**[S-27] 03-详细设计.md:417 | 证据存储"本地目录或 MinIO"二选一未定，MinIO 未出现在 02 §5 部署表 | 部署时无法决策，且两者生命周期/锁机制完全不同（B-17 依赖对象锁） | 定为本机目录 `apps/server/storage/ic/evidence`（V1）+ 接口抽象保留 MinIO；02 §5 增加一行"对象存储（可选，Phase 2）"。

**[S-28] 03-详细设计.md:484-497 vs 01-需求规格.md:83 | NFR-IC-05 要求"健康度、延迟、成功率、配额消耗"四项，03 列了 8 项指标但**没有配额消耗指标**，04 §8 却说"八项指标全部上报" | 门禁与 NFR 对不上 | 增补 `ic_quota_used` Gauge(platform) 与 `ic_quota_limit` Gauge(platform)；04 §8 改为"九项指标"，或直接改为"NFR-IC-05 四项必达 + 其余五项尽力"。

**[S-29] 03-详细设计.md:510-517 vs 04-开发进度.md:198 | 03 定义了 4 条告警规则，04 门禁写"三类告警规则生效" | 门禁不可验证 | 统一为 4 类：`source_fail_streak` / `source_7d_success_rate` / `pipeline_zero_output` / `gate_block_burst`，并在 04 §8 改写。

**[S-30] 04-开发进度.md:28 | M1 判据含"Influx 接入"，但 InfluxDB 按基线**仅 TT 使用**（02-tech-selection §5、systems README §4） | IC 不该引入 Influx，判据错误会误导开发 | 删除 M1 判据中的 Influx；若 IC 需要采集健康时序改为写 Prometheus/observability（本仓已有）。

**[S-31] 04-开发进度.md:59-124 | 任务清单**完全没有前端任务**（02 §6.3 定义 4 个 Quasar 页面）**，只有一行菜单 seed | 前端工作量被隐形，排期必然失守 | 见 B-23 增补前端 10 人日与任务组：`/media/ic/sources`、`/media/ic/intels`、`/media/ic/intels/{id}`、`/media/ic/review`，每项含"buf 生成 TS 客户端 + API Resource + Casbin 同步"。

**[S-32] 01-需求规格.md:53 vs 03-详细设计.md:24-31 | FR-INT-01 验收"新增一个信源不改代码"，但 `SourceKind` 是 proto enum | 新增一种信源种类必须改 proto 重新生成，与"零代码改动"直接冲突 | 把验收判据限定为"在**已支持的 kind/method 组合内**新增一个信源（如新增一个微博热榜源、一个 RSS 订阅源）零代码改动"；超出该范围属新功能，走 ADR 与契约演进。

**[S-33] 01-需求规格.md:137 | 验收第 1 条"≤10 分钟零代码改动"依赖 newsnow 已支持该平台 | newsnow 不支持的平台仍需改代码或直采适配器，验收判据不可达 | 在 01 §5 附"newsnow 已支持平台 id 清单（≥10 国内 + ≥5 国外）"作为验收基线；未能覆盖的平台走直采适配器并在文档标注。

**[S-34] 03-详细设计.md:526 vs 00-shared/01-data-contract.md:361 | 契约测试要求 JSON Schema 校验，但 `docs/systems/00-shared/schema/*.json` 当前**不存在**（契约注明"随实现补充"） | M1 的契约门禁无可校验对象，IC 无法自证 | 在 04 §4.1 明确"M1 首日产出 `docs/systems/00-shared/schema/intel_item.schema.json`"，并规定：schema 必须**由 proto 生成**（`buf build -o schema` 或 protoc-gen-jsonschema），禁止手写，避免 proto 与 schema 双份真值漂移。

**[S-35] 03-详细设计.md:521-531 | 覆盖率门槛（biz/ic ≥70%、去重与闸门 ≥85%）无 CI 落地方式 | 门槛形同虚设 | 写入 Makefile：`make test-ic` 带 `-coverpkg=./app/admin/internal/biz/ic/...,./app/admin/internal/collector/...`，并在 CI 对 `dedup`/`governance` 包单独断言 ≥85%。

---

## 三、一般问题 / 改进建议

**[M-1] 03-详细设计.md:18 | `option go_package` 手写与仓库现状冲突**：本仓 `buf.gen.yaml` 开了 `managed` 模式 + `go_package_prefix`，`api/radar/v1/*.proto` 明确注释"go_package 由 buf managed 注入，勿手写"。建议删除该行，或保留但注明会被 managed 覆盖。

**[M-2] 03-详细设计.md:16 | proto package 用 `media.v1.ic`**，既有 proto 均为 `<domain>.service.v1`（`admin.service.v1`、`radar.service.v1`）。建议统一为 `media.ic.v1` 或在文档注明"因 buf lint PACKAGE_DIRECTORY_MATCH 要求目录 `media/v1/ic` 对应 `media.v1.ic`，与历史 proto 风格不一，已确认 lint 放行"。

**[M-3] 02-概要设计.md:245 vs 03-详细设计.md:434 | 同一约束两种相反表述**：02 写"热榜 30 分钟（**勿高于** newsnow 默认缓存频率）"，03 写"不得低于 30"。建议 02 改为"采集周期不得低于 30 分钟（比 newsnow 缓存更频繁无意义）"。

**[M-4] 02-概要设计.md:44-45 | 五层架构图中 L1 标"五类信源 hotlist/keyword/creator/rss/competitor"，紧邻的 L2 标"五路 hotlist/api/rss/crawl/sidecar"**，两个"五"极易混淆。建议在图中加一行"kind（采什么）vs method（怎么采）"。

**[M-5] 03-详细设计.md:263-271 | 五路采集器均未写"每路可独立启用"（FR-INT-03 验收项） | 建议 yaml 增 `ic.collectors: {hotlist: true, api: true, rss: true, crawl: true, sidecar: false}`，注册表按此过滤。

**[M-6] 03-详细设计.md:297 | robots 缓存 24h 无主动失效入口 | 建议提供 `POST /api/v1/media/ic/robots/invalidate?host=` 或缓存 key 带版本号。

**[M-7] 03-详细设计.md:502-508 | 结构化日志示例无 `trace_id` / `run_id` | M2 的 `run_id` 已定义却没进日志，排障时无法串起一次采集。建议在日志字段中强制 `run_id`、`source_id`、`intel_id`。

**[M-8] 03-详细设计.md:419 | 脱敏只写了 headers 的三项 | 建议增补：`Cookie`、`X-CSRF-Token`、URL 中的 `token/access_token/signature` query 参数，并写成 `pkg/log` 的统一脱敏 hook 而非各点手写。

**[M-9] 02-概要设计.md:240-248 | 容量估算不自洽**：证据快照 200MB/天，但若存完整 HTML（单条 200KB × 3000-8000 条）应为 600MB-1.6GB/天。建议给单条快照上限（建议 ≤64KB，截断并记 `raw_truncated`）后重算。

**[M-10] 02-概要设计.md:226-236 | ADR 缺两条**：建议增 ADR-IC-09「去重选型 SimHash（理由见 B-9）」、ADR-IC-10「若参考 TrendRadar 权重模型，姿态为 Reimpl，不引代码」（00-shared §3.2 红线）。

**[M-11] 01-需求规格.md:66 | FR-INT-14 提到"可选 MCP ≤3 tool（search/get/export）"，但 02/03/04 全文无 MCP | 建议在 02 §6 增补 MCP tool 定义，或在 01 中标注"V1 不提供 MCP，降级为 REST"。

**[M-12] 01-需求规格.md:109、113 | §5.2 引入 `yt-dlp` 未在任何选型表/NOTICE 登记 | 违反 00-shared §6"新增任何第三方依赖前逐条确认"。建议删除（YouTube 已用 Data API v3）或补登记（yt-dlp 许可证需 API 实测，且其 ToS 明确限制规避技术措施）。

**[M-13] 03-详细设计.md:269、178 | 引入 `gofeed`、02 §5 引入 `go-readability`、04 §4.5 引入 `jieba/HanLP`，以及 ULID 库、embedding 模型权重——全部未登记 | 建议在 00-shared/02-tech-selection §2 增行，并逐项做 SPDX 实测（`gofeed` MIT、`gojieba` 需复核其绑定的 cppjieba 许可、embedding 模型权重许可需单独确认）。

**[M-14] 03-详细设计.md:270 | crawl 路"保留 Attribution（NFR-14）"无落地位置 | 建议：① 仓库 `NOTICE` 登记；② 证据包 `meta.json` 增 `"extractor": {"name":"Crawl4AI","license":"Apache-2.0"}`；③ 前端"关于"页展示 attribution。

**[M-15] 03-详细设计.md:435-441 | RSSHub 只写 `base_url` | 建议补纪律三条（AGPL 边界）：① 禁止 fork/嵌入/拷路由文件进主仓；② 若自建路由（修改 RSSHub），该修改须在独立仓库公开；③ 若本系统对外 SaaS 化，须开放该 RSSHub 服务源码。写进 03 §3.3 与 04 §8 许可门禁。

**[M-16] 02-概要设计.md:176-178 | newsnow/RSSHub/Crawl4AI 只说"自部署锁定版本"，未给版本标识 | 建议 M0 产出时记录 `<repo>@<commit-sha>` 与镜像 digest，写入 04 §6 依赖表。

**[M-17] 01-需求规格.md:110 | TikTok Research/Display API 需资质，未标"待资质" | 建议在 §5.2 备注栏标 `[待资质]`，并在 §8 依赖表增加"平台资质"行。

**[M-18] 01-需求规格.md:26、59 | PIPL 声明"不采集个人信息"，但 `author` 字段与证据快照原文可能含个人信息 | 建议：`author` 仅存公开昵称、禁止存平台用户 ID 与主页 URL；证据快照落盘前执行 PIPL 清洗（邮箱、手机号、身份证样式的正则替换），并在 03 §6 写明。

**[M-19] 03-详细设计.md:407-415 | 证据包 ZIP 无导出审计与防替换 | 建议 ZIP 内 `meta.json` 增 `exported_by`、`exported_at`、`zip_sha256`；导出动作写操作审计。

**[M-20] 03-详细设计.md:125、145 | Ent 主键前缀（`src_`/`it_`/`cl_`/`ev_`）未写生成方式 | 建议统一在 `schema/` 各文件用 `DefaultFunc` 或 `BeforeCreate` Hook：`id = "<prefix>_" + ulid.Make().String()`，字段 `field.String().Unique().Immutable().MaxLen(30)`，并在文档注明 ULID 库与单调性保证。

**[M-21] 03-详细设计.md:154 | `gin(entities)` / `fulltext(title)` 未给 Ent 注解或 Atlas 迁移写法 | 见 B-19；至少注明"该类索引需 Atlas 自定义迁移，Ent schema 中仅声明普通索引占位"。

**[M-22] 04-开发进度.md:64 | "Atlas 迁移脚本确认"未给方式 | 建议明确用 versioned migration（`app/admin/internal/data/ent/migrate/migrations/`），`atlas migrate diff --env ent`，并禁用在生产直接 `Schema.Create`。

**[M-23] 04-开发进度.md:19 vs 01-需求规格.md:155 | 决策阻塞项不一致**：04 写 Q4/Q5/Q8，01 写 Q1/Q4/Q5/Q6/Q8。建议 04 补 Q1、Q6 并说明是否阻塞 IC（Q6 FreshRSS 已由 ADR-IC-08 处置，可标已闭环）。

**[M-24] 04-开发进度.md:69 | Wire providers 只提一句 | 建议列出 IC 的 ProviderSet：`biz/ic` 的 `NewCollectUsecase`、`NewNormalizeUsecase`、`NewGate`、`NewDedup`，`data/ic` 的 repo，`collector` 的 registry，`worker/ic` 的 PeriodicTaskManager，并注明注入到 admin 进程。

**[M-25] 03-详细设计.md:535-547 | 落地检查清单缺 6 项 | 建议补：① 契约字段 `json_name` 与 snake_case 校验；② 时间字段 UTC + timestamptz 校验；③ NATS consumer 与 MsgId 去重窗口已配置；④ 熔断与 `paused_until` 生效；⑤ `pkg/retention` 清理任务已注册；⑥ 前端 TS 生成物已提交且未手改。

**[M-26] 02-概要设计.md:214-221 vs 04-开发进度.md:70 | 前端 4 个页面 vs seed 3 个菜单 | 建议 `/media/ic/intels/{id}` 作为 `/media/ic/intels` 的子菜单（FOLDER + MENU 成对），并在 seed 中显式列出 5 条记录（1 FOLDER + 4 MENU）。

**[M-27] 03-详细设计.md:531 | 回归测试"回放 100 条真实条目快照" | 该快照集不存在，建议 M3 同步产出 `testdata/ic/dedup_golden.jsonl`（含期望 cluster 归属），否则回归无法建立。

**[M-28] 03-详细设计.md:123-139 | Ent schema 与既有 40+ 张表同目录 | 建议表名统一 `intel_*` 前缀，schema 文件命名 `intel_item.go`/`intel_source.go`/...，避免与既有 `menu`/`user`/`task` 等语义冲突（`task.go` 已存在，注意与 asynq task 命名区分）。

**[M-29] 02-概要设计.md:162-168 | 回流（FR-INT-16）"更新 source 的选题权重字段"，但 `IntelSource` 无该字段 | 建议增 `weight float`（默认 1.0）并定义更新公式与边界（0.5-2.0，仅影响排序不影响启停）。

**[M-30] 03-详细设计.md:340 | "预计占总量的 10% 以内"无依据 | 建议改为可验证指标：`ic_dedup_level_total` 按 level 打点，M3 验收时导出实际占比回填文档。

---

## 四、契约违规清单

| # | 契约字段/规则 | IC 文档中的实际写法 | 应改为 |
|---|---|---|---|
| **C-1** | `api_version`（顶层必填，"1.0"） | 03 §2.2 Ent `IntelItem` **无该字段**，proto 未定义 | Ent 增 `api_version string`（默认 "1.0"）；proto 见 B-1；序列化前非空校验 |
| **C-2** | `intel_id`（`it_` 前缀） | Ent 用 `id`（02 §2.1/03 §2.2 写 `it_` + ULID，但字段名是 `id`） | 保留 Ent `id` 为内部主键，但必须给出映射 `id → intel_id`（B-5），禁止下游直接用 `id` |
| **C-3** | `metrics.rank_total` | 03 §2.2 metrics 只写 `rank/views/likes/comments + _as_of`，**漏 rank_total** | 补 `rank_total`；无榜取 **0**（契约 §0.2：0 表示未知）。注意 02-TT/03:31 写的是 `-1`（float），需回写 TT 与契约统一为 0 |
| **C-4** | `dedup.fingerprint`（契约是 **string**） | 03 §2.2 定义为 `uint64` | 存 4 段 smallint + `fingerprint_hex`（16 位小写 hex），输出用 hex 字符串（见 B-26） |
| **C-5** | `source.evidence{snapshot_path, raw_ref, http_headers}` | Ent 用独立 `EvidenceSnapshot` 表 + `evidence_id`，**映射未定义** | 明确映射：`snapshot_path ← raw_path`、`raw_ref ← "ev_"+id`、`http_headers ← 脱敏后的 http_headers`；禁止把原始 HTML 塞进消息（ADR-IC-04） |
| **C-6** | `entities[].type` 六类 `person\|org\|policy\|tech\|place\|topic` | 01 §3 FR-INT-08 写"人物/机构/地点/**事件**/话题"——`event` 不在契约枚举内，`policy`/`tech` 被漏 | 改为"人物 person / 机构 org / 政策 policy / 技术 tech / 地点 place / 话题 topic"；proto 用 `string + in:[]` 强校验 |
| **C-7** | 枚举一律小写下划线（§0.2） | `collect_method` 用 proto enum → 序列化为 `COLLECT_METHOD_HOTLIST`；`category=A_hot`、`gate=pass` 无法用 enum 表达 | 全部改 `string` + validate `in:`（见 B-1/B-3） |
| **C-8** | 存储一律 UTC RFC3339 带 Z（§0.2） | IC 四份文档**零处**提及时区；契约 §1 示例自身也写成 `+08:00` | IC 侧全部 UTC（B-15）；同时回写契约 §1 示例为 `2026-10-17T01:00:00Z` |
| **C-9** | ID 前缀表（§0.3）只登记 `it_`/`cl_`/`ev_` | 03 §2.1 给信源用了 **`src_`**，未登记 | 契约 §0.3 增行 `src_ \| IntelSource \| ① IC（内部实体，不进契约）`，或改用不带前缀的内部 UUID 避免混淆 |
| **C-10** | `source.collected_at` 语义 | 文档未区分"采集时间"与"上游发布时间"，NFR-IC-02 无法度量 | `collected_at` = 本地接收时间 UTC（必填）；新增 `published_at` = 上游发布时间（可空，需回写契约 §1 登记） |
| **C-11** | 跨系统只允许传七份契约结构（§0） | 02 §4.4/§10、FR-INT-16 消费 `mm.map.feedback.v1` 的 `PerformanceSnapshot`，该结构**未登记进 §1-§7**（仅 §8 表格提及，定义散落在 05-MAP/03 §1.6） | IC 侧注明"P2 前必须把 `PerformanceSnapshot` 登记进契约 §8 结构族并完成 SPDX/ToS 复核"，否则 IC 不得消费 |
| **C-12** | `source.collect_method` 五值枚举 | 02 §6.2 的 `supplement`（手贴正文续跑）无合法 method 值可填 | 手贴内容不新建 IntelItem，而是对既有条目 `UPDATE payload.text`，并置 `source.collect_method` 不变、增 `payload.supplemented=true`（需回写契约登记 `payload` 的新可选字段，走 §9 补丁流程） |
| **C-13** | `MM_INTEL` 保留策略 limits（按条数）+ 90d | 04 §4.1 只写"保留 90d"，未给条数上限；03/04 无 `max_msgs`/`max_bytes` | 补 `max_msgs: 2000000`、`max_bytes: 10GiB`（见 B-11）；并按 §8 表格的 `mm.intel.>` 建 subjects |
| **C-14** | `payload.lang` | 03 §2.2 只写 `zh-CN / en` | 改 BCP-47 + `und` 兜底（见 S-25），并在契约 §1 登记取值域 |
| **C-15** | `source.platform` / `region` 取值域 | 契约未定义枚举，IC 只写"weibo / bilibili / youtube…"与"cn / intl" | 在契约 §1 增"平台标识枚举表"（IC/TT/MAP 共用：`weibo\|zhihu\|baidu\|toutiao\|douyin\|bilibili\|xiaohongshu\|kuaishou\|youtube\|reddit\|x\|tiktok\|...`），`region ∈ {cn, intl, global}`（TT 已用 global，需对齐） |
| **C-16** | 契约 §0.2"禁止 `map[string]any`" | `source.evidence.http_headers` 本身是自由 map | 明确为 `map<string,string>` 且为**脱敏后**的受限集合（允许键白名单），并在 JSON Schema 中限定 `additionalProperties` 的 value 类型为 string |
| **C-17** | `dedup.cluster_id` 前缀 `cl_`（§0.3 已修正） | 03 §2.3 Ent 用 `cl_` ✅ 正确；但 02 §2 L52 写"保留多来源指针"，契约无该字段 | 说明"多来源指针通过 `cluster_id` 反查 `IntelItem` 集合获得，不冗余进契约"，避免下游要求加字段 |

---

## 五、缺失项清单（生产必需但文档未覆盖）

1. **幂等键的正式定义**（`dedup_key` 的构成、冲突策略、与 NFR-IC-03 的对应关系）——没有它，重复投递和数据重复无法从根上消除。
2. **并发模型**（asynq `TaskID` + `Unique` + 进程内 singleflight + errgroup 限流）——否则同一信源跨轮重叠，8 并发形同虚设且内存击穿。
3. **背压与丢弃策略**（inflight 上限、超限限速、丢弃计数指标）——采集快于归一化时必然 OOM 或写满磁盘。
4. **熔断与自动恢复**（`fail_streak` → `paused` → 冷却 → 恢复）——只有告警没有动作，被封的源会持续重试直至账号封禁。
5. **断点续爬**（热榜分页游标、API `page_token`、进程重启后的续采点持久化）——现在只有 RSS 的 ETag，其余三路重启即从头重采。
6. **去重缓存淘汰策略**（Redis 中 url_key / fingerprint / 24h 向量的 key 设计与 TTL）——不定义则 Redis 内存无限增长或误淘汰导致去重失效。
7. **时钟漂移与未来时间处理**（NTP、`published_at_suspect` 标记、窗口边界只用本地时间）——上游时间不可信会污染去重窗口与时序。
8. **失败重试矩阵**（哪些错误可重试、最大次数、退避基线、429 的 `Retry-After`）——现在三处文档给了三个不同的重试次数。
9. **死信表结构与重放入口**（含 `replayed_by` 审计）——错误可恢复时数据永久丢失。
10. **数据保留与清理任务**（evidence 90d、intel 365d、cluster 关闭、NATS 条数上限、`pkg/retention` 的 cron 参数）——NFR-IC-08 与磁盘风险缓解（02 §9）都依赖它。
11. **证据哈希链与只追加存储**（`prev_hash`/`chain_hash`/对象锁/链断裂告警）——SHA-256 单值无法防篡改，审计时不可自证。
12. **授权状态（authorization）的判据来源**——闸门规则依赖"素材授权状态"，但没有任何表或外部接口提供该状态，该规则永远不命中。
13. **账号池与轮询**（FR-INT-13 的多账号轮询）——只有限流没有账号载体，无法落地也无法做账号级熔断。
14. **导出与规则变更的审计日志**（谁导出证据、谁改了合规规则、谁合并了簇）——合规审计的必备追溯链。
15. **PIPL 清洗规则**（author 只存昵称、证据快照的邮箱/手机号/证件号正则脱敏）——否则"不采集个人信息"的声明与实现相悖。
16. **短链展开的 SSRF 防护**（私网 IP 黑名单、重定向上限、超时）——展开任意 URL 是典型的内网探测入口。
17. **NATS 消费者与投递语义定义**（durable、ack 策略、`MaxDeliver`、`Nats-Msg-Id` 去重窗口）——至少一次投递下没有它必然产生重复条目。
18. **限流的 Redis 原子性与多实例语义**（Lua 脚本 / `cl.throttle`）——多实例部署时限流完全失效。
19. **配置热加载与变更生效路径**（信源增删改后 asynq 周期任务如何同步、合规规则如何热更新）——否则改配置必须重启进程。
20. **容量上限与压测基线**（单条快照大小上限、日产上限、PG 表分区策略）——90 天 18GB 的估算建立在未定义上限的假设上。
21. **备份与恢复**（证据存储异地备份、PG 备份、迁移回滚方案）——取证链一旦丢失不可补录。
22. **LLM/embedding 成本帽在 IC 的落地**（`embedding_daily_cap`、云端调用总量硬熔断）——02 §9 提到"硬熔断"但 IC 无配置与实现。
23. **契约版本演进在 IC 的落地**（`api_version=1.0` 如何随契约 §9 升级、新旧 subject 并行、双写窗口）——否则首次契约升级就会级联故障。
24. **前端工作量与任务**（4 个 Quasar 页面 + TS 生成物 + Casbin）——04 排期完全隐形，是排期失守的直接原因。
25. **JSON Schema 的产出归属与生成方式**（必须 M1 产出、必须由 proto 生成）——契约门禁与双向兼容测试都依赖它，目前文件不存在。

[Agent ID: agent-f3965805-8fba-4ff5-bdbe-afd2668a695e]