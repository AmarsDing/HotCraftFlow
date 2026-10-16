package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/tx7do/go-utils/entgo/mixin"

	adminV1 "framework/api/gen/go/admin/v1"
	appmixin "framework/pkg/entgo/mixin"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

func (Task) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "sys_tasks",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("任务表"),
	}
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("type").
			Comment("任务类型").
			NamedValues(
				"Periodic", "PERIODIC",
				"Delay", "DELAY",
				"WaitResult", "WAIT_RESULT",
			).
			Optional().
			Nillable(),

		field.String("type_name").
			Comment("任务执行类型名").
			Unique().
			Optional().
			Nillable(),

		field.String("name").
			Comment("任务显示名（人类可读；留空时前端回退显示 type_name）").
			Optional().
			Nillable(),

		field.String("task_payload").
			Comment("任务数据").
			SchemaType(map[string]string{
				dialect.MySQL:    "json",
				dialect.Postgres: "jsonb",
			}).
			Optional().
			Nillable(),

		field.String("cron_spec").
			Comment("cron表达式").
			Optional().
			Nillable(),

		field.JSON("task_options", &adminV1.TaskOption{}).
			Comment("任务选项").
			Optional(),

		field.Bool("enable").
			Comment("启用/禁用任务").
			Optional().
			Nillable(),
	}
}

// Mixin of the Task.
func (Task) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.Remark{},
		appmixin.TenantID{},
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (Task) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
