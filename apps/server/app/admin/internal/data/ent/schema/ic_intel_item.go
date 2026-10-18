package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-utils/entgo/mixin"

	contract "framework/pkg/mediacontract"
)

// IntelItem 情报条目（① IC 情报收集系统的核心实体，契约 §1 / 文档 01-IC/03 §2.2）。
// 存储视角：Ent 是内部持久化视图（契约 §0.4），字段名与契约对齐，经 toContract 映射对外。
// 评审修正：B-6（dedup_key 幂等键）、B-26（SimHash 拆 4 段 smallint）、S-15（published_at）、
// S-16（published_at_suspect）、C-3（rank_total）、C-14（lang BCP-47 + und）。
type IntelItem struct {
	ent.Schema
}

func (IntelItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_intel_items",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("情报条目：采集后归一化的原始素材（契约 IntelItem 的内部持久化视图）"),
	}
}

// Fields of the IntelItem.
func (IntelItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("情报 ID，it_ 前缀 ULID（契约 intel_id，评审 M-20 统一 Prefix+ULID）").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewIntelItemID),

		field.String("api_version").
			Comment("契约版本（契约 §0.2，恒为 1.0；演进走契约 §9）").
			MaxLen(8).
			Default("1.0"),

		// —— source 块 ——
		field.String("platform").
			Comment("来源平台标识（契约 §1 平台枚举表，如 weibo/x/reddit）").
			MaxLen(32),
		field.String("region").
			Comment("区域：cn|intl|global（契约 §1 区域枚举）").
			MaxLen(8),
		field.String("collect_method").
			Comment("采集方法：hotlist|api|rss|crawl|sidecar（契约 §1，不用 enum 避免跨系统魔法值）").
			MaxLen(16),
		field.String("source_url").
			Comment("来源 URL，可空（如手动录入）").
			Optional().
			Nillable(),
		field.String("author").
			Comment("作者公开昵称，可空；禁止存平台用户 ID 与主页 URL（PIPL，评审 M-18）").
			MaxLen(128).
			Optional().
			Nillable(),
		field.Time("collected_at").
			Comment("本地接收时间 UTC（必填，采集端 clock.Now().UTC() 生成，禁止用上游时间兜底）"),
		field.Time("published_at").
			Comment("上游发布时间 UTC，可空（评审 S-15；缺失时不计 NFR-IC-02 指标，不伪造）").
			Optional().
			Nillable(),
		field.Bool("published_at_suspect").
			Comment("发布时间可疑标记：published_at > collected_at + 5min 时置真（评审 S-16），保留原值").
			Default(false),

		// —— payload 块 ——
		field.String("title").
			Comment("标题").
			MaxLen(512).
			Default(""),
		field.Text("text").
			Comment("正文/摘要（可很长）").
			Optional().
			Nillable(),
		field.JSON("media_refs", []string{}).
			Comment("媒体引用：asset:// 可加工；https:// 须先下载（契约 §1 必须区分）").
			Optional(),
		field.String("lang").
			Comment("BCP-47 语言码，und 兜底（评审 C-14/S-25）").
			MaxLen(16).
			Default("und"),

		// —— entities（JSON：[{type,value,confidence}]）——
		field.JSON("entities", []map[string]any{}).
			Comment("抽取实体列表；confidence < 0.6 不进 MIA 实体链接（契约 §1）").
			Optional(),

		// —— category ——
		field.String("category").
			Comment("分类：A_hot|B_material|C_market|D_compliance（契约 §1 路由）").
			MaxLen(16),

		// —— metrics ——
		field.Int32("rank").
			Comment("榜单排名；无榜 = 0（契约 §0.2：0 与 null 语义不同）").
			Default(0),
		field.Int32("rank_total").
			Comment("榜单总条目数；无榜 = 0（评审 C-3：uint 不可表 null，0 表示未知）").
			Default(0),
		field.Int64("views").
			Comment("阅读/播放").
			Default(0),
		field.Int64("likes").
			Comment("点赞").
			Default(0),
		field.Int64("comments").
			Comment("评论").
			Default(0),
		field.Time("metrics_as_of").
			Comment("指标快照时间 UTC（时变数据必须带时间，契约 §1 metrics._as_of）").
			Optional().
			Nillable(),

		// —— dedup 块（评审 B-6/B-26）——
		field.String("url_key").
			Comment("URL 归一化键 sha256 hex（03 §4.1：去追踪参数/尾斜杠/大小写，短链先展开）").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Int64("cycle_bucket").
			Comment("采集周期桶：floor(unix(collected_at)/(interval*60))*(interval*60)（评审 B-6）").
			Default(0),
		field.String("dedup_key").
			Comment("幂等键 sha256(url_key|source_id|cycle_bucket)（唯一；冲突走 ON CONFLICT 更新，满足 NFR-IC-03）").
			MaxLen(64).
			Unique().
			NotEmpty(),
		field.Bool("fingerprint_valid").
			Comment("指纹是否稳定：tokens<40 或文本过短时为 false，L2 跳过直进 L3（评审 B-9）").
			Default(false),
		field.Int16("fp_b0").
			Comment("SimHash 位段 [63:48]（pigeonhole 倒排，评审 B-26）").
			Default(0),
		field.Int16("fp_b1").
			Comment("SimHash 位段 [47:32]").
			Default(0),
		field.Int16("fp_b2").
			Comment("SimHash 位段 [31:16]").
			Default(0),
		field.Int16("fp_b3").
			Comment("SimHash 位段 [15:0]").
			Default(0),
		field.String("fingerprint_hex").
			Comment("SimHash 16 位小写 hex（契约 dedup.fingerprint 是 string，评审 C-4）").
			MaxLen(16).
			Optional().
			Nillable(),
		field.String("embedding_id").
			Comment("向量库引用（不存向量本体，03 §4.3；V1 用 Redis VSS / pgvector key）").
			MaxLen(64).
			Optional().
			Nillable(),
		field.String("cluster_id").
			Comment("聚类 ID，cl_ 前缀 ULID（契约 §0.3；多来源指针经 cluster_id 反查获得，评审 C-17）").
			MaxLen(30).
			Optional().
			Nillable(),

		// —— compliance ——
		field.String("compliance_gate").
			Comment("合规闸门：pass|review|block（闸门无旁路，评审 B-16）").
			MaxLen(8).
			Default("review"),
		field.JSON("compliance_reasons", []string{}).
			Comment("闸门命中原因与规则 ID（可解释，合规审计要求）").
			Optional(),

		// —— workability ——
		field.Float("workability_score").
			Comment("可加工性评分 [0,1]（03 B-18 公式；MME 准入阈值由 IC 配置，评审 S-26）").
			Default(0),
		field.String("workability_reason").
			Comment("评分理由：素材不足|无明确观点|时长不足|语言不支持（必填）").
			MaxLen(255).
			Optional().
			Nillable(),

		// —— 关联 ——
		field.String("source_id").
			Comment("信源 ID（ic_intel_sources 外键，src_ 前缀）").
			MaxLen(30).
			Optional().
			Nillable(),
		field.String("evidence_id").
			Comment("取证快照 ID（ic_evidence_snapshots 外键，ev_ 前缀；B-8 先取证后归一）").
			MaxLen(30).
			Optional().
			Nillable(),

		// —— 发布状态（评审 B-25：NATS 降级重试）——
		field.String("publish_state").
			Comment("发布状态：pending|published|failed（NATS 不可用落 pending，republish 每 30s 重试 ≤1h）").
			MaxLen(12).
			Default("published"),

		field.JSON("raw_payload", map[string]any{}).
			Comment("归一化原文附加片段（调试回放用，非契约；不得塞入完整 HTML）").
			Optional(),

		// —— 原始片段（评审 B-21：NATS 载荷上限 256KB，超限截断）——
		field.Int64("raw_size").
			Comment("采集原始片段字节数（内存上限 4MB 丢弃）").
			Default(0),
		field.Bool("raw_truncated").
			Comment("原始片段是否被截断").
			Default(false),
	}
}

// Indexes of the IntelItem.
func (IntelItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "collected_at"),
		index.Fields("url_key"),
		index.Fields("cluster_id"),
		index.Fields("category"),
		index.Fields("compliance_gate"),
		index.Fields("source_id"),
		index.Fields("collected_at"),
		index.Fields("publish_state"),
		// pigeonhole 倒排（评审 B-26）：汉明 ≤3 时必有一段相同，四组候选并集后精算
		index.Fields("fp_b0", "fp_b1"),
		index.Fields("fp_b1", "fp_b2"),
		index.Fields("fp_b2", "fp_b3"),
		index.Fields("fp_b0", "fp_b3"),
	}
}

// Edges of the IntelItem.
func (IntelItem) Edges() []ent.Edge {
	return nil
}

// Mixin of the IntelItem.
func (IntelItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the IntelItem.
func (IntelItem) Hooks() []ent.Hook {
	return nil
}
