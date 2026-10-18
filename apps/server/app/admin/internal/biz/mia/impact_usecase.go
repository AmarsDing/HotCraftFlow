package mia

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// ImpactUsecase 影响卡查询与校准（FR-MIA-06/11）。
type ImpactUsecase struct {
	cfg    Config
	impacts ImpactCardRepo
	quotes QuoteService
	log    *log.Helper
}

// QuoteService 行情查询端口（聚合：缓存 + 提供方），data 层组合实现。
type QuoteService interface {
	// GetQuote 只读行情查询（Redis 缓存 5 分钟 / PG 快照兜底）。
	GetQuote(ctx context.Context, symbol string) (*QuoteSnapshot, error)
	// GetQuoteWithProvider 指定提供方。
	GetQuoteWithProvider(ctx context.Context, provider, symbol string) (*QuoteSnapshot, error)
}

// NewImpactUsecase 构造影响卡用例。
func NewImpactUsecase(
	cfg Config,
	impacts ImpactCardRepo,
	quotes QuoteService,
	logger log.Logger,
) *ImpactUsecase {
	return &ImpactUsecase{
		cfg:     cfg,
		impacts: impacts,
		quotes:  quotes,
		log:     log.NewHelper(log.With(logger, "module", "biz/mia/impact")),
	}
}

// List 分页查询（默认过滤已过期——TTL 纪律：过期卡不占排行榜但不删除）。
func (u *ImpactUsecase) List(ctx context.Context, q ImpactQuery) ([]*ImpactCardRow, int, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 500 {
		q.PageSize = 50
	}
	return u.impacts.List(ctx, q)
}

// Get 详情。
func (u *ImpactUsecase) Get(ctx context.Context, impactID string) (*ImpactCardRow, error) {
	card, err := u.impacts.Get(ctx, impactID)
	if err != nil {
		return nil, err
	}
	if card == nil {
		return nil, fmt.Errorf("impact card %s not found", impactID)
	}
	return card, nil
}

// GetQuote 行情只读查询（FR-MIA-07；provider 为 nil 走默认链）。
func (u *ImpactUsecase) GetQuote(ctx context.Context, symbol string) (*QuoteSnapshot, error) {
	if u.quotes == nil {
		return nil, ErrQuoteReadFail
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol required")
	}
	return u.quotes.GetQuote(ctx, symbol)
}

// CalibrationReport 校准报告视图（FR-MIA-11，P2）。
type CalibrationReport struct {
	Enabled      bool
	TotalSamples int
	Window       string
	Note         string // 固定提示文案（不承诺准确率）
	Buckets      []CalibrationBucket
}

// CalibrationBucket 按规则的统计桶。
type CalibrationBucket struct {
	RuleID  string
	Samples int
	HitRate float64
}

// calibrationNote 固定提示（详细设计 §2.7：sample<30 不具统计意义；非预测准确率）。
const calibrationNote = "本统计为规则校准参考，不代表预测准确率；样本量 <30 时结论不具统计意义。"

// GetCalibration 校准报告。verify 开启前返回空报告 + 固定提示（诚实披露）。
func (u *ImpactUsecase) GetCalibration(ctx context.Context, since, until time.Time) (*CalibrationReport, error) {
	report := &CalibrationReport{
		Enabled: u.cfg.VerifyEnabled,
		Note:    calibrationNote,
		Window:  since.UTC().Format("2006-01-02") + " ~ " + until.UTC().Format("2006-01-02"),
	}
	if !u.cfg.VerifyEnabled {
		// P2 未启用：返回显式空报告而非伪装成「无数据」。
		return report, nil
	}
	// P2 阶段接 verify_job 的落表数据聚合；当前返回空桶但报告结构可用。
	return report, nil
}

// RuleUsecase 传导规则管理（FR-MIA-08）。
type RuleUsecase struct {
	repo TransmissionRuleRepo
	log  *log.Helper
}

// NewRuleUsecase 构造传导规则用例。
func NewRuleUsecase(repo TransmissionRuleRepo, logger log.Logger) *RuleUsecase {
	return &RuleUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "biz/mia/rule")),
	}
}

// List 查询传导规则（enabledOnly=ture 看启动中）。
func (u *RuleUsecase) List(ctx context.Context, enabledOnly bool) ([]*TransmissionRuleRow, error) {
	return u.repo.List(ctx, enabledOnly)
}

// Create 新增（🔴 强制 confidence ≤0.6 + note 必填——详细设计 §2.5 诚意底线）。
func (u *RuleUsecase) Create(ctx context.Context, row *TransmissionRuleRow) (*TransmissionRuleRow, error) {
	if err := validateTransmissionRule(row); err != nil {
		return nil, err
	}
	row.RuleID = mediacontract.NewID(mediacontract.PrefixRule)
	return u.repo.Create(ctx, row)
}

// UpdateEnabled 启停切换。
func (u *RuleUsecase) UpdateEnabled(ctx context.Context, id int64, row *TransmissionRuleRow) (*TransmissionRuleRow, error) {
	if err := validateTransmissionRule(row); err != nil {
		return nil, err
	}
	return u.repo.Update(ctx, id, row)
}

// Delete 删除。
func (u *RuleUsecase) Delete(ctx context.Context, id int64) error {
	return u.repo.Delete(ctx, id)
}

// validateTransmissionRule 规则校验：🔴 note 必填 + confidence ∈(0, 0.6]（假设非因果）。
func validateTransmissionRule(row *TransmissionRuleRow) error {
	if row == nil {
		return fmt.Errorf("nil rule")
	}
	if strings.TrimSpace(row.Note) == "" {
		return fmt.Errorf("%w: note required", ErrPathEmpty)
	}
	if row.Confidence <= 0 || row.Confidence > 0.6 {
		return ErrTransmissionConfidenceTooHigh
	}
	if row.ToAsset == "" {
		return fmt.Errorf("to_asset required")
	}
	if !row.ToDirection.Valid() {
		return fmt.Errorf("%w: to_direction invalid", ErrConfidenceInvalid)
	}
	return nil
}

// SeedDefaultRules 初始 6 条核心传导规则（详细设计 §2.5 例表），表空时补种。
// 规则全部为「假设」，confidence ≤0.6（重点：721 降息的 BTC 项弱相关仅 0.40）。
func SeedDefaultRules(ctx context.Context, repo TransmissionRuleRepo, logger log.Logger) error {
	count, err := repo.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	l := log.NewHelper(log.With(logger, "module", "biz/mia/rule/seed"))
	defaults := defaultTransmissionRules()
	for _, d := range defaults {
		if _, err := repo.Create(ctx, &d); err != nil {
			l.Errorf("seed transmission rule %s failed: %v", d.RuleID, err)
			return err
		}
	}
	l.Infof("seeded %d default transmission rules", len(defaults))
	return nil
}

// defaultTransmissionRules 详细设计 §2.5 初始规则表（全量 confidence ≤0.6，假设非因果）。
func defaultTransmissionRules() []TransmissionRuleRow {
	mk := func(asset, dir string, min float64, to string, toDir mediacontract.Direction, conf float64, note string) TransmissionRuleRow {
		return TransmissionRuleRow{
			RuleID: mediacontract.NewID(mediacontract.PrefixRule),
			Trigger: mediacontract.TriggerSpec{
				Asset:      asset,
				Direction:  mediacontract.Direction(dir),
				MinMovePct: min,
			},
			FromAsset:   asset,
			ToAsset:     to,
			ToDirection: toDir,
			Confidence:  conf,
			Note:        note,
			Enabled:     true,
		}
	}
	return []TransmissionRuleRow{
		mk("DXY", "up", 0.5, "^EEM", mediacontract.DirectionBearish, 0.50, "美元指数走强 → 新兴市场股承压（资金外流假设）"),
		mk("DXY", "up", 0.5, "XAUUSD", mediacontract.DirectionBearish, 0.55, "美元指数走强 → 黄金计价下压（计价效应假设）"),
		mk("DXY", "up", 0.5, "BTC/USDT", mediacontract.DirectionBearish, 0.45, "强美元压制风险偏好（风险偏好假设）"),
		mk("US10Y", "up", 0.10, "^IXIC", mediacontract.DirectionBearish, 0.50, "美债 10Y 上行 → 成长股/科技承压（折现率假设）"),
		mk("POLICY_CUT", "up", 0, "^LOCAL", mediacontract.DirectionBullish, 0.60, "央行降息 → 本地股支持（流动性假设）"),
		mk("POLICY_CUT", "up", 0, "BTC/USDT", mediacontract.DirectionBullish, 0.40, "降息周期利于加密（弱相关，仅作参考）"),
	}
}
