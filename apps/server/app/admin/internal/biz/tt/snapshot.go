package tt

import (
	"fmt"
	"time"

	"framework/pkg/mediacontract"
)

// ============================================================================
// 快照时间戳规则（评审 A-14，NFR-TT-07 幂等 / B-8 时间戳来源 / S-25 跨平台对齐）。
//
// 三件事必须同时成立：
//  1. t 来源可复现：_as_of 优先，collected_at 兜底，禁止用 now() 兜底
//     （否则 JetStream 至少一次投递产生的新 t 会追加而非覆盖，幂等失效）。
//  2. 时钟漂移校验：[now-late_hours, now+future_skew_minutes] 之外分别
//     打 late 或丢弃，防止上游坏点污染曲线。
//  3. 时间桶对齐：t_bucket = floor(t / bucket) × bucket，写入时间戳用 t_bucket，
//     使 (topic_id, platform, t_bucket) 唯一，重投覆盖而非追加（NFR-TT-07）；
//     同时让微博 10:05 与 B 站 10:22 同落 10:00 桶，跨平台曲线可对齐（S-25）。
// ============================================================================

// TimeKind _as_of 校验结论。
type TimeKind int

const (
	TimeOK     TimeKind = iota // 正常写入
	TimeLate                   // 迟到：写入并打 late=true，触发回填标记
	TimeFuture                 // 未来时间戳：丢弃 + 计数
	TimeMissing                // 无 _as_of 且无 collected_at：丢弃 + 计数
)

func (k TimeKind) String() string {
	switch k {
	case TimeOK:
		return "ok"
	case TimeLate:
		return "late"
	case TimeFuture:
		return "future"
	case TimeMissing:
		return "missing"
	}
	return "unknown"
}

// ResolveSampleTime 按 A-14 优先级解析样本时间。
// 返回 (t, kind)：t 恒为 UTC（mediacontract 侧 time.Time 本带时区信息，统一 .UTC()）。
func ResolveSampleTime(it *mediacontract.IntelItem, now time.Time, cfg SamplingConf) (time.Time, TimeKind) {
	// 1) metrics._as_of 首选；2) source.collected_at 兜底。
	var t time.Time
	switch {
	case it.Metrics.AsOf != nil && !it.Metrics.AsOf.IsZero():
		t = it.Metrics.AsOf.UTC()
	case !it.Source.CollectedAt.IsZero():
		t = it.Source.CollectedAt.UTC()
	default:
		return time.Time{}, TimeMissing
	}

	now = now.UTC()
	switch {
	case t.After(now.Add(time.Duration(cfg.FutureSkewMinutes) * time.Minute)):
		return t, TimeFuture
	case t.Before(now.Add(-time.Duration(cfg.LateHours) * time.Hour)):
		return t, TimeLate
	default:
		return t, TimeOK
	}
}

// AlignToBucket 时间桶对齐（UTC 日历桶，bucketMinutes ∈ [2,60]，秒/纳秒清零）。
// bucket=30 时，10:05 与 10:22 同落 10:00:00Z 桶。
func AlignToBucket(t time.Time, bucketMinutes int) time.Time {
	if bucketMinutes <= 0 {
		bucketMinutes = 30
	}
	t = t.UTC()
	d := time.Duration(bucketMinutes) * time.Minute
	return t.Truncate(d)
}

// BuildSnapshotPoint 由 IntelItem 构造快照点（纯函数，不写存储）。
//
// 哨兵映射（C-7 内部口径）：rank/rank_total ≤0 → -1；无曝光（views ≤ 0）→ eng_rate = -1。
// heat/velocity/accel/四因子 在 M1 统一写 SentinelHeat/SentinelFactor（未计算），
// M2 normalize/dynamics 反算回填后覆盖（同 (series,ts) 幂等写）。
func BuildSnapshotPoint(topicID string, it *mediacontract.IntelItem, t time.Time, kind TimeKind, cfg SamplingConf) SnapshotPoint {
	rank := it.Metrics.Rank
	if rank <= 0 {
		rank = SentinelRank
	}
	rankTotal := it.Metrics.RankTotal
	if rankTotal <= 0 {
		rankTotal = SentinelRankTotal
	}
	engRate := SentinelEngRate
	if it.Metrics.Views > 0 {
		engRate = float64(it.Metrics.Likes+it.Metrics.Comments) / float64(it.Metrics.Views)
	}

	return SnapshotPoint{
		TopicID:    topicID,
		Platform:   it.Source.Platform,
		TBucket:    AlignToBucket(t, cfg.BucketMinutes),
		TRaw:       t.UTC(),
		Rank:       rank,
		RankTotal:  rankTotal,
		Views:      it.Metrics.Views,
		Likes:      it.Metrics.Likes,
		Comments:   it.Metrics.Comments,
		EngRate:    engRate,
		HeatScore:  SentinelHeat,
		Velocity:   0,
		Accel:      0,
		RankPct:    SentinelFactor,
		EngPct:     SentinelFactor,
		Freshness:  SentinelFactor,
		Breadth:    SentinelFactor,
		Late:       kind == TimeLate,
		SampleCount: 1,
	}
}

// LastWinsKey 同桶 last-wins 守卫的 Redis key（A-14「取 _as_of 最大者」判据存储）。
func LastWinsKey(topicID, platform string, bucket time.Time) string {
	return fmt.Sprintf("tt:ingest:lastasof:%s:%s:%d", topicID, platform, bucket.UTC().Unix())
}

// RecomputeMarkKey 迟到回填重算标记的 Redis key（M1 占位：M2/M3 重算 worker 扫描消费）。
func RecomputeMarkKey(topicID string) string {
	return "tt:recompute:" + topicID
}
