package tt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Conf TT 子系统配置（对应 app/admin/config/tt.yaml 的 `tt:` 段，详设 §4）。
//
// 不设为 proto（避免 conf.pb.go 再生成的重链），由 main 用 kratos config
// `c.Value("tt").Scan(&ttb)` 扫描该结构（json tag 须与 yaml 键名一致）。
type Conf struct {
	Enabled   bool                `json:"enabled"`
	Ingest    IngestConf          `json:"ingest"`
	Sampling  SamplingConf        `json:"sampling"`
	Influx    InfluxConf          `json:"influx"`
	Buffer    BufferConf          `json:"buffer"`
	Timezone  TimezoneConf        `json:"timezone"`
	Heat      HeatConf            `json:"heat"`
	Lifecycle LifecycleWorkerConf `json:"lifecycle"`
}

// durationJSONPaths 登记 Conf 中所有 time.Duration 字段的 JSON 路径。
// 时长在 yaml 按 Go 字面量书写（"5s"/"24h"），而 kratos Scan 走 encoding/json，
// json 向 time.Duration 只收纳秒整数——此钩子先把字符串解析成纳秒数再让标准
// 解码接手。⚠️ 新增 Duration 字段必须在此登记，否则 Scan 直接报错（fail-fast）。
var durationJSONPaths = [][]string{
	{"ingest", "flush_interval"},
	{"ingest", "dedup_ttl"},
	{"ingest", "last_wins_guard_ttl"},
	{"influx", "timeout"},
	{"buffer", "replay_interval"},
	{"lifecycle", "interval"},
}

// UnmarshalJSON 时长字面量 "24h" → 纳秒数的预归一（动机见 durationJSONPaths 注释）。
func (c *Conf) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for _, path := range durationJSONPaths {
		if err := rewriteDurationJSON(m, path); err != nil {
			return err
		}
	}
	rb, err := json.Marshal(m)
	if err != nil {
		return err
	}
	// 去方法别名防 UnmarshalJSON 递归；其余字段走标准解码。
	type confAlias Conf
	return json.Unmarshal(rb, (*confAlias)(c))
}

// rewriteDurationJSON 沿 path 找到字符串值并就地改写为纳秒数；路径缺块或
// 已为数字（纳秒直传）时不介入——缺省值由 Normalize 兜底。
func rewriteDurationJSON(m map[string]any, path []string) error {
	cur := m
	for _, k := range path[:len(path)-1] {
		sub, ok := cur[k].(map[string]any)
		if !ok {
			return nil
		}
		cur = sub
	}
	s, ok := cur[path[len(path)-1]].(string)
	if !ok {
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("tt 配置时长 %s=%q 非法: %w", strings.Join(path, "."), s, err)
	}
	cur[path[len(path)-1]] = float64(d)
	return nil
}

// HeatConf 归一化与动态参数（03 §2.1 + A-3/A-4/A-5/S-7 可配项）。
type HeatConf struct {
	// FreshnessTauHours 新鲜度时间常数 τ（A-4；半衰期 = τ·ln2 ≈ 8.32h）。
	FreshnessTauHours float64 `json:"freshness_tau_hours"`
	// FreshnessFloor 新鲜度地板（A-4：0.05，保二次爆发可达 peak）。
	FreshnessFloor float64 `json:"freshness_floor"`
	// SupportedPlatforms IC 实际已接入平台数（A-5：ref=min(8,P)；0=按 8 兼容）。
	SupportedPlatforms int `json:"supported_platforms"`
	// EngMinWindowN eng 分位冷启动阈值（A-3：N<200 退化静态基线）。
	EngMinWindowN int64 `json:"eng_min_window_n"`
	// Dynamics EWMA 时间常数等（S-7：T=0.75h，α 时间感知）。
	Dynamics DynamicsConf `json:"dynamics"`
}

// IngestConf 消费 mm.intel.normalized.v1 的行为参数。
type IngestConf struct {
	// Durable JetStream 持久订阅名。
	Durable string `json:"durable"`
	// BatchSize 单批写 InfluxDB 的最大点数（详设 §4 ingest.batch_size）。
	BatchSize int `json:"batch_size"`
	// FlushInterval 未满批时的最长驻留时间。
	FlushInterval time.Duration `json:"flush_interval"`
	// DedupTTL event_id 幂等去重窗口（Redis SET NX PX）。
	DedupTTL time.Duration `json:"dedup_ttl"`
	// LastWinsGuardTTL 同桶 last-wins 守卫 key 的 TTL（A-14 _as_of 最大者胜出）。
	LastWinsGuardTTL time.Duration `json:"last_wins_guard_ttl"`
}

// SamplingConf 采样纪律（NFR-TT-04：热榜默认 30 分钟，不得高于 newsnow 缓存频率）。
type SamplingConf struct {
	// BucketMinutes 时间桶对齐粒度（A-14：t_bucket = floor(t / bucket) × bucket）。
	BucketMinutes int `json:"bucket_minutes"`
	// HotlistIntervalMinutes 热榜源采样间隔下限（info 用，不做强制下发）。
	HotlistIntervalMinutes int `json:"hotlist_interval_minutes"`
	// LateHours _as_of 落后超过该值视为迟到数据（A-14），写入并打 late=true。
	LateHours int `json:"late_hours"`
	// FutureSkewMinutes _as_of 超出 now 该值视为未来时间戳，丢弃并计数。
	FutureSkewMinutes int `json:"future_skew_minutes"`
}

// InfluxConf InfluxDB 2.x 连接与 bucket 规划（A-17：mm_raw/mm_1h/mm_1d 三层）。
type InfluxConf struct {
	URL   string `json:"url"`
	Token string `json:"token"` // 支持 ${ENV} 环境变量引用（禁止明文入库）
	Org   string `json:"org"`
	// BucketRaw 原始采样点 bucket（90d 保留）。
	BucketRaw string `json:"bucket_raw"`
	// Bucket1h 1 小时聚合 bucket（1y 保留，A-17 由原 mm_5m 升级）。
	Bucket1h string `json:"bucket_1h"`
	// Bucket1d 1 天聚合 bucket（3y 保留）。
	Bucket1d string `json:"bucket_1d"`
	// BatchSize 写批大小（详设 §4 influx.batch_size）。
	BatchSize int `json:"batch_size"`
	// FlushMs 批驻留毫秒数。
	FlushMs int `json:"flush_ms"`
	// Timeout 单次写/查超时。
	Timeout time.Duration `json:"timeout"`
	// PingOnStart 启动时健康检查（失败告警不阻塞，降级运行时由 buffer 兜底）。
	PingOnStart bool `json:"ping_on_start"`
}

// BufferConf 写失败缓冲（A-16：Redis Stream 死信队列 + 背压，替代「TTL 1h 队列」）。
type BufferConf struct {
	// StreamKey 缓冲流名。
	StreamKey string `json:"stream_key"`
	// PoisonKey 重试超限后的毒消息流名（保留 30d，人工介入）。
	PoisonKey string `json:"poison_key"`
	// ConsumerGroup 重放消费组。
	ConsumerGroup string `json:"consumer_group"`
	// MaxLen 流上限（XADD MAXLEN ≈）；超出时 XTRIM 最旧并计数 tt_influx_dropped_total。
	MaxLen int64 `json:"max_len"`
	// PauseDepth 缓冲深度超过该值时暂停 NATS 拉取（背压，不 nack 防 redelivery 风暴）。
	PauseDepth int64 `json:"pause_depth"`
	// ResumeDepth 缓冲深度回落到该值以下时恢复拉取。
	ResumeDepth int64 `json:"resume_depth"`
	// MaxAttempts 单条重放重试上限，超限进 poison。
	MaxAttempts int `json:"max_attempts"`
	// ReplayBatch 每次重放读取条数。
	ReplayBatch int64 `json:"replay_batch"`
	// ReplayInterval 重放周期。
	ReplayInterval time.Duration `json:"replay_interval"`
}

// TimezoneConf 时区纪律（NFR-TT-03：存储一律 UTC，展示 Asia/Shanghai）。
type TimezoneConf struct {
	// Storage 恒为 UTC（🔴 不可改）。
	Storage string `json:"storage"`
	// Display 展示时区。
	Display string `json:"display"`
}

// LifecycleWorkerConf 生命周期评估 worker 运行参数（A-8 周期评估）。
type LifecycleWorkerConf struct {
	// Interval 评估周期（默认 15min，对齐采样桶节奏；重启幂等使周期错位无害）。
	Interval time.Duration `json:"interval"`
	// ScanLimit 单轮扫描话题上限（≤0 取 data 层默认 500；topic_id 升序游标）。
	ScanLimit int `json:"scan_limit"`
	// Concurrency 单轮并发评估数（每话题 2 次时序读，钳制 InfluxDB 查询尖峰）。
	Concurrency int `json:"concurrency"`
	// Rules 阶段判定阈值（零值整块由 Normalize 填 DefaultLifecycleConf；
	// 部分覆盖的逐项填，见 NormalizeLifecycleRules）。
	Rules LifecycleConf `json:"rules"`
}

// NormalizeLifecycleRules 逐项填充零值规则字段（yaml 只覆盖关心的键）。
func (c *LifecycleWorkerConf) NormalizeLifecycleRules() {
	d := DefaultLifecycleConf()
	if c.Rules.VMin == 0 {
		c.Rules.VMin = d.VMin
	}
	if c.Rules.HeatPeakMin == 0 {
		c.Rules.HeatPeakMin = d.HeatPeakMin
	}
	if c.Rules.HeatTurnMin == 0 {
		c.Rules.HeatTurnMin = d.HeatTurnMin
	}
	if c.Rules.LongTailHeatMax == 0 {
		c.Rules.LongTailHeatMax = d.LongTailHeatMax
	}
	if c.Rules.LongTailAgeDays == 0 {
		c.Rules.LongTailAgeDays = d.LongTailAgeDays
	}
	if c.Rules.DormantHours == 0 {
		c.Rules.DormantHours = d.DormantHours
	}
	if c.Rules.RisingMinStreak == 0 {
		c.Rules.RisingMinStreak = d.RisingMinStreak
	}
	if c.Rules.DecayMinStreak == 0 {
		c.Rules.DecayMinStreak = d.DecayMinStreak
	}
}

// DefaultConf 返回文档默认配置（与 02-TT 03-详细设计.md §4 tt.yaml 对齐）。
func DefaultConf() Conf {
	return Conf{
		Enabled: true,
		Ingest: IngestConf{
			Durable:          "tt-ingest-intel",
			BatchSize:        200,
			FlushInterval:    5 * time.Second,
			DedupTTL:         24 * time.Hour,
			LastWinsGuardTTL: 48 * time.Hour,
		},
		Sampling: SamplingConf{
			BucketMinutes:        30,
			HotlistIntervalMinutes: 30,
			LateHours:            24,
			FutureSkewMinutes:    5,
		},
		Influx: InfluxConf{
			URL:        "http://127.0.0.1:8086",
			Org:        "media",
			BucketRaw:  "mm_raw",
			Bucket1h:   "mm_1h",
			Bucket1d:   "mm_1d",
			BatchSize:  500,
			FlushMs:    1000,
			Timeout:    10 * time.Second,
			PingOnStart: true,
		},
		Buffer: BufferConf{
			StreamKey:      "tt:influx:buf",
			PoisonKey:      "tt:influx:poison",
			ConsumerGroup:  "tt_writer",
			MaxLen:         2_000_000,
			PauseDepth:     500_000,
			ResumeDepth:    100_000,
			MaxAttempts:    5,
			ReplayBatch:    200,
			ReplayInterval: 15 * time.Second,
		},
		Timezone: TimezoneConf{
			Storage: "UTC",
			Display: "Asia/Shanghai",
		},
		Heat: HeatConf{
			FreshnessTauHours:  12,
			FreshnessFloor:     0.05,
			SupportedPlatforms: 0, // 0 = 按 A-5 ref=8 兼容口径
			EngMinWindowN:      200,
			Dynamics:           DefaultDynamicsConf(),
		},
		Lifecycle: LifecycleWorkerConf{
			Interval:    15 * time.Minute,
			ScanLimit:   500,
			Concurrency: 8,
			Rules:       DefaultLifecycleConf(),
		},
	}
}

// Normalize 填充空值为默认值（yaml 只覆盖关心的键）。
func (c *Conf) Normalize() {
	d := DefaultConf()
	if c.Ingest.Durable == "" {
		c.Ingest.Durable = d.Ingest.Durable
	}
	if c.Ingest.BatchSize <= 0 {
		c.Ingest.BatchSize = d.Ingest.BatchSize
	}
	if c.Ingest.FlushInterval <= 0 {
		c.Ingest.FlushInterval = d.Ingest.FlushInterval
	}
	if c.Ingest.DedupTTL <= 0 {
		c.Ingest.DedupTTL = d.Ingest.DedupTTL
	}
	if c.Ingest.LastWinsGuardTTL <= 0 {
		c.Ingest.LastWinsGuardTTL = d.Ingest.LastWinsGuardTTL
	}
	if c.Sampling.BucketMinutes <= 0 {
		c.Sampling.BucketMinutes = d.Sampling.BucketMinutes
	}
	if c.Sampling.HotlistIntervalMinutes <= 0 {
		c.Sampling.HotlistIntervalMinutes = d.Sampling.HotlistIntervalMinutes
	}
	if c.Sampling.LateHours <= 0 {
		c.Sampling.LateHours = d.Sampling.LateHours
	}
	if c.Sampling.FutureSkewMinutes <= 0 {
		c.Sampling.FutureSkewMinutes = d.Sampling.FutureSkewMinutes
	}
	if c.Influx.URL == "" {
		c.Influx.URL = d.Influx.URL
	}
	if c.Influx.Org == "" {
		c.Influx.Org = d.Influx.Org
	}
	if c.Influx.BucketRaw == "" {
		c.Influx.BucketRaw = d.Influx.BucketRaw
	}
	if c.Influx.Bucket1h == "" {
		c.Influx.Bucket1h = d.Influx.Bucket1h
	}
	if c.Influx.Bucket1d == "" {
		c.Influx.Bucket1d = d.Influx.Bucket1d
	}
	if c.Influx.BatchSize <= 0 {
		c.Influx.BatchSize = d.Influx.BatchSize
	}
	if c.Influx.FlushMs <= 0 {
		c.Influx.FlushMs = d.Influx.FlushMs
	}
	if c.Influx.Timeout <= 0 {
		c.Influx.Timeout = d.Influx.Timeout
	}
	if c.Buffer.StreamKey == "" {
		c.Buffer.StreamKey = d.Buffer.StreamKey
	}
	if c.Buffer.PoisonKey == "" {
		c.Buffer.PoisonKey = d.Buffer.PoisonKey
	}
	if c.Buffer.ConsumerGroup == "" {
		c.Buffer.ConsumerGroup = d.Buffer.ConsumerGroup
	}
	if c.Buffer.MaxLen <= 0 {
		c.Buffer.MaxLen = d.Buffer.MaxLen
	}
	if c.Buffer.PauseDepth <= 0 {
		c.Buffer.PauseDepth = d.Buffer.PauseDepth
	}
	if c.Buffer.ResumeDepth <= 0 {
		c.Buffer.ResumeDepth = d.Buffer.ResumeDepth
	}
	if c.Buffer.MaxAttempts <= 0 {
		c.Buffer.MaxAttempts = d.Buffer.MaxAttempts
	}
	if c.Buffer.ReplayBatch <= 0 {
		c.Buffer.ReplayBatch = d.Buffer.ReplayBatch
	}
	if c.Buffer.ReplayInterval <= 0 {
		c.Buffer.ReplayInterval = d.Buffer.ReplayInterval
	}
	if c.Timezone.Storage == "" {
		c.Timezone.Storage = d.Timezone.Storage
	}
	if c.Timezone.Display == "" {
		c.Timezone.Display = d.Timezone.Display
	}
	if c.Heat.FreshnessTauHours <= 0 {
		c.Heat.FreshnessTauHours = d.Heat.FreshnessTauHours
	}
	if c.Heat.FreshnessFloor <= 0 {
		c.Heat.FreshnessFloor = d.Heat.FreshnessFloor
	}
	if c.Heat.EngMinWindowN <= 0 {
		c.Heat.EngMinWindowN = d.Heat.EngMinWindowN
	}
	if c.Heat.Dynamics.EWMATauHours <= 0 {
		c.Heat.Dynamics.EWMATauHours = d.Heat.Dynamics.EWMATauHours
	}
	if c.Lifecycle.Interval <= 0 {
		c.Lifecycle.Interval = d.Lifecycle.Interval
	}
	if c.Lifecycle.ScanLimit <= 0 {
		c.Lifecycle.ScanLimit = d.Lifecycle.ScanLimit
	}
	if c.Lifecycle.Concurrency <= 0 {
		c.Lifecycle.Concurrency = d.Lifecycle.Concurrency
	}
	// 规则零值逐项兜底（Rules 无整块零值判定——Validate 拒零值配置前须先填默认）。
	c.Lifecycle.NormalizeLifecycleRules()
}

// Validate 校验红线配置。
func (c *Conf) Validate() error {
	if c.Sampling.BucketMinutes < 2 || c.Sampling.BucketMinutes > 60 {
		return fmt.Errorf("sampling.bucket_minutes 须在 2-60（FR-TRK-01），got %d", c.Sampling.BucketMinutes)
	}
	if c.Sampling.HotlistIntervalMinutes < 30 {
		return errors.New("sampling.hotlist_interval_minutes 不得低于 30（NFR-TT-04 newsnow 缓存限制）")
	}
	if c.Timezone.Storage != "UTC" {
		return errors.New("timezone.storage 恒为 UTC（NFR-TT-03，🔴 不可改）")
	}
	if c.Buffer.ResumeDepth >= c.Buffer.PauseDepth {
		return errors.New("buffer.resume_depth 必须小于 buffer.pause_depth（背压滞回）")
	}
	return nil
}
