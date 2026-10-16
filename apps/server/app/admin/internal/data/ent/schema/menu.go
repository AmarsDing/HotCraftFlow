package schema

import (
	"entgo.io/contrib/entproto"
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/tx7do/go-utils/entgo/mixin"

	adminV1 "framework/api/gen/go/admin/v1"
)

// Menu holds the schema definition for the Menu entity.
type Menu struct {
	ent.Schema
}

func (Menu) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "menu_permissions",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("菜单权限表"),
	}
}

// Fields of the Menu.
func (Menu) Fields() []ent.Field {
	return []ent.Field{
		field.Int32("id").
			Comment("id").
			StructTag(`json:"id,omitempty"`).
			Positive().
			Immutable().
			Unique(),

		field.Int32("parent_id").
			Comment("上一层菜单ID").
			Optional().
			Nillable(),

		field.Enum("type").
			Comment("菜单类型 FOLDER: 目录 MENU: 菜单 BUTTON: 按钮").
			NamedValues(
				"Folder", "FOLDER",
				"Menu", "MENU",
				"Button", "BUTTON",
			).
			Default("MENU").
			Optional().
			Nillable(),

		field.String("name").
			Comment("菜单名称").
			Optional().
			Nillable(),

		field.String("path").
			Comment("路径,当其类型为'按钮'的时候对应的数据操作名,例如:/user.service.v1.UserService/Login").
			Default("").
			Optional().
			Nillable(),

		field.String("component").
			Comment("前端页面组件").
			Default("").
			Optional().
			Nillable(),

		// 已废弃：图标真相在 meta.icon（RouteMeta JSON），本列 CRUD 从不写入，仅保留兼容历史数据。
		field.String("icon").
			Comment("【已废弃，用 meta.icon】图标名称").
			Default("").
			MaxLen(64).
			Optional().
			Nillable(),

		// 已废弃：排序真相在 meta.order（biz List/ListByIDs 均按 meta.order 排序），本列 CRUD 从不写入。
		field.Int32("sort").
			Comment("【已废弃，用 meta.order】排序号").
			Default(0),

		// 已废弃：权限码真相在 meta.authority（BUTTON 类型经 ListPermissionCode 下发），本列 CRUD 从不写入。
		field.String("permission").
			Comment("【已废弃，用 meta.authority】权限标识").
			Default("").
			MaxLen(128).
			Optional().
			Nillable(),

		field.String("redirect").
			Comment("重定向地址").
			Optional().
			Nillable(),

		field.String("alias").
			Comment("路由别名").
			Optional().
			Nillable(),

		field.JSON("meta", &adminV1.RouteMeta{}).
			Comment("前端页面组件元数据").
			Optional(),
	}
}

func (Menu) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.SwitchStatus{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Remark{},
	}
}

// Edges of the Menu.
func (Menu) Edges() []ent.Edge {
	return []ent.Edge{
		edge.
			To("children", Menu.Type).Annotations(entproto.Field(10)).
			From("parent").Unique().Field("parent_id").Annotations(entproto.Field(11)),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (Menu) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
