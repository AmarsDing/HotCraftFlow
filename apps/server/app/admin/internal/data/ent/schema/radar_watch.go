package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"
)

// RadarWatch holds the schema definition for the RadarWatch entity.
// 单用户对某 GitHub 仓库的盯盘记录（按 user_id 隔离）。
type RadarWatch struct {
	ent.Schema
}

func (RadarWatch) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "radar_watches",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("GitHub 热门追踪盯盘表（按用户隔离）"),
	}
}

// Fields of the RadarWatch.
func (RadarWatch) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("user_id").
			Comment("用户ID"),

		field.String("full_name").
			Comment("仓库 full_name（owner/repo）").
			MaxLen(200),

		field.Int64("baseline_stars").
			Comment("盯盘时基线星标数（增量=当前-基线）").
			Default(0),

		field.Int64("stars").
			Comment("最近一次已知星标数").
			Default(0).
			Optional().
			Nillable(),

		field.Int64("stars_delta").
			Comment("相对基线的星标增量").
			Default(0).
			Optional().
			Nillable(),

		field.Uint32("unread_count").
			Comment("未读变更事件数").
			Default(0).
			Optional().
			Nillable(),

		field.String("html_url").
			Comment("仓库链接").
			Optional().
			Nillable(),

		field.JSON("sparkline", []int64{}).
			Comment("星标增量迷你趋势（最近 7 个点）").
			Optional(),
	}
}

// Indexes of the RadarWatch.
func (RadarWatch) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "full_name").Unique(),
		index.Fields("user_id"),
	}
}

// Mixin of the RadarWatch.
func (RadarWatch) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

// Policy 隐私策略占位：行级归属由 repo 以 user_id 条件强制过滤（见 RadarWatchRepo）。
func (RadarWatch) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
