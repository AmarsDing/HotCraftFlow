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

// DeadLetter 死信记录（01-IC/03 §8 + 评审 B-14/S-13）。
// 归一化 schema 校验失败等不可恢复错误落此表，提供重放入口（需记录 replayed_by 审计）。
type DeadLetter struct {
	ent.Schema
}

func (DeadLetter) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_dead_letters",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("死信表：采集/归一化失败消息留存与重放"),
	}
}

// Fields of the DeadLetter.
func (DeadLetter) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("死信 ID（裸 ULID，内部实体未注册契约前缀）").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.ULID),

		field.String("task_type").
			Comment("失败任务类型：collect|normalize|publish|replay").
			MaxLen(20),
		field.JSON("payload", map[string]any{}).
			Comment("失败时的消息载荷（RawFragment 等）"),
		field.String("error_code").
			Comment("错误码（IC_*，03 §8 错误码表）").
			MaxLen(40),
		field.String("error_msg").
			Comment("错误描述（脱敏）").
			MaxLen(1024).
			Optional().
			Nillable(),
		field.Int32("attempt").
			Comment("已尝试次数").
			Default(1),
		field.Time("first_failed_at").
			Comment("首次失败时间 UTC"),
		field.Time("last_failed_at").
			Comment("最近失败时间 UTC"),
		field.String("status").
			Comment("处置状态：pending|replayed|discarded").
			MaxLen(12).
			Default("pending"),
		field.String("replayed_by").
			Comment("重放操作人（审计必填）").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Time("replayed_at").
			Comment("重放时间，可空").
			Optional().
			Nillable(),
	}
}

// Indexes of the DeadLetter.
func (DeadLetter) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "last_failed_at"),
		index.Fields("task_type"),
	}
}

// Edges of the DeadLetter.
func (DeadLetter) Edges() []ent.Edge {
	return nil
}

// Mixin of the DeadLetter.
func (DeadLetter) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
