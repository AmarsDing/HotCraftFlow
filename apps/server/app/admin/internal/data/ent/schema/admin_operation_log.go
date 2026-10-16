package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"
)

// AdminOperationLog holds the schema definition for the AdminOperationLog entity.
type AdminOperationLog struct {
	ent.Schema
}

func (AdminOperationLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "admin_operation_log",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("后台操作日志表（framework_log 库）"),
	}
}

// Fields of the AdminOperationLog.
func (AdminOperationLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").
			Comment("请求ID").
			Optional().
			Nillable(),

		field.String("method").
			Comment("请求方法").
			Optional().
			Nillable(),

		field.String("operation").
			Comment("操作方法").
			Optional().
			Nillable(),

		field.String("path").
			Comment("请求路径").
			Optional().
			Nillable(),

		field.String("module").
			Comment("操作模块").
			Default("").
			MaxLen(64),

		field.String("action").
			Comment("操作类型：create/update/delete/import/export").
			Default("").
			MaxLen(32),

		field.String("target").
			Comment("操作对象").
			Default("").
			MaxLen(128),

		field.String("target_id").
			Comment("操作对象ID").
			Default("").
			MaxLen(64).
			Optional().
			Nillable(),

		field.String("before_data").
			Comment("变更前数据（JSON）").
			SchemaType(map[string]string{
				dialect.MySQL:    "json",
				dialect.Postgres: "jsonb",
			}).
			Optional().
			Nillable(),

		field.String("after_data").
			Comment("变更后数据（JSON）").
			SchemaType(map[string]string{
				dialect.MySQL:    "json",
				dialect.Postgres: "jsonb",
			}).
			Optional().
			Nillable(),

		field.String("referer").
			Comment("请求源").
			Optional().
			Nillable(),

		field.String("request_uri").
			Comment("请求URI").
			Optional().
			Nillable(),

		field.String("request_body").
			Comment("请求体").
			Optional().
			Nillable(),

		field.String("request_header").
			Comment("请求头").
			Optional().
			Nillable(),

		field.String("response").
			Comment("响应信息").
			Optional().
			Nillable(),

		field.Int64("duration").
			Comment("操作耗时（毫秒）").
			Default(0),

		field.Float("cost_time").
			Comment("操作耗时（秒，兼容旧字段）").
			Optional().
			Nillable(),

		field.Uint32("user_id").
			Comment("操作者用户ID").
			Optional().
			Nillable(),

		field.String("username").
			Comment("操作者账号名").
			Optional().
			Nillable(),

		field.String("client_ip").
			Comment("操作者IP").
			Optional().
			Nillable(),

		field.Int32("status_code").
			Comment("状态码").
			Optional().
			Nillable(),

		field.String("reason").
			Comment("操作失败原因").
			Optional().
			Nillable(),

		field.Bool("success").
			Comment("操作成功").
			Optional().
			Nillable(),

		field.String("location").
			Comment("操作地理位置").
			Optional().
			Nillable(),

		field.String("user_agent").
			Comment("浏览器的用户代理信息").
			Optional().
			Nillable(),

		field.String("browser_name").
			Comment("浏览器名称").
			Optional().
			Nillable(),

		field.String("browser_version").
			Comment("浏览器版本").
			Optional().
			Nillable(),

		field.String("client_id").
			Comment("客户端ID").
			Optional().
			Nillable(),

		field.String("client_name").
			Comment("客户端名称").
			Optional().
			Nillable(),

		field.String("os_name").
			Comment("操作系统名称").
			Optional().
			Nillable(),

		field.String("os_version").
			Comment("操作系统版本").
			Optional().
			Nillable(),
	}
}

// Mixin of the AdminOperationLog.
func (AdminOperationLog) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateTime{},
	}
}

// Indexes of the AdminOperationLog.
func (AdminOperationLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "create_time"),
		index.Fields("module", "action"),
		index.Fields("create_time"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (AdminOperationLog) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
