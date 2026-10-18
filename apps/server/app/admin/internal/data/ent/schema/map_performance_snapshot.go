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

// MapPerformanceSnapshot 表现回流快照（⑤ MAP，FR-PUB-08 / 详细设计 §1.6 / 契约 00-shared §8.3）。
// 回灌 ② TT（选题反馈）/ ① IC（信源权重）/ ④ MME（变体效果）。
// ⚠️ V1 仅落表建结构：拉取 worker 与 NATS 回流（mm.map.feedback.v1）属 Phase 3（M6）。
type MapPerformanceSnapshot struct {
	ent.Schema
}

func (MapPerformanceSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "map_performance_snapshots",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MAP 表现数据回流快照（结构预留，拉取与回流管道 Phase 3）"),
	}
}

// Fields of the MapPerformanceSnapshot.
func (MapPerformanceSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("snapshot_id").
			Comment("快照 ID，ps_ 前缀 ULID（契约 00-shared §0.3）").
			Unique().
			NotEmpty(),

		field.String("receipt_id").
			Comment("关联发布回执（pr_ 前缀）").
			Optional().
			Nillable(),

		field.String("dispatch_unit_id").
			Comment("回指分发单元（du_ 前缀，形成 发布→表现→反哺选题 闭环）").
			Optional().
			Nillable(),

		field.String("variant_id").
			Comment("回指 MME 变体（v_ 前缀）").
			Optional().
			Nillable(),

		field.String("platform_key").
			Comment("平台").
			NotEmpty(),

		field.Int64("views").
			Comment("播放/阅读").
			Default(0),

		field.Int64("likes").
			Comment("点赞").
			Default(0),

		field.Int64("comments").
			Comment("评论").
			Default(0),

		field.Int64("shares").
			Comment("分享（V1.1 契约补登）").
			Default(0),

		field.Int64("favorites").
			Comment("收藏（V1.1 契约补登）").
			Default(0),

		field.Float("completion_rate").
			Comment("完播率 [0,1]；未知用 0 不伪造").
			Default(0),

		field.Time("fetched_at").
			Comment("抓取该快照的时间，UTC（表现数时变，必须带时间）"),

		field.String("candidate_id").
			Comment("回指选题（tc_ 前缀，可空）").
			Optional().
			Nillable(),
	}
}

// Indexes of the MapPerformanceSnapshot.
func (MapPerformanceSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("receipt_id"),
		index.Fields("platform_key"),
		index.Fields("fetched_at"),
	}
}

// Edges of the MapPerformanceSnapshot.
func (MapPerformanceSnapshot) Edges() []ent.Edge {
	return nil
}

// Mixin of the MapPerformanceSnapshot.
func (MapPerformanceSnapshot) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
