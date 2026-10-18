package data

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/conf"

	bizmia "framework/app/admin/internal/biz/mia"
)

// quoteRedisKeyPrefix Redis 行情缓存键前缀（TTL 5 分钟，详细设计 §2.5）。
const quoteRedisKeyPrefix = "mia:quote:"

// quotePGStaleMaxAge PG 快照兜底的最大接受陈旧度（24h 内仍比没有强，返回时打告警）。
const quotePGStaleMaxAge = 24 * time.Hour

// MiaQuoteService 行情查询聚合（bizmia.QuoteService 实现）。
// 链式：Redis（5min）→ 提供方（按配置顺序）→ PG 快照兜底；
// 提供方全挂且 PG 无快照 → ErrQuoteReadFail（上游据此返回「行情不可用」而非空图）。
type MiaQuoteService struct {
	cache     bizmia.QuoteCacheRepo
	redis     *RedisClient
	providers []bizmia.QuoteProvider
	cacheTTL  time.Duration
	log       *log.Helper
}

// NewMiaQuoteService 构造：按 mia.yaml providers 顺序组装提供方链。
// 🔴 openbb 恒为阻断适配器（许可证未复核）；providers 未配置时默认仅 ccxt。
func NewMiaQuoteService(
	cfg *conf.Bootstrap,
	cache bizmia.QuoteCacheRepo,
	redis *RedisClient,
	ccxt *MiaCCXTQuoteProvider,
	openbb *MiaBlockedOpenBBProvider,
	logger log.Logger,
) bizmia.QuoteService {
	mcfg := bizmia.ResolveConfig(cfg)
	byName := map[string]bizmia.QuoteProvider{
		ccxt.Name():   ccxt,
		openbb.Name(): openbb,
	}
	names := mcfg.QuoteProviders
	if len(names) == 0 {
		names = []string{"ccxt"}
	}
	providers := make([]bizmia.QuoteProvider, 0, len(names))
	for _, n := range names {
		if p, ok := byName[n]; ok {
			providers = append(providers, p)
		}
	}
	return &MiaQuoteService{
		cache:     cache,
		redis:     redis,
		providers: providers,
		cacheTTL:  mcfg.QuoteCacheTTL,
		log:       log.NewHelper(log.With(logger, "module", "mia/quote-service/admin-service")),
	}
}

// GetQuote 默认链查询。
func (s *MiaQuoteService) GetQuote(ctx context.Context, symbol string) (*bizmia.QuoteSnapshot, error) {
	if snap := s.fromRedis(ctx, symbol); snap != nil {
		return snap, nil
	}

	var firstErr error
	for _, p := range s.providers {
		snap, err := p.FetchOHLCV(ctx, symbol)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			s.log.Warnf("quote provider %s failed for %s: %s", p.Name(), symbol, err.Error())
			continue
		}
		s.persist(ctx, snap)
		return snap, nil
	}

	// 提供方全挂：PG 快照兜底（24h 窗口，陈旧但可读）。
	if s.cache != nil {
		if snap, err := s.cache.Get(ctx, symbol, s.fallbackProviderName(), quotePGStaleMaxAge); err == nil && snap != nil {
			s.log.Warnf("quote providers all down for %s; serving stale pg snapshot (fetched_at=%s)",
				symbol, snap.FetchedAt.Format(time.RFC3339))
			snap.FromCache = true
			return snap, nil
		}
	}
	if firstErr != nil && errors.Is(firstErr, bizmia.ErrOpenBBLicenseUnverified) && len(s.providers) == 1 {
		return nil, firstErr // 唯一提供方是被阻断的 openbb：如实返回许可证错误
	}
	return nil, bizmia.ErrQuoteReadFail
}

// GetQuoteWithProvider 指定提供方查询（UI 下拉选择 provider）。
func (s *MiaQuoteService) GetQuoteWithProvider(ctx context.Context, provider, symbol string) (*bizmia.QuoteSnapshot, error) {
	for _, p := range s.providers {
		if p.Name() != provider {
			continue
		}
		snap, err := p.FetchOHLCV(ctx, symbol)
		if err != nil {
			return nil, err
		}
		s.persist(ctx, snap)
		return snap, nil
	}
	// 不在链上的提供方（如未配置的 yahoo）：明确不可用。
	return nil, bizmia.ErrQuoteReadFail
}

// fromRedis Redis 缓存命中（JSON 编/解码失败按未命中处理）。
func (s *MiaQuoteService) fromRedis(ctx context.Context, symbol string) *bizmia.QuoteSnapshot {
	if s.redis == nil || !s.redis.IsConfigured() {
		return nil
	}
	raw, err := s.redis.GetString(ctx, quoteRedisKeyPrefix+symbol)
	if err != nil || raw == "" {
		return nil
	}
	var snap bizmia.QuoteSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil
	}
	snap.FromCache = true
	return &snap
}

// persist 成功拉取后双写：Redis（短 TTL）+ PG 快照（兜底）。失败仅告警不阻断返回。
func (s *MiaQuoteService) persist(ctx context.Context, snap *bizmia.QuoteSnapshot) {
	if s.redis != nil && s.redis.IsConfigured() {
		if raw, err := json.Marshal(snap); err == nil {
			if err := s.redis.SetStringEX(ctx, quoteRedisKeyPrefix+snap.Symbol, string(raw), s.cacheTTL); err != nil {
				s.log.Warnf("quote redis cache write failed (%s): %s", snap.Symbol, err.Error())
			}
		}
	}
	if s.cache != nil {
		if err := s.cache.Upsert(ctx, snap); err != nil {
			s.log.Warnf("quote pg snapshot upsert failed (%s): %s", snap.Symbol, err.Error())
		}
	}
}

func (s *MiaQuoteService) fallbackProviderName() string {
	if len(s.providers) == 0 {
		return ""
	}
	return s.providers[0].Name()
}
