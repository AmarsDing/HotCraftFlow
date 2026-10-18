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

// MmeVariant 单个变体（④ MME，详细设计 §1.4 Variant）。
// selected 是「先展开后渲染」的实现关键（ADR-MME-01）：
// 展开时全部 selected=true，用户删减后只渲染 selected=true 的组合。
type MmeVariant struct {
	ent.Schema
}

func (MmeVariant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_variants",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 单个变体（v_ 前缀：内容版本 × 画幅 × Hook 的一个组合）"),
	}
}

// Fields of the MmeVariant.
func (MmeVariant) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("变体 ID，v_ 前缀 ULID").
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewVariantID),

		field.String("set_id").
			Comment("所属变体簇（vs_ 前缀）").
			MaxLen(36),
		field.String("content_version").
			Comment("B 维度：内容版本标识（「高光1」等的 highlight id）").
			MaxLen(64),
		field.String("aspect").
			Comment("A 维度：画幅 9:16|1:1|16:9").
			MaxLen(8),
		field.String("hook_id").
			Comment("C 维度：Hook 标识").
			MaxLen(64).
			Default(""),
		field.String("cover_id").
			Comment("C 维度：封面标识").
			MaxLen(64).
			Default(""),
		field.String("edl_ref").
			Comment("切点定义 JSON（{start_sec,end_sec}；在 B 维度确定）").
			MaxLen(512).
			Default(""),
		field.Float("duration_sec").
			Comment("成片时长").
			Default(0),
		field.String("video_path").
			Comment("成片文件路径（对象存储路径，非 HTTP URL）").
			MaxLen(1024).
			Default(""),
		field.String("cover_path").
			Comment("封面图路径").
			MaxLen(1024).
			Default(""),
		field.String("caption_style").
			Comment("字幕档位：simple|dynamic").
			MaxLen(16).
			Default("simple"),
		field.String("srt_path").
			Comment("字幕文件路径").
			MaxLen(1024).
			Default(""),
		field.JSON("copy_json", map[string]any{}).
			Comment("平台文案 {douyin:{title,desc,tags},xiaohongshu:{...},bilibili:{...},youtube_shorts:{...}}；键须与 MAP platform_key 一致").
			Optional(),
		field.Bool("copy_degraded").
			Comment("文案为模板降级产物（LLM 不可用；诚实标注，manifest 同步）").
			Default(false),
		field.String("manifest_path").
			Comment("manifest.json 路径（含 QC 块，详细设计 §2.7）").
			MaxLen(1024).
			Default(""),
		field.Bool("qc_boundary_ok").
			Comment("边界自检（时长/分辨率/黑帧；🔴 硬门禁）").
			Default(false),
		field.Bool("qc_subtitle_ok").
			Comment("字幕自检（安全区/无句中硬切/时长匹配；🔴 硬门禁）").
			Default(false),
		field.Float("qc_score").
			Comment("质检综合分 0-1").
			Default(0),
		field.String("render_state").
			Comment("渲染状态：pending|running|done|failed").
			MaxLen(12).
			Default("pending"),
		field.String("render_error").
			Comment("渲染失败原因（可读）").
			MaxLen(1024).
			Optional().
			Nillable(),
		field.Bool("selected").
			Comment("是否保留渲染（展开=true；删减只渲染 selected=true 的）").
			Default(true),
		field.String("review_state").
			Comment("审核状态：pending|approved|rejected（FR-MIX-06 批量审核台）").
			MaxLen(12).
			Default("pending"),
		field.String("review_reason").
			Comment("驳回原因（驳回必填；批准为空）").
			MaxLen(512).
			Optional().
			Nillable(),
	}
}

// Indexes of the MmeVariant.
func (MmeVariant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("set_id"),
		index.Fields("content_version"),
		index.Fields("aspect"),
		index.Fields("render_state"),
		index.Fields("selected"),
		index.Fields("set_id", "selected"),
	}
}

// Edges of the MmeVariant.
func (MmeVariant) Edges() []ent.Edge { return nil }

// Mixin of the MmeVariant.
func (MmeVariant) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeVariant.
func (MmeVariant) Hooks() []ent.Hook { return nil }
