package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"
	appmixin "framework/pkg/entgo/mixin"
)

// Dict holds the schema definition for the Dict entity.
type Dict struct {
	ent.Schema
}

func (Dict) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "dict",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("数据字典表"),
	}
}

// Fields of the Dict.
func (Dict) Fields() []ent.Field {
	return []ent.Field{
		field.String("type").
			Comment("字典类型编码").
			Default("").
			MaxLen(64),

		field.String("type_name").
			Comment("字典类型名称").
			Default("").
			MaxLen(128),

		field.String("item_key").
			Comment("字典项键").
			Default("").
			MaxLen(64),

		field.String("item_value").
		Comment("字典项值").
		Default("").
		MaxLen(2048),

	field.String("value_desc").
		Comment("字典值名称/描述").
		Optional().
		Nillable(),

	field.String("value_data_type").
			Comment("字典值数据类型").
			Optional().
			Nillable(),

		field.Int32("sort_id").
			Comment("排序ID").
			Optional().
			Nillable().
			Default(0),
	}
}

// Mixin of the Dict.
func (Dict) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.SwitchStatus{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Remark{},
		appmixin.TenantID{},
	}
}

// Indexes of the Dict.
func (Dict) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("type", "item_key").Unique(),
		index.Fields("type"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (Dict) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
