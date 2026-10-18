package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	miaV1 "framework/api/gen/go/media/v1/mia"
	bizmia "framework/app/admin/internal/biz/mia"
)

// MiaEventService 事件卡查询与人工聚类干预 REST（FR-MIA-05）。
type MiaEventService struct {
	uc  *bizmia.EventUsecase
	log *log.Helper
}

// NewMiaEventService 构造。
func NewMiaEventService(uc *bizmia.EventUsecase, logger log.Logger) *MiaEventService {
	return &MiaEventService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "mia/event/service")),
	}
}

func (s *MiaEventService) ListEventCards(ctx context.Context, req *miaV1.ListEventCardsRequest) (*miaV1.ListEventCardsResponse, error) {
	q := bizmia.EventQuery{
		Page:     normalizePage(req.GetPage()),
		PageSize: normalizePageSize(req.GetPageSize()),
		Category: req.GetCategory(),
		Keyword:  req.GetKeyword(),
	}
	if t, err := parseRFC3339Ptr(req.GetOccurredAfter()); err != nil {
		return nil, adminV1.ErrorBadRequest("occurred_after 格式非法（RFC3339）")
	} else {
		q.OccurredAfter = t
	}
	if t, err := parseRFC3339Ptr(req.GetOccurredBefore()); err != nil {
		return nil, adminV1.ErrorBadRequest("occurred_before 格式非法（RFC3339）")
	} else {
		q.OccurredBefore = t
	}

	items, total, err := s.uc.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]*miaV1.EventCardItem, 0, len(items))
	for _, it := range items {
		out = append(out, toProtoEventCard(it))
	}
	return &miaV1.ListEventCardsResponse{Items: out, Total: int32(total)}, nil
}

func (s *MiaEventService) GetEventCard(ctx context.Context, req *miaV1.GetEventCardRequest) (*miaV1.EventCardItem, error) {
	item, err := s.uc.Get(ctx, req.GetEventId())
	if err != nil {
		return nil, err
	}
	return toProtoEventCard(item), nil
}

// SplitEventCard 人工拆分（锁定两卡；自动聚类此后不再吞并）。
func (s *MiaEventService) SplitEventCard(ctx context.Context, req *miaV1.SplitEventCardRequest) (*miaV1.SplitEventCardResponse, error) {
	if len(req.GetSourceIntelIds()) == 0 {
		return nil, adminV1.ErrorBadRequest("source_intel_ids 至少 1 条")
	}
	parent, newCard, err := s.uc.Split(ctx, req.GetEventId(), req.GetSourceIntelIds())
	if err != nil {
		return nil, err
	}
	return &miaV1.SplitEventCardResponse{
		NewItem:    toProtoEventCard(newCard),
		ParentItem: toProtoEventCard(parent),
	}, nil
}

// MergeEventCards 人工合并（第一张为目标卡）。
func (s *MiaEventService) MergeEventCards(ctx context.Context, req *miaV1.MergeEventCardsRequest) (*miaV1.MergeEventCardsResponse, error) {
	if len(req.GetEventIds()) < 2 {
		return nil, adminV1.ErrorBadRequest("合并至少需要 2 张事件卡")
	}
	merged, err := s.uc.Merge(ctx, req.GetEventIds())
	if err != nil {
		return nil, err
	}
	return &miaV1.MergeEventCardsResponse{MergedItem: toProtoEventCard(merged)}, nil
}

func toProtoEventCard(r *bizmia.EventCardRow) *miaV1.EventCardItem {
	return &miaV1.EventCardItem{
		Id:             r.ID,
		EventId:        r.EventID,
		Title:          r.Title,
		Summary:        r.Summary,
		OccurredAt:     formatRFC3339OrEmpty(r.OccurredAt),
		TopicId:        r.TopicID,
		SourceIntelIds: r.SourceIntelIDs,
		SourceCount:    int32(r.SourceCount),
		Domains:        r.Domains,
		Category:       r.Category,
		Lang:           r.Lang,
		MergedBy:       r.MergedBy,
		SplitLocked:    r.SplitLocked,
	}
}

// parseRFC3339Ptr 可选时间参数解析；空串返回 (nil, nil)。
func parseRFC3339Ptr(v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}
