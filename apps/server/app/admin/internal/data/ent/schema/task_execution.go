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

// TaskExecution holds the schema definition for the TaskExecution entity.
type TaskExecution struct {
	ent.Schema
}

func (TaskExecution) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "sys_task_executions",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("调度任务执行记录表"),
	}
}

// Fields of the TaskExecution.
func (TaskExecution) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("task_id").
			Comment("关联任务ID（sys_tasks.id；任务删除后记录保留，仅作查询关联）").
			Optional().
			Nillable(),

		field.String("type_name").
			Comment("任务类型名（冗余存储，任务删除后仍可辨识）").
			Optional().
			Nillable(),

		field.Enum("status").
			Comment("执行状态").
			NamedValues(
				"Success", "ExecutionStatus_Success",
				"Failed", "ExecutionStatus_Failed",
			).
			Optional().
			Nillable(),

		field.String("error").
			Comment("失败原因").
			Optional().
			Nillable(),

		field.Time("started_at").
			Comment("开始时间").
			Optional().
			Nillable(),

		field.Time("finished_at").
			Comment("结束时间").
			Optional().
			Nillable(),

		field.Uint64("duration_ms").
			Comment("耗时（毫秒）").
			Optional().
			Nillable(),
	}
}

// Mixin of the TaskExecution.
func (TaskExecution) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateTime{},
	}
}

// Indexes of the TaskExecution.
func (TaskExecution) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
		index.Fields("type_name"),
		index.Fields("create_time"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (TaskExecution) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
