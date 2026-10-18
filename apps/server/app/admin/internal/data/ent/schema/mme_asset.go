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

// MmeAsset 素材去重库（④ MME，FR-MIX-07；详细设计 §1.2）。
// 去重逻辑：下载完成算指纹 → 查 fingerprint → 命中则 ref_count++ 并删除新下载文件。
// ref_count 归零且过留存期后可回收文件与记录（NFR-MME-04）。
type MmeAsset struct {
	ent.Schema
}

func (MmeAsset) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "mme_assets",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MME 素材去重库（as_ 前缀；fingerprint 唯一，ref_count 引用计数）"),
	}
}

// Fields of the MmeAsset.
func (MmeAsset) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("素材 ID，as_ 前缀 ULID").
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewAssetID),

		field.String("fingerprint").
			Comment("内容指纹 SHA-256 hex（前 64MB 采样 + 文件大小 + 时长；唯一去重键）").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("source_url").
			Comment("来源 URL（本地导入为空）").
			MaxLen(1024).
			Optional().
			Nillable(),
		field.String("local_path").
			Comment("媒体文件存储路径（mme_media_storage 管理，对象存储/本地相对路径）").
			MaxLen(1024),
		field.Float("duration_sec").
			Comment("时长（秒）").
			Default(0),
		field.Int32("width").
			Comment("原始宽").
			Default(0),
		field.Int32("height").
			Comment("原始高").
			Default(0),
		field.Int64("size_bytes").
			Comment("文件字节数").
			Default(0),
		field.Int32("ref_count").
			Comment("引用计数（任务数）；归零且过留存期后可回收").
			Default(1),
		field.String("meta_title").
			Comment("yt-dlp/文件元数据标题（展示用）").
			MaxLen(512).
			Optional().
			Nillable(),
		field.String("subtitle_path").
			Comment("源字幕路径（yt-dlp --write-subs 产物；ASR source_subtitle 通路用）").
			MaxLen(1024).
			Optional().
			Nillable(),
	}
}

// Indexes of the MmeAsset.
func (MmeAsset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_url"),
	}
}

// Edges of the MmeAsset.
func (MmeAsset) Edges() []ent.Edge { return nil }

// Mixin of the MmeAsset.
func (MmeAsset) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Hooks of the MmeAsset.
func (MmeAsset) Hooks() []ent.Hook { return nil }
