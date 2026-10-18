package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/radaruserconfig"
	"framework/pkg/crypto"

	adminV1 "framework/api/gen/go/admin/v1"
)

// RadarUserConfigRepo 雷达配置仓储：平台级单行（user_id=0），密钥 AES-256-GCM 加密存储。
type RadarUserConfigRepo struct {
	data *Data
	log  *log.Helper
}

func NewRadarUserConfigRepo(data *Data, logger log.Logger) biz.RadarUserConfigRepo {
	return &RadarUserConfigRepo{
		log:  log.NewHelper(log.With(logger, "module", "radar/user-config/repo/admin-service")),
		data: data,
	}
}

func (r *RadarUserConfigRepo) Get(ctx context.Context, userID uint32) (*biz.RadarUserConfigData, error) {
	entity, err := r.data.db.Client().RadarUserConfig.Query().
		Where(radaruserconfig.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query radar user config failed (user_id=%d): %s", userID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query radar config failed")
	}
	return r.decrypt(entity)
}

func (r *RadarUserConfigRepo) Save(ctx context.Context, userID uint32, cfg *biz.RadarUserConfigData) error {
	if cfg == nil {
		return adminV1.ErrorBadRequest("invalid parameter: config is required")
	}
	tokenEnc, err := crypto.GetCrypto().Encrypt(cfg.GitHubToken)
	if err != nil {
		r.log.Errorf("encrypt github token failed (user_id=%d): %s", userID, err.Error())
		return adminV1.ErrorInternalServerError("save radar config failed")
	}
	apiKeyEnc, err := crypto.GetCrypto().Encrypt(cfg.LLMAPIKey)
	if err != nil {
		r.log.Errorf("encrypt llm api key failed (user_id=%d): %s", userID, err.Error())
		return adminV1.ErrorInternalServerError("save radar config failed")
	}

	existing, err := r.data.db.Client().RadarUserConfig.Query().
		Where(radaruserconfig.UserIDEQ(userID)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("query radar user config failed (user_id=%d): %s", userID, err.Error())
		return adminV1.ErrorInternalServerError("save radar config failed")
	}

	now := time.Now()
	if existing == nil {
		_, err = r.data.db.Client().RadarUserConfig.Create().
			SetUserID(userID).
			SetGithubTokenEnc(tokenEnc).
			SetLlmEnabled(cfg.LLMEnabled).
			SetLlmBaseURL(cfg.LLMBaseURL).
			SetLlmAPIKeyEnc(apiKeyEnc).
			SetLlmModel(cfg.LLMModel).
			SetCloneEnabled(cfg.CloneEnabled).
			SetCloneDir(cfg.CloneDir).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx)
	} else {
		_, err = r.data.db.Client().RadarUserConfig.UpdateOneID(existing.ID).
			SetGithubTokenEnc(tokenEnc).
			SetLlmEnabled(cfg.LLMEnabled).
			SetLlmBaseURL(cfg.LLMBaseURL).
			SetLlmAPIKeyEnc(apiKeyEnc).
			SetLlmModel(cfg.LLMModel).
			SetCloneEnabled(cfg.CloneEnabled).
			SetCloneDir(cfg.CloneDir).
			SetUpdateTime(now).
			Save(ctx)
	}
	if err != nil {
		r.log.Errorf("save radar user config failed (user_id=%d): %s", userID, err.Error())
		return adminV1.ErrorInternalServerError("save radar config failed")
	}
	return nil
}

// decrypt 解密密钥字段；解密失败按空值处理并记日志（不影响非密钥字段读取）。
func (r *RadarUserConfigRepo) decrypt(e *ent.RadarUserConfig) (*biz.RadarUserConfigData, error) {
	cfg := &biz.RadarUserConfigData{
		LLMEnabled:   e.LlmEnabled,
		CloneEnabled: e.CloneEnabled,
	}
	if e.LlmBaseURL != nil {
		cfg.LLMBaseURL = *e.LlmBaseURL
	}
	if e.LlmModel != nil {
		cfg.LLMModel = *e.LlmModel
	}
	if e.CloneDir != nil {
		cfg.CloneDir = *e.CloneDir
	}
	if e.GithubTokenEnc != nil && *e.GithubTokenEnc != "" {
		plain, err := crypto.GetCrypto().Decrypt(*e.GithubTokenEnc)
		if err != nil {
			r.log.Errorf("decrypt github token failed (user_id=%d): %s", e.UserID, err.Error())
		} else {
			cfg.GitHubToken = plain
		}
	}
	if e.LlmAPIKeyEnc != nil && *e.LlmAPIKeyEnc != "" {
		plain, err := crypto.GetCrypto().Decrypt(*e.LlmAPIKeyEnc)
		if err != nil {
			r.log.Errorf("decrypt llm api key failed (user_id=%d): %s", e.UserID, err.Error())
		} else {
			cfg.LLMAPIKey = plain
		}
	}
	return cfg, nil
}
