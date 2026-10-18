package tt

import (
	"math"
	"testing"

	"framework/pkg/mediacontract"
)

func TestRankPct(t *testing.T) {
	// 榜首满分，榜尾零分（评审 A-7）。
	if got := RankPct(1, 50); got == nil || *got != 1.0 {
		t.Fatalf("rank=1/50 应为 1.0，got %v", got)
	}
	if got := RankPct(50, 50); got == nil || *got != 0.0 {
		t.Fatalf("rank=50/50 应为 0.0，got %v", got)
	}
	if got := RankPct(0, 50); got != nil {
		t.Fatalf("rank=0（无榜）应为 nil（缺失），got %v", *got)
	}
}

func TestFreshnessSampleTime(t *testing.T) {
	first := int64(0)
	// 同一历史点用相同样本时间重算，结果必须一致（可重现，评审 B-4）。
	t0 := int64(12 * 3600) // 12h 后
	a := Freshness(t0, first, 12, 0.05)
	b := Freshness(t0, first, 12, 0.05)
	if a != b {
		t.Fatalf("相同样本时间必须可重现")
	}
	// τ=12h 时，12h 后 exp(-1)≈0.368。
	if math.Abs(a-math.Exp(-1)) > 1e-6 {
		t.Fatalf("freshness(12h) 应≈exp(-1)=0.368，got %f", a)
	}
	// 地板：1000h 后仍 ≥ 0.05（评审 S-3：老话题不会归零）。
	if got := Freshness(1000*3600, first, 12, 0.05); got != 0.05 {
		t.Fatalf("freshness 应有 0.05 地板，got %f", got)
	}
}

func TestBreadthZeroPoint(t *testing.T) {
	// 单平台必须为 0（评审 S-4：原文档单平台得 0.3155 是语义错误）。
	if got := Breadth(1, 8); got != 0 {
		t.Fatalf("单平台 breadth 应为 0，got %f", got)
	}
	// 覆盖 8 平台 → 1。
	if got := Breadth(8, 8); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("8 平台 breadth 应为 1，got %f", got)
	}
	// 覆盖数 <8 时上限自适应（评审 S-4）：supported=5 时，5 平台应能取满。
	if got := Breadth(5, 5); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("supported=5 时 5 平台应取满，got %f", got)
	}
}

func TestComputeHeatMissingRealloc(t *testing.T) {
	w := DefaultHeatWeights()
	one := 1.0
	// 只有 rank 因子有效：权重应全部归给它（eff=1.0）。
	out := ComputeHeat(HeatInput{RankPct: &one}, w)
	if out.NoFactor {
		t.Fatalf("单因子不应判 NoFactor")
	}
	if !HeatFactorsSumIsOne(out.Factors) {
		t.Fatalf("生效权重之和应为 1（CI 恒等断言）")
	}
	if math.Abs(out.Score-100) > 1e-6 {
		t.Fatalf("单因子满分应为 100，got %f", out.Score)
	}
	if len(out.Missing) != 3 {
		t.Fatalf("应有 3 个缺失因子，got %v", out.Missing)
	}
}

func TestComputeHeatAllMissing(t *testing.T) {
	// 全因子缺失 → heat=NaN + NoFactor（评审 A-6：不写时序、强制 latent）。
	out := ComputeHeat(HeatInput{}, DefaultHeatWeights())
	if !out.NoFactor || !math.IsNaN(out.Score) {
		t.Fatalf("全缺失应 NoFactor + NaN")
	}
}

func TestLifecycleNoHole(t *testing.T) {
	// 评审 B-6 的反例：heat=50, velocity=+2, n=5 时必须落到某阶段（不得无匹配）。
	st := DecideStage(LifecycleInput{
		CurrentStage: mediacontract.StageLatent,
		NPoints:      5, Heat: 50, VelocityEWMA: 2,
		P99Heat24h: 90, P25Heat24h: 10,
	})
	if !st.Valid() {
		t.Fatalf("状态机不得产生非法阶段，got %s", st)
	}
	// n<3 强制 latent。
	if st := DecideStage(LifecycleInput{NPoints: 2, Heat: 99}); st != mediacontract.StageLatent {
		t.Fatalf("n<3 应强制 latent，got %s", st)
	}
	// 48h 无更新 → dormant。
	if st := DecideStage(LifecycleInput{NPoints: 10, Heat: 50, HoursSinceUpdate: 50}); st != mediacontract.StageDormant {
		t.Fatalf("48h 无更新应为 dormant，got %s", st)
	}
}

func TestLifecycleMinDwellPoints(t *testing.T) {
	// 评审 B-7：rising 最小驻留 2 点。dwell=1 不允许转出 rising。
	if CanTransition(mediacontract.StageRising, mediacontract.StageDecaying, 1) {
		t.Fatalf("rising 驻留 1 点不应允许转出（min=2）")
	}
	if !CanTransition(mediacontract.StageRising, mediacontract.StageDecaying, 2) {
		t.Fatalf("rising 驻留 2 点应允许转出")
	}
}

// TestBurstChannelVoting 验证评审 A-12 的核心修复：
// 只有 heat 通道的两个方法命中（zscore+velocity）不得判突发——因为它们数学相关。
func TestBurstChannelVoting(t *testing.T) {
	cfg := DefaultBurstConfig()
	// 构造一个 heat 持续上升的序列（会让 zscore 与 velocity 同时命中）。
	pts := make([]Point, 12)
	for i := range pts {
		pts[i] = Point{T: int64(i * 1800), Heat: float64(30 + i*5), Rank: 0, RankTotal: 0}
	}
	res := DetectBurst(pts, cfg)
	// 即使 zscore 与 velocity 都命中（同通道），也**不得**判突发。
	heatOnly := len(res.ChannelsHit) == 1 && res.ChannelsHit[mediacontract.ChannelHeat]
	if heatOnly && res.IsBurst {
		t.Fatalf("仅 heat 通道命中不得判突发（评审 A-12：ρ=1 的两个方法不算两票）")
	}
}

// TestBurstCrossChannel 验证跨通道命中才判突发。
func TestBurstCrossChannel(t *testing.T) {
	cfg := DefaultBurstConfig()
	cfg.EngBaselineMedian = 500 // 有互动基线
	pts := make([]Point, 12)
	for i := range pts {
		pts[i] = Point{T: int64(i * 1800), Heat: 40, Rank: 0, RankTotal: 0, Eng: 500}
	}
	// 最后一个点：heat 暴涨 + 互动暴涨（两个独立通道）。
	pts[11].Heat = 95
	pts[11].Eng = 5000
	res := DetectBurst(pts, cfg)
	if len(res.ChannelsHit) >= 2 && !res.IsBurst {
		t.Fatalf("跨通道 ≥2 票应判突发")
	}
}

// TestBurstColdStart 验证冷启动分级（评审 A-13）。
func TestBurstColdStart(t *testing.T) {
	cfg := DefaultBurstConfig()
	// n<3 不检测。
	res := DetectBurst([]Point{{T: 0, Heat: 10}, {T: 1800, Heat: 90}}, cfg)
	if !res.Skipped {
		t.Fatalf("n<3 应跳过检测（不计入误报率分母）")
	}
	if res.IsBurst {
		t.Fatalf("n<3 不得判突发")
	}
}
