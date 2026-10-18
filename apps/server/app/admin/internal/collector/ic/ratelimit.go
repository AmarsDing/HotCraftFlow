package iccollector

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data"
)

// =============================================================================
// 令牌桶限流（S-10；评审 gap#18：多实例语义必须 Redis 原子，单机 map 直接否）
// Redis 未配置 → 降级放行 + Warn（本地开发态；生产部署 Redis 为硬依赖，启动体检列出）。
// =============================================================================

// tokenBucketLua 原子取牌（容量 = rate，注水 = rate/60 每秒；KEYS[1] 桶键，
// ARGV: rate_per_min, now_ms。返回 1=取到, 0=不足）。
var tokenBucketLua = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local capacity = rate
local refill_per_ms = rate / 60000.0

local bucket = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(bucket[1])
local ts = tonumber(bucket[2])
if tokens == nil then
  tokens = capacity
  ts = now
end

local elapsed = math.max(0, now - ts)
tokens = math.min(capacity, tokens + elapsed * refill_per_ms)

if tokens < 1 then
  redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
  redis.call('PEXPIRE', key, 120000)
  return 0
end
tokens = tokens - 1
redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', key, 120000)
return 1
`)

// RedisRateLimiter biz.RateLimiterPort 实现。
type RedisRateLimiter struct {
	redis *data.RedisClient
	log   *log.Helper
}

func NewRedisRateLimiter(rc *data.RedisClient, logger log.Logger) *RedisRateLimiter {
	return &RedisRateLimiter{redis: rc, log: log.NewHelper(log.With(logger, "module", "ic/ratelimit"))}
}

// Allow key 粒度取一枚令牌。
func (l *RedisRateLimiter) Allow(ctx context.Context, key string, perMin int32) (bool, error) {
	if !l.redis.IsConfigured() {
		l.log.Warnf("redis not configured, rate limit degraded to allow-all (key=%s)", key)
		return true, nil
	}
	if perMin <= 0 {
		perMin = 20
	}
	res, err := tokenBucketLua.Run(ctx, l.redis.Client(),
		[]string{"ic:rl:" + key}, perMin, time.Now().UnixMilli()).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

var _ biz.RateLimiterPort = (*RedisRateLimiter)(nil)
