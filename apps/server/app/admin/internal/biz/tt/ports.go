package tt

import (
	"context"
	"time"
)

// ============================================================================
// 端口定义（02-TT 02-概要设计.md §3：biz 层禁止 import data，由 data/tt 实现，
// wire.Bind 绑定）。
// ============================================================================

// PointSink 快照点写入端口（data/tt 的批处理器实现：攒批 → 写 InfluxDB →
// 失败批次转 SnapshotBuffer 兜底，A-16）。
//
// Record 允许并发调用；实现方内部串行化批处理。
// Record 返回 nil 仅表示「已受理」（入批队列），不代表已落库。
type PointSink interface {
	// Record 受理一个快照点。背压判定在调用方（BufferDepth）。
	Record(ctx context.Context, p SnapshotPoint) error
	// Flush 强制冲刷当前未满批次（优雅停机/定时器）。
	Flush(ctx context.Context) error
	// Pending 返回尚未落库的点数（批队列 + 失败缓冲）。
	Pending(ctx context.Context) int
	// Close 停止批处理 goroutine 并冲刷余量。
	Close(ctx context.Context) error
}

// TimeSeries 时序库直连端口（data/tt/influx.go 唯一 InfluxDB 接触面）。
// 供 worker 重放批、健康检查与生命周期评估的序列查询使用。
type TimeSeries interface {
	// WriteSnapshots 同步写一批点到 bucket_raw。
	// (measurement + topic_id + platform + t_bucket) 重复写覆盖（NFR-TT-07 幂等）。
	WriteSnapshots(ctx context.Context, points []SnapshotPoint) error
	// Ping 健康检查。
	Ping(ctx context.Context) (bool, error)

	// ReadRecentPoints 按 t_raw 倒序读某话题某平台的最近 N 个快照点
	// （生命周期评估推导 streak 用；N≤10 足够——rising 需 2 点、decaying 3 点）。
	// 返回按 t_raw 降序（最新在前）；不足 limit 时返回实际条数。
	ReadRecentPoints(ctx context.Context, topicID, platform string, limit int) ([]SnapshotPoint, error)

	// ReadActiveHeatRange 读近 24h 全体活跃话题的 heat 序列（生命周期 P25/P99 分位用）。
	// 返回 heat 值列表（0-100）；空窗口返回空切片（调用方禁用相对线）。
	ReadActiveHeatRange(ctx context.Context, from, to time.Time) ([]float64, error)

	// ReadPlatforms 列出某话题在 [from, to] 内有采样点的平台集合（迟到回填分平台回放用）。
	ReadPlatforms(ctx context.Context, topicID string, from, to time.Time) ([]string, error)
	// ReadSeriesAsc 按 t_bucket 升序读某话题某平台 [from, to] 内的完整快照点
	// （迟到回填重放 EWMA 链用；返回点含全部 field，不足字段保持哨兵/零值）。
	ReadSeriesAsc(ctx context.Context, topicID, platform string, from, to time.Time) ([]SnapshotPoint, error)
}

// SnapshotBuffer 写失败缓冲端口（A-16：Redis Stream 死信队列）。
//
// 语义：XADD MAXLEN ≈ 上限，超限 XTRIM 最旧 + 计数 tt_influx_dropped_total。
// 需求 §9 诚实口径：「尽力不丢 + 显式告知丢失量」，不保证绝对不丢。
type SnapshotBuffer interface {
	// Enqueue 入队一批失败点，返回因流上限被裁剪丢弃的点数（>0 必须计 dropped）。
	Enqueue(ctx context.Context, points []SnapshotPoint) (dropped int, err error)
	// Depth 返回缓冲流深度（背压判定与 tt_influx_buffer_depth 指标）。
	Depth(ctx context.Context) (int64, error)
	// ReadBatch 为重放读取一批（消费组语义，XPENDING 处理）。
	// batchID 为非幂等去重内部句柄，Ack 后失效。
	ReadBatch(ctx context.Context, max int64) (batch []SnapshotPoint, batchID string, err error)
	// Ack 确认批已落库。
	Ack(ctx context.Context, batchID string) error
	// Nack 处理失败：累计 attempt；达到 MaxAttempts 转 poison 并 Ack。
	Nack(ctx context.Context, batchID string, points []SnapshotPoint, maxAttempts int) (poisoned bool, err error)
}

// TopicRepo 话题元数据端口（data/tt/topic_repo.go 实现，PG/Ent）。
type TopicRepo interface {
	// FindTopicIDByClusterKey B-1 Step1 硬键：按 IC cluster_id 查已归属话题。
	// 命中 merge_lock=false 的活跃话题直接返回其 topic_id。
	FindTopicIDByClusterKey(ctx context.Context, clusterKey string) (topicID string, found bool, err error)
	// CreateTopicWithSource 新建话题并关联来源（Step1 未命中或无 cluster_id 的新事件）。
	// label/category/region 取自首条 IntelItem；first/last_seen_at 取首次样本时间。
	CreateTopicWithSource(ctx context.Context, in CreateTopicInput) (topicID string, err error)
	// AttachSource 追加来源关联（同 (topic_id, intel_id) 幂等）。
	AttachSource(ctx context.Context, topicID string, in AttachSourceInput) error
	// TouchSeen 推进 last_seen_at（早到不后退）；收录首日置 first_seen_at。
	TouchSeen(ctx context.Context, topicID string, seenAt time.Time) error
	// GetTopicHeatMeta normalize 取数（M2）：首现时间/分类/近 24h 平台统计。
	// at 为样本时间：pc(t) 窗口 (at-24h, at] 按样本口径计算（B-4 历史可重现：
	// TopicSource 只追加，迟到点重算仍得当时窗口——move/拆分除外，记为已知局限）。
	GetTopicHeatMeta(ctx context.Context, topicID string, at time.Time) (TopicHeatMeta, error)
	// UpdateTopicHeatState 回写热度/动态/因子明细（B-3 落库，NFR-TT-05 门禁）。
	UpdateTopicHeatState(ctx context.Context, topicID string, in TopicHeatUpdate) error
}

// CreateTopicInput 新建话题入参。
type CreateTopicInput struct {
	Label      string
	Category   string // A_hot / B_material / C_market / D_compliance
	Region     string // cn / intl / global
	SeenAt     time.Time
	IntelID    string
	ClusterKey string // 可空（无 cluster_id 时）
	OriginURL  string
	Platform   string
	// TitleSimhash B-1 Step2 召回倒排键（标题 SimHash64；0 = 无标题/无效指纹）
	TitleSimhash uint64
	// Entities 实体集合（confidence≥0.6 过滤后的规范化 (type:value) 串；Step2 召回通道①）
	Entities []string
	// Lang 语言脚本（zh/en/...；Step3 跨语言判定依据）
	Lang string
}

// AttachSourceInput 追加来源入参。
type AttachSourceInput struct {
	IntelID       string
	ClusterKey    string
	Platform      string // A-5 breadth 统计维度（来源平台，冗余落 TopicSource.platform）
	ContributedAt time.Time
	Similarity    float64
	// TitleSimhash B-1 Step2 召回倒排键（标题 SimHash64；0 = 无标题/无效指纹）
	TitleSimhash uint64
	// Entities 实体集合（Step2 召回通道①，写入 tt_entity_index）
	Entities []string
	// Lang 语言脚本（Step3 跨语言判定依据）
	Lang string
}

// TopicHeatMeta 话题热度元数据（normalize 取数一次性回包）。
type TopicHeatMeta struct {
	FirstSeenAt time.Time
	Category    string
	// PlatformCount24h 近 24h 有贡献的 distinct 平台数（A-5 pc(t)，含本条来源）。
	PlatformCount24h int
	// Platforms24h 近 24h 平台列表（C-6 契约 breadth.platforms）。
	Platforms24h []string
}

// TopicHeatUpdate 话题热度状态回写（B-3 因子取值/生效权重落库，NFR-TT-05 门禁）。
type TopicHeatUpdate struct {
	HeatScore, Velocity, Accel          float64
	RankPct, EngPct, Freshness, Breadth float64
	WeightsEff                          HeatWeights
	Missing                             []string
	WeightEpoch                         string
	PlatformCount24h                    int
	Platforms24h                        []string
}

// SnapshotPublisher 快照事件发布端口（C-9：mm.trend.snapshot.v1 由 ingest 产出、
// 由 timeseries writer 消费，InfluxDB 写入只发生在消费端）。
//
// 实现：core 层经 pkg/nats 发布（core NATS 发布也会被 MM_TREND stream 按 subject
// 捕获，与 JetStream API 发布等价，见 pkg/events/streams.go 注释）。
type SnapshotPublisher interface {
	PublishSnapshot(ctx context.Context, p SnapshotPoint, traceID string) error
}

// SnapshotPublished 快照事件的内部载荷（event_id 幂等键由消费侧复用）。
type SnapshotPublished struct {
	EventID string        `json:"event_id"`
	TraceID string        `json:"trace_id"`
	Point   SnapshotPoint `json:"point"`
}

// GuardStore 同桶 last-wins 等原子守卫端口（A-14）。
//
// 由 data/tt 用 Redis Lua 实现（GET-COMPARE-SET 必须原子，否则并发消费下同桶
// 旧点可能覆盖新点，破坏「取 _as_of 最大者」语义）。
type GuardStore interface {
	// TryAcceptSample 原子判定：仅当 epoch 严格大于已存值时接受并更新。
	// 首次（无 key）总是接受。ttl 到期后 key 消失，后续样本重新竞争。
	TryAcceptSample(ctx context.Context, key string, epoch int64, ttl time.Duration) (accepted bool, err error)
}

// ============================================================================
// M2 端口：eng 分位直方图 / 动态链状态 / 权重配置仓。
// ============================================================================

// EngHistogram 互动分位直方图端口（A-3：分桶直方图替代 ZSET 方案——
// 评审已推翻 03 §2.1 的「Redis 有序集合滚动 7 日」：ZRANK 无法跨 7 个日键求秩）。
//
// 语义：ObserveAndPercentile 先 HINCRBY 当日键（0.1 对数粒度 600 桶，TTL 8d），
// 再合并 (t-7d, t] 覆盖的 7 个日键求 ECDF 分位；返回窗口样本总数 n 供调用方
// 做冷启动判定（n < EngMinWindowN → 退化静态基线）。
type EngHistogram interface {
	ObserveAndPercentile(ctx context.Context, platform, category string, t time.Time, engagement float64) (pct float64, n int64, err error)
	// ReadPercentile 只读分位（不 HINCRBY；S-23 迟到回填重算用——迟到点已由
	// ingest 观测过当日键，重算再观测会双倍计数）。category 为话题分类（直方图日键
	// 组成）。返回 -1 表示无窗口数据/故障（调用方保留原值）。
	ReadPercentile(ctx context.Context, platform, category string, t time.Time, engagement float64) (float64, error)
}

// DynamicsStateStore 动态链状态端口（每 (topic, platform) 一条）。
// SaveIfNotAdvanced 为乐观条件写：仅当已存状态的 LastT 未越过 expectedLastT
// （即本次 Load 后无并发推进）才落盘；返回 false 表示并发抢先，调用方放弃本次
// 状态写入（热度仍写入时序，动态下个样本点自愈）。
type DynamicsStateStore interface {
	Load(ctx context.Context, topicID, platform string) (st *DynamicsState, err error)
	SaveIfNotAdvanced(ctx context.Context, topicID, platform string, st DynamicsState, expectedLastT time.Time) (saved bool, err error)
}

// WeightConfigRow 权重配置行（A-15：只追加不改写）。
type WeightConfigRow struct {
	ConfigID      string      // wc_ 前缀 ULID
	Scope         string      // heat / candidate
	Weights       HeatWeights // scope=heat 时有效
	EffectiveFrom time.Time
	ActivatedBy   string
	Note          string
}

// WeightConfigRepo 权重配置端口。
type WeightConfigRepo interface {
	// ActiveHeatWeights 取 at 时刻生效的 heat 配置（effective_from ≤ at 最新一行）。
	// found=false 表示表内无行（调用方用 DefaultHeatWeights 兜底）。
	ActiveHeatWeights(ctx context.Context, at time.Time) (row WeightConfigRow, found bool, err error)
	// ListHeatWeights 查看用历史列表（effective_from 倒序，limit≤0 默认 20）。
	ListHeatWeights(ctx context.Context, limit int) ([]WeightConfigRow, error)
	// AppendHeatWeights 插入新行（effective_from 由调用方取变更时刻，不得回溯）。
	AppendHeatWeights(ctx context.Context, row WeightConfigRow) error
}
