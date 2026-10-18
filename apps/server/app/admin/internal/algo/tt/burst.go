package tt

import (
	"math"
	"sort"

	"framework/pkg/mediacontract"
)

// 本文件实现突发检测，并已吸收评审的最严重修复（TT-深度评审 B-5 / A-9~A-13）：
//
//  【B-5 数学缺陷】原文档的 residual 与 velocity 是同一统计量的两个阈值（ρ=1），
//      「≥2 方法投票」实为单方法双阈值，投票机制失效。
//  【修复】通道化投票：heat 通道（zscore+velocity）合计最多 1 票，
//      必须再搭配 rank 通道（rank_jump/first_cross_platform）或
//      engagement 通道（eng_spike）的票才判突发。

// 突发检测阈值（默认值，可经 system_config 覆盖，key 前缀 tt.burst.*）。
const (
	DefaultEWMAPeriodHours = 0.75 // 时间感知 EWMA 的时间常数 T（评审 A-6）
	DefaultKZScore         = 3.0  // zscore 阈值
	DefaultSigmaMin        = 1.0  // σ 下限（单位：heat 分）
	DefaultZWindow         = 24   // zscore 残差窗口点数
	DefaultZMinWindow      = 8    // zscore 窗口下限（低于此不启用 zscore）
	DefaultVMinBurst       = 8.0  // velocity 突发阈值（分/小时，比 lifecycle 的 6.0 更严）
	DefaultHMinBurst       = 40.0 // velocity 突发的热度门槛
	DefaultEngSpikeMult    = 3.0  // 互动突增倍数（相对近 7 日同时段中位）
	DefaultEngSpikeAbs     = 1000 // 互动突增绝对值下限
)

// Point 是一个采样点（样本时间口径）。
type Point struct {
	T        int64   // 样本时间戳（秒，UTC）
	Heat     float64 // 该点热度（可重现口径）
	Rank     int32   // 榜单名次，无榜 = 0（评审 C-7：契约用 null，内部 0 视为缺失）
	RankTotal int32  // 榜长，未知 = 0
	Eng      int64   // 互动量（likes+comments+shares+favorites）
}

// BurstConfig 突发检测配置。
type BurstConfig struct {
	EWMAPeriodHours float64
	KZScore         float64
	SigmaMin        float64
	ZWindow         int
	ZMinWindow      int
	VMinBurst       float64
	HMinBurst       float64
	EngSpikeMult    float64
	EngSpikeAbs     int64
	// EngBaselineMedian 近 7 日同时段互动量中位数（由调用方提供，冷启动可传 0）。
	EngBaselineMedian float64
}

// DefaultBurstConfig 返回默认配置。
func DefaultBurstConfig() BurstConfig {
	return BurstConfig{
		EWMAPeriodHours: DefaultEWMAPeriodHours,
		KZScore:         DefaultKZScore,
		SigmaMin:        DefaultSigmaMin,
		ZWindow:         DefaultZWindow,
		ZMinWindow:      DefaultZMinWindow,
		VMinBurst:       DefaultVMinBurst,
		HMinBurst:       DefaultHMinBurst,
		EngSpikeMult:    DefaultEngSpikeMult,
		EngSpikeAbs:     DefaultEngSpikeAbs,
	}
}

// TimeAwareAlpha 计算时间感知 EWMA 的平滑系数（评审 A-6）。
// α = 1 - exp(-Δt_h / T)。采样间隔可配，固定 α 会导致 velocity 跨配置不可比。
func TimeAwareAlpha(dtSeconds int64, periodHours float64) float64 {
	if periodHours <= 0 {
		periodHours = DefaultEWMAPeriodHours
	}
	dtH := float64(dtSeconds) / 3600.0
	if dtH < 0 {
		dtH = 0
	}
	return 1 - math.Exp(-dtH/periodHours)
}

// EWMA 计算时间感知指数加权移动平均。
// prev 为上一点平滑值（首点传 NaN，则返回当前值）。
func EWMA(prev, x float64, dtSeconds int64, periodHours float64) float64 {
	if math.IsNaN(prev) {
		return x
	}
	a := TimeAwareAlpha(dtSeconds, periodHours)
	return a*x + (1-a)*prev
}

// median 返回中位数。
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return 0.5 * (s[n/2-1] + s[n/2])
}

// robustSigma 用 MAD（中位绝对偏差）估计稳健 σ（评审 A-9）。
// σ = max(1.4826 × MAD, sigma_min)。窗口**不含当前点**（避免掩蔽效应）。
func robustSigma(residuals []float64, sigmaMin float64) float64 {
	if len(residuals) == 0 {
		return sigmaMin
	}
	med := median(residuals)
	devs := make([]float64, len(residuals))
	for i, r := range residuals {
		devs[i] = math.Abs(r - med)
	}
	sigma := 1.4826 * median(devs)
	if sigma < sigmaMin {
		sigma = sigmaMin
	}
	return sigma
}

// BurstResult 是一次突发检测的结果。
type BurstResult struct {
	IsBurst    bool
	MethodsHit []mediacontract.BurstMethod
	Evidence   map[string]float64
	// ChannelsHit 命中的数据通道集合（评审 A-12）。
	ChannelsHit map[mediacontract.BurstChannel]bool
	// Skipped 表示因数据不足未检测（n<3），不计入误报率分母。
	Skipped bool
}

// DetectBurst 对单平台时序执行突发检测（评审 A-12 通道化投票 + A-13 冷启动分级）。
//
// points 必须按时间升序。只检测最后一个点（当前点）。
// 冷启动分级（评审 A-13）：
//   n < 3        → 不检测（Skipped=true，不计入分母）
//   3 ≤ n < 8    → 仅 velocity 与 rank_jump；zscore 记 unavailable
//   n ≥ 8        → 全方法启用
//
// 判据（评审 A-12）：|hit| ≥ 2 且命中方法覆盖 ≥2 个不同数据通道。
func DetectBurst(points []Point, cfg BurstConfig) BurstResult {
	res := BurstResult{Evidence: map[string]float64{}, ChannelsHit: map[mediacontract.BurstChannel]bool{}}
	n := len(points)
	if n < 3 {
		res.Skipped = true
		return res
	}
	cur := points[n-1]

	// 计算平滑序列 h_ewma（时间感知，评审 A-6）。
	h := make([]float64, n)
	for i, p := range points {
		if i == 0 {
			h[i] = p.Heat
			continue
		}
		h[i] = EWMA(h[i-1], p.Heat, p.T-points[i-1].T, cfg.EWMAPeriodHours)
	}
	// v_ewma：增速的平滑（分/小时）。
	vEWMA := math.NaN()
	if n >= 2 {
		dt := float64(cur.T-points[n-2].T) / 3600.0
		if dt <= 0 {
			dt = 0.5
		}
		vRaw := (h[n-1] - h[n-2]) / dt
		if n >= 3 {
			dtPrev := float64(points[n-2].T-points[n-3].T) / 3600.0
			if dtPrev <= 0 {
				dtPrev = 0.5
			}
			vPrev := (h[n-2] - h[n-3]) / dtPrev
			vEWMA = EWMA(vPrev, vRaw, cur.T-points[n-2].T, cfg.EWMAPeriodHours)
		} else {
			vEWMA = vRaw
		}
	}

	zscoreEnabled := n >= cfg.ZMinWindow+1

	// ---- 方法一：Z-score 残差（heat 通道，评审 A-9）----
	if zscoreEnabled {
		// 残差基线只含历史（h_ewma(t-1)），窗口不含当前点。
		w := cfg.ZWindow
		if w > n-1 {
			w = n - 1
		}
		residuals := make([]float64, 0, w)
		start := n - 1 - w
		for i := start; i < n-1; i++ {
			if i < 1 {
				continue
			}
			residuals = append(residuals, points[i].Heat-h[i-1])
		}
		if len(residuals) >= cfg.ZMinWindow {
			sigma := robustSigma(residuals, cfg.SigmaMin)
			r := cur.Heat - h[n-2] // 单尾（上尾）残差
			z := r / sigma
			res.Evidence["zscore"] = z
			if z > cfg.KZScore {
				res.MethodsHit = append(res.MethodsHit, mediacontract.BurstZScore)
			}
		}
	}

	// ---- 方法二：双阈值增速（heat 通道，评审 A-10）----
	if !math.IsNaN(vEWMA) {
		res.Evidence["velocity"] = vEWMA
		res.Evidence["h_ewma"] = h[n-1]
		if vEWMA > cfg.VMinBurst && h[n-1] > cfg.HMinBurst {
			res.MethodsHit = append(res.MethodsHit, mediacontract.BurstVelocity)
		}
	}

	// ---- 方法三：榜单跃迁（rank 通道，评审 A-11）----
	if cur.Rank > 0 && cur.RankTotal > 0 {
		prev := points[n-2]
		thr := math.Max(3, math.Ceil(0.2*float64(cur.RankTotal)))
		// 首次进榜：本轮在榜、上轮不在榜。
		if cur.Rank > 0 && prev.Rank <= 0 && cur.Heat > 20 {
			res.MethodsHit = append(res.MethodsHit, mediacontract.BurstRankJump)
			res.Evidence["rank_jump"] = float64(cur.RankTotal - cur.Rank)
		} else if prev.Rank > 0 && cur.Rank > 0 && prev.RankTotal == cur.RankTotal {
			jump := float64(prev.Rank - cur.Rank)
			res.Evidence["rank_jump"] = jump
			res.Evidence["rank_thr"] = thr
			if jump >= thr {
				res.MethodsHit = append(res.MethodsHit, mediacontract.BurstRankJump)
			}
		}
	}
	// rank 缺失时该方法记 unavailable（不投票、不计入分母）——评审 A-11。

	// ---- 方法四：互动突增（engagement 通道，评审 A-12 新增，修复相关性与冷启动）----
	if cfg.EngBaselineMedian > 0 {
		ratio := float64(cur.Eng) / cfg.EngBaselineMedian
		res.Evidence["eng_spike_ratio"] = ratio
		if float64(cur.Eng) > cfg.EngSpikeMult*cfg.EngBaselineMedian &&
			cur.Eng > cfg.EngSpikeAbs {
			res.MethodsHit = append(res.MethodsHit, mediacontract.BurstEngSpike)
		}
	}

	// ---- 通道化投票（评审 A-12）：≥2 票且覆盖 ≥2 个不同通道 ----
	for _, m := range res.MethodsHit {
		res.ChannelsHit[m.Channel()] = true
	}
	res.IsBurst = len(res.MethodsHit) >= 2 && len(res.ChannelsHit) >= 2
	return res
}
