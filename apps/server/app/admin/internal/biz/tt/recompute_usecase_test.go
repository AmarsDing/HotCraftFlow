package tt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// ---- fakes ----

type fakeRecomputeRepo struct {
	lockHeld bool
	setCalls int
	cleared  bool
}

func (f *fakeRecomputeRepo) SetRecomputeLock(_ context.Context, _ string, at *time.Time, _ time.Time) (bool, error) {
	f.setCalls++
	if at == nil {
		f.cleared = true
		return true, nil
	}
	if f.lockHeld {
		return false, nil // 模拟并发持锁
	}
	f.lockHeld = true
	return true, nil
}

type fakeRecomputeTS struct {
	platforms []string
	series    map[string][]SnapshotPoint // key = platform
	written   []SnapshotPoint
}

func (f *fakeRecomputeTS) WriteSnapshots(_ context.Context, pts []SnapshotPoint) error {
	f.written = append(f.written, pts...)
	return nil
}
func (f *fakeRecomputeTS) Ping(_ context.Context) (bool, error) { return true, nil }
func (f *fakeRecomputeTS) ReadRecentPoints(_ context.Context, _, _ string, _ int) ([]SnapshotPoint, error) {
	return nil, nil
}
func (f *fakeRecomputeTS) ReadActiveHeatRange(_ context.Context, _, _ time.Time) ([]float64, error) {
	return nil, nil
}
func (f *fakeRecomputeTS) ReadPlatforms(_ context.Context, _ string, _, _ time.Time) ([]string, error) {
	return f.platforms, nil
}
func (f *fakeRecomputeTS) ReadSeriesAsc(_ context.Context, _, platform string, _, _ time.Time) ([]SnapshotPoint, error) {
	return f.series[platform], nil
}

type fakeRecomputeRedis struct {
	marks map[string]string
	keys  []string
	delN  int
}

func (f *fakeRecomputeRedis) IsConfigured() bool { return true }
func (f *fakeRecomputeRedis) TryLock(_ context.Context, _ string, _ time.Duration) (string, bool, error) {
	return "", true, nil
}
func (f *fakeRecomputeRedis) Unlock(_ context.Context, _, _ string) {}
func (f *fakeRecomputeRedis) IncrWithExpire(_ context.Context, _ string, _ time.Duration) (int64, error) {
	return 0, errors.New("not impl")
}
func (f *fakeRecomputeRedis) HIncrBy(_ context.Context, _, _ string, _ int64) error {
	return errors.New("not impl")
}
func (f *fakeRecomputeRedis) ScanKeys(_ context.Context, _ string, _ int) ([]string, error) {
	return f.keys, nil
}
func (f *fakeRecomputeRedis) HGetInt64(_ context.Context, _, _ string) (int64, error) { return 0, nil }
func (f *fakeRecomputeRedis) HGetDel(_ context.Context, _, _ string) (int64, error)   { return 0, nil }
func (f *fakeRecomputeRedis) Del(_ context.Context, keys ...string) error {
	f.delN += len(keys)
	for _, k := range keys {
		delete(f.marks, k)
	}
	return nil
}
func (f *fakeRecomputeRedis) GetString(_ context.Context, key string) (string, error) {
	return f.marks[key], nil
}
func (f *fakeRecomputeRedis) SetStringEX(_ context.Context, key, value string, _ time.Duration) error {
	f.marks[key] = value
	return nil
}

type fakeRecomputeTopicRepo struct{ meta TopicHeatMeta }

func (f *fakeRecomputeTopicRepo) FindTopicIDByClusterKey(_ context.Context, _ string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeRecomputeTopicRepo) CreateTopicWithSource(_ context.Context, _ CreateTopicInput) (string, error) {
	return "", nil
}
func (f *fakeRecomputeTopicRepo) AttachSource(_ context.Context, _ string, _ AttachSourceInput) error {
	return nil
}
func (f *fakeRecomputeTopicRepo) TouchSeen(_ context.Context, _ string, _ time.Time) error { return nil }
func (f *fakeRecomputeTopicRepo) GetTopicHeatMeta(_ context.Context, _ string, _ time.Time) (TopicHeatMeta, error) {
	return f.meta, nil
}
func (f *fakeRecomputeTopicRepo) UpdateTopicHeatState(_ context.Context, _ string, _ TopicHeatUpdate) error {
	return nil
}

type fakeRecomputeHist struct{ pct float64 }

func (f *fakeRecomputeHist) ObserveAndPercentile(_ context.Context, _, _ string, _ time.Time, _ float64) (float64, int64, error) {
	return f.pct, 500, nil
}
func (f *fakeRecomputeHist) ReadPercentile(_ context.Context, _, _ string, _ time.Time, _ float64) (float64, error) {
	return f.pct, nil
}

type fakeRecomputeDyn struct {
	stored map[string]*DynamicsState
	saved  int
}

func (f *fakeRecomputeDyn) Load(_ context.Context, topicID, platform string) (*DynamicsState, error) {
	return f.stored[topicID+":"+platform], nil
}
func (f *fakeRecomputeDyn) SaveIfNotAdvanced(_ context.Context, topicID, platform string, st DynamicsState, _ time.Time) (bool, error) {
	f.saved++
	cp := st
	f.stored[topicID+":"+platform] = &cp
	return true, nil
}

type fakeRecomputeWeights struct{}

func (f *fakeRecomputeWeights) ActiveHeatWeights(_ context.Context, _ time.Time) (WeightConfigRow, bool, error) {
	return WeightConfigRow{ConfigID: "wc_test", Weights: DefaultHeatWeights()}, true, nil
}
func (f *fakeRecomputeWeights) ListHeatWeights(_ context.Context, _ int) ([]WeightConfigRow, error) {
	return nil, nil
}
func (f *fakeRecomputeWeights) AppendHeatWeights(_ context.Context, _ WeightConfigRow) error { return nil }

type fakeCleaner struct{ dropped int }

func (f *fakeCleaner) CleanupExpiredEntities(_ context.Context, _ time.Time) (int, error) {
	return f.dropped, nil
}

// ---- tests ----

func newTestRecomputeUsecase(repo RecomputeRepo, topic TopicRepo, ts TimeSeries, dyn DynamicsStateStore, hist EngHistogram, redis *fakeRecomputeRedis) *RecomputeUsecase {
	conf := DefaultConf()
	return &RecomputeUsecase{
		conf:    &conf,
		repo:    repo,
		topic:   topic,
		ts:      ts,
		dyn:     dyn,
		hist:    hist,
		weights: &fakeRecomputeWeights{},
		redis:   redis,
		cleaner: &fakeCleaner{},
		log:     log.NewHelper(log.DefaultLogger),
	}
}

// TestRecomputeOneLocked 他人持锁 → 跳过，不清标记不写点。
func TestRecomputeOneLocked(t *testing.T) {
	repo := &fakeRecomputeRepo{lockHeld: true} // 预先持锁
	ts := &fakeRecomputeTS{}
	redis := &fakeRecomputeRedis{marks: map[string]string{}}
	uc := newTestRecomputeUsecase(repo, &fakeRecomputeTopicRepo{}, ts, &fakeRecomputeDyn{stored: map[string]*DynamicsState{}}, nil, redis)

	n, err := uc.RecomputeOne(context.Background(), "tp_x", time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 || len(ts.written) != 0 {
		t.Fatalf("locked topic must not write points: n=%d written=%d", n, len(ts.written))
	}
	if repo.cleared {
		t.Fatal("must not clear lock when acquisition failed")
	}
}

// TestRecomputeOneReplay 序列回放：3 点升序，velocity 链推进单调，覆盖写全点。
func TestRecomputeOneReplay(t *testing.T) {
	base := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	series := []SnapshotPoint{
		{TopicID: "tp_1", Platform: "weibo", TBucket: base, TRaw: base, Rank: 5, RankTotal: 50, Views: 10000, Likes: 100, Comments: 20, HeatScore: 40},
		{TopicID: "tp_1", Platform: "weibo", TBucket: base.Add(30 * time.Minute), TRaw: base.Add(30 * time.Minute), Rank: 3, RankTotal: 50, Views: 12000, Likes: 150, Comments: 30, HeatScore: 50},
		{TopicID: "tp_1", Platform: "weibo", TBucket: base.Add(60 * time.Minute), TRaw: base.Add(60 * time.Minute), Rank: 1, RankTotal: 50, Views: 20000, Likes: 300, Comments: 60, HeatScore: 60},
	}
	repo := &fakeRecomputeRepo{}
	ts := &fakeRecomputeTS{platforms: []string{"weibo"}, series: map[string][]SnapshotPoint{"weibo": series}}
	dyn := &fakeRecomputeDyn{stored: map[string]*DynamicsState{}}
	redis := &fakeRecomputeRedis{marks: map[string]string{}}
	meta := TopicHeatMeta{Category: "A_hot", FirstSeenAt: base.Add(-time.Hour)}
	uc := newTestRecomputeUsecase(repo, &fakeRecomputeTopicRepo{meta: meta}, ts, dyn, &fakeRecomputeHist{pct: 0.7}, redis)

	n, err := uc.RecomputeOne(context.Background(), "tp_1", base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 points rewritten, got %d", n)
	}
	if len(ts.written) != 3 {
		t.Fatalf("expected 3 written, got %d", len(ts.written))
	}
	// 链推进：首点 α=1 重置 velocity=0；后两点 velocity 非零（heat 递增）。
	if ts.written[0].Velocity != 0 {
		t.Errorf("first point velocity must be 0 (α=1 reset), got %v", ts.written[0].Velocity)
	}
	if ts.written[1].Velocity == 0 || ts.written[2].Velocity == 0 {
		t.Errorf("subsequent velocities must be non-zero on rising series: v1=%v v2=%v", ts.written[1].Velocity, ts.written[2].Velocity)
	}
	// heat 已重算（非原 40/50/60，因子合成结果）。
	for i, p := range ts.written {
		if p.HeatScore < 0 || p.HeatScore > 100 {
			t.Errorf("point %d heat_score out of range: %v", i, p.HeatScore)
		}
		if p.WeightEpoch == "" {
			t.Errorf("point %d weight_epoch must be set", i)
		}
	}
	// dyn store 回写终态。
	if dyn.saved == 0 {
		t.Error("dyn store must be saved after replay")
	}
	// 锁已清除。
	if !repo.cleared {
		t.Error("recompute lock must be cleared after success")
	}
}

// TestRecomputeScanConsume 扫描消费：标记清除 + 实体索引清理触发。
func TestRecomputeScanConsume(t *testing.T) {
	base := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	series := []SnapshotPoint{
		{TopicID: "tp_2", Platform: "weibo", TBucket: base, TRaw: base, Rank: 1, RankTotal: 10, Views: 100, Likes: 10, Comments: 2, HeatScore: 50},
	}
	ts := &fakeRecomputeTS{platforms: []string{"weibo"}, series: map[string][]SnapshotPoint{"weibo": series}}
	redis := &fakeRecomputeRedis{
		marks: map[string]string{"tt:recompute:tp_2": "1"},
		keys:  []string{"tt:recompute:tp_2"},
	}
	uc := newTestRecomputeUsecase(&fakeRecomputeRepo{}, &fakeRecomputeTopicRepo{meta: TopicHeatMeta{Category: "A_hot", FirstSeenAt: base}}, ts, &fakeRecomputeDyn{stored: map[string]*DynamicsState{}}, nil, redis)

	n, err := uc.RecomputeScan(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 topic processed, got %d", n)
	}
	if redis.delN != 1 {
		t.Fatalf("expected mark deleted, delN=%d", redis.delN)
	}
	if _, still := redis.marks["tt:recompute:tp_2"]; still {
		t.Fatal("mark must be consumed")
	}
}

// TestRecomputeScanNoRedis Redis 未配置 → 空跑（标记本就没落）。
func TestRecomputeScanNoRedis(t *testing.T) {
	conf := DefaultConf()
	uc := &RecomputeUsecase{
		conf: &conf,
		log:  log.NewHelper(log.DefaultLogger),
	}
	n, err := uc.RecomputeScan(context.Background(), 100)
	if err != nil || n != 0 {
		t.Fatalf("no-redis scan must be no-op: n=%d err=%v", n, err)
	}
}
