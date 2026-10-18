package pubdata

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data"
)

// publishLockKeyPrefix Redis 锁键前缀（NFR-MAP-05：map:publish:{unitID}）。
const publishLockKeyPrefix = "map:publish:"

// RedisPublishLocker bizpub.PublishLocker 的 Redis SETNX 实现。
// 未配置 Redis 时 TryLock 直接放行（data.RedisClient 既定降级语义）：
// 幂等仍由 receipts 唯一索引兜底，构造时 Warn 提示。
type RedisPublishLocker struct {
	redis *data.RedisClient
	log   *log.Helper

	// tokens 进程内持有 unit→token 映射，供 Unlock 校验 ownership（RedisClient.Unlock Lua 比对）。
	tokens sync.Map
}

// NewRedisPublishLocker 构造。
func NewRedisPublishLocker(rc *data.RedisClient, logger log.Logger) *RedisPublishLocker {
	l := log.NewHelper(log.With(logger, "module", "data/map-publish-locker"))
	if rc == nil || !rc.IsConfigured() {
		l.Warn("[map-locker] Redis 未配置，发布幂等锁降级为放行（DB 唯一索引仍兜底）")
	}
	return &RedisPublishLocker{redis: rc, log: l}
}

// Lock 获取 unit 的发布锁，ttl 内防重；已占用返回 (false, nil)。
func (l *RedisPublishLocker) Lock(ctx context.Context, unitID string, ttl time.Duration) (bool, error) {
	if l.redis == nil || !l.redis.IsConfigured() {
		return true, nil // 降级放行（构造时已 Warn；DB 唯一索引兜底）
	}
	token, ok, err := l.redis.TryLock(ctx, publishLockKeyPrefix+unitID, ttl)
	if err != nil {
		return false, err
	}
	if ok {
		l.tokens.Store(unitID, token)
	}
	return ok, nil
}

// Unlock 释放锁（尽力而为；token 校验防误释放他人锁）。
func (l *RedisPublishLocker) Unlock(ctx context.Context, unitID string) error {
	if l.redis == nil || !l.redis.IsConfigured() {
		return nil
	}
	tok, _ := l.tokens.LoadAndDelete(unitID)
	token, _ := tok.(string)
	l.redis.Unlock(ctx, publishLockKeyPrefix+unitID, token)
	return nil
}

// 编译期断言：实现与端口一致。
var (
	_ bizpub.CredentialVault = (*MapCredentialVault)(nil)
	_ bizpub.PublishLocker   = (*RedisPublishLocker)(nil)
)
