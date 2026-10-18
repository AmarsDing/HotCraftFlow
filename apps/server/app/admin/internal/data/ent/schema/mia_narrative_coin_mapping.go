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

// NarrativeCoinMapping 叙事→币种映射表（映射表 B，FR-MIA-02，详细设计 §1.2）。
// 加密叙事（现货 ETF、监管、公链升级、稳定币…）→ 受影响币种与默认情感提示。
type NarrativeCoinMapping struct {
	ent.Schema
}

func (NarrativeCoinMapping) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mia_narrative_coin_mappings", Charset: "utf8mb4", Collation: "utf8mb4_bin"},
		entsql.WithComments(true),
		schema.Comment("映射表 B：加密叙事 → 币种（FR-MIA-02）"),
	}
}

func (NarrativeCoinMapping) Fields() []ent.Field {
	return []ent.Field{
		field.String("narrative").
			Comment("叙事名，如「现货 ETF 获批」").
			NotEmpty(),
		field.JSON("keywords", []string{}).
			Comment("触发关键词数组"),
		field.JSON("coins", []string{}).
			Comment("受影响币种，如 [BTC, ETH]"),
		field.Enum("default_sentiment_hint").
			Comment("默认情感提示：bullish|bearish|neutral").
			NamedValues(
				"Bullish", "bullish",
				"Bearish", "bearish",
				"Neutral", "neutral",
			).
			Default("neutral"),
		field.Int("ttl_hours").
			Comment("该叙事的影响衰减时长（小时）。加密叙事衰减快，常 6h").
			Default(6),
		field.Float("confidence").
			Comment("映射可信度 0–1").
			Default(0.6),
		field.Bool("enabled").
			Comment("是否启用").
			Default(true),
	}
}

func (NarrativeCoinMapping) Edges() []ent.Edge { return nil }

func (NarrativeCoinMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		// 同一叙事不重复建（详细设计 §1.2）
		index.Fields("narrative").Unique(),
	}
}

func (NarrativeCoinMapping) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
