package tt

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---- fakes ----

type fakeLifecycleRepo struct {
	topics      []LifecycleTopicRow
	updated     []StageUpdate
	updateCalls []string // topic_id 轨迹
	err         error
}

func (f *fakeLifecycleRepo) ListActiveTopics(context.Context, int) ([]LifecycleTopicRow, error) {
	return f.topics, f.err
}
func (f *fakeLifecycleRepo) UpdateStage(_ context.Context, topicID string, in StageUpdate) (bool, error) {
	f.updateCalls = append(f.updateCalls, topicID)
	f.updated = append(f.updated, in)
	if f.err != nil {
		return false, f.err
	}
	return true, nil
}

type fakeTimeSeries struct {
	points []SnapshotPoint
	heats  []float64
	err    error
}

func (f *fakeTimeSeries) WriteSnapshots(context.Context, []SnapshotPoint) error { return f.err }
func (f *fakeTimeSeries) Ping(context.Context) (bool, error)                    { return true, f.err }
func (f *fakeTimeSeries) ReadRecentPoints(_ context.Context, _, _ string, limit int) ([]SnapshotPoint, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.points) > limit {
		return f.points[:limit], nil
	}
	return f.points, nil
}
func (f *fakeTimeSeries) ReadActiveHeatRange(context.Context, time.Time, time.Time) ([]float64, error) {
	return f.heats, f.err
}

// ---- helpers ----

func evalFixture(repo *fakeLifecycleRepo, ts *fakeTimeSeries) *LifecycleEvaluator {
	uc, err := NewLifecycleEvaluator(repo, ts, DefaultLifecycleConf())
	if err != nil {
		panic(err)
	}
	return uc
}

func mkPoint(heat, v float64, t time.Time) SnapshotPoint {
	return SnapshotPoint{HeatScore: heat, Velocity: v, TRaw: t}
}

// ---- tests ----

// TestEvaluateOneNoPoints 无数据点保持现状（B-6：任何输入都有归宿，不 panic）。
func TestEvaluateOneNoPoints(t *testing.T) {
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{{TopicID: "tp_01JA", Stage: StageLatent}}}
	ts := &fakeTimeSeries{points: nil}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], fixedTime())
	if err != nil || changed {
		t.Fatalf("无点应无变更: changed=%v err=%v", changed, err)
	}
	if len(repo.updateCalls) != 0 {
		t.Fatal("无点不得落库")
	}
}

// TestEvaluateOneRisingStreak 连续 2 点 v>6.0 → rising（规则 3）。
func TestEvaluateOneRisingStreak(t *testing.T) {
	now := fixedTime()
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{
		{TopicID: "tp_01JA", Stage: StageLatent, StageSince: now.Add(-2 * time.Hour), StageDurationPoints: 1},
	}}
	ts := &fakeTimeSeries{
		points: []SnapshotPoint{
			mkPoint(30, 8.0, now),                 // 最新
			mkPoint(25, 7.0, now.Add(-time.Hour)), // 次新（streak=2）
			mkPoint(20, 1.0, now.Add(-2*time.Hour)),
		},
		heats: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}, // P25=25, P99=90.1
	}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], now)
	if err != nil || !changed {
		t.Fatalf("rising 应变更: changed=%v err=%v", changed, err)
	}
	if repo.updated[0].Stage != StageRising {
		t.Fatalf("目标阶段应 rising: %s", repo.updated[0].Stage)
	}
	if repo.updated[0].StageDurationPoints != 0 {
		t.Fatalf("新阶段点数应清零: %d", repo.updated[0].StageDurationPoints)
	}
}

// TestEvaluateOneDecayingStreak 连续 3 点 v<0 → decaying（规则 4）。
func TestEvaluateOneDecayingStreak(t *testing.T) {
	now := fixedTime()
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{
		{TopicID: "tp_01JB", Stage: StagePeak, StageSince: now.Add(-3 * time.Hour), StageDurationPoints: 3},
	}}
	ts := &fakeTimeSeries{
		points: []SnapshotPoint{
			mkPoint(50, -2.0, now),
			mkPoint(55, -1.5, now.Add(-time.Hour)),
			mkPoint(60, -1.0, now.Add(-2*time.Hour)),
		},
		heats: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], now)
	if err != nil || !changed {
		t.Fatalf("decaying 应变更: changed=%v err=%v", changed, err)
	}
	if repo.updated[0].Stage != StageDecaying {
		t.Fatalf("目标阶段应 decaying: %s", repo.updated[0].Stage)
	}
}

// TestEvaluateOnePeakAbsolute 绝对线 heat≥80 → peak（规则 2，无 streak 依赖）。
// rising 转出需驻满 2 点（B-7）——StageDurationPoints=2 放行。
// 点数须 ≥3：规则 0（n<3 强制 latent）是有序判据第一条，压过绝对线。
func TestEvaluateOnePeakAbsolute(t *testing.T) {
	now := fixedTime()
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{
		{TopicID: "tp_01JC", Stage: StageRising, StageSince: now.Add(-2 * time.Hour), StageDurationPoints: 2},
	}}
	ts := &fakeTimeSeries{
		points: []SnapshotPoint{
			mkPoint(85, 3.0, now),                  // heat≥80 → 规则 2
			mkPoint(80, 2.0, now.Add(-time.Hour)),
			mkPoint(75, 1.0, now.Add(-2*time.Hour)), // 第三点跳出规则 0
		},
		heats: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], now)
	if err != nil || !changed {
		t.Fatalf("peak 应变更: changed=%v err=%v", changed, err)
	}
	if repo.updated[0].Stage != StagePeak {
		t.Fatalf("目标阶段应 peak: %s", repo.updated[0].Stage)
	}
}

// TestEvaluateOneColdStartP99 冷启动 heats<4 点 → P99=0，相对线禁用（对抗裁决修复）。
func TestEvaluateOneColdStartP99(t *testing.T) {
	now := fixedTime()
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{
		{TopicID: "tp_01JD", Stage: StageLatent, StageSince: now.Add(-time.Hour), StageDurationPoints: 1},
	}}
	ts := &fakeTimeSeries{
		points: []SnapshotPoint{mkPoint(50, 2.0, now)},
		heats:  []float64{10, 20, 30}, // <4 点，P99 禁用
	}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], now)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if changed {
		// 50 < 80 绝对线不命中，P99 禁用，应无变更
		t.Fatalf("冷启动 P99 禁用时不应误判 peak: %+v", repo.updated)
	}
}

// TestEvaluateOneStreakBreak 连续中断（v 符号交替）→ streak=1，不满足规则 3/4。
func TestEvaluateOneStreakBreak(t *testing.T) {
	now := fixedTime()
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{
		{TopicID: "tp_01JE", Stage: StageLatent, StageSince: now.Add(-2 * time.Hour), StageDurationPoints: 1},
	}}
	ts := &fakeTimeSeries{
		points: []SnapshotPoint{
			mkPoint(30, 8.0, now),                  // 最新正
			mkPoint(25, -1.0, now.Add(-time.Hour)), // 次新负（中断）
			mkPoint(20, 7.0, now.Add(-2*time.Hour)),
		},
		heats: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], now)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if changed {
		t.Fatalf("streak 中断应无变更: %+v", repo.updated)
	}
}

// TestEvaluateOneEvidence 变更证据 JSON 含规则编号与双阶段。
func TestEvaluateOneEvidence(t *testing.T) {
	now := fixedTime()
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{
		{TopicID: "tp_01JF", Stage: StageLatent, StageSince: now.Add(-2 * time.Hour), StageDurationPoints: 1},
	}}
	ts := &fakeTimeSeries{
		points: []SnapshotPoint{
			mkPoint(30, 8.0, now),
			mkPoint(25, 7.0, now.Add(-time.Hour)),
			mkPoint(20, 6.5, now.Add(-2*time.Hour)), // 第三点：n_points≥3 跳出规则 0
		},
		heats: []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
	}
	changed, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], now)
	if err != nil || !changed {
		t.Fatalf("err=%v changed=%v", err, changed)
	}
	ev := repo.updated[0].Evidence
	if ev == "" {
		t.Fatal("证据 JSON 不得为空")
	}
	if !contains(ev, `"rule":3`) || !contains(ev, `"from":"latent"`) || !contains(ev, `"to":"rising"`) {
		t.Fatalf("证据应含规则 3 与 latent→rising: %s", ev)
	}
}

// TestEvaluateOneError 端口故障上抛。
func TestEvaluateOneError(t *testing.T) {
	repo := &fakeLifecycleRepo{topics: []LifecycleTopicRow{{TopicID: "tp_01JG", Stage: StageLatent}}}
	ts := &fakeTimeSeries{err: errors.New("influx down")}
	_, err := evalFixture(repo, ts).EvaluateOne(context.Background(), repo.topics[0], fixedTime())
	if err == nil {
		t.Fatal("时序故障应上抛")
	}
}

// TestPercentile 线性插值分位。
func TestPercentile(t *testing.T) {
	sorted := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	// 线性插值：idx=0.25×9=2.25 → 30×0.75+40×0.25=32.5（非近邻秩的 25）
	if got := percentile(sorted, 0.25); got != 32.5 {
		t.Fatalf("P25 线性插值应 32.5: %v", got)
	}
	if got := percentile(sorted, 0.99); got < 99.0 || got > 99.5 {
		t.Fatalf("P99 应≈99: %v", got)
	}
	if got := percentile(sorted, 0.0); got != 10.0 {
		t.Fatalf("P0 应 10: %v", got)
	}
	if got := percentile(sorted, 1.0); got != 100.0 {
		t.Fatalf("P100 应 100: %v", got)
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("空应 0: %v", got)
	}
	if got := percentile([]float64{42}, 0.99); got != 42 {
		t.Fatalf("单点应自身: %v", got)
	}
}

// contains 轻量子串检查（避免 strings 依赖）。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
