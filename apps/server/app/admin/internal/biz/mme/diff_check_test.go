package mme

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

func TestSimhashIdentical(t *testing.T) {
	s := "三句话告诉你为什么这只基金不能买"
	if got := SimhashSimilarity(s, s); got != 1 {
		t.Fatalf("identical text want 1.0, got %f", got)
	}
}

func TestSimhashNearDuplicate(t *testing.T) {
	a := "三句话告诉你为什么这只基金不能买"
	b := "三句话告诉你为什么这只基金不能买！"
	sim := SimhashSimilarity(a, b)
	if sim < 0.80 {
		t.Fatalf("near-duplicate want high similarity, got %f", sim)
	}
}

func TestSimhashDifferent(t *testing.T) {
	a := "美联储降息对科技股的影响分析"
	b := "厨房收纳的十个实用技巧分享"
	sim := SimhashSimilarity(a, b)
	if sim > 0.85 {
		t.Fatalf("unrelated texts want low similarity, got %f", sim)
	}
}

func TestDiffCheckerWarnOnSimilarHooks(t *testing.T) {
	cfg := testConfig()
	dc := NewDiffChecker(cfg, log.DefaultLogger)
	hookTexts := map[string]string{
		"hook_a": "三句话告诉你为什么这只基金不能买",
		"hook_b": "三句话告诉你为什么这只基金不能买", // 刻意同文本 → 必须告警
	}
	vs := []*Variant{
		{ID: "v_a", SetID: "vs_1", ContentVersion: "hl_1", Aspect: "9:16", HookID: "hook_a",
			Copy: map[string]PlatformCopy{"douyin": {Title: "基金避雷指南", Desc: "深扒这只基金的三个坑"}}},
		{ID: "v_b", SetID: "vs_1", ContentVersion: "hl_1", Aspect: "9:16", HookID: "hook_b",
			Copy: map[string]PlatformCopy{"douyin": {Title: "基金避雷指南", Desc: "深扒这只基金的三个坑"}}},
	}
	out := dc.Check(context.Background(), vs, func(id string) string { return hookTexts[id] }, "douyin", nil)
	if !out.Warn {
		t.Fatalf("🔴 FR-MIX-05: identical hook+copy pairs must warn (min=%.3f)", out.MinSimilarity)
	}
	if out.MinSimilarity < cfg.DiffWarnThreshold {
		t.Fatalf("min similarity %.3f should >= warn threshold %.3f", out.MinSimilarity, cfg.DiffWarnThreshold)
	}
	if len(out.Pairs) != 1 {
		t.Fatalf("want 1 compared pair, got %d", len(out.Pairs))
	}
	if !strings.Contains(out.Summary, "审核台复核") {
		t.Fatalf("summary should instruct review desk, got %q", out.Summary)
	}
}

func TestDiffCheckerSkipsCrossContentVersion(t *testing.T) {
	dc := NewDiffChecker(testConfig(), log.DefaultLogger)
	vs := []*Variant{
		{ID: "v_a", SetID: "vs_1", ContentVersion: "hl_1", HookID: "hook_a"},
		{ID: "v_b", SetID: "vs_1", ContentVersion: "hl_2", HookID: "hook_a"},
	}
	out := dc.Check(context.Background(), vs, func(string) string { return "x" }, "douyin", nil)
	if out.Warn || len(out.Pairs) != 0 {
		t.Fatalf("cross content_version pairs must be skipped, got warn=%v pairs=%d", out.Warn, len(out.Pairs))
	}
}

func TestDiffCheckerDistinctCopiesNoWarn(t *testing.T) {
	dc := NewDiffChecker(testConfig(), log.DefaultLogger)
	hooks := map[string]string{
		"hook_a": "三句话告诉你为什么这只基金不能买",
		"hook_b": "看完这段你再决定要不要入场",
	}
	vs := []*Variant{
		{ID: "v_a", SetID: "vs_1", ContentVersion: "hl_1", HookID: "hook_a",
			Copy: map[string]PlatformCopy{"douyin": {Title: "基金避雷指南", Desc: "深扒这只基金的三个坑"}}},
		{ID: "v_b", SetID: "vs_1", ContentVersion: "hl_1", HookID: "hook_b",
			Copy: map[string]PlatformCopy{"douyin": {Title: "入场前最后提醒", Desc: "流动性、费率、持仓集中度逐一拆解"}}},
	}
	out := dc.Check(context.Background(), vs, func(id string) string { return hooks[id] }, "douyin", nil)
	if out.Warn {
		t.Fatalf("distinct hooks+copies should not warn (min=%.3f)", out.MinSimilarity)
	}
}
