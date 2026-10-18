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

// TopicSource 话题 ↔ IntelItem 关联（文档 02-TT 详细设计 §1.2）。
//
// 评审增补：
//   - B-1：cluster_key（IC dedup.cluster_id 的归并硬键，Step1 直接命中归入）。
//   - B-1：moved_by / moved_at（人工移动溯源，配合 merge_lock）。
//   - S-17：contributed_at 是 platform_count 滑动 24h 窗口的统计基础，必须可查。
type TopicSource struct {
	ent.Schema
}

func (TopicSource) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tt_topic_sources",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("TT 话题来源关联：话题由哪些情报条目贡献、何时贡献、相似度"),
	}
}

// Fields of the TopicSource.
func (TopicSource) Fields() []ent.Field {
	return []ent.Field{
		field.String("topic_id").
			Comment("话题 ID（tt_topics.topic_id）").
			NotEmpty(),
		field.String("intel_id").
			Comment("情报 ID（ic_intel_items.intel_id，it_ 前缀）").
			NotEmpty(),

		field.String("cluster_key").
			Comment("IC 去重簇 ID（cl_ 前缀；B-1 Step1 硬键，命中直接归入本话题）").
			Optional().
			Nillable(),

		// A-5 breadth 的统计基础：pc(t) = distinct platform where contributed_at ∈ (t-24h, t]，
		// 平台取自 IntelItem.Source.Platform（冗余落本表，免回查 ic_intel_items）。
		field.String("platform").
			Comment("贡献平台（weibo/bilibili/youtube ……；A-5 breadth 滑动窗口统计维度）").
			NotEmpty(),

		field.Time("contributed_at").
			Comment("该条目归入话题的样本时间（UTC；S-17 滑动窗口统计基础）"),

		field.Float("similarity").
			Comment("归入时的相似度（Step1 硬键命中记 1.0）").
			Default(1.0),

		// —— B-1：人工移动溯源 ——
		field.String("moved_by").
			Comment("人工合并/拆分操作者，可空（空=自动聚类归入）").
			Optional().
			Nillable(),
		field.Time("moved_at").
			Comment("人工移动时间（UTC）").
			Optional().
			Nillable(),
	}
}

// Edges of the TopicSource.
func (TopicSource) Edges() []ent.Edge {
	return nil
}

// Indexes of the TopicSource.
func (TopicSource) Indexes() []ent.Index {
	return []ent.Index{
		// (topic_id, intel_id) 复合唯一：同一情报只归属话题一次
		index.Fields("topic_id", "intel_id").Unique(),
		// B-1 Step1 硬键查询路径
		index.Fields("cluster_key"),
		// S-17 滑动窗口统计路径
		index.Fields("topic_id", "contributed_at"),
	}
}

// Mixin of the TopicSource.
func (TopicSource) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
