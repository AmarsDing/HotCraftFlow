package tt

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TT 观测性指标（02-TT 03-详细设计.md §6 + A-14/A-16 增补计数）。
// 命名统一 tt_ 前缀；CounterVec 标签控制在低基数集合内。

var (
	// SnapshotWrittenTotal 快照写入计数（status: ok / buffered / dropped）。
	SnapshotWrittenTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_snapshot_written_total",
		Help: "快照点写入计数（status: ok=直入 InfluxDB, buffered=失败入缓冲, dropped=缓冲超限丢弃）",
	}, []string{"platform", "status"})

	// IngestEventTotal 消费事件计数（result: dedup/gate/ok）。
	IngestEventTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_ingest_event_total",
		Help: "IntelItem 消费事件计数（result: ok=受理, dedup=幂等去重, gate=门禁跳过）",
	}, []string{"result"})

	// IngestBadTimestampTotal 坏时间戳计数（reason: missing/future/late）。
	IngestBadTimestampTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_ingest_bad_timestamp_total",
		Help: "时间戳校验失败计数（A-14：missing=无可用时间, future=未来时间, late=迟到写入）",
	}, []string{"reason"})

	// IngestStaleBucketTotal 同桶旧点丢弃计数（A-14 last-wins 守卫）。
	IngestStaleBucketTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tt_ingest_stale_bucket_total",
		Help: "同桶内 _as_of 未超已有值的样本被丢弃计数（last-wins 守卫）",
	})

	// InfluxWriteDuration 同步写 InfluxDB 耗时。
	InfluxWriteDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tt_influx_write_duration_seconds",
		Help:    "InfluxDB 批写耗时",
		Buckets: prometheus.DefBuckets,
	})

	// InfluxBufferDepth 失败缓冲流深度（A-16 背压与告警：>100k warn，>500k critical）。
	InfluxBufferDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tt_influx_buffer_depth",
		Help: "InfluxDB 失败缓冲流深度（Redis Stream XLEN 近似）",
	})

	// InfluxDroppedTotal 缓冲超限裁剪丢弃点数（A-16：违反 NFR-TT-01，需人工介入）。
	InfluxDroppedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tt_influx_dropped_total",
		Help: "失败缓冲到达 MAXLEN 后被 XTRIM 裁剪丢弃的点数",
	})

	// BufferReplayTotal 重放批处理计数（result: ok / retry / poison）。
	BufferReplayTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_buffer_replay_total",
		Help: "缓冲重放批计数",
	}, []string{"result"})

	// TopicCreatedTotal 话题新建计数（source: cluster_key/no_cluster_key）。
	TopicCreatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_topic_created_total",
		Help: "话题新建计数",
	}, []string{"source"})

	// HeatNoFactorTotal 四因子全缺失计数（A-6：heat=null 不写 InfluxDB）。
	HeatNoFactorTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tt_heat_no_factor_total",
		Help: "热度四因子全缺失计数（A-6：TT_HEAT_NO_FACTOR，该点不写 InfluxDB）",
	})

	// EngBaselineFallbackTotal eng 退化静态基线计数（reason: cold_start/store_error/no_store）。
	EngBaselineFallbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_eng_baseline_fallback_total",
		Help: "eng_pct 退化静态基线计数（A-3：cold_start=N<200, store_error=Redis 故障；均不标 missing）",
	}, []string{"reason"})

	// DynamicsResetTotal 动态链 α=1 重置计数（reason: first/epoch_change）。
	DynamicsResetTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tt_dynamics_reset_total",
		Help: "动态链重置计数（S-7/A-15：first=首点, epoch_change=权重口径断点）",
	}, []string{"reason"})

	// WeightChangedTotal heat 权重变更计数（A-15）。
	WeightChangedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tt_weight_changed_total",
		Help: "heat 权重变更次数（每次插入一行 WeightConfig，生效后首点强制 α=1）",
	})
)
