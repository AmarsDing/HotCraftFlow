package tt

import (
	"math"
	"testing"
	"time"
)

// ============================================================================
// M2 动态链单测（S-7 时间感知 EWMA / A-15 epoch 重置 / 乱序不推进）。
// ============================================================================

var dynT0 = time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

func TestAdvanceDynamicsFirstPoint(t *testing.T) {
	step := AdvanceDynamics(nil, 42, dynT0, "ep1", DefaultDynamicsConf())
	if !step.Advanced || !step.Reset {
		t.Fatalf("first point must advance+reset: %+v", step)
	}
	if step.Velocity != 0 || step.Accel != 0 {
		t.Fatalf("first point v/a must be 0: %+v", step)
	}
	if !almostEq(step.Next.HEwma, 42) || step.Next.WeightEpoch != "ep1" {
		t.Fatalf("state: %+v", step.Next)
	}
}

func TestAdvanceDynamicsTimeAwareAlpha(t *testing.T) {
	conf := DefaultDynamicsConf() // T = 0.75h
	prev := &DynamicsState{HEwma: 40, VEwma: 0, LastT: dynT0, WeightEpoch: "ep1"}

	// Δt = 0.75h → α = 1 - e^-1 ≈ 0.6321
	next := dynT0.Add(45 * time.Minute)
	step := AdvanceDynamics(prev, 60, next, "ep1", conf)
	alpha := 1 - math.Exp(-1)
	wantH := alpha*60 + (1-alpha)*40
	if math.Abs(step.Next.HEwma-wantH) > 1e-9 {
		t.Fatalf("h_ewma: got %v want %v", step.Next.HEwma, wantH)
	}
	// 瞬时 velocity = (h(t)-h(t-1))/Δt = (wantH-40)/0.75；
	// step.Velocity 是平滑后 v_ewma = α·v_inst + (1-α)·prev.VEwma（prev.VEwma=0）。
	wantVInst := (wantH - 40) / 0.75
	wantVEwma := alpha*wantVInst + (1-alpha)*0
	if math.Abs(step.Velocity-wantVEwma) > 1e-9 {
		t.Fatalf("v_ewma: got %v want %v", step.Velocity, wantVEwma)
	}
	// accel = (v_ewma(t) - v_ewma(t-1)) / Δt = (wantVEwma - 0)/0.75。
	wantAcc := wantVEwma / 0.75
	if math.Abs(step.Accel-wantAcc) > 1e-9 {
		t.Fatalf("accel: got %v want %v", step.Accel, wantAcc)
	}
	// S-7 要害：α 由 Δt 决定而非固定——换采样间隔时同一物理时长的平滑轨迹一致。
}

func TestAdvanceDynamicsEpochChangeResets(t *testing.T) {
	prev := &DynamicsState{HEwma: 80, VEwma: 9, LastT: dynT0, WeightEpoch: "ep1"}
	step := AdvanceDynamics(prev, 20, dynT0.Add(30*time.Minute), "ep2", DefaultDynamicsConf())
	// A-15 ④：α=1 重置——新口径首点直接采信，velocity/accel 归零防伪突发
	if !step.Reset || !step.Advanced {
		t.Fatalf("epoch change must reset: %+v", step)
	}
	if !almostEq(step.Next.HEwma, 20) || step.Velocity != 0 || step.Accel != 0 {
		t.Fatalf("epoch reset state: %+v", step)
	}
	if step.Next.WeightEpoch != "ep2" {
		t.Fatalf("epoch must advance: %+v", step.Next)
	}
}

func TestAdvanceDynamicsOutOfOrder(t *testing.T) {
	prev := &DynamicsState{HEwma: 50, VEwma: 3, LastT: dynT0, WeightEpoch: "ep1"}

	// 同桶重复（t == last_t）：不推进
	step := AdvanceDynamics(prev, 90, dynT0, "ep1", DefaultDynamicsConf())
	if step.Advanced || step.Velocity != 0 {
		t.Fatalf("duplicate bucket must not advance: %+v", step)
	}
	if !almostEq(step.Next.HEwma, 50) {
		t.Fatalf("state must be untouched: %+v", step.Next)
	}

	// 迟到乱序（t < last_t）：不推进
	step = AdvanceDynamics(prev, 90, dynT0.Add(-time.Hour), "ep1", DefaultDynamicsConf())
	if step.Advanced {
		t.Fatalf("late point must not advance: %+v", step)
	}
}

func TestAdvanceDynamicsZeroTauFallback(t *testing.T) {
	// conf 缺省（τ=0）→ 默认 0.75h，不除零/panic
	prev := &DynamicsState{HEwma: 10, VEwma: 0, LastT: dynT0, WeightEpoch: "ep"}
	step := AdvanceDynamics(prev, 20, dynT0.Add(time.Hour), "ep", DynamicsConf{})
	if !step.Advanced || math.IsNaN(step.Velocity) || math.IsInf(step.Accel, 0) {
		t.Fatalf("zero tau must fallback safely: %+v", step)
	}
}
