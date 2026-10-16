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

// StorageLayer 对象存储分层元数据表（2026-08-21 存储分层设计 §6.3.1）。
// 记录每个对象（bucket/object_name）当前所在存储层（hot/warm/cold），
// 由 pkg/storage.LayerResolver 对接；DB 中业务表 file_path 不含层前缀，层信息只存于本表。
// 机器写入（上传/迁移/删除回调），无人工编辑场景，故不含 CreateBy/UpdateBy/TenantID。
type StorageLayer struct {
	ent.Schema
}

func (StorageLayer) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "storage_layers",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("对象存储分层元数据表"),
	}
}

func (StorageLayer) Fields() []ent.Field {
	return []ent.Field{
		field.String("bucket").
			Comment("存储桶（一级子目录）：radar/windfield/satellite/images 等").
			NotEmpty().
			MaxLen(64),

		field.String("object_name").
			Comment("对象名（bucket 内相对路径），如 SINGLE/20260821/xxx.bin").
			NotEmpty().
			MaxLen(512),

		field.String("layer").
			Comment("当前所在层：hot-热 / warm-温 / cold-冷").
			NotEmpty().
			MaxLen(16).
			Default("hot"),

		field.Time("moved_at").
			Comment("最近一次迁入当前层的时间（retention 降级判定口径）"),

		field.Int64("size_bytes").
			Comment("对象大小（字节），用于分层容量统计与迁移校验").
			Default(0).
			NonNegative(),

		field.String("checksum").
			Comment("内容 SHA-256（hex），迁移完整性校验用；Q5 决议保留").
			MaxLen(64).
			Optional().
			Nillable(),
	}
}

func (StorageLayer) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

func (StorageLayer) Edges() []ent.Edge {
	return nil
}

func (StorageLayer) Indexes() []ent.Index {
	return []ent.Index{
		// 对象唯一性：一个 (bucket, object_name) 全局唯一一条层记录。
		index.Fields("bucket", "object_name").Unique(),
		// retention 降级扫描口径：WHERE layer = ? AND moved_at < ?。
		index.Fields("layer", "moved_at"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致。
func (StorageLayer) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
