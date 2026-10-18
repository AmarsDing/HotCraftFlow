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
// 供 worker 重放批、健康检查与未来 series 查询使用。
type TimeSeries interface {
	// WriteSnapshots 同步写一批点到 bucket_raw。
	// (measurement + topic_id + platform + t_bucket) 重复写覆盖（NFR-TT-07 幂等）。
	WriteSnapshots(ctx context.Context, points []SnapshotPoint) error
	// Ping 健康检查。
	Ping(ctx context.Context) (bool, error)
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
}

// CreateTopicInput 新建话题入参。
type CreateTopicInput struct {
	Label        string
	Category     string // A_hot / B_material / C_market / D_compliance
	Region       string // cn / intl / global
	SeenAt       time.Time
	IntelID      string
	ClusterKey   string // 可空（无 cluster_id 时）
	OriginURL    string
	Platform     string
}

// AttachSourceInput 追加来源入参。
type AttachSourceInput struct {
	IntelID       string
	ClusterKey    string
	ContributedAt time.Time
	Similarity    float64
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
