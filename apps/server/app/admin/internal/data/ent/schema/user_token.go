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

// UserToken holds the schema definition for the UserToken entity.
// 用户令牌表：替代 Redis 缓存，存储 access/refresh 令牌以支持登出撤销与服务端校验。
type UserToken struct {
	ent.Schema
}

func (UserToken) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "user_tokens",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("用户令牌表"),
	}
}

// Fields of the UserToken.
func (UserToken) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("user_id").
			Comment("关联用户ID").
			Optional().
			Nillable(),

		field.String("token").
			Comment("令牌内容（access 为 JWT，refresh 为 UUID）").
			MaxLen(512).
			NotEmpty(),

		field.Enum("token_type").
			Comment("令牌类型").
			NamedValues(
				"Access", "ACCESS",
				"Refresh", "REFRESH",
				"Mfa", "MFA",
			),

		field.String("client_id").
			Comment("客户端标识（access 令牌用）").
			Default("").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("login_ip").
			Comment("登录IP").
			Default("").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("login_location").
			Comment("登录地点（IP归属地，qqwry查询填充）").
			Default("").
			MaxLen(128).
			Optional().
			Nillable(),

		field.String("user_agent").
			Comment("浏览器User-Agent").
			Default("").
			MaxLen(512).
			Optional().
			Nillable(),

		field.Time("expires_at").
			Comment("过期时间，nil 表示不过期").
			Optional().
			Nillable(),

		field.Bool("revoked").
			Comment("是否已撤销（Token黑名单语义）：0-否，1-是").
			Default(false),
	}
}

// Mixin of the UserToken.
func (UserToken) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		appmixin.TenantID{},
	}
}

// Edges of the UserToken.
func (UserToken) Edges() []ent.Edge {
	return nil
}

// Indexes of the UserToken.
func (UserToken) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token", "token_type").Unique(),
		index.Fields("user_id"),
		index.Fields("token"),
		index.Fields("user_id", "token_type", "revoked", "expires_at"),
		index.Fields("expires_at"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (UserToken) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
