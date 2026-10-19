package tt

import "testing"

var lc = DefaultLifecycleConf()

func eval(in StageInput) StageResult { return EvalStage(in, lc) }

// base 输入：默认「健康的成长期话题」语境，各用例按需改写。
func baseInput() StageInput {
	return StageInput{
		Current: StageLatent, DwellPoints: 0, NPoints: 5,
		Heat: 50, V: 0, AgeDays: 1, Updated24h: true,
		HeatP25: 10, HeatP99: 90, // 真实分位基线，否则 P99=0 会让规则 2 恒命中
		RisingStreak: 0, DecayStreak: 0,
	}
}

// TestEvalStageOrderedRules 判据 0–5 逐项命中（A-8 有序列表）。
func TestEvalStageOrderedRules(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*StageInput)
		want Stage
		rule int
	}{
		{"规则0: 点数<3 强制 latent（即便高热高增速）", func(i *StageInput) {
			i.NPoints = 2
			i.Heat, i.V, i.RisingStreak = 95, 30, 5
		}, StageLatent, 0},
		{"规则1A: heat<20 → long_tail", func(i *StageInput) {
			i.Heat = 15
		}, StageLongTail, 1},
		{"规则1B: 低于P25且>7天龄且24h有更新 → long_tail", func(i *StageInput) {
			i.Heat, i.AgeDays, i.HeatP25 = 40, 8, 45
		}, StageLongTail, 1},
		{"规则1B豁免: 24h无更新不成绝对长尾（规则5 dormant 覆盖沉寂）", func(i *StageInput) {
			i.Heat, i.AgeDays, i.HeatP25 = 40, 8, 45
			i.Updated24h = false
		}, StageLatent, -1}, // 无规则命中 → 保持（见 B-6 默认分支）
		{"规则2: heat≥80 → peak（胜过其余）", func(i *StageInput) {
			i.Heat, i.V, i.DecayStreak = 90, -0.5, 2
		}, StagePeak, 2},
		{"规则2: heat≥P99 → peak（相对线）", func(i *StageInput) {
			i.Heat, i.HeatP99 = 55, 50
		}, StagePeak, 2},
		{"规则2: 由正转负高位回落 rising→peak", func(i *StageInput) {
			i.Current, i.DwellPoints = StageRising, 2
			i.Heat, i.V, i.DecayStreak = 65, -1, 1
		}, StagePeak, 2},
		{"规则2分支限源: 自 decaying 的 v<0 不归 peak（落规则4）", func(i *StageInput) {
			i.Current, i.DwellPoints = StageDecaying, 3
			i.Heat, i.V, i.DecayStreak = 65, -1, 1
		}, StageDecaying, -1}, // decay 持续仅 1 点(<3)且不静 → 保持 decaying
		{"规则3: 持续2点高增长 → rising（无 heat 条件，M-3）", func(i *StageInput) {
			i.V, i.RisingStreak = 7, 2
			i.Heat = 25 // 高于长尾线但低于 P25 口径外的任意值；M-3：rising 判据无 heat 条件
		}, StageRising, 3},
		{"规则3: 持续仅1点门槛未达 → 保持", func(i *StageInput) {
			i.V, i.RisingStreak = 9, 1
		}, StageLatent, -1},
		{"规则4: 持续3点负增速 → decaying", func(i *StageInput) {
			i.V, i.DecayStreak = -2, 3
			i.Heat = 30
		}, StageDecaying, 4},
		{"规则4: 持续仅2点 → 保持", func(i *StageInput) {
			i.V, i.DecayStreak = -2, 2
			i.Heat = 30
		}, StageLatent, -1},
		{"规则5: 48h 无更新 → dormant", func(i *StageInput) {
			i.NoUpdate48h = true
			i.V, i.DecayStreak = 0, 0
		}, StageDormant, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := baseInput()
			c.mut(&in)
			got := eval(in)
			if got.Rule != c.rule {
				t.Fatalf("规则编号不符: got %d want %d", got.Rule, c.rule)
			}
			if c.rule >= 0 {
				if got.Stage != c.want {
					t.Fatalf("目标阶段不符: got %s want %s", got.Stage, c.want)
				}
			} else if got.Stage != in.Current {
				t.Fatalf("default 应保持当前: got %s want %s", got.Stage, in.Current)
			}
		})
	}
}

// TestEvalStageBlackHole B-6 原文黑洞：heat=50, v=+2, n_points=5, pc=3
// 在旧五条判据下无匹配分支。有序列表下必须无损保持（不 panic、不脏值）。
func TestEvalStageBlackHole(t *testing.T) {
	for _, cur := range []Stage{StageLatent, StageRising, StageDecaying, StagePeak} {
		in := StageInput{
			Current: cur, DwellPoints: 3, NPoints: 5,
			Heat: 50, V: 2, AgeDays: 1, Updated24h: true,
			HeatP25: 10, HeatP99: 90,
		}
		got := eval(in)
		if got.Stage != cur {
			t.Fatalf("B-6 黑洞输入必须保持当前阶段: cur=%s got=%s rule=%d", cur, got.Stage, got.Rule)
		}
		if got.Changed {
			t.Fatalf("B-6 黑洞输入不得发生切换: cur=%s", cur)
		}
	}
}

// TestEvalStageHysteresis 最小驻留闸（B-7 点数口径）：驻留不足拦下，
// 驻满放行；rising 需要 2 点、decaying 3 点、peak 1 点、long_tail 6 点。
func TestEvalStageHysteresis(t *testing.T) {
	// decaying(dwell=1<3) 想做二次爆发 → Held 保持 decaying
	in := StageInput{Current: StageDecaying, DwellPoints: 1, NPoints: 8,
		Heat: 50, V: 8, RisingStreak: 2, Updated24h: true, HeatP25: 10, HeatP99: 90}
	if got := eval(in); !got.Held || got.Changed || got.Stage != StageDecaying {
		t.Fatalf("驻留不足应 Held 保持: %+v", got)
	}
	// dwell=3 驻满 → 二次爆发 decaying→rising 放行（M-3 无 heat>30）
	in.DwellPoints = 3
	if got := eval(in); !got.Changed || got.Stage != StageRising || got.Rule != 3 {
		t.Fatalf("二次爆发应放行: %+v", got)
	}
	// rising(dwell=1<2) 转 decaying → 拦下；dwell=2 放行
	in = StageInput{Current: StageRising, DwellPoints: 1, NPoints: 8,
		Heat: 40, V: -2, DecayStreak: 3, Updated24h: true, HeatP25: 10, HeatP99: 90}
	if got := eval(in); !got.Held || got.Stage != StageRising {
		t.Fatalf("rising 驻留不足应拦下: %+v", got)
	}
	in.DwellPoints = 2
	if got := eval(in); !got.Changed || got.Stage != StageDecaying || got.Rule != 4 {
		t.Fatalf("rising 驻满应转 decaying: %+v", got)
	}
	// peak(dwell 0<1) 首点不得立即转出；dwell=1 即可（peak 驻留=1）
	in = StageInput{Current: StagePeak, DwellPoints: 0, NPoints: 10,
		Heat: 30, V: -2, DecayStreak: 3, Updated24h: true, HeatP25: 10, HeatP99: 90}
	if got := eval(in); !got.Held || got.Stage != StagePeak {
		t.Fatalf("peak 首点驻留不足应拦下: %+v", got)
	}
	in.DwellPoints = 1
	if got := eval(in); !got.Changed || got.Stage != StageDecaying {
		t.Fatalf("peak 驻 1 点应可转出: %+v", got)
	}
	// latent 与 dormant 无驻留门槛（0）
	in = StageInput{Current: StageDormant, DwellPoints: 0, NPoints: 9,
		Heat: 45, V: 8, RisingStreak: 2, Updated24h: true, HeatP25: 10, HeatP99: 90}
	if got := eval(in); !got.Changed || got.Stage != StageRising {
		t.Fatalf("dormant 应即时转出: %+v", got)
	}
}

// TestStageMinDwellSpec 驻留阈值即评审终稿（改动这里=改动防抖强度，有测试兜底）。
func TestStageMinDwellSpec(t *testing.T) {
	want := map[Stage]int{
		StageLatent: 0, StageRising: 2, StagePeak: 1,
		StageDecaying: 3, StageLongTail: 6, StageDormant: 0,
	}
	for s, w := range want {
		if got := MinDwellStage(s); got != w {
			t.Fatalf("min_dwell(%s)=%d want %d（A-8/B-7 终稿）", s, got, w)
		}
	}
	// 未知阶段返回 0（不设防，尽快转出脏状态）
	if got := MinDwellStage(Stage("unknown")); got != 0 {
		t.Fatalf("unknown stage 应返回 0: %d", got)
	}
}

// TestLifecycleRule2ColdStart 对抗裁决发现：HeatP99=0（冷启动无分位基线）时
// heat≥0 恒真 → 全站高热洪泛 peak。修复后相对线要求 P99>0，绝对线仍生效。
func TestLifecycleRule2ColdStart(t *testing.T) {
	in := baseInput()
	in.HeatP99 = 0 // 冷启动：无相对线
	in.Heat = 50   // < HeatPeakMin(80)，相对线若未禁用会误判
	got := eval(in)
	if got.Stage == StagePeak {
		t.Fatalf("P99=0 时相对线必须禁用，heat=50 不应入 peak: %+v", got)
	}
	// 绝对线不受冷启动影响
	in.Heat = 85
	got = eval(in)
	if got.Stage != StagePeak || got.Rule != 2 {
		t.Fatalf("P99=0 时绝对线仍应生效: %+v", got)
	}
}

// TestLifecycleConfValidate 零值配置静默灾变防线。
func TestLifecycleConfValidate(t *testing.T) {
	if err := DefaultLifecycleConf().Validate(); err != nil {
		t.Fatalf("默认配置须过红线: %v", err)
	}
	if err := (LifecycleConf{}).Validate(); err == nil {
		t.Fatal("零值 LifecycleConf 必须拒绝（HeatPeakMin=0 全量 peak 灾变）")
	}
	bad := DefaultLifecycleConf()
	bad.RisingMinStreak = 0
	if err := bad.Validate(); err == nil {
		t.Fatal("RisingMinStreak=0 必须拒绝（抖动直入 rising）")
	}
}
