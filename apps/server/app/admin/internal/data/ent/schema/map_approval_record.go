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

// MapApprovalRecord 审批/发布审计记录（⑤ MAP，FR-PUB-07 / 详细设计 §1.4）。
// submit/approve/reject/publish/cancel 全动作落库，谁做的、何时、以何种可见性、
// 携带何种原因，全部可导出审计。自批尝试与自动化身份尝试同样记录（拒绝也要留痕）。
// 内部审计实体，无跨系统契约前缀，API 对外暴露 ent 整型主键。
type MapApprovalRecord struct {
	ent.Schema
}

func (MapApprovalRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "map_approval_records",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MAP 审批与发布操作审计日志（⑤ MAP 详细设计 §1.4，FR-PUB-07）"),
	}
}

// Fields of the MapApprovalRecord.
func (MapApprovalRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("dispatch_unit_id").
			Comment("关联分发单元（du_ 前缀）").
			NotEmpty(),

		field.Enum("action").
			Comment("动作：submit / approve / reject / publish / cancel").
			Values("submit", "approve", "reject", "publish", "cancel"),

		field.String("actor").
			Comment("操作者标识").
			NotEmpty(),

		field.Enum("actor_type").
			Comment("操作者类型：human / automation（自动化身份不得审批/发布）").
			Values("human", "automation"),

		field.String("from_state").
			Comment("动作前审批状态").
			Optional().
			Nillable(),

		field.String("to_state").
			Comment("动作后审批状态").
			Optional().
			Nillable(),

		field.Text("reason").
			Comment("原因（驳回必填；publish 非 private 时留 override_reason）").
			Optional().
			Nillable(),

		field.String("visibility_at_action").
			Comment("动作时刻的可见性（private/unlisted/public）").
			Optional().
			Nillable(),

		field.String("ip").
			Comment("操作者 IP（可空，内部调用无）").
			Optional().
			Nillable(),

		field.String("user_agent").
			Comment("操作者 UA（可空）").
			Optional().
			Nillable(),

		// 被审计的拒绝也属于审计：result 记录 allow/deny，留痕越权尝试。
		field.Enum("result").
			Comment("裁决：allow / deny（deny 即越权尝试痕迹，如自批/自动化/未审批发布）").
			Values("allow", "deny").
			Default("allow"),
	}
}

// Indexes of the MapApprovalRecord.
func (MapApprovalRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dispatch_unit_id"),
		index.Fields("action"),
		index.Fields("actor"),
		index.Fields("create_time"),
	}
}

// Edges of the MapApprovalRecord.
func (MapApprovalRecord) Edges() []ent.Edge {
	return nil
}

// Mixin of the MapApprovalRecord.
func (MapApprovalRecord) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
