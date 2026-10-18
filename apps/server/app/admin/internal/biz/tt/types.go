package tt

import "time"

// SnapshotPoint 是 (topic, platform, t) 维度的热度快照内部结构。
// 对齐 02-TT 03-详细设计.md §1.1 measurement `topic_heat` 的 fields，
// 含评审增补（B-3 因子取值、A-14 t_raw/sample_count/late、A-15 weight_epoch）。
//
// 序列说明：
//   - 该结构是 biz ↔ data 的传输体；同时作为 mm.trend.snapshot.v1 的内部消息载荷
//     （契约 §8.2 标注其为「内部结构，非七契约之一」，不进 JSON Schema 门禁）。
//   - 无值语义统一用 -1 哨兵（C-7：仅 InfluxDB field 内部用哨兵；契约输出一律转 null）。
type SnapshotPoint struct {
	TopicID  string    `json:"topic_id"`  // tp_ 前缀
	Platform string    `json:"platform"`  // weibo / bilibili / youtube ……
	TBucket  time.Time `json:"t_bucket"`  // 时间桶对齐后的 UTC 时间戳（InfluxDB 写入时间，A-14）
	TRaw     time.Time `json:"t_raw"`     // 原始样本时间（_as_of 或 collected_at，UTC）

	// —— 榜单与互动（-1 = 无/未知，详设 §1.1）——
	Rank      int32   `json:"rank"`       // 榜单排名，1 最热；无榜 -1
	RankTotal int32   `json:"rank_total"` // 榜单总长；无榜 -1
	Views     int64   `json:"views"`
	Likes     int64   `json:"likes"`
	Comments  int64   `json:"comments"`
	EngRate   float64 `json:"eng_rate"` // 互动/曝光；无曝光 -1

	// —— 热度与动态（M2 normalize/dynamics 反算后回填；M1 统一写 -1 哨兵 = 未计算）——
	HeatScore float64 `json:"heat_score"`
	Velocity  float64 `json:"velocity"`
	Accel     float64 `json:"accel"`

	// —— B-3：四因子取值与生效权重（NFR-TT-05 可解释性；M1 占位 -1/0，M2 normalize 落盘）——
	RankPct     float64  `json:"rank_pct"`
	EngPct      float64  `json:"eng_pct"`
	Freshness   float64  `json:"freshness"`
	Breadth     float64  `json:"breadth"`
	WRankEff    float64  `json:"w_rank_eff"`
	WEngEff     float64  `json:"w_eng_eff"`
	WFreshEff   float64  `json:"w_fresh_eff"`
	WBreadthEff float64  `json:"w_breadth_eff"`
	Missing     []string `json:"missing,omitempty"` // 缺失因子名（rank/eng/fresh/breadth）
	WeightEpoch string   `json:"weight_epoch,omitempty"` // A-15 权重口径断点

	// —— 采样质量 ——
	IsInterpolated bool  `json:"is_interpolated"` // 上游明确声明的补采点（查询层插值不写库，M-13）
	Late           bool  `json:"late"`            // A-14：_as_of 落后 late_hours 的迟到回填点
	SampleCount    int32 `json:"sample_count"`    // 同桶折叠条数（A-14；被守卫丢弃的重复样本计数见指标）
}

// Sentinel 哨兵值（仅 InfluxDB field 内部；C-7 契约输出侧必须转为 null/0）。
const (
	SentinelRank      int32   = -1 // 无榜
	SentinelEngRate   float64 = -1 // 无曝光
	SentinelHeat      float64 = -1 // 热度未计算（M1；M2 normalize 回填后必为 0-100）
	SentinelFactor    float64 = -1 // 因子未计算
	SentinelRankTotal int32   = -1 // 榜长未知
)
