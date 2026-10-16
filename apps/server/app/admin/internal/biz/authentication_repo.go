package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"

	adminV1 "framework/api/gen/go/admin/v1"
	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"

	"framework/pkg/events"
	"framework/pkg/mfa"
	"framework/pkg/password"
)

// loginLockDisplayLoc 锁定截止时间展示时区。
// locked_until 经 ent 从 PG 扫回后携带 PG 会话时区偏移（44 为 +00），time.Format 按时间
// 自身 location 渲染——直接 Format 会把北京时间显示成 UTC（2026-08-27 44 冒烟实测：
// 实际 00:43 +0800 的锁定提示显示为 16:43 UTC）。全站展示口径统一 Asia/Shanghai；
// 部分容器 TZ/tzdata 不可靠，与 report/builder.go 同模式用 FixedZone 固定 +8。
var loginLockDisplayLoc = time.FixedZone("CST", 8*3600)

// formatLockUntil 锁定截止时间展示格式（统一 +08:00 时钟面）。
func formatLockUntil(until time.Time) string {
	return until.In(loginLockDisplayLoc).Format("2006-01-02 15:04:05")
}

// LoginFailTargetType 目标类型：IP 或 USER。
// 定义在 biz 层（端口入参），data 层以类型别名兼容旧引用。
type LoginFailTargetType string

const (
	LoginFailTargetIP   LoginFailTargetType = "IP"
	LoginFailTargetUser LoginFailTargetType = "USER"
)

// LoginFailIncrResult 登录失败计数递增结果。
type LoginFailIncrResult struct {
	FailCount   int       // 递增后的失败计数
	NewlyLocked bool      // 本次递增是否使目标从"未锁定"转为"锁定"（首次触阈）
	LockedUntil time.Time // 锁定时长截止（未触发锁定时为零值）
}

// LoginFailCounterRepo 登录失败计数器仓储端口（IP/账户双重锁定），由 data 层实现并经 wire.Bind 绑定。
type LoginFailCounterRepo interface {
	IsLocked(ctx context.Context, targetType LoginFailTargetType, targetValue string) (bool, time.Time, error)
	IncrFail(ctx context.Context, targetType LoginFailTargetType, targetValue string, lockCount int, lockMinutes int) (*LoginFailIncrResult, error)
	Reset(ctx context.Context, targetType LoginFailTargetType, targetValue string) error
}

// PasswordHistoryRepo 密码历史仓储端口（密码策略校验），由 data 层实现并经 wire.Bind 绑定。
type PasswordHistoryRepo interface {
	AddRecord(ctx context.Context, userID uint32, plainPassword string) error
	IsPasswordUsed(ctx context.Context, userID uint32, plainPassword string, historyCount int) (bool, error)
	CleanOldRecords(ctx context.Context, userID uint32, keepCount int) error
}

// AuthenticationUsecase 认证业务用例。
// 登录编排（黑白名单 → IP/账户锁定 → 密码 → MFA 二步 → 签发令牌）、
// 注册、改密、MFA 管理逻辑自 service 层整体下移，行为不变；
// 传输上下文提取（operator/clientIP/userAgent）保留在 service 边缘，以参数传入。
type AuthenticationUsecase struct {
	userRepo             UserRepo
	userCredentialRepo   UserCredentialRepo
	tenantRepo           TenantRepo
	userToken            UserTokenRepo
	loginRestrictionRepo AdminLoginRestrictionRepo
	loginFailCounterRepo LoginFailCounterRepo
	passwordHistoryRepo  PasswordHistoryRepo
	configUC             *SystemConfigUsecase
	publisher            EventPublisher

	log *log.Helper
}

func NewAuthenticationUsecase(
	userRepo UserRepo,
	userCredentialRepo UserCredentialRepo,
	tenantRepo TenantRepo,
	userToken UserTokenRepo,
	loginRestrictionRepo AdminLoginRestrictionRepo,
	loginFailCounterRepo LoginFailCounterRepo,
	passwordHistoryRepo PasswordHistoryRepo,
	configUC *SystemConfigUsecase,
	publisher EventPublisher,
	logger log.Logger,
) *AuthenticationUsecase {
	return &AuthenticationUsecase{
		userRepo:             userRepo,
		userCredentialRepo:   userCredentialRepo,
		tenantRepo:           tenantRepo,
		userToken:            userToken,
		loginRestrictionRepo: loginRestrictionRepo,
		loginFailCounterRepo: loginFailCounterRepo,
		passwordHistoryRepo:  passwordHistoryRepo,
		configUC:             configUC,
		publisher:            publisher,
		log:                  log.NewHelper(log.With(logger, "module", "authn/usecase/admin-service")),
	}
}

// LoginByPassword 处理授权类型 - 密码
// 流程：IP/用户黑白名单 → IP 锁定 → 账户锁定 → 密码 → MFA 二步 → 签发令牌
func (uc *AuthenticationUsecase) LoginByPassword(ctx context.Context, req *authenticationV1.LoginRequest, clientIP, userAgent string) (*authenticationV1.LoginResponse, error) {
	// 1. 登录限制校验：IP 黑白名单（密码校验前短路，避免无谓的 DB 查询与哈希计算）
	if err := uc.enforceLoginRestriction(ctx, adminV1.AdminLoginRestrictionTargetType_IP, clientIP); err != nil {
		return nil, err
	}
	// 2. 登录限制校验：用户黑白名单
	if err := uc.enforceLoginRestriction(ctx, adminV1.AdminLoginRestrictionTargetType_USER, req.GetUsername()); err != nil {
		return nil, err
	}

	// 3. IP 登录失败锁定校验
	if locked, until, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetIP, clientIP); err != nil {
		uc.log.Warnf("check ip lock failed (non-fatal): %v", err)
	} else if locked {
		return nil, authenticationV1.ErrorIpLocked("IP 已被锁定至 %s", formatLockUntil(until))
	}

	// 4. 账户登录失败锁定校验
	if locked, until, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetUser, req.GetUsername()); err != nil {
		uc.log.Warnf("check user lock failed (non-fatal): %v", err)
	} else if locked {
		return nil, authenticationV1.ErrorAccountLocked("账户已被锁定至 %s", formatLockUntil(until))
	}

	// 5. 密码校验
	if _, err := uc.userCredentialRepo.VerifyCredential(ctx, &authenticationV1.VerifyCredentialRequest{
		IdentityType: authenticationV1.IdentityType_USERNAME,
		Identifier:   req.GetUsername(),
		Credential:   req.GetPassword(),
		NeedDecrypt:  false,
	}); err != nil {
		// 密码错误：递增 IP + 账户失败计数
		uc.recordLoginFailure(ctx, clientIP, req.GetUsername())
		return nil, err
	}

	// 6. 获取用户信息
	user, err := uc.userRepo.GetUserByUserName(ctx, req.GetUsername())
	if err != nil {
		return nil, err
	}

	// 7. 验证权限
	if user.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return &authenticationV1.LoginResponse{}, authenticationV1.ErrorForbidden("权限不够")
	}

	// 8. MFA 二步校验
	mfaEnabled, mfaSecret, err := uc.userRepo.GetMFAStatus(ctx, user.GetId())
	if err != nil {
		// 安全属性查询必须 fail-closed：DB 抖动期放行 = MFA 控制静默失效
		uc.log.Errorf("query mfa status failed (fail-closed, login rejected): %v", err)
		return nil, authenticationV1.ErrorServiceUnavailable("MFA 状态查询失败，请稍后重试")
	}
	if mfaEnabled && mfaSecret != "" {
		// 8a. 请求携带 mfa_code → 校验并签发令牌
		if req.GetMfaCode() != "" {
			if !mfa.ValidateCode(req.GetMfaCode(), mfaSecret) {
				// MFA 验证码错误同样计入 IP+账户双维度失败计数（防"密码正确+MFA 爆破"绕过锁定）
				uc.recordLoginFailure(ctx, clientIP, req.GetUsername())
				return nil, authenticationV1.ErrorMfaCodeInvalid("MFA 验证码错误")
			}
			// 校验通过，进入签发令牌流程
		} else {
			// 8b. 未携带 mfa_code → 签发 MFA 临时令牌，引导前端二步
			meta := TokenMeta{
				ClientID:  req.GetClientId(),
				LoginIP:   clientIP,
				UserAgent: userAgent,
			}
			mfaToken, err := uc.userToken.GenerateMFAToken(ctx, user.GetId(), meta)
			if err != nil {
				return nil, authenticationV1.ErrorServiceUnavailable("generate mfa token failed")
			}
			return &authenticationV1.LoginResponse{
				MfaRequired: trans.Ptr(true),
				MfaToken:    trans.Ptr(mfaToken),
			}, nil
		}
	}

	// 9. 登录成功：重置 IP + 账户失败计数
	uc.resetLoginFailure(ctx, clientIP, req.GetUsername())

	// 10. 生成令牌
	meta := TokenMeta{
		ClientID:  req.GetClientId(),
		LoginIP:   clientIP,
		UserAgent: userAgent,
	}
	accessToken, refreshToken, err := uc.userToken.GenerateToken(ctx, user, meta)
	if err != nil {
		return nil, err
	}

	// 11. 回写最后登录时间/IP（此时登录已完整成功；MFA 二步场景在 VerifyMFA 中回写）
	uc.touchLastLogin(ctx, user.GetId(), clientIP)

	return &authenticationV1.LoginResponse{
		TokenType:    authenticationV1.TokenType_bearer.String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// touchLastLogin 登录成功回写最后登录时间/IP；失败仅记日志，不影响令牌签发。
func (uc *AuthenticationUsecase) touchLastLogin(ctx context.Context, userID uint32, ip string) {
	if err := uc.userRepo.UpdateLastLogin(ctx, userID, ip); err != nil {
		uc.log.Warnf("update last login failed (non-fatal): %v", err)
	}
}

// recordLoginFailure 登录失败时递增 IP + 账户失败计数。
// 锁定阈值与时长由 system_configs 运行时提供。
// 首次触阈（未锁定→锁定）时发布 security.login.abuse 撞库告警事件（monitoralarm 订阅转告警）。
func (uc *AuthenticationUsecase) recordLoginFailure(ctx context.Context, ip, username string) {
	lockCfg := uc.configUC.GetLoginLockConfig()
	if ip != "" {
		res, err := uc.loginFailCounterRepo.IncrFail(ctx, LoginFailTargetIP, ip, lockCfg.IPLockCount, lockCfg.IPLockMinutes)
		if err != nil {
			uc.log.Errorf("incr ip fail count failed: %v", err)
		} else if res != nil && res.NewlyLocked {
			uc.publishLoginAbuse(ctx, LoginFailTargetIP, ip, res, lockCfg.IPLockMinutes)
		}
	}
	if username != "" {
		res, err := uc.loginFailCounterRepo.IncrFail(ctx, LoginFailTargetUser, username, lockCfg.UserLockCount, lockCfg.UserLockMinutes)
		if err != nil {
			uc.log.Errorf("incr user fail count failed: %v", err)
		} else if res != nil && res.NewlyLocked {
			uc.publishLoginAbuse(ctx, LoginFailTargetUser, username, res, lockCfg.UserLockMinutes)
			// 锁定联动：账户首次触阈锁定即撤销其全部令牌（含 refresh/MFA），防止已持有会话继续可用
			if u, err := uc.userRepo.GetUserByUserName(ctx, username); err != nil {
				uc.log.Warnf("query locked user failed (skip token revoke): %v", err)
			} else if u != nil {
				if err := uc.userToken.RemoveToken(ctx, u.GetId()); err != nil {
					uc.log.Warnf("revoke locked user tokens failed (non-fatal): %v", err)
				}
			}
		}
	}
}

// publishLoginAbuse 发布撞库/暴力破解告警事件（security.login.abuse）。
// 级别口径：IP 锁定 warning（无差别扫射），账户锁定 critical（定向撞库）。
// 发布失败写重试队列，不影响登录响应。
func (uc *AuthenticationUsecase) publishLoginAbuse(ctx context.Context, targetType LoginFailTargetType, targetValue string, res *LoginFailIncrResult, lockMinutes int) {
	severity := "warning"
	if targetType == LoginFailTargetUser {
		severity = "critical"
	}
	uc.log.Warnf("login abuse detected: targetType=%s target=%s failCount=%d lockMinutes=%d",
		targetType, targetValue, res.FailCount, lockMinutes)
	PublishAssetEventWithRetry(ctx, uc.publisher, nil, uc.log, events.SubjectSecurityLoginAbuse, 0, nil, map[string]any{
		"target_type":  string(targetType),
		"target_value": targetValue,
		"fail_count":   res.FailCount,
		"lock_minutes": lockMinutes,
		"locked_until": res.LockedUntil.Format(time.RFC3339),
		"severity":     severity,
	})
}

// resetLoginFailure 登录成功后重置 IP + 账户失败计数。
func (uc *AuthenticationUsecase) resetLoginFailure(ctx context.Context, ip, username string) {
	if ip != "" {
		if err := uc.loginFailCounterRepo.Reset(ctx, LoginFailTargetIP, ip); err != nil {
			uc.log.Warnf("reset ip fail count failed (non-fatal): %v", err)
		}
	}
	if username != "" {
		if err := uc.loginFailCounterRepo.Reset(ctx, LoginFailTargetUser, username); err != nil {
			uc.log.Warnf("reset user fail count failed (non-fatal): %v", err)
		}
	}
}

// enforceLoginRestriction 登录限制校验：按目标类型加载规则，判定语义由纯函数 EvaluateLoginRestriction 承担。
// targetValue 为空时跳过（处理取不到 IP 等情况），避免误拦所有请求。
func (uc *AuthenticationUsecase) enforceLoginRestriction(ctx context.Context, targetType adminV1.AdminLoginRestrictionTargetType, targetValue string) error {
	if targetValue == "" {
		return nil
	}

	rules, err := uc.loginRestrictionRepo.ListByTargetType(ctx, targetType)
	if err != nil {
		return err
	}

	// 判定语义收敛到纯函数 EvaluateLoginRestriction（阻断优先/白名单模式/状态与过期过滤）
	decision := EvaluateLoginRestriction(rules, targetType, targetValue, time.Now())

	// 命中统计：决定性命中（ALLOW_MATCH/BLOCK_MATCH）异步累加 hit_count/last_hit_time。
	// ctx 脱钩（WithoutCancel）：请求结束后统计仍能落库；失败仅记日志，不影响登录判定。
	if decision.MatchedRule != nil {
		ruleID := decision.MatchedRule.GetId()
		go func() {
			hitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			if err := uc.loginRestrictionRepo.IncrementHit(hitCtx, ruleID); err != nil {
				uc.log.Warnf("increment login restriction hit failed (non-fatal): rule=%d err=%v", ruleID, err)
			}
		}()
	}

	if decision.Allowed {
		return nil
	}
	uc.log.Warnf("login restricted: targetType=%s target=%s verdict=%s matchedRuleId=%d",
		targetType.String(), targetValue, decision.Verdict.String(), decision.MatchedRule.GetId())
	return authenticationV1.ErrorLoginRestricted("%s", decision.Reason)
}

// LoginByRefreshToken 处理授权类型 - 刷新令牌。
// 免 JWT 链路：RefreshToken 接口在 access token 过期后调用，请求上下文无用户身份，
// 用户 ID 由 refresh_token 记录反查（同时完成存在/未撤销/未过期校验）。
func (uc *AuthenticationUsecase) LoginByRefreshToken(ctx context.Context, req *authenticationV1.LoginRequest, clientIP, userAgent string) (*authenticationV1.LoginResponse, error) {
	// 登录限制复查：IP（持有有效 refresh token 不豁免登录限制——被阻断后不得续期）
	if err := uc.enforceLoginRestriction(ctx, adminV1.AdminLoginRestrictionTargetType_IP, clientIP); err != nil {
		return nil, err
	}

	// IP 失败锁定复查：refresh token 不豁免撞库锁定（锁定期间不得续期）
	if locked, until, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetIP, clientIP); err != nil {
		uc.log.Warnf("check ip lock failed (non-fatal): %v", err)
	} else if locked {
		return nil, authenticationV1.ErrorIpLocked("IP 已被锁定至 %s", formatLockUntil(until))
	}

	// 校验刷新令牌并反查用户 ID
	userID, ok := uc.userToken.GetUserIDByRefreshToken(ctx, req.GetRefreshToken())
	if !ok {
		return nil, authenticationV1.ErrorIncorrectRefreshToken("invalid refresh token")
	}

	// 获取用户信息
	user, err := uc.userRepo.Get(ctx, userID)
	if err != nil {
		return &authenticationV1.LoginResponse{}, err
	}

	// 登录限制复查：USER（用户被加入阻断/移出白名单后，refresh token 一并失效）
	if err := uc.enforceLoginRestriction(ctx, adminV1.AdminLoginRestrictionTargetType_USER, user.GetUsername()); err != nil {
		return nil, err
	}

	// 账户失败锁定复查：refresh token 不豁免账户锁定（锁定期间不得续期）
	if locked, until, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetUser, user.GetUsername()); err != nil {
		uc.log.Warnf("check user lock failed (non-fatal): %v", err)
	} else if locked {
		return nil, authenticationV1.ErrorAccountLocked("账户已被锁定至 %s", formatLockUntil(until))
	}

	if err = uc.userToken.RemoveRefreshToken(ctx, userID, req.GetRefreshToken()); err != nil {
		uc.log.Errorf("remove refresh token failed [%s]", err.Error())
	}

	// 生成令牌
	meta := TokenMeta{
		ClientID:  req.GetClientId(),
		LoginIP:   clientIP,
		UserAgent: userAgent,
	}
	accessToken, refreshToken, err := uc.userToken.GenerateToken(ctx, user, meta)
	if err != nil {
		return nil, authenticationV1.ErrorServiceUnavailable("generate token failed")
	}

	return &authenticationV1.LoginResponse{
		TokenType:    authenticationV1.TokenType_bearer.String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// Logout 登出：撤销用户全部令牌。
func (uc *AuthenticationUsecase) Logout(ctx context.Context, userID uint32) error {
	return uc.userToken.RemoveToken(ctx, userID)
}

// RegisterUser 注册用户（CUSTOMER_USER 权限，可按 tenantCode 挂靠租户）。
func (uc *AuthenticationUsecase) RegisterUser(ctx context.Context, req *authenticationV1.RegisterUserRequest) (*authenticationV1.RegisterUserResponse, error) {
	var err error

	var tenantId *uint32
	if req.GetTenantCode() != "" {
		tenant, err := uc.tenantRepo.GetTenantByTenantCode(ctx, req.GetTenantCode())
		if err != nil {
			return nil, err
		}
		if tenant != nil {
			tenantId = tenant.Id
		}
	}

	user, err := uc.userRepo.Create(ctx, &userV1.CreateUserRequest{
		Data: &userV1.User{
			TenantId:  tenantId,
			Username:  trans.Ptr(req.Username),
			Email:     req.Email,
			Authority: trans.Ptr(userV1.UserAuthority_CUSTOMER_USER),
			Status:    trans.Ptr(userV1.UserStatus_ON),
		},
	})
	if err != nil {
		uc.log.Errorf("create user error: %v", err)
		return nil, err
	}

	if err = uc.userCredentialRepo.Create(ctx, &authenticationV1.CreateUserCredentialRequest{
		Data: &authenticationV1.UserCredential{
			UserId:   user.Id,
			TenantId: user.TenantId,

			IdentityType: authenticationV1.IdentityType_USERNAME.Enum(),
			Identifier:   trans.Ptr(req.GetUsername()),

			CredentialType: authenticationV1.CredentialType_PASSWORD_HASH.Enum(),
			Credential:     trans.Ptr(req.GetPassword()),

			IsPrimary: trans.Ptr(true),
			Status:    authenticationV1.UserCredentialStatus_ENABLED.Enum(),
		},
	}); err != nil {
		uc.log.Errorf("create user credentials error: %v", err)
		return nil, err
	}

	return &authenticationV1.RegisterUserResponse{
		UserId: user.GetId(),
	}, nil
}

// ChangePassword 修改用户密码
// 流程：复杂度校验 → 历史密码校验 → 旧密码校验+新密码哈希落库 → 写入密码历史 → 更新密码时间戳
func (uc *AuthenticationUsecase) ChangePassword(ctx context.Context, req *authenticationV1.ChangePasswordRequest) error {
	policy := uc.configUC.GetPasswordPolicy()

	// 1. 复杂度校验
	if err := password.ValidateComplexity(req.GetNewPassword(), policy); err != nil {
		return authenticationV1.ErrorPasswordTooWeak("%s", err.Error())
	}

	// 2. 解析目标用户：req.UserId 优先（管理员重置），否则按 username 查
	username := req.GetUsername()
	var userID uint32
	if req.GetUserId() != 0 {
		userID = req.GetUserId()
	} else if username != "" {
		user, err := uc.userRepo.GetUserByUserName(ctx, username)
		if err != nil {
			return err
		}
		userID = user.GetId()
	}

	// 3. 历史密码校验（防止重复使用最近 N 次密码）
	if userID != 0 && policy.HistoryCheckCount > 0 {
		used, err := uc.passwordHistoryRepo.IsPasswordUsed(ctx, userID, req.GetNewPassword(), policy.HistoryCheckCount)
		if err != nil {
			uc.log.Warnf("check password history failed (non-fatal): %v", err)
		} else if used {
			return authenticationV1.ErrorPasswordReused("新密码与最近使用过的历史密码重复")
		}
	}

	// 4. 旧密码校验 + 新密码哈希落库
	if err := uc.userCredentialRepo.ChangeCredential(ctx, &authenticationV1.ChangeCredentialRequest{
		IdentityType:  authenticationV1.IdentityType_USERNAME,
		Identifier:    username,
		OldCredential: req.GetOldPassword(),
		NewCredential: req.GetNewPassword(),
	}); err != nil {
		return err
	}

	// 5. 写入密码历史（AddRecord 内部用 passwordCrypto 哈希后落库）
	if userID != 0 {
		if err := uc.passwordHistoryRepo.AddRecord(ctx, userID, req.GetNewPassword()); err != nil {
			uc.log.Warnf("add password history failed (non-fatal): %v", err)
		}
		// 6. 更新密码修改时间与过期时间
		if err := uc.userRepo.UpdatePasswordTimestamps(ctx, userID, policy.ExpireDays); err != nil {
			uc.log.Warnf("update password timestamps failed (non-fatal): %v", err)
		}
		// 7. 清理超出保留数量的旧历史记录
		if err := uc.passwordHistoryRepo.CleanOldRecords(ctx, userID, policy.HistoryCheckCount*2); err != nil {
			uc.log.Warnf("clean old password history failed (non-fatal): %v", err)
		}
	}

	return nil
}

// VerifyMFA MFA 二步登录：校验 TOTP 验证码并签发访问令牌
func (uc *AuthenticationUsecase) VerifyMFA(ctx context.Context, req *authenticationV1.VerifyMFARequest, clientIP, userAgent string) (*authenticationV1.LoginResponse, error) {
	// 0. 登录限制复查：IP（MFA 临时令牌不豁免登录限制——一步与二步之间规则可能已变更，或伪造来源重放）
	if err := uc.enforceLoginRestriction(ctx, adminV1.AdminLoginRestrictionTargetType_IP, clientIP); err != nil {
		return nil, err
	}

	// 0.5 IP 失败锁定复查：MFA 临时令牌不豁免撞库锁定
	if locked, until, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetIP, clientIP); err != nil {
		uc.log.Warnf("check ip lock failed (non-fatal): %v", err)
	} else if locked {
		return nil, authenticationV1.ErrorIpLocked("IP 已被锁定至 %s", formatLockUntil(until))
	}

	// 1. 校验并消费 MFA 临时令牌
	userID, err := uc.userToken.ConsumeMFAToken(ctx, req.GetMfaToken())
	if err != nil {
		return nil, authenticationV1.ErrorServiceUnavailable("validate mfa token failed")
	}
	if userID == 0 {
		return nil, authenticationV1.ErrorUnauthorized("invalid or expired mfa token")
	}

	// 2. 读取用户 MFA 密钥
	enabled, secret, err := uc.userRepo.GetMFAStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !enabled || secret == "" {
		return nil, authenticationV1.ErrorBadRequest("mfa not enabled")
	}

	// 2.5 获取用户信息（前移至验证码校验前：MFA 失败需按账户维度计数）
	user, err := uc.userRepo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	// 3. 账户失败锁定复查（前移至验证码校验前：锁定期间对正确/错误验证码须返回同质错误，
	// 否则锁定状态下正确码得"账户已锁定"、错误码得"验证码错误"，构成 TOTP 爆破 oracle）
	if locked, until, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetUser, user.GetUsername()); err != nil {
		uc.log.Warnf("check user lock failed (non-fatal): %v", err)
	} else if locked {
		return nil, authenticationV1.ErrorAccountLocked("账户已被锁定至 %s", formatLockUntil(until))
	}

	// 4. 校验 TOTP 验证码（失败计入 IP+账户双维度失败计数，防 MFA 爆破绕过密码失败锁定）
	if !mfa.ValidateCode(req.GetMfaCode(), secret) {
		uc.recordLoginFailure(ctx, clientIP, user.GetUsername())
		return nil, authenticationV1.ErrorMfaCodeInvalid("MFA 验证码错误")
	}

	// 5. 登录限制复查：USER（MFA 一步通过后用户可能已被加入阻断/移出白名单）
	if err := uc.enforceLoginRestriction(ctx, adminV1.AdminLoginRestrictionTargetType_USER, user.GetUsername()); err != nil {
		return nil, err
	}

	// 6. 登录成功：重置 IP + 账户失败计数
	uc.resetLoginFailure(ctx, clientIP, user.GetUsername())

	meta := TokenMeta{
		LoginIP:   clientIP,
		UserAgent: userAgent,
	}
	accessToken, refreshToken, err := uc.userToken.GenerateToken(ctx, user, meta)
	if err != nil {
		return nil, authenticationV1.ErrorServiceUnavailable("generate token failed")
	}

	// 5. 回写最后登录时间/IP（MFA 二步登录的最终成功点）
	uc.touchLastLogin(ctx, userID, clientIP)

	return &authenticationV1.LoginResponse{
		TokenType:    authenticationV1.TokenType_bearer.String(),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// EnableMFA 发起 MFA 绑定：生成 TOTP 密钥与 otpauth URL（仅写入密钥，mfa_enabled=false）。
// 用户扫码后须调用 ConfirmMFA 校验首个验证码，通过后才正式启用（防误绑/扫错码即锁死账户）。
func (uc *AuthenticationUsecase) EnableMFA(ctx context.Context, userID uint32) (*authenticationV1.EnableMFAResponse, error) {
	// 已启用防护：SetMFASecret 会覆盖密钥并置 mfa_enabled=false，
	// 若不拦截，会话持有者可零验证码覆盖已绑定 MFA（劫持/降级窗口）。
	// 换绑必须显式走 DisableMFA（需持有效验证码）→ 重新绑定。
	enabled, _, err := uc.userRepo.GetMFAStatus(ctx, userID)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, authenticationV1.ErrorBadRequest("MFA 已启用，请先禁用后再重新绑定")
	}

	// 读取用户名用于 otpauth account
	user, err := uc.userRepo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	account := user.GetUsername()
	if account == "" {
		account = user.GetNickname()
	}

	secret, otpauthURL, err := mfa.GenerateSecret(account)
	if err != nil {
		return nil, authenticationV1.ErrorServiceUnavailable("generate mfa secret failed")
	}

	// 仅落密钥不启用（SetMFASecret 内部置 mfa_enabled=false），启用动作收口到 ConfirmMFA。
	if err := uc.userRepo.SetMFASecret(ctx, userID, secret); err != nil {
		return nil, err
	}

	return &authenticationV1.EnableMFAResponse{
		Secret:     secret,
		OtpauthUrl: otpauthURL,
	}, nil
}

// ConfirmMFA 确认 MFA 绑定：校验首个 TOTP 验证码，通过后正式启用。
// 前置：须先经 EnableMFA 落密钥；已启用时拒绝重复确认（防重放覆盖语义混淆）。
// 爆破防护：TOTP 有效域仅 10^6，失败计入账户维度锁定（复用登录失败计数器）。
func (uc *AuthenticationUsecase) ConfirmMFA(ctx context.Context, userID uint32, mfaCode string) error {
	enabled, secret, err := uc.userRepo.GetMFAStatus(ctx, userID)
	if err != nil {
		return err
	}
	if enabled {
		return authenticationV1.ErrorBadRequest("MFA 已启用，无需重复确认")
	}
	if secret == "" {
		return authenticationV1.ErrorBadRequest("请先生成 MFA 密钥")
	}
	if err := uc.checkMFAAttemptLocked(ctx, userID); err != nil {
		return err
	}
	if !mfa.ValidateCode(mfaCode, secret) {
		uc.recordMFAAttemptFailure(ctx, userID)
		return authenticationV1.ErrorMfaCodeInvalid("MFA 验证码错误")
	}
	uc.resetMFAAttempt(ctx, userID)
	return uc.userRepo.EnableMFA(ctx, userID)
}

// DisableMFA 禁用 MFA：校验 TOTP 验证码后关闭（失败计数口径同 ConfirmMFA）
func (uc *AuthenticationUsecase) DisableMFA(ctx context.Context, userID uint32, mfaCode string) error {
	enabled, secret, err := uc.userRepo.GetMFAStatus(ctx, userID)
	if err != nil {
		return err
	}
	if !enabled {
		// 半绑定态（EnableMFA 落了密钥但未经 ConfirmMFA 启用）：一并清掉滞留密钥，
		// 避免旧密钥此后仍可被 ConfirmMFA 启用。
		if secret != "" {
			return uc.userRepo.DisableMFA(ctx, userID)
		}
		return nil
	}
	if err := uc.checkMFAAttemptLocked(ctx, userID); err != nil {
		return err
	}
	if !mfa.ValidateCode(mfaCode, secret) {
		uc.recordMFAAttemptFailure(ctx, userID)
		return authenticationV1.ErrorMfaCodeInvalid("MFA 验证码错误")
	}
	uc.resetMFAAttempt(ctx, userID)
	return uc.userRepo.DisableMFA(ctx, userID)
}

// mfaAttemptUsername 取 MFA 失败计数的账户维度键（username；取不到时退化为 userID 串，
// 保证计数永不因用户查询失败而静默失效——安全控制 fail-closed 到底）。
func (uc *AuthenticationUsecase) mfaAttemptUsername(ctx context.Context, userID uint32) string {
	user, err := uc.userRepo.Get(ctx, userID)
	if err != nil || user.GetUsername() == "" {
		return fmt.Sprintf("uid:%d", userID)
	}
	return user.GetUsername()
}

// checkMFAAttemptLocked 账户维度锁定检查：锁定期间拒绝 MFA 确认/禁用操作。
func (uc *AuthenticationUsecase) checkMFAAttemptLocked(ctx context.Context, userID uint32) error {
	locked, _, err := uc.loginFailCounterRepo.IsLocked(ctx, LoginFailTargetUser, uc.mfaAttemptUsername(ctx, userID))
	if err != nil {
		uc.log.Warnf("check mfa attempt lock failed (non-fatal): %v", err)
		return nil
	}
	if locked {
		return authenticationV1.ErrorForbidden("失败次数过多，账户已锁定，请稍后再试")
	}
	return nil
}

// recordMFAAttemptFailure 记录 MFA 确认/禁用失败（仅账户维度；IP 维度由登录链覆盖）。
func (uc *AuthenticationUsecase) recordMFAAttemptFailure(ctx context.Context, userID uint32) {
	uc.recordLoginFailure(ctx, "", uc.mfaAttemptUsername(ctx, userID))
}

// resetMFAAttempt MFA 操作成功后清零账户失败计数。
func (uc *AuthenticationUsecase) resetMFAAttempt(ctx context.Context, userID uint32) {
	uc.resetLoginFailure(ctx, "", uc.mfaAttemptUsername(ctx, userID))
}
