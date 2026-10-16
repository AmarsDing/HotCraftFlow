package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"

	"entgo.io/ent/privacy"

	"github.com/tx7do/go-utils/entgo/mixin"
)

// DictCategory 字典分类实体（P2 分类实体化）。
// 分类由 dict 条目上的冗余字符串升级为独立实体：编码全局唯一，
// 中文名/用途（enum 枚举 / param 参数）单点维护，消除同分类各条目 type_name 不一致的结构性问题。
// 消费方（consumers）不入库——那是代码耦合事实，由 biz 层注册表权威声明。
type DictCategory struct {
	ent.Schema
}

func (DictCategory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "dict_category",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("数据字典分类表"),
	}
}

// Fields of the DictCategory.
func (DictCategory) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			Comment("分类编码（= dict.type，身份不可改）").
			MaxLen(64).
			NotEmpty().
			Unique(),

		field.String("name").
			Comment("分类中文名").
			Default("").
			MaxLen(128),

		field.String("kind").
			Comment("分类用途：enum 枚举选项 / param 系统参数").
			Default("enum").
			MaxLen(16),

		field.Bool("builtin").
			Comment("系统内置分类（禁删、禁改编码，值按 schema 校验）").
			Default(false),

		field.Int32("sort_id").
			Comment("排序ID").
			Optional().
			Nillable().
			Default(0),
	}
}

// Mixin of the DictCategory.
// 分类是全局元数据（非租户数据），不挂 TenantID——与 dict 条目表刻意区分。
func (DictCategory) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Remark{},
	}
}

// Indexes of the DictCategory.
func (DictCategory) Indexes() []ent.Index {
	return nil
}

// Policy 与 dict 一致：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/鉴权仍由各 repo 手动条件与上层鉴权承担。
func (DictCategory) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
