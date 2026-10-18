package ttdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"

	biztt "framework/app/admin/internal/biz/tt"
)

// RedisSnapshotBuffer 写失败缓冲（biz/tt.SnapshotBuffer 实现）。
//
// A-16 语义替代旧「TTL 1h 队列」：
//   - Redis Stream + 消费组；XADD MAXLEN ≈ 上限，超限 XTRIM 最旧并如实计数
//     tt_influx_dropped_total（需求 §9 诚实口径：尽力不丢 + 显式告知丢失量）。
//   - 重放经 XAUTOCLAIM 回收超时挂起消息；attempt 超限进 poison 流（人工介入）。
//   - 深度口径为条目数（1 条 = 1 点），与 pause/resume 阈值同单位。
type RedisSnapshotBuffer struct {
	conf     biztt.BufferConf
	rdb      *redis.Client
	consumer string // 组内消费者名（重放 worker）
	log      *log.Helper
	enabled  bool
}

// NewRedisSnapshotBuffer 构造。rdb 为 nil 时降级：所有操作 no-op 并告警
// （此时 InfluxDB 失败的批将重新入批队列，见 sink.Flush 的 buffer_error 分支）。
func NewRedisSnapshotBuffer(conf *biztt.Conf, rdb *redis.Client, logger log.Logger) (*RedisSnapshotBuffer, error) {
	b := &RedisSnapshotBuffer{
		conf:     conf.Buffer,
		rdb:      rdb,
		consumer: "replay-1",
		log:      log.NewHelper(log.With(logger, "module", "data/tt-buffer")),
	}
	if rdb == nil {
		b.log.Warn("redis not configured, snapshot buffer degraded (no persistence for failed writes)")
		return b, nil
	}
	b.enabled = true

	// 消费组不存在时按 MKSTREAM 创建（幂等）。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := rdb.XGroupCreateMkStream(ctx, b.conf.StreamKey, b.conf.ConsumerGroup, "0").Err()
	if err != nil && !isBusyGroupErr(err) {
		return nil, fmt.Errorf("create consumer group on %s: %w", b.conf.StreamKey, err)
	}
	return b, nil
}

// Enqueue 入队若干点。返回因 MAXLEN 裁剪而永久丢弃的点数（近似：before+n-len(瓶颈)）。
func (b *RedisSnapshotBuffer) Enqueue(ctx context.Context, points []biztt.SnapshotPoint) (dropped int, err error) {
	if len(points) == 0 {
		return 0, nil
	}
	if !b.enabled {
		return len(points), errors.New("snapshot buffer not available (redis disabled)")
	}

	before, _ := b.rdb.XLen(ctx, b.conf.StreamKey).Result()

	pipe := b.rdb.Pipeline()
	nowMs := time.Now().UTC().UnixMilli()
	for i := range points {
		payload, merr := json.Marshal(points[i])
		if merr != nil {
			pipe.Discard()
			return 0, fmt.Errorf("marshal snapshot point: %w", merr)
		}
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: b.conf.StreamKey,
			MaxLen: b.conf.MaxLen,
			Approx: true,
			Values: map[string]any{"p": payload, "e": nowMs},
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		pipe.Discard()
		return 0, fmt.Errorf("xadd batch: %w", err)
	}

	// 裁剪丢弃量估算（容忍并发入队的误差；指标用途，非审计）。
	after, _ := b.rdb.XLen(ctx, b.conf.StreamKey).Result()
	expected := before + int64(len(points))
	if after < expected {
		dropped = int(expected - after)
		biztt.InfluxBufferDepth.Set(float64(after))
		return dropped, nil
	}
	biztt.InfluxBufferDepth.Set(float64(after))
	return 0, nil
}

// Depth 缓冲深度（条目数）。
func (b *RedisSnapshotBuffer) Depth(ctx context.Context) (int64, error) {
	if !b.enabled {
		return 0, nil
	}
	return b.rdb.XLen(ctx, b.conf.StreamKey).Result()
}

// ReadBatch 为重放读取一批。先 XAUTOCLAIM 回收超时挂起，无则非阻塞取新消息。
// batchID 为逗号拼接的 entry IDs（Ack/Nack 用）。
func (b *RedisSnapshotBuffer) ReadBatch(ctx context.Context, max int64) ([]biztt.SnapshotPoint, string, error) {
	if !b.enabled {
		return nil, "", errors.New("snapshot buffer not available")
	}

	msgs, _, err := b.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   b.conf.StreamKey,
		Group:    b.conf.ConsumerGroup,
		Consumer: b.consumer,
		MinIdle:  b.conf.ReplayInterval,
		Start:    "0",
		Count:    max,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, "", fmt.Errorf("xautoclaim: %w", err)
	}
	if len(msgs) == 0 {
		streams, rerr := b.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    b.conf.ConsumerGroup,
			Consumer: b.consumer,
			Streams:  []string{b.conf.StreamKey, ">"},
			Count:    max,
			// Block=-1：显式非阻塞。Block=0 在 go-redis 里是 BLOCK 0（无限等待），
			// 会把空流上的重放循环永久挂死；节拍由 worker 的 ReplayInterval ticker 承担。
			Block: -1,
		}).Result()
		if rerr == nil && len(streams) > 0 {
			msgs = streams[0].Messages
		}
	}

	if len(msgs) == 0 {
		return nil, "", nil
	}

	points := make([]biztt.SnapshotPoint, 0, len(msgs))
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		raw, _ := m.Values["p"].(string)
		var p biztt.SnapshotPoint
		if jerr := json.Unmarshal([]byte(raw), &p); jerr != nil {
			// 坏条目：直接 poison 化处理由 Nack 承担，这里跳过但保留 ID 以便清理。
			b.log.Warnf("bad buffer entry %s: %v", m.ID, jerr)
			ids = append(ids, m.ID)
			continue
		}
		points = append(points, p)
		ids = append(ids, m.ID)
	}
	return points, strings.Join(ids, ","), nil
}

// Ack 确认批已落库（XACK + XDEL，保持流紧凑）。
func (b *RedisSnapshotBuffer) Ack(ctx context.Context, batchID string) error {
	if !b.enabled || batchID == "" {
		return nil
	}
	ids := strings.Split(batchID, ",")
	if err := b.rdb.XAck(ctx, b.conf.StreamKey, b.conf.ConsumerGroup, ids...).Err(); err != nil {
		return fmt.Errorf("xack: %w", err)
	}
	_ = b.rdb.XDel(ctx, b.conf.StreamKey, ids...).Err()
	if d, err := b.rdb.XLen(ctx, b.conf.StreamKey).Result(); err == nil {
		biztt.InfluxBufferDepth.Set(float64(d))
	}
	return nil
}

// Nack 批处理失败：逐条查 delivery 次数，超限的进 poison 并确认；否则留 pending 待回收。
func (b *RedisSnapshotBuffer) Nack(ctx context.Context, batchID string, points []biztt.SnapshotPoint, maxAttempts int) (bool, error) {
	if !b.enabled || batchID == "" {
		return false, nil
	}
	ids := strings.Split(batchID, ",")
	poisoned := false

	for idx, id := range ids {
		pend, err := b.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: b.conf.StreamKey,
			Group:  b.conf.ConsumerGroup,
			Start:  id,
			End:    id,
			Count:  1,
		}).Result()
		if err != nil || len(pend) == 0 {
			continue
		}
		// A-16 口径：RetryCount（已投递次数，含本次在途）≥ MaxAttempts 才进 poison，
		// 即允许满 maxAttempts 次尝试。
		if int(pend[0].RetryCount) < maxAttempts {
			continue // 留 pending，下轮 XAUTOCLAIM 回收重试
		}

		// 超限 → poison（保留 30d 的人工介入队列）。
		var p biztt.SnapshotPoint
		if idx < len(points) {
			p = points[idx]
		}
		payload, _ := json.Marshal(map[string]any{
			"point":       p,
			"attempts":    pend[0].RetryCount,
			"entry_id":    id,
			"poisoned_at": time.Now().UTC().Format(time.RFC3339),
		})
		_ = b.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: b.conf.PoisonKey,
			MaxLen: 100_000,
			Approx: true,
			Values: map[string]any{"p": payload},
		}).Err()
		_ = b.rdb.XAck(ctx, b.conf.StreamKey, b.conf.ConsumerGroup, id).Err()
		_ = b.rdb.XDel(ctx, b.conf.StreamKey, id).Err()
		poisoned = true
	}
	return poisoned, nil
}

// isBusyGroupErr BUSYGROUP（组已存在）判定。
func isBusyGroupErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

// AttemptsOf 解析 Nack 日志辅助（保留给调试）。
func AttemptsOf(retry int64) string { return strconv.FormatInt(retry, 10) }
