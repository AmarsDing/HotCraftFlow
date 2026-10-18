package mia

import (
	"time"

	"framework/app/admin/internal/conf"
	"framework/pkg/mediacontract"
)

// Config 是 biz 层解析后的 MIA 运行配置视图（来自 conf.Mia，带默认值补齐）。
// 合规红线字段在此再次兜底：allow_guess 强制 false、read_only 强制 true、
// enable_trading 强制 false（详细设计 §4：配置层硬关闭）。
type Config struct {
	Enabled        bool
	InternalOnly   bool
	Disclaimer     string // 🔴 空则启动即视为配置错误（ingest 处硬失败）
	MinScoreToPush float64

	EquityCronTrading    string
	EquityCronAfterClose string
	EquityTimezone       string // IANA 时区名，默认 Asia/Shanghai
	CryptoCron           string
	CryptoEnabled        bool

	FinBERTURL     string
	FinBERTTimeout time.Duration
	LLMCloudBaseURL string
	LLMCloudAPIKey  string
	LLMCloudModel   string
	LLMOllamaURL    string
	LLMOllamaModel  string
	LLMDailyCallCap int

	QuoteProviders  []string
	CCXTSidecarURL  string
	QuoteCacheTTL   time.Duration

	DefaultTTLHours     map[mediacontract.AssetClass]int
	SourceWeightBase    float64
	SourceWeightLogCoef float64
	MinEntityConfidence float64

	ClusterSimilarityThreshold float64
	ClusterWindowHours         int

	VerifyEnabled           bool
	VerifyMoveThresholdPct  float64
	VerifyMinSampleForReport int

	NotifyWebhookURL string
	NotifyTimeout    time.Duration
}

// RuleVersion 当前规则集版本（可复算，NFR-MIA-10）。
// 权重/阈值变更时每张卡快照 weights_snapshot。
const RuleVersion = "mia-rules-v1.0"

// ResolveConfig 把 conf.Mia 解析为运行时 Config，并补齐默认值。
// conf 为 nil（未配置 mia.yaml）时返回 MIA 关闭的安全默认（Enabled=false）。
func ResolveConfig(bc *conf.Bootstrap) Config {
	cfg := Config{
		Enabled:            false,
		InternalOnly:       true,
		Disclaimer:         mediacontract.ImpactDisclaimer,
		MinScoreToPush:     60,
		EquityTimezone:     "Asia/Shanghai",
		CryptoEnabled:      true,
		FinBERTTimeout:     5 * time.Second,
		LLMDailyCallCap:    500,
		QuoteCacheTTL:      5 * time.Minute,
		SourceWeightBase:   0.4,
		SourceWeightLogCoef: 0.15,
		MinEntityConfidence: 0.6,
		ClusterSimilarityThreshold: 0.82,
		ClusterWindowHours: 24,
		VerifyMoveThresholdPct:   0.3,
		VerifyMinSampleForReport: 30,
		NotifyTimeout:   5 * time.Second,
		DefaultTTLHours: map[mediacontract.AssetClass]int{
			mediacontract.AssetEquity:    24,
			mediacontract.AssetIndex:     24,
			mediacontract.AssetCommodity: 24,
			mediacontract.AssetFX:        12,
			mediacontract.AssetCrypto:    12,
		},
	}
	if bc == nil || bc.GetMia() == nil {
		return cfg
	}
	m := bc.GetMia()
	cfg.Enabled = m.GetEnabled()

	if o := m.GetOutput(); o != nil {
		cfg.InternalOnly = o.GetInternalOnly()
		if d := o.GetDisclaimer(); d != "" {
			cfg.Disclaimer = d
		}
		if v := o.GetMinScoreToPush(); v > 0 {
			cfg.MinScoreToPush = v
		}
	}
	if s := m.GetSchedule(); s != nil {
		if e := s.GetEquity(); e != nil {
			cfg.EquityCronTrading = e.GetCronTrading()
			cfg.EquityCronAfterClose = e.GetCronAfterClose()
			if tz := e.GetTimezone(); tz != "" {
				cfg.EquityTimezone = tz
			}
		}
		if c := s.GetCrypto(); c != nil {
			cfg.CryptoCron = c.GetCron()
			cfg.CryptoEnabled = c.GetEnabled()
		}
	}
	if sm := m.GetSentiment(); sm != nil {
		cfg.FinBERTURL = sm.GetFinbertUrl()
		if t := sm.GetFinbertTimeout(); t != nil && t.AsDuration() > 0 {
			cfg.FinBERTTimeout = t.AsDuration()
		}
		if l := sm.GetLlm(); l != nil {
			cfg.LLMCloudBaseURL = l.GetCloudBaseUrl()
			cfg.LLMCloudAPIKey = l.GetCloudApiKey()
			cfg.LLMCloudModel = firstNonEmpty(l.GetCloudModel(), "gpt-4o-mini")
			cfg.LLMOllamaURL = l.GetOllamaUrl()
			cfg.LLMOllamaModel = firstNonEmpty(l.GetOllamaModel(), "qwen2.5:7b-instruct")
			if cap := l.GetDailyCallCap(); cap > 0 {
				cfg.LLMDailyCallCap = int(cap)
			}
		}
		// 🔴 allow_guess 无论怎样都强制 false——NFR-MIA-08 禁止猜测，配置层不可开启。
	}
	if q := m.GetQuote(); q != nil {
		cfg.QuoteProviders = q.GetProviders()
		cfg.CCXTSidecarURL = q.GetCcxtSidecarUrl()
		if t := q.GetCacheTtl(); t != nil && t.AsDuration() > 0 {
			cfg.QuoteCacheTTL = t.AsDuration()
		}
		// 🔴 read_only / enable_trading 不落到配置视图——本系统没有交易通路（NFR-MIA-02）。
	}
	if sc := m.GetScoring(); sc != nil {
		for k, v := range sc.GetDefaultTtlHours() {
			if ac := mediacontract.AssetClass(k); ac.Valid() && v >= 6 && v <= 24 {
				cfg.DefaultTTLHours[ac] = int(v)
			}
		}
		if v := sc.GetSourceWeightBase(); v > 0 {
			cfg.SourceWeightBase = v
		}
		if v := sc.GetSourceWeightLogCoef(); v > 0 {
			cfg.SourceWeightLogCoef = v
		}
		if v := sc.GetMinEntityConfidence(); v > 0 && v <= 1 {
			cfg.MinEntityConfidence = v
		}
	}
	if cl := m.GetClustering(); cl != nil {
		if v := cl.GetSimilarityThreshold(); v > 0 && v <= 1 {
			cfg.ClusterSimilarityThreshold = v
		}
		if v := cl.GetWindowHours(); v > 0 {
			cfg.ClusterWindowHours = int(v)
		}
	}
	if v := m.GetVerify(); v != nil {
		cfg.VerifyEnabled = v.GetEnabled()
		if t := v.GetMoveThresholdPct(); t > 0 {
			cfg.VerifyMoveThresholdPct = t
		}
		if n := v.GetMinSampleForReport(); n > 0 {
			cfg.VerifyMinSampleForReport = int(n)
		}
	}
	if n := m.GetNotify(); n != nil {
		cfg.NotifyWebhookURL = n.GetWebhookUrl()
		if t := n.GetTimeout(); t != nil && t.AsDuration() > 0 {
			cfg.NotifyTimeout = t.AsDuration()
		}
	}
	return cfg
}

func firstNonEmpty(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

// TTLFor 资产类的默认 TTL（叙事映射可针对加密叙事覆盖为更短）。
func (c Config) TTLFor(ac mediacontract.AssetClass) int {
	if v, ok := c.DefaultTTLHours[ac]; ok && v >= 6 && v <= 24 {
		return v
	}
	if ac == mediacontract.AssetCrypto {
		return 12
	}
	return 24
}

// MappingItem 映射表 A 一行（biz 视图）。
type MappingItem struct {
	ID            int64
	EntityName    string
	EntityAlias   []string
	EntityType    mediacontract.EntityType
	AssetClass    mediacontract.AssetClass
	Symbol        string
	DirectionBias mediacontract.Direction
	Confidence    float64
	Source        string
	Notes         string
	Enabled       bool
	UpdatedAt     time.Time
}

// Validate 校验映射行（notes 必填、confidence ∈[0,1]、枚举合法）。
func (m MappingItem) Validate() error {
	if m.EntityName == "" || m.Symbol == "" {
		return ErrMappingNotFound
	}
	if !m.EntityType.Valid() || !m.AssetClass.Valid() || !m.DirectionBias.Valid() {
		return ErrMappingNotFound
	}
	if m.Notes == "" {
		return ErrPathEmpty // notes 即映射的影响路径依据，必填
	}
	if m.Confidence < 0 || m.Confidence > 1 {
		return ErrConfidenceInvalid
	}
	return nil
}

// NarrativeItem 映射表 B 一行（biz 视图）。
type NarrativeItem struct {
	ID                   int64
	Narrative            string
	Keywords             []string
	Coins                []string
	DefaultSentimentHint mediacontract.Direction
	TTLHours             int
	Confidence           float64
	Enabled              bool
	UpdatedAt            time.Time
}

// Validate 校验叙事映射行。
func (n NarrativeItem) Validate() error {
	if n.Narrative == "" || len(n.Coins) == 0 {
		return ErrMappingNotFound
	}
	if !n.DefaultSentimentHint.Valid() {
		return ErrConfidenceInvalid
	}
	if n.Confidence < 0 || n.Confidence > 1 {
		return ErrConfidenceInvalid
	}
	if n.TTLHours < 1 || n.TTLHours > 24 {
		return ErrMappingNotFound // 叙事衰减 TTL 须在 1–24h（契约 §3）
	}
	return nil
}

// EventMappingQuery 映射表 A 列表查询条件。
type MappingQuery struct {
	Page, PageSize int
	Keyword        string
	EntityType     string
	AssetClass     string
	EnabledOnly    bool
}

// NarrativeQuery 映射表 B 列表查询条件。
type NarrativeQuery struct {
	Page, PageSize int
	Keyword        string
	EnabledOnly    bool
}

// OhlcvBar 行情 K 线（biz 视图，只读）。
type OhlcvBar struct {
	T      time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// QuoteAssetSnapshot 行情快照（含来源与拉取时间，供 UI 展示与校验用）。
type QuoteSnapshot struct {
	Symbol    string
	Provider  string
	Bars      []OhlcvBar
	FetchedAt time.Time
	FromCache bool
}

// ImpactEventQuery 影响卡列表查询条件。
type ImpactQuery struct {
	Page, PageSize  int
	Direction       string
	MinScore        float64
	IncludeExpired  bool
	OccurredAfter   *time.Time
}

// EventQuery 事件卡列表查询条件。
type EventQuery struct {
	Page, PageSize int
	Category       string
	Keyword        string
	OccurredAfter  *time.Time
	OccurredBefore *time.Time
}
