package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"
)

// ToolLink holds the schema definition for the ToolLink entity.
type ToolLink struct {
	ent.Schema
}

func (ToolLink) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tool_links",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("工具链接门户表"),
	}
}

// Fields of the ToolLink.
func (ToolLink) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("工具名称").
			MaxLen(128).
			Optional().
			Nillable(),

		field.String("url").
			Comment("工具链接（http/https）").
			MaxLen(512).
			Optional().
			Nillable(),

		field.Enum("icon_type").
			Comment("图标类型（LUCIDE=Lucide图标/IMAGE=自定义图片）").
			NamedValues(
				"Lucide", "LUCIDE",
				"Image", "IMAGE",
			).
			Default("LUCIDE").
			Optional().
			Nillable(),

		field.String("icon_value").
			Comment("图标值：LUCIDE 时为图标名；IMAGE 时为文件相对路径").
			MaxLen(512).
			Optional().
			Nillable(),

		field.String("category").
			Comment("分类（自由文本，如 监控/日志/文档）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("description").
			Comment("描述").
			MaxLen(512).
			Optional().
			Nillable(),

		field.Int32("sort").
			Comment("排序号，越小越靠前").
			Default(0).
			Optional().
			Nillable(),

		field.Bool("external").
			Comment("是否外部链接新窗口打开").
			Default(true).
			Optional().
			Nillable(),

		field.Uint32("category_id").
			Comment("分类ID（引用 tool_link_categories；为空时回退 category 自由文本）").
			Optional().
			Nillable(),

		field.Enum("health_status").
			Comment("健康状态（UNKNOWN=未检测/UP=可达/DOWN=不可达）").
			NamedValues(
				"Unknown", "UNKNOWN",
				"Up", "UP",
				"Down", "DOWN",
			).
			Default("UNKNOWN").
			Optional().
			Nillable(),

		field.Time("last_check_at").
			Comment("最近一次健康检测时间").
			Optional().
			Nillable(),

		field.Uint32("click_count").
			Comment("累计点击次数（全用户）").
			Default(0).
			Optional().
			Nillable(),

		field.JSON("visible_roles", []string{}).
			Comment("可见角色码列表（空=全员可见）").
			Optional(),

		field.Enum("link_type").
			Comment("链接类型（NORMAL=普通跳转/SSO=免密跳转签票）").
			NamedValues(
				"Normal", "NORMAL",
				"Sso", "SSO",
			).
			Default("NORMAL").
			Optional().
			Nillable(),

		field.JSON("name_i18n", map[string]string{}).
			Comment("名称多语言（locale->名称），缺省回退 name").
			Optional(),

		field.JSON("description_i18n", map[string]string{}).
			Comment("描述多语言（locale->描述），缺省回退 description").
			Optional(),
	}
}

// Indexes of the ToolLink.
func (ToolLink) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sort"),
		index.Fields("category"),
		index.Fields("category_id"),
	}
}

// Mixin of the ToolLink.
func (ToolLink) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.SwitchStatus{},
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (ToolLink) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
