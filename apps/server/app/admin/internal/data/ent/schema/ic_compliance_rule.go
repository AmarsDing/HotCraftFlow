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

// ComplianceRule 合规闸门规则（01-IC/03 §2.5 + 评审 S-9，🔴 合规项判据来源）。
// 多重命中取最严（block > review > pass）；变更写 admin_operation_log（本仓已有审计）。
type ComplianceRule struct {
	ent.Schema
}

func (ComplianceRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_compliance_rules",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("合规闸门规则表：敏感类目/权利人黑名单/授权状态/robots/平台条款五类判据"),
	}
}

// Fields of the ComplianceRule.
func (ComplianceRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("规则 ID，rule_ 前缀 ULID").
			MaxLen(32).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(func() string { return contract.NewID(contract.PrefixRule) }),

		field.String("rule_type").
			Comment("规则类型：sensitive_category|rights_holder_blacklist|authorization|robots|platform_tos（评审 S-8）").
			MaxLen(32),
		field.String("pattern").
			Comment("匹配模式：关键词/正则/域名（按 rule_type 解释）").
			MaxLen(512).
			NotEmpty(),
		field.String("action").
			Comment("命中动作：pass|review|block（多重命中取最严）").
			MaxLen(8),
		field.Int32("priority").
			Comment("优先级，仅影响解释展示顺序，不影响最严判定").
			Default(0),
		field.Bool("enabled").
			Comment("是否启用").
			Default(true),

		// —— 生命周期与审计（评审 S-9）——
		field.Time("effective_from").
			Comment("生效起始，可空（空 = 立即生效）").
			Optional().
			Nillable(),
		field.Time("effective_until").
			Comment("生效截止，可空（过期规则不参与判定）").
			Optional().
			Nillable(),
		field.Int64("hit_count").
			Comment("累计命中次数（由闸门异步累加）").
			Default(0),
		field.String("scope").
			Comment("作用域：global|platform").
			MaxLen(16).
			Default("global"),
		field.String("platform").
			Comment("作用域为 platform 时的平台标识，可空").
			MaxLen(32).
			Optional().
			Nillable(),
		field.String("remark").
			Comment("规则说明（审计可读）").
			MaxLen(512).
			Optional().
			Nillable(),
	}
}

// Indexes of the ComplianceRule.
func (ComplianceRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("rule_type", "enabled"),
		index.Fields("scope", "platform"),
	}
}

// Edges of the ComplianceRule.
func (ComplianceRule) Edges() []ent.Edge {
	return nil
}

// Mixin of the ComplianceRule.
func (ComplianceRule) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
	}
}
