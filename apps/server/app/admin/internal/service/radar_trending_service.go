package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	radarV1 "framework/api/gen/go/radar/v1"
	"framework/app/admin/internal/biz"
)

// RadarTrendingService implements radar.service.v1.TrendingService HTTP/gRPC.
type RadarTrendingService struct {
	uc  *biz.RadarTrendingUsecase
	log *log.Helper
}

func NewRadarTrendingService(uc *biz.RadarTrendingUsecase, logger log.Logger) *RadarTrendingService {
	return &RadarTrendingService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "radar/trending/service")),
	}
}

func (s *RadarTrendingService) ListTrending(ctx context.Context, req *radarV1.ListTrendingRequest) (*radarV1.ListTrendingResponse, error) {
	if req == nil {
		req = &radarV1.ListTrendingRequest{}
	}
	res, err := s.uc.List(ctx, biz.RadarListQuery{
		Since: req.Since, Language: req.Language, Topic: req.Topic,
		OnlyNew: req.OnlyNew, Page: int(req.Page), PageSize: int(req.PageSize),
	})
	if err != nil {
		return nil, err
	}
	return &radarV1.ListTrendingResponse{
		Items:       toRepoItems(res.Items),
		Stale:       res.Stale,
		StaleReason: res.StaleReason,
		RefreshedAt: res.RefreshedAt,
		LanguageTop: res.LanguageTop,
		RisingCount: res.RisingCount,
		NewCount:    res.NewCount,
	}, nil
}

func (s *RadarTrendingService) SearchRising(ctx context.Context, req *radarV1.SearchRisingRequest) (*radarV1.SearchRisingResponse, error) {
	if req == nil {
		req = &radarV1.SearchRisingRequest{}
	}
	res, err := s.uc.Rising(ctx, biz.RadarRisingQuery{
		Query: req.Query, Language: req.Language,
		Page: int(req.Page), PageSize: int(req.PageSize),
	})
	if err != nil {
		return nil, err
	}
	return &radarV1.SearchRisingResponse{
		Items: toRepoItems(res.Items), Stale: res.Stale,
		StaleReason: res.StaleReason, RefreshedAt: res.RefreshedAt,
	}, nil
}

func toRepoItems(in []biz.RadarRepoView) []*radarV1.RepoItem {
	out := make([]*radarV1.RepoItem, 0, len(in))
	for _, v := range in {
		out = append(out, &radarV1.RepoItem{
			Id: v.ID, FullName: v.FullName, Description: v.Description,
			Language: v.Language, Topics: v.Topics, Stars: v.Stars, Forks: v.Forks,
			Rank: v.Rank, StarsDelta: v.StarsDelta, IsNew: v.IsNew,
			HtmlUrl: v.HTMLURL, Sparkline: v.Sparkline,
		})
	}
	return out
}
