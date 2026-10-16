package data

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/password"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/passwordhistory"
)

// PasswordHistoryRepo 管理用户历史密码，用于密码策略校验（防止重复使用最近 N 次密码）。
type PasswordHistoryRepo struct {
	data          *Data
	log           *log.Helper
	passwordCrypto password.Crypto
}

func NewPasswordHistoryRepo(data *Data, logger log.Logger, crypto password.Crypto) *PasswordHistoryRepo {
	return &PasswordHistoryRepo{
		data:           data,
		log:            log.NewHelper(log.With(logger, "module", "password-history/repo/admin-service")),
		passwordCrypto: crypto,
	}
}

// AddRecord 写入一条密码历史记录。
// 接收明文密码，内部用 passwordCrypto 哈希后落库（bcrypt salt 随机，与 user_credentials 表的哈希相互独立，不影响校验）。
func (r *PasswordHistoryRepo) AddRecord(ctx context.Context, userID uint32, plainPassword string) error {
	hashed, err := r.passwordCrypto.Encrypt(plainPassword)
	if err != nil {
		return fmt.Errorf("hash password for history failed: %w", err)
	}
	return r.data.db.Client().PasswordHistory.Create().
		SetUserID(userID).
		SetPassword(hashed).
		Exec(ctx)
}

// IsPasswordUsed 检查明文密码是否在最近 historyCount 条历史记录中被使用过。
func (r *PasswordHistoryRepo) IsPasswordUsed(ctx context.Context, userID uint32, plainPassword string, historyCount int) (bool, error) {
	if historyCount <= 0 {
		return false, nil
	}

	records, err := r.data.db.Client().PasswordHistory.Query().
		Where(passwordhistory.UserIDEQ(userID)).
		Order(ent.Desc(passwordhistory.FieldCreateTime)).
		Limit(historyCount).
		All(ctx)
	if err != nil {
		return false, fmt.Errorf("query password history failed: %w", err)
	}

	for _, rec := range records {
		matched, err := r.passwordCrypto.Verify(plainPassword, rec.Password)
		if err != nil {
			r.log.Warnf("verify password history failed (non-fatal): %v", err)
			continue
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// CleanOldRecords 清理超过保留数量的旧历史记录（保留最近 keepCount 条）。
func (r *PasswordHistoryRepo) CleanOldRecords(ctx context.Context, userID uint32, keepCount int) error {
	if keepCount <= 0 {
		return nil
	}

	// 查询该用户全部历史记录 ID（按时间降序）
	ids, err := r.data.db.Client().PasswordHistory.Query().
		Where(passwordhistory.UserIDEQ(userID)).
		Order(ent.Desc(passwordhistory.FieldCreateTime)).
		Limit(keepCount).
		IDs(ctx)
	if err != nil {
		return fmt.Errorf("query recent password history ids failed: %w", err)
	}

	if len(ids) < keepCount {
		return nil // 记录数不足，无需清理
	}

	// 删除不在保留列表中的旧记录
	_, err = r.data.db.Client().PasswordHistory.Delete().
		Where(
			passwordhistory.UserIDEQ(userID),
			passwordhistory.IDNotIn(ids...),
		).Exec(ctx)
	if err != nil {
		return fmt.Errorf("clean old password history failed: %w", err)
	}
	return nil
}

// UpdatePasswordChangedAt 辅助方法：在 user 表更新密码变更时间（由 service 层调用 userRepo）。
// 这里仅提供时间戳生成，不直接操作 user 表。
func PasswordChangedAt() time.Time {
	return time.Now()
}
