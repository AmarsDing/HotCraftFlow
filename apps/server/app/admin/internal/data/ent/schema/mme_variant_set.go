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

// MmeVariantSet 变体簇（④ MME，FR-MIX-01；契约 §4 的持久化视图）。
// 一源多版的统一管理单元：N 内容版本 × 3 画幅 × M Hook 笛卡尔展开，
// 「先展开后渲染、允许删减」（ADR-MME-01），可整体送审（FR-MIX-06）。
type MmeVariantSet struct {
	ent.Schema
}

func (MmeVariantSet) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_variant_sets",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 变体簇（vs_ 前缀；矩阵展开的统一管理容器）"),
	}
}

// Fields of the MmeVariantSet.
func (MmeVariantSet) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("变体簇 ID，vs_ 前缀 ULID").
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewVariantSetID),

		field.String("task_id").
			Comment("加工任务 ID（task_ 前缀）").
			MaxLen(36),
		field.String("candidate_id").
			Comment("回指 ② TT 选题，可空（手动传素材场景为 null，契约 §4）").
			MaxLen(36).
			Optional().
			Nillable(),
		field.String("asset_id").
			Comment("素材 ID（as_ 前缀）").
			MaxLen(36),
		field.String("transcript_ref").
			Comment("转写产物引用（AnalysisCache payload_path 或 asset:// 字幕）").
			MaxLen(1024).
			Default(""),
		field.String("state").
			Comment("状态：planned|rendering|review|approved|rejected").
			MaxLen(16).
			Default("planned"),
		field.Int32("variant_count").
			Comment("变体总数").
			Default(0),
		field.Float("diff_min_similarity").
			Comment("差异化检查：同簇两两最大相似度的最小值（FR-MIX-05，越高越危险）").
			Default(0),
		field.Bool("diff_warn").
			Comment("差异化告警（综合 ≥0.90；强提示不硬阻断，ADR-MME-04）").
			Default(false),
		field.JSON("diff_pairs", []map[string]any{}).
			Comment("相似度对明细 [{variant_a,variant_b,visual,hook,copy,overall}]（告警对高亮用）").
			Optional(),
		field.String("template_id").
			Comment("混剪模板 ID（FR-MIX-08），可空").
			MaxLen(36).
			Optional().
			Nillable(),
	}
}

// Indexes of the MmeVariantSet.
func (MmeVariantSet) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
		index.Fields("candidate_id"),
		index.Fields("state"),
	}
}

// Edges of the MmeVariantSet.
func (MmeVariantSet) Edges() []ent.Edge { return nil }

// Mixin of the MmeVariantSet.
func (MmeVariantSet) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeVariantSet.
func (MmeVariantSet) Hooks() []ent.Hook { return nil }
