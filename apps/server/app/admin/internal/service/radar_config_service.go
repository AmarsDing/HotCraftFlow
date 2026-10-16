package service

import (
	"context"
	"strings"
	"sync"

	"github.com/go-kratos/kratos/v2/log"

	radarV1 "framework/api/gen/go/radar/v1"
)

// RadarConfigService stores per-user GitHub token mask (demo in-memory).
// MUST be behind platform JWT; do not expose unauthenticated.
// TODO(ent): encrypt token at rest; never return plaintext.
type RadarConfigService struct {
	log   *log.Helper
	mu    sync.RWMutex
	token string // demo only
}

func NewRadarConfigService(logger log.Logger) *RadarConfigService {
	return &RadarConfigService{
		log: log.NewHelper(log.With(logger, "module", "radar/config/service")),
	}
}

func (s *RadarConfigService) GetConfig(_ context.Context, _ *radarV1.GetRadarConfigRequest) (*radarV1.GetRadarConfigResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &radarV1.GetRadarConfigResponse{
		HasGithubToken:    s.token != "",
		GithubTokenMasked: maskToken(s.token),
	}, nil
}

func (s *RadarConfigService) SetConfig(_ context.Context, req *radarV1.SetRadarConfigRequest) (*radarV1.SetRadarConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req != nil {
		s.token = strings.TrimSpace(req.GithubToken)
	}
	return &radarV1.SetRadarConfigResponse{
		HasGithubToken:    s.token != "",
		GithubTokenMasked: maskToken(s.token),
	}, nil
}

func maskToken(t string) string {
	if t == "" {
		return ""
	}
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "\u2026" + t[len(t)-4:]
}
