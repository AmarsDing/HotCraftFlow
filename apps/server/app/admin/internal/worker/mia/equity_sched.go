package miaworker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	cron "github.com/robfig/cron/v3"

	bizmia "framework/app/admin/internal/biz/mia"
	"framework/pkg/mediacontract"
)

var _ transport.Server = (*MiaEquityScheduler)(nil)

// quoteWarmBatchLimit 单次预热上限（防映射表膨胀拖垮一次 tick）。
const quoteWarmBatchLimit = 50

// MiaEquityScheduler 权益市场调度（FR-MIA-10，概要设计：交易时段密集 + 盘后摘要）：
//   - cron_trading：交易时段预热启用映射的权益行情缓存（列表/详情页命中 Redis）；
//   - cron_after_close：盘后摘要（当日 Top10 影响卡 markdown 推送，🔴 摘要体含免责声明）。
//
// 与加密调度相互独立（独立 cron 实例），单任务 panic/超时不影响后续触发。
type MiaEquityScheduler struct {
	cfg      bizmia.Config
	mappings bizmia.EntityMappingRepo
	quotes   bizmia.QuoteService
	impacts  *bizmia.ImpactUsecase
	sender   bizmia.WebhookSender
	log      *log.Helper
	cron     *cron.Cron
}

// NewMiaEquityScheduler 构造。
func NewMiaEquityScheduler(
	cfg bizmia.Config,
	mappings bizmia.EntityMappingRepo,
	quotes bizmia.QuoteService,
	impacts *bizmia.ImpactUsecase,
	sender bizmia.WebhookSender,
	logger log.Logger,
) *MiaEquityScheduler {
	return &MiaEquityScheduler{
		cfg:      cfg,
		mappings: mappings,
		quotes:   quotes,
		impacts:  impacts,
		sender:   sender,
		log:      log.NewHelper(log.With(logger, "module", "worker/mia-equity-sched")),
	}
}

// Start 注册并启动 cron；MIA 关闭或两条 cron 均为空时 no-op。
func (s *MiaEquityScheduler) Start(_ context.Context) error {
	if !s.cfg.Enabled {
		s.log.Info("mia disabled, equity scheduler not started")
		return nil
	}
	loc, err := time.LoadLocation(s.cfg.EquityTimezone)
	if err != nil {
		s.log.Warnf("invalid equity timezone %q, fallback Asia/Shanghai: %v", s.cfg.EquityTimezone, err)
		loc, err = time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.UTC
		}
	}
	s.cron = cron.New(cron.WithLocation(loc))
	registered := 0
	if spec := strings.TrimSpace(s.cfg.EquityCronTrading); spec != "" {
		if _, err := s.cron.AddFunc(spec, func() { s.runGuarded("equity-quote-warm", s.warmQuotes) }); err != nil {
			s.log.Warnf("invalid equity trading cron %q: %v", spec, err)
		} else {
			registered++
		}
	}
	if spec := strings.TrimSpace(s.cfg.EquityCronAfterClose); spec != "" {
		if _, err := s.cron.AddFunc(spec, func() { s.runGuarded("after-close-digest", s.afterCloseDigest) }); err != nil {
			s.log.Warnf("invalid equity after-close cron %q: %v", spec, err)
		} else {
			registered++
		}
	}
	if registered == 0 {
		s.log.Info("no equity cron configured, scheduler not started")
		return nil
	}
	s.cron.Start()
	s.log.Infof("mia equity scheduler started: jobs=%d tz=%s", registered, loc.String())
	return nil
}

// Stop 停止 cron 并等待在跑任务结束（robfig/cron Stop 返回的 ctx 在任务完成后关闭）。
func (s *MiaEquityScheduler) Stop(ctx context.Context) error {
	if s.cron == nil {
		return nil
	}
	done := s.cron.Stop()
	select {
	case <-done.Done():
	case <-ctx.Done():
	}
	s.log.Info("mia equity scheduler stopped")
	return nil
}

// runGuarded 任务守卫：5 分钟超时 + panic recover，单次失败不影响调度器本身。
func (s *MiaEquityScheduler) runGuarded(name string, fn func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			s.log.Errorf("mia equity job %s panicked: %v", name, r)
		}
	}()
	if err := fn(ctx); err != nil {
		s.log.Warnf("mia equity job %s failed: %v", name, err)
	}
}

// warmQuotes 预热启用映射中**非加密**资产的行情缓存（≤50 个去重符号/轮）。
func (s *MiaEquityScheduler) warmQuotes(ctx context.Context) error {
	if s.quotes == nil || s.mappings == nil {
		return nil
	}
	items, err := s.mappings.ListEnabled(ctx)
	if err != nil {
		return err
	}
	symbols := make([]string, 0, quoteWarmBatchLimit)
	seen := make(map[string]struct{})
	for _, it := range items {
		if it.AssetClass == mediacontract.AssetCrypto {
			continue
		}
		sym := strings.TrimSpace(it.Symbol)
		if sym == "" {
			continue
		}
		if _, dup := seen[sym]; dup {
			continue
		}
		seen[sym] = struct{}{}
		symbols = append(symbols, sym)
		if len(symbols) >= quoteWarmBatchLimit {
			break
		}
	}
	warmed := 0
	for _, sym := range symbols {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := s.quotes.GetQuote(ctx, sym); err != nil {
			s.log.Warnf("warm equity quote %s failed: %v", sym, err) // 单符号失败不阻断本轮
			continue
		}
		warmed++
	}
	s.log.Infof("equity quote warm done: %d/%d", warmed, len(symbols))
	return nil
}

// afterCloseDigest 盘后摘要：当日（权益时区自然日）Top10 影响卡聚合推送。
// 🔴 摘要含免责声明；无条目或未配 webhook 时静默跳过。
func (s *MiaEquityScheduler) afterCloseDigest(ctx context.Context) error {
	if s.sender == nil || s.impacts == nil {
		return nil
	}
	loc, err := time.LoadLocation(s.cfg.EquityTimezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	rows, total, err := s.impacts.List(ctx, bizmia.ImpactQuery{
		Page:          1,
		PageSize:      10,
		OccurredAfter: &dayStart,
	})
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		s.log.Info("after-close digest: no impact cards today, skip")
		return nil
	}
	// 🔴 disclaimer 兜底：卡自身携带（落库已硬校验）；此处再验一次防配置漂移。
	disclaimer := rows[0].Disclaimer
	if strings.TrimSpace(disclaimer) == "" {
		return fmt.Errorf("mia: disclaimer empty, refuse to send digest")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**MIA 盘后影响摘要** %s（当日 %d 条，示前 %d）\n", now.Format("2006-01-02"), total, len(rows))
	for i, c := range rows {
		fmt.Fprintf(&b, "%d. [%.1f] %s %s（%s，%s）\n",
			i+1, c.ImpactScore, directionArrow(c.Direction), c.Title, c.ImpactID, c.SentimentLabel)
	}
	b.WriteString("_" + disclaimer + "_")

	body, err := json.Marshal(map[string]any{
		"msgtype":    "markdown",
		"markdown":   map[string]any{"content": b.String()},
		"kind":       "mia_after_close_digest",
		"date":       now.Format("2006-01-02"),
		"total":      total,
		"disclaimer": disclaimer,
	})
	if err != nil {
		return err
	}
	if err := s.sender.Send(ctx, body); err != nil {
		return err
	}
	s.log.Infof("after-close digest sent: date=%s total=%d", now.Format("2006-01-02"), total)
	return nil
}

// directionArrow 摘要里的方向记号（↑利多 ↓利空 →中性）。
func directionArrow(d mediacontract.Direction) string {
	switch d {
	case mediacontract.DirectionBullish:
		return "↑"
	case mediacontract.DirectionBearish:
		return "↓"
	default:
		return "→"
	}
}
