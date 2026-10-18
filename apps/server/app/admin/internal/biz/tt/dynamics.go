package tt

import (
	"math"
	"time"
)

// ============================================================================
// 热度动态（03-详细设计 §2.1 L3 + 评审 S-7/A-15）。
//
//   S-7  时间感知 EWMA：α_n = 1 - exp(-Δt_h / T)，T = 0.75h。
//        采样间隔 2–60min 可配（FR-TRK-01），固定 α 会让同一话题换个采集频率
//        velocity 就不可比；时间感知化后平滑窗口的物理时长恒定。
//   A-15 权重口径断点：weight_epoch 变化后第一个点强制 α=1（重置 h_ewma/v_ewma），
//        避免跨口径差分产生全站伪 velocity 尖峰（伪突发潮）。
//   乱序点（t ≤ 已见 t_bucket）：不推进状态机（链式状态不可回插），
//        该点 velocity/accel 记 0 由后续 recompute（S-23/A-14 重算 worker）修正。
// ============================================================================

// DynamicsConf 动态参数（03 §4 可配项）。
type DynamicsConf struct {
	// EWMATauHours EWMA 时间常数 T（α = 1-exp(-Δt/T)）。默认 0.75h（S-7）。
	EWMATauHours float64 `json:"ewma_tau_hours"`
}

// DefaultDynamicsConf 默认动态参数。
func DefaultDynamicsConf() DynamicsConf {
	return DynamicsConf{EWMATauHours: 0.75}
}

// DynamicsState 单 (topic_id, platform) 的动态链状态（Redis JSON 持久化）。
type DynamicsState struct {
	HEwma      float64   `json:"h_ewma"`      // 热度平滑值
	VEwma      float64   `json:"v_ewma"`      // 增速平滑值（分/小时）
	LastT      time.Time `json:"last_t"`      // 已推进到的 t_bucket（UTC）
	WeightEpoch string   `json:"weight_epoch"` // 状态所属权重口径（A-15）
}

// DynamicsStep 一次推进的结果。
type DynamicsStep struct {
	Velocity float64 // 本点 v_ewma（分/小时）
	Accel    float64 // 本点加速度（分/小时²）
	Advanced bool    // 是否推进了状态（false=乱序点，状态未动，本点记 0）
	Reset    bool    // 是否发生了 α=1 重置（首点或 epoch 变更）
	Next     DynamicsState
}

// AdvanceDynamics 推进一个样本点的动态链（纯函数；存储由端口负责）。
//
// 语义：
//   - prev == nil（首点）或 epoch 变更：α=1 重置（A-15 ④），h_ewma=h、v_ewma=0、accel=0；
//   - 乱序/同桶重复（t ≤ prev.LastT）：返回 Advanced=false，状态原样返回
//     （写入侧应跳过状态落盘，velocity/accel 落 0，待 recompute 修正）；
//   - 正常前进：α = 1-exp(-Δt/T)，velocity = (h_ewma(t)-h_ewma(t-1))/Δt，
//     v_ewma 同式平滑，accel = (v_ewma(t)-v_ewma(t-1))/Δt。
func AdvanceDynamics(prev *DynamicsState, heat float64, t time.Time, weightEpoch string, conf DynamicsConf) DynamicsStep {
	t = t.UTC()
	if conf.EWMATauHours <= 0 {
		conf = DefaultDynamicsConf()
	}

	// 首点 / 权重口径断点 → α=1 重置。
	if prev == nil || prev.WeightEpoch != weightEpoch {
		next := DynamicsState{
			HEwma: heat, VEwma: 0, LastT: t, WeightEpoch: weightEpoch,
		}
		return DynamicsStep{Velocity: 0, Accel: 0, Advanced: true, Reset: true, Next: next}
	}

	if !t.After(prev.LastT) {
		// 乱序/同桶重复点：链式状态不可回插（否则会双重计入）。
		return DynamicsStep{Velocity: 0, Accel: 0, Advanced: false, Next: *prev}
	}

	dt := t.Sub(prev.LastT).Hours()
	alpha := 1 - math.Exp(-dt/conf.EWMATauHours)

	hEwma := alpha*heat + (1-alpha)*prev.HEwma
	velocity := (hEwma - prev.HEwma) / dt
	vEwma := alpha*velocity + (1-alpha)*prev.VEwma
	accel := (vEwma - prev.VEwma) / dt

	next := DynamicsState{
		HEwma: hEwma, VEwma: vEwma, LastT: t, WeightEpoch: weightEpoch,
	}
	return DynamicsStep{Velocity: vEwma, Accel: accel, Advanced: true, Next: next}
}
