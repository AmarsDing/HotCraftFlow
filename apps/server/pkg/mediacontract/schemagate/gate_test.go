package schemagate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"framework/pkg/mediacontract"
)

// ============================================================================
// 契约闸门测试（评审 C-1~C-15 防回归）：
//  1. 好样本用 mediacontract Go 结构体序列化生成——同时证明「结构体 ⇔ schema」对齐；
//  2. 坏样本逐类违约必须被拒；
//  3. 漂移测试：内嵌副本必须与 docs/systems/00-shared/schema/ SSOT 逐字节一致。
// ============================================================================

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func goodTrendSeries(t *testing.T) map[string]any {
	t.Helper()
	ts := &mediacontract.TrendSeries{
		APIVersion: "1.0",
		TopicID:    mediacontract.NewID("tp"),
		Label:      "示例话题",
		Category:   mediacontract.CategoryAHot,
		Stage:      mediacontract.StageRising,
		StageEvidence: mediacontract.StageEvidence{
			Velocity: 12.5, Accel: 1.2, DurationPoints: 6,
		},
		Heat: mediacontract.Heat{
			Score: 78.5, RankPct: 0.9, EngPct: 0.8, EngRate: 0.05,
			Freshness: 0.7, Velocity: 12.5, Accel: 1.2,
			Breadth: mediacontract.Breadth{PlatformCount: 2, Platforms: []string{"weibo", "bilibili"}},
			Factors: mediacontract.HeatFactors{
				WRank: 0.4, WEng: 0.3, WFresh: 0.2, WBreadth: 0.1,
				WRankEff: 0.4, WEngEff: 0.3, WFreshEff: 0.2, WBreadthEff: 0.1,
			},
		},
		Series: []mediacontract.SeriesPoint{
			{T: "2026-10-18T10:00:00Z", Platform: "weibo", Rank: 3, Heat: 78.5},
		},
		Burst: mediacontract.Burst{
			IsBurst:    true,
			MethodsHit: []mediacontract.BurstMethod{mediacontract.BurstZScore, mediacontract.BurstRankJump},
			Evidence:   map[string]float64{"zscore": 3.2, "rank_jump": 15},
		},
		Origin: mediacontract.Origin{
			FirstSeenAt:   "2026-10-18T08:00:00Z",
			FirstPlatform: "weibo",
			FirstURL:      "https://example.com/p/1",
		},
		Diagnostics: &mediacontract.Diagnostics{InsufficientPoints: false, NPoints: 48},
	}
	var m map[string]any
	if err := json.Unmarshal(mustJSON(t, ts), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func goodCandidate(t *testing.T) map[string]any {
	t.Helper()
	tc := &mediacontract.TopicCandidate{
		APIVersion:      "1.0",
		CandidateID:     mediacontract.NewID("tc"),
		TopicID:         mediacontract.NewID("tp"),
		Route:           mediacontract.RouteMME,
		Score:           mediacontract.CandidateScore{Hot: 0.9, Freshness: 0.8, Workability: 0.7, Compliance: 1.0, Total: 84.5},
		SuggestedAngles: []string{"科普切入"},
		MaterialRefs:    []string{"asset://demo"},
		State:           mediacontract.CandidatePending,
	}
	var m map[string]any
	if err := json.Unmarshal(mustJSON(t, tc), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func goodAlert(t *testing.T) map[string]any {
	t.Helper()
	al := &mediacontract.Alert{
		APIVersion:  "1.0",
		AlertID:     mediacontract.NewID("al"),
		TopicID:     mediacontract.NewID("tp"),
		TriggeredAt: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC),
		MethodsHit:  []mediacontract.BurstMethod{mediacontract.BurstZScore, mediacontract.BurstEngSpike},
		Evidence:    map[string]float64{"zscore": 3.2, "eng_spike": 5.1},
		Channels:    []string{"webhook", "inapp"},
		State:       mediacontract.AlertNew,
	}
	var m map[string]any
	if err := json.Unmarshal(mustJSON(t, al), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func TestGoodDocsPass(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		doc    map[string]any
	}{
		{"TrendSeries", TrendSeries, goodTrendSeries(t)},
		{"TopicCandidate", TopicCandidate, goodCandidate(t)},
		{"Alert", Alert, goodAlert(t)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Validate(c.schema, mustJSON(t, c.doc)); err != nil {
				t.Fatalf("good doc rejected: %v", err)
			}
		})
	}
}

func TestBadDocsRejected(t *testing.T) {
	mut := func(base map[string]any, f func(m map[string]any)) map[string]any {
		b, _ := json.Marshal(base)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		f(m)
		return m
	}

	cases := []struct {
		name   string
		schema string
		doc    map[string]any
	}{
		{"TS 缺 stage", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) { delete(m, "stage") })},
		{"TS 非法 category", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) { m["category"] = "Z_x" })},
		{"TS topic_id 含小写 ULID", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) {
			m["topic_id"] = "tp_01j8v3m5wqaz2x4c6v7b8n9m0k"
		})},
		{"TS 多出未知字段", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) { m["surprise"] = 1 })},
		{"TS is_burst 但命中 <2", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) {
			m["burst"] = map[string]any{"is_burst": true, "methods_hit": []any{"zscore"}, "evidence": map[string]any{"zscore": 3.2}}
		})},
		{"TS series 超 200 点", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) {
			pts := make([]any, 0, 201)
			for i := 0; i < 201; i++ {
				pts = append(pts, map[string]any{"t": "2026-10-18T10:00:00Z", "platform": "weibo", "rank": i, "heat": 50.0})
			}
			m["series"] = pts
		})},
		{"TS origin 带 +08:00（M-21 违例）", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) {
			m["origin"] = map[string]any{"first_seen_at": "2026-10-18T08:00:00+08:00", "first_platform": "weibo", "first_url": "https://x"}
		})},
		{"TS heat.score 越界", TrendSeries, mut(goodTrendSeries(t), func(m map[string]any) {
			m["heat"].(map[string]any)["score"] = 120.0
		})},
		{"TC 非法 route", TopicCandidate, mut(goodCandidate(t), func(m map[string]any) { m["route"] = "tt" })},
		{"TC reject_reason 类型错", TopicCandidate, mut(goodCandidate(t), func(m map[string]any) { m["reject_reason"] = 42 })},
		{"AL methods_hit <2", Alert, mut(goodAlert(t), func(m map[string]any) { m["methods_hit"] = []any{"zscore"} })},
		{"AL methods_hit 非法方法", Alert, mut(goodAlert(t), func(m map[string]any) {
			m["methods_hit"] = []any{"zscore", "moon_phase"}
		})},
		{"AL triggered_at 带 +08:00", Alert, mut(goodAlert(t), func(m map[string]any) {
			m["triggered_at"] = "2026-10-18T10:00:00+08:00"
		})},
		{"AL channels 非法渠道", Alert, mut(goodAlert(t), func(m map[string]any) { m["channels"] = []any{"sms"} })},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Validate(c.schema, mustJSON(t, c.doc)); err == nil {
				t.Fatal("bad doc must be rejected")
			}
		})
	}
}

// TestSchemaDrift 内嵌副本与 docs SSOT 逐字节一致（防双写漂移）。
func TestSchemaDrift(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "docs", "systems", "00-shared", "schema")
	for _, name := range []string{TrendSeries, TopicCandidate, Alert} {
		embedded, err := schemaFS.ReadFile("schemas/" + name)
		if err != nil {
			t.Fatalf("read embedded %s: %v", name, err)
		}
		upstream, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read docs schema %s（SSOT 缺失？）: %v", name, err)
		}
		if !bytes.Equal(bytes.TrimSpace(embedded), bytes.TrimSpace(upstream)) {
			t.Fatalf("%s 与 docs SSOT 漂移——以 docs 为准重新拷贝", name)
		}
	}
}
