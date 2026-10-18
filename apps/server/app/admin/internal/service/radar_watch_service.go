package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	radarV1 "framework/api/gen/go/radar/v1"
	"framework/app/admin/internal/biz"
	"framework/pkg/middleware/auth"
)

// radarStarSyncStaleness 盯盘星标同步节流：距上次同步小于该值跳过（保护 GitHub 限额）。
const radarStarSyncStaleness = 10 * time.Minute

// radarStarSyncConcurrency 单次列表同步的并发上限。
const radarStarSyncConcurrency = 4

// radarStarEventThreshold 触发盯盘事件的最小新增星标数。
const radarStarEventThreshold int64 = 50

// radarWatchSyncMax 单次列表最多同步多少个盯盘（其余等下次）。
const radarWatchSyncMax = 12

// RadarWatchService 盯盘服务：行级按用户隔离（JWT user_id）。
type RadarWatchService struct {
	uc  *biz.RadarWatchUsecase
	gh  *biz.GitHubClient
	log *log.Helper
}

func NewRadarWatchService(uc *biz.RadarWatchUsecase, gh *biz.GitHubClient, logger log.Logger) *RadarWatchService {
	return &RadarWatchService{
		uc:  uc,
		gh:  gh,
		log: log.NewHelper(log.With(logger, "module", "radar/watch/service")),
	}
}

// currentUserID 从平台 JWT 提取用户ID；缺失返回 Unauthorized。
func currentUserID(ctx context.Context) (uint32, error) {
	payload, err := auth.FromContext(ctx)
	if err != nil || payload == nil {
		return 0, adminV1.ErrorUnauthorized("authentication required")
	}
	return payload.UserId, nil
}

func (s *RadarWatchService) ListWatches(ctx context.Context, _ *radarV1.ListWatchesRequest) (*radarV1.ListWatchesResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	watches, err := s.uc.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.syncStars(ctx, userID, watches)
	// 同步可能产生新事件/未读数，重读一次保证回显一致
	watches, err = s.uc.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &radarV1.ListWatchesResponse{Items: toWatchItems(watches)}, nil
}

// syncStars 并发刷新盯盘仓库星标（节流 + 上限 + 失败不影响列表）。
func (s *RadarWatchService) syncStars(ctx context.Context, userID uint32, watches []*biz.RadarWatch) {
	if s.gh == nil || len(watches) == 0 {
		return
	}
	now := time.Now()
	targets := make([]*biz.RadarWatch, 0, len(watches))
	for _, w := range watches {
		if now.Sub(w.UpdateTime) < radarStarSyncStaleness && w.Stars > 0 {
			continue
		}
		targets = append(targets, w)
		if len(targets) >= radarWatchSyncMax {
			break
		}
	}
	if len(targets) == 0 {
		return
	}
	sem := make(chan struct{}, radarStarSyncConcurrency)
	var wg sync.WaitGroup
	for _, w := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(w *biz.RadarWatch) {
			defer wg.Done()
			defer func() { <-sem }()
			detail, err := s.gh.GetRepoDetail(ctx, w.FullName)
			if err != nil || detail == nil {
				if err != nil {
					s.log.Warnf("sync watch stars failed (user_id=%d, full_name=%s): %v", userID, w.FullName, err)
				}
				return
			}
			sparkline := appendSparklineValue(w.Sparkline, detail.Stars-w.BaselineStars)
			if err := s.uc.SyncStars(ctx, userID, w.FullName, detail.Stars, sparkline, radarStarEventThreshold); err != nil {
				s.log.Warnf("sync watch stars usecase failed (user_id=%d, full_name=%s): %v", userID, w.FullName, err)
			}
		}(w)
	}
	wg.Wait()
}

// appendSparklineValue 追加一个增量点并保持最近 7 个。
func appendSparklineValue(prev []int64, v int64) []int64 {
	next := append(append([]int64{}, prev...), v)
	if len(next) > 7 {
		next = next[len(next)-7:]
	}
	return next
}

func (s *RadarWatchService) AddWatch(ctx context.Context, req *radarV1.AddWatchRequest) (*radarV1.AddWatchResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	fullName := ""
	if req != nil {
		fullName = req.FullName
	}
	item, err := s.uc.Add(ctx, userID, fullName)
	if err != nil {
		return nil, err
	}
	return &radarV1.AddWatchResponse{Item: toWatchItem(item)}, nil
}

func (s *RadarWatchService) RemoveWatch(ctx context.Context, req *radarV1.RemoveWatchRequest) (*radarV1.RemoveWatchResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id == "" {
		return nil, adminV1.ErrorBadRequest("invalid parameter: id is required")
	}
	id, err := strconv.ParseUint(req.Id, 10, 32)
	if err != nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter: id must be numeric")
	}
	ok, err := s.uc.Remove(ctx, userID, uint32(id))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, adminV1.ErrorNotFound("radar watch not found")
	}
	return &radarV1.RemoveWatchResponse{}, nil
}

func (s *RadarWatchService) ListWatchEvents(ctx context.Context, req *radarV1.ListWatchEventsRequest) (*radarV1.ListWatchEventsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	unreadOnly := req != nil && req.UnreadOnly
	events, err := s.uc.ListEvents(ctx, userID, unreadOnly)
	if err != nil {
		return nil, err
	}
	return &radarV1.ListWatchEventsResponse{Items: toWatchEvents(events)}, nil
}

func (s *RadarWatchService) MarkWatchEventsRead(ctx context.Context, req *radarV1.MarkWatchEventsReadRequest) (*radarV1.MarkWatchEventsReadResponse, error) {
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
	if _, err := s.uc.MarkEventsRead(ctx, userID, ids); err != nil {
		return nil, err
	}
	return &radarV1.MarkWatchEventsReadResponse{}, nil
}

// ── proto 映射 ──

func toWatchItems(in []*biz.RadarWatch) []*radarV1.WatchItem {
	out := make([]*radarV1.WatchItem, 0, len(in))
	for _, w := range in {
		out = append(out, toWatchItem(w))
	}
	return out
}

func toWatchItem(w *biz.RadarWatch) *radarV1.WatchItem {
	return &radarV1.WatchItem{
		Id:          strconv.FormatUint(uint64(w.ID), 10),
		FullName:    w.FullName,
		Stars:       w.Stars,
		StarsDelta:  w.StarsDelta,
		UnreadCount: int32(w.UnreadCount),
		Sparkline:   w.Sparkline,
		HtmlUrl:     w.HTMLURL,
	}
}

func toWatchEvents(in []*biz.RadarWatchEvent) []*radarV1.WatchEvent {
	out := make([]*radarV1.WatchEvent, 0, len(in))
	for _, e := range in {
		out = append(out, &radarV1.WatchEvent{
			Id:        strconv.FormatUint(uint64(e.ID), 10),
			FullName:  e.FullName,
			Kind:      e.Kind,
			Message:   e.Message,
			CreatedAt: e.CreateTime.UTC().Format(time.RFC3339),
			Read:      e.Read,
		})
	}
	return out
}
