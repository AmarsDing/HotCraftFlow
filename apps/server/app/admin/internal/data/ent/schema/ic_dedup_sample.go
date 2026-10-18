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

// DedupSample 去重调参样本（01-IC/03 §4.4 + 评审 B-14）。
// 自动合并与人工拆分/合并均记样本，供阈值优化（验收第 3 条：归并正确率 ≥85% 的调参依据）。
type DedupSample struct {
	ent.Schema
}

func (DedupSample) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_dedup_samples",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("去重调参样本：auto_merge / manual_split / manual_merge 操作留痕"),
	}
}

// Fields of the DedupSample.
func (DedupSample) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("样本 ID（裸 ULID，内部实体未注册契约前缀）").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.ULID),

		field.String("intel_id").
			Comment("涉及情报 ID").
			MaxLen(30),
		field.String("cluster_id").
			Comment("涉及簇 ID").
			MaxLen(30),
		field.String("action").
			Comment("操作：auto_merge|manual_split|manual_merge").
			MaxLen(16),
		field.String("level").
			Comment("命中级：url|fingerprint|embedding|manual").
			MaxLen(16).
			Optional().
			Nillable(),
		field.Float("sim_score").
			Comment("相似度（汉明距离或余弦相似度，随 level 解释）").
			Default(0),
		field.String("operator").
			Comment("操作人；自动合并记 system").
			MaxLen(64).
			Default("system"),
	}
}

// Indexes of the DedupSample.
func (DedupSample) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("cluster_id"),
		index.Fields("action"),
		index.Fields("intel_id"),
	}
}

// Edges of the DedupSample.
func (DedupSample) Edges() []ent.Edge {
	return nil
}

// Mixin of the DedupSample.
func (DedupSample) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
