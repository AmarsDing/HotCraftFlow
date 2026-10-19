package tt

// ============================================================================
// 生命周期阶段评估（评审 A-8 有序判据 + B-6 状态空间全覆盖 + B-7 点数口径）
//
// 与 03-详细设计 §2.2 判据表的差异（以评审终稿为准）：
//  - 判据表改为有序列表 0–6，首个命中即目标阶段；default 分支保持当前阶段，
//    任何输入都不会落入「无匹配」空转（B-6：heat=50, v=+2, n=5 曾是黑洞）；
//  - latent 不再有 heat<30 ∧ platform_count≤2 条件（A-8 规则 0 仅 n_points<3，强制）；
//  - rising 判据不含 heat 条件（M-3 统一，二次爆发 decaying→rising 同一路径）；
//  - 新增 dormant 兜底态（近 48h 无更新）；
//  - min_dwell 以「当前阶段驻留样本点数」计数（B-7），驻留不足时保持原阶段
//    （Held），不随墙钟/采样间隔漂移。
//
// 持续类判据（rising 持续≥2 点、decaying 持续≥3 点）的连击数由调用方从
// 时序查询推导后作为输入传入（A-8：禁累加器，重启可幂等恢复）。
// ============================================================================

// Stage 生命周期阶段（与 ent schema tt_topic.stage 枚举一一对应，顺序无关）。
type Stage string

const (
	StageLatent   Stage = "latent"
	StageRising   Stage = "rising"
	StagePeak     Stage = "peak"
	StageDecaying Stage = "decaying"
	StageLongTail Stage = "long_tail"
	StageDormant  Stage = "dormant"
)

// StageMinDwell 各阶段最小驻留点数（A-8/B-7；切换离开该阶段前必须驻满）。
var StageMinDwell = map[Stage]int{
	StageLatent:   0, // 允许快速转出
	StageRising:   2,
	StagePeak:     1,
	StageDecaying: 3,
	StageLongTail: 6,
	StageDormant:  0,
}

// LifecycleConf 阶段判定阈值（03 §2.2 判据表默认 + A-8 终稿）。
type LifecycleConf struct {
	// VMin rising 判据的 v_ewma 下限（分/小时；A-10 burst 级 8.0 更严另有其用）。
	VMin float64
	// HeatPeakMin 绝对热度 peak 线（heat ≥ 80 → peak）。
	HeatPeakMin float64
	// HeatTurnMin velocity 由正转分支的 heat 下限（v<0 ∧ heat≥60 ∧ 上阶段∈{rising,peak} → peak）。
	HeatTurnMin float64
	// LongTailHeatMax 长尾热度上限（heat < 20 → long_tail 分支 A）。
	LongTailHeatMax float64
	// LongTailAgeDays 长尾分支 B 的话题年龄下限（>7d）。
	LongTailAgeDays float64
	// DormantHours 无更新进入 dormant 的墙钟阈值（48h）。
	DormantHours float64
	// RisingMinStreak rising 持续点数（A-8 规则 3：≥2）。
	RisingMinStreak int
	// DecayMinStreak decaying 持续点数（A-8 规则 4：≥3）。
	DecayMinStreak int
}

// DefaultLifecycleConf 评审终稿默认值。
func DefaultLifecycleConf() LifecycleConf {
	return LifecycleConf{
		VMin:            6.0,
		HeatPeakMin:     80,
		HeatTurnMin:     60,
		LongTailHeatMax: 20,
		LongTailAgeDays: 7,
		DormantHours:    48,
		RisingMinStreak: 2,
		DecayMinStreak:  3,
	}
}

// StageInput 一次评估的事实输入（全部「由样本时间推导的事实」，禁止 now()）。
type StageInput struct {
	// Current 当前阶段（Topic.stage）。
	Current Stage
	// DwellPoints 当前阶段已驻留的样本点数（stage_duration_points 口径，B-7）。
	DwellPoints int
	// NPoints 话题累计样本点数（迟滞成长期的强制 latent 判据）。
	NPoints int
	// Heat 本点热度（0-100）。
	Heat float64
	// V 本点 v_ewma（分/小时；M-2 统一平滑口径）。
	V float64
	// AgeDays 话题年龄（样本时间 - first_seen_at，天）。
	AgeDays float64
	// Updated24h 近 24h 仍有更新（long_tail 分支 B 条件；新点到来的前置语境）。
	Updated24h bool
	// NoUpdate48h 在本点之前已连续 ≥48h 无更新（dormant 判据）。
	NoUpdate48h bool
	// HeatP25 / HeatP99 近 24h 全体活跃话题 heat 分位（A-8 规则 1/2 的相对线）。
	HeatP25 float64
	HeatP99 float64
	// RisingStreak 连续 v_ewma > VMin 的点数（含本点；调用方由时序查询推导）。
	RisingStreak int
	// DecayStreak 连续 v_ewma < 0 的点数（含本点）。
	DecayStreak int
}

// StageResult 一次评估结果。
type StageResult struct {
	// Stage 评估后的阶段（规则未命中或被驻留闸拦下时 = Current）。
	Stage Stage
	// Rule 命中的有序规则序号（0–5）；-1 表示无任何规则命中（default 保持）。
	Rule int
	// Changed 是否发生阶段切换。
	Changed bool
	// Held 规则命中但被最小驻留闸拦下（防抖；观测/告警依据，不算切换）。
	Held bool
}

// EvalStage 有序求值（A-8：第一个命中为目标阶段；随后过最小驻留闸）。
func EvalStage(in StageInput, c LifecycleConf) StageResult {
	cand, rule := matchStageRule(in, c)
	if rule < 0 || cand == in.Current {
		// 无命中（default）或命中即现状：保持。
		return StageResult{Stage: in.Current, Rule: rule, Changed: false}
	}
	// 最小驻留闸（B-7 点数口径）：当前阶段驻留不足则保持（防抖）。
	if in.DwellPoints < StageMinDwell[in.Current] {
		return StageResult{Stage: in.Current, Rule: rule, Changed: false, Held: true}
	}
	return StageResult{Stage: cand, Rule: rule, Changed: true}
}

// matchStageRule 有序判据（首个命中；规则编号与 A-8 列表一致）。
func matchStageRule(in StageInput, c LifecycleConf) (Stage, int) {
	// 0) 强制 latent：采样点不足（覆盖所有初始态歧义，B-6 初始黑洞闭合）。
	if in.NPoints < 3 {
		return StageLatent, 0
	}
	// 1) long_tail：绝对低热 ∨（低于近 24h P25 且老话题且仍有更新）。
	if in.Heat < c.LongTailHeatMax ||
		(in.Heat < in.HeatP25 && in.AgeDays > c.LongTailAgeDays && in.Updated24h) {
		return StageLongTail, 1
	}
	// 2) peak：绝对阈值 ∨ 相对阈值 ∨ 由正转负的高位回落（仅自 rising/peak）。
	if in.Heat >= c.HeatPeakMin || in.Heat >= in.HeatP99 ||
		(in.V < 0 && in.Heat >= c.HeatTurnMin &&
			(in.Current == StageRising || in.Current == StagePeak)) {
		return StagePeak, 2
	}
	// 3) rising：v_ewma > v_min 且持续 ≥2 点（M-3：无 heat 条件；二次爆发同路径）。
	if in.V > c.VMin && in.RisingStreak >= c.RisingMinStreak {
		return StageRising, 3
	}
	// 4) decaying：v_ewma < 0 且持续 ≥3 点。
	if in.V < 0 && in.DecayStreak >= c.DecayMinStreak {
		return StageDecaying, 4
	}
	// 5) dormant：近 48h 无更新（B-6 兜底态）。
	if in.NoUpdate48h {
		return StageDormant, 5
	}
	// 6) default：保持当前阶段（B-6：任何输入都有归宿，永不空转）。
	return in.Current, -1
}
