package service

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	radarV1 "framework/api/gen/go/radar/v1"
)

// RadarNotifyService demo notifications for the radar module.
// Distinct from platform NotificationMessageService.
// TODO(ent): persist + join platform user; optional daily digest worker.
type RadarNotifyService struct {
	log   *log.Helper
	mu    sync.RWMutex
	items []*radarV1.RadarNotification
}

func NewRadarNotifyService(logger log.Logger) *RadarNotifyService {
	return &RadarNotifyService{
		log: log.NewHelper(log.With(logger, "module", "radar/notify/service")),
		items: []*radarV1.RadarNotification{{
			Id: "n1", Title: "\u6bcf\u65e5\u70ed\u95e8\u6458\u8981", Body: "\u4eca\u65e5\u699c\u5df2\u66f4\u65b0\uff1b3 \u4e2a\u65b0\u8fdb\u4ed3\u5e93\u503c\u5f97\u5173\u6ce8",
			Kind: "daily_digest", CreatedAt: time.Now().UTC().Format(time.RFC3339), Read: false,
		}},
	}
}

func (s *RadarNotifyService) ListNotifications(_ context.Context, req *radarV1.ListRadarNotificationsRequest) (*radarV1.ListRadarNotificationsResponse, error) {
	unreadOnly := req != nil && req.UnreadOnly
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*radarV1.RadarNotification, 0, len(s.items))
	for _, n := range s.items {
		if unreadOnly && n.Read {
			continue
		}
		out = append(out, n)
	}
	return &radarV1.ListRadarNotificationsResponse{Items: out}, nil
}

func (s *RadarNotifyService) MarkNotificationsRead(_ context.Context, req *radarV1.MarkRadarNotificationsReadRequest) (*radarV1.MarkRadarNotificationsReadResponse, error) {
	ids := []string{}
	if req != nil {
		ids = req.Ids
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	set := map[string]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
	}
	for i := range s.items {
		if _, ok := set[s.items[i].Id]; ok || len(ids) == 0 {
			s.items[i].Read = true
		}
	}
	return &radarV1.MarkRadarNotificationsReadResponse{}, nil
}
