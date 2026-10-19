package tt

import (
	"testing"

	contract "framework/pkg/mediacontract"
)

// TestSurgeNoBurst：平稳序列、低速、排名无跃迁 → 不触发。
func TestSurgeNoBurst(t *testing.T) {
	d := DefaultSurgeConfig()
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{30, 31, 30.5, 31.5, 30.8},
		Velocity: 0.4,
		HeatNow:  30.8,
		RankNow:  40,
		RankPrev: 42,
	})
	if v.Fired {
		t.Fatalf("calm series must not fire: %+v", v)
	}
	if len(v.Methods) != 0 {
		t.Fatalf("no method should hit on calm series: %+v", v.Methods)
	}
}

// TestSurgeSingleMethodDoesNotFire：单方法命中显式不触发（契约 §7 ≥2 红线）。
func TestSurgeSingleMethodDoesNotFire(t *testing.T) {
	d := DefaultSurgeConfig()
	// 仅双阈值增速命中（velocity=8>VMin=5 且 heat=45>HMin=30）；zscore 平稳；排名无跃迁
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{30, 31, 30.5, 31.5, 31.8},
		Velocity: 8,
		HeatNow:  45,
		RankNow:  30,
		RankPrev: 33,
	})
	if v.Fired {
		t.Fatalf("single method must not fire: %+v", v)
	}
	if len(v.Methods) != 1 || v.Methods[0] != contract.BurstVelocity {
		t.Fatalf("expected exactly velocity hit, got %+v", v.Methods)
	}
	if got := v.Evidence[string(contract.BurstVelocity)]; got != 8 {
		t.Fatalf("velocity evidence = %v, want 8", got)
	}
}

// TestSurgeZScorePlusVelocityFires：Z-score 突变 + 双阈值增速 → 触发，两方法各带数值。
func TestSurgeZScorePlusVelocityFires(t *testing.T) {
	d := DefaultSurgeConfig()
	// 历史平稳（μ≈20，σ 小）+ 最新点 45 → 残差 >>3
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{20, 20.5, 19.5, 20.2, 45},
		Velocity: 8,
		HeatNow:  45,
		RankNow:  30,
		RankPrev: 31,
	})
	if !v.Fired {
		t.Fatalf("zscore+velocity must fire: %+v", v)
	}
	if len(v.Methods) != 2 {
		t.Fatalf("want 2 methods, got %+v", v.Methods)
	}
	for _, m := range []contract.BurstMethod{contract.BurstZScore, contract.BurstVelocity} {
		if _, ok := v.Evidence[string(m)]; !ok {
			t.Fatalf("evidence missing for %s: %+v", m, v.Evidence)
		}
	}
	if v.Evidence[string(contract.BurstVelocity)] != 8 {
		t.Fatalf("velocity evidence mismatch: %+v", v.Evidence)
	}
}

// TestSurgeRankJumpFirstEntry：首进榜 + 双阈值 → 触发；首进榜数值 = 当前名次。
func TestSurgeRankJumpFirstEntry(t *testing.T) {
	d := DefaultSurgeConfig()
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{35, 36, 35.5, 36.5, 37},
		Velocity: 7,
		HeatNow:  45,
		RankNow:  8, // 首进榜第 8
		RankPrev: 0, // 上一周期榜外
	})
	if !v.Fired {
		t.Fatalf("first-entry + velocity must fire: %+v", v)
	}
	if got := v.Evidence[string(contract.BurstRankJump)]; got != 8 {
		t.Fatalf("first-entry evidence = current rank 8, got %v", got)
	}
}

// TestSurgeRankJumpLargeLeap：在榜跃升 ≥N 位 + Z-score → 触发；数值 = 跃升位数。
func TestSurgeRankJumpLargeLeap(t *testing.T) {
	d := DefaultSurgeConfig()
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{20, 20.5, 19.5, 20.2, 42},
		Velocity: 0.5,
		HeatNow:  42,
		RankNow:  5,
		RankPrev: 50, // 跃升 45 ≥ 20
	})
	if !v.Fired {
		t.Fatalf("leap + zscore must fire: %+v", v)
	}
	if got := v.Evidence[string(contract.BurstRankJump)]; got != 45 {
		t.Fatalf("leap evidence = 45, got %v", got)
	}
}

// TestSurgeRankOutOfListNoHit：榜外→榜外不构成跃迁。
func TestSurgeRankOutOfListNoHit(t *testing.T) {
	d := DefaultSurgeConfig()
	if _, ok := d.rankJump(SurgeInput{RankNow: 0, RankPrev: 0}); ok {
		t.Fatalf("out-of-list → out-of-list must not hit rank_jump")
	}
	if _, ok := d.rankJump(SurgeInput{RankNow: 0, RankPrev: 10}); ok {
		t.Fatalf("drop out of list must not hit rank_jump")
	}
}

// TestSurgeZScoreAbstainsShortSeries：序列 <4 点 → zscore 弃权（不猜）。
func TestSurgeZScoreAbstainsShortSeries(t *testing.T) {
	d := DefaultSurgeConfig()
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{10, 50}, // 2 点：即使突变也不判 zscore
		Velocity: 100,
		HeatNow:  50,
		RankNow:  1,
		RankPrev: 0,
	})
	for _, m := range v.Methods {
		if m == contract.BurstZScore {
			t.Fatalf("zscore must abstain on short series: %+v", v)
		}
	}
	// velocity + rank_jump 两方法仍可独立触发
	if !v.Fired {
		t.Fatalf("velocity+first-entry should still fire: %+v", v)
	}
}

// TestSurgeZScoreAbstainsConstantSeries：恒定序列 σ=0 → 弃权（除零防护）。
func TestSurgeZScoreAbstainsConstantSeries(t *testing.T) {
	d := DefaultSurgeConfig()
	if _, ok := d.zscore(SurgeInput{HeatAsc: []float64{20, 20, 20, 20, 25}}); ok {
		t.Fatalf("constant history (σ=0) must abstain")
	}
}

// TestSurgeEvidenceCoversAllHitMethods：触发时 Evidence 键集合与 Methods 一致
//（直接对齐契约 Alert.Validate 的硬校验口径，防接线后落库被拒）。
func TestSurgeEvidenceCoversAllHitMethods(t *testing.T) {
	d := DefaultSurgeConfig()
	v := d.Detect(SurgeInput{
		HeatAsc:  []float64{20, 20.5, 19.5, 20.2, 45},
		Velocity: 8,
		HeatNow:  45,
		RankNow:  10,
		RankPrev: 0,
	})
	if !v.Fired {
		t.Fatalf("triple-hit scenario must fire: %+v", v)
	}
	for _, m := range v.Methods {
		if _, ok := v.Evidence[string(m)]; !ok {
			t.Fatalf("method %s lacks evidence value (契约 §7)", m)
		}
	}
	if len(v.Evidence) != len(v.Methods) {
		t.Fatalf("evidence keys must exactly match hit methods: %+v vs %+v", v.Evidence, v.Methods)
	}
}
