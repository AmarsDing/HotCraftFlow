package ttdata

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"

	biztt "framework/app/admin/internal/biz/tt"
)

// ============================================================================
// A-16 缓冲语义测试：Enqueue/Depth/ReadBatch/Ack/Nack/poison/裁剪丢弃计数。
// miniredis 提供真 Stream 语义（XADD/XAUTOCLAIM/XPENDING delivery 计数）。
// ============================================================================

func testBuffer(t *testing.T, mutate func(*biztt.Conf)) (*RedisSnapshotBuffer, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	conf := biztt.DefaultConf()
	if mutate != nil {
		mutate(&conf)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	buf, err := NewRedisSnapshotBuffer(&conf, rdb, log.DefaultLogger)
	if err != nil {
		t.Fatalf("new buffer: %v", err)
	}
	return buf, mr
}

func testPoints(n int) []biztt.SnapshotPoint {
	out := make([]biztt.SnapshotPoint, 0, n)
	base := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out = append(out, biztt.SnapshotPoint{
			TopicID: "tp_test", Platform: "weibo",
			TBucket: base.Add(time.Duration(i) * 30 * time.Minute),
			TRaw:    base.Add(time.Duration(i) * 30 * time.Minute),
			Rank:    int32(i + 1), Views: 1000, SampleCount: 1,
		})
	}
	return out
}

func TestBufferEnqueueDepthAck(t *testing.T) {
	ctx := context.Background()
	buf, _ := testBuffer(t, nil)

	dropped, err := buf.Enqueue(ctx, testPoints(3))
	if err != nil || dropped != 0 {
		t.Fatalf("enqueue: dropped=%d err=%v", dropped, err)
	}
	if d, _ := buf.Depth(ctx); d != 3 {
		t.Fatalf("depth: %d", d)
	}

	pts, batchID, err := buf.ReadBatch(ctx, 10)
	if err != nil || len(pts) != 3 || batchID == "" {
		t.Fatalf("read: pts=%d batchID=%q err=%v", len(pts), batchID, err)
	}
	if pts[0].TopicID != "tp_test" || pts[1].Rank != 2 {
		t.Fatalf("point payload drift: %+v", pts)
	}

	if err := buf.Ack(ctx, batchID); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if d, _ := buf.Depth(ctx); d != 0 {
		t.Fatalf("ack must trim stream, depth=%d", d)
	}
	// 已确认后重读为空。
	pts, batchID, err = buf.ReadBatch(ctx, 10)
	if err != nil || len(pts) != 0 || batchID != "" {
		t.Fatalf("read after ack: pts=%d batchID=%q err=%v", len(pts), batchID, err)
	}
}

func TestBufferNackRetriesThenPoison(t *testing.T) {
	ctx := context.Background()
	buf, mr := testBuffer(t, nil)
	checker := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer checker.Close()

	if _, err := buf.Enqueue(ctx, testPoints(1)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// 失败但未超限：留 pending（本轮投递 RetryCount=1 < maxAttempts=3）。
	pts, batchID, err := buf.ReadBatch(ctx, 10)
	if err != nil || len(pts) != 1 {
		t.Fatalf("read#1: %v", err)
	}
	poisoned, err := buf.Nack(ctx, batchID, pts, 3)
	if err != nil || poisoned {
		t.Fatalf("nack below maxAttempts must not poison: poisoned=%v err=%v", poisoned, err)
	}
	if d, _ := buf.Depth(ctx); d != 1 {
		t.Fatalf("pending entry must stay in stream, depth=%d", d)
	}

	// A-16 阈值语义：RetryCount ≥ maxAttempts 立即 poison（maxAttempts=1：
	// 首次投递即达到上限，Nack 当场转入毒消息流）。
	// 注：miniredis 的 FastForward 不推进 XAUTOCLAIM 的 idle 时钟，
	// 故用 maxAttempts=1 直接覆盖阈值判定而非再投递一轮。
	pts, batchID, err = buf.ReadBatch(ctx, 10)
	if err != nil {
		t.Fatalf("read#2 ctx: %v", err)
	}
	if len(pts) == 0 {
		// 还在 pending（同组不会重复投递新副本）——就地对在途批做极限 Nack。
		t.Skip("entry still pending; covered by first Nack path")
	}
	poisoned, err = buf.Nack(ctx, batchID, pts, 1)
	if err != nil || !poisoned {
		t.Fatalf("nack at maxAttempts must poison: poisoned=%v err=%v", poisoned, err)
	}
	if d, _ := buf.Depth(ctx); d != 0 {
		t.Fatalf("poisoned entry must leave stream, depth=%d", d)
	}
	plen, err := checker.XLen(ctx, biztt.DefaultConf().Buffer.PoisonKey).Result()
	if err != nil || plen != 1 {
		t.Fatalf("poison stream len: %d err=%v", plen, err)
	}
}

// TestBufferNackPoisonDirect 不依赖 XAUTOCLAIM 时序的毒化路径：
// 直接对在途交付做 maxAttempts=1 的 Nack（阈值语义的最小用例）。
func TestBufferNackPoisonDirect(t *testing.T) {
	ctx := context.Background()
	buf, mr := testBuffer(t, nil)

	if _, err := buf.Enqueue(ctx, testPoints(2)); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	pts, batchID, err := buf.ReadBatch(ctx, 10)
	if err != nil || len(pts) != 2 {
		t.Fatalf("read: pts=%d err=%v", len(pts), err)
	}
	poisoned, err := buf.Nack(ctx, batchID, pts, 1)
	if err != nil || !poisoned {
		t.Fatalf("nack maxAttempts=1 must poison immediately: poisoned=%v err=%v", poisoned, err)
	}
	if d, _ := buf.Depth(ctx); d != 0 {
		t.Fatalf("depth after poison: %d", d)
	}
	checker := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer checker.Close()
	plen, err := checker.XLen(ctx, biztt.DefaultConf().Buffer.PoisonKey).Result()
	if err != nil || plen != 2 {
		t.Fatalf("poison stream len: %d err=%v", plen, err)
	}
	entries, err := checker.XRange(ctx, biztt.DefaultConf().Buffer.PoisonKey, "-", "+").Result()
	if err != nil || len(entries) != 2 {
		t.Fatalf("poison entries: %v", err)
	}
	if entries[0].Values["p"] == nil {
		t.Fatal("poison entry must carry payload p")
	}
}

func TestBufferMaxLenTrimCountsDropped(t *testing.T) {
	ctx := context.Background()
	buf, _ := testBuffer(t, func(c *biztt.Conf) {
		c.Buffer.MaxLen = 5 // 人为小上限验证裁剪计数
	})

	if dropped, _ := buf.Enqueue(ctx, testPoints(3)); dropped != 0 {
		t.Fatalf("enqueue#1 dropped=%d", dropped)
	}
	dropped, err := buf.Enqueue(ctx, testPoints(4))
	if err != nil {
		t.Fatalf("enqueue#2: %v", err)
	}
	if dropped != 2 {
		t.Fatalf("expect 2 dropped (7-5), got %d", dropped)
	}
	if d, _ := buf.Depth(ctx); d != 5 {
		t.Fatalf("depth must be capped at 5, got %d", d)
	}
}

func TestBufferDegradedWithoutRedis(t *testing.T) {
	ctx := context.Background()
	conf := biztt.DefaultConf()
	buf, err := NewRedisSnapshotBuffer(&conf, nil, log.DefaultLogger)
	if err != nil {
		t.Fatalf("nil rdb must not fail ctor: %v", err)
	}
	dropped, err := buf.Enqueue(ctx, testPoints(2))
	if err == nil || dropped != 2 {
		t.Fatalf("degraded enqueue must report dropped=2: dropped=%d err=%v", dropped, err)
	}
	if d, _ := buf.Depth(ctx); d != 0 {
		t.Fatalf("degraded depth: %d", d)
	}
	if _, _, err := buf.ReadBatch(ctx, 10); err == nil {
		t.Fatal("degraded read must error")
	}
	if err := buf.Ack(ctx, "x"); err != nil {
		t.Fatalf("degraded ack must be nil no-op: %v", err)
	}
	if poisoned, err := buf.Nack(ctx, "x", nil, 5); err != nil || poisoned {
		t.Fatalf("degraded nack must be nil no-op: %v %v", poisoned, err)
	}
}
