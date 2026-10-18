package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// MapPublishAccount 账号池（⑤ MAP，FR-PUB-05 / 详细设计 §1.2）。
// 🔴 credential_ref 绝不存明文——实际凭据经 MAP_CREDENTIAL_KEY 加密后由
// data/map_credential_vault.go 管理，本字段只存引用/句柄。日志全程脱敏（NFR-MAP-03）。
type MapPublishAccount struct {
	ent.Schema
}

func (MapPublishAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "map_publish_accounts",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MAP 账号池：凭据加密引用 + 健康状态 + 熔断字段（⑤ MAP 详细设计 §1.2）"),
	}
}

// Fields of the MapPublishAccount.
func (MapPublishAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("account_id").
			Comment("账号 ID，pa_ 前缀 ULID（契约 00-shared §0.3）").
			Unique().
			NotEmpty(),

		field.String("channel_id").
			Comment("所属渠道（map_publish_channels.channel_id）").
			NotEmpty(),

		field.String("display_name").
			Comment("便于识别的名称（如「抖音-主号」）").
			NotEmpty(),

		field.String("credential_ref").
			Comment("🔴 加密凭据引用（enc:v1:...），非明文；由 CredentialVault 管理").
			Optional().
			Nillable(),

		field.Enum("credential_type").
			Comment("凭据类型：oauth / cookie / api_key").
			Values("oauth", "cookie", "api_key").
			Optional().
			Nillable(),

		field.Bool("enabled").
			Comment("是否启用（人工启停）").
			Default(true),

		field.Enum("health").
			Comment("健康状态：healthy / degraded / paused / credential_invalid（凭据失效，不清凭据待重新授权）").
			NamedValues(
				"Healthy", "healthy",
				"Degraded", "degraded",
				"Paused", "paused",
				"CredentialInvalid", "credential_invalid",
			).
			Default("healthy"),

		field.Int("consecutive_failures").
			Comment("连续发布失败数（熔断计数）").
			Default(0),

		field.Time("paused_until").
			Comment("熔断暂停截止时间，UTC；paused 时有效").
			Optional().
			Nillable(),

		field.Int("published_24h").
			Comment("24h 滚动发布计数（应用层维护 + receipts 复核）").
			Default(0),

		field.Time("last_published_at").
			Comment("最近一次成功发布时间，UTC").
			Optional().
			Nillable(),

		field.String("last_error").
			Comment("最近一次错误（🔴 已脱敏，不含凭据/堆栈原文）").
			Optional().
			Nillable(),
	}
}

// Indexes of the MapPublishAccount.
func (MapPublishAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("channel_id"),
		index.Fields("enabled"),
		index.Fields("health"),
		index.Fields("paused_until"),
	}
}

// Edges of the MapPublishAccount.
func (MapPublishAccount) Edges() []ent.Edge {
	return nil
}

// Mixin of the MapPublishAccount.
func (MapPublishAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
