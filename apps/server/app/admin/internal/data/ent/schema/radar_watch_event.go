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

// RadarWatchEvent holds the schema definition for the RadarWatchEvent entity.
// 盯盘仓库的变更事件（星标跃升 / Release / 描述变更），按用户隔离。
type RadarWatchEvent struct {
	ent.Schema
}

func (RadarWatchEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "radar_watch_events",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("GitHub 热门追踪盯盘变更事件表（按用户隔离）"),
	}
}

// Fields of the RadarWatchEvent.
func (RadarWatchEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("user_id").
			Comment("用户ID"),

		field.String("full_name").
			Comment("仓库 full_name（owner/repo）").
			MaxLen(200),

		field.Enum("kind").
			Comment("事件类型").
			NamedValues(
				"Unknown", "UNKNOWN",
				"Stars", "STARS",
				"Release", "RELEASE",
				"Description", "DESCRIPTION",
			).
			Default("STARS"),

		field.String("message").
			Comment("事件描述（展示文案）").
			MaxLen(500),

		field.Int64("delta").
			Comment("本次变更增量（星标数）").
			Default(0).
			Optional().
			Nillable(),

		field.Bool("read").
			Comment("是否已读").
			Default(false),
	}
}

// Indexes of the RadarWatchEvent.
func (RadarWatchEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "read"),
		index.Fields("user_id", "full_name"),
		index.Fields("create_time"),
	}
}

// Mixin of the RadarWatchEvent.
func (RadarWatchEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

// Policy 隐私策略占位：行级归属由 repo 以 user_id 条件强制过滤（见 RadarWatchEventRepo）。
func (RadarWatchEvent) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
