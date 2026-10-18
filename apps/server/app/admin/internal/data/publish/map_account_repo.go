package pubdata

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mappublishaccount"
)

// EntAccountRepo bizpub.AccountRepo 的 Ent 实现。
// 🔴 本 repo 只搬运 credential_ref 引用串，永远不接触明文/密文内容本身。
type EntAccountRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntAccountRepo 构造。
func NewEntAccountRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntAccountRepo {
	return &EntAccountRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/map-account-repo")),
	}
}

// CreateAccount 新建账号。
func (r *EntAccountRepo) CreateAccount(ctx context.Context, row *bizpub.AccountRow) error {
	builder := r.cli.Client().MapPublishAccount.Create().
		SetAccountID(row.AccountID).
		SetChannelID(row.ChannelID).
		SetDisplayName(row.DisplayName).
		SetEnabled(row.Enabled).
		SetHealth(mappublishaccount.Health(row.Health))
	if row.CredentialRef != nil {
		builder.SetNillableCredentialRef(row.CredentialRef)
	}
	if row.CredentialType != "" {
		ct := mappublishaccount.CredentialType(row.CredentialType)
		builder.SetNillableCredentialType(&ct)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return fmt.Errorf("%w: 账号 %s 已存在", bizpub.ErrBadRequest, row.AccountID)
		}
		r.log.Errorf("create map account failed: %v", err)
		return fmt.Errorf("%w: 创建账号失败", bizpub.ErrInternal)
	}
	row.ID = uint32(e.ID)
	row.CreateTime = derefTime(e.CreateTime)
	return nil
}

// GetAccount 按 account_id（pa_ 前缀）查询。
func (r *EntAccountRepo) GetAccount(ctx context.Context, accountID string) (*bizpub.AccountRow, error) {
	e, err := r.cli.Client().MapPublishAccount.Query().
		Where(mappublishaccount.AccountIDEQ(accountID)).
		Only(ctx)
	if err != nil {
		return nil, mapQueryErr(err, "账号")
	}
	return accountToRow(e), nil
}

// ListAccounts 分页列出（create_time 倒序）。
func (r *EntAccountRepo) ListAccounts(ctx context.Context, pageNo, pageSize int) ([]*bizpub.AccountRow, int, error) {
	pageNo, pageSize = clampPage(pageNo, pageSize)
	q := r.cli.Client().MapPublishAccount.Query()
	total, err := q.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count map accounts failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询账号总数失败", bizpub.ErrInternal)
	}
	entities, err := q.
		Order(ent.Desc(mappublishaccount.FieldCreateTime)).
		Offset((pageNo - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map accounts failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询账号列表失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.AccountRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, accountToRow(e))
	}
	return rows, total, nil
}

// ListAccountsByChannel 列出渠道下全部账号（发布指派用；量小不分页）。
func (r *EntAccountRepo) ListAccountsByChannel(ctx context.Context, channelID string) ([]*bizpub.AccountRow, error) {
	entities, err := r.cli.Client().MapPublishAccount.Query().
		Where(mappublishaccount.ChannelIDEQ(channelID)).
		Order(ent.Asc(mappublishaccount.FieldCreateTime)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map accounts by channel failed: %v", err)
		return nil, fmt.Errorf("%w: 查询渠道账号失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.AccountRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, accountToRow(e))
	}
	return rows, nil
}

// SetEnabled 人工启停。
func (r *EntAccountRepo) SetEnabled(ctx context.Context, accountID string, enabled bool) error {
	return r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		return u.SetEnabled(enabled)
	}, "启停账号")
}

// SetHealth 设置健康状态；lastErr 为 nil 时不动 last_error（已脱敏文案才允许传入）。
func (r *EntAccountRepo) SetHealth(ctx context.Context, accountID, health string, lastErr *string) error {
	return r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		u = u.SetHealth(mappublishaccount.Health(health))
		// 恢复健康时顺手清除过期暂停窗（懒惰恢复语义，与 biz Usable() 一致）。
		if health == bizpub.AccountHealthHealthy {
			u = u.ClearPausedUntil()
		}
		if lastErr != nil {
			u = u.SetNillableLastError(lastErr)
		}
		return u
	}, "设置账号健康态")
}

// SetCredentialRef 回写凭据引用与类型（vault.Store 成功后调用）。
func (r *EntAccountRepo) SetCredentialRef(ctx context.Context, accountID, ref, credType string) error {
	return r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		ct := mappublishaccount.CredentialType(credType)
		return u.SetCredentialRef(ref).SetNillableCredentialType(&ct)
	}, "回写凭据引用")
}

// IncrementFailures 连续失败数 +1，返回新值。
func (r *EntAccountRepo) IncrementFailures(ctx context.Context, accountID string) (int, error) {
	if err := r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		return u.AddConsecutiveFailures(1)
	}, "递增连续失败数"); err != nil {
		return 0, err
	}
	acct, err := r.GetAccount(ctx, accountID)
	if err != nil {
		return 0, err
	}
	return acct.ConsecutiveFailures, nil
}

// ResetFailures 连续失败数清零。
func (r *EntAccountRepo) ResetFailures(ctx context.Context, accountID string) error {
	return r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		return u.SetConsecutiveFailures(0)
	}, "清零连续失败数")
}

// PauseAccount 熔断暂停：health=paused + paused_until（已脱敏 lastErr 一并记录）。
func (r *EntAccountRepo) PauseAccount(ctx context.Context, accountID string, until time.Time, lastErr *string) error {
	return r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		u = u.SetHealth(mappublishaccount.HealthPaused).SetPausedUntil(until.UTC())
		if lastErr != nil {
			u = u.SetNillableLastError(lastErr)
		}
		return u
	}, "熔断暂停账号")
}

// TouchLastPublished 发布成功后刷新 last_published_at 并快递增 published_24h（近似展示值；
// 配额判定以 receipts 时间窗计数为权威）。
func (r *EntAccountRepo) TouchLastPublished(ctx context.Context, accountID string, at time.Time) error {
	return r.updateByAccountID(ctx, accountID, func(u *ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate {
		return u.SetLastPublishedAt(at.UTC()).AddPublished24h(1)
	}, "刷新最近发布")
}

// updateByAccountID 通用按业务 ID 更新（影响 0 行视为不存在）。
func (r *EntAccountRepo) updateByAccountID(ctx context.Context, accountID string, apply func(*ent.MapPublishAccountUpdate) *ent.MapPublishAccountUpdate, op string) error {
	n, err := apply(r.cli.Client().MapPublishAccount.Update().Where(mappublishaccount.AccountIDEQ(accountID))).Save(ctx)
	if err != nil {
		r.log.Errorf("%s failed account=%s: %v", op, accountID, err)
		return fmt.Errorf("%w: %s失败", bizpub.ErrInternal, op)
	}
	if n == 0 {
		return fmt.Errorf("%w: 账号不存在", bizpub.ErrNotFound)
	}
	return nil
}

func accountToRow(e *ent.MapPublishAccount) *bizpub.AccountRow {
	return &bizpub.AccountRow{
		ID:                  uint32(e.ID),
		AccountID:           e.AccountID,
		ChannelID:           e.ChannelID,
		DisplayName:         e.DisplayName,
		HasCredential:       e.CredentialRef != nil && *e.CredentialRef != "",
		CredentialRef:       e.CredentialRef,
		CredentialType:      enumPtrToStr(e.CredentialType),
		Enabled:             e.Enabled,
		Health:              string(e.Health),
		ConsecutiveFailures: e.ConsecutiveFailures,
		PausedUntil:         e.PausedUntil,
		Published24h:        e.Published24h,
		LastPublishedAt:     e.LastPublishedAt,
		LastError:           e.LastError,
		CreateTime:          derefTime(e.CreateTime),
	}
}
