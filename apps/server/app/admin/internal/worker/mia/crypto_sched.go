package miaworker

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	cron "github.com/robfig/cron/v3"

	bizmia "framework/app/admin/internal/biz/mia"
	"framework/pkg/mediacontract"
)

var _ transport.Server = (*MiaCryptoScheduler)(nil)

// MiaCryptoScheduler 加密市场调度（FR-MIA-10，概要设计：7×24 短周期）：
// 预热启用映射中的加密资产行情缓存——映射表 A 的 crypto 符号 + 映射表 B 的叙事币对。
// 币对符号口径与影响卡 affected[].symbol 一致（原样取用，不在此处补 /USDT 等猜测）。
type MiaCryptoScheduler struct {
	cfg        bizmia.Config
	mappings   bizmia.EntityMappingRepo
	narratives bizmia.NarrativeMappingRepo
	quotes     bizmia.QuoteService
	log        *log.Helper
	cron       *cron.Cron
}

// NewMiaCryptoScheduler 构造。
func NewMiaCryptoScheduler(
	cfg bizmia.Config,
	mappings bizmia.EntityMappingRepo,
	narratives bizmia.NarrativeMappingRepo,
	quotes bizmia.QuoteService,
	logger log.Logger,
) *MiaCryptoScheduler {
	return &MiaCryptoScheduler{
		cfg:        cfg,
		mappings:   mappings,
		narratives: narratives,
		quotes:     quotes,
		log:        log.NewHelper(log.With(logger, "module", "worker/mia-crypto-sched")),
	}
}

// Start 注册并启动 cron；MIA 关闭、crypto 子开关关闭或 cron 为空时 no-op。
func (s *MiaCryptoScheduler) Start(_ context.Context) error {
	if !s.cfg.Enabled || !s.cfg.CryptoEnabled {
		s.log.Info("mia crypto scheduler disabled, not started")
		return nil
	}
	spec := strings.TrimSpace(s.cfg.CryptoCron)
	if spec == "" {
		s.log.Info("no crypto cron configured, scheduler not started")
		return nil
	}
	s.cron = cron.New() // 加密 7×24，无时区概念（UTC 语义由配置方自行把握）
	if _, err := s.cron.AddFunc(spec, func() { s.runGuarded(s.warmQuotes) }); err != nil {
		s.log.Warnf("invalid crypto cron %q, scheduler not started: %v", spec, err)
		return nil
	}
	s.cron.Start()
	s.log.Infof("mia crypto scheduler started: cron=%q", spec)
	return nil
}

// Stop 停止 cron 并等待在跑任务结束。
func (s *MiaCryptoScheduler) Stop(ctx context.Context) error {
	if s.cron == nil {
		return nil
	}
	done := s.cron.Stop()
	select {
	case <-done.Done():
	case <-ctx.Done():
	}
	s.log.Info("mia crypto scheduler stopped")
	return nil
}

// runGuarded 任务守卫：5 分钟超时 + panic recover。
func (s *MiaCryptoScheduler) runGuarded(fn func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			s.log.Errorf("mia crypto quote warm panicked: %v", r)
		}
	}()
	if err := fn(ctx); err != nil {
		s.log.Warnf("mia crypto quote warm failed: %v", err)
	}
}

// warmQuotes 预热加密符号行情缓存：A 表 crypto 符号 ∪ B 表叙事 coins（去重，≤50/轮）。
func (s *MiaCryptoScheduler) warmQuotes(ctx context.Context) error {
	if s.quotes == nil {
		return nil
	}
	seen := make(map[string]struct{})
	symbols := make([]string, 0, quoteWarmBatchLimit)
	push := func(sym string) bool { // false = 已达上限
		sym = strings.TrimSpace(sym)
		if sym == "" {
			return true
		}
		if _, dup := seen[sym]; dup {
			return true
		}
		seen[sym] = struct{}{}
		symbols = append(symbols, sym)
		return len(symbols) < quoteWarmBatchLimit
	}

	if s.mappings != nil {
		items, err := s.mappings.ListEnabled(ctx)
		if err != nil {
			return err
		}
		for _, it := range items {
			if it.AssetClass != mediacontract.AssetCrypto {
				continue
			}
			if !push(it.Symbol) {
				break
			}
		}
	}
	if s.narratives != nil && len(symbols) < quoteWarmBatchLimit {
		items, err := s.narratives.ListEnabled(ctx)
		if err != nil {
			return err
		}
	outer:
		for _, it := range items {
			for _, coin := range it.Coins {
				if !push(coin) {
					break outer
				}
			}
		}
	}

	warmed := 0
	for _, sym := range symbols {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := s.quotes.GetQuote(ctx, sym); err != nil {
			s.log.Warnf("warm crypto quote %s failed: %v", sym, err) // 单符号失败不阻断本轮
			continue
		}
		warmed++
	}
	s.log.Infof("crypto quote warm done: %d/%d", warmed, len(symbols))
	return nil
}
