package tt

import (
	"errors"
	"math"
	"testing"
	"time"
)

// ============================================================================
// M2 归一化单测（04-开发进度 §4.2 门禁：榜首/榜尾/无榜/权重和为 1 边界）。
// 公式基准：评审 A-2/A-4/A-5/A-6/A-7。
// ============================================================================

func almostEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRankPctBoundaries(t *testing.T) {
	cases := []struct {
		name            string
		rank, rankTotal int32
		want            float64
		valid           bool
	}{
		{"榜首 50 位榜", 1, 50, 1.0, true},
		{"榜尾 50 位榜", 50, 50, 0.0, true},
		{"榜首 10 位短榜(A-7 差 0.10 用例)", 1, 10, 1.0, true}, // 03 式：短榜榜首也满分
		{"榜中 50 位榜", 25, 50, float64(50-25) / 49, true},
		{"无榜哨兵", -1, -1, 0, false},
		{"榜长未知", 3, -1, 0, false},
		{"rank 零值", 0, 50, 0, false},
		{"单人榜 rank=1 rt=1", 1, 1, 0, true}, // (1-1)/max(0,1)=0，公式口径
		{"超榜钳制", 60, 50, 0, true},         // rank>rank_total 数据异常 → 0
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, valid := RankPct(c.rank, c.rankTotal)
			if valid != c.valid {
				t.Fatalf("valid: got %v want %v", valid, c.valid)
			}
			if valid && !almostEq(got, c.want) {
				t.Fatalf("pct: got %v want %v", got, c.want)
			}
		})
	}
}

func TestFreshness(t *testing.T) {
	first := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)

	// t = first_seen → 1.0
	if got := Freshness(first, first, 12, 0.05); !almostEq(got, 1.0) {
		t.Fatalf("at birth: %v", got)
	}
	// t = first+τ → e^-1 ≈ 0.3679
	if got := Freshness(first.Add(12*time.Hour), first, 12, 0.05); math.Abs(got-math.Exp(-1)) > 1e-6 {
		t.Fatalf("at tau: %v", got)
	}
	// 半衰期 8.32h → ≈0.5（A-4 半衰期口径 τ·ln2）
	halfLife := 12 * math.Ln2 // 8.3177h，用变量避开常量转换限制
	if got := Freshness(first.Add(time.Duration(halfLife*float64(time.Hour))), first, 12, 0.05); math.Abs(got-0.5) > 1e-3 {
		t.Fatalf("at half-life: %v", got)
	}
	// 久远 → 地板 0.05（S-3：二次爆发可达 peak）
	if got := Freshness(first.Add(30*24*time.Hour), first, 12, 0.05); !almostEq(got, 0.05) {
		t.Fatalf("floor: %v", got)
	}
	// 负偏移（乱序）钳制为出生即 1.0
	if got := Freshness(first.Add(-time.Hour), first, 12, 0.05); !almostEq(got, 1.0) {
		t.Fatalf("negative dt clamp: %v", got)
	}
}

func TestBreadth(t *testing.T) {
	// P=8（默认）：单平台 → 0（A-5 修正 0.3155 白送分）
	if got := Breadth(1, 0); !almostEq(got, 0) {
		t.Fatalf("single platform: %v", got)
	}
	// pc ≥ ref → 1
	if got := Breadth(8, 0); !almostEq(got, 1) {
		t.Fatalf("pc>=ref: %v", got)
	}
	// 中间值单调
	mid := Breadth(4, 0)
	if mid <= 0 || mid >= 1 {
		t.Fatalf("mid range: %v", mid)
	}
	// P=5（IC 实际覆盖 5 平台）：ref=5，pc=5 应满分（A-5 隐性降权修复）
	if got := Breadth(5, 5); !almostEq(got, 1) {
		t.Fatalf("P=5 full coverage: %v", got)
	}
	// pc=0/负数异常 → 按单平台 0 处理
	if got := Breadth(0, 0); !almostEq(got, 0) {
		t.Fatalf("pc=0: %v", got)
	}
}

func TestHeatWeightsValidate(t *testing.T) {
	if err := DefaultHeatWeights().Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if err := (HeatWeights{0.5, 0.3, 0.1, 0.05}).Validate(); err == nil {
		t.Fatal("sum=0.95 must fail (权重和为 1 校验)")
	}
	if err := (HeatWeights{1.2, 0, 0, -0.2}).Validate(); err == nil {
		t.Fatal("out-of-range must fail")
	}
	if err := (HeatWeights{0.25, 0.25, 0.25, 0.25}).Validate(); err != nil {
		t.Fatalf("equal quarters must pass: %v", err)
	}
}

func TestCombineHeatDefaultAllValid(t *testing.T) {
	// 全因子有效 + 默认权重：解析解
	// 0.35×1 + 0.25×0.5 + 0.20×1 + 0.20×0 = 0.675 → 67.5
	res, err := CombineHeat(DefaultHeatWeights(), 1.0, 0.5, 1.0, 0.0, true, true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !almostEq(res.Score, 67.5) {
		t.Fatalf("score: %v", res.Score)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("missing should be empty: %v", res.Missing)
	}
	// A-6 恒等断言：Σ w_eff == 1
	sum := res.WeightsEff.WRank + res.WeightsEff.WEng + res.WeightsEff.WFresh + res.WeightsEff.WBreadth
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("eff weights sum: %v", sum)
	}
	if res.Weights != DefaultHeatWeights() {
		t.Fatalf("raw weights drifted: %+v", res.Weights)
	}
}

func TestCombineHeatMissingRankReallocates(t *testing.T) {
	// 无榜：rank 缺失 → 权重 0.35 按比例重分给其余三因子
	// S = 0.25+0.20+0.20 = 0.65；eng/fresh 均 1.0、breadth 0
	// heat = 100 × (0.25/0.65×1 + 0.20/0.65×1 + 0.20/0.65×0) = 100 × 0.45/0.65
	res, err := CombineHeat(DefaultHeatWeights(), 0, 1.0, 1.0, 0.0, false, true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	want := 100 * 0.45 / 0.65
	if math.Abs(res.Score-want) > 1e-6 {
		t.Fatalf("score: got %v want %v", res.Score, want)
	}
	if !almostEq(res.WeightsEff.WRank, 0) {
		t.Fatalf("eff rank weight must be 0: %v", res.WeightsEff.WRank)
	}
	if !almostEq(res.WeightsEff.WEng, 0.25/0.65) {
		t.Fatalf("eff eng: %v", res.WeightsEff.WEng)
	}
	if len(res.Missing) != 1 || res.Missing[0] != FactorRank {
		t.Fatalf("missing: %v", res.Missing)
	}
	// 恒等：有效权重和为 1
	sum := res.WeightsEff.WRank + res.WeightsEff.WEng + res.WeightsEff.WFresh + res.WeightsEff.WBreadth
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("eff weights sum: %v", sum)
	}
}

func TestCombineHeatAllMissing(t *testing.T) {
	res, err := CombineHeat(DefaultHeatWeights(), 0, 0, 0, 0, false, false, false, false)
	if !errors.Is(err, ErrNoFactor) {
		t.Fatalf("must be ErrNoFactor: %v", err)
	}
	if len(res.Missing) != 4 {
		t.Fatalf("missing must list all four: %v", res.Missing)
	}
	if !almostEq(res.Score, 0) {
		t.Fatalf("heat=null → score 0 sentinel: %v", res.Score)
	}
}

func TestCombineHeatEffSumIdentity(t *testing.T) {
	// 遍历各缺失组合，CI 级恒等断言：只要不全缺失，Σ w_eff == 1 (±1e-9)。
	for mask := 1; mask < 16; mask++ {
		rv, ev, fv, bv := mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0
		res, err := CombineHeat(DefaultHeatWeights(), 0.8, 0.6, 0.9, 0.3, rv, ev, fv, bv)
		if err != nil {
			t.Fatalf("mask %d: %v", mask, err)
		}
		sum := res.WeightsEff.WRank + res.WeightsEff.WEng + res.WeightsEff.WFresh + res.WeightsEff.WBreadth
		if math.Abs(sum-1) > 1e-9 {
			t.Fatalf("mask %d eff sum %v", mask, sum)
		}
		if res.Score < 0 || res.Score > 100 {
			t.Fatalf("mask %d score out of range: %v", mask, res.Score)
		}
	}
}

func TestBaselineEngPct(t *testing.T) {
	// raw=0 → 0；raw≥p100 → 1；中间单调
	if got := BaselineEngPct("weibo", 0); !almostEq(got, 0) {
		t.Fatalf("raw=0: %v", got)
	}
	if got := BaselineEngPct("weibo", 13); !almostEq(got, 1) {
		t.Fatalf("raw>=max: %v", got)
	}
	a := BaselineEngPct("weibo", 3.0)
	b := BaselineEngPct("weibo", 6.0)
	if !(a < b) {
		t.Fatalf("monotonic: %v !< %v", a, b)
	}
	// 未知平台 → 默认基线不 panic
	if got := BaselineEngPct("unknown-x", 5.0); got <= 0 || got >= 1 {
		t.Fatalf("default baseline mid: %v", got)
	}
}
