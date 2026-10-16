package biz

import (
	"context"
	"time"
)

// RedisStore Redis 能力端口（biz 层禁止 import data，由 data.RedisClient 实现，wire.Bind 绑定）。
// 覆盖：排班生成分布式锁、工单/变更/事件编号日计数器、知识库 use_count 异步累加与定期落库、统计缓存。
// 未配置 Redis 时实现方按方法语义降级（锁放行、计数返回错误由调用方走数据库兜底）。
type RedisStore interface {
	// IsConfigured 返回 Redis 是否已配置。
	IsConfigured() bool

	// TryLock 尝试获取分布式锁（SET NX EX，值为随机 token），返回 token 与是否获取成功。
	// 未配置时退化为放行且 token 为空串。
	TryLock(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	// Unlock 释放分布式锁：校验 token 一致才删除，防止误释放他人的锁。
	Unlock(ctx context.Context, key, token string)

	// IncrWithExpire 原子自增并在新 key 时设置过期时间（编号日计数器）。未配置时返回错误。
	IncrWithExpire(ctx context.Context, key string, ttl time.Duration) (int64, error)

	// HIncrBy 哈希字段原子自增（知识库 use_count 异步累加）。未配置时返回错误。
	HIncrBy(ctx context.Context, key, field string, delta int64) error
	// ScanKeys 按模式扫描 key（如 kb:use_count:*），最多返回 limit 个。
	ScanKeys(ctx context.Context, pattern string, limit int) ([]string, error)
	// HGetInt64 读取哈希字段整数值，key 或字段不存在返回 0。
	HGetInt64(ctx context.Context, key, field string) (int64, error)
	// HGetDel 原子读取哈希字段值并删除整个 key（防读后删新增量丢失）。key 或字段不存在返回 (0, nil)。
	HGetDel(ctx context.Context, key, field string) (int64, error)
	// Del 删除 key。
	Del(ctx context.Context, keys ...string) error

	// GetString 读取字符串值，key 不存在返回 ("", nil)。
	GetString(ctx context.Context, key string) (string, error)
	// SetStringEX 写入字符串值并设置过期时间（统计缓存）。
	SetStringEX(ctx context.Context, key, value string, ttl time.Duration) error
}
