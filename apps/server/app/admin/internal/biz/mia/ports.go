package mia

import (
	"context"
	"time"

	"framework/pkg/mediacontract"
)

// ============ 仓储端口（data 层实现） ============

// EntityMappingRepo 映射表 A 仓储。
type EntityMappingRepo interface {
	Create(ctx context.Context, item *MappingItem) (*MappingItem, error)
	Update(ctx context.Context, id int64, item *MappingItem) (*MappingItem, error)
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*MappingItem, error)
	List(ctx context.Context, q MappingQuery) ([]*MappingItem, int, error)
	// ListEnabled 加载全部启用的映射（实体链接在内存中匹配）。
	ListEnabled(ctx context.Context) ([]*MappingItem, error)
	// UpsertByKey CSV 导入按唯一键 (entity_name, asset_class, symbol) 幂等写入。
	UpsertByKey(ctx context.Context, item *MappingItem) (created bool, err error)
	// Count 映射行数（种子判定：≥200 条时跳过默认种子）。
	Count(ctx context.Context) (int, error)
}

// NarrativeMappingRepo 映射表 B 仓储。
type NarrativeMappingRepo interface {
	Create(ctx context.Context, item *NarrativeItem) (*NarrativeItem, error)
	Update(ctx context.Context, id int64, item *NarrativeItem) (*NarrativeItem, error)
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*NarrativeItem, error)
	List(ctx context.Context, q NarrativeQuery) ([]*NarrativeItem, int, error)
	// ListEnabled 加载全部启用的叙事映射（叙事匹配在内存中完成）。
	ListEnabled(ctx context.Context) ([]*NarrativeItem, error)
	// UpsertByKey CSV 导入按唯一键 narrative 幂等写入。
	UpsertByKey(ctx context.Context, item *NarrativeItem) (created bool, err error)
	Count(ctx context.Context) (int, error)
}

// EventCardRepo 事件卡仓储（FR-MIA-05）。
type EventCardRepo interface {
	Create(ctx context.Context, row *EventCardRow) (*EventCardRow, error)
	Get(ctx context.Context, eventID string) (*EventCardRow, error)
	List(ctx context.Context, q EventQuery) ([]*EventCardRow, int, error)
	// ListRecent 拉取近 window 的事件卡，供自动聚类比对。
	ListRecent(ctx context.Context, window time.Duration) ([]*EventCardRow, error)
	// AbsorbSource 把一条新来源追加进既有事件卡（自动聚类合并）。
	AbsorbSource(ctx context.Context, eventID, intelID, domain string, occurredAt time.Time) error
	// ReplaceSourcesForSplit 人工拆分：原子地把指定来源从父卡拆出为新卡，两卡均锁定。
	ReplaceSourcesForSplit(ctx context.Context, parent, newCard *EventCardRow) error
	// MergeInto 人工合并：把 absorbedIDs 全部来源并入 target，两卡均锁定，被并卡删除。
	MergeInto(ctx context.Context, targetID string, absorbedIDs []string) error
}

// ImpactCardRepo 影响卡仓储（FR-MIA-06）。
type ImpactCardRepo interface {
	// Create 落库前 biz 层必须已过三项硬校验（disclaimer/path/confidence）。
	Create(ctx context.Context, row *ImpactCardRow) (*ImpactCardRow, error)
	Get(ctx context.Context, impactID string) (*ImpactCardRow, error)
	List(ctx context.Context, q ImpactQuery) ([]*ImpactCardRow, int, error)
	// MarkPushed 推送幂等标记。
	MarkPushed(ctx context.Context, impactID string) error
}

// TransmissionRuleRepo 传导规则仓储（FR-MIA-08）。
type TransmissionRuleRepo interface {
	Create(ctx context.Context, row *TransmissionRuleRow) (*TransmissionRuleRow, error)
	Update(ctx context.Context, id int64, row *TransmissionRuleRow) (*TransmissionRuleRow, error)
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, enabledOnly bool) ([]*TransmissionRuleRow, error)
	Count(ctx context.Context) (int, error)
}

// QuoteCacheRepo 行情缓存仓储（FR-MIA-07）。
type QuoteCacheRepo interface {
	Get(ctx context.Context, symbol, provider string, maxAge time.Duration) (*QuoteSnapshot, error)
	Upsert(ctx context.Context, snap *QuoteSnapshot) error
}

// ============ 外部依赖端口（data 层适配器实现） ============

// SentimentInput 情感分类输入。
type SentimentInput struct {
	Title   string
	Text    string
	Lang    string
	IsMacro bool
}

// SentimentResult 情感分类输出。
type SentimentResult struct {
	Label mediacontract.SentimentLabel
	Score float64 // -1 ~ 1
}

// SentimentEngine 情感引擎端口（详细设计 §2.2 SentimentEngine）。
// 实现：FinBERT 自托管推理 / OpenAI 兼容 LLM（云）/ Ollama（本地降级）。
type SentimentEngine interface {
	// Available 引擎是否已配置（URL/Key 齐全）。不配置 = 降级跳过，不算失败。
	Available() bool
	Name() string // finbert | llm-cloud | llm-ollama（观测标签）
	Classify(ctx context.Context, in SentimentInput) (SentimentResult, error)
}

// QuoteProvider 行情提供方端口（🔴 只读；绝不出现下单/撤单方法）。
type QuoteProvider interface {
	Name() string // openbb | ccxt
	FetchOHLCV(ctx context.Context, symbol string) (*QuoteSnapshot, error)
}

// ImpactPublisher ImpactCard 事件发布端口（NATS mm.impact.card.v1）。
type ImpactPublisher interface {
	// PublishImpactCard 发布包裹 EventEnvelope 的影响卡（含 disclaimer）。
	PublishImpactCard(ctx context.Context, card *mediacontract.ImpactCard, traceID string) error
}

// WebhookSender 高分推送端口（Webhook / 企业微信）。
type WebhookSender interface {
	Send(ctx context.Context, body []byte) error
}

// ============ biz 层行类型（与 ent 行解耦） ============

// EventCardRow 事件卡（biz 视图）。
type EventCardRow struct {
	ID             int64
	EventID        string // ec_
	Title          string
	Summary        string
	OccurredAt     time.Time
	TopicID        string
	SourceIntelIDs []string
	SourceCount    int
	Domains        []string
	Entities       []mediacontract.Entity
	Category       string // policy|company|macro|tech|crypto
	Lang           string
	MergedBy       string // auto|manual
	SplitLocked    bool
	CreatedAt      time.Time
}

// ImpactCardRow 影响卡（biz 视图）。
type ImpactCardRow struct {
	ID              int64
	ImpactID        string // im_
	EventID         string
	TopicID         string
	Title           string
	Summary         string
	Lang            string
	SentimentLabel  mediacontract.SentimentLabel
	SentimentScore  float64
	SentimentEngine mediacontract.SentimentEngine
	ImpactScore     float64
	Direction       mediacontract.Direction
	TTLHours        int
	ExpiresAt       time.Time
	Factors         mediacontract.ImpactFactors
	Affected        []mediacontract.AffectedItem
	NoMapping       bool
	Disclaimer      string
	RuleVersion     string
	WeightsSnapshot map[string]any
	OccurredAt      time.Time
	Pushed          bool
	CreatedAt       time.Time
}

// ToContract 转换契约 ImpactCard（发布/详情用）。
func (r *ImpactCardRow) ToContract() *mediacontract.ImpactCard {
	return &mediacontract.ImpactCard{
		APIVersion: "1.0",
		ImpactID:   r.ImpactID,
		TopicID:    r.TopicID,
		Event: mediacontract.ImpactEvent{
			Title:      r.Title,
			Summary:    r.Summary,
			Sources:    nil, // 契约 event.sources 由事件卡指针回填
			OccurredAt: r.OccurredAt,
		},
		Sentiment: mediacontract.Sentiment{
			Label:  r.SentimentLabel,
			Score:  r.SentimentScore,
			Engine: r.SentimentEngine,
			Lang:   r.Lang,
		},
		Impact: mediacontract.Impact{
			Score:    r.ImpactScore,
			TTLHours: r.TTLHours,
			Factors:  r.Factors,
		},
		Affected:   r.Affected,
		Disclaimer: r.Disclaimer,
	}
}

// TransmissionRuleRow 传导规则（biz 视图）。
type TransmissionRuleRow struct {
	ID           int64
	RuleID       string // rule_
	Trigger      mediacontract.TriggerSpec
	FromAsset    string
	ToAsset      string
	ToDirection  mediacontract.Direction
	Confidence   float64 // 🔴 (0, 0.6]
	Note         string  // 🔴 必填
	Enabled      bool
	HitCount     int
	CorrectCount int
}
