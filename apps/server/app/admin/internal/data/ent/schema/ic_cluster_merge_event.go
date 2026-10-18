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

// ClusterMergeEvent 簇合并事件（01-IC/03 §4.4 + 评审 B-14）。
// 多 cluster 被合并时落事件：单事务内完成成员迁移 + member_count 重算 + 本表写入（评审 S-3）。
type ClusterMergeEvent struct {
	ent.Schema
}

func (ClusterMergeEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_cluster_merge_events",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("簇合并事件：from → to 的合并审计"),
	}
}

// Fields of the ClusterMergeEvent.
func (ClusterMergeEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("事件 ID（裸 ULID，内部实体）").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.ULID),

		field.String("from_cluster_id").
			Comment("被并入的簇 ID").
			MaxLen(30),
		field.String("to_cluster_id").
			Comment("主簇 ID").
			MaxLen(30),
		field.String("reason").
			Comment("合并理由（命中级 / 人工理由）").
			MaxLen(255).
			Optional().
			Nillable(),
		field.Int32("member_count").
			Comment("迁移的成员数").
			Default(0),
	}
}

// Indexes of the ClusterMergeEvent.
func (ClusterMergeEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("to_cluster_id"),
		index.Fields("from_cluster_id"),
	}
}

// Edges of the ClusterMergeEvent.
func (ClusterMergeEvent) Edges() []ent.Edge {
	return nil
}

// Mixin of the ClusterMergeEvent.
func (ClusterMergeEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
