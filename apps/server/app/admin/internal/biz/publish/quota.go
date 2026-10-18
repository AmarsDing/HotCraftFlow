package publish

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
)

// QuotaUsecase 发布配额用例（NFR-MAP-06）。权威来源为 receipts 时间窗计数，
// 账号行上的 published_24h 仅是展示近似值（data 层快递增，不作判定依据）。
type QuotaUsecase struct {
	conf     *Conf
	receipts ReceiptRepo
	clock    func() time.Time
	log      *log.Helper
}

// NewQuotaUsecase 构造配额用例。
func NewQuotaUsecase(conf *Conf, receipts ReceiptRepo, logger log.Logger) *QuotaUsecase {
	return &QuotaUsecase{
		conf:     conf,
		receipts: receipts,
		clock:    time.Now,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish-quota")),
	}
}

// Check 发布前配额校验：24h 总量 / 平台小时量 / 账号最小间隔。
// 超限错误一律包装 algo 的 ErrQuotaExceeded（service 层统一映射 MAP_QUOTA_EXCEEDED），
// 消息体带具体命中项，便于定位。
func (uc *QuotaUsecase) Check(ctx context.Context, platform string, account *AccountRow) error {
	now := uc.clock()

	// 1) 24h 全站总量硬上限（超限阻断，需人工解除——不静默排队）。
	total, err := uc.receipts.CountReceiptsSince(ctx, now.Add(-24*time.Hour))
	if err != nil {
		return err
	}
	if total >= uc.conf.Quota.DailyPublishLimit {
		uc.log.Warnf("[map-quota] 24h 总量超限: %d/%d", total, uc.conf.Quota.DailyPublishLimit)
		return fmt.Errorf("%w: 24h 发布总量已达上限 %d", algopublisher.ErrQuotaExceeded, uc.conf.Quota.DailyPublishLimit)
	}

	// 2) 单平台小时上限。
	platformCnt, err := uc.receipts.CountReceiptsByPlatformSince(ctx, platform, now.Add(-time.Hour))
	if err != nil {
		return err
	}
	if platformCnt >= uc.conf.Quota.PlatformHourlyLimit {
		uc.log.Warnf("[map-quota] 平台小时超限 platform=%s: %d/%d", platform, platformCnt, uc.conf.Quota.PlatformHourlyLimit)
		return fmt.Errorf("%w: 平台 %s 每小时上限 %d 已达", algopublisher.ErrQuotaExceeded, platform, uc.conf.Quota.PlatformHourlyLimit)
	}

	// 3) 账号最小发布间隔（拟人化节奏，防平台风控连击）。
	if account != nil && account.LastPublishedAt != nil {
		interval := now.Sub(*account.LastPublishedAt)
		if interval < uc.conf.Quota.AccountMinInterval {
			return fmt.Errorf("%w: 账号 %s 距上次发布仅 %s（最小间隔 %s）",
				algopublisher.ErrQuotaExceeded, account.AccountID, interval.Round(time.Second), uc.conf.Quota.AccountMinInterval)
		}
	}
	return nil
}
