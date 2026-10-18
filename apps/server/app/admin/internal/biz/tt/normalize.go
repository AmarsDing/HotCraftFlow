package tt

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ============================================================================
// 热度归一化（02-TT 03-详细设计.md §2.1 + 深度评审 A-2/A-4/A-5/A-6/A-7 终稿口径）。
//
// 四因子：
//   rank_pct   榜内相对位置（A-7 统一公式，缺失 → 不参与，权重重分配）
//   eng_pct    平台内互动对数分位（A-2：恒为分位，与有无曝光无关；eng_rate 仅展示）
//   freshness  新鲜度指数衰减 + 地板（A-4：样本时间口径，禁止 now()，历史可重现）
//   breadth    跨平台扩散度（A-5：ref=min(8,实际平台数)，单平台零点平移为 0）
//
// 合成（A-6）：缺失因子的权重按比例重分给有效因子；全缺失 → ErrNoFactor
// （heat=null、不写 InfluxDB、发 TT_HEAT_NO_FACTOR）。
// ============================================================================

// ErrNoFactor 四因子全部缺失（A-6：heat=null，不写 InfluxDB）。
var ErrNoFactor = errors.New("TT_HEAT_NO_FACTOR: all heat factors missing")

// 因子名（HeatResult.Missing 取值集合，低基数可进指标 label 的调试字段）。
const (
	FactorRank    = "rank"
	FactorEng     = "eng"
	FactorFresh   = "fresh"
	FactorBreadth = "breadth"
)

// HeatWeights heat 四权重（Σ=1）。默认详设值 {0.35, 0.25, 0.20, 0.20}。
type HeatWeights struct {
	WRank    float64 `json:"w_rank"`
	WEng     float64 `json:"w_eng"`
	WFresh   float64 `json:"w_fresh"`
	WBreadth float64 `json:"w_breadth"`
}

// DefaultHeatWeights 详设 §2.1 默认权重（WeightConfig 表无行时的兜底盘）。
func DefaultHeatWeights() HeatWeights {
	return HeatWeights{WRank: 0.35, WEng: 0.25, WFresh: 0.20, WBreadth: 0.20}
}

// weightSumTol 权重和容差（浮点录入误差；01 式明确要求「和为 1 校验」）。
const weightSumTol = 1e-6

// Validate Σ=1 校验（权重配置 API 的准入闸；各分量须落在 [0,1]）。
func (w HeatWeights) Validate() error {
	for name, v := range map[string]float64{
		"w_rank": w.WRank, "w_eng": w.WEng, "w_fresh": w.WFresh, "w_breadth": w.WBreadth,
	} {
		if v < 0 || v > 1 {
			return fmt.Errorf("%s 须在 [0,1]，got %v", name, v)
		}
	}
	sum := w.WRank + w.WEng + w.WFresh + w.WBreadth
	if math.Abs(sum-1) > weightSumTol {
		return fmt.Errorf("权重和必须为 1（±%g），got %v", weightSumTol, sum)
	}
	return nil
}

// HeatResult 归一化结果（同时保留原始权重与本次生效权重，A-6 ⑦ 审计双口径）。
// 契约输出对应 mediacontract.HeatFactors + Heat 块。
type HeatResult struct {
	Score     float64 // 0–100
	RankPct   float64
	EngPct    float64
	Freshness float64
	Breadth   float64

	Weights    HeatWeights // 原始配置权重
	WeightsEff HeatWeights // 本次实际生效权重（缺失重分配后）
	Missing    []string    // 缺失因子名（rank/eng/fresh/breadth）
}

// RankPct 榜内相对位置（A-7 统一式：rank=1 → 1.0；rank=rank_total → 0.0）。
// valid=false 表示无榜/榜长未知（rank 或 rank_total ≤ 0），权重按 A-6 重分配。
func RankPct(rank, rankTotal int32) (pct float64, valid bool) {
	if rank <= 0 || rankTotal <= 0 {
		return 0, false
	}
	pct = float64(rankTotal-rank) / math.Max(float64(rankTotal-1), 1)
	return clamp01(pct), true
}

// Freshness 新鲜度（A-4 主口径，进 heat 公式）：
// freshness(t) = max( exp(-(t - first_seen_at)/τ), f_floor )，t 为样本时间。
// f_floor=0.05 保证二次爆发话题不被地板压死（S-3：24h 后 peak 仍可达）。
// Δt 若因迟到修正出现负值（first_seen 晚于样本，TouchSeen 已防）按 0 钳制。
func Freshness(t, firstSeenAt time.Time, tauHours, floor float64) float64 {
	if tauHours <= 0 {
		tauHours = 12
	}
	dt := t.Sub(firstSeenAt).Hours()
	if dt < 0 {
		dt = 0
	}
	f := math.Exp(-dt / tauHours)
	if f < floor {
		return floor
	}
	return clamp01(f)
}

// Breadth 跨平台扩散度（A-5 零点平移式）：
// ref = min(8, P)，pc = 近 24h distinct 平台数；
// 单平台 → 0（修正 ln2/ln9≈0.3155 的白送分）；pc ≥ ref → 1。
// supportedPlatforms 为 IC 实际已接入平台数；≤0 时按 ref=8 兼容旧口径。
func Breadth(platformCount, supportedPlatforms int) float64 {
	ref := 8
	if supportedPlatforms > 0 && supportedPlatforms < ref {
		ref = supportedPlatforms
	}
	if platformCount < 1 {
		platformCount = 1
	}
	if platformCount >= ref {
		return 1
	}
	raw := math.Log1p(float64(platformCount)) / math.Log1p(float64(ref))
	b0 := math.Ln2 / math.Log1p(float64(ref)) // 单平台基线（ref=8 时 0.3155）
	return clamp01((raw - b0) / (1 - b0))
}

// CombineHeat 热度合成 + 缺失权重重分配（A-6 显式公式）。
// 入参 validXxx 标记各因子是否有效（内部哨兵 ≤0 视同 null，由调用方判定后传入）。
// 恒等不变量（CI 断言）：factorValid 时 Σ w_eff ∈ {1±1e-9}；全缺失 → ErrNoFactor。
func CombineHeat(w HeatWeights, rankPct, engPct, freshness, breadth float64,
	rankValid, engValid, freshValid, breadthValid bool) (HeatResult, error) {

	res := HeatResult{
		RankPct: rankPct, EngPct: engPct, Freshness: freshness, Breadth: breadth,
		Weights: w,
	}

	type factor struct {
		name  string
		w     float64
		v     float64
		valid bool
	}
	factors := []factor{
		{FactorRank, w.WRank, rankPct, rankValid},
		{FactorEng, w.WEng, engPct, engValid},
		{FactorFresh, w.WFresh, freshness, freshValid},
		{FactorBreadth, w.WBreadth, breadth, breadthValid},
	}

	var s float64 // 有效权重之和 S
	for _, f := range factors {
		if f.valid {
			s += f.w
		} else {
			res.Missing = append(res.Missing, f.name)
		}
	}
	if s <= 0 {
		// A-6：heat=null、stage 强制 latent、missing 全量、不写 InfluxDB。
		res.Missing = []string{FactorRank, FactorEng, FactorFresh, FactorBreadth}
		return res, ErrNoFactor
	}

	var acc float64
	eff := make(map[string]float64, 4)
	for _, f := range factors {
		if !f.valid {
			continue
		}
		wEff := f.w / s
		eff[f.name] = wEff
		acc += wEff * f.v
	}
	res.WeightsEff = HeatWeights{
		WRank: eff[FactorRank], WEng: eff[FactorEng],
		WFresh: eff[FactorFresh], WBreadth: eff[FactorBreadth],
	}
	res.Score = clamp01(acc) * 100
	return res, nil
}

// clamp01 [0,1] 钳制。
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ============================================================================
// eng_pct 静态基线（A-3 冷启动/降级兜底：N<200 或 Redis 不可用时使用）。
//
// 随代码发布、季度标定（用 IC 真实数据离线重算后更新）。value 为 log1p(互动数)
// 的 11 档分位锚点（p0..p100 步进 10）；stance 未知平台共用 defaultBaseline。
// 当前为占位保守值：互动 0 → p0.v=0；log1p(1000)≈6.9 ≈ p90；log1p(1e5)≈11.5 = p100。
// ============================================================================

// EngBaseline 平台互动基线（log1p(互动数) 的 11 档分位锚点，升序）。
type EngBaseline [11]float64

// defaultEngBaseline 全平台兜底基线（占位值，待 IC 数据季度标定）。
var defaultEngBaseline = EngBaseline{0, 0.7, 1.4, 2.3, 3.2, 4.1, 5.0, 5.8, 6.6, 6.9, 11.5}

// platformEngBaseline 平台特异基线（无条目 → defaultEngBaseline）。
var platformEngBaseline = map[string]EngBaseline{
	"weibo":    {0, 1.1, 2.2, 3.4, 4.6, 5.5, 6.4, 7.3, 8.2, 9.2, 12.0},
	"bilibili": {0, 0.7, 1.6, 2.6, 3.7, 4.7, 5.6, 6.5, 7.4, 8.4, 11.5},
}

// BaselineEngPct 静态基线下的 eng_pct（raw = log1p(互动数) 的线性插值分位）。
func BaselineEngPct(platform string, raw float64) float64 {
	b, ok := platformEngBaseline[platform]
	if !ok {
		b = defaultEngBaseline
	}
	if raw <= b[0] {
		return 0
	}
	if raw >= b[10] {
		return 1
	}
	for i := 1; i <= 10; i++ {
		if raw <= b[i] {
			frac := (raw - b[i-1]) / (b[i] - b[i-1])
			return clamp01((float64(i-1) + frac) / 10)
		}
	}
	return 1
}
