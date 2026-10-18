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

// EntityAssetMapping 实体→标的映射表（映射表 A，FR-MIA-01，详细设计 §1.1）。
// 人物/机构/政策/技术 → 市场标的的默认映射；notes 必填（无理由的映射不可信）。
type EntityAssetMapping struct {
	ent.Schema
}

func (EntityAssetMapping) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mia_entity_asset_mappings", Charset: "utf8mb4", Collation: "utf8mb4_bin"},
		entsql.WithComments(true),
		schema.Comment("映射表 A：实体 → 市场标的（FR-MIA-01）"),
	}
}

func (EntityAssetMapping) Fields() []ent.Field {
	return []ent.Field{
		field.String("entity_name").
			Comment("实体名（人物/机构/技术/政策），归一化后主名").
			NotEmpty(),
		field.JSON("entity_alias", []string{}).
			Comment("别名数组（如「美联储」/「Fed」/「FOMC」）").
			Optional(),
		field.Enum("entity_type").
			Comment("实体类型：person|org|policy|tech|place|topic").
			NamedValues(
				"Person", "person",
				"Org", "org",
				"Policy", "policy",
				"Tech", "tech",
				"Place", "place",
				"Topic", "topic",
			),
		field.Enum("asset_class").
			Comment("资产类别：equity|index|fx|commodity|crypto（契约 §3）").
			NamedValues(
				"Equity", "equity",
				"Index", "index",
				"Fx", "fx",
				"Commodity", "commodity",
				"Crypto", "crypto",
			),
		field.String("symbol").
			Comment("标的代码：SPY / ^GSPC / EURUSD / BTC/USDT").
			NotEmpty(),
		field.Enum("direction_bias").
			Comment("事件命中该实体时的默认偏向：bullish|bearish|neutral").
			NamedValues(
				"Bullish", "bullish",
				"Bearish", "bearish",
				"Neutral", "neutral",
			).
			Default("neutral"),
		field.Float("confidence").
			Comment("映射可信度 0–1").
			Default(0.6),
		field.String("source").
			Comment("来源：manual|rule_gen|import").
			Default("manual"),
		field.Text("notes").
			Comment("理由说明（🔴 必填，无理由的映射不可信）"),
		field.Bool("enabled").
			Comment("是否启用").
			Default(true),
	}
}

func (EntityAssetMapping) Edges() []ent.Edge { return nil }

func (EntityAssetMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("entity_name"),
		index.Fields("entity_type"),
		index.Fields("asset_class"),
		index.Fields("symbol"),
		index.Fields("enabled"),
		// 🔴 唯一约束：防重复导入（详细设计 §1.1）
		index.Fields("entity_name", "asset_class", "symbol").Unique(),
	}
}

func (EntityAssetMapping) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
