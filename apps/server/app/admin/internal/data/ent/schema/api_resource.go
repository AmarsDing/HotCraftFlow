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

// ApiResource holds the schema definition for the ApiResource entity.
type ApiResource struct {
	ent.Schema
}

func (ApiResource) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "api_resource",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("API资源表"),
	}
}

// Fields of the ApiResource.
func (ApiResource) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("资源名称").
			Default("").
			MaxLen(128),

		field.String("path").
			Comment("接口路径").
			Default("").
			MaxLen(256),

		field.String("method").
			Comment("请求方法").
			Default("").
			MaxLen(10),

		field.String("permission").
			Comment("权限标识").
			Default("").
			MaxLen(128),

		field.String("description").
			Comment("描述").
			Optional().
			Nillable(),

		field.String("module").
			Comment("所属业务模块").
			Optional().
			Nillable(),

		field.String("module_description").
			Comment("业务模块描述").
			Optional().
			Nillable(),

		field.String("operation").
			Comment("接口操作名").
			Optional().
			Nillable(),

		field.String("source").
			Comment("来源：manual=手工创建，sync=OpenAPI 同步；同步仅重建 sync 行").
			Default("manual").
			MaxLen(16),

		field.Bool("delisted").
			Comment("已从 OpenAPI 文档消失（疑似下线），由同步独占维护；不参与鉴权，端点回到文档时自动复活").
			Default(false),
	}
}

// Mixin of the ApiResource.
func (ApiResource) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.SwitchStatus{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
	}
}

// Indexes of the ApiResource.
func (ApiResource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("path", "method").Unique(),
		index.Fields("delisted"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (ApiResource) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
