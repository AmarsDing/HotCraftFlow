package data

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	conf "framework/app/admin/internal/conf"
)

// RedisClient 包装 redis.Client，用于排班生成分布式锁、工单编号原子计数、知识库 use_count 异步累加与统计缓存。
// 配置来自公共 data.yaml（data.redis），未配置时各能力按 nil 安全降级（锁退化为无锁、计数退化为数据库兜底）。
type RedisClient struct {
	client *redis.Client
	log    *log.Helper
}

func NewRedisClient(cfg *conf.Bootstrap, logger log.Logger) (*RedisClient, func(), error) {
	l := log.NewHelper(log.With(logger, "module", "data/admin-redis"))

	redisCfg := cfg.Data.GetRedis()
	if redisCfg == nil || redisCfg.GetAddr() == "" {
		l.Warn("redis not configured, schedule lock / ticket seq / kb counter will degrade")
		return &RedisClient{}, func() {}, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     redisCfg.GetAddr(),
		Password: redisCfg.GetPassword(),
		DB:       int(redisCfg.GetDb()),
	})

	cleanup := func() {
		if err := client.Close(); err != nil {
			l.Error(err)
		}
	}

	return &RedisClient{client: client, log: l}, cleanup, nil
}

func (r *RedisClient) Client() *redis.Client {
	return r.client
}

// IsConfigured 返回 Redis 是否已配置（client 非 nil）。
func (r *RedisClient) IsConfigured() bool {
	return r != nil && r.client != nil
}

func (r *RedisClient) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

// Ping 检查 Redis 连通性，供 /ready 就绪探针使用。
func (r *RedisClient) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("redis not configured")
	}
	return r.client.Ping(ctx).Err()
}

// TryLock 尝试获取分布式锁（SET NX EX，值为随机 token），返回 token 与是否获取成功。
// 未配置 Redis 时退化为直接放行（单实例部署下可接受，日志已在构造时告警），此时 token 为空串。
func (r *RedisClient) TryLock(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if r == nil || r.client == nil {
		return "", true, nil
	}
	token := uuid.NewString()
	ok, err := r.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return "", ok, err
	}
	return token, true, nil
}

// Unlock 释放分布式锁：Lua 校验 token 一致才删除，防止误释放他人（锁过期后被重建）的锁。
func (r *RedisClient) Unlock(ctx context.Context, key, token string) {
	if r == nil || r.client == nil || token == "" {
		return
	}
	const script = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
	if err := r.client.Eval(ctx, script, []string{key}, token).Err(); err != nil {
		r.log.Warnf("release lock %s failed: %s", key, err.Error())
	}
}

// HGetDel 原子读取哈希字段值并删除整个 key（Lua 保证读删原子，防止读后删前的新增量丢失）。
// key 或字段不存在返回 (0, nil)。
func (r *RedisClient) HGetDel(ctx context.Context, key, field string) (int64, error) {
	if r == nil || r.client == nil {
		return 0, errors.New("redis not configured")
	}
	const script = `
local v = redis.call("HGET", KEYS[1], ARGV[1])
if v then
  redis.call("DEL", KEYS[1])
end
return v`
	res, err := r.client.Eval(ctx, script, []string{key}, field).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	switch v := res.(type) {
	case int64:
		return v, nil
	case string:
		n, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return 0, perr
		}
		return n, nil
	default:
		return 0, nil
	}
}

// IncrWithExpire 原子自增并在首次创建 key 时设置过期时间（用于工单编号日计数器）。
// 未配置 Redis 时返回 0 与错误，调用方需走数据库兜底。
func (r *RedisClient) IncrWithExpire(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if r == nil || r.client == nil {
		return 0, errors.New("redis not configured")
	}
	// Lua 保证 incr + expire 原子：仅当计数为 1（新 key）时设置 TTL
	const script = `
local v = redis.call("INCR", KEYS[1])
if v == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return v`
	res, err := r.client.Eval(ctx, script, []string{key}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, err
	}
	return res, nil
}

// HIncrBy 哈希字段原子自增（用于知识库 use_count 异步累加）。
func (r *RedisClient) HIncrBy(ctx context.Context, key, field string, delta int64) error {
	if r == nil || r.client == nil {
		return errors.New("redis not configured")
	}
	return r.client.HIncrBy(ctx, key, field, delta).Err()
}

// ScanKeys 按模式扫描 key（SCAN 游标遍历），最多返回 limit 个，防止长时间占用连接。
func (r *RedisClient) ScanKeys(ctx context.Context, pattern string, limit int) ([]string, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("redis not configured")
	}
	if limit <= 0 {
		limit = 1000
	}
	keys := make([]string, 0, limit)
	var cursor uint64
	for {
		batch, next, err := r.client.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return keys, err
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 || len(keys) >= limit {
			break
		}
	}
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return keys, nil
}

// HGetInt64 读取哈希字段整数值，key 或字段不存在返回 0。
func (r *RedisClient) HGetInt64(ctx context.Context, key, field string) (int64, error) {
	if r == nil || r.client == nil {
		return 0, errors.New("redis not configured")
	}
	v, err := r.client.HGet(ctx, key, field).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return v, err
}

// Del 删除 key。
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	if r == nil || r.client == nil {
		return errors.New("redis not configured")
	}
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

// GetString 读取字符串值，key 不存在返回 ("", nil)。
func (r *RedisClient) GetString(ctx context.Context, key string) (string, error) {
	if r == nil || r.client == nil {
		return "", errors.New("redis not configured")
	}
	v, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

// SetStringEX 写入字符串值并设置过期时间（统计缓存）。
func (r *RedisClient) SetStringEX(ctx context.Context, key, value string, ttl time.Duration) error {
	if r == nil || r.client == nil {
		return errors.New("redis not configured")
	}
	return r.client.Set(ctx, key, value, ttl).Err()
}
