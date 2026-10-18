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

// IntelSource 信源注册表（01-IC/03 §2.1）。
// 配置驱动：新增信源不改代码（FR-INT-01）；kind（采什么）× method（怎么采）组合矩阵见 03 §1 / 评审 S-20。
// 评审修正：S-8（robots 豁免理由）、S-12（熔断字段）、S-17（schema hint）、M-29（回流权重）。
type IntelSource struct {
	ent.Schema
}

func (IntelSource) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_intel_sources",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("信源注册表：热榜/关键词/创作者/RSS/竞品五类信源及其调度与熔断状态"),
	}
}

// Fields of the IntelSource.
func (IntelSource) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("信源 ID，src_ 前缀 ULID（契约 §0.3 内部实体，不进契约）").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(func() string { return contract.NewID(contract.PrefixIntelSource) }),

		field.String("platform").
			Comment("来源平台标识（契约 §1 平台枚举表）").
			MaxLen(32),
		field.String("region").
			Comment("区域：cn|intl|global").
			MaxLen(8).
			Default("cn"),
		field.String("kind").
			Comment("信源类型：hotlist|keyword|creator|rss|competitor（采什么）").
			MaxLen(16),
		field.String("method").
			Comment("采集路：hotlist|api|rss|crawl|sidecar（怎么采；与 kind 组合受矩阵约束，S-20）").
			MaxLen(16),
		field.JSON("config", map[string]any{}).
			Comment("采集器私有配置（写入前 json.Valid 校验，S-17；凭据不进此字段）").
			Optional(),
		field.String("config_schema_hint").
			Comment("配置 schema 提示（如 newsnow.hotlist/v1），前端动态表单渲染依据（S-17）").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Bool("enabled").
			Comment("运营启停开关").
			Default(true),
		field.Int32("interval_minutes").
			Comment("采集周期（分钟），调度唯一真值（评审 B-11）；按 method 分级下限：hotlist≥30/rss≥5/api≥10/crawl≥60（S-18）").
			Default(30),
		field.Int32("rate_limit_per_min").
			Comment("令牌桶速率（每分钟请求数）；平台级 key 优先于信源级（03 §3.4）").
			Default(20),
		field.Int32("priority").
			Comment("调度优先级，越大越靠前").
			Default(0),
		field.String("credential_ref").
			Comment("凭据引用（指向凭据存储，非明文；NFR-IC-06）").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Float("weight").
			Comment("选题权重（回流 FR-INT-16 更新，仅影响调度优先级，范围 0.5~2.0，评审 M-29）").
			Default(1.0),

		// —— robots 豁免（评审 S-8：api/rss/hotlist 路豁免必须登记理由）——
		field.Bool("robots_exempt").
			Comment("robots 检查豁免（api/rss/hotlist 路按平台条款豁免）").
			Default(false),
		field.String("robots_exempt_reason").
			Comment("豁免理由（robots_exempt=true 时必填，审计可自证）").
			MaxLen(255).
			Optional().
			Nillable(),

		// —— 熔断（评审 S-12：fail_streak≥5 → paused + 30m 冷却）——
		field.String("status").
			Comment("熔断状态：active|paused|disabled").
			MaxLen(12).
			Default("active"),
		field.Time("paused_until").
			Comment("自动暂停截止时间，到期自动恢复").
			Optional().
			Nillable(),
		field.Int32("fail_streak").
			Comment("连续失败次数").
			Default(0),

		// —— 健康度统计（NFR-IC-05；7 日滚动口径由 health_job 每日重算，评审 S-14）——
		field.Time("last_run_at").
			Comment("最近一次调度触发时间，可空").
			Optional().
			Nillable(),
		field.Time("last_success_at").
			Comment("最近一次成功采集时间，可空").
			Optional().
			Nillable(),
		field.Int32("success_count_7d").
			Comment("近 7 日成功次数（health_job 重算，非累加）").
			Default(0),
		field.Int32("fail_count_7d").
			Comment("近 7 日失败次数（health_job 重算）").
			Default(0),
		field.String("last_error").
			Comment("最近一次错误（脱敏，不含凭据）").
			MaxLen(512).
			Optional().
			Nillable(),
	}
}

// Indexes of the IntelSource.
func (IntelSource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "enabled"),
		index.Fields("region"),
		index.Fields("kind"),
		index.Fields("enabled"),
		index.Fields("priority"),
		index.Fields("status"),
	}
}

// Edges of the IntelSource.
func (IntelSource) Edges() []ent.Edge {
	return nil
}

// Mixin of the IntelSource.
func (IntelSource) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
