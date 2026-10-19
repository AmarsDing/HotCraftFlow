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

	// GetFirstSeenAt 取主题首见时间（A-8 AgeDays 锚点；data 层读 tt_topic.first_seen_at）。
	// 未找到时返回零值 + nil（组装侧退化为 AgeDays≈0，保守不迁老话题分支）。
	GetFirstSeenAt(ctx context.Context, topicID string) (time.Time, error)
}

// LifecycleTopicRow 活跃话题元数据行。
type LifecycleTopicRow struct {
	TopicID             string    // tp_ 前缀
	Stage               Stage     // 当前阶段
	StageSince          time.Time // 进入当前阶段的样本时间（UTC）
	StageDurationPoints int       // 当前阶段已驻留点数（B-7 口径）
	// FirstSeenAt 主题首见时间（UTC；A-8 话题年龄口径 AgeDays=样本时间−first_seen_at
	// 的锚点来源）。零值表示数据缺失——组装侧退化为最新样本时间（AgeDays≈0），
	// 不致把老话题误判为新话题（错误方向是错杀：宁可不迁 long_tail 分支 B）。
	FirstSeenAt time.Time
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
// quant 为 sweep 级共享分位（M1 修复：一轮 sweep 只算一次 P25/P99 注入各话题，
// 不再每话题全量扫 24h 聚合）；quant 为 nil 时按需现场计算（单话题补评/测试路径）。
func (e *LifecycleEvaluator) EvaluateOne(ctx context.Context, row LifecycleTopicRow, now time.Time, quant *HeatQuantiles) (bool, error) {
	// 1. 读最近 6 点推导 streak（rising 需 2 点、decaying 3 点，6 点冗余安全）。
	points, err := e.ts.ReadRecentPoints(ctx, row.TopicID, "", 6)
	if err != nil {
		return false, err
	}
	if len(points) == 0 {
		return false, nil // 无数据点：保持现状
	}

	// 2. 推导 streak（连续 v>v_min 或 v<0 的点数；points 按 t_raw 降序）。
	// v=0 是中断值（M1 未回填/中断语义），不计入任何 streak。
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

	// 3. 分位：注入优先；未注入则现场算（保持单话题调用自洽）。
	var heatP25, heatP99 float64
	if quant != nil {
		heatP25, heatP99 = quant.P25, quant.P99
	} else {
		heats, err := e.ts.ReadActiveHeatRange(ctx, now.Add(-24*time.Hour), now)
		if err != nil {
			return false, err
		}
		if len(heats) >= 4 { // 最少 4 点才能算有意义的分位
			sort.Float64s(heats)
			heatP25 = percentile(heats, 0.25)
			heatP99 = percentile(heats, 0.99)
		}
	}

	// 4. 话题年龄锚点（H4）：A-8 口径 AgeDays = 最新样本时间 − first_seen_at。
	// first_seen 缺失时退化为 0（新话题），保守不错杀老话题为 long_tail。
	latest := points[0] // 最新点
	firstSeen := row.FirstSeenAt
	if firstSeen.IsZero() {
		if fs, ferr := e.repo.GetFirstSeenAt(ctx, row.TopicID); ferr == nil && !fs.IsZero() {
			firstSeen = fs
		} else {
			firstSeen = latest.TRaw // 退化：年龄≈0
		}
	}
	ageDays := latest.TRaw.Sub(firstSeen).Hours() / 24
	if ageDays < 0 {
		ageDays = 0 // first_seen 晚于最新样本（脏数据）按 0 处理，不出负年龄
	}

	// 5. 组装 StageInput。
	in := StageInput{
		Current:      row.Stage,
		DwellPoints:  row.StageDurationPoints,
		NPoints:      len(points), // 近似：近 6 点（latent 判据 n<3 用）
		Heat:         latest.HeatScore,
		V:            latest.Velocity,
		AgeDays:      ageDays,
		Updated24h:   now.Sub(latest.TRaw) < 24*time.Hour,
		NoUpdate48h:  now.Sub(latest.TRaw) >= 48*time.Hour,
		HeatP25:      heatP25,
		HeatP99:      heatP99,
		RisingStreak: risingStreak,
		DecayStreak:  decayStreak,
	}
	// H5 修复：热度哨兵守卫——HeatScore=SentinelHeat(-1) 是 M1 未回填的「未计算」
	// 标记，绝非实测低热/高热。放行会让规则 1（heat<20）把所有未回填话题误判
	// long_tail，或让规则 2（heat≥80 绝对线 / heat≥P99 相对线）误判 peak。热度是
	// 阶段判据的硬输入，未知时任何阶段推断都是臆测——唯一安全方向是短路：保持
	// 现状，等 M1 回填后重估（不错杀也不捧杀）。注：哨兵只查最新点——历史点的
	// heat 哨兵不影响 streak（streak 只看 Velocity），回填后重估自然纠正。
	if latest.HeatScore == SentinelHeat {
		return false, nil
	}

	// 6. 调 EvalStage。
	result := EvalStage(in, e.conf)
	if !result.Changed {
		return false, nil
	}

	// 7. 落库变更。
	changed, err := e.repo.UpdateStage(ctx, row.TopicID, StageUpdate{
		Stage:               result.Stage,
		StageSince:          latest.TRaw,
		StageDurationPoints: 0,
		Evidence:            buildStageEvidence(result, in),
	})
	return changed, err
}

// HeatQuantiles sweep 级共享热度分位（A-8 规则 1/2 相对线；M1 一轮一算）。
type HeatQuantiles struct {
	P25 float64
	P99 float64
}

// ComputeQuantiles 一轮 sweep 的分位计算入口（worker 调用一次，注入各 EvaluateOne）。
// 数据不足 4 点时返回零值分位（相对线禁用，绝对线仍生效——A-8 冷启动保护）。
func (e *LifecycleEvaluator) ComputeQuantiles(ctx context.Context, now time.Time) (*HeatQuantiles, error) {
	heats, err := e.ts.ReadActiveHeatRange(ctx, now.Add(-24*time.Hour), now)
	if err != nil {
		return nil, err
	}
	q := &HeatQuantiles{}
	if len(heats) >= 4 {
		sort.Float64s(heats)
		q.P25 = percentile(heats, 0.25)
		q.P99 = percentile(heats, 0.99)
	}
	return q, nil
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
