package service

import (
	"context"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	radarV1 "framework/api/gen/go/radar/v1"
	"framework/app/admin/internal/biz"
)

// RadarNotifyService 雷达通知服务：通知=当前用户的盯盘事件视图。
// 与平台 NotificationMessageService 的分工：平台通知走站内信/推送体系；
// 雷达通知只表达"我盯的仓库有变化"，统一由盯盘事件驱动，不再独立存储（消除重复造轮子）。
type RadarNotifyService struct {
	uc  *biz.RadarNotifyUsecase
	log *log.Helper
}

func NewRadarNotifyService(uc *biz.RadarNotifyUsecase, logger log.Logger) *RadarNotifyService {
	return &RadarNotifyService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "radar/notify/service")),
	}
}

func (s *RadarNotifyService) ListNotifications(ctx context.Context, req *radarV1.ListRadarNotificationsRequest) (*radarV1.ListRadarNotificationsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	unreadOnly := req != nil && req.UnreadOnly
	events, err := s.uc.ListNotifications(ctx, userID, unreadOnly)
	if err != nil {
		return nil, err
	}
	return &radarV1.ListRadarNotificationsResponse{Items: toRadarNotifications(events)}, nil
}

func (s *RadarNotifyService) MarkNotificationsRead(ctx context.Context, req *radarV1.MarkRadarNotificationsReadRequest) (*radarV1.MarkRadarNotificationsReadResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]uint32, 0, 4)
	if req != nil {
		for _, raw := range req.Ids {
			id, err := strconv.ParseUint(raw, 10, 32)
			if err != nil {
				return nil, adminV1.ErrorBadRequest("invalid parameter: ids must be numeric")
			}
			ids = append(ids, uint32(id))
		}
	}
	if _, err := s.uc.MarkNotificationsRead(ctx, userID, ids); err != nil {
		return nil, err
	}
	return &radarV1.MarkRadarNotificationsReadResponse{}, nil
}

// toRadarNotifications 盯盘事件 → 通知视图（title=仓库，body=事件文案）。
func toRadarNotifications(in []*biz.RadarWatchEvent) []*radarV1.RadarNotification {
	out := make([]*radarV1.RadarNotification, 0, len(in))
	for _, e := range in {
		kind := e.Kind
		if kind == "" {
			kind = "stars"
		}
		out = append(out, &radarV1.RadarNotification{
			Id:        strconv.FormatUint(uint64(e.ID), 10),
			Title:     e.FullName,
			Body:      e.Message,
			Kind:      kind,
			CreatedAt: e.CreateTime.UTC().Format(time.RFC3339),
			Read:      e.Read,
		})
	}
	return out
}
