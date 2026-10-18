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
// eng 直方图单测（miniredis 真 Redis 语义）：分桶/窗口合并/并列中点/TTL。
// ============================================================================

func histTestT() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }

func TestEngBucketMapping(t *testing.T) {
	if engBucket(0) != 0 {
		t.Fatalf("v=0 → bucket 0, got %d", engBucket(0))
	}
	// log1p(9)=2.3026 → 23
	if engBucket(9) != 23 {
		t.Fatalf("v=9 → 23, got %d", engBucket(9))
	}
	// 超大值钳到 599
	if engBucket(1e30) != 599 {
		t.Fatalf("clamp: %d", engBucket(1e30))
	}
	if engBucket(-5) != 0 {
		t.Fatalf("negative → 0: %d", engBucket(-5))
	}
}

func TestEngHistogramObserveAndPercentile(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	h := NewRedisEngHistogram(rdb, log.DefaultLogger)
	now := histTestT()

	// 单点：自包含 ECDF，(0 + 0.5×1)/1 = 0.5
	pct, n, err := h.ObserveAndPercentile(ctx, "weibo", "A_hot", now, 100)
	if err != nil || n != 1 {
		t.Fatalf("first: pct=%v n=%d err=%v", pct, n, err)
	}
	if pct != 0.5 {
		t.Fatalf("single point mid-rank must be 0.5: %v", pct)
	}

	// 再加 9 个更大值：原样本分位 = (0 + 0.5×1)/10 = 0.05；新值分位 = (1 + 0.5×9)/10 = 0.55
	for i := 0; i < 9; i++ {
		if _, _, err = h.ObserveAndPercentile(ctx, "weibo", "A_hot", now, 1e4); err != nil {
			t.Fatal(err)
		}
	}
	// 直接查窗口判定：再观测一个与首样本同桶的值 → c_b=2 (v=100 桶), below=0
	// 新样本分位 = (0 + 0.5×2)/11 ≈ 0.0909
	pct, n, err = h.ObserveAndPercentile(ctx, "weibo", "A_hot", now, 100)
	if err != nil || n != 11 {
		t.Fatalf("window: n=%d err=%v", n, err)
	}
	want := (0 + 0.5*2) / 11.0
	if pct < want-1e-9 || pct > want+1e-9 {
		t.Fatalf("tie mid-point: got %v want %v", pct, want)
	}
}

func TestEngHistogramWindowAcrossDays(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	h := NewRedisEngHistogram(rdb, log.DefaultLogger)
	now := histTestT()

	// 8 天前的点不应进窗口；(t-7d, t] 覆盖 7 个日键（含当日）
	old, _, err := h.ObserveAndPercentile(ctx, "bilibili", "B_material", now.AddDate(0, 0, -7), 5)
	if err != nil {
		t.Fatal(err)
	}
	_ = old // 写入发生在第 -7 天键上

	pct, n, err := h.ObserveAndPercentile(ctx, "bilibili", "B_material", now, 5)
	if err != nil {
		t.Fatal(err)
	}
	// 窗口只含刚刚的 1 个点（第 -7 天键在 (t-7d, t] 之外）
	if n != 1 {
		t.Fatalf("7-day window must exclude day -7 key: n=%d", n)
	}
	if pct != 0.5 {
		t.Fatalf("pct: %v", pct)
	}

	// TTL 已设置（8d）
	ttl := mr.TTL(engHistKey("bilibili", "B_material", now))
	if ttl <= 7*24*time.Hour || ttl > 8*24*time.Hour {
		t.Fatalf("ttl should be ~8d: %v", ttl)
	}
}

func TestEngHistogramDisabledWithoutRedis(t *testing.T) {
	h := NewRedisEngHistogram(nil, log.DefaultLogger)
	if _, _, err := h.ObserveAndPercentile(context.Background(), "weibo", "A_hot", histTestT(), 10); err == nil {
		t.Fatal("disabled store must error (biz falls back to baseline)")
	}
}

func TestDynamicsStoreRoundTripAndRace(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	s := NewRedisDynamicsStore(rdb, log.DefaultLogger)

	// 空读 → nil（首点语义）
	st, err := s.Load(ctx, "tp_1", "weibo")
	if err != nil || st != nil {
		t.Fatalf("empty load: %+v err=%v", st, err)
	}

	t10 := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	next := biztt.DynamicsState{HEwma: 42, LastT: t10, WeightEpoch: "ep1"}
	// 首写（expected 零值 = 断言无 key）
	ok, err := s.SaveIfNotAdvanced(ctx, "tp_1", "weibo", next, time.Time{})
	if err != nil || !ok {
		t.Fatalf("first save: ok=%v err=%v", ok, err)
	}

	st, err = s.Load(ctx, "tp_1", "weibo")
	if err != nil || st == nil {
		t.Fatalf("load: %+v err=%v", st, err)
	}
	if st.HEwma != 42 || !st.LastT.Equal(t10) || st.WeightEpoch != "ep1" {
		t.Fatalf("state drift: %+v", st)
	}

	// 并发抢先：stored last_t=10:00，以 expected=10:00 写入推进到 10:30（stored ≤
	// expected 是「未被越过」允许写）；stored 已推进到 10:30 后，旧 expected=10:00 的写必拒。
	t1030 := t10.Add(30 * time.Minute)
	ok, _ = s.SaveIfNotAdvanced(ctx, "tp_1", "weibo", biztt.DynamicsState{HEwma: 50, VEwma: 1, LastT: t1030, WeightEpoch: "ep1"}, t10)
	if !ok {
		t.Fatal("equal expected must pass (stored not advanced past expected)")
	}
	ok, _ = s.SaveIfNotAdvanced(ctx, "tp_1", "weibo", biztt.DynamicsState{HEwma: 60, VEwma: 2, LastT: t1030, WeightEpoch: "ep1"}, t10)
	if ok {
		t.Fatal("stale expected (10:00 < stored 10:30) must be rejected")
	}

	// 禁用态：Load nil / Save false
	s2 := NewRedisDynamicsStore(nil, log.DefaultLogger)
	st, _ = s2.Load(ctx, "tp_1", "weibo")
	if st != nil {
		t.Fatal("disabled load must be nil")
	}
	ok, _ = s2.SaveIfNotAdvanced(ctx, "tp_1", "weibo", next, time.Time{})
	if ok {
		t.Fatal("disabled save must be false")
	}
}
