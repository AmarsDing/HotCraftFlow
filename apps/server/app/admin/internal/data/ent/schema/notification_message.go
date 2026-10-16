package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/tx7do/go-utils/entgo/mixin"
	appmixin "framework/pkg/entgo/mixin"
)

// NotificationMessage holds the schema definition for the NotificationMessage entity.
type NotificationMessage struct {
	ent.Schema
}

func (NotificationMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "notification_messages",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("站内信通知消息表"),
	}
}

// Fields of the NotificationMessage.
func (NotificationMessage) Fields() []ent.Field {
	return []ent.Field{
		field.String("subject").
			Comment("主题").
			Optional().
			Nillable(),

		field.String("content").
			Comment("内容").
			Optional().
			Nillable(),

		field.Uint32("category_id").
			Comment("分类ID").
			Optional().
			Nillable(),

		field.Enum("status").
			Comment("消息状态").
			NamedValues(
				"Unknown", "UNKNOWN",
				"Draft", "DRAFT",
				"Published", "PUBLISHED",
				"Scheduled", "SCHEDULED",
				"Revoked", "REVOKED",
				"Archived", "ARCHIVED",
				"Deleted", "DELETED",
			).
			Optional().
			Nillable(),
	}
}

// Mixin of the NotificationMessage.
func (NotificationMessage) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (NotificationMessage) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
