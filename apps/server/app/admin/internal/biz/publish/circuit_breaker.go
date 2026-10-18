package publish

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// BreakerUsecase 熔断用例（详细设计 §6）。
//
// 🔴 凭据失效铁律：health=credential_invalid 后**不清凭据**（保留密文便于排查），
// 账号停用直到人工重新授权（reauth 覆盖凭据并复位健康态）。
//
// 平台级熔断近似说明（砍单第 5 项允许降级）：回执表仅存成功记录，无失败事件表，
// 无法精确计算「24h 平台失败率」。V1 以**渠道内 enabled 账号暂停占比**为代理：
// 占比 > platform_fail_rate_threshold 时，其余 enabled 账号一并暂停（防止在平台
// 明显限流/封禁时继续撞）。代理结论偏保守（宁可多停），误差上限在文档中诚实标注。
type BreakerUsecase struct {
	conf     *Conf
	accounts AccountRepo
	clock    func() time.Time
	log      *log.Helper
}

// NewBreakerUsecase 构造熔断用例。
func NewBreakerUsecase(conf *Conf, accounts AccountRepo, logger log.Logger) *BreakerUsecase {
	return &BreakerUsecase{
		conf:     conf,
		accounts: accounts,
		clock:    time.Now,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish-breaker")),
	}
}

// OnFailure 发布失败处理（sanitizedErr 必须已脱敏——适配器契约保证不含凭据/堆栈原文）。
// 连续失败达阈值 → paused + paused_until；阈值前一级 → degraded 预警。
func (uc *BreakerUsecase) OnFailure(ctx context.Context, account *AccountRow, sanitizedErr string) error {
	n, err := uc.accounts.IncrementFailures(ctx, account.AccountID)
	if err != nil {
		return err
	}
	threshold := uc.conf.Breaker.AccountFailThreshold
	switch {
	case n >= threshold:
		until := uc.clock().Add(uc.conf.Breaker.PauseDuration)
		uc.log.Warnf("[map-breaker] 账号熔断暂停 account=%s failures=%d until=%s", account.AccountID, n, until.Format(time.RFC3339))
		return uc.accounts.PauseAccount(ctx, account.AccountID, until, strPtrOrNil(sanitizedErr))
	case n >= threshold-1 && account.Health == AccountHealthHealthy:
		uc.log.Warnf("[map-breaker] 账号降级预警 account=%s failures=%d", account.AccountID, n)
		return uc.accounts.SetHealth(ctx, account.AccountID, AccountHealthDegraded, strPtrOrNil(sanitizedErr))
	default:
		// 保持原健康态，仅刷新 last_error（已脱敏）。
		return uc.accounts.SetHealth(ctx, account.AccountID, account.Health, strPtrOrNil(sanitizedErr))
	}
}

// OnSuccess 发布成功复位：连续失败清零 + 恢复健康（last_error 保留作历史）。
func (uc *BreakerUsecase) OnSuccess(ctx context.Context, accountID string) error {
	if err := uc.accounts.ResetFailures(ctx, accountID); err != nil {
		return err
	}
	return uc.accounts.SetHealth(ctx, accountID, AccountHealthHealthy, nil)
}

// MarkCredentialInvalid 凭据失效（🔴 不清凭据；账号停用待人工重新授权）。
func (uc *BreakerUsecase) MarkCredentialInvalid(ctx context.Context, accountID, sanitizedErr string) error {
	uc.log.Warnf("[map-breaker] 凭据失效停用账号 account=%s（凭据保留，待重新授权）", accountID)
	return uc.accounts.SetHealth(ctx, accountID, AccountHealthCredentialInvalid, strPtrOrNil(sanitizedErr))
}

// CheckChannelContagion 渠道熔断近似（见类型注释）：渠道内 enabled 账号暂停占比
// 超过阈值时，其余 enabled 且未暂停的账号一并暂停。返回是否需要人工关注（占比超阈即视为平台异常信号）。
func (uc *BreakerUsecase) CheckChannelContagion(ctx context.Context, channelID string) (bool, error) {
	accounts, err := uc.accounts.ListAccountsByChannel(ctx, channelID)
	if err != nil {
		return false, err
	}
	var enabled, paused int
	for _, a := range accounts {
		if !a.Enabled {
			continue
		}
		enabled++
		if a.Health == AccountHealthPaused || a.Health == AccountHealthCredentialInvalid {
			paused++
		}
	}
	if enabled == 0 {
		return false, nil
	}
	rate := float64(paused) / float64(enabled)
	if rate <= uc.conf.Breaker.PlatformFailRateThreshold {
		return false, nil
	}
	// 🔴 平台异常信号：传染式暂停其余账号（保守宁停勿撞），日志告警引导人工介入。
	until := uc.clock().Add(uc.conf.Breaker.PauseDuration)
	for _, a := range accounts {
		if !a.Enabled {
			continue
		}
		if a.Health == AccountHealthPaused || a.Health == AccountHealthCredentialInvalid {
			continue
		}
		note := "渠道级熔断：同槽位账号大面积异常，疑似平台限流/封禁（近似判定，见详细设计 §6 降级说明）"
		if err := uc.accounts.PauseAccount(ctx, a.AccountID, until, &note); err != nil {
			uc.log.Errorf("[map-breaker] 渠道传染暂停失败 account=%s err=%v", a.AccountID, err)
		}
	}
	uc.log.Warnf("[map-breaker] 渠道熔断触发 channel=%s paused=%d/%d rate=%.2f", channelID, paused, enabled, rate)
	return true, nil
}

// Usable 判定账号当前可承担发布（enabled 且健康态可用且暂停已过期的由调用方先复位——
// 过期暂停的自动恢复由 data 层查询时把 paused_until 已过期的视为可用；此处保守判定）。
func Usable(account *AccountRow, now time.Time) bool {
	if !account.Enabled {
		return false
	}
	switch account.Health {
	case AccountHealthCredentialInvalid:
		return false // 🔴 必须人工重新授权
	case AccountHealthPaused:
		// 暂停到期自动视为可用（懒惰恢复，无后台扫描任务）。
		return !account.Paused(now)
	default:
		return true
	}
}
