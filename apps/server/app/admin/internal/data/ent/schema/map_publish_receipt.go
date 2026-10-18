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

// MapPublishReceipt 发布回执（⑤ MAP，FR-PUB-07 / 详细设计 §1.5）。
// post_id / 时间 / 可见性 / 操作者全记录，可导出审计。
// (dispatch_unit_id, platform_key) 唯一索引是发布幂等（NFR-MAP-05）的 DB 兜底：
// 应用层 Redis SETNX 锁防并发，DB 唯一约束防重入，双保险。
type MapPublishReceipt struct {
	ent.Schema
}

func (MapPublishReceipt) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "map_publish_receipts",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MAP 发布回执：一个分发单元在一个平台至多一条（幂等唯一索引）"),
	}
}

// Fields of the MapPublishReceipt.
func (MapPublishReceipt) Fields() []ent.Field {
	return []ent.Field{
		field.String("receipt_id").
			Comment("回执 ID，pr_ 前缀 ULID（契约 00-shared §0.3）").
			Unique().
			NotEmpty(),

		field.String("dispatch_unit_id").
			Comment("关联分发单元（du_ 前缀）").
			NotEmpty(),

		field.String("account_id").
			Comment("发布所用账号（pa_ 前缀），可空").
			Optional().
			Nillable(),

		field.String("platform_key").
			Comment("目标平台（与渠道/适配器 PlatformKey 一致）").
			NotEmpty(),

		field.String("post_id").
			Comment("平台返回的作品 ID").
			Optional().
			Nillable(),

		field.String("post_url").
			Comment("作品链接").
			Optional().
			Nillable(),

		field.Enum("visibility").
			Comment("实际对外可见性（🔴 验收随机抽查要求全部 private/unlisted，无一无理由 public）").
			Values("private", "unlisted", "public"),

		field.Time("published_at").
			Comment("实际发布时间，UTC"),

		field.String("operator").
			Comment("谁点的发布").
			NotEmpty(),

		field.String("request_payload_hash").
			Comment("请求体 SHA-256（防篡改，可空）").
			Optional().
			Nillable(),

		field.String("response_raw_ref").
			Comment("平台原始响应引用（🔴 已脱敏后存储，可空）").
			Optional().
			Nillable(),
	}
}

// Indexes of the MapPublishReceipt.
func (MapPublishReceipt) Indexes() []ent.Index {
	return []ent.Index{
		// 🔴 幂等 DB 兜底（详细设计 §4.1）：同 unit × 同 platform 至多一条回执。
		index.Fields("dispatch_unit_id", "platform_key").Unique(),
		index.Fields("account_id"),
		index.Fields("platform_key"),
		index.Fields("published_at"),
	}
}

// Edges of the MapPublishReceipt.
func (MapPublishReceipt) Edges() []ent.Edge {
	return nil
}

// Mixin of the MapPublishReceipt.
func (MapPublishReceipt) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
