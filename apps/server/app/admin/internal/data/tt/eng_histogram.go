package ttdata

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"

	biztt "framework/app/admin/internal/biz/tt"
)

// ============================================================================
// eng_pct 分桶直方图（评审 A-3 终案，替代 03 §2.1 的「ZSET 滚动 7 日」：
// ZRANK 无法跨 7 个日键求秩，ZSET 方案已被评审推翻）。
//
//   key    = tt:eng:{platform}:{category}:{yyyymmdd}（UTC 日），TTL = 8d
//   bucket = clamp(floor(log1p(v) × 10), 0, 599)   # 0.1 对数粒度
//   写入   = HINCRBY key bucket 1
//   查询   = 合并 (t-7d, t] 覆盖的 7 个日键后 ECDF：
//            eng_pct = (Σ_{b'<b} c_b' + 0.5 × c_b) / max(N, 1)   # 并列项取中点
//
// 冷启动 N<200 与 Redis 故障的静态基线退化在 biz 层（BaselineEngPct）——
// 本实现只负责「写入 + 分位查询 + 窗口总数」，错误即返回由调用方兜底。
// ============================================================================

// engHistBuckets 桶数（log1p(v) ∈ [0, 59.9]，覆盖 v ≈ 0..1e26 互动量）。
const engHistBuckets = 600

// engHistTTL 日键保留时长（窗口 7d + 1d 余量）。
const engHistTTL = 8 * 24 * time.Hour

// engHistWindowDays ECDF 窗口天数（含当日）。
const engHistWindowDays = 7

// RedisEngHistogram biz/tt.EngHistogram 实现。rdb=nil → 禁用态（错误返回，biz 走基线）。
type RedisEngHistogram struct {
	rdb *redis.Client
	log *log.Helper
}

// NewRedisEngHistogram 构造。
func NewRedisEngHistogram(rdb *redis.Client, logger log.Logger) *RedisEngHistogram {
	h := &RedisEngHistogram{rdb: rdb, log: log.NewHelper(log.With(logger, "module", "data/tt-eng-hist"))}
	if rdb == nil {
		h.log.Warn("redis not configured, eng histogram disabled (baseline fallback at biz layer)")
	}
	return h
}

// engHistKey 日键（UTC 日历日；存储纪律 NFR-TT-03）。
func engHistKey(platform, category string, t time.Time) string {
	return fmt.Sprintf("tt:eng:%s:%s:%s", platform, category, t.UTC().Format("20060102"))
}

// engBucket 互动量 → 桶号（0.1 × log1p 粒度）。
func engBucket(v float64) int {
	if v < 0 {
		v = 0
	}
	b := int(math.Floor(math.Log1p(v) * 10))
	if b < 0 {
		return 0
	}
	if b >= engHistBuckets {
		return engHistBuckets - 1
	}
	return b
}

// ObserveAndPercentile 先计入样本再求分位（自包含：ECDF 含本样本，并列取中点）。
// 返回窗口样本总数 n（biz 做 N<200 冷启动判定）。
func (h *RedisEngHistogram) ObserveAndPercentile(
	ctx context.Context, platform, category string, t time.Time, engagement float64,
) (pct float64, n int64, err error) {
	if h.rdb == nil {
		return 0, 0, errRedisDisabled
	}
	t = t.UTC()
	b := engBucket(engagement)

	// 1) 写入当日桶（+TTL 刷新由写入顺带完成）。
	dayKey := engHistKey(platform, category, t)
	pipe := h.rdb.Pipeline()
	pipe.HIncrBy(ctx, dayKey, strconv.Itoa(b), 1)
	pipe.Expire(ctx, dayKey, engHistTTL)
	if _, err = pipe.Exec(ctx); err != nil {
		return 0, 0, fmt.Errorf("observe bucket: %w", err)
	}

	// 2) 合并窗口内 7 个日键（HGETALL 全 600 桶内仅存非零桶，代价可控）。
	counts := make([]int64, engHistBuckets)
	for i := 0; i < engHistWindowDays; i++ {
		key := engHistKey(platform, category, t.AddDate(0, 0, -i))
		fields, err := h.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return 0, 0, fmt.Errorf("read window key %s: %w", key, err)
		}
		for f, cs := range fields {
			bi, cerr := strconv.Atoi(f)
			if cerr != nil || bi < 0 || bi >= engHistBuckets {
				continue
			}
			c, cerr := strconv.ParseInt(cs, 10, 64)
			if cerr != nil {
				continue
			}
			counts[bi] += c
		}
	}

	// 3) ECDF：eng_pct = (Σ_{b'<b} + 0.5 × c_b) / max(N, 1)。
	var below, total int64
	for bi, c := range counts {
		total += c
		if bi < b {
			below += c
		}
	}
	n = total
	if n == 0 {
		return 0, 0, nil // biz 按 n<阈值 走基线
	}
	pct = (float64(below) + 0.5*float64(counts[b])) / float64(n)
	if pct > 1 {
		pct = 1
	}
	return pct, n, nil
}

// ReadPercentile 只读分位（不 HINCRBY；S-23 迟到回填重算用——迟到点已由
// ingest 观测过当日键，重算再观测会双倍计数）。窗口数据为空或故障返回 -1。
// category 为空串时无法定位日键，直接返回 -1（调用方保留原值）。
func (h *RedisEngHistogram) ReadPercentile(
	ctx context.Context, platform, category string, t time.Time, engagement float64,
) (float64, error) {
	if h.rdb == nil {
		return -1, errRedisDisabled
	}
	if category == "" {
		return -1, nil
	}
	t = t.UTC()
	b := engBucket(engagement)

	// 合并窗口内 7 个日键（与 ObserveAndPercentile 同一查询路径，仅差写入步）。
	counts := make([]int64, engHistBuckets)
	for i := 0; i < engHistWindowDays; i++ {
		key := engHistKey(platform, category, t.AddDate(0, 0, -i))
		fields, err := h.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return -1, fmt.Errorf("read window key %s: %w", key, err)
		}
		for f, cs := range fields {
			bi, cerr := strconv.Atoi(f)
			if cerr != nil || bi < 0 || bi >= engHistBuckets {
				continue
			}
			c, cerr := strconv.ParseInt(cs, 10, 64)
			if cerr != nil {
				continue
			}
			counts[bi] += c
		}
	}

	var below, total int64
	for bi, c := range counts {
		total += c
		if bi < b {
			below += c
		}
	}
	if total == 0 {
		return -1, nil // 无窗口数据：调用方保留原值（口径不随空窗跳变）
	}
	pct := (float64(below) + 0.5*float64(counts[b])) / float64(total)
	if pct > 1 {
		pct = 1
	}
	return pct, nil
}

// errRedisDisabled Redis 未配置（与 buffer/guard 的禁用语义一致，biz 层兜底）。
var errRedisDisabled = fmt.Errorf("redis disabled")

// 编译期接口校验。
var _ biztt.EngHistogram = (*RedisEngHistogram)(nil)
