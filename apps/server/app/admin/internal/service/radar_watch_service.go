package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	radarV1 "framework/api/gen/go/radar/v1"
)

// RadarWatchService in-memory watch list (demo).
// TODO(ent): persist watches / events per user via data layer + platform auth user id.
type RadarWatchService struct {
	log *log.Helper

	mu     sync.RWMutex
	items  map[string]*radarV1.WatchItem
	events []*radarV1.WatchEvent
	seq    int
}

func NewRadarWatchService(logger log.Logger) *RadarWatchService {
	s := &RadarWatchService{
		log:   log.NewHelper(log.With(logger, "module", "radar/watch/service")),
		items: map[string]*radarV1.WatchItem{},
	}
	s.items["w1"] = &radarV1.WatchItem{
		Id: "w1", FullName: "astral-sh/uv", Stars: 45000, StarsDelta: 120,
		Sparkline: []int64{10, 20, 40, 60, 80, 100, 120},
		HtmlUrl:   "https://github.com/astral-sh/uv",
	}
	s.events = []*radarV1.WatchEvent{{
		Id: "e1", FullName: "astral-sh/uv", Kind: "stars",
		Message:   "astral-sh/uv \u661f\u6807 +120\uff08\u8fdc\u8d85\u540c\u76d8\u5747\u7ebf\uff09",
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Read: false,
	}}
	s.seq = 1
	return s
}

func (s *RadarWatchService) ListWatches(_ context.Context, _ *radarV1.ListWatchesRequest) (*radarV1.ListWatchesResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*radarV1.WatchItem, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it)
	}
	return &radarV1.ListWatchesResponse{Items: out}, nil
}

func (s *RadarWatchService) AddWatch(_ context.Context, req *radarV1.AddWatchRequest) (*radarV1.AddWatchResponse, error) {
	if req == nil || req.FullName == "" {
		return nil, fmt.Errorf("full_name required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.items {
		if it.FullName == req.FullName {
			return &radarV1.AddWatchResponse{Item: it}, nil
		}
	}
	s.seq++
	id := fmt.Sprintf("w%d", s.seq)
	item := &radarV1.WatchItem{
		Id: id, FullName: req.FullName, StarsDelta: 0,
		HtmlUrl:   "https://github.com/" + req.FullName,
		Sparkline: []int64{0, 0, 0, 0, 0, 0, 0},
	}
	s.items[id] = item
	return &radarV1.AddWatchResponse{Item: item}, nil
}

func (s *RadarWatchService) RemoveWatch(_ context.Context, req *radarV1.RemoveWatchRequest) (*radarV1.RemoveWatchResponse, error) {
	if req == nil {
		return &radarV1.RemoveWatchResponse{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, req.Id)
	return &radarV1.RemoveWatchResponse{}, nil
}

func (s *RadarWatchService) ListWatchEvents(_ context.Context, req *radarV1.ListWatchEventsRequest) (*radarV1.ListWatchEventsResponse, error) {
	unreadOnly := req != nil && req.UnreadOnly
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*radarV1.WatchEvent, 0, len(s.events))
	for _, e := range s.events {
		if unreadOnly && e.Read {
			continue
		}
		out = append(out, e)
	}
	return &radarV1.ListWatchEventsResponse{Items: out}, nil
}

func (s *RadarWatchService) MarkWatchEventsRead(_ context.Context, req *radarV1.MarkWatchEventsReadRequest) (*radarV1.MarkWatchEventsReadResponse, error) {
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
	for i := range s.events {
		if _, ok := set[s.events[i].Id]; ok || len(ids) == 0 {
			s.events[i].Read = true
		}
	}
	return &radarV1.MarkWatchEventsReadResponse{}, nil
}
