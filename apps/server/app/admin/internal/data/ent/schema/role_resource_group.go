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

// RoleResourceGroup 角色与资源组的关联表。
// 角色勾选资源组 → 展开为组内所有 API 资源 → 生成 Casbin 策略 (role_id, api_path, method)。
type RoleResourceGroup struct {
	ent.Schema
}

func (RoleResourceGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ap_org_role_resource_group",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("角色资源组关联表（API 权限通过资源组聚合）"),
	}
}

func (RoleResourceGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("role_id").
			Comment("角色ID"),

		field.Uint32("resource_group_id").
			Comment("资源组ID"),
	}
}

func (RoleResourceGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateTime{},
		appmixin.TenantID{},
	}
}

func (RoleResourceGroup) Edges() []ent.Edge {
	return nil
}

func (RoleResourceGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role_id"),
		index.Fields("resource_group_id"),
		index.Fields("role_id", "resource_group_id").Unique(),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (RoleResourceGroup) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
