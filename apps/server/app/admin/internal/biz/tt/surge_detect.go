package tt

import (
	"math"

	contract "framework/pkg/mediacontract"
)

// SurgeDetector 突发检测决策核心（FR-TRK-05；02-TT/01-需求规格 §3.3 三方法投票）。
//
// 纯逻辑件：输入热度时序/最新动态/相邻周期排名，输出投票结果与逐方法依据。
// 与 M3-b/c 同模式——先决策核心后接线；worker 调度/conf 阈值/service API
// 随 M4 数据层一并落位（评审 B 系列口径）。
//
// 验收红线（契约 §7 / 需求 §3.3）：
//  1. ≥2 个方法命中才 Fired——单方法命中显式不触发
//  2. 每个命中方法必须给具体数值（Evidence）——无数值的告警不允许发出
//  3. 数据不足以判某方法时该方法弃权（不猜），由其余方法投票
type SurgeDetector struct {
	KSigma    float64 // Z-score 残差阈值（默认 3）
	VMin      float64 // 双阈值增速：velocity 下界
	HMin      float64 // 双阈值增速：heat_score 下界
	RankJumpN int     // 榜单跃迁：单周期跃升 ≥N 位
}

// DefaultSurgeConfig 文档默认阈值（02-TT/01-需求规格 §3.3）。
func DefaultSurgeConfig() SurgeDetector {
	return SurgeDetector{KSigma: 3, VMin: 5, HMin: 30, RankJumpN: 20}
}

// SurgeInput 单话题单周期检测输入。
type SurgeInput struct {
	HeatAsc  []float64 // heat_score 升序时序（旧→新）；判定 Z-score 需 ≥4 点（历史 ≥3）
	Velocity float64   // 最新 EWMA velocity（biz/tt/dynamics.go 口径）
	HeatNow  float64   // 最新 heat_score
	RankNow  int       // 本周期排名；榜外 = 0
	RankPrev int       // 上一周期排名；榜外 = 0
}

// SurgeVerdict 投票结果。
type SurgeVerdict struct {
	Fired    bool
	Methods  []contract.BurstMethod
	Evidence map[string]float64 // 逐方法具体数值（契约 §7 硬要求）
}

// Detect 三方法投票。每个方法独立判定；≥2 命中才 Fired。
func (d SurgeDetector) Detect(in SurgeInput) SurgeVerdict {
	v := SurgeVerdict{Evidence: map[string]float64{}}
	if val, ok := d.zscore(in); ok {
		v.Methods = append(v.Methods, contract.BurstZScore)
		v.Evidence[string(contract.BurstZScore)] = val
	}
	if val, ok := d.velocity(in); ok {
		v.Methods = append(v.Methods, contract.BurstVelocity)
		v.Evidence[string(contract.BurstVelocity)] = val
	}
	if val, ok := d.rankJump(in); ok {
		v.Methods = append(v.Methods, contract.BurstRankJump)
		v.Evidence[string(contract.BurstRankJump)] = val
	}
	v.Fired = len(v.Methods) >= 2
	return v
}

// zscore 残差法：历史（除最新点）均值 μ、标准差 σ，残差 = (最新-μ)/σ > k 命中。
// 数据不足（总点数 <4）或 σ=0（恒定序列无突变语义）→ 弃权。
func (d SurgeDetector) zscore(in SurgeInput) (float64, bool) {
	n := len(in.HeatAsc)
	if n < 4 {
		return 0, false
	}
	hist := in.HeatAsc[:n-1]
	mean := 0.0
	for _, h := range hist {
		mean += h
	}
	mean /= float64(len(hist))
	variance := 0.0
	for _, h := range hist {
		variance += (h - mean) * (h - mean)
	}
	sigma := math.Sqrt(variance / float64(len(hist)))
	if sigma == 0 {
		return 0, false
	}
	residual := (in.HeatAsc[n-1] - mean) / sigma
	return residual, residual > d.KSigma
}

// velocity 双阈值：velocity > VMin 且 heat_score > HMin 同时成立才命中
//（避免低基数噪声——低热度的增速无告警价值）。数值取 velocity（触发量）。
func (d SurgeDetector) velocity(in SurgeInput) (float64, bool) {
	if in.Velocity > d.VMin && in.HeatNow > d.HMin {
		return in.Velocity, true
	}
	return 0, false
}

// rankJump 榜单跃迁：首进榜（prev 榜外 → now 榜内）或单周期跃升 ≥N 位。
// 数值口径：跃升 = prev-now（位）；首进榜 = 当前名次（越小越强，见需求 §3.3 附注）。
// 榜外→榜外不构成命中。
func (d SurgeDetector) rankJump(in SurgeInput) (float64, bool) {
	if in.RankNow <= 0 {
		return 0, false // 本周期不在榜，无跃迁语义
	}
	if in.RankPrev <= 0 {
		return float64(in.RankNow), true // 首进榜
	}
	jump := in.RankPrev - in.RankNow
	if jump >= d.RankJumpN {
		return float64(jump), true
	}
	return 0, false
}
