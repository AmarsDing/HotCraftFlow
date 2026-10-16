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

// LoginFailCounter holds the schema definition for the LoginFailCounter entity.
type LoginFailCounter struct {
	ent.Schema
}

func (LoginFailCounter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "login_fail_counters",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("登录失败计数器表（IP与账户锁定独立计数）"),
	}
}

func (LoginFailCounter) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("target_type").
			Comment("目标类型：ip/user").
			NamedValues(
				"Ip", "IP",
				"User", "USER",
			),

		field.String("target_value").
			Comment("目标值（IP地址或用户名）").
			MaxLen(128).
			NotEmpty(),

		field.Int("fail_count").
			Comment("连续失败次数").
			Default(0),

		field.Time("locked_until").
			Comment("锁定截止时间（NULL表示未锁定）").
			Optional().
			Nillable(),
	}
}

func (LoginFailCounter) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.UpdateTime{},
	}
}

func (LoginFailCounter) Edges() []ent.Edge {
	return nil
}

func (LoginFailCounter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("target_type", "target_value").Unique(),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (LoginFailCounter) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
