package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "framework/api/gen/go/admin/v1"
	miaV1 "framework/api/gen/go/media/v1/mia"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/pkg/mediacontract"
)

// MiaImpactService 影响卡查询 / 校准 / 传导规则 / 行情只读 REST（FR-MIA-06/07/08/11）。
type MiaImpactService struct {
	impact *bizmia.ImpactUsecase
	events *bizmia.EventUsecase
	rules  *bizmia.RuleUsecase
	log    *log.Helper
}

// NewMiaImpactService 构造。
func NewMiaImpactService(
	impact *bizmia.ImpactUsecase,
	events *bizmia.EventUsecase,
	rules *bizmia.RuleUsecase,
	logger log.Logger,
) *MiaImpactService {
	return &MiaImpactService{
		impact: impact,
		events: events,
		rules:  rules,
		log:    log.NewHelper(log.With(logger, "module", "mia/impact/service")),
	}
}

func (s *MiaImpactService) ListImpactCards(ctx context.Context, req *miaV1.ListImpactCardsRequest) (*miaV1.ListImpactCardsResponse, error) {
	q := bizmia.ImpactQuery{
		Page:           normalizePage(req.GetPage()),
		PageSize:       normalizePageSize(req.GetPageSize()),
		Direction:      req.GetDirection(),
		MinScore:       req.GetMinScore(),
		IncludeExpired: req.GetIncludeExpired(),
	}
	if t, err := parseRFC3339Ptr(req.GetOccurredAfter()); err != nil {
		return nil, adminV1.ErrorBadRequest("occurred_after 格式非法（RFC3339）")
	} else {
		q.OccurredAfter = t
	}

	items, total, err := s.impact.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]*miaV1.ImpactCardListItem, 0, len(items))
	for _, r := range items {
		out = append(out, &miaV1.ImpactCardListItem{
			ImpactId:        r.ImpactID,
			EventId:         r.EventID,
			Title:           r.Title,
			SentimentLabel:  string(r.SentimentLabel),
			SentimentScore:  r.SentimentScore,
			SentimentEngine: string(r.SentimentEngine),
			ImpactScore:     r.ImpactScore,
			Direction:       string(r.Direction),
			TtlHours:        int32(r.TTLHours),
			ExpiresAt:       formatRFC3339OrEmpty(r.ExpiresAt),
			OccurredAt:      formatRFC3339OrEmpty(r.OccurredAt),
			CreatedAt:       formatRFC3339OrEmpty(r.CreatedAt),
			AffectedCount:   int32(len(r.Affected)),
			Pushed:          r.Pushed,
			NoMapping:       r.NoMapping,
		})
	}
	return &miaV1.ListImpactCardsResponse{Items: out, Total: int32(total)}, nil
}

// GetImpactCard 详情：完整契约卡（event.sources 由关联事件卡回填）。
func (s *MiaImpactService) GetImpactCard(ctx context.Context, req *miaV1.GetImpactCardRequest) (*miaV1.GetImpactCardResponse, error) {
	row, err := s.impact.Get(ctx, req.GetImpactId())
	if err != nil {
		return nil, err
	}
	card := row.ToContract()

	// 事件卡回填 sources（事件已被合并删除时容忍为空——影响卡自含标题/路径）。
	if s.events != nil && row.EventID != "" {
		if ev, err := s.events.Get(ctx, row.EventID); err == nil && ev != nil {
			card.Event.Sources = ev.SourceIntelIDs
		}
	}

	return &miaV1.GetImpactCardResponse{
		Card:        toProtoImpactCard(card),
		ImpactId:    row.ImpactID,
		EventId:     row.EventID,
		ExpiresAt:   formatRFC3339OrEmpty(row.ExpiresAt),
		CreatedAt:   formatRFC3339OrEmpty(row.CreatedAt),
		Pushed:      row.Pushed,
		NoMapping:   row.NoMapping,
		RuleVersion: row.RuleVersion,
	}, nil
}

// GetCalibration 校准报告（FR-MIA-11；verify 未启用返回显式空报告 + 固定提示）。
func (s *MiaImpactService) GetCalibration(ctx context.Context, req *miaV1.GetCalibrationRequest) (*miaV1.CalibrationReport, error) {
	until := time.Now().UTC()
	since := until.Add(-90 * 24 * time.Hour)
	if t, err := parseDateOrRFC3339(req.GetSince()); err != nil {
		return nil, adminV1.ErrorBadRequest("since 格式非法（RFC3339 或 YYYY-MM-DD）")
	} else if t != nil {
		since = *t
	}
	if t, err := parseDateOrRFC3339(req.GetUntil()); err != nil {
		return nil, adminV1.ErrorBadRequest("until 格式非法（RFC3339 或 YYYY-MM-DD）")
	} else if t != nil {
		until = *t
	}

	report, err := s.impact.GetCalibration(ctx, since, until)
	if err != nil {
		return nil, err
	}
	buckets := make([]*miaV1.CalibrationBucket, 0, len(report.Buckets))
	for _, b := range report.Buckets {
		buckets = append(buckets, &miaV1.CalibrationBucket{
			RuleId:  b.RuleID,
			Samples: int32(b.Samples),
			HitRate: b.HitRate,
		})
	}
	return &miaV1.CalibrationReport{
		Buckets:      buckets,
		TotalSamples: int32(report.TotalSamples),
		Window:       report.Window,
		Note:         report.Note,
		Enabled:      report.Enabled,
	}, nil
}

func (s *MiaImpactService) ListTransmissionRules(ctx context.Context, req *miaV1.ListTransmissionRulesRequest) (*miaV1.ListTransmissionRulesResponse, error) {
	rules, err := s.rules.List(ctx, req.GetEnabledOnly())
	if err != nil {
		return nil, err
	}
	out := make([]*miaV1.TransmissionRuleItem, 0, len(rules))
	for _, r := range rules {
		out = append(out, &miaV1.TransmissionRuleItem{
			Id: r.ID,
			Rule: &miaV1.TransmissionRule{
				RuleId: r.RuleID,
				Trigger: &miaV1.TriggerSpec{
					Asset:      r.Trigger.Asset,
					Direction:  string(r.Trigger.Direction),
					MinMovePct: r.Trigger.MinMovePct,
				},
				ToAsset:     r.ToAsset,
				ToDirection: string(r.ToDirection),
				Confidence:  r.Confidence,
				Note:        r.Note,
			},
			Enabled:      r.Enabled,
			HitCount:     int32(r.HitCount),
			CorrectCount: int32(r.CorrectCount),
		})
	}
	return &miaV1.ListTransmissionRulesResponse{Items: out}, nil
}

// GetQuote 行情只读查询（FR-MIA-07；只读端口，无交易通路）。
func (s *MiaImpactService) GetQuote(ctx context.Context, req *miaV1.GetQuoteRequest) (*miaV1.GetQuoteResponse, error) {
	snap, err := s.impact.GetQuote(ctx, req.GetSymbol())
	if err != nil {
		return nil, err
	}
	bars := make([]*miaV1.OhlcvBar, 0, len(snap.Bars))
	for _, b := range snap.Bars {
		bars = append(bars, &miaV1.OhlcvBar{
			T:      b.T.UTC().Format(time.RFC3339),
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: b.Volume,
		})
	}
	return &miaV1.GetQuoteResponse{
		Symbol:    snap.Symbol,
		Provider:  snap.Provider,
		Bars:      bars,
		FetchedAt: formatRFC3339OrEmpty(snap.FetchedAt),
		FromCache: snap.FromCache,
	}, nil
}

// toProtoImpactCard 契约卡 → proto 卡。
func toProtoImpactCard(c *mediacontract.ImpactCard) *miaV1.ImpactCard {
	affected := make([]*miaV1.AffectedItem, 0, len(c.Affected))
	for _, a := range c.Affected {
		affected = append(affected, &miaV1.AffectedItem{
			Symbol:         a.Symbol,
			AssetClass:     string(a.AssetClass),
			Direction:      string(a.Direction),
			Confidence:     a.Confidence,
			Path:           a.Path,
			RuleId:         a.RuleID,
			IsTransmission: a.IsTransmission,
		})
	}
	var occurredAt *timestamppb.Timestamp
	if !c.Event.OccurredAt.IsZero() {
		occurredAt = timestamppb.New(c.Event.OccurredAt.UTC())
	}
	// verified.hit=null 表示未校验——P2 校验落地前不序列化该块（不得默认解释为「不准」）。
	return &miaV1.ImpactCard{
		ApiVersion: c.APIVersion,
		ImpactId:   c.ImpactID,
		TopicId:    c.TopicID,
		Event: &miaV1.ImpactEvent{
			Title:      c.Event.Title,
			Summary:    c.Event.Summary,
			Sources:    c.Event.Sources,
			OccurredAt: occurredAt,
		},
		Sentiment: &miaV1.Sentiment{
			Label:  string(c.Sentiment.Label),
			Score:  c.Sentiment.Score,
			Engine: string(c.Sentiment.Engine),
			Lang:   c.Sentiment.Lang,
		},
		Impact: &miaV1.Impact{
			Score:    c.Impact.Score,
			TtlHours: int32(c.Impact.TTLHours),
			Factors: &miaV1.ImpactFactors{
				Sentiment:    c.Impact.Factors.Sentiment,
				SourceWeight: c.Impact.Factors.SourceWeight,
				Relevance:    c.Impact.Factors.Relevance,
				Decay:        c.Impact.Factors.Decay,
			},
		},
		Affected:   affected,
		Disclaimer: c.Disclaimer,
	}
}

// parseDateOrRFC3339 接受 YYYY-MM-DD 或 RFC3339；空串返回 (nil, nil)。
func parseDateOrRFC3339(v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		t = t.UTC()
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}
