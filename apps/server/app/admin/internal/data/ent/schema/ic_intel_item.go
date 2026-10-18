package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// IntelItem 情报条目（① IC 情报收集系统的核心实体，契约 §1 / 文档 01-IC）。
// 字段对齐 mediacontract.IntelItem：source/payload/entities/category/metrics/dedup/compliance/workability。
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
		schema.Comment("情报条目：从国内外平台采集后归一化的原始素材"),
	}
}

// Fields of the IntelItem.
func (IntelItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("intel_id").
			Comment("情报 ID，it_ 前缀 ULID（契约 §0.2）").
			Unique().
			NotEmpty(),

		field.String("api_version").
			Comment("契约版本，如 v1").
			Default("v1"),

		// —— source 块 ——
		field.String("platform").
			Comment("来源平台标识，如 weibo/x/reddit"),
		field.String("region").
			Comment("区域：domestic|overseas").
			Default("domestic"),
		field.Enum("collect_method").
			Comment("采集方法：hotlist|api|rss|crawl|sidecar").
			NamedValues(
				"Hotlist", "hotlist",
				"Api", "api",
				"Rss", "rss",
				"Crawl", "crawl",
				"Sidecar", "sidecar",
			),
		field.String("source_url").
			Comment("来源 URL，可空（如手动录入）").
			Optional().
			Nillable(),
		field.String("author").
			Comment("作者/账号，可空").
			Optional().
			Nillable(),
		field.Time("collected_at").
			Comment("采集时间，UTC"),

		// —— payload 块 ——
		field.String("title").
			Comment("标题，可空").
			Optional().
			Nillable(),
		field.Text("text").
			Comment("正文/摘要").
			Optional().
			Nillable(),
		field.JSON("media_refs", []string{}).
			Comment("媒体引用（asset:// 路径）").
			Optional(),
		field.String("lang").
			Comment("语言，如 zh-CN/en").
			Default("zh-CN"),

		// —— entities（JSON：type/value/confidence）——
		field.JSON("entities", []map[string]any{}).
			Comment("抽取出的实体列表").
			Optional(),

		// —— category ——
		field.Enum("category").
			Comment("分类：A_hot|B_material|C_market|D_compliance").
			NamedValues(
				"Hot", "A_hot",
				"Material", "B_material",
				"Market", "C_market",
				"Compliance", "D_compliance",
			),

		// —— metrics ——
		field.Int("rank").
			Comment("榜单排名，可空").
			Optional().
			Nillable(),
		field.Int("rank_total").
			Comment("榜单总条目数，可空").
			Optional().
			Nillable(),
		field.Int64("views").
			Comment("阅读/播放，可空").
			Optional().
			Nillable(),
		field.Int64("likes").
			Comment("点赞，可空").
			Optional().
			Nillable(),
		field.Int64("comments").
			Comment("评论，可空").
			Optional().
			Nillable(),
		field.Time("metrics_as_of").
			Comment("指标快照时间，UTC，可空").
			Optional().
			Nillable(),

		// —— dedup ——
		field.String("url_key").
			Comment("URL 规范化键（评审 IC C-9）").
			Optional().
			Nillable(),
		field.String("fingerprint").
			Comment("SimHash 指纹（评审 IC B-9）").
			Optional().
			Nillable(),
		field.String("cluster_id").
			Comment("聚类 ID，cl_ 前缀 ULID（契约 §0.3，初版误用 tp_）").
			Optional().
			Nillable(),

		// —— compliance ——
		field.Enum("compliance_gate").
			Comment("合规闸门：pass|review|block").
			NamedValues(
				"Pass", "pass",
				"Review", "review",
				"Block", "block",
			).
			Default("pass"),
		field.JSON("compliance_reasons", []string{}).
			Comment("合规拦截/待审原因").
			Optional(),

		// —— workability ——
		field.Int("workability_score").
			Comment("可加工性评分 0-100").
			Default(0),
		field.String("workability_reason").
			Comment("评分理由，可空").
			Optional().
			Nillable(),

		// —— 取证 ——
		field.String("evidence_path").
			Comment("取证快照路径（不可篡改，评审 IC S-31），可空").
			Optional().
			Nillable(),
		field.String("evidence_hash").
			Comment("取证快照哈希，可空").
			Optional().
			Nillable(),

		// —— 状态 ——
		field.Enum("status").
			Comment("处理状态：raw|normalized|consumed|blocked").
			NamedValues(
				"Raw", "raw",
				"Normalized", "normalized",
				"Consumed", "consumed",
				"Blocked", "blocked",
			).
			Default("raw"),

		field.JSON("raw_payload", map[string]any{}).
			Comment("原始采集 JSON（含 http_headers 等）").
			SchemaType(map[string]string{dialect.MySQL: "json"}).
			Optional(),
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
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
