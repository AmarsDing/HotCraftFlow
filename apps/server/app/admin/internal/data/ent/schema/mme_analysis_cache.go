package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-utils/entgo/mixin"
)

// MmeAnalysisCache 分析产物缓存（④ MME，FR-MIX-02；详细设计 §1.3/§2.2）。
// cache_key = sha256(asset_id|kind|engine|engine_version|prompt_version|intent_hash|lang)。
// 验收判据：二次成片 ASR/LLM 调用次数为 0（NFR-MME-02 命中率 ≥95%）。
type MmeAnalysisCache struct {
	ent.Schema
}

func (MmeAnalysisCache) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_analysis_caches",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 分析产物缓存（transcript/chapters/report/keyframes/reframed）"),
	}
}

// Fields of the MmeAnalysisCache.
func (MmeAnalysisCache) Fields() []ent.Field {
	return []ent.Field{
		field.String("cache_key").
			Comment("缓存键 sha256 hex（详细设计 §2.2 公式；唯一）").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("kind").
			Comment("产物类型：transcript|chapters|report|keyframes|reframed").
			MaxLen(20),
		field.String("asset_id").
			Comment("素材 ID（as_ 前缀）").
			MaxLen(36),
		field.String("payload_path").
			Comment("产物存储路径（SRT/VTT/JSON/中间片）").
			MaxLen(1024),
		field.JSON("payload_meta", map[string]any{}).
			Comment("摘要信息（列表展示用，不存全文）").
			Optional(),
		field.String("engine").
			Comment("引擎：whisper|funasr|cloud|ollama|ffmpeg").
			MaxLen(32).
			Default(""),
		field.String("engine_version").
			Comment("引擎版本（缓存失效因素之一）").
			MaxLen(64).
			Default(""),
		field.String("prompt_version").
			Comment("分析提示词版本").
			MaxLen(32).
			Default(""),
		field.Int64("hit_count").
			Comment("命中次数（评估缓存价值；命中率指标 mme_cache_hit_rate）").
			Default(0),
		field.Int64("size_bytes").
			Comment("产物字节数（max_cache_bytes 总量帽管理）").
			Default(0),
		field.Time("expires_at").
			Comment("过期时间（默认 +90d；transcript +180d）"),
	}
}

// Indexes of the MmeAnalysisCache.
func (MmeAnalysisCache) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind"),
		index.Fields("asset_id"),
		index.Fields("expires_at"),
	}
}

// Edges of the MmeAnalysisCache.
func (MmeAnalysisCache) Edges() []ent.Edge { return nil }

// Mixin of the MmeAnalysisCache.
func (MmeAnalysisCache) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeAnalysisCache.
func (MmeAnalysisCache) Hooks() []ent.Hook { return nil }
