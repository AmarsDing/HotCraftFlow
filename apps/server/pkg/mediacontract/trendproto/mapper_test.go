package trendproto

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	ttv1 "framework/api/gen/go/media/v1/tt"
	"framework/pkg/mediacontract"
)

// ============================================================================
// M1 第 2 项：契约 → Proto → 契约 往返无损测试。
// 走真实 protobuf wire 序列化（不是纯函数对转），防 json_name / 字段号错位。
// ============================================================================

func wireRoundTrip[M proto.Message](t *testing.T, in M) M {
	t.Helper()
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatalf("proto marshal: %v", err)
	}
	out := in.ProtoReflect().New().Interface().(M)
	if err := proto.Unmarshal(b, out); err != nil {
		t.Fatalf("proto unmarshal: %v", err)
	}
	return out
}

func TestTrendSeriesRoundTrip(t *testing.T) {
	src := &mediacontract.TrendSeries{
		APIVersion: "1.0",
		TopicID:    mediacontract.NewID("tp"),
		Label:      "往返话题",
		Category:   mediacontract.CategoryAHot,
		Stage:      mediacontract.StagePeak,
		StageEvidence: mediacontract.StageEvidence{
			Velocity: 12.5, Accel: -0.4, DurationPoints: 17,
		},
		Heat: mediacontract.Heat{
			Score: 88.25, RankPct: 0.95, EngPct: 0.87, EngRate: 0.042,
			Freshness: 0.66, Velocity: 12.5, Accel: -0.4,
			Breadth: mediacontract.Breadth{PlatformCount: 3, Platforms: []string{"weibo", "bilibili", "douyin"}},
			Factors: mediacontract.HeatFactors{
				WRank: 0.4, WEng: 0.3, WFresh: 0.2, WBreadth: 0.1,
				WRankEff: 0.5, WEngEff: 0.0, WFreshEff: 0.3, WBreadthEff: 0.2,
				Missing: []string{"eng"},
			},
		},
		Series: []mediacontract.SeriesPoint{
			{T: "2026-10-18T09:30:00Z", Platform: "weibo", Rank: 5, Heat: 70.5},
			{T: "2026-10-18T10:00:00Z", Platform: "bilibili", Rank: 2, Heat: 88.25},
		},
		Burst: mediacontract.Burst{
			IsBurst:    true,
			MethodsHit: []mediacontract.BurstMethod{mediacontract.BurstZScore, mediacontract.BurstFirstXPlat},
			Evidence:   map[string]float64{"zscore": 3.75, "first_cross_platform": 2},
		},
		Origin: mediacontract.Origin{
			FirstSeenAt:   "2026-10-18T08:00:00Z",
			FirstPlatform: "weibo",
			FirstURL:      "https://example.com/p/99",
		},
		Diagnostics: &mediacontract.Diagnostics{InsufficientPoints: true, NPoints: 3},
	}

	back := TrendSeriesFromProto(wireRoundTrip(t, TrendSeriesToProto(src)))
	if !reflect.DeepEqual(src, back) {
		t.Fatalf("TrendSeries roundtrip drift:\nsrc=%+v\nback=%+v", src, back)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("roundtrip result invalid: %v", err)
	}
}

func TestTopicCandidateRoundTrip(t *testing.T) {
	reason := "证据不足"
	src := &mediacontract.TopicCandidate{
		APIVersion:      "1.0",
		CandidateID:     mediacontract.NewID("tc"),
		TopicID:         mediacontract.NewID("tp"),
		Route:           mediacontract.RouteBoth,
		Score:           mediacontract.CandidateScore{Hot: 0.9, Freshness: 0.8, Workability: 0.7, Compliance: 1.0, Total: 84.5},
		SuggestedAngles: []string{"科普", "剧情二创"},
		MaterialRefs:    []string{"asset://a1"},
		State:           mediacontract.CandidateRejected,
		RejectReason:    &reason,
	}

	back := TopicCandidateFromProto(wireRoundTrip(t, TopicCandidateToProto(src)))
	if !reflect.DeepEqual(src, back) {
		t.Fatalf("TopicCandidate roundtrip drift:\nsrc=%+v\nback=%+v", src, back)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("roundtrip result invalid: %v", err)
	}

	// nil RejectReason ↔ 空串约定（mapper 头注释的无损口径）。
	nilSrc := &mediacontract.TopicCandidate{
		APIVersion:      "1.0",
		CandidateID:     mediacontract.NewID("tc"),
		TopicID:         mediacontract.NewID("tp"),
		Route:           mediacontract.RouteMIA,
		Score:           mediacontract.CandidateScore{Hot: 0.5, Freshness: 0.5, Workability: 0.5, Compliance: 0.5, Total: 50},
		SuggestedAngles: []string{},
		State:           mediacontract.CandidatePending,
	}
	nilBack := TopicCandidateFromProto(wireRoundTrip(t, TopicCandidateToProto(nilSrc)))
	if nilBack.RejectReason != nil {
		t.Fatalf("empty reject_reason must round-trip to nil, got %q", *nilBack.RejectReason)
	}
}

func TestAlertRoundTrip(t *testing.T) {
	src := &mediacontract.Alert{
		APIVersion:  "1.0",
		AlertID:     mediacontract.NewID("al"),
		TopicID:     mediacontract.NewID("tp"),
		TriggeredAt: time.Date(2026, 10, 18, 10, 5, 30, 123456789, time.UTC),
		MethodsHit:  []mediacontract.BurstMethod{mediacontract.BurstZScore, mediacontract.BurstEngSpike},
		Evidence:    map[string]float64{"zscore": 4.2, "eng_spike": 7.7},
		Channels:    []string{"webhook", "wecom"},
		State:       mediacontract.AlertNew,
	}

	back := AlertFromProto(wireRoundTrip(t, AlertToProto(src)))
	if !reflect.DeepEqual(src, back) {
		t.Fatalf("Alert roundtrip drift:\nsrc=%+v\nback=%+v", src, back)
	}
	if err := back.Validate(); err != nil {
		t.Fatalf("roundtrip result invalid: %v", err)
	}
	if back.TriggeredAt.Location() != time.UTC {
		t.Fatalf("triggered_at must stay UTC, got %v", back.TriggeredAt.Location())
	}
}

// 保护 mapper 覆盖率的元测试：proto 新加字段而 mapper 未跟进会在这里爆红。
func TestProtoProjectionComplete(t *testing.T) {
	want := []string{"api_version", "topic_id", "label", "category", "stage",
		"stage_evidence", "heat", "series", "burst", "origin", "diagnostics"}
	got := (&ttv1.TrendSeries{}).ProtoReflect().Descriptor().Fields()
	if got.Len() != len(want) {
		names := make([]string, 0, got.Len())
		for i := 0; i < got.Len(); i++ {
			names = append(names, string(got.Get(i).JSONName()))
		}
		t.Fatalf("TrendSeries proto fields changed: %v（mapper 需同步）", names)
	}
}
