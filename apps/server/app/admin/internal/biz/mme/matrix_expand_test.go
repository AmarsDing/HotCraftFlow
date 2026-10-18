package mme

import (
	"strings"
	"testing"
)

func testConfig() Config {
	c := ResolveConfig(nil) // 全默认
	return c
}

func TestExpandMatrixCartesian(t *testing.T) {
	cfg := testConfig()
	hl := []Highlight{
		{ID: "hl_1", Label: "高光1", StartSec: 0, EndSec: 15, Origin: "manual"},
		{ID: "hl_2", Label: "高光2", StartSec: 20, EndSec: 40, Origin: "manual"},
	}
	all, err := ExpandMatrix(cfg, "vs_test", hl, []string{"9:16", "1:1", "16:9"}, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	// 2 内容 × 3 画幅 × 默认 2 hook = 12
	if len(all) != 12 {
		t.Fatalf("want 12 variants, got %d", len(all))
	}
	byVersion := map[string]int{}
	for _, v := range all {
		if !v.Selected {
			t.Fatal("🔴 先展开后渲染：全部变体须默认 selected=true")
		}
		if v.RenderState != RenderPending {
			t.Fatalf("initial render_state want pending, got %s", v.RenderState)
		}
		if !strings.HasPrefix(v.ID, "v_") {
			t.Fatalf("variant id prefix want v_, got %s", v.ID)
		}
		if v.DurationSec <= 0 {
			t.Fatal("duration should come from highlight")
		}
		byVersion[v.ContentVersion]++
	}
	for id, n := range byVersion {
		if n != 6 {
			t.Fatalf("content %s want 6 combos, got %d", id, n)
		}
	}
}

func TestExpandMatrixTooLarge(t *testing.T) {
	cfg := testConfig()
	cfg.MaxVariantsPerSet = 10
	hl := make([]Highlight, 3)
	for i := range hl {
		hl[i] = Highlight{ID: "h", StartSec: 0, EndSec: 10}
	}
	// 3 × 3 × 2 = 18 > 10 → 拒绝
	_, err := ExpandMatrix(cfg, "vs_x", hl, nil, nil)
	if err == nil || !strings.Contains(err.Error(), ErrExpandTooLarge.Error()) {
		t.Fatalf("want MME_EXPAND_TOO_LARGE, got %v", err)
	}
}

func TestExpandMatrixEmptyHighlights(t *testing.T) {
	_, err := ExpandMatrix(testConfig(), "vs_x", nil, nil, nil)
	if err != ErrHighlightsEmpty {
		t.Fatalf("want ErrHighlightsEmpty, got %v", err)
	}
}

func TestExpandMatrixBadAspect(t *testing.T) {
	cfg := testConfig()
	hl := []Highlight{{ID: "hl_1", StartSec: 0, EndSec: 10}}
	_, err := ExpandMatrix(cfg, "vs_x", hl, []string{"4:3"}, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown aspect") {
		t.Fatalf("want unknown aspect error, got %v", err)
	}
}

func TestExpandMatrixDefaultAspects(t *testing.T) {
	cfg := testConfig()
	hl := []Highlight{{ID: "hl_1", StartSec: 0, EndSec: 10}}
	all, err := ExpandMatrix(cfg, "vs_x", hl, nil, nil)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	// 1 × 3 默认画幅 × 2 默认 hook = 6
	if len(all) != 6 {
		t.Fatalf("want 6 (default aspects x default hooks), got %d", len(all))
	}
	seen := map[string]bool{}
	for _, v := range all {
		seen[v.Aspect] = true
	}
	for _, want := range []string{"9:16", "1:1", "16:9"} {
		if !seen[want] {
			t.Fatalf("missing default aspect %s", want)
		}
	}
}
