package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// DataPermission holds the schema definition for the DataPermission entity.
type DataPermission struct {
	ent.Schema
}

func (DataPermission) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "data_permissions",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("数据权限规则表"),
	}
}

func (DataPermission) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("role_id").
			Comment("角色ID"),

		field.String("name").
			Comment("规则名称").
			Default("").
			MaxLen(128),

		field.String("field").
			Comment("数据字段名").
			Default("").
			MaxLen(64),

		field.Enum("scope").
			Comment("范围：all/dept/custom/self").
			NamedValues(
				"All", "ALL",
				"Dept", "DEPT",
				"Custom", "CUSTOM",
				"Self", "SELF",
			).
			Default("SELF"),

		field.String("rule_json").
			Comment("自定义规则JSON").
			Optional().
			Nillable(),
	}
}

func (DataPermission) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.SwitchStatus{},
		appmixin.TenantID{},
	}
}

func (DataPermission) Edges() []ent.Edge {
	return nil
}

func (DataPermission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role_id"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (DataPermission) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
