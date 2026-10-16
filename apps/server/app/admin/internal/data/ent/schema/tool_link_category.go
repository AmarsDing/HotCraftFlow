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

// ToolLinkCategory holds the schema definition for the ToolLinkCategory entity.
type ToolLinkCategory struct {
	ent.Schema
}

func (ToolLinkCategory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tool_link_categories",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("工具链接分类表"),
	}
}

// Fields of the ToolLinkCategory.
func (ToolLinkCategory) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("分类名称（唯一，如 监控/日志/文档）").
			MaxLen(64).
			Unique().
			Optional().
			Nillable(),

		field.String("icon").
			Comment("Lucide 图标名（如 wrench）").
			MaxLen(128).
			Optional().
			Nillable(),

		field.String("description").
			Comment("描述").
			MaxLen(512).
			Optional().
			Nillable(),

		field.Int32("sort").
			Comment("排序号，越小越靠前").
			Default(0).
			Optional().
			Nillable(),
	}
}

// Indexes of the ToolLinkCategory.
func (ToolLinkCategory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sort"),
	}
}

// Mixin of the ToolLinkCategory.
func (ToolLinkCategory) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.SwitchStatus{},
	}
}

// Policy 隐私策略占位：与 ToolLink 一致，行级过滤由 repo 手动条件与上层鉴权承担。
func (ToolLinkCategory) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
