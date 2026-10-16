package data

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/loginfailcounter"
)

// LoginFailTargetType 目标类型：IP 或 USER。
// 类型本体已上移到 biz 层（端口入参），此处保留别名兼容。
type LoginFailTargetType = biz.LoginFailTargetType

const (
	LoginFailTargetIP   = biz.LoginFailTargetIP
	LoginFailTargetUser = biz.LoginFailTargetUser
)

// LoginFailCounterRepo 登录失败计数器，支持 IP 与账户双重锁定。
// 计数策略（参数由 system_configs 提供，这里接收外部传入的阈值）：
//   - IP 维度：失败满 ipLockCount 次锁 ipLockMinutes 分钟
//   - 账户维度：失败满 userLockCount 次锁 userLockMinutes 分钟
//   - 成功登录后重置对应维度计数
type LoginFailCounterRepo struct {
	data *Data
	log  *log.Helper
}

func NewLoginFailCounterRepo(data *Data, logger log.Logger) *LoginFailCounterRepo {
	return &LoginFailCounterRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "login-fail-counter/repo/admin-service")),
	}
}

// IsLocked 检查目标是否已被锁定（locked_until > now）。
func (r *LoginFailCounterRepo) IsLocked(ctx context.Context, targetType LoginFailTargetType, targetValue string) (bool, time.Time, error) {
	if targetValue == "" {
		return false, time.Time{}, nil
	}

	counter, err := r.getCounter(ctx, targetType, targetValue)
	if err != nil {
		return false, time.Time{}, err
	}
	if counter == nil || counter.LockedUntil == nil {
		return false, time.Time{}, nil
	}

	if time.Now().Before(*counter.LockedUntil) {
		return true, *counter.LockedUntil, nil
	}

	// 锁定已过期，重置计数
	if err := r.Reset(ctx, targetType, targetValue); err != nil {
		r.log.Errorf("reset expired lock failed: %v", err)
	}
	return false, time.Time{}, nil
}

// IncrFail 登录失败时递增计数；达到阈值则写入 locked_until。
// NewlyLocked 仅当本次递增使目标从"未锁定"转为"锁定"（首次触阈）为 true，
// 供调用方触发撞库告警（security.login.abuse）；锁定存续期内的继续失败只顺延 locked_until，不重复告警。
// 并发安全：fail_count 用 SQL 原子自增（AddFailCount），锁定用条件 UPDATE 抢占认领，
// 避免并发撞库下"读-改-写"丢计数导致阈值延迟触发或重复告警。
func (r *LoginFailCounterRepo) IncrFail(ctx context.Context, targetType LoginFailTargetType, targetValue string, lockCount int, lockMinutes int) (*biz.LoginFailIncrResult, error) {
	if targetValue == "" {
		return &biz.LoginFailIncrResult{}, nil
	}

	counter, err := r.getOrCreateCounter(ctx, targetType, targetValue)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	// 步骤1：原子自增（fail_count = fail_count + 1），无读改写竞态
	if err := r.data.db.Client().LoginFailCounter.UpdateOneID(counter.ID).
		AddFailCount(1).
		SetUpdateTime(now).
		Exec(ctx); err != nil {
		r.log.Errorf("incr login fail count failed: %s", err.Error())
		return nil, fmt.Errorf("incr login fail count failed: %w", err)
	}

	// 步骤2：重读拿到自增后的准确计数（单调递增，晚到者必见最终值，不会漏判阈值）
	updated, err := r.getCounter(ctx, targetType, targetValue)
	if err != nil {
		return nil, fmt.Errorf("requery login fail counter failed: %w", err)
	}

	res := &biz.LoginFailIncrResult{FailCount: updated.FailCount}
	if updated.FailCount < lockCount || lockCount <= 0 {
		return res, nil
	}

	res.LockedUntil = now.Add(time.Duration(lockMinutes) * time.Minute)

	// 步骤3：条件抢占锁定——仅当"此前未锁定或锁定已过期"的行才会被命中，
	// 并发下全局只有一个请求能成功认领 NewlyLocked（ affected=1 ）。
	affected, err := r.data.db.Client().LoginFailCounter.Update().
		Where(
			loginfailcounter.IDEQ(updated.ID),
			loginfailcounter.Or(
				loginfailcounter.LockedUntilIsNil(),
				loginfailcounter.LockedUntilLTE(now),
			),
		).
		SetLockedUntil(res.LockedUntil).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("claim login lock failed: %s", err.Error())
		return nil, fmt.Errorf("claim login lock failed: %w", err)
	}
	res.NewlyLocked = affected > 0

	if !res.NewlyLocked {
		// 锁定存续期内的继续失败：无条件顺延 locked_until（保持原有"顺延不重复告警"语义）
		if err := r.data.db.Client().LoginFailCounter.UpdateOneID(updated.ID).
			SetLockedUntil(res.LockedUntil).
			SetUpdateTime(now).
			Exec(ctx); err != nil {
			r.log.Errorf("extend login lock failed: %s", err.Error())
			return nil, fmt.Errorf("extend login lock failed: %w", err)
		}
	}
	return res, nil
}

// Reset 登录成功后重置计数（清零 fail_count 和 locked_until）。
func (r *LoginFailCounterRepo) Reset(ctx context.Context, targetType LoginFailTargetType, targetValue string) error {
	if targetValue == "" {
		return nil
	}

	counter, err := r.getCounter(ctx, targetType, targetValue)
	if err != nil {
		return err
	}
	if counter == nil {
		return nil
	}

	if err := r.data.db.Client().LoginFailCounter.UpdateOneID(counter.ID).
		SetFailCount(0).
		ClearLockedUntil().
		SetUpdateTime(time.Now()).
		Exec(ctx); err != nil {
		r.log.Errorf("reset login fail count failed: %s", err.Error())
		return fmt.Errorf("reset login fail count failed: %w", err)
	}
	return nil
}

func (r *LoginFailCounterRepo) getCounter(ctx context.Context, targetType LoginFailTargetType, targetValue string) (*ent.LoginFailCounter, error) {
	return r.data.db.Client().LoginFailCounter.Query().
		Where(
			loginfailcounter.TargetTypeEQ(loginfailcounter.TargetType(targetType)),
			loginfailcounter.TargetValueEQ(targetValue),
		).
		Only(ctx)
}

func (r *LoginFailCounterRepo) getOrCreateCounter(ctx context.Context, targetType LoginFailTargetType, targetValue string) (*ent.LoginFailCounter, error) {
	counter, err := r.getCounter(ctx, targetType, targetValue)
	if err == nil {
		return counter, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query login fail counter failed: %w", err)
	}

	// 不存在则创建
	counter, err = r.data.db.Client().LoginFailCounter.Create().
		SetTargetType(loginfailcounter.TargetType(targetType)).
		SetTargetValue(targetValue).
		SetFailCount(0).
		Save(ctx)
	if err != nil {
		// 唯一索引冲突（并发创建），重新查询
		return r.getCounter(ctx, targetType, targetValue)
	}
	return counter, nil
}
