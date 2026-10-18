package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// MapDispatchUnit 分发单元（⑤ MAP，契约 00-shared §5 / 详细设计 §1.3）。
// 三条铁律（概要设计 §4.3）由应用层审批门 + 本表 CHECK 约束双保险：
//   publish_state='published' 要求 approval_state='approved' 且 approver ≠ submitted_by。
// 可见性默认恒为 private；改公开必须填 visibility_override_reason 且经审批（NFR-MAP-01）。
type MapDispatchUnit struct {
	ent.Schema
}

func (MapDispatchUnit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "map_dispatch_units",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
			// 🔴 铁律 DB 双保险（详细设计 §3.1）：
			// 绕过应用层的直接 DB 写入也无法让「未审批 / 自批」的单元进入 published。
			// NOTE: 部分老版本 MySQL 不强制 CHECK（解析但忽略）；PostgreSQL 5434 强制执行。
			Checks: map[string]string{
				"map_dispatch_publish_requires_approval": "publish_state <> 'published' OR (approval_state = 'approved' AND approver IS NOT NULL AND approver <> submitted_by)",
			},
		},
		entsql.WithComments(true),
		schema.Comment("MAP 分发单元：由 VariantSet 派生，审批门约束 + 发布回执字段（⑤ MAP 详细设计 §1.3）"),
	}
}

// Fields of the MapDispatchUnit.
func (MapDispatchUnit) Fields() []ent.Field {
	return []ent.Field{
		field.String("unit_id").
			Comment("分发单元 ID，du_ 前缀 ULID（契约 00-shared §0.3）").
			Unique().
			NotEmpty(),

		field.String("variant_set_id").
			Comment("来源 VariantSet（vs_ 前缀），消费 mm.mme.variant.ready.v1 生成，可空（手工建单）").
			Optional().
			Nillable(),

		field.String("variant_id").
			Comment("单个变体（v_ 前缀），可空").
			Optional().
			Nillable(),

		field.JSON("targets", []string{}).
			Comment("发布目标列表 [platform:slot]，如「youtube:main」「douyin:a1」；凭据不进契约").
			Optional(),

		field.Time("scheduled_for").
			Comment("计划发布时间，UTC；空 = 审批通过后立即发布").
			Optional().
			Nillable(),

		field.Enum("schedule_mode").
			Comment("排期模式：native（平台原生排期）/ local_worker（本机 worker 到点发）/ none（未排期）").
			Values("native", "local_worker", "none").
			Default("none"),

		field.Enum("visibility_default").
			Comment("🔴 可见性默认，恒 private 起步（NFR-MAP-01）；升公须 override_reason + 审批").
			Values("private", "unlisted", "public").
			Default("private"),

		field.Text("visibility_override_reason").
			Comment("非 private 时必填的升公理由（NFR-17）").
			Optional().
			Nillable(),

		// —— 审批区（状态机 draft → approved / rejected）——
		field.Enum("approval_state").
			Comment("审批状态：draft / approved / rejected").
			Values("draft", "approved", "rejected").
			Default("draft"),

		field.String("submitted_by").
			Comment("提交者（禁自批判据，与 approver 不可相同）").
			NotEmpty(),

		field.String("approver").
			Comment("审批者；必须 ≠ submitted_by，且不得为自动化身份").
			Optional().
			Nillable(),

		field.Time("approved_at").
			Comment("审批通过时间，UTC").
			Optional().
			Nillable(),

		field.Text("reject_reason").
			Comment("驳回原因（驳回时必填）").
			Optional().
			Nillable(),

		// —— 发布区（approve 与 publish 分离，发布须二次确认，ADR-MAP-03）——
		field.Enum("publish_state").
			Comment("发布状态：pending / publishing / published / failed").
			Values("pending", "publishing", "published", "failed").
			Default("pending"),

		field.String("post_id").
			Comment("平台返回的作品 ID（回执字段）").
			Optional().
			Nillable(),

		field.String("post_url").
			Comment("作品链接（回执字段）").
			Optional().
			Nillable(),

		field.Time("published_at").
			Comment("实际发布时间，UTC（回执字段）").
			Optional().
			Nillable(),

		field.String("operator").
			Comment("谁点的发布（不得与 submitted_by 相同的自动化身份）").
			Optional().
			Nillable(),

		field.String("feedback_ref").
			Comment("回流快照引用（FR-PUB-08，Phase 3），可空").
			Optional().
			Nillable(),
	}
}

// Indexes of the MapDispatchUnit.
func (MapDispatchUnit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("variant_set_id"),
		index.Fields("variant_id"),
		index.Fields("scheduled_for"),
		index.Fields("approval_state"),
		index.Fields("publish_state"),
		index.Fields("create_time"),
	}
}

// Edges of the MapDispatchUnit.
func (MapDispatchUnit) Edges() []ent.Edge {
	return nil
}

// Mixin of the MapDispatchUnit.
func (MapDispatchUnit) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
