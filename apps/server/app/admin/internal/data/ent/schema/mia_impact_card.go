package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
	"framework/pkg/mediacontract"
)

// MiaImpactCard 影响卡（FR-MIA-06，详细设计 §1.4 / 契约 §3）。
// 🔴 三项硬校验在 biz 层强制执行：disclaimer 非空 / affected[].path 非空 / confidence 有效。
type MiaImpactCard struct {
	ent.Schema
}

func (MiaImpactCard) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mia_impact_cards", Charset: "utf8mb4", Collation: "utf8mb4_bin"},
		entsql.WithComments(true),
		schema.Comment("影响卡：事件 + 影响路径 + 关联标的 + 置信度（FR-MIA-06）"),
	}
}

func (MiaImpactCard) Fields() []ent.Field {
	return []ent.Field{
		field.String("impact_id").
			Comment("影响卡 ID，im_ 前缀 ULID（契约 §0.3）").
			Unique().
			NotEmpty(),
		field.String("event_id").
			Comment("关联事件卡 ec_").
			NotEmpty(),
		field.String("topic_id").
			Comment("关联 TT 话题 tp_，可空").
			Optional().
			Nillable(),
		field.String("title").
			Comment("标题"),
		field.Text("summary").
			Comment("摘要").
			Optional().
			Nillable(),
		field.String("lang").
			Comment("语言，如 en/zh").
			Default("en"),

		// —— 情感 ——
		field.Enum("sentiment_label").
			Comment("positive|neutral|negative|unknown（引擎全失效时禁止猜测）").
			NamedValues(
				"Positive", "positive",
				"Neutral", "neutral",
				"Negative", "negative",
				"Unknown", "unknown",
			).
			Default("unknown"),
		field.Float("sentiment_score").
			Comment("情感分 −1~1；unknown 时为 0").
			Default(0),
		field.Enum("sentiment_engine").
			Comment("finbert|llm|none（不同引擎分数不可混比）").
			NamedValues(
				"Finbert", "finbert",
				"Llm", "llm",
				"None", "none",
			).
			Default("none"),

		// —— 影响分 ——
		field.Float("impact_score").
			Comment("影响面评分 0–100").
			Default(0),
		field.Enum("direction").
			Comment("bullish|bearish|neutral（方向性独立于分数）").
			NamedValues(
				"Bullish", "bullish",
				"Bearish", "bearish",
				"Neutral", "neutral",
			).
			Default("neutral"),
		field.Int("ttl_hours").
			Comment("TTL 衰减时长（小时）").
			Default(24),
		field.Time("expires_at").
			Comment("过期时刻 UTC；过期卡不占排行榜但不删除（审计）"),
		field.JSON("factors", map[string]any{}).
			Comment("四因子明细 {sentiment, source_weight, relevance, decay}"),
		field.JSON("affected", []mediacontract.AffectedItem{}).
			Comment("受影响标的（🔴 每项必须 path 非空、confidence 有效）"),
		field.Bool("no_mapping").
			Comment("无实体映射（score=0，不强行匹配）").
			Default(false),

		// —— 合规 ——
		field.String("disclaimer").
			Comment("🔴 强制非空免责声明（空卡不允许落库与推送）").
			NotEmpty(),
		field.String("rule_version").
			Comment("规则版本（可复算，NFR-MIA-10）").
			Optional().
			Nillable(),
		field.JSON("weights_snapshot", map[string]any{}).
			Comment("权重快照（可复算）").
			Optional(),

		// —— 事后校验（FR-MIA-11，P2）——
		field.JSON("verified", map[string]any{}).
			Comment("{checked_at, actual_move, hit}；hit=null 表示未校验").
			Optional(),

		// —— 事件时间（decay 计算基准）——
		field.Time("occurred_at").
			Comment("事件发生时间 UTC"),

		field.Bool("pushed").
			Comment("是否已推送（高分卡推送幂等）").
			Default(false),
	}
}

func (MiaImpactCard) Edges() []ent.Edge { return nil }

func (MiaImpactCard) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_id"),
		index.Fields("topic_id"),
		index.Fields("sentiment_label"),
		index.Fields("impact_score"),
		index.Fields("expires_at"),
		// 列表主查询：未过期 + 按时间倒序
		index.Fields("expires_at", "occurred_at"),
		index.Fields("occurred_at"),
	}
}

func (MiaImpactCard) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
