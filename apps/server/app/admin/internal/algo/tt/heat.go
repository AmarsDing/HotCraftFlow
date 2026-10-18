// Package tt 实现 ② TT 热点追踪的核心算法。
// 对应 docs/systems/02-TT/03-详细设计.md，并已吸收评审报告的修复
// （docs/systems/review/TT-深度评审.md 的 A-2~A-8）。
//
// 所有算法均为纯函数，不依赖 Ent/DB/Redis，可独立单测。
// 时区纪律：一律以「样本时间 t」为口径，禁止用 now()（评审 B-4：历史点必须可重现）。
package tt

import (
	"math"

	"framework/pkg/mediacontract"
)

// HeatWeights 热度四因子的配置权重。
type HeatWeights struct {
	Rank    float64 // 榜内相对位置，默认 0.35
	Eng     float64 // 互动分位，默认 0.25
	Fresh   float64 // 新鲜度，默认 0.20
	Breadth float64 // 跨平台扩散，默认 0.20
}

// DefaultHeatWeights 返回默认权重 {0.35, 0.25, 0.20, 0.20}。
func DefaultHeatWeights() HeatWeights {
	return HeatWeights{Rank: 0.35, Eng: 0.25, Fresh: 0.20, Breadth: 0.20}
}

// HeatInput 是热度计算的输入（均为样本时间 t 口径的取值）。
// 因子用指针表示「可能缺失」：nil = 缺失（参与权重重分配）。
type HeatInput struct {
	RankPct   *float64 // rank_pct ∈ [0,1]，无榜为 nil
	EngPct    *float64 // eng_pct ∈ [0,1]（恒为平台内分位，与有无曝光无关，评审 A-2）
	Freshness *float64 // freshness ∈ [0,1]
	Breadth   *float64 // breadth ∈ [0,1]
}

// HeatOutput 是热度计算结果（含可解释性所需的全部中间量）。
type HeatOutput struct {
	Score   float64  // 0–100；全因子缺失时为 NaN（调用方应判 latent）
	Factors mediacontract.HeatFactors
	Missing []string // 缺失因子名
	NoFactor bool   // 全因子缺失（评审 A-6：此时不写时序、强制 latent）
}

// clamp01 把值夹到 [0,1]。
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// RankPct 计算榜内相对位置（评审 A-7 统一口径）。
// rank=1 → 1.0（榜首满分）；rank=rank_total → 0.0（榜尾零分）。
// rank 或 rank_total 缺失/≤0 返回 nil（缺失，参与权重重分配）。
//
// ⚠️ 跨平台可比性：本式只做「榜内相对位置」归一化，不解决榜长不同的含金量差异
// （百度第5位与微博第5位不可比）。兜底是「按 heat 的 24h 分位排序」而非绝对值。
func RankPct(rank, rankTotal int32) *float64 {
	if rank <= 0 || rankTotal <= 0 {
		return nil
	}
	den := float64(rankTotal - 1)
	if den < 1 {
		den = 1
	}
	v := clamp01(float64(rankTotal-rank) / den)
	return &v
}

// Freshness 计算新鲜度（评审 A-4：样本时间口径 + 地板）。
// freshness(t) = max(exp(-(t - firstSeen)/τ), f_floor)
// τ = tauHours（默认 12h，半衰期 t_1/2 = τ·ln2 ≈ 8.3h）；f_floor = 0.05。
// t 必须是样本时间戳，禁止传 now()——否则历史点不可重现（评审 B-4）。
func Freshness(t, firstSeen int64, tauHours, floor float64) float64 {
	if tauHours <= 0 {
		tauHours = 12
	}
	if floor <= 0 {
		floor = 0.05
	}
	dtHours := float64(t-firstSeen) / 3600.0
	if dtHours < 0 {
		dtHours = 0 // 防御：first_seen 晚于样本时间
	}
	v := math.Exp(-dtHours / tauHours)
	if v < floor {
		v = floor
	}
	return v
}

// Breadth 计算跨平台扩散度（评审 A-5：零点平移 + 覆盖数自适应）。
// platformCount = 滑动 24h 窗口内的 distinct 平台数（评审 S-17）。
// supportedCount = IC 实际已接入的平台总数；ref = min(8, supportedCount)。
// 单平台 → 0；platformCount ≥ ref → 1。
func Breadth(platformCount, supportedCount int32) float64 {
	ref := supportedCount
	if ref > 8 {
		ref = 8
	}
	if ref < 1 {
		ref = 1
	}
	if platformCount < 0 {
		platformCount = 0
	}
	raw := math.Log1p(float64(platformCount)) / math.Log1p(float64(ref))
	b0 := math.Log1p(1) / math.Log1p(float64(ref)) // 单平台基线
	v := (raw - b0) / (1 - b0)
	return clamp01(v)
}

// ComputeHeat 合成热度（评审 A-6：因子缺失按有效权重重新归一化）。
//
// 规则：
//  - 有效判定：因子非 nil。
//  - 全缺失（Σw=0）→ NoFactor=true，Score=NaN，调用方强制 latent 且不写时序。
//  - 部分缺失 → 权重按有效权重之和归一化（w_i' = w_i / Σw_valid）。
//  - 输出同时保留原始权重与生效权重（审计需要）。
func ComputeHeat(in HeatInput, w HeatWeights) HeatOutput {
	out := HeatOutput{}

	type factor struct {
		name  string
		val   *float64
		w     float64
		eff   float64
		valid bool
	}
	fs := []factor{
		{"rank", in.RankPct, w.Rank, 0, in.RankPct != nil},
		{"eng", in.EngPct, w.Eng, 0, in.EngPct != nil},
		{"fresh", in.Freshness, w.Fresh, 0, in.Freshness != nil},
		{"breadth", in.Breadth, w.Breadth, 0, in.Breadth != nil},
	}

	var sumW float64
	for _, f := range fs {
		if f.valid {
			sumW += f.w
		} else {
			out.Missing = append(out.Missing, f.name)
		}
	}

	out.Factors = mediacontract.HeatFactors{
		WRank: w.Rank, WEng: w.Eng, WFresh: w.Fresh, WBreadth: w.Breadth,
		Missing: out.Missing,
	}

	if sumW <= 0 {
		out.NoFactor = true
		out.Score = math.NaN()
		return out
	}

	var acc float64
	for i := range fs {
		if !fs[i].valid {
			continue
		}
		fs[i].eff = fs[i].w / sumW
		acc += fs[i].eff * clamp01(*fs[i].val)
	}
	out.Score = 100 * acc
	out.Factors.WRankEff = fs[0].eff
	out.Factors.WEngEff = fs[1].eff
	out.Factors.WFreshEff = fs[2].eff
	out.Factors.WBreadthEff = fs[3].eff
	return out
}

// HeatFactorsSumIsOne 校验生效权重之和为 1（评审 A-6 的 CI 恒等断言）。
// 浮点容差 1e-9。全缺失（NoFactor）时返回 true（无权重可言）。
func HeatFactorsSumIsOne(f mediacontract.HeatFactors) bool {
	sum := f.WRankEff + f.WEngEff + f.WFreshEff + f.WBreadthEff
	if sum == 0 {
		return true
	}
	return math.Abs(sum-1.0) < 1e-9
}
