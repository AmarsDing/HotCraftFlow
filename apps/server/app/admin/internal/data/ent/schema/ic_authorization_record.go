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

// AuthorizationRecord 授权记录（01-IC/03 §5.1 + 评审 B-14 缺失项清单 §12，🔴 合规项判据来源）。
// 闸门 authorization 规则的判据表：无此表 = authorization 规则永远不命中 = 合规形同虚设。
type AuthorizationRecord struct {
	ent.Schema
}

func (AuthorizationRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_authorization_records",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("授权记录：作者/域名/素材的授权状态，闸门 authorization 规则判据"),
	}
}

// Fields of the AuthorizationRecord.
func (AuthorizationRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("记录 ID（裸 ULID，内部实体）").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.ULID),

		field.String("subject").
			Comment("主体类型：author|domain|asset").
			MaxLen(16),
		field.String("subject_key").
			Comment("主体键（作者昵称/域名/asset:// 引用）").
			MaxLen(255),
		field.String("status").
			Comment("授权状态：granted|unknown|denied").
			MaxLen(12).
			Default("unknown"),
		field.String("scope").
			Comment("授权范围（如 可加工/可发布/仅参考）").
			MaxLen(255).
			Optional().
			Nillable(),
		field.String("evidence_ref").
			Comment("授权凭证引用（合同/邮件/截图存储路径）").
			MaxLen(255).
			Optional().
			Nillable(),
		field.Time("valid_until").
			Comment("授权有效期，可空（空 = 长期）").
			Optional().
			Nillable(),
	}
}

// Indexes of the AuthorizationRecord.
func (AuthorizationRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subject", "subject_key"),
		index.Fields("status"),
	}
}

// Edges of the AuthorizationRecord.
func (AuthorizationRecord) Edges() []ent.Edge {
	return nil
}

// Mixin of the AuthorizationRecord.
func (AuthorizationRecord) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
	}
}
