package tt

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// ============================================================================
// HeatEnricher 编排单测：fake 端口验证因子取值/权重口径/动态推进全链路接线。
// ============================================================================

// teTime RFC3339 解析 shorthand（与 tt_test.go 的 mustTime(t, s) 区分用法）。
func teTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

type fakeEnrichRepo struct {
	meta    TopicHeatMeta
	metaErr error
	updates []TopicHeatUpdate
}

func (f *fakeEnrichRepo) FindTopicIDByClusterKey(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeEnrichRepo) CreateTopicWithSource(context.Context, CreateTopicInput) (string, error) {
	return "", nil
}
func (f *fakeEnrichRepo) AttachSource(context.Context, string, AttachSourceInput) error { return nil }
func (f *fakeEnrichRepo) TouchSeen(context.Context, string, time.Time) error            { return nil }
func (f *fakeEnrichRepo) GetTopicHeatMeta(context.Context, string, time.Time) (TopicHeatMeta, error) {
	return f.meta, f.metaErr
}
func (f *fakeEnrichRepo) UpdateTopicHeatState(_ context.Context, _ string, in TopicHeatUpdate) error {
	f.updates = append(f.updates, in)
	return nil
}

type fakeHist struct {
	pct float64
	n   int64
	err error
}

func (f fakeHist) ObserveAndPercentile(context.Context, string, string, time.Time, float64) (float64, int64, error) {
	return f.pct, f.n, f.err
}

type fakeDynStore struct {
	prev   *DynamicsState
	saved  []DynamicsState
	expect []time.Time
}

func (f *fakeDynStore) Load(context.Context, string, string) (*DynamicsState, error) { return f.prev, nil }
func (f *fakeDynStore) SaveIfNotAdvanced(_ context.Context, _, _ string, st DynamicsState, expected time.Time) (bool, error) {
	f.saved = append(f.saved, st)
	f.expect = append(f.expect, expected)
	return true, nil
}

type fakeWeightRepo struct {
	row    WeightConfigRow
	found  bool
	calls  int
	append []WeightConfigRow
}

func (f *fakeWeightRepo) ActiveHeatWeights(context.Context, time.Time) (WeightConfigRow, bool, error) {
	f.calls++
	return f.row, f.found, nil
}
func (f *fakeWeightRepo) ListHeatWeights(context.Context, int) ([]WeightConfigRow, error) {
	return nil, nil
}
func (f *fakeWeightRepo) AppendHeatWeights(_ context.Context, r WeightConfigRow) error {
	f.append = append(f.append, r)
	return nil
}

func newEnricher(repo TopicRepo, hist EngHistogram, dyn DynamicsStateStore, w WeightConfigRepo) *HeatEnricher {
	conf := DefaultConf()
	return NewHeatEnricher(&conf, repo, hist, dyn, w, log.DefaultLogger)
}

func enrichTestPoint() *SnapshotPoint {
	return &SnapshotPoint{
		TopicID: "tp_x", Platform: "weibo",
		TBucket: teTime("2026-10-18T10:00:00Z"), TRaw: teTime("2026-10-18T10:05:00Z"),
		Rank: 3, RankTotal: 50, Likes: 1200, Comments: 300,
	}
}

func TestEnrichFullPath(t *testing.T) {
	first := teTime("2026-10-18T04:05:00Z") // 6h 前首现 → freshness = e^-0.5
	repo := &fakeEnrichRepo{meta: TopicHeatMeta{
		FirstSeenAt: first, Category: "A_hot", PlatformCount24h: 3, Platforms24h: []string{"weibo", "bilibili", "youtube"},
	}}
	hist := fakeHist{pct: 0.8, n: 500}
	dyn := &fakeDynStore{}
	wr := &fakeWeightRepo{found: false} // 无配置行 → 默认权重 + default epoch

	p := enrichTestPoint()
	meta, err := newEnricher(repo, hist, dyn, wr).Enrich(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}

	wantRank, _ := RankPct(3, 50)
	wantFresh := math.Exp(-0.5)
	wantBreadth := Breadth(3, 0)
	want, _ := CombineHeat(DefaultHeatWeights(), wantRank, 0.8, wantFresh, wantBreadth, true, true, true, true)

	if !almostEq(p.RankPct, wantRank) || !almostEq(p.EngPct, 0.8) {
		t.Fatalf("factors: %+v", p)
	}
	if math.Abs(p.Freshness-wantFresh) > 1e-9 {
		t.Fatalf("freshness: %v want %v", p.Freshness, wantFresh)
	}
	if !almostEq(p.Breadth, wantBreadth) || !almostEq(p.HeatScore, want.Score) {
		t.Fatalf("breadth/score: %+v", p)
	}
	if p.WeightEpoch != DefaultWeightEpoch {
		t.Fatalf("epoch: %s", p.WeightEpoch)
	}
	if len(p.Missing) != 0 {
		t.Fatalf("missing: %v", p.Missing)
	}
	// 首点动态：α=1 重置，v/a=0，状态已保存
	if p.Velocity != 0 || p.Accel != 0 || len(dyn.saved) != 1 {
		t.Fatalf("dynamics: v=%v a=%v saved=%d", p.Velocity, p.Accel, len(dyn.saved))
	}
	if !almostEq(dyn.saved[0].HEwma, want.Score) || dyn.saved[0].WeightEpoch != DefaultWeightEpoch {
		t.Fatalf("saved state: %+v", dyn.saved[0])
	}
	// meta 透出（breadth 平台数供 Topic 回写）
	if meta.PlatformCount24h != 3 {
		t.Fatalf("meta: %+v", meta)
	}
}

func TestEnrichNoRankReallocates(t *testing.T) {
	repo := &fakeEnrichRepo{meta: TopicHeatMeta{
		FirstSeenAt: teTime("2026-10-18T04:05:00Z"), Category: "A_hot", PlatformCount24h: 1,
	}}
	p := enrichTestPoint()
	p.Rank, p.RankTotal = SentinelRank, SentinelRankTotal // 无榜

	_, err := newEnricher(repo, fakeHist{0.5, 500, nil}, &fakeDynStore{}, &fakeWeightRepo{}).Enrich(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Missing) != 1 || p.Missing[0] != FactorRank {
		t.Fatalf("missing: %v", p.Missing)
	}
	if !almostEq(p.WRankEff, 0) {
		t.Fatalf("w_rank_eff must be 0: %v", p.WRankEff)
	}
	sum := p.WRankEff + p.WEngEff + p.WFreshEff + p.WBreadthEff
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("eff sum: %v", sum)
	}
}

func TestEnrichEngFallbacks(t *testing.T) {
	repo := &fakeEnrichRepo{meta: TopicHeatMeta{
		FirstSeenAt: teTime("2026-10-18T04:05:00Z"), Category: "A_hot", PlatformCount24h: 1,
	}}

	// 冷启动（N<200）→ 静态基线
	p := enrichTestPoint()
	if _, err := newEnricher(repo, fakeHist{0.9, 50, nil}, &fakeDynStore{}, &fakeWeightRepo{}).Enrich(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	raw := math.Log1p(float64(1200 + 300))
	if !almostEq(p.EngPct, BaselineEngPct("weibo", raw)) {
		t.Fatalf("cold start must use baseline: %v", p.EngPct)
	}

	// 存储故障 → 静态基线，且不标 missing（A-3：口径不随基础设施跳变）
	p = enrichTestPoint()
	if _, err := newEnricher(repo, fakeHist{0, 0, errors.New("redis down")}, &fakeDynStore{}, &fakeWeightRepo{}).Enrich(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !almostEq(p.EngPct, BaselineEngPct("weibo", raw)) {
		t.Fatalf("store error must use baseline: %v", p.EngPct)
	}
	for _, m := range p.Missing {
		if m == FactorEng {
			t.Fatal("eng must NEVER be marked missing (A-3)")
		}
	}
}

func TestEnrichMetaErrorIsFatal(t *testing.T) {
	repo := &fakeEnrichRepo{metaErr: errors.New("pg down")}
	p := enrichTestPoint()
	_, err := newEnricher(repo, fakeHist{0.5, 500, nil}, &fakeDynStore{}, &fakeWeightRepo{}).Enrich(context.Background(), p)
	if err == nil || errors.Is(err, ErrNoFactor) {
		t.Fatalf("meta error must propagate as retryable: %v", err)
	}
}

func TestEnrichWeightEpochAndCache(t *testing.T) {
	repo := &fakeEnrichRepo{meta: TopicHeatMeta{
		FirstSeenAt: teTime("2026-10-18T04:05:00Z"), Category: "A_hot", PlatformCount24h: 1,
	}}
	wr := &fakeWeightRepo{
		found: true,
		row: WeightConfigRow{
			ConfigID: "wc_01JABC", Scope: "heat",
			Weights:       HeatWeights{0.5, 0.2, 0.2, 0.1},
			EffectiveFrom: teTime("2026-10-17T00:00:00Z"),
		},
	}
	e := newEnricher(repo, fakeHist{0.5, 500, nil}, &fakeDynStore{}, wr)

	p := enrichTestPoint()
	if _, err := e.Enrich(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	wantEpoch := WeightEpochOf("wc_01JABC")
	if p.WeightEpoch != wantEpoch {
		t.Fatalf("epoch: got %s want %s", p.WeightEpoch, wantEpoch)
	}
	// 第二次 enrich 命中缓存（repo 只被查一次）
	p2 := enrichTestPoint()
	p2.TRaw = p2.TRaw.Add(30 * time.Minute)
	p2.TBucket = p2.TBucket.Add(30 * time.Minute)
	if _, err := e.Enrich(context.Background(), p2); err != nil {
		t.Fatal(err)
	}
	if wr.calls != 1 {
		t.Fatalf("weight cache miss: repo calls=%d", wr.calls)
	}
	// epoch 变更 → 动态链重置：第二点 heat 直接采信
	dyn := &fakeDynStore{prev: &DynamicsState{
		HEwma: 10, VEwma: 5, LastT: p.TBucket, WeightEpoch: DefaultWeightEpoch,
	}}
	e2 := newEnricher(repo, fakeHist{0.5, 500, nil}, dyn, wr)
	p3 := enrichTestPoint()
	p3.TBucket = p.TBucket.Add(30 * time.Minute)
	p3.TRaw = p.TRaw.Add(30 * time.Minute)
	if _, err := e2.Enrich(context.Background(), p3); err != nil {
		t.Fatal(err)
	}
	if p3.Velocity != 0 || p3.Accel != 0 {
		t.Fatalf("epoch change must reset v/a: v=%v a=%v", p3.Velocity, p3.Accel)
	}
	if len(dyn.saved) != 1 || dyn.saved[0].WeightEpoch != wantEpoch {
		t.Fatalf("reset save: %+v", dyn.saved)
	}
}

func TestEnrichOutOfOrderSkipsDynamics(t *testing.T) {
	repo := &fakeEnrichRepo{meta: TopicHeatMeta{
		FirstSeenAt: teTime("2026-10-18T04:05:00Z"), Category: "A_hot", PlatformCount24h: 1,
	}}
	// 已推进到 10:30 的状态，来了 10:00 的迟到点
	dyn := &fakeDynStore{prev: &DynamicsState{
		HEwma: 55, VEwma: 2, LastT: teTime("2026-10-18T10:30:00Z"), WeightEpoch: DefaultWeightEpoch,
	}}
	p := enrichTestPoint() // TBucket=10:00
	if _, err := newEnricher(repo, fakeHist{0.5, 500, nil}, dyn, &fakeWeightRepo{}).Enrich(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p.Velocity != 0 || p.Accel != 0 {
		t.Fatalf("out-of-order must zero v/a: v=%v a=%v", p.Velocity, p.Accel)
	}
	if len(dyn.saved) != 0 {
		t.Fatalf("out-of-order must NOT save state: %+v", dyn.saved)
	}
}

func TestWeightUsecaseLifecycle(t *testing.T) {
	wr := &fakeWeightRepo{}
	uc := NewWeightUsecase(wr, log.DefaultLogger)
	ctx := context.Background()

	// 空表 → 默认权重 + default epoch
	v, err := uc.CurrentHeat(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Weights != DefaultHeatWeights() || v.Epoch != DefaultWeightEpoch || !v.Effective {
		t.Fatalf("default view: %+v", v)
	}

	// 非法权重被拒（权重和为 1 校验）
	if _, err = uc.UpdateHeat(ctx, HeatWeights{0.5, 0.3, 0.1, 0.05}, "op", ""); err == nil {
		t.Fatal("sum!=1 must be rejected")
	}

	// 合法变更 → 插行 + epoch 派生
	v, err = uc.UpdateHeat(ctx, HeatWeights{0.4, 0.3, 0.2, 0.1}, "admin", "提榜权")
	if err != nil {
		t.Fatal(err)
	}
	if len(wr.append) != 1 {
		t.Fatalf("append: %d", len(wr.append))
	}
	if v.Epoch != WeightEpochOf(v.ConfigID) {
		t.Fatalf("epoch derivation: %+v", v)
	}
	if v.EffectiveFrom.IsZero() || v.EffectiveFrom.Location() != time.UTC {
		t.Fatalf("effective_from must be UTC now: %v", v.EffectiveFrom)
	}
}
