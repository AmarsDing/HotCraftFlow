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

// ToolLinkUserMark holds the schema definition for the ToolLinkUserMark entity.
// 记录单个用户对某工具链接的收藏与点击行为（收藏/常用工具数据源）。
type ToolLinkUserMark struct {
	ent.Schema
}

func (ToolLinkUserMark) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tool_link_user_marks",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("工具链接用户标记表（收藏/个人点击统计）"),
	}
}

// Fields of the ToolLinkUserMark.
func (ToolLinkUserMark) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("user_id").
			Comment("用户ID"),

		field.Uint32("tool_link_id").
			Comment("工具链接ID"),

		field.Bool("favorite").
			Comment("是否收藏").
			Default(false).
			Optional().
			Nillable(),

		field.Uint32("click_count").
			Comment("该用户点击次数").
			Default(0).
			Optional().
			Nillable(),

		field.Time("last_click_at").
			Comment("最近一次点击时间").
			Optional().
			Nillable(),
	}
}

// Indexes of the ToolLinkUserMark.
func (ToolLinkUserMark) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "tool_link_id").Unique(),
		index.Fields("tool_link_id"),
	}
}

// Mixin of the ToolLinkUserMark.
func (ToolLinkUserMark) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

// Policy 隐私策略占位：行级归属由 repo 以当前用户条件过滤。
func (ToolLinkUserMark) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
