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

// AdminLoginRestriction holds the schema definition for the AdminLoginRestriction entity.
type AdminLoginRestriction struct {
	ent.Schema
}

func (AdminLoginRestriction) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "admin_login_restrictions",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("后台登录限制表"),
	}
}

// Fields of the AdminLoginRestriction.
func (AdminLoginRestriction) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("target_type").
			Comment("限制目标类型（IP/USER）").
			NamedValues(
				"Ip", "IP",
				"User", "USER",
			).
			Default("IP").
			Optional().
			Nillable(),

		field.String("target_value").
			Comment("限制目标值（IP地址或用户名，按 target_type 解释）").
			Optional().
			Nillable(),

		field.Enum("action").
			Comment("动作（BLOCK阻断/ALLOW放行）").
			NamedValues(
				"Block", "BLOCK",
				"Allow", "ALLOW",
			).
			Default("BLOCK").
			Optional().
			Nillable(),

		field.String("reason").
			Comment("限制原因").
			Optional().
			Nillable(),

		field.Time("expire_time").
			Comment("过期时间（可空=永久有效）；过期规则不参与登录判定").
			Optional().
			Nillable(),

		field.Uint64("hit_count").
			Comment("命中次数（登录判定命中时异步累加，统计用途）").
			Default(0).
			Optional(),

		field.Time("last_hit_time").
			Comment("最近命中时间").
			Optional().
			Nillable(),
	}
}

// Indexes of the AdminLoginRestriction.
func (AdminLoginRestriction) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_type", "target_value", "action").Unique(),
	}
}

// Mixin of the AdminLoginRestriction.
func (AdminLoginRestriction) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.SwitchStatus{},
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (AdminLoginRestriction) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
