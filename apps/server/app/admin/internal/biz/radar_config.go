package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 平台级 LLM 默认值（与 insight 服务 / 前端 placeholder 保持一致）。
const (
	radarDefaultLLMBaseURL = "https://api.openai.com/v1"
	radarDefaultLLMModel   = "gpt-4o-mini"
)

// ─────────────────────────────────────────────────────────────
// RadarConfigUsecase 雷达配置用例（平台级单行，userID=RadarSystemUserID）。
// 密钥明文只在本层与 GitHub 客户端之间流动，repo 层加密落库。
// ─────────────────────────────────────────────────────────────

type RadarConfigUsecase struct {
	repo RadarUserConfigRepo
	log  *log.Helper
}

func NewRadarConfigUsecase(repo RadarUserConfigRepo, logger log.Logger) *RadarConfigUsecase {
	return &RadarConfigUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "radar/config/usecase")),
	}
}

// Get 读取平台级配置；未配置返回带默认值的空结构（非 nil）。
func (uc *RadarConfigUsecase) Get(ctx context.Context) (*RadarUserConfigData, error) {
	cfg, err := uc.repo.Get(ctx, RadarSystemUserID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &RadarUserConfigData{}
	}
	normalizeRadarConfig(cfg)
	return cfg, nil
}

// Set 保存平台级配置。githubToken / llmAPIKey 传空串表示保留原值（与脱敏回显的局部更新语义一致）。
// 本层负责归一化：字符串 trim、LLM 空值落默认（service 层也 trim 一次，biz 层收口保证任何入口一致）。
func (uc *RadarConfigUsecase) Set(ctx context.Context, in *RadarUserConfigData) (*RadarUserConfigData, error) {
	if in == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter: config is required")
	}
	in.GitHubToken = strings.TrimSpace(in.GitHubToken)
	in.LLMBaseURL = strings.TrimSpace(in.LLMBaseURL)
	in.LLMAPIKey = strings.TrimSpace(in.LLMAPIKey)
	in.LLMModel = strings.TrimSpace(in.LLMModel)
	in.CloneDir = strings.TrimSpace(in.CloneDir)

	current, err := uc.repo.Get(ctx, RadarSystemUserID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		current = &RadarUserConfigData{}
	}
	if in.GitHubToken != "" {
		current.GitHubToken = in.GitHubToken
	}
	if in.LLMAPIKey != "" {
		current.LLMAPIKey = in.LLMAPIKey
	}
	current.LLMEnabled = in.LLMEnabled
	current.LLMBaseURL = in.LLMBaseURL
	current.LLMModel = in.LLMModel
	current.CloneEnabled = in.CloneEnabled
	current.CloneDir = in.CloneDir
	normalizeRadarConfig(current)

	if err := uc.repo.Save(ctx, RadarSystemUserID, current); err != nil {
		return nil, err
	}
	return current, nil
}

// normalizeRadarConfig 填充 LLM 默认值（与 InsightService.complete / 前端 placeholder 对齐）。
func normalizeRadarConfig(cfg *RadarUserConfigData) {
	if cfg.LLMBaseURL == "" {
		cfg.LLMBaseURL = radarDefaultLLMBaseURL
	}
	if cfg.LLMModel == "" {
		cfg.LLMModel = radarDefaultLLMModel
	}
}

// GitHubToken 平台 GitHub Token（供 GitHub 客户端使用）。
func (uc *RadarConfigUsecase) GitHubToken(ctx context.Context) string {
	cfg, err := uc.repo.Get(ctx, RadarSystemUserID)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.GitHubToken
}
