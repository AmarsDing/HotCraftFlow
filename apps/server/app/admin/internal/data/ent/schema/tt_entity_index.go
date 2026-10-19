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

// EntityIndex 实体倒排索引（② TT 详细设计 §2 聚类 Step2 召回通道①）。
//
// 评审 B-1/A-1：
//   - 倒排结构：entity_key → topic_id 多对一映射（同一实体可属多话题）；
//   - 召回窗口：仅保留近 7d（A-1 T_window=7d）——由独立清理任务删除 indexed_at < now-7d 行；
//   - 实体来源：IntelItem.Payload.Entities 经 confidence≥0.6 过滤 + (type:value) 规范化。
type EntityIndex struct {
	ent.Schema
}

func (EntityIndex) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tt_entity_index",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("TT 实体倒排索引：Step2 召回 term→topic 关联（7d 窗口）"),
	}
}

// Fields of the EntityIndex.
func (EntityIndex) Fields() []ent.Field {
	return []ent.Field{
		field.String("entity_key").
			Comment("(type:value) 规范化实体串，如 'person:马云' / 'org:阿里巴巴'").
			NotEmpty(),

		field.String("topic_id").
			Comment("话题 ID（tt_topics.topic_id，tp_ 前缀）").
			NotEmpty(),

		field.String("intel_id").
			Comment("贡献该实体的情报 ID（ic_intel_items.intel_id，溯源用）").
			NotEmpty(),

		field.Time("indexed_at").
			Comment("索引建立时间（UTC；7d 窗口清理依据）"),
	}
}

// Edges of the EntityIndex.
func (EntityIndex) Edges() []ent.Edge {
	return nil
}

// Indexes of the EntityIndex.
func (EntityIndex) Indexes() []ent.Index {
	return []ent.Index{
		// 召回主路径：按 entity_key + indexed_at 查近 7d 的 topic_id 集合
		index.Fields("entity_key", "indexed_at"),
		// 清理路径：按 indexed_at 扫超期行
		index.Fields("indexed_at"),
		// 同一实体在同一话题的同一情报只索引一次（幂等兜底）
		index.Fields("entity_key", "topic_id", "intel_id").Unique(),
	}
}

// Mixin of the EntityIndex.
func (EntityIndex) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		appmixin.TenantID{},
	}
}
