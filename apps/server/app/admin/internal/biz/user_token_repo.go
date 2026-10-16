package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "framework/api/gen/go/admin/v1"
	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"
)

// TokenMeta 令牌元信息（登录时从请求上下文提取）。
// 定义在 biz 层（端口入参），data 层以类型别名兼容旧引用。
type TokenMeta struct {
	ClientID      string
	LoginIP       string
	LoginLocation string
	UserAgent     string
}

// OnlineUserInfo 在线用户信息。
// 定义在 biz 层（端口出参），data 层以类型别名兼容旧引用。
type OnlineUserInfo struct {
	UserID        uint32
	Username      string
	Nickname      string
	DeptID        uint32
	Token         string
	ClientID      string
	LoginIP       string
	LoginLocation string
	UserAgent     string
	LoginTime     time.Time
	ExpiresAt     time.Time
}

// UserTokenRepo 用户令牌仓储端口，由 data 层 UserTokenCacheRepo 实现并经 wire.Bind 绑定。
type UserTokenRepo interface {
	// GenerateToken 创建访问令牌 + 刷新令牌
	GenerateToken(ctx context.Context, user *userV1.User, meta TokenMeta) (accessToken string, refreshToken string, err error)
	// GenerateAccessToken 创建访问令牌（含会话并发限制）
	GenerateAccessToken(ctx context.Context, user *userV1.User, meta TokenMeta) (accessToken string, err error)
	// GenerateRefreshToken 创建刷新令牌
	GenerateRefreshToken(ctx context.Context, user *userV1.User, meta TokenMeta) (refreshToken string, err error)
	// RemoveToken 撤销用户全部令牌（登出）
	RemoveToken(ctx context.Context, userId uint32) error
	// RemoveAccessToken 撤销指定访问令牌
	RemoveAccessToken(ctx context.Context, userId uint32, accessToken string) error
	// RemoveRefreshToken 撤销指定刷新令牌
	RemoveRefreshToken(ctx context.Context, userId uint32, refreshToken string) error
	// GenerateMFAToken 签发 MFA 临时令牌（5 分钟有效，一次性）
	GenerateMFAToken(ctx context.Context, userId uint32, meta TokenMeta) (string, error)
	// ConsumeMFAToken 校验并消费 MFA 临时令牌，返回关联用户 ID（无效返回 0）
	ConsumeMFAToken(ctx context.Context, mfaToken string) (uint32, error)
	// RevokeAllUserAccessTokens 撤销用户所有访问令牌（强制下线）
	RevokeAllUserAccessTokens(ctx context.Context, userId uint32) error
	// IsExistAccessToken 访问令牌是否有效（存在、未过期、未撤销）
	IsExistAccessToken(ctx context.Context, userId uint32, accessToken string) bool
	// IsExistRefreshToken 刷新令牌是否存在
	IsExistRefreshToken(ctx context.Context, userId uint32, refreshToken string) bool
	// GetUserIDByRefreshToken 按刷新令牌反查用户 ID（存在、未撤销、未过期）；无效返回 (0, false)。
	// 用于免 JWT 的刷新链路：access token 过期后请求上下文无用户身份，需由 refresh_token 解析
	GetUserIDByRefreshToken(ctx context.Context, refreshToken string) (uint32, bool)
	// ListOnlineUsers 查询在线用户（access token 未撤销且未过期）
	ListOnlineUsers(ctx context.Context) ([]OnlineUserInfo, error)
	// DeleteExpiredTokens 物理删除 expires_at 早于 before 的令牌记录（含已撤销），返回删除条数。
	// 仅删过期且过宽限期的历史数据，不影响任何有效会话。
	DeleteExpiredTokens(ctx context.Context, before time.Time) (int64, error)
}

// SessionUsecase 会话管理业务用例：在线用户查询、强制下线（逻辑自 service 层下移，行为不变）。
type SessionUsecase struct {
	userToken UserTokenRepo
	userRepo  UserRepo
	log       *log.Helper
}

func NewSessionUsecase(userToken UserTokenRepo, userRepo UserRepo, logger log.Logger) *SessionUsecase {
	return &SessionUsecase{
		userToken: userToken,
		userRepo:  userRepo,
		log:       log.NewHelper(log.With(logger, "module", "session/usecase/admin-service")),
	}
}

// ListOnline 查询在线用户列表。
// user_tokens 表仅记录登录元信息，username/nickname 通过 user 表批量补充；
// dept_id 由 ListDeptIDs 批量补充（proto User DTO 的 OrgId 与 ent DeptID 名称不匹配，copier 不自动映射）。
func (uc *SessionUsecase) ListOnline(ctx context.Context) (*adminV1.ListOnlineUserResponse, error) {
	onlineUsers, err := uc.userToken.ListOnlineUsers(ctx)
	if err != nil {
		uc.log.Errorf("list online users failed: %v", err)
		return nil, authenticationV1.ErrorInternalServerError("list online users failed")
	}

	// 用户信息缓存（同用户多 token 时避免重复查询）
	userCache := make(map[uint32]*userV1.User, len(onlineUsers))

	// 批量部门映射：一次查询避免 N+1
	deptIDs := make(map[uint32]uint32, len(onlineUsers))
	uniqueIDs := make([]uint32, 0, len(onlineUsers))
	seen := make(map[uint32]struct{}, len(onlineUsers))
	for _, u := range onlineUsers {
		if u.UserID == 0 {
			continue
		}
		if _, ok := seen[u.UserID]; !ok {
			seen[u.UserID] = struct{}{}
			uniqueIDs = append(uniqueIDs, u.UserID)
		}
	}
	if len(uniqueIDs) > 0 {
		if m, err := uc.userRepo.ListDeptIDs(ctx, uniqueIDs); err != nil {
			uc.log.Warnf("list user dept ids failed (non-fatal): %v", err)
		} else {
			deptIDs = m
		}
	}

	items := make([]*adminV1.OnlineUser, 0, len(onlineUsers))
	for _, u := range onlineUsers {
		item := &adminV1.OnlineUser{
			Token:         trans.Ptr(maskToken(u.Token)),
			ClientId:      trans.Ptr(u.ClientID),
			LoginIp:       trans.Ptr(u.LoginIP),
			LoginLocation: trans.Ptr(u.LoginLocation),
			UserAgent:     trans.Ptr(u.UserAgent),
		}
		if u.UserID > 0 {
			item.UserId = trans.Ptr(u.UserID)
			if deptID, ok := deptIDs[u.UserID]; ok {
				item.DeptId = trans.Ptr(deptID)
			}
			if cached, ok := userCache[u.UserID]; ok {
				item.Username = cached.Username
				item.Nickname = cached.Nickname
			} else {
				user, err := uc.userRepo.Get(ctx, u.UserID)
				if err != nil {
					uc.log.Warnf("get user %d failed (non-fatal): %v", u.UserID, err)
				} else {
					userCache[u.UserID] = user
					item.Username = user.Username
					item.Nickname = user.Nickname
				}
			}
		}
		if !u.LoginTime.IsZero() {
			item.LoginTime = timestamppb.New(u.LoginTime)
		}
		if !u.ExpiresAt.IsZero() {
			item.ExpiresAt = timestamppb.New(u.ExpiresAt)
		}
		items = append(items, item)
	}

	return &adminV1.ListOnlineUserResponse{
		Items: items,
		Total: uint32(len(items)),
	}, nil
}

// ForceOffline 强制下线：撤销指定用户的所有 access token。
func (uc *SessionUsecase) ForceOffline(ctx context.Context, userID uint32) error {
	if userID == 0 {
		return authenticationV1.ErrorBadRequest("invalid user id")
	}
	if err := uc.userToken.RevokeAllUserAccessTokens(ctx, userID); err != nil {
		uc.log.Errorf("force offline user %d failed: %v", userID, err)
		return authenticationV1.ErrorInternalServerError("force offline failed")
	}
	return nil
}

// expiredTokenGracePeriod 过期令牌保留宽限期：过期 7 天内仍保留（供登录日志/审计追溯），
// 超过宽限期才物理删除。由内置任务 user_token_cleanup 每日触发。
const expiredTokenGracePeriod = 7 * 24 * time.Hour

// CleanupExpiredTokens 清理过期超过宽限期的令牌记录，返回删除条数。
// 由 asynq 内置周期任务 user_token_cleanup 调用（处理器在 server 层注册）。
func (uc *SessionUsecase) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	before := time.Now().Add(-expiredTokenGracePeriod)
	deleted, err := uc.userToken.DeleteExpiredTokens(ctx, before)
	if err != nil {
		uc.log.Errorf("cleanup expired tokens failed: %v", err)
		return 0, authenticationV1.ErrorInternalServerError("cleanup expired tokens failed")
	}
	uc.log.Infof("cleanup expired tokens done: deleted=%d (before %s)", deleted, before.Format(time.DateTime))
	return deleted, nil
}

// maskToken 脱敏显示访问令牌（仅保留前 8 位 + 后 4 位）。
func maskToken(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:8] + "****" + token[len(token)-4:]
}
