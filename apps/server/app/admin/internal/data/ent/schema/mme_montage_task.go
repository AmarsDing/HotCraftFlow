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

// MmeMontageTask 加工任务（④ MME，详细设计 §1.1）。
// 任务驱动「素材获取→转写→理解→脚本→矩阵展开→渲染→质检」流水线状态机。
type MmeMontageTask struct {
	ent.Schema
}

func (MmeMontageTask) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_montage_tasks",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 加工任务（task_ 前缀；source_type=url/local/intel/candidate）"),
	}
}

// Fields of the MmeMontageTask.
func (MmeMontageTask) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("任务 ID，task_ 前缀 ULID").
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewMontageTaskID),

		field.String("title").
			Comment("任务标题").
			MaxLen(256).
			Default(""),
		field.String("source_type").
			Comment("来源：url|local|intel|candidate").
			MaxLen(16),
		field.String("source_ref").
			Comment("来源引用：URL / 本地路径 / asset://").
			MaxLen(1024),
		field.String("candidate_id").
			Comment("回指 ② TT 选题（tc_ 前缀），可空").
			MaxLen(36).
			Optional().
			Nillable(),
		field.String("intel_id").
			Comment("回指 ① IC 情报（it_ 前缀），可空").
			MaxLen(36).
			Optional().
			Nillable(),
		field.String("asset_id").
			Comment("去重后的素材 ID（as_ 前缀，mme_assets 外键），下载完成后回填").
			MaxLen(36).
			Optional().
			Nillable(),
		field.Text("user_intent").
			Comment("用户意图（M1-06 user-intent；纳入分析缓存 key，不同意图产出不同章节/报告）").
			Optional().
			Nillable(),
		field.String("state").
			Comment("状态机：created|downloading|downloaded|transcribing|analyzing|expanded|rendering|reviewing|done|failed").
			MaxLen(20).
			Default("created"),
		field.String("prompt_version").
			Comment("分析提示词版本（缓存 key 组成；版本变更即使旧分析缓存失效）").
			MaxLen(32).
			Default("v1.0"),
		field.String("asr_engine").
			Comment("ASR 引擎选择：source_subtitle|whisper|funasr|cloud").
			MaxLen(20).
			Default(""),
		field.String("error_msg").
			Comment("可读错误（下载/分析失败原因；不存堆栈）").
			MaxLen(1024).
			Optional().
			Nillable(),

		// —— 高光候选（B 维度内容版本；手工 EDL 或分析产物，JSON 数组）——
		// 结构：[{id,label,start_sec,end_sec,note,origin:manual|auto}]
		field.JSON("highlights", []map[string]any{}).
			Comment("高光候选列表（内容版本来源；手工 EDL 或 LLM 分析产物）").
			Optional(),

		// —— 扩展显示字段冗余（列表页免联表）——
		field.Float("progress").
			Comment("加工进度 0-1（下载/分析阶段粗粒度上报）").
			Default(0),
	}
}

// Indexes of the MmeMontageTask.
func (MmeMontageTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_type"),
		index.Fields("state"),
		index.Fields("candidate_id"),
		index.Fields("intel_id"),
		index.Fields("asset_id"),
	}
}

// Edges of the MmeMontageTask.
func (MmeMontageTask) Edges() []ent.Edge { return nil }

// Mixin of the MmeMontageTask.
func (MmeMontageTask) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeMontageTask.
func (MmeMontageTask) Hooks() []ent.Hook { return nil }
