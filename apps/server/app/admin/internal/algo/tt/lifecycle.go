package tt

import (
	"framework/pkg/mediacontract"
)

// 生命周期阈值（默认值，可经 system_config 覆盖，key 前缀 tt.lifecycle.*）。
const (
	DefaultVMinRising    = 6.0  // rising 判据：v_ewma > 此值（分/小时）
	DefaultPeakHeat      = 80.0 // peak 判据之一：heat ≥ 此值
	DefaultLongTailHeat  = 20.0 // long_tail 判据之一：heat < 此值
	DefaultPeakHoldHeat  = 60.0 // peak 保持：v_ewma<0 且 heat ≥ 此值
	DefaultDormantHours  = 48.0 // dormant：近 48h 无更新
)

// MinDwellPoints 各阶段最小驻留**采样点数**（评审 B-7：统一为点数，不用墙钟时长）。
// 采样间隔可配 2–60 分钟，用点数才能保证防抖强度一致。
var MinDwellPoints = map[mediacontract.Stage]int32{
	mediacontract.StageLatent:   0,
	mediacontract.StageRising:   2,
	mediacontract.StagePeak:     1,
	mediacontract.StageDecaying: 3,
	mediacontract.StageLongTail: 6,
	mediacontract.StageDormant:  0,
}

// LifecycleInput 生命周期判定的输入（均为样本时间 t 口径的可重现值）。
type LifecycleInput struct {
	CurrentStage mediacontract.Stage // 当前阶段（首次判定传 latent）
	DwellPoints  int32               // 当前阶段内已写入的样本点数（由 stage_since 反查，非累加器）
	NPoints      int32               // 该话题总采样点数
	Heat         float64             // 当前 heat（样本时间口径）
	VelocityEWMA float64             // 平滑增速 v_ewma（分/小时）
	// RisingSustain / DecayingSustain 分别表示 v_ewma>v_min / v_ewma<0 已连续持续的采样点数。
	RisingSustain   int32
	DecayingSustain int32
	AgeHours        float64 // 话题年龄（t - first_seen，小时）
	HoursSinceUpdate float64 // 距最近一次有数据更新的小时数
	// 相对判据（评审 S-3）：P99/P25 为近 24h 全体活跃话题 heat 分位。
	P99Heat24h float64
	P25Heat24h float64
}

// DecideStage 判定目标阶段（评审 A-8：有序求值 + default 保持 + dormant 兜底）。
//
// 求值顺序（第一个命中为目标阶段；全不命中 → default 保持当前阶段）：
//  0) n_points < 3 → latent（强制，不动）
//  1) long_tail / dormant（见下）
//  2) peak / rising / decaying
//  6) default → 保持当前阶段
//
// 该函数保证「状态空间全覆盖」：任何输入都有确定输出，不会 panic 或留脏值（评审 B-6）。
func DecideStage(in LifecycleInput) mediacontract.Stage {
	// 0) 数据不足强制 latent（评审 B-6：点数 <3 不允许进入其他阶段）。
	if in.NPoints < 3 {
		return mediacontract.StageLatent
	}

	// 5) dormant：近 48h 无更新（优先于其它，因为无数据无法判热度）。
	if in.HoursSinceUpdate >= DefaultDormantHours {
		return mediacontract.StageDormant
	}

	// 1) long_tail：低热但持续存在（评审 S-3 修复：绝对值 ∨ 相对分位）。
	if in.Heat < DefaultLongTailHeat ||
		(in.Heat < in.P25Heat24h && in.AgeHours > 7*24 && in.HoursSinceUpdate < 24) {
		return mediacontract.StageLongTail
	}

	// 2) peak：绝对高热 ∨ 相对高热 ∨ 高位回落保持（评审 S-3 修复：加相对判据）。
	if in.Heat >= DefaultPeakHeat || in.Heat >= in.P99Heat24h ||
		(in.VelocityEWMA < 0 && in.Heat >= DefaultPeakHoldHeat &&
			(in.CurrentStage == mediacontract.StageRising || in.CurrentStage == mediacontract.StagePeak)) {
		return mediacontract.StagePeak
	}

	// 3) rising：平滑增速超阈值且持续 ≥2 点。
	if in.VelocityEWMA > DefaultVMinRising && in.RisingSustain >= 2 {
		return mediacontract.StageRising
	}

	// 4) decaying：平滑增速为负且持续 ≥3 点。
	if in.VelocityEWMA < 0 && in.DecayingSustain >= 3 {
		return mediacontract.StageDecaying
	}

	// 6) default：保持当前阶段（不转移、不重置 dwell 计数）。
	return in.CurrentStage
}

// CanTransition 判断是否允许从 current 转移到 target（评审 B-7：min_dwell 点数门槛）。
// 目标阶段与当前相同不算转移（恒允许，用于刷新）。
func CanTransition(current, target mediacontract.Stage, dwellPoints int32) bool {
	if current == target {
		return true
	}
	min, ok := MinDwellPoints[current]
	if !ok {
		min = 0
	}
	return dwellPoints >= min
}

// Transition 执行一次带防抖的阶段转移。
// 返回新阶段与是否发生转移。只有「目标阶段 ≠ 当前 且 满足最小驻留」才真正转移。
func Transition(in LifecycleInput) (mediacontract.Stage, bool) {
	target := DecideStage(in)
	if target == in.CurrentStage {
		return in.CurrentStage, false
	}
	if !CanTransition(in.CurrentStage, target, in.DwellPoints) {
		return in.CurrentStage, false // 驻留不足，防抖不转移
	}
	return target, true
}
