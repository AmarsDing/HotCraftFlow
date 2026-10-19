package tt

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// ============================================================================
// 生命周期评估编排（A-8：streak/P25/P99 由时序查询推导，禁累加器，重启幂等）。
//
// 调用方（worker）每 15min 扫活跃话题（stage≠dormant 且 merge_lock=false），
// 对每个话题组装 StageInput 调 EvalStage，变更则落库（stage/stage_since/
// stage_duration_points + stage_evidence）。
//
// 侧壁：
//  - 本层只做「组装输入 → 调 EvalStage → 组装输出」；streak 推导与分位计算
//    是纯函数，InfluxDB/PG 查询在端口后；
//  - stage_duration_points 由 stage_since 经查询推导（B-7 点数口径），
//    不存累加器（重启幂等：读 stage_since 后数点数即可）；
//  - 并发安全：同一话题的评估由调用方串行化（按 topic_id 分片或单协程）。
// ============================================================================

// LifecycleRepo 生命周期评估的持久化端口（data/tt 实现）。
type LifecycleRepo interface {
	// ListActiveTopics 活跃话题列表（stage≠dormant 且 merge_lock=false 且未归档）。
	// 返回 (topic_id, stage, stage_since, stage_duration_points) 元组。
	ListActiveTopics(ctx context.Context, limit int) ([]LifecycleTopicRow, error)

	// UpdateStage 落库阶段变更：stage/stage_since/stage_duration_points/
	// stage_evidence（变更证据 JSON：{rule, from, to, at, heat, v}）。
	// 返回是否实际变更（并发抢先时 false——乐观并发控制）。
	UpdateStage(ctx context.Context, topicID string, in StageUpdate) (changed bool, err error)
}

// LifecycleTopicRow 活跃话题元数据行。
type LifecycleTopicRow struct {
	TopicID             string    // tp_ 前缀
	Stage               Stage     // 当前阶段
	StageSince          time.Time // 进入当前阶段的样本时间（UTC）
	StageDurationPoints int       // 当前阶段已驻留点数（B-7 口径）
}

// StageUpdate 阶段变更落库载荷。
type StageUpdate struct {
	Stage               Stage
	StageSince          time.Time // 变更时刻的样本时间（UTC）
	StageDurationPoints int       // 新阶段起始点数（0）
	Evidence            string    // JSON：{rule, from, to, at, heat, v, streak}
}

// LifecycleEvaluator 生命周期评估编排器。
type LifecycleEvaluator struct {
	repo LifecycleRepo
	ts   TimeSeries
	conf LifecycleConf
}

// NewLifecycleEvaluator 组装。
func NewLifecycleEvaluator(repo LifecycleRepo, ts TimeSeries, conf LifecycleConf) (*LifecycleEvaluator, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return &LifecycleEvaluator{repo: repo, ts: ts, conf: conf}, nil
}

// ListActiveTopics 透传扫描一批活跃话题（worker 每轮入口；limit≤0 由
// data 层取默认批次）。挂在评估器上而非让 worker 直持 repo：
// worker 只见评估抽象，端口不外泄。
func (e *LifecycleEvaluator) ListActiveTopics(ctx context.Context, limit int) ([]LifecycleTopicRow, error) {
	return e.repo.ListActiveTopics(ctx, limit)
}

// EvaluateOne 评估单个话题。返回是否变更（供 metrics/日志）。
// now 为评估时刻（UTC）；样本时间口径（B-4：历史可重现）。
func (e *LifecycleEvaluator) EvaluateOne(ctx context.Context, row LifecycleTopicRow, now time.Time) (bool, error) {
	// 1. 读最近 6 点推导 streak（rising 需 2 点、decaying 3 点，6 点冗余安全）。
	points, err := e.ts.ReadRecentPoints(ctx, row.TopicID, "", 6)
	if err != nil {
		return false, err
	}
	if len(points) == 0 {
		return false, nil // 无数据点：保持现状
	}

	// 2. 推导 streak（连续 v>v_min 或 v<0 的点数；points 按 t_raw 降序）。
	risingStreak, decayStreak := 0, 0
	for _, p := range points {
		if p.Velocity > e.conf.VMin {
			if decayStreak > 0 {
				break // 连续中断
			}
			risingStreak++
		} else if p.Velocity < 0 {
			if risingStreak > 0 {
				break
			}
			decayStreak++
		} else {
			break // v=0 中断连续
		}
	}

	// 3. 读近 24h 全体活跃话题 heat 序列求 P25/P99。
	heats, err := e.ts.ReadActiveHeatRange(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		return false, err
	}
	var heatP25, heatP99 float64
	if len(heats) >= 4 { // 最少 4 点才能算有意义的分位
		sort.Float64s(heats)
		heatP25 = percentile(heats, 0.25)
		heatP99 = percentile(heats, 0.99)
	}

	// 4. 组装 StageInput。
	latest := points[0] // 最新点
	in := StageInput{
		Current:      row.Stage,
		DwellPoints:  row.StageDurationPoints,
		NPoints:      len(points), // 近似：近 6 点（latent 判据 n<3 用）
		Heat:         latest.HeatScore,
		V:            latest.Velocity,
		AgeDays:      now.Sub(latest.TRaw).Hours() / 24,
		Updated24h:   len(points) > 0 && now.Sub(latest.TRaw) < 24*time.Hour,
		NoUpdate48h:  len(points) == 0 || now.Sub(latest.TRaw) >= 48*time.Hour,
		HeatP25:      heatP25,
		HeatP99:      heatP99,
		RisingStreak: risingStreak,
		DecayStreak:  decayStreak,
	}

	// 5. 调 EvalStage。
	result := EvalStage(in, e.conf)
	if !result.Changed {
		return false, nil
	}

	// 6. 落库变更。
	changed, err := e.repo.UpdateStage(ctx, row.TopicID, StageUpdate{
		Stage:               result.Stage,
		StageSince:          latest.TRaw,
		StageDurationPoints: 0,
		Evidence:            buildStageEvidence(result, in),
	})
	return changed, err
}

// percentile 线性插值分位（sorted 升序；p∈[0,1]）。
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := p * float64(len(sorted)-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= len(sorted) {
		return sorted[len(sorted)-1]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// buildStageEvidence 组装变更证据 JSON（A-8：stage_evidence 落库可追溯）。
func buildStageEvidence(r StageResult, in StageInput) string {
	return `{"rule":` + strconv.Itoa(r.Rule) +
		`,"from":"` + string(in.Current) + `"` +
		`,"to":"` + string(r.Stage) + `"` +
		`,"heat":` + strconv.FormatFloat(in.Heat, 'f', 2, 64) +
		`,"v":` + strconv.FormatFloat(in.V, 'f', 2, 64) +
		`,"rising_streak":` + strconv.Itoa(in.RisingStreak) +
		`,"decay_streak":` + strconv.Itoa(in.DecayStreak) + `}`
}
