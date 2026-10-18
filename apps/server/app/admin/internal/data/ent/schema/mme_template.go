package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"github.com/tx7do/go-utils/entgo/mixin"
)

// MmeTemplate 混剪模板（④ MME，FR-MIX-08；详细设计 §1.6）。
// 开场/转场/结尾 + 字幕皮肤，保证矩阵产线一致性；模板变更可批量重渲。
type MmeTemplate struct {
	ent.Schema
}

func (MmeTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_templates",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 混剪模板（开场/转场/结尾 + 字幕皮肤；≥3 套；变更可批量重渲）"),
	}
}

// Fields of the MmeTemplate.
func (MmeTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("模板 ID，tpl_ 前缀").
			MaxLen(40).
			NotEmpty().
			Unique().
			Immutable(),
		field.String("name").
			Comment("模板名").
			MaxLen(128),
		field.String("intro_path").
			Comment("开场素材路径（可空 = 无片头）").
			MaxLen(1024).
			Default(""),
		field.String("outro_path").
			Comment("结尾素材路径（可空 = 无片尾）").
			MaxLen(1024).
			Default(""),
		field.String("transition").
			Comment("转场：cut|fade|slide").
			MaxLen(16).
			Default("cut"),
		field.JSON("caption_skin", map[string]any{}).
			Comment("字幕皮肤配置（字体/字号/颜色/描边/位置覆盖）").
			Optional(),
		field.Bool("enabled").
			Comment("是否可用").
			Default(true),
	}
}

// Edges of the MmeTemplate.
func (MmeTemplate) Edges() []ent.Edge { return nil }

// Mixin of the MmeTemplate.
func (MmeTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeTemplate.
func (MmeTemplate) Hooks() []ent.Hook { return nil }
