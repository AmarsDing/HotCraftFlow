package publisher

import "time"

// AccountHealth 账号健康状态。
type AccountHealth string

const (
	HealthOK                AccountHealth = "ok"
	HealthDegraded          AccountHealth = "degraded"
	HealthPaused            AccountHealth = "paused"
	HealthCredentialInvalid AccountHealth = "credential_invalid" // 凭据失效，不清凭据便于排查
)

// BreakerConfig 熔断配置（详细设计 §6）。
type BreakerConfig struct {
	AccountFailThreshold      int           // 账号级连续失败阈值，默认 3
	DegradedThreshold         int           // 降级阈值，默认 2
	PauseDuration             time.Duration // 暂停时长，默认 30min
	PlatformFailRateThreshold float64       // 平台级 24h 失败率阈值，默认 0.5
}

// DefaultBreakerConfig 返回默认熔断配置。
func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		AccountFailThreshold:      3,
		DegradedThreshold:         2,
		PauseDuration:             30 * time.Minute,
		PlatformFailRateThreshold: 0.5,
	}
}

// BreakerDecision 熔断判定结果。
type BreakerDecision struct {
	Health       AccountHealth
	PausedUntil  time.Time
	PauseAccount bool // 是否需暂停该账号
	PausePlatform bool // 是否需暂停整个平台（需人工介入）
	Alert        string
}

// EvaluateAccountFailure 评估一次账号失败后的熔断动作（详细设计 §6）。
//
// ⚠️ 并发原子性（评审 MAP 严重项）：consecutiveFailures 的读-改-写必须由调用方
// 用 Redis Lua 或 DB 行锁保证原子，本函数只做纯判定。
func EvaluateAccountFailure(consecutiveFailures int, platformFailRate24h float64, cfg BreakerConfig, now time.Time) BreakerDecision {
	d := BreakerDecision{Health: HealthOK}
	switch {
	case consecutiveFailures >= cfg.AccountFailThreshold:
		d.Health = HealthPaused
		d.PausedUntil = now.Add(cfg.PauseDuration)
		d.PauseAccount = true
		d.Alert = "account_paused"
	case consecutiveFailures >= cfg.DegradedThreshold:
		d.Health = HealthDegraded
	}
	// 平台级检查：24h 失败率超阈值 → 暂停该平台全部账号（需人工介入）。
	if platformFailRate24h > cfg.PlatformFailRateThreshold {
		d.PausePlatform = true
		d.Alert = "platform_paused"
	}
	return d
}

// OnCredentialInvalid 鉴权失败的健康结论：credential_invalid，不清凭据（便于排查）。
func OnCredentialInvalid() BreakerDecision {
	return BreakerDecision{Health: HealthCredentialInvalid}
}

// ---------- 速率治理（详细设计 §7.2） ----------

// RateConfig 速率治理配置。
type RateConfig struct {
	AccountMinInterval time.Duration // 账号级两次发布最小间隔，默认 5min
	PlatformPerHour    int           // 单平台每小时上限，默认 20
	GlobalPer24h       int           // 全局 24h 发布总量，默认 100（超限硬阻断）
	JitterMinutes      int           // 拟人化随机偏移 ±N 分钟，默认 3
}

// DefaultRateConfig 返回默认速率配置。
func DefaultRateConfig() RateConfig {
	return RateConfig{
		AccountMinInterval: 5 * time.Minute,
		PlatformPerHour:    20,
		GlobalPer24h:       100,
		JitterMinutes:      3,
	}
}

// CanPublishNow 速率治理判定（纯函数；计数由调用方用 Redis 原子维护）。
func CanPublishNow(secondsSinceLastAccountPublish int64, platformCountThisHour, globalCount24h int, cfg RateConfig) (bool, string) {
	if int64(cfg.AccountMinInterval.Seconds()) > secondsSinceLastAccountPublish {
		return false, "account_min_interval"
	}
	if platformCountThisHour >= cfg.PlatformPerHour {
		return false, "platform_hourly_cap"
	}
	if globalCount24h >= cfg.GlobalPer24h {
		return false, "global_daily_cap"
	}
	return true, ""
}
