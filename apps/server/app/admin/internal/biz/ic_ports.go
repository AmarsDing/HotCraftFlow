package biz

import (
	"context"
	"time"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// ① IC 端口（依赖倒置：biz 只认接口；collector/data 适配器注入实现）
// 评审 S-22：RawFragment 已下沉 mediacontract，端口仅声明行为。
// =============================================================================

// RawCollectItem 采集器单条产出（未归一化；确认 B-8 主链路第 1 步产物形态）。
type RawCollectItem struct {
	SourceURL   string
	Author      string  // 公开昵称；平台用户 ID 与主页 URL 禁止（PIPL / M-18）
	Title       string
	Text        string
	MediaRefs   []string
	Lang        string  // 上游声明可为空；S-25 检测兜底在归一化层
	PublishedAt *time.Time

	Rank      int32 // 热榜序（hotlist 路必须带，否则视为解析失败）
	RankTotal int32
	Views     int64
	Likes     int64
	Comments  int64

	Fragment *contract.RawFragment // 证据原文（B-8 第 3 步落盘）
}

// Collector 一路采集器的方法级端口（S-21：运行期注册表只读，init 自注册）。
type Collector interface {
	// Method 返回 hotlist|api|rss|crawl|sidecar（契约 §1）。
	Method() string
	// Collect 一轮抓取；级联失败以带 reason 的错误返回（错误码映射在 collect_usecase）。
	Collect(ctx context.Context, src *IntelSourceRow) ([]*RawCollectItem, error)
}

// CollectorRegistry 方法分发表（M-5：ic.collectors 每路可独立启用，注册器负责过滤）。
type CollectorRegistry interface {
	// Get 返回该 method 的采集器；不可用（未注册/被配置禁用）返回 nil,false。
	Get(method string) (Collector, bool)
	// EnabledMethods 当前启用清单（观测/健康检查用）。
	EnabledMethods() []string
}

// EvidenceStorePort 证据落盘（S-27：V1 本机目录；B-17 对象锁语义由实现保证）。
type EvidenceStorePort interface {
	// Save 落盘 + 截断 + 返回元信息。rawHash 基于截断后字节。
	Save(ctx context.Context, intelID string, fragment *contract.RawFragment, maxBytes int64) (rawPath string, rawHash string, size int64, truncated bool, err error)
	// Delete 保留期清扫。
	Delete(ctx context.Context, rawPath string) error
	// Export 打包 ZIP（M-19：导出方负责 meta.json 审计字段）。
	Export(ctx context.Context, intelID string, meta map[string]string) (zipPath string, zipSHA256 string, err error)
}

// RateLimiterPort 令牌桶（S-10：平台级 > 信源级；持久退避带抖动）。
type RateLimiterPort interface {
	// Allow key 粒度令牌；不足返回 false（采集器本轮跳过，错误码 IC_RATE_LIMITED）。
	Allow(ctx context.Context, key string, perMin int32) (bool, error)
}

// RobotsCheckerPort robots 三态（B-16 fail-closed：取不到 robots 视为「未声明」而非放行）。
type RobotsCheckerPort interface {
	// Check：返回 (allowed, crawlDelayMs, robotsFound, err)。
	// robotsExempt=true（S-8 授权豁免）时跳过抓取直接放行。
	Check(ctx context.Context, method, rawURL string, exempt bool) (allowed bool, crawlDelayMs int32, found bool, err error)
}

// ShortlinkExpanderPort 短链展开（S-6：block_private_ip 恒真，重定向上限由实现强制执行）。
type ShortlinkExpanderPort interface {
	// Expand 返回最终 URL；SSRF 命中私网返回错误（错误码 IC_URL_BLOCKED）。
	Expand(ctx context.Context, rawURL string) (string, error)
}

// EmbedderPort 语义向量（B-7 日配额硬熔断；store=none 时实现返回不可用并跳过 L3）。
type EmbedderPort interface {
	// Embed 写入向量库返回引用 ID；日配额用尽返回 (empty, ErrEmbeddingCap)。
	Embed(ctx context.Context, intelID, text string) (embeddingID string, err error)
	// SearchSimilar L3 召回：窗口内相似度 ≥ threshold 的近邻（按分数降序）。
	// 实现需排除 intelID 自身；不可用时返回 ErrEmbeddingUnavailable。
	SearchSimilar(ctx context.Context, intelID string, threshold float64, windowHours int32) ([]EmbeddingNeighbor, error)
	// Enabled 配置开关（store != none）。
	Enabled() bool
	// DailyRemaining 剩余配额（观测用）。
	DailyRemaining(ctx context.Context) int32
}

// EmbeddingNeighbor L3 召回邻居。
type EmbeddingNeighbor struct {
	IntelID string
	Score   float64 // 余弦相似度 [0,1]
}

// LLMEnhancePort M5 增强预留（ADR-IC-07：默认规则版，LLM 失败回退规则）。
type LLMEnhancePort interface {
	Enabled() bool
	// Extract 返回 (entities, category, workability) 建议；服务不可靠返回错误。
	Extract(ctx context.Context, item *contract.IntelItem) (*ExtractSuggestion, error)
}

type ExtractSuggestion struct {
	Entities         []contract.Entity
	Category         string
	WorkabilityScore float64
	WorkabilityReason string
}

// IntelPublisherPort 契约 §8.2 三条 subject 的发布端口（实现：core NATS JetStream）。
// subject 取 mediacontract.SubjectIntelRaw / SubjectIntelNormalized / SubjectIntelBlocked；
// msgID = intel_id（Nats-Msg-Id 幂等键，评审 B-25 投递去重）。
type IntelPublisherPort interface {
	PublishIntel(ctx context.Context, subject string, env *contract.EventEnvelope, msgID string) error
}
