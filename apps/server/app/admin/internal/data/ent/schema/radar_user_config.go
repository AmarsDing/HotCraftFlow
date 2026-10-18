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

// RadarUserConfig holds the schema definition for the RadarUserConfig entity.
// 每用户雷达配置：GitHub Token / LLM / 克隆设置；密钥字段 AES-256-GCM 加密存储，不明文落库。
type RadarUserConfig struct {
	ent.Schema
}

func (RadarUserConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "radar_user_configs",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("GitHub 热门追踪用户配置表（密钥加密存储）"),
	}
}

// Fields of the RadarUserConfig.
func (RadarUserConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("user_id").
			Comment("用户ID").
			Unique(),

		field.String("github_token_enc").
			Comment("GitHub Token 密文（AES-256-GCM，空=未配置）").
			Optional().
			Nillable(),

		field.Bool("llm_enabled").
			Comment("是否启用 AI 总结").
			Default(false),

		field.String("llm_base_url").
			Comment("LLM 服务地址").
			Optional().
			Nillable(),

		field.String("llm_api_key_enc").
			Comment("LLM API Key 密文（AES-256-GCM）").
			Optional().
			Nillable(),

		field.String("llm_model").
			Comment("LLM 模型名").
			Optional().
			Nillable(),

		field.Bool("clone_enabled").
			Comment("是否允许服务端克隆仓库").
			Default(false),

		field.String("clone_dir").
			Comment("服务端克隆目录").
			Optional().
			Nillable(),
	}
}

// Indexes of the RadarUserConfig.
func (RadarUserConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
	}
}

// Mixin of the RadarUserConfig.
func (RadarUserConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

// Policy 隐私策略占位：行级归属由 repo 以 user_id 条件强制过滤（见 RadarUserConfigRepo）。
func (RadarUserConfig) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
