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

// RoleMenu holds the schema definition for the RoleMenu entity.
type RoleMenu struct {
	ent.Schema
}

func (RoleMenu) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ap_org_role_menu",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("角色菜单关联表（含按钮 type=3）"),
	}
}

func (RoleMenu) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("role_id").
			Comment("角色ID"),

		field.Uint32("menu_id").
			Comment("菜单ID（含 type=3 按钮）"),
	}
}

func (RoleMenu) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateTime{},
		appmixin.TenantID{},
	}
}

func (RoleMenu) Edges() []ent.Edge {
	return nil
}

func (RoleMenu) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role_id"),
		index.Fields("menu_id"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (RoleMenu) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
