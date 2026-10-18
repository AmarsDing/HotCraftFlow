package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// MiaEventCard 事件卡（FR-MIA-05，详细设计 §1.3）：多源转载合并为一条事件，
// 保留全部来源指针；source_count 按不同域名去重计数（影响 source_weight）。
type MiaEventCard struct {
	ent.Schema
}

func (MiaEventCard) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mia_event_cards", Charset: "utf8mb4", Collation: "utf8mb4_bin"},
		entsql.WithComments(true),
		schema.Comment("事件卡：多源转载合并后的单一事件（FR-MIA-05）"),
	}
}

func (MiaEventCard) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_id").
			Comment("事件卡 ID，ec_ 前缀 ULID（契约 §0.3）").
			Unique().
			NotEmpty(),
		field.String("title").
			Comment("事件标题"),
		field.Text("summary").
			Comment("事件摘要").
			Optional().
			Nillable(),
		field.Time("occurred_at").
			Comment("事件发生时间 UTC（非采集时间）"),
		field.String("topic_id").
			Comment("关联 TT 话题 tp_，可空").
			Optional().
			Nillable(),
		field.JSON("source_intel_ids", []string{}).
			Comment("全部来源指针（it_ id 列表，合并后仍保留）"),
		field.Int("source_count").
			Comment("转载源数（按不同域名去重计数，影响 source_weight）").
			Default(1),
		field.JSON("domains", []string{}).
			Comment("来源去重域名列表（source_weight 的输入）").
			Optional(),
		field.JSON("entities", []map[string]any{}).
			Comment("抽取实体（type/value/confidence）").
			Optional(),
		field.Enum("category").
			Comment("事件类别：policy|company|macro|tech|crypto").
			NamedValues(
				"Policy", "policy",
				"Company", "company",
				"Macro", "macro",
				"Tech", "tech",
				"Crypto", "crypto",
			),
		field.String("lang").
			Comment("语言，如 en/zh").
			Default("en"),
		field.Enum("merged_by").
			Comment("聚类方式：auto|manual").
			NamedValues(
				"Auto", "auto",
				"Manual", "manual",
			).
			Default("auto"),
		field.Bool("split_locked").
			Comment("人工干预后锁定（锁定卡不再被自动聚类吞并）").
			Default(false),
	}
}

func (MiaEventCard) Edges() []ent.Edge { return nil }

func (MiaEventCard) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("occurred_at"),
		index.Fields("topic_id"),
		index.Fields("category"),
		// 聚类窗口查询：近 24h 且未被锁定的卡
		index.Fields("occurred_at", "split_locked"),
	}
}

func (MiaEventCard) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
