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

// ToolLinkSsoTicket holds the schema definition for the ToolLinkSsoTicket entity.
// SSO 免密跳转一次性票据：点击 SSO 型链接时签发，目标系统/网关回调核验。
type ToolLinkSsoTicket struct {
	ent.Schema
}

func (ToolLinkSsoTicket) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tool_link_sso_tickets",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("工具链接 SSO 跳转票据表"),
	}
}

// Fields of the ToolLinkSsoTicket.
func (ToolLinkSsoTicket) Fields() []ent.Field {
	return []ent.Field{
		field.String("ticket").
			Comment("一次性票据（UUID）").
			MaxLen(64).
			Unique(),

		field.Uint32("user_id").
			Comment("票据签发用户ID"),

		field.Uint32("tool_link_id").
			Comment("关联工具链接ID"),

		field.Time("expire_at").
			Comment("票据过期时间"),

		field.Time("used_at").
			Comment("使用时间（一次性语义，核验成功后写入）").
			Optional().
			Nillable(),
	}
}

// Indexes of the ToolLinkSsoTicket.
func (ToolLinkSsoTicket) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("ticket").Unique(),
		index.Fields("user_id"),
	}
}

// Mixin of the ToolLinkSsoTicket.
func (ToolLinkSsoTicket) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

// Policy 隐私策略占位：票据核验走唯一索引精确查询。
func (ToolLinkSsoTicket) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
