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

// MmeRenderJob 渲染任务（④ MME，详细设计 §1.5）。
// asynq 消费（队列 mme_render，默认并发 2）；失败保留中间产物（NFR-MME-08）可断点重试。
// 进度与 ETA 向上汇报——不得让用户误以为卡死（需求 §9-6 诚实声明）。
type MmeRenderJob struct {
	ent.Schema
}

func (MmeRenderJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_render_jobs",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 渲染任务（job_ 前缀；asynq 队列 mme_render 消费）"),
	}
}

// Fields of the MmeRenderJob.
func (MmeRenderJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("渲染任务 ID，job_ 前缀 ULID").
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewRenderJobID),

		field.String("variant_id").
			Comment("变体 ID（v_ 前缀）").
			MaxLen(36),
		field.String("state").
			Comment("状态：queued|running|done|failed|cancelled").
			MaxLen(12).
			Default("queued"),
		field.Float("progress").
			Comment("进度 0-1").
			Default(0),
		field.Time("started_at").
			Comment("开渲时间（ETA 推算基准）").
			Optional().
			Nillable(),
		field.Time("finished_at").
			Comment("完成时间").
			Optional().
			Nillable(),
		field.String("worker_id").
			Comment("消费者实例标识（排查归属）").
			MaxLen(64).
			Default(""),
		field.String("error_code").
			Comment("错误码：MME_RENDER_FAIL 等").
			MaxLen(48).
			Optional().
			Nillable(),
		field.String("error_msg").
			Comment("错误摘要（ffmpeg stderr 截断 2KB）").
			MaxLen(2048).
			Optional().
			Nillable(),
		field.String("intermediate_path").
			Comment("失败保留的中间产物路径（断点续渲/排查）").
			MaxLen(1024).
			Optional().
			Nillable(),
		field.Int32("attempt").
			Comment("已尝试次数（重试 +1）").
			Default(0),
	}
}

// Indexes of the MmeRenderJob.
func (MmeRenderJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("variant_id"),
		index.Fields("state"),
		index.Fields("state", "create_time"),
	}
}

// Edges of the MmeRenderJob.
func (MmeRenderJob) Edges() []ent.Edge { return nil }

// Mixin of the MmeRenderJob.
func (MmeRenderJob) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeRenderJob.
func (MmeRenderJob) Hooks() []ent.Hook { return nil }
