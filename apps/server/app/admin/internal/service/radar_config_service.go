package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	radarV1 "framework/api/gen/go/radar/v1"
	"framework/app/admin/internal/biz"
	"framework/pkg/middleware/auth"

	userV1 "framework/api/gen/go/user/v1"
)

// radarConfigCacheTTL 平台配置快照缓存（避免每次 GitHub 调用都读库）。
const radarConfigCacheTTL = 30 * time.Second

// RadarConfigService 雷达平台级配置服务（GitHub Token / LLM / 克隆）。
//   - 存储：平台级单行，密钥 AES-256-GCM 加密落库（见 RadarUserConfigRepo）；
//   - 读取：任何登录用户可读（脱敏回显）；
//   - 写入：仅系统/租户管理员（修复多租户 token 互相覆盖与泄露）。
//
// 同时实现 biz.RadarConfigProvider，向 GitHub 客户端 / Insight 服务提供配置。
type RadarConfigService struct {
	uc  *biz.RadarConfigUsecase
	log *log.Helper

	mu          sync.RWMutex
	cachedAt    time.Time
	cachedToken string
	cachedLLM   biz.RadarLLMConfig
	cachedClone struct {
		enabled bool
		dir     string
	}
}

func NewRadarConfigService(uc *biz.RadarConfigUsecase, logger log.Logger) *RadarConfigService {
	return &RadarConfigService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "radar/config/service")),
	}
}

func (s *RadarConfigService) GetConfig(ctx context.Context, _ *radarV1.GetRadarConfigRequest) (*radarV1.GetRadarConfigResponse, error) {
	if _, err := currentUserID(ctx); err != nil {
		return nil, err
	}
	cfg, err := s.uc.Get(ctx)
	if err != nil {
		return nil, err
	}
	s.invalidateCache()
	return &radarV1.GetRadarConfigResponse{
		HasGithubToken:    cfg.GitHubToken != "",
		GithubTokenMasked: maskToken(cfg.GitHubToken),
		LlmEnabled:        cfg.LLMEnabled,
		LlmBaseUrl:        cfg.LLMBaseURL,
		LlmModel:          cfg.LLMModel,
		LlmApiKeyMasked:   maskToken(cfg.LLMAPIKey),
		CloneEnabled:      cfg.CloneEnabled,
		CloneDir:          cfg.CloneDir,
	}, nil
}

func (s *RadarConfigService) SetConfig(ctx context.Context, req *radarV1.SetRadarConfigRequest) (*radarV1.SetRadarConfigResponse, error) {
	payload, err := auth.FromContext(ctx)
	if err != nil || payload == nil {
		return nil, adminV1.ErrorUnauthorized("authentication required")
	}
	if payload.Authority != userV1.UserAuthority_SYS_ADMIN && payload.Authority != userV1.UserAuthority_TENANT_ADMIN {
		return nil, adminV1.ErrorForbidden("only administrators can change radar platform config")
	}
	in := &biz.RadarUserConfigData{}
	if req != nil {
		in.GitHubToken = strings.TrimSpace(req.GithubToken)
		in.LLMEnabled = req.LlmEnabled
		in.LLMBaseURL = strings.TrimSpace(req.LlmBaseUrl)
		in.LLMAPIKey = strings.TrimSpace(req.LlmApiKey)
		in.LLMModel = strings.TrimSpace(req.LlmModel)
		in.CloneEnabled = req.CloneEnabled
		in.CloneDir = strings.TrimSpace(req.CloneDir)
	}
	cfg, err := s.uc.Set(ctx, in)
	if err != nil {
		return nil, err
	}
	s.invalidateCache()
	s.log.Infof("radar platform config updated by user %d (github_token=%t llm=%t clone=%t)",
		payload.UserId, cfg.GitHubToken != "", cfg.LLMEnabled, cfg.CloneEnabled)
	return &radarV1.SetRadarConfigResponse{
		HasGithubToken:    cfg.GitHubToken != "",
		GithubTokenMasked: maskToken(cfg.GitHubToken),
		LlmEnabled:        cfg.LLMEnabled,
		LlmBaseUrl:        cfg.LLMBaseURL,
		LlmModel:          cfg.LLMModel,
		LlmApiKeyMasked:   maskToken(cfg.LLMAPIKey),
		CloneEnabled:      cfg.CloneEnabled,
		CloneDir:          cfg.CloneDir,
	}, nil
}

// ── biz.RadarConfigProvider 实现（带 30s 快照缓存） ──

func (s *RadarConfigService) GitHubToken() string {
	s.refreshCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedToken
}

func (s *RadarConfigService) RadarLLM() biz.RadarLLMConfig {
	s.refreshCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedLLM
}

func (s *RadarConfigService) CloneSettings() (bool, string) {
	s.refreshCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cachedClone.enabled, s.cachedClone.dir
}

// refreshCache 过期则重读平台配置（读失败保留旧快照并记日志）。
func (s *RadarConfigService) refreshCache() {
	s.mu.RLock()
	fresh := time.Since(s.cachedAt) < radarConfigCacheTTL
	s.mu.RUnlock()
	if fresh {
		return
	}
	cfg, err := s.uc.Get(context.Background())
	if err != nil || cfg == nil {
		if err != nil {
			s.log.Warnf("refresh radar platform config failed: %v", err)
		}
		return
	}
	s.mu.Lock()
	s.cachedAt = time.Now()
	s.cachedToken = cfg.GitHubToken
	s.cachedLLM = biz.RadarLLMConfig{
		Enabled: cfg.LLMEnabled, BaseURL: cfg.LLMBaseURL,
		APIKey: cfg.LLMAPIKey, Model: cfg.LLMModel,
	}
	s.cachedClone.enabled = cfg.CloneEnabled
	s.cachedClone.dir = cfg.CloneDir
	s.mu.Unlock()
}

func (s *RadarConfigService) invalidateCache() {
	s.mu.Lock()
	s.cachedAt = time.Time{}
	s.mu.Unlock()
}

func maskToken(t string) string {
	if t == "" {
		return ""
	}
	if len(t) <= 8 {
		return "****"
	}
	return t[:4] + "…" + t[len(t)-4:]
}
