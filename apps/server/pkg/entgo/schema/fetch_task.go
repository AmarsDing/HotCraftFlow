// fetch_task.go 拉取任务表公共 schema 构造面（schema 治理单一化）。
// 收敛 metar/taf/satellite/windfield/radar 五个 fetch_task 表的公共核：
// 字段口径（name/cron/enabled/last_run_at/last_status/last_error）、mixin 集合、
// 隐私策略、索引命名规范统一由此定义，各服务 schema 仅声明业务特有字段。
//
// 既有漂移的归一口径（改造时对齐，均安全迁移）：
//   - last_error 统一 field.Text（radar 原 varchar(1024)，PG 下 ALTER TYPE 无损）；
//   - 运行计数字段统一 Optional+Default(0)（satellite/radar 原为 NOT NULL，放宽安全）；
//   - delete_time 索引统一 StorageKey idx_<table>_deleted_at（radar 原自动名，重建索引）。
// 注意：last_error 从 radar String 改 Text 等变更由 ent 启动迁移自动执行，禁止手工改库。
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// FetchTaskCoreFields 拉取任务表公共字段。
// cronDefault 由服务按数据时次自定（metar/satellite/taf="*/10 * * * *"，
// windfield="0 4,10,16,22 * * *"，radar="*/6 * * * *"）。
func FetchTaskCoreFields(cronDefault string) []ent.Field {
	return []ent.Field{
		field.String("name").
			Comment("任务名称").
			NotEmpty().
			MaxLen(64),

		field.String("cron").
			Comment("cron 表达式（5 字段）").
			Default(cronDefault).
			MaxLen(32),

		field.Bool("enabled").
			Comment("是否启用").
			Default(true),

		field.Time("last_run_at").
			Comment("最近运行时间").
			Optional().
			Nillable(),

		field.String("last_status").
			Comment("最近状态：SUCCESS/FAILED/RUNNING").
			Optional().
			MaxLen(16),

		field.Text("last_error").
			Comment("最近错误信息").
			Optional(),
	}
}

// RunCountField 最近运行入册数量字段（列名/注释服务自定：
// metar/taf/satellite="last_count"，radar="fetched_count"）。
// 口径统一 Optional+Default(0)（原 satellite/radar 为 NOT NULL，放宽方向迁移安全）。
func RunCountField(name, comment string) ent.Field {
	return field.Int32(name).
		Comment(comment).
		Optional().
		Default(0)
}

// FetchTaskMixins 拉取任务表公共 mixin 集合（五服务完全一致）。
func FetchTaskMixins() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.DeleteBy{},
		mixin.Time{},
		mixin.Remark{},
		appmixin.TenantID{},
	}
}

// ConfigDataPolicy 配置数据隐私策略（放行并由上层鉴权）。
func ConfigDataPolicy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}

// FetchTaskIndexes 拉取任务表公共索引：enabled + delete_time（StorageKey 规范 idx_<table>_deleted_at）。
func FetchTaskIndexes(table string) []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("delete_time").StorageKey("idx_" + table + "_deleted_at"),
	}
}

// TableAnnotation 表注解公共构造（utf8mb4 + 注释开启 + 表注释）。
func TableAnnotation(table, comment string) []entschema.Annotation {
	return []entschema.Annotation{
		entsql.Annotation{
			Table:     table,
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		entschema.Comment(comment),
	}
}
