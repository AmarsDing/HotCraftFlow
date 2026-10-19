package biz

import (
	"time"

	"framework/app/admin/internal/conf"
)

// =============================================================================
// ① IC 运行时配置（conf.Ic 的蒸馏视图；biz 不裸读 proto，默认值集中在此兜底）
// ============================================================================

// IcConfig IC 子系统运行参数。ResolveIcConfig 负责零值补默认（与 ic.yaml 注释同源）。
type IcConfig struct {
	Enabled             bool
	Concurrency         int32
	WorkabilityMinScore float64 // S-26：MME 准入阈值，IC 配置 MME 只读
	RawQueueCapacity    int64

	// 去重/理解层
	SimhashThreshold      int32   // B-9：汉明 ≤3
	EmbeddingThreshold    float64 // 默认 0.86
	EmbeddingWindowHours  int32   // 默认 24（质心比对 72h 由簇逻辑另取）
	EmbeddingDailyCap     int32   // B-7 硬熔断
	EmbeddingStoreEnabled bool    // store != none
	// DedupMergeWindow L1 URL 归一 + L2 SimHash 的回望合并窗口。
	// 默认 72h = 48h 闲时闭簇（S-2，闭簇不再吸新成员）+ 一个采集周期余量（03 §4.4）。
	DedupMergeWindow time.Duration
	// IDFPath 周任务产出路径（非配置项，conf proto 未开放——周任务与该默认值同源，
	// 倒挂契约风险低于增面：见 B-9 「idf 表 data/ic/idf.json 周任务产出」）。
	IDFPath string

	// 证据
	EvidenceStorageDir  string
	EvidenceRawMaxBytes int64
	EvidenceRetainDays  int32

	// 限流/熔断
	RateLimitDefaultPerMin int32
	RateLimitBackoffMax    time.Duration
	BreakerFailThreshold   int32
	BreakerCooldown        time.Duration

	// 合规外部依赖
	RobotsUserAgent       string
	RobotsTimeout         time.Duration
	RobotsTTL             time.Duration
	ShortlinkMaxRedirects int32

	// 保留期
	RetentionIntelDays        int32
	RetentionClusterIdleHours int32 // S-2：48h 无更新闭簇
	RetentionSweepCron        string

	// LLM（ADR-IC-07：默认关闭，开启失败回退规则）
	LLMEnabled        bool
	LLMFallbackToRule bool
}

// NewIcConfig wire provider（conf.Bootstrap → IcConfig）。
func NewIcConfig(bc *conf.Bootstrap) *IcConfig { return ResolveIcConfig(bc) }

// ResolveIcConfig 解析 conf.Ic，零值补默认（默认值即 ic.yaml 文案基线）。
func ResolveIcConfig(bc *conf.Bootstrap) *IcConfig {
	cfg := &IcConfig{
		Enabled:             true,
		Concurrency:         8,
		WorkabilityMinScore: 0.5,
		RawQueueCapacity:    5000,

		SimhashThreshold:     3,
		EmbeddingThreshold:   0.86,
		EmbeddingWindowHours: 24,
		EmbeddingDailyCap:    5000,
		DedupMergeWindow:     72 * time.Hour,

		EvidenceStorageDir:  "./storage/ic/evidence",
		EvidenceRawMaxBytes: 4 << 20,
		EvidenceRetainDays:  90,

		RateLimitDefaultPerMin: 20,
		RateLimitBackoffMax:    15 * time.Minute,
		BreakerFailThreshold:   5,
		BreakerCooldown:        30 * time.Minute,

		RobotsUserAgent: "FrameWorkBot",
		RobotsTimeout:   5 * time.Second,
		RobotsTTL:       24 * time.Hour,

		ShortlinkMaxRedirects: 5,

		RetentionIntelDays:        365,
		RetentionClusterIdleHours: 48,
		RetentionSweepCron:        "0 3 * * *",

		IDFPath:           "./data/ic/idf.json",
		LLMFallbackToRule: true,
	}
	if bc == nil {
		return cfg
	}
	ic := bc.GetIc()
	if ic == nil {
		return cfg
	}

	// Enabled 不做零值覆写：proto3 下省略 enabled 字段时 GetEnabled() 返回 false，
	// 若无条件赋值，配置段写了 ic 但漏写 enabled=true 就会把默认开启的整个 IC
	// 子系统静默关闭（评审坐实 major）。显式关站须写 enabled:false——但 proto3
	// 无法区分「省略」与「显式 false」，故约定：ic 段存在即视为启用，关站走
	// 删除整段或调度层开关，不由此字段承载。
	_ = ic.GetEnabled()
	if v := ic.GetConcurrency(); v > 0 {
		cfg.Concurrency = v
	}
	if v := ic.GetWorkabilityMinScore(); v > 0 {
		cfg.WorkabilityMinScore = v
	}
	if v := ic.GetRawQueueCapacity(); v > 0 {
		cfg.RawQueueCapacity = v
	}

	if d := ic.GetDedup(); d != nil {
		if v := d.GetSimhashThreshold(); v > 0 {
			cfg.SimhashThreshold = v
		}
		if v := d.GetEmbeddingThreshold(); v > 0 {
			cfg.EmbeddingThreshold = v
		}
		if v := d.GetEmbeddingWindowHours(); v > 0 {
			cfg.EmbeddingWindowHours = v
		}
		if v := d.GetMergeWindowHours(); v > 0 {
			cfg.DedupMergeWindow = time.Duration(v) * time.Hour
		}
		if e := d.GetEmbedding(); e != nil {
			cfg.EmbeddingStoreEnabled = e.GetStore() != "" && e.GetStore() != "none"
			if v := e.GetDailyCap(); v > 0 {
				cfg.EmbeddingDailyCap = v
			}
		}
	}
	if ev := ic.GetEvidence(); ev != nil {
		if v := ev.GetStorageDir(); v != "" {
			cfg.EvidenceStorageDir = v
		}
		if v := ev.GetRawMaxBytes(); v > 0 {
			cfg.EvidenceRawMaxBytes = v
		}
		if v := ev.GetRetainDays(); v > 0 {
			cfg.EvidenceRetainDays = v
		}
	}
	if rl := ic.GetRateLimit(); rl != nil {
		if v := rl.GetDefaultPerMin(); v > 0 {
			cfg.RateLimitDefaultPerMin = v
		}
		if v := rl.GetBackoffMax(); v != nil {
			cfg.RateLimitBackoffMax = v.AsDuration()
		}
	}
	if br := ic.GetBreaker(); br != nil {
		if v := br.GetFailThreshold(); v > 0 {
			cfg.BreakerFailThreshold = v
		}
		if v := br.GetCooldown(); v != nil {
			cfg.BreakerCooldown = v.AsDuration()
		}
	}
	if rb := ic.GetRobots(); rb != nil {
		if v := rb.GetUserAgent(); v != "" {
			cfg.RobotsUserAgent = v
		}
		if v := rb.GetTimeout(); v != nil {
			cfg.RobotsTimeout = v.AsDuration()
		}
		if v := rb.GetCacheTtl(); v != nil {
			cfg.RobotsTTL = v.AsDuration()
		}
	}
	if sl := ic.GetShortlink(); sl != nil {
		if v := sl.GetMaxRedirects(); v > 0 {
			cfg.ShortlinkMaxRedirects = v
		}
	}
	if rt := ic.GetRetention(); rt != nil {
		if v := rt.GetIntelDays(); v > 0 {
			cfg.RetentionIntelDays = v
		}
		if v := rt.GetClusterIdleCloseHours(); v > 0 {
			cfg.RetentionClusterIdleHours = v
		}
		if v := rt.GetSweepCron(); v != "" {
			cfg.RetentionSweepCron = v
		}
	}
	if ll := ic.GetLlm(); ll != nil {
		cfg.LLMEnabled = ll.GetEnabled()
		cfg.LLMFallbackToRule = ll.GetFallbackToRule()
	}
	return cfg
}
