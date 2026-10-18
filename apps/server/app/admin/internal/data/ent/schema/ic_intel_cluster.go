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

// IntelCluster 去重聚类簇（01-IC/03 §2.3 + 评审 S-2/S-5）。
// 生命周期：open → 48h 无更新自动 closed（retention.cluster_idle_close_hours）。
// centroid 存在 Redis（centroid_ref key），成员 >50 冻结（评审 S-4）。
type IntelCluster struct {
	ent.Schema
}

func (IntelCluster) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_intel_clusters",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("去重聚类簇：同一事件跨源归并的代表，TT 话题聚类的重要输入"),
	}
}

// Fields of the IntelCluster.
func (IntelCluster) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("簇 ID，cl_ 前缀 ULID").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewClusterID),

		field.String("label").
			Comment("代表标题（取最早条目标题）").
			MaxLen(512).
			Default(""),
		field.Int32("member_count").
			Comment("归并条数").
			Default(1),
		field.Int32("platform_count").
			Comment("覆盖平台数（判断是否真热点而非单平台噪音，US-2）").
			Default(1),
		field.String("representative_id").
			Comment("主条目 intel_id（最早条目）").
			MaxLen(30).
			Optional().
			Nillable(),
		field.String("region").
			Comment("区域：cn|intl|global（评审 S-2：跨 region 误合并防护）").
			MaxLen(8).
			Default("cn"),
		field.String("category").
			Comment("类目（归一后由理解层回填，S-2）").
			MaxLen(16).
			Optional().
			Nillable(),

		field.Time("first_seen_at").
			Comment("首次见时间 UTC"),
		field.Time("last_seen_at").
			Comment("最近见时间 UTC（>48h 未更新 → closed，S-2）"),

		field.String("merged_by").
			Comment("本次级联级：url|fingerprint|embedding|manual").
			MaxLen(16).
			Default("url"),
		field.String("centroid_ref").
			Comment("中心向量 Redis key（成员 ≤50 增量更新，>50 冻结，评审 S-4）").
			MaxLen(64).
			Optional().
			Nillable(),
		field.String("status").
			Comment("簇状态：open|closed").
			MaxLen(10).
			Default("open"),
		field.Time("closed_at").
			Comment("关闭时间，可空").
			Optional().
			Nillable(),
	}
}

// Indexes of the IntelCluster.
func (IntelCluster) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "last_seen_at"),
		index.Fields("region", "status"),
		index.Fields("category"),
	}
}

// Edges of the IntelCluster.
func (IntelCluster) Edges() []ent.Edge {
	return nil
}

// Mixin of the IntelCluster.
func (IntelCluster) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
