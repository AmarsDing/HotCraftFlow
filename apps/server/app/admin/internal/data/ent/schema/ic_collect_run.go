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

// IntelCollectRun 采集运行记录（01-IC/03 + 评审 B-14，契约 §0.3 run_ 前缀）。
// 审计「谁在何时采了什么」；run_id 强制进日志（评审 M-7），串起一次采集全链。
type CollectRun struct {
	ent.Schema
}

func (CollectRun) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_collect_runs",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("采集运行记录：每次采集触发的审计与统计"),
	}
}

// Fields of the CollectRun.
func (CollectRun) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("运行 ID，run_ 前缀 ULID（契约 §0.3 内部实体）").
			MaxLen(32).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(func() string { return contract.NewID(contract.PrefixCollectRun) }),

		field.String("source_id").
			Comment("信源 ID").
			MaxLen(30),
		field.String("trigger").
			Comment("触发方式：cron|manual|retry").
			MaxLen(10).
			Default("cron"),
		field.Time("started_at").
			Comment("开始时间 UTC"),
		field.Time("finished_at").
			Comment("结束时间 UTC，可空（运行中为空）").
			Optional().
			Nillable(),
		field.Int32("fetched").
			Comment("采集到原始片段数").
			Default(0),
		field.Int32("normalized").
			Comment("归一化成功数").
			Default(0),
		field.Int32("merged").
			Comment("合并进既有簇数").
			Default(0),
		field.Int32("blocked").
			Comment("闸门阻断数").
			Default(0),
		field.Int32("skipped").
			Comment("跳过数（限流/熔断/robots）").
			Default(0),
		field.String("status").
			Comment("运行状态：running|succeeded|failed").
			MaxLen(12).
			Default("running"),
		field.String("error_code").
			Comment("失败时的错误码（IC_*）").
			MaxLen(40).
			Optional().
			Nillable(),
	}
}

// Indexes of the CollectRun.
func (CollectRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_id", "started_at"),
		index.Fields("status"),
	}
}

// Edges of the CollectRun.
func (CollectRun) Edges() []ent.Edge {
	return nil
}

// Mixin of the CollectRun.
func (CollectRun) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
