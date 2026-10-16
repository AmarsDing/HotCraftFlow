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

// CasbinRule holds the schema definition for the CasbinRule entity.
type CasbinRule struct {
	ent.Schema
}

func (CasbinRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "casbin_rule",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("Casbin权限规则表"),
	}
}

func (CasbinRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("ptype").
			Comment("策略类型：p/g").
			MaxLen(12).
			NotEmpty(),

		field.String("v0").
			Comment("主体（角色ID/用户ID）").
			MaxLen(256).
			Default(""),

		field.String("v1").
			Comment("资源（API路径）").
			MaxLen(256).
			Default(""),

		field.String("v2").
			Comment("动作（HTTP方法）").
			MaxLen(256).
			Default(""),

		field.String("v3").
			Comment("扩展字段").
			MaxLen(256).
			Default("").
			Optional().
			Nillable(),

		field.String("v4").
			Comment("扩展字段").
			MaxLen(256).
			Default("").
			Optional().
			Nillable(),

		field.String("v5").
			Comment("扩展字段").
			MaxLen(256).
			Default("").
			Optional().
			Nillable(),
	}
}

func (CasbinRule) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
	}
}

func (CasbinRule) Edges() []ent.Edge {
	return nil
}

func (CasbinRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ptype", "v0", "v1", "v2").Unique(),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (CasbinRule) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
