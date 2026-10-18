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

// WeightConfig 热度/选题权重配置（文档 02-TT 详细设计 §1.2 + 评审 A-15）。
//
// 只追加不改写：每次变更新插一行，effective_from = 变更时刻（不得回溯），
// 历史行保留以保证「当时的权重」可查（ADR-TT-05 历史可比）。
// weight_epoch = sha256(id)[:8]，随快照点/话题冗余落盘，权重口径断点标记。
type WeightConfig struct {
	ent.Schema
}

func (WeightConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tt_weight_configs",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("TT 权重配置（A-15：只追加，effective_from 生效，epoch 断点标记）"),
	}
}

// Fields of the WeightConfig.
func (WeightConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("config_id").
			Comment("配置 ID，wc_ 前缀 ULID；weight_epoch = sha256(config_id)[:8]（A-15 ②）").
			Unique().
			NotEmpty(),

		field.Enum("scope").
			Comment("用途：heat=热度四权重 | candidate=选题四权重").
			NamedValues(
				"Heat", "heat",
				"Candidate", "candidate",
			),

		// —— heat 四权重（Σ=1，详设默认 {0.35, 0.25, 0.20, 0.20}）——
		field.Float("w_rank").Comment("榜内位置权重").Default(0.35),
		field.Float("w_eng").Comment("互动分位权重").Default(0.25),
		field.Float("w_fresh").Comment("新鲜度权重").Default(0.20),
		field.Float("w_breadth").Comment("跨平台扩散权重").Default(0.20),

		// —— candidate 四权重（M5 选题评分使用，详设默认 {0.40, 0.25, 0.20, 0.15}）——
		field.Float("w_hot").Comment("热度分权重（选题）").Default(0.40),
		field.Float("w_freshness").Comment("新鲜度分权重（选题）").Default(0.25),
		field.Float("w_workability").Comment("可操作性分权重（选题）").Default(0.20),
		field.Float("w_compliance").Comment("合规分权重（选题）").Default(0.15),

		field.Time("effective_from").
			Comment("权重生效时间（A-15 ①：恒为变更时刻，不得回溯）"),

		field.String("activated_by").
			Comment("变更操作者（登录账号；审计）").
			Optional().
			Nillable(),

		field.String("note").
			Comment("变更备注（可选）").
			Optional().
			Nillable(),
	}
}

// Edges of the WeightConfig.
func (WeightConfig) Edges() []ent.Edge {
	return nil
}

// Indexes of the WeightConfig.
func (WeightConfig) Indexes() []ent.Index {
	return []ent.Index{
		// 按 scope 取 effective_from 最新一行 = 当前生效配置
		index.Fields("scope", "effective_from"),
	}
}

// Mixin of the WeightConfig.
func (WeightConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		appmixin.TenantID{},
	}
}
