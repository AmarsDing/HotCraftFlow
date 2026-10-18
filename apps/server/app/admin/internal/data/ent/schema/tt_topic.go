package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// Topic 话题（② TT 热点追踪核心实体，文档 02-TT 详细设计 §1.2）。
//
// 评审增补：
//   - B-3：热度四因子取值与生效权重必须落盘（NFR-TT-05 可解释门禁），否则历史点不可追溯。
//   - C-6：platforms 列表（滑动 24h 窗口 distinct 平台，契约必填）。
//   - C-15：stage_duration_points（当前阶段已持续采样点数，按点数计，B-7 口径）。
//   - A-15：weight_epoch（权重口径断点，看板分段渲染与伪突发抑制依据）。
//   - B-1：merge_lock / needs_review（人工干预后自动聚类不得再拆散）。
//   - M-17：索引改为 (is_archived, stage, update_time)，归档任务落 is_archived。
type Topic struct {
	ent.Schema
}

func (Topic) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "tt_topics",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("TT 话题：跨平台同一事件的归并单元，热度/阶段/动态指标的元数据载体"),
	}
}

// Fields of the Topic.
func (Topic) Fields() []ent.Field {
	return []ent.Field{
		field.String("topic_id").
			Comment("话题 ID，tp_ 前缀 ULID（契约 §0.3）").
			Unique().
			NotEmpty(),

		field.String("api_version").
			Comment("契约版本").
			Default("1.0"),

		field.String("label").
			Comment("话题标签（代表标题，M-19 选取规则：gate=pass 优先 → workability 降序 → 互动降序 → collected_at 升序）"),

		field.Enum("category").
			Comment("分类：A_hot|B_material|C_market|D_compliance（决定候选路由）").
			NamedValues(
				"Hot", "A_hot",
				"Material", "B_material",
				"Market", "C_market",
				"Compliance", "D_compliance",
			),

		field.Enum("region").
			Comment("区域：cn|intl|global").
			NamedValues(
				"Cn", "cn",
				"Intl", "intl",
				"Global", "global",
			).
			Default("cn"),

		field.Enum("stage").
			Comment("生命周期阶段（评审 B-6 补 dormant 兜底态）").
			NamedValues(
				"Latent", "latent",
				"Rising", "rising",
				"Peak", "peak",
				"Decaying", "decaying",
				"LongTail", "long_tail",
				"Dormant", "dormant",
			).
			Default("latent"),

		field.Time("stage_since").
			Comment("进入当前阶段的样本时间（UTC）").
			Optional(),

		field.Int("stage_duration_points").
			Comment("当前阶段内已写入的样本点数（B-7：按点数计而非墙钟时长；C-15）").
			Default(0),

		field.Float("heat_score").
			Comment("当前热度（0-100 相对可比分数，冗余便于排序）").
			Default(0),

		field.Float("velocity").
			Comment("当前 v_ewma，单位：分/小时（M-2 统一口径）").
			Default(0),

		field.Float("accel").
			Comment("当前加速度").
			Default(0),

		// —— B-3：热度四因子取值与生效权重（NFR-TT-05 硬要求，缺则历史点不可解释）——
		field.Float("rank_pct").Comment("榜内相对位置因子取值 0-1").Default(0),
		field.Float("eng_pct").Comment("互动对数分位因子取值 0-1（A-2 平台内分位）").Default(0),
		field.Float("freshness").Comment("新鲜度因子取值 0-1（A-4：样本时间口径 + 地板 0.05）").Default(0),
		field.Float("breadth").Comment("跨平台扩散度因子取值 0-1（A-5 零点平移）").Default(0),
		field.Float("w_rank_eff").Comment("本次实际生效权重（A-6 缺失重分配后）").Default(0),
		field.Float("w_eng_eff").Comment("本次实际生效权重").Default(0),
		field.Float("w_fresh_eff").Comment("本次实际生效权重").Default(0),
		field.Float("w_breadth_eff").Comment("本次实际生效权重").Default(0),
		field.JSON("missing_factors", []string{}).
			Comment("本次缺失的因子名（rank/eng/fresh/breadth）").
			Optional(),
		field.String("weight_epoch").
			Comment("权重口径 epoch（A-15：sha256(WeightConfig.id)[:8]，变更断点标记）").
			Optional().
			Nillable(),

		// —— C-6 / S-17：滑动 24h 窗口的 distinct 平台 ——
		field.Int("platform_count").
			Comment("近 24h 有贡献的 distinct 平台数").
			Default(0),
		field.JSON("platforms", []string{}).
			Comment("近 24h 窗口内的平台列表（契约 heat.breadth.platforms 必填）").
			SchemaType(map[string]string{dialect.MySQL: "json"}).
			Optional(),

		field.Time("first_seen_at").
			Comment("首次出现的样本时间（UTC）"),
		field.Time("last_seen_at").
			Comment("最近出现的样本时间（UTC）"),

		field.JSON("origin", map[string]any{}).
			Comment("溯源：{first_platform, first_url, first_intel_id}").
			SchemaType(map[string]string{dialect.MySQL: "json"}).
			Optional(),

		field.String("representative_intel_id").
			Comment("主条目 it_ ID（M-19 同 label 选取规则）").
			Optional().
			Nillable(),

		// —— B-1：人工干预锁 ——
		field.Bool("merge_lock").
			Comment("人工合并/拆分后置 true，自动聚类不得再拆散").
			Default(false),
		field.Bool("needs_review").
			Comment("低置信归并（0.70≤s<0.82）待人工复核").
			Default(false),

		field.Bool("is_archived").
			Comment("已归档（M-17 归档任务：last_seen_at < now-30d 或 dormant>7d）").
			Default(false),
	}
}

// Edges of the Topic.
func (Topic) Edges() []ent.Edge {
	return nil
}

// Indexes of the Topic.
func (Topic) Indexes() []ent.Index {
	return []ent.Index{
		// M-17：看板/候选扫描主过滤路径
		index.Fields("is_archived", "stage", "last_seen_at"),
		index.Fields("heat_score"),
		index.Fields("last_seen_at"),
	}
}

// Mixin of the Topic.
func (Topic) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
