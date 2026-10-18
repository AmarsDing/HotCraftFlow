package tt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// ============================================================================
// HeatEnricher（M2 归一化编排器，03 §2.1 + 评审 A-2..A-7 / A-15 / S-7）。
//
// 每点流水线（ingest 在 last-wins 守卫通过后调用，发布前把因子/热度/动态
// 全部回填进 SnapshotPoint——B-3/C-15：因子取值与生效权重随点落 InfluxDB，
// Topic 行冗余最新值，NFR-TT-05 硬门禁）：
//
//  1. TopicHeatMeta（样本时间窗口，B-4 可重现）
//  2. 权重：样本时刻生效的 WeightConfig 行（A-15，内存缓存最新行）
//  3. rank_pct（A-7）/ eng_pct（A-2+A-3 直方图，N<200 或 Redis 故障→静态基线，
//     且**不标 missing**：口径不得随基础设施状态跳变）
//  4. freshness（A-4 样本时间 + 地板）/ breadth（A-5 零点平移，pc 按样本窗口）
//  5. CombineHeat（A-6 缺失重分配；全缺失 → ErrNoFactor，调用方跳过发布）
//  6. DynamicsStateStore 推进 EWMA（S-7 时间感知 α；A-15 epoch 变更 α=1 重置；
//     乱序点 Advanced=false 记 0）
// ============================================================================

// DefaultWeightEpoch 无配置行时的 epoch 占位（审计可辨「未配置过权重」时段）。
const DefaultWeightEpoch = "default"

// weightCacheTTL 最新权重行的内存缓存时长（权重行 append-only，TTL 只做散热）。
const weightCacheTTL = 60 * time.Second

// HeatEnricher 归一化编排器。
type HeatEnricher struct {
	conf    *Conf
	repo    TopicRepo
	hist    EngHistogram
	dyn     DynamicsStateStore
	weights WeightConfigRepo

	mu          sync.Mutex
	cachedHeat  *WeightConfigRow
	cacheExpiry time.Time
	clock       func() time.Time // 仅用于缓存过期与兜底，禁止进因子计算（B-4）

	log *log.Helper
}

// NewHeatEnricher 构造。
func NewHeatEnricher(
	conf *Conf,
	repo TopicRepo,
	hist EngHistogram,
	dyn DynamicsStateStore,
	weights WeightConfigRepo,
	logger log.Logger,
) *HeatEnricher {
	return &HeatEnricher{
		conf:    conf,
		repo:    repo,
		hist:    hist,
		dyn:     dyn,
		weights: weights,
		clock:   time.Now,
		log:     log.NewHelper(log.With(logger, "module", "biz/tt-heat")),
	}
}

// WeightEpochOf A-15 ②：weight_epoch = sha256(config_id)[:8]。
func WeightEpochOf(configID string) string {
	sum := sha256.Sum256([]byte(configID))
	return hex.EncodeToString(sum[:])[:8]
}

// heatWeightsAt 取样本时刻 t 生效的 heat 权重（缓存最新行；迟到点回退直查）。
func (e *HeatEnricher) heatWeightsAt(ctx context.Context, t time.Time) (HeatWeights, string) {
	e.mu.Lock()
	cached := e.cachedHeat
	expiry := e.cacheExpiry
	e.mu.Unlock()

	if cached != nil && !e.clock().After(expiry) && !t.Before(cached.EffectiveFrom) {
		return cached.Weights, WeightEpochOf(cached.ConfigID)
	}

	row, found, err := e.weights.ActiveHeatWeights(ctx, t)
	if err != nil {
		e.log.Warnf("load weight config failed (use defaults): %v", err)
		return DefaultHeatWeights(), DefaultWeightEpoch
	}
	if !found {
		return DefaultHeatWeights(), DefaultWeightEpoch
	}

	// 仅缓存「最新行」（有效窗口探到未来的行即当前行）。
	e.mu.Lock()
	e.cachedHeat = &row
	e.cacheExpiry = e.clock().Add(weightCacheTTL)
	e.mu.Unlock()
	return row.Weights, WeightEpochOf(row.ConfigID)
}

// Enrich 归一化并回填快照点（point 原地修改）。返回话题元数据供调用方回写 Topic 行。
// 返回 ErrNoFactor：四因子全缺失（A-6：不写 InfluxDB），调用方须跳过发布并计数。
func (e *HeatEnricher) Enrich(ctx context.Context, point *SnapshotPoint) (TopicHeatMeta, error) {
	t := point.TRaw

	// 1) 话题元数据（PG 为主存，取数失败 → error → NAK 重试，不降级口径）。
	meta, err := e.repo.GetTopicHeatMeta(ctx, point.TopicID, t)
	if err != nil {
		return meta, err
	}

	// 2) 权重（样本时刻生效口径）。
	weights, epoch := e.heatWeightsAt(ctx, t)

	// 3) 四因子。
	rankPct, rankValid := RankPct(point.Rank, point.RankTotal)

	engPct := e.engPercentile(ctx, point, meta) // A-3：恒有效，不标 missing
	fresh := Freshness(t, meta.FirstSeenAt, e.conf.Heat.FreshnessTauHours, e.conf.Heat.FreshnessFloor)
	freshValid := !meta.FirstSeenAt.IsZero()
	breadth := Breadth(meta.PlatformCount24h, e.conf.Heat.SupportedPlatforms)

	// 4) 合成 + 重分配。
	res, err := CombineHeat(weights,
		rankPct, engPct, fresh, breadth,
		rankValid, true, freshValid, true)
	if err != nil {
		HeatNoFactorTotal.Inc()
		return meta, err
	}

	// 5) 动态链推进（S-7/A-15）。
	velocity, accel := e.advanceDynamics(ctx, point, res.Score, epoch)

	// 6) 回填（B-3：因子与生效权重随点落盘）。
	point.HeatScore = res.Score
	point.Velocity = velocity
	point.Accel = accel
	point.RankPct = res.RankPct
	point.EngPct = res.EngPct
	point.Freshness = res.Freshness
	point.Breadth = res.Breadth
	point.WRankEff = res.WeightsEff.WRank
	point.WEngEff = res.WeightsEff.WEng
	point.WFreshEff = res.WeightsEff.WFresh
	point.WBreadthEff = res.WeightsEff.WBreadth
	point.Missing = res.Missing
	point.WeightEpoch = epoch
	return meta, nil
}

// engPercentile A-2/A-3：raw = log1p(likes+comments)，窗口 (t-7d, t] ECDF。
// 冷启动（N<EngMinWindowN）与基础设施故障统一退化静态基线，不标 missing。
func (e *HeatEnricher) engPercentile(ctx context.Context, point *SnapshotPoint, meta TopicHeatMeta) float64 {
	raw := math.Log1p(float64(point.Likes + point.Comments))
	if e.hist == nil {
		EngBaselineFallbackTotal.WithLabelValues("no_store").Inc()
		return BaselineEngPct(point.Platform, raw)
	}
	pct, n, err := e.hist.ObserveAndPercentile(ctx, point.Platform, meta.Category, point.TRaw,
		float64(point.Likes+point.Comments))
	if err != nil {
		e.log.Warnf("eng histogram failed (baseline fallback): %v", err)
		EngBaselineFallbackTotal.WithLabelValues("store_error").Inc()
		return BaselineEngPct(point.Platform, raw)
	}
	if n < e.conf.Heat.EngMinWindowN {
		EngBaselineFallbackTotal.WithLabelValues("cold_start").Inc()
		return BaselineEngPct(point.Platform, raw)
	}
	return pct
}

// advanceDynamics 加载 → 推进 → 条件落盘。存储故障降级为「本点无状态计算」
// （velocity/accel 记 0，不阻断主链路；下个点自愈）。
func (e *HeatEnricher) advanceDynamics(ctx context.Context, point *SnapshotPoint, heat float64, epoch string) (float64, float64) {
	if e.dyn == nil {
		return 0, 0
	}
	prev, err := e.dyn.Load(ctx, point.TopicID, point.Platform)
	if err != nil {
		e.log.Warnf("dynamics load failed (treat as reset): %v", err)
		prev = nil
	}
	var expectedLastT time.Time
	if prev != nil {
		expectedLastT = prev.LastT
	}

	step := AdvanceDynamics(prev, heat, point.TBucket, epoch, e.conf.Heat.Dynamics)
	if step.Reset {
		reason := "first"
		if prev != nil {
			reason = "epoch_change"
		}
		DynamicsResetTotal.WithLabelValues(reason).Inc()
	}
	if !step.Advanced {
		return 0, 0 // 乱序点：状态不动，待 recompute 修正（A-14/S-23）
	}
	saved, err := e.dyn.SaveIfNotAdvanced(ctx, point.TopicID, point.Platform, step.Next, expectedLastT)
	if err != nil || !saved {
		e.log.Warnf("dynamics save race (topic=%s platform=%s saved=%v err=%v)",
			point.TopicID, point.Platform, saved, err)
	}
	return step.Velocity, step.Accel
}
