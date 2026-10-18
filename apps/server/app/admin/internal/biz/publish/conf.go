package publish

import (
	"errors"
	"fmt"
	"time"
)

// Conf MAP 子系统配置（对应 app/admin/config/map.yaml 的 `map:` 段，详细设计 §8）。
//
// 不设为 proto（避免 conf.pb.go 再生成重链），由 main 用 kratos config
// `c.Value("map").Scan(&mc)` 扫描该结构（json tag 须与 yaml 键名一致）。
//
// 🔴 安全默认值铁律：scan 缺省/Normalize 时一律回落到「更严」而不是「更松」
// （默认私密、禁自批、禁自动化、日志脱敏全部默认开且 Validate 强制）。
type Conf struct {
	Enabled    bool           `json:"enabled"`
	Security   SecurityConf   `json:"security"`
	Quota      QuotaConf      `json:"quota"`
	Breaker    BreakerConf    `json:"breaker"`
	Retry      RetryConf      `json:"retry"`
	Schedule   ScheduleConf   `json:"schedule"`
	Feedback   FeedbackConf   `json:"feedback"`
	Credential CredentialConf `json:"credential"`
}

// SecurityConf 安全红线（详细设计 §8 security 段）。
type SecurityConf struct {
	// DefaultVisibility 默认可见性。🔴 恒为 "private"，改为任何其他值 Validate 拒绝启动。
	DefaultVisibility string `json:"default_visibility"`
	// AllowPublic 是否允许升公（仍须 override_reason + 审批）。
	AllowPublic bool `json:"allow_public"`
	// ForbidSelfApproval 禁自批。🔴 恒 true。
	ForbidSelfApproval bool `json:"forbid_self_approval"`
	// ForbidAutomationApproval 禁自动化身份审批。🔴 恒 true。
	ForbidAutomationApproval bool `json:"forbid_automation_approval"`
	// ForbidAutomationPublish 禁自动化身份点发布。🔴 恒 true。
	ForbidAutomationPublish bool `json:"forbid_automation_publish"`
}

// QuotaConf 发布配额（NFR-MAP-06）。
type QuotaConf struct {
	// DailyPublishLimit 24h 发布总量硬上限（超限阻断，需人工解除）。
	DailyPublishLimit int `json:"daily_publish_limit"`
	// PlatformHourlyLimit 单平台每小时上限。
	PlatformHourlyLimit int `json:"platform_hourly_limit"`
	// AccountMinInterval 账号两次发布最小间隔。
	AccountMinInterval time.Duration `json:"account_min_interval"`
}

// BreakerConf 熔断（NFR-MAP-04，详细设计 §6）。
type BreakerConf struct {
	// AccountFailThreshold 单账号连续失败熔断阈值（默认 3）。
	AccountFailThreshold int `json:"account_fail_threshold"`
	// PlatformFailRateThreshold 单平台 24h 失败率熔断阈值（默认 0.5）。
	PlatformFailRateThreshold float64 `json:"platform_fail_rate_threshold"`
	// PauseDuration 账号熔断暂停时长（默认 30m）。
	PauseDuration time.Duration `json:"pause_duration"`
}

// RetryConf 发布重试（asynq 重试/死信属 M4；此处定义语义供 worker 使用）。
type RetryConf struct {
	MaxAttempts int           `json:"max_attempts"`
	Backoff     string        `json:"backoff"` // exponential 固定
	BaseDelay   time.Duration `json:"base_delay"`
}

// ScheduleConf 排期（FR-PUB-06，双模式）。LocalWorkerEnabled 默认 false：asynq worker 属 M4 后续。
type ScheduleConf struct {
	// HumanizeJitter 拟人化偏移（±该值），避免整点批量可识别模式。
	HumanizeJitter time.Duration `json:"humanize_jitter"`
	// LocalWorkerEnabled 本机 worker 到点发是否启用（M4 后置，V1 仅平台原生/立即）。
	LocalWorkerEnabled bool `json:"local_worker_enabled"`
}

// FeedbackConf 表现回流（FR-PUB-08，Phase 3）。
type FeedbackConf struct {
	Enabled       bool          `json:"enabled"`
	FetchInterval time.Duration `json:"fetch_interval"`
	CsvFallback   bool          `json:"csv_fallback"`
}

// CredentialConf 凭据安全（🔴 NFR-MAP-03）。
type CredentialConf struct {
	// Vault 实现标识（V1 仅 local_encrypted）。
	Vault string `json:"vault"`
	// KeyEnv 密钥来源环境变量名（密钥不入库）。
	KeyEnv string `json:"key_env"`
	// MaskInLogs 日志脱敏。🔴 恒 true。
	MaskInLogs bool `json:"mask_in_logs"`
}

// DefaultConf 返回文档默认配置（详细设计 §8 map.yaml 数值对齐）。
func DefaultConf() Conf {
	return Conf{
		Enabled: true,
		Security: SecurityConf{
			DefaultVisibility:        "private",
			AllowPublic:              true,
			ForbidSelfApproval:       true,
			ForbidAutomationApproval: true,
			ForbidAutomationPublish:  true,
		},
		Quota: QuotaConf{
			DailyPublishLimit:   100,
			PlatformHourlyLimit: 20,
			AccountMinInterval:  5 * time.Minute,
		},
		Breaker: BreakerConf{
			AccountFailThreshold:      3,
			PlatformFailRateThreshold: 0.5,
			PauseDuration:             30 * time.Minute,
		},
		Retry: RetryConf{
			MaxAttempts: 3,
			Backoff:     "exponential",
			BaseDelay:   30 * time.Second,
		},
		Schedule: ScheduleConf{
			HumanizeJitter:     3 * time.Minute,
			LocalWorkerEnabled: false,
		},
		Feedback: FeedbackConf{
			Enabled:       false,
			FetchInterval: 6 * time.Hour,
			CsvFallback:   true,
		},
		Credential: CredentialConf{
			Vault:      "local_encrypted",
			KeyEnv:     "MAP_CREDENTIAL_KEY",
			MaskInLogs: true,
		},
	}
}

// Normalize 填充空值为默认值（yaml 只覆盖关心的键）。
// 🔴 含零值与时长大小的方向性：可以调得更严（更小配额/更长间隔），不可调得更松。
func (c *Conf) Normalize() {
	d := DefaultConf()

	if c.Security.DefaultVisibility == "" {
		c.Security.DefaultVisibility = d.Security.DefaultVisibility
	}
	// 四个布尔开关语义为「禁能」：缺省零值 false 视为未配置 → 回落到开启（更严）。
	if !c.Security.ForbidSelfApproval {
		c.Security.ForbidSelfApproval = d.Security.ForbidSelfApproval
	}
	if !c.Security.ForbidAutomationApproval {
		c.Security.ForbidAutomationApproval = d.Security.ForbidAutomationApproval
	}
	if !c.Security.ForbidAutomationPublish {
		c.Security.ForbidAutomationPublish = d.Security.ForbidAutomationPublish
	}

	if c.Quota.DailyPublishLimit <= 0 {
		c.Quota.DailyPublishLimit = d.Quota.DailyPublishLimit
	}
	if c.Quota.PlatformHourlyLimit <= 0 {
		c.Quota.PlatformHourlyLimit = d.Quota.PlatformHourlyLimit
	}
	if c.Quota.AccountMinInterval <= 0 {
		c.Quota.AccountMinInterval = d.Quota.AccountMinInterval
	}

	if c.Breaker.AccountFailThreshold <= 0 {
		c.Breaker.AccountFailThreshold = d.Breaker.AccountFailThreshold
	}
	if c.Breaker.PlatformFailRateThreshold <= 0 {
		c.Breaker.PlatformFailRateThreshold = d.Breaker.PlatformFailRateThreshold
	}
	if c.Breaker.PauseDuration <= 0 {
		c.Breaker.PauseDuration = d.Breaker.PauseDuration
	}

	if c.Retry.MaxAttempts <= 0 {
		c.Retry.MaxAttempts = d.Retry.MaxAttempts
	}
	if c.Retry.Backoff == "" {
		c.Retry.Backoff = d.Retry.Backoff
	}
	if c.Retry.BaseDelay <= 0 {
		c.Retry.BaseDelay = d.Retry.BaseDelay
	}

	if c.Schedule.HumanizeJitter <= 0 {
		c.Schedule.HumanizeJitter = d.Schedule.HumanizeJitter
	}

	if c.Feedback.FetchInterval <= 0 {
		c.Feedback.FetchInterval = d.Feedback.FetchInterval
	}

	if c.Credential.Vault == "" {
		c.Credential.Vault = d.Credential.Vault
	}
	if c.Credential.KeyEnv == "" {
		c.Credential.KeyEnv = d.Credential.KeyEnv
	}
	// 🔴 MaskInLogs 缺省强制开。
	if !c.Credential.MaskInLogs {
		c.Credential.MaskInLogs = true
	}
}

// Validate 校验红线配置（🔴 等同 biz/tt/conf.go 的红线校验模式）。
func (c *Conf) Validate() error {
	if c.Security.DefaultVisibility != "private" {
		return fmt.Errorf("security.default_visibility 恒为 private（NFR-MAP-01，🔴 不可改），got %q", c.Security.DefaultVisibility)
	}
	if !c.Security.ForbidSelfApproval {
		return errors.New("security.forbid_self_approval 恒为 true（NFR-MAP-02，🔴 不可关闭）")
	}
	if !c.Security.ForbidAutomationApproval {
		return errors.New("security.forbid_automation_approval 恒为 true（NFR-MAP-02，🔴 不可关闭）")
	}
	if !c.Security.ForbidAutomationPublish {
		return errors.New("security.forbid_automation_publish 恒为 true（🔴 不可关闭）")
	}
	if !c.Credential.MaskInLogs {
		return errors.New("credential.mask_in_logs 恒为 true（NFR-MAP-03，🔴 不可关闭）")
	}
	if c.Quota.DailyPublishLimit <= 0 {
		return errors.New("quota.daily_publish_limit 必须 > 0（NFR-MAP-06 总量硬上限）")
	}
	if c.Breaker.AccountFailThreshold < 2 {
		return errors.New("breaker.account_fail_threshold 至少 2（熔断过快过慢都失义）")
	}
	if c.Breaker.PlatformFailRateThreshold <= 0 || c.Breaker.PlatformFailRateThreshold > 1 {
		return fmt.Errorf("breaker.platform_fail_rate_threshold 须在 (0,1]，got %v", c.Breaker.PlatformFailRateThreshold)
	}
	if c.Retry.Backoff != "" && c.Retry.Backoff != "exponential" {
		return fmt.Errorf("retry.backoff 仅支持 exponential，got %q", c.Retry.Backoff)
	}
	return nil
}
