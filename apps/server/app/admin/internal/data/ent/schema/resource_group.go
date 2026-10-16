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

// ResourceGroup holds the schema definition for the ResourceGroup entity.
type ResourceGroup struct {
	ent.Schema
}

func (ResourceGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "resource_groups",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("资源组表"),
	}
}

func (ResourceGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("资源组名称").
			Unique().
			MaxLen(128),

		field.String("description").
			Comment("描述").
			Default("").
			MaxLen(512).
			Optional().
			Nillable(),
	}
}

func (ResourceGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.SwitchStatus{},
	}
}

func (ResourceGroup) Edges() []ent.Edge {
	return nil
}

func (ResourceGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (ResourceGroup) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
