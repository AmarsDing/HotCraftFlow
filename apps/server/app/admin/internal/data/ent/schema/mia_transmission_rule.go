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

// MiaTransmissionRule 跨资产传导规则（FR-MIA-08，详细设计 §1.6）。
// 🔴 每条规则强制 confidence ≤0.6 且 note 必填——传导是假设不是因果。
type MiaTransmissionRule struct {
	ent.Schema
}

func (MiaTransmissionRule) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mia_transmission_rules", Charset: "utf8mb4", Collation: "utf8mb4_bin"},
		entsql.WithComments(true),
		schema.Comment("跨资产传导规则：假设非因果，confidence ≤0.6（FR-MIA-08）"),
	}
}

func (MiaTransmissionRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("rule_id").
			Comment("规则 ID，rule_ 前缀 ULID（契约 §0.3）").
			Unique().
			NotEmpty(),
		field.JSON("trigger", map[string]any{}).
			Comment("触发条件，如 {asset=DXY, direction=up, min_move_pct=0.5}"),
		field.String("from_asset").
			Comment("触发资产（冗余于 trigger.asset，便于查询）").
			Optional().
			Nillable(),
		field.String("to_asset").
			Comment("传导目标标的").
			NotEmpty(),
		field.Enum("to_direction").
			Comment("传导方向：bullish|bearish").
			NamedValues(
				"Bullish", "bullish",
				"Bearish", "bearish",
			),
		field.Float("confidence").
			Comment("🔴 必填 0<置信度≤0.6（假设非因果，标 0.9 是自欺欺人）"),
		field.Text("note").
			Comment("🔴 必填：说明这是假设及依据"),
		field.Bool("enabled").
			Comment("是否启用").
			Default(true),
		// 校准统计（FR-MIA-11，P2）
		field.Int("hit_count").
			Comment("该规则被触发的传导条数").
			Default(0),
		field.Int("correct_count").
			Comment("事后校验命中条数").
			Default(0),
	}
}

func (MiaTransmissionRule) Edges() []ent.Edge { return nil }

func (MiaTransmissionRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("from_asset"),
	}
}

func (MiaTransmissionRule) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
