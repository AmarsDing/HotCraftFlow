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

// MapPublishChannel 平台渠道（⑤ MAP，详细设计 §1.1）。
// 渠道 = 平台的一个发布槽位（platform_key:slot），DispatchUnit.targets 以此解析。
// capability 为能力声明快照（目标态，详细设计 §2.2），不适配即明确报错（ADR-MAP-05）。
type MapPublishChannel struct {
	ent.Schema
}

func (MapPublishChannel) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "map_publish_channels",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("MAP 平台渠道：platform:slot 槽位 + 能力声明快照（⑤ MAP 详细设计 §1.1）"),
	}
}

// Fields of the MapPublishChannel.
func (MapPublishChannel) Fields() []ent.Field {
	return []ent.Field{
		field.String("channel_id").
			Comment("渠道 ID，ch_ 前缀 ULID（⑤ MAP 内部实体）").
			Unique().
			NotEmpty(),

		field.String("platform_key").
			Comment("平台键（douyin/youtube/...），须与 VariantSet.copy 键及适配器 PlatformKey() 一致").
			NotEmpty(),

		field.String("slot").
			Comment("槽位名（main/a1/...），与 platform_key 组成唯一槽位").
			NotEmpty(),

		field.Enum("region").
			Comment("区域：cn（国内）/intl（国外）/global（跨市场），契约 00-shared §1").
			NamedValues(
				"Cn", "cn",
				"Intl", "intl",
				"Global", "global",
			),

		field.Bool("enabled").
			Comment("是否启用").
			Default(true),

		field.JSON("capability", map[string]any{}).
			Comment("能力声明快照（video/im[...]/native_schedule/draft_state/analytics/limits），不支持即明确报错").
			Optional(),

		field.String("adapter_version").
			Comment("适配器版本（去灰用），可空").
			Optional().
			Nillable(),

		field.Int("priority").
			Comment("发布优先级（越大越先）").
			Default(0),
	}
}

// Indexes of the MapPublishChannel.
func (MapPublishChannel) Indexes() []ent.Index {
	return []ent.Index{
		// DispatchUnit.targets 的 platform:slot 解析唯一性。
		index.Fields("platform_key", "slot").Unique(),
		index.Fields("region"),
		index.Fields("enabled"),
	}
}

// Edges of the MapPublishChannel.
func (MapPublishChannel) Edges() []ent.Edge {
	return nil
}

// Mixin of the MapPublishChannel.
func (MapPublishChannel) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
