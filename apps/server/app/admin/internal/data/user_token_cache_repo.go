package data

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/lib/pq"

	authnEngine "github.com/tx7do/kratos-authn/engine"

	userV1 "framework/api/gen/go/user/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/usertoken"

	"framework/pkg/jwt"
)

// pgUniqueViolationCode PostgreSQL 唯一约束冲突的 SQLSTATE 代码（与 locale 无关）。
// ent 的 sqlgraph.IsUniqueConstraintError 仅匹配英文错误串，中文 locale 下会漏判，
// 故在此补一层基于错误码的识别，保证 storeToken 在任意 locale 下都能正确幂等。
const pgUniqueViolationCode = "23505"

// isPgUniqueViolation 判断错误是否为 PostgreSQL 唯一约束冲突。
// 兼容 ent.ConstraintError 包装与 pq 原生错误两种返回路径。
func isPgUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == pgUniqueViolationCode
	}
	// ent.ConstraintError 通过 Unwrap 暴露底层 pq.Error，errors.As 会穿透包装。
	return false
}

// tokenExistsPositiveTTL "令牌有效"结论的进程内缓存时长：稳态下每活跃用户每 30s 仅 1 次 DB 校验。
// JWT 自身 exp 由签名解析兜底，缓存不会让过期令牌复活。
const tokenExistsPositiveTTL = 30 * time.Second

// tokenExistsNegativeTTL "令牌无效/已撤销"结论的缓存时长（较短，控制撤销生效延迟、防无效令牌重放打库）。
const tokenExistsNegativeTTL = 5 * time.Second

// tokenExistsCacheMax 缓存条目上限，超限整体清空重建（活跃会话量级小，重建成本低）。
const tokenExistsCacheMax = 4096

// tokenExistsCacheEntry 令牌存在性校验结论缓存。
type tokenExistsCacheEntry struct {
	valid     bool
	expiresAt time.Time
}

// UserTokenCacheRepo 基于 ent 的用户令牌仓储，替代原 Redis 实现。
// access 令牌为 JWT，refresh 令牌为 UUID；均落 user_tokens 表，
// 支持 登录签发 / 登出撤销 / 服务端校验令牌是否仍有效 / Token 黑名单 / 会话并发限制 / 在线用户查询。
type UserTokenCacheRepo struct {
	log *log.Helper

	data *Data

	authenticator authnEngine.Authenticator // 身份验证器

	accessTokenExpires  time.Duration // 访问令牌过期时间，<=0 表示不过期
	refreshTokenExpires time.Duration // 刷新令牌过期时间，<=0 表示不过期

	// existsCache 每请求吊销检查（auth 中间件 IsExistAccessToken）的 TTL 缓存，
	// 避免每个 API 请求打一次 user_tokens 表；显式撤销路径即时清除该用户全部缓存。
	existsMu    sync.RWMutex
	existsCache map[string]tokenExistsCacheEntry
}

func NewUserTokenCacheRepo(
	logger log.Logger,
	data *Data,
	authenticator authnEngine.Authenticator,
	accessTokenExpires time.Duration,
	refreshTokenExpires time.Duration,
) *UserTokenCacheRepo {
	l := log.NewHelper(log.With(logger, "module", "user-token/cache"))
	return &UserTokenCacheRepo{
		data:                data,
		log:                 l,
		authenticator:       authenticator,
		accessTokenExpires:  accessTokenExpires,
		refreshTokenExpires: refreshTokenExpires,
		existsCache:         make(map[string]tokenExistsCacheEntry),
	}
}

// existsCacheKey 缓存键：userId|tokenType|sha256(token)。token 为长 JWT，哈希收敛内存。
func existsCacheKey(userId uint32, token string, tokenType usertoken.TokenType) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%d|%s|%x", userId, tokenType, sum)
}

// getExistsCache 读缓存；过期视为未命中。
func (r *UserTokenCacheRepo) getExistsCache(key string) (bool, bool) {
	r.existsMu.RLock()
	entry, ok := r.existsCache[key]
	r.existsMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return false, false
	}
	return entry.valid, true
}

// putExistsCache 写缓存；超上限时整体清空重建。
func (r *UserTokenCacheRepo) putExistsCache(key string, valid bool) {
	ttl := tokenExistsPositiveTTL
	if !valid {
		ttl = tokenExistsNegativeTTL
	}
	r.existsMu.Lock()
	if len(r.existsCache) >= tokenExistsCacheMax {
		r.existsCache = make(map[string]tokenExistsCacheEntry, tokenExistsCacheMax/2)
	}
	r.existsCache[key] = tokenExistsCacheEntry{valid: valid, expiresAt: time.Now().Add(ttl)}
	r.existsMu.Unlock()
}

// purgeExistsCacheForUser 显式撤销（登出/强制下线/改密）后即时清除该用户全部缓存条目，
// 保证主动撤销语义即时生效，TTL 仅兜底自然过期等被动失效场景。
func (r *UserTokenCacheRepo) purgeExistsCacheForUser(userId uint32) {
	prefix := fmt.Sprintf("%d|", userId)
	r.existsMu.Lock()
	for key := range r.existsCache {
		if strings.HasPrefix(key, prefix) {
			delete(r.existsCache, key)
		}
	}
	r.existsMu.Unlock()
}

// TokenMeta 令牌元信息（登录时从请求上下文提取）。
// 类型本体已上移到 biz 层（端口入参），此处保留别名兼容。
type TokenMeta = biz.TokenMeta

// GenerateToken 创建令牌
func (r *UserTokenCacheRepo) GenerateToken(ctx context.Context, user *userV1.User, meta TokenMeta) (accessToken string, refreshToken string, err error) {
	// 创建访问令牌
	accessToken, err = r.GenerateAccessToken(ctx, user, meta)
	if err != nil {
		return
	}
	if accessToken == "" {
		err = errors.New("create access token failed")
		return
	}

	// 创建刷新令牌
	refreshToken, err = r.GenerateRefreshToken(ctx, user, meta)
	if err != nil {
		return
	}
	if refreshToken == "" {
		err = errors.New("create refresh token failed")
		return
	}

	return
}

// GenerateAccessToken 创建访问令牌。
// 会话并发限制：若用户已有 maxConcurrent 个有效 access token，则撤销最早的。
func (r *UserTokenCacheRepo) GenerateAccessToken(ctx context.Context, user *userV1.User, meta TokenMeta) (accessToken string, err error) {
	// 并发限制：超出 maxConcurrent 时撤销最早的 access token
	maxConcurrent := r.getMaxConcurrent(ctx, user.GetId())
	if maxConcurrent > 0 {
		if err = r.enforceConcurrentLimit(ctx, user.GetId(), maxConcurrent); err != nil {
			r.log.Warnf("enforce concurrent limit failed (non-fatal): %v", err)
		}
	}

	if accessToken = r.createAccessJwtToken(user, meta.ClientID); accessToken == "" {
		err = errors.New("create access token failed")
		return
	}

	if err = r.storeToken(ctx, user.GetId(), accessToken, usertoken.TokenTypeAccess, &meta.ClientID, r.accessTokenExpires, meta); err != nil {
		return
	}

	return
}

// getMaxConcurrent 从 ent User 实体读取最大并发会话数。
// max_concurrent 已入 proto DTO（B7），此处仍直查 ent：令牌签发属安全控制点，
// 以签发时刻的库内权威值为准，不依赖调用方传入 DTO 的新鲜度。
// 查询失败时返回 0（不做并发限制），避免登录主流程被次要功能阻断。
func (r *UserTokenCacheRepo) getMaxConcurrent(ctx context.Context, userID uint32) int {
	if userID == 0 {
		return 0
	}
	u, err := r.data.db.Client().User.Get(ctx, userID)
	if err != nil {
		r.log.Warnf("query user max_concurrent failed (non-fatal): %v", err)
		return 0
	}
	return u.MaxConcurrent
}

// GenerateRefreshToken 创建刷新令牌
func (r *UserTokenCacheRepo) GenerateRefreshToken(ctx context.Context, user *userV1.User, meta TokenMeta) (refreshToken string, err error) {
	if refreshToken = r.createRefreshToken(); refreshToken == "" {
		err = errors.New("create refresh token failed")
		return
	}

	if err = r.storeToken(ctx, user.GetId(), refreshToken, usertoken.TokenTypeRefresh, nil, r.refreshTokenExpires, meta); err != nil {
		return
	}

	return
}

// RemoveToken 撤销用户所有令牌（登出 / 强制下线）。
// 软删除：置 revoked=true，使 auth 中间件的 isExistAccessToken 校验失败。
func (r *UserTokenCacheRepo) RemoveToken(ctx context.Context, userId uint32) error {
	_, err := r.data.db.Client().UserToken.Update().
		Where(usertoken.UserID(userId)).
		SetRevoked(true).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("revoke user tokens failed: [%v]", err)
		return err
	}
	r.purgeExistsCacheForUser(userId)
	return nil
}

// RemoveAccessToken 撤销指定访问令牌（登出当前会话）。
func (r *UserTokenCacheRepo) RemoveAccessToken(ctx context.Context, userId uint32, accessToken string) error {
	return r.revokeToken(ctx, userId, accessToken, usertoken.TokenTypeAccess)
}

// RemoveRefreshToken 撤销指定刷新令牌
func (r *UserTokenCacheRepo) RemoveRefreshToken(ctx context.Context, userId uint32, refreshToken string) error {
	return r.revokeToken(ctx, userId, refreshToken, usertoken.TokenTypeRefresh)
}

// GenerateMFAToken 生成 MFA 二步登录临时令牌（UUID，5 分钟过期）。
// 登录第一步密码校验通过但用户启用了 MFA 时签发，前端据此调用 VerifyMFA 完成第二步。
// 此令牌不授予 API 访问权限（auth 中间件仅校验 token_type=ACCESS）。
func (r *UserTokenCacheRepo) GenerateMFAToken(ctx context.Context, userId uint32, meta TokenMeta) (string, error) {
	token := uuid.NewString()
	const mfaTokenExpiry = 5 * time.Minute
	if err := r.storeToken(ctx, userId, token, usertoken.TokenTypeMfa, &meta.ClientID, mfaTokenExpiry, meta); err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeMFAToken 校验并消费 MFA 临时令牌（校验成功后立即撤销，一次性使用）。
// 返回令牌关联的用户 ID；令牌不存在、已撤销或已过期时返回 0。
func (r *UserTokenCacheRepo) ConsumeMFAToken(ctx context.Context, mfaToken string) (uint32, error) {
	if mfaToken == "" {
		return 0, nil
	}
	entity, err := r.data.db.Client().UserToken.Query().
		Where(
			usertoken.Token(mfaToken),
			usertoken.TokenTypeEQ(usertoken.TokenTypeMfa),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		return 0, err
	}
	if entity.Revoked {
		return 0, nil
	}
	if entity.ExpiresAt != nil && time.Now().After(*entity.ExpiresAt) {
		return 0, nil
	}
	// 撤销（一次性）
	if err := r.data.db.Client().UserToken.UpdateOneID(entity.ID).
		SetRevoked(true).
		SetUpdateTime(time.Now()).
		Exec(ctx); err != nil {
		r.log.Warnf("revoke mfa token failed (non-fatal): %v", err)
	}
	if entity.UserID == nil {
		return 0, nil
	}
	return *entity.UserID, nil
}

// RevokeAllUserAccessTokens 撤销用户所有 access token（强制下线）。
func (r *UserTokenCacheRepo) RevokeAllUserAccessTokens(ctx context.Context, userId uint32) error {
	_, err := r.data.db.Client().UserToken.Update().
		Where(
			usertoken.UserID(userId),
			usertoken.TokenTypeEQ(usertoken.TokenTypeAccess),
		).
		SetRevoked(true).
		SetUpdateTime(time.Now()).
		Save(ctx)
	return err
}

// IsExistAccessToken 访问令牌是否有效（存在、未过期、未撤销）。
// auth 中间件通过 WithIsExistAccessTokenFunc 注入此方法，实现 Token 黑名单。
func (r *UserTokenCacheRepo) IsExistAccessToken(ctx context.Context, userId uint32, accessToken string) bool {
	if accessToken == "" {
		return false
	}
	return r.exists(ctx, userId, accessToken, usertoken.TokenTypeAccess)
}

// IsExistRefreshToken 刷新令牌是否存在
func (r *UserTokenCacheRepo) IsExistRefreshToken(ctx context.Context, userId uint32, refreshToken string) bool {
	return r.exists(ctx, userId, refreshToken, usertoken.TokenTypeRefresh)
}

// GetUserIDByRefreshToken 按刷新令牌反查用户 ID（存在、未撤销、未过期）；无效返回 (0, false)。
// 用于免 JWT 的刷新链路：access token 过期后请求上下文无用户身份，需由 refresh_token 解析。
func (r *UserTokenCacheRepo) GetUserIDByRefreshToken(ctx context.Context, refreshToken string) (uint32, bool) {
	if refreshToken == "" {
		return 0, false
	}
	now := time.Now()
	entity, err := r.data.db.Client().UserToken.Query().
		Where(
			usertoken.Token(refreshToken),
			usertoken.TokenTypeEQ(usertoken.TokenTypeRefresh),
			usertoken.RevokedEQ(false),
			usertoken.Or(
				usertoken.ExpiresAtIsNil(),
				usertoken.ExpiresAtGT(now),
			),
		).
		Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			r.log.Errorf("query refresh token failed: [%v]", err)
		}
		return 0, false
	}
	if entity.UserID == nil {
		return 0, false
	}
	return *entity.UserID, true
}

// OnlineUserInfo 在线用户信息。
// 类型本体已上移到 biz 层（端口出参），此处保留别名兼容。
type OnlineUserInfo = biz.OnlineUserInfo

// ListOnlineUsers 查询在线用户（access token 未撤销且未过期）。
// 按自增 ID 倒序：最新签发的会话排前（create_time 存在历史 NULL 数据，不能作为排序键）。
func (r *UserTokenCacheRepo) ListOnlineUsers(ctx context.Context) ([]OnlineUserInfo, error) {
	now := time.Now()
	entities, err := r.data.db.Client().UserToken.Query().
		Where(
			usertoken.TokenTypeEQ(usertoken.TokenTypeAccess),
			usertoken.RevokedEQ(false),
			usertoken.Or(
				usertoken.ExpiresAtIsNil(),
				usertoken.ExpiresAtGT(now),
			),
		).
		Order(ent.Desc(usertoken.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]OnlineUserInfo, 0, len(entities))
	for _, e := range entities {
		info := OnlineUserInfo{
			Token: e.Token,
		}
		if e.UserID != nil {
			info.UserID = *e.UserID
		}
		if e.CreateTime != nil {
			info.LoginTime = *e.CreateTime
		}
		if e.ClientID != nil {
			info.ClientID = *e.ClientID
		}
		if e.LoginIP != nil {
			info.LoginIP = *e.LoginIP
		}
		if e.LoginLocation != nil {
			info.LoginLocation = *e.LoginLocation
		}
		if e.UserAgent != nil {
			info.UserAgent = *e.UserAgent
		}
		if e.ExpiresAt != nil {
			info.ExpiresAt = *e.ExpiresAt
		}
		result = append(result, info)
	}
	return result, nil
}

// DeleteExpiredTokens 物理删除 expires_at 早于 before 的令牌记录，返回删除条数。
// 无 expires_at（永不过期）的记录不删；未过期的记录不删。
func (r *UserTokenCacheRepo) DeleteExpiredTokens(ctx context.Context, before time.Time) (int64, error) {
	deleted, err := r.data.db.Client().UserToken.Delete().
		Where(
			usertoken.ExpiresAtNotNil(),
			usertoken.ExpiresAtLT(before),
		).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete expired tokens failed: [%v]", err)
		return 0, err
	}
	return int64(deleted), nil
}

// storeToken 落库一条令牌记录。同一 (user_id, token, token_type) 已存在则忽略（幂等）。
func (r *UserTokenCacheRepo) storeToken(ctx context.Context, userId uint32, token string, tokenType usertoken.TokenType, clientId *string, expires time.Duration, meta TokenMeta) error {
	var expiresAt *time.Time
	if expires > 0 {
		t := time.Now().Add(expires)
		expiresAt = &t
	}

	builder := r.data.db.Client().UserToken.Create().
		SetUserID(userId).
		SetToken(token).
		SetTokenType(tokenType).
		SetRevoked(false).
		SetNillableClientID(clientId).
		SetNillableExpiresAt(expiresAt).
		SetNillableLoginIP(&meta.LoginIP).
		SetNillableLoginLocation(&meta.LoginLocation).
		SetNillableUserAgent(&meta.UserAgent).
		// mixin.Time 的 create_time 无 Default(time.Now)，必须显式写入，否则登录时间恒为 NULL
		SetCreateTime(time.Now())

	_, err := builder.Save(ctx)
	if err != nil {
		// 幂等：同一 (user_id, token, token_type) 已存在则忽略。
		// 双层识别：ent.IsConstraintError 覆盖英文 locale（ent 包装为 *ConstraintError），
		// isPgUniqueViolation 覆盖中文 locale（ent 字符串匹配失效时基于 SQLSTATE 23505 识别）。
		if ent.IsConstraintError(err) || isPgUniqueViolation(err) {
			return nil
		}
		r.log.Errorf("store %s token failed: [%v]", tokenType, err)
		return err
	}
	return nil
}

func (r *UserTokenCacheRepo) revokeToken(ctx context.Context, userId uint32, token string, tokenType usertoken.TokenType) error {
	_, err := r.data.db.Client().UserToken.Update().
		Where(
			usertoken.UserID(userId),
			usertoken.Token(token),
			usertoken.TokenTypeEQ(tokenType),
		).
		SetRevoked(true).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err == nil {
		r.purgeExistsCacheForUser(userId)
	}
	return err
}

func (r *UserTokenCacheRepo) exists(ctx context.Context, userId uint32, token string, tokenType usertoken.TokenType) bool {
	key := existsCacheKey(userId, token, tokenType)
	if valid, hit := r.getExistsCache(key); hit {
		return valid
	}
	now := time.Now()
	count, err := r.data.db.Client().UserToken.Query().
		Where(
			usertoken.UserID(userId),
			usertoken.Token(token),
			usertoken.TokenTypeEQ(tokenType),
			usertoken.RevokedEQ(false),
			usertoken.Or(
				usertoken.ExpiresAtIsNil(),
				usertoken.ExpiresAtGT(now),
			),
		).
		Count(ctx)
	if err != nil {
		// 查询失败不写缓存：瞬时故障不应被固化为"无效"结论
		r.log.Errorf("query %s token exist failed: [%v]", tokenType, err)
		return false
	}
	valid := count > 0
	r.putExistsCache(key, valid)
	return valid
}

// enforceConcurrentLimit 会话并发限制：超出 maxConcurrent 时撤销最早的 access token。
func (r *UserTokenCacheRepo) enforceConcurrentLimit(ctx context.Context, userId uint32, maxConcurrent int) error {
	now := time.Now()
	// 查询当前有效的 access token 数量
	activeTokens, err := r.data.db.Client().UserToken.Query().
		Where(
			usertoken.UserID(userId),
			usertoken.TokenTypeEQ(usertoken.TokenTypeAccess),
			usertoken.RevokedEQ(false),
			usertoken.Or(
				usertoken.ExpiresAtIsNil(),
				usertoken.ExpiresAtGT(now),
			),
		).
		// 按自增 ID 排序（= 插入顺序）：mixin.Time 的 create_time 无默认值恒为 NULL，
		// 按 create_time 排序在饱和时撤销目标不定，会把刚签发的 token 随机杀掉（401 风暴）。
		Order(ent.Asc(usertoken.FieldID)).
		All(ctx)
	if err != nil {
		return err
	}

	// 需要淘汰的数量 = 当前数量 - (maxConcurrent - 1)，为新 token 预留一个位置
	excess := len(activeTokens) - (maxConcurrent - 1)
	if excess <= 0 {
		return nil
	}

	// 撤销最早的 excess 个 token
	revokedAny := false
	for i := 0; i < excess && i < len(activeTokens); i++ {
		if err := r.data.db.Client().UserToken.UpdateOneID(activeTokens[i].ID).
			SetRevoked(true).
			SetUpdateTime(now).
			Exec(ctx); err != nil {
			r.log.Warnf("revoke old token %d failed (non-fatal): %v", activeTokens[i].ID, err)
		} else {
			revokedAny = true
		}
	}
	if revokedAny {
		r.purgeExistsCacheForUser(userId)
	}
	return nil
}

// createAccessJwtToken 生成JWT访问令牌
func (r *UserTokenCacheRepo) createAccessJwtToken(user *userV1.User, clientId string) string {
	authClaims := jwt.NewUserTokenAuthClaims(user, clientId)

	signedToken, err := r.authenticator.CreateIdentity(*authClaims)
	if err != nil {
		r.log.Error("create access token failed: [%v]", err)
	}

	return signedToken
}

// createRefreshToken 生成刷新令牌
func (r *UserTokenCacheRepo) createRefreshToken() string {
	strUUID := uuid.New()
	return strUUID.String()
}
