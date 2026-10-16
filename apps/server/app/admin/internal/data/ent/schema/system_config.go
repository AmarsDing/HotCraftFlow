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

// SystemConfig holds the schema definition for the SystemConfig entity.
type SystemConfig struct {
	ent.Schema
}

func (SystemConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "system_configs",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("系统参数配置表"),
	}
}

func (SystemConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("config_key").
			Comment("配置键").
			MaxLen(128).
			NotEmpty(),

		field.String("config_value").
			Comment("配置值").
			NotEmpty(),

		field.String("config_type").
			Comment("配置类型：string/number/bool/json").
			Default("string").
			MaxLen(32),

		field.String("description").
			Comment("配置说明").
			Default("").
			MaxLen(512).
			Optional().
			Nillable(),
	}
}

func (SystemConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Time{},
	}
}

func (SystemConfig) Edges() []ent.Edge {
	return nil
}

func (SystemConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("config_key").Unique(),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (SystemConfig) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
