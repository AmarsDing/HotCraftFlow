package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-utils/entgo/mixin"

	contract "framework/pkg/mediacontract"
)

// CredentialAccount 多账号池（01-IC FR-INT-13 + 评审 B-14/S-11）。
// 轮询算法（写死）：按 (fail_streak ASC, last_used_at ASC) 选 enabled 且 paused_until < now 的账号；
// 命中 429/401 → fail_streak++、paused_until = now + min(2^fail_streak × 5m, 4h)。
type CredentialAccount struct {
	ent.Schema
}

func (CredentialAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_credential_accounts",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("采集账号池：多账号轮询与账号级熔断"),
	}
}

// Fields of the CredentialAccount.
func (CredentialAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("账号 ID，acc_ 前缀 ULID（契约 §0.3 内部实体）").
			MaxLen(32).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(func() string { return contract.NewID(contract.PrefixCredentialAcct) }),

		field.String("platform").
			Comment("平台标识").
			MaxLen(32),
		field.String("account_key").
			Comment("账号槽位名（如 main / a1）；同平台内唯一").
			MaxLen(64),
		field.String("credential_ref").
			Comment("凭据引用（加密存储，非明文；NFR-IC-06）").
			MaxLen(64).
			NotEmpty(),
		field.Bool("enabled").
			Comment("是否启用").
			Default(true),

		field.Int64("quota_used").
			Comment("配额已用（日维度，由限流层累加）").
			Default(0),
		field.Int64("quota_limit").
			Comment("配额上限（0 = 不限）").
			Default(0),
		field.Int32("fail_streak").
			Comment("连续失败次数").
			Default(0),
		field.Time("paused_until").
			Comment("账号暂停截止时间，可空").
			Optional().
			Nillable(),
		field.Time("last_used_at").
			Comment("最近使用时间，可空").
			Optional().
			Nillable(),
	}
}

// Indexes of the CredentialAccount.
func (CredentialAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "account_key").Unique(),
		index.Fields("platform", "enabled"),
	}
}

// Edges of the CredentialAccount.
func (CredentialAccount) Edges() []ent.Edge {
	return nil
}

// Mixin of the CredentialAccount.
func (CredentialAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
