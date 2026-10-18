// Package trendproto ② TT 三份契约（TrendSeries / TopicCandidate / Alert）
// 与 api/media/v1/tt proto 消息之间的双向映射（评审 B-2 / M1「往返测试」）。
//
// 往返无损口径：契约 → Proto → 契约字段恒等（RejectReason nil 与 "" 视为同一
// 值：proto3 无可空 string，空串无业务意义）。枚举以 string 原样过，合法性由
// mediacontract.Validate() 与 schemagate 各管一侧。
package trendproto

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	ttv1 "framework/api/gen/go/media/v1/tt"
	"framework/pkg/mediacontract"
)

// ---------------------------------------------------------------- TrendSeries

// TrendSeriesToProto 契约 → proto。
func TrendSeriesToProto(x *mediacontract.TrendSeries) *ttv1.TrendSeries {
	if x == nil {
		return nil
	}
	out := &ttv1.TrendSeries{
		ApiVersion: x.APIVersion,
		TopicId:    x.TopicID,
		Label:      x.Label,
		Category:   string(x.Category),
		Stage:      string(x.Stage),
		StageEvidence: &ttv1.StageEvidence{
			Velocity:       x.StageEvidence.Velocity,
			Accel:          x.StageEvidence.Accel,
			DurationPoints: x.StageEvidence.DurationPoints,
		},
		Heat: heatToProto(&x.Heat),
		Burst: &ttv1.Burst{
			IsBurst:    x.Burst.IsBurst,
			MethodsHit: burstMethodsToProto(x.Burst.MethodsHit),
			Evidence:   cloneEvidence(x.Burst.Evidence),
		},
		Origin: &ttv1.Origin{
			FirstSeenAt:   x.Origin.FirstSeenAt,
			FirstPlatform: x.Origin.FirstPlatform,
			FirstUrl:      x.Origin.FirstURL,
		},
	}
	if len(x.Series) > 0 {
		out.Series = make([]*ttv1.SeriesPoint, 0, len(x.Series))
		for _, p := range x.Series {
			out.Series = append(out.Series, &ttv1.SeriesPoint{
				T: p.T, Platform: p.Platform, Rank: p.Rank, Heat: p.Heat,
			})
		}
	}
	if x.Diagnostics != nil {
		out.Diagnostics = &ttv1.Diagnostics{
			InsufficientPoints: x.Diagnostics.InsufficientPoints,
			NPoints:            x.Diagnostics.NPoints,
		}
	}
	return out
}

// TrendSeriesFromProto proto → 契约。
func TrendSeriesFromProto(m *ttv1.TrendSeries) *mediacontract.TrendSeries {
	if m == nil {
		return nil
	}
	out := &mediacontract.TrendSeries{
		APIVersion: m.ApiVersion,
		TopicID:    m.TopicId,
		Label:      m.Label,
		Category:   mediacontract.Category(m.Category),
		Stage:      mediacontract.Stage(m.Stage),
	}
	if se := m.StageEvidence; se != nil {
		out.StageEvidence = mediacontract.StageEvidence{
			Velocity:       se.Velocity,
			Accel:          se.Accel,
			DurationPoints: se.DurationPoints,
		}
	}
	if h := m.Heat; h != nil {
		out.Heat = heatFromProto(h)
	}
	if len(m.Series) > 0 {
		out.Series = make([]mediacontract.SeriesPoint, 0, len(m.Series))
		for _, p := range m.Series {
			if p == nil {
				continue
			}
			out.Series = append(out.Series, mediacontract.SeriesPoint{
				T: p.T, Platform: p.Platform, Rank: p.Rank, Heat: p.Heat,
			})
		}
	}
	if b := m.Burst; b != nil {
		out.Burst = mediacontract.Burst{
			IsBurst:    b.IsBurst,
			MethodsHit: burstMethodsFromProto(b.MethodsHit),
			Evidence:   cloneEvidence(b.Evidence),
		}
	}
	if o := m.Origin; o != nil {
		out.Origin = mediacontract.Origin{
			FirstSeenAt:   o.FirstSeenAt,
			FirstPlatform: o.FirstPlatform,
			FirstURL:      o.FirstUrl,
		}
	}
	if d := m.Diagnostics; d != nil {
		out.Diagnostics = &mediacontract.Diagnostics{
			InsufficientPoints: d.InsufficientPoints,
			NPoints:            d.NPoints,
		}
	}
	return out
}

func heatToProto(h *mediacontract.Heat) *ttv1.Heat {
	return &ttv1.Heat{
		Score: h.Score, RankPct: h.RankPct, EngPct: h.EngPct, EngRate: h.EngRate,
		Freshness: h.Freshness, Velocity: h.Velocity, Accel: h.Accel,
		Breadth: &ttv1.Breadth{
			PlatformCount: h.Breadth.PlatformCount,
			Platforms:     append([]string(nil), h.Breadth.Platforms...),
		},
		Factors: &ttv1.HeatFactors{
			WRank: h.Factors.WRank, WEng: h.Factors.WEng,
			WFresh: h.Factors.WFresh, WBreadth: h.Factors.WBreadth,
			WRankEff: h.Factors.WRankEff, WEngEff: h.Factors.WEngEff,
			WFreshEff: h.Factors.WFreshEff, WBreadthEff: h.Factors.WBreadthEff,
			Missing: append([]string(nil), h.Factors.Missing...),
		},
	}
}

func heatFromProto(h *ttv1.Heat) mediacontract.Heat {
	out := mediacontract.Heat{
		Score: h.Score, RankPct: h.RankPct, EngPct: h.EngPct, EngRate: h.EngRate,
		Freshness: h.Freshness, Velocity: h.Velocity, Accel: h.Accel,
	}
	if b := h.Breadth; b != nil {
		out.Breadth = mediacontract.Breadth{
			PlatformCount: b.PlatformCount,
			Platforms:     append([]string(nil), b.Platforms...),
		}
	}
	if f := h.Factors; f != nil {
		out.Factors = mediacontract.HeatFactors{
			WRank: f.WRank, WEng: f.WEng, WFresh: f.WFresh, WBreadth: f.WBreadth,
			WRankEff: f.WRankEff, WEngEff: f.WEngEff,
			WFreshEff: f.WFreshEff, WBreadthEff: f.WBreadthEff,
			Missing: append([]string(nil), f.Missing...),
		}
	}
	return out
}

// ------------------------------------------------------------- TopicCandidate

// TopicCandidateToProto 契约 → proto。
func TopicCandidateToProto(x *mediacontract.TopicCandidate) *ttv1.TopicCandidate {
	if x == nil {
		return nil
	}
	out := &ttv1.TopicCandidate{
		ApiVersion: x.APIVersion,
		CandidateId: x.CandidateID,
		TopicId:     x.TopicID,
		Route:       string(x.Route),
		Score: &ttv1.CandidateScore{
			Hot: x.Score.Hot, Freshness: x.Score.Freshness,
			Workability: x.Score.Workability, Compliance: x.Score.Compliance,
			Total: x.Score.Total,
		},
		SuggestedAngles: append([]string(nil), x.SuggestedAngles...),
		MaterialRefs:    append([]string(nil), x.MaterialRefs...),
		State:           string(x.State),
	}
	if x.RejectReason != nil {
		out.RejectReason = *x.RejectReason
	}
	return out
}

// TopicCandidateFromProto proto → 契约。reject_at 无契约对应字段（契约 §6 未定
// 义，proto 侧保留扩展位），往返不经过它。
func TopicCandidateFromProto(m *ttv1.TopicCandidate) *mediacontract.TopicCandidate {
	if m == nil {
		return nil
	}
	out := &mediacontract.TopicCandidate{
		APIVersion:  m.ApiVersion,
		CandidateID: m.CandidateId,
		TopicID:     m.TopicId,
		Route:       mediacontract.CandidateRoute(m.Route),
		State:       mediacontract.CandidateState(m.State),
	}
	if m.Score != nil {
		out.Score = mediacontract.CandidateScore{
			Hot: m.Score.Hot, Freshness: m.Score.Freshness,
			Workability: m.Score.Workability, Compliance: m.Score.Compliance,
			Total: m.Score.Total,
		}
	}
	out.SuggestedAngles = append([]string(nil), m.SuggestedAngles...)
	out.MaterialRefs = append([]string(nil), m.MaterialRefs...)
	if m.RejectReason != "" {
		r := m.RejectReason
		out.RejectReason = &r
	}
	return out
}

// ---------------------------------------------------------------------- Alert

// AlertToProto 契约 → proto（triggered_at 恒 UTC）。
func AlertToProto(x *mediacontract.Alert) *ttv1.Alert {
	if x == nil {
		return nil
	}
	return &ttv1.Alert{
		ApiVersion:  x.APIVersion,
		AlertId:     x.AlertID,
		TopicId:     x.TopicID,
		TriggeredAt: timestamppb.New(x.TriggeredAt.UTC()),
		MethodsHit:  burstMethodsToProto(x.MethodsHit),
		Evidence:    cloneEvidence(x.Evidence),
		Channels:    append([]string(nil), x.Channels...),
		State:       string(x.State),
	}
}

// AlertFromProto proto → 契约（timestamppb.AsTime 恒返 UTC 位置）。
func AlertFromProto(m *ttv1.Alert) *mediacontract.Alert {
	if m == nil {
		return nil
	}
	out := &mediacontract.Alert{
		APIVersion: m.ApiVersion,
		AlertID:    m.AlertId,
		TopicID:    m.TopicId,
		Evidence:   cloneEvidence(m.Evidence),
		Channels:   append([]string(nil), m.Channels...),
		State:      mediacontract.AlertState(m.State),
	}
	if m.TriggeredAt != nil {
		out.TriggeredAt = m.TriggeredAt.AsTime()
	}
	out.MethodsHit = burstMethodsFromProto(m.MethodsHit)
	return out
}

// ------------------------------------------------------------------- helpers

func burstMethodsToProto(ms []mediacontract.BurstMethod) []string {
	if len(ms) == 0 {
		return nil
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, string(m))
	}
	return out
}

func burstMethodsFromProto(ms []string) []mediacontract.BurstMethod {
	if len(ms) == 0 {
		return nil
	}
	out := make([]mediacontract.BurstMethod, 0, len(ms))
	for _, m := range ms {
		out = append(out, mediacontract.BurstMethod(m))
	}
	return out
}

func cloneEvidence(m map[string]float64) map[string]float64 {
	if m == nil {
		return nil
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
