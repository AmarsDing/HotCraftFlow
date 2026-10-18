package mediacontract

import "time"

// ImpactDisclaimer 是 ImpactCard 的强制免责声明（契约 §3 disclaimer 强制非空）。
const ImpactDisclaimer = "研究/监测辅助，非投资建议；模型输出须人工复核。"

// ImpactCard 是 ③ MIA 的输出契约（契约 §3）。默认不对外发布（PRD §12 Q2）。
type ImpactCard struct {
	APIVersion string         `json:"api_version"`
	ImpactID   string         `json:"impact_id"` // im_ 前缀
	TopicID    string         `json:"topic_id"`
	Event      ImpactEvent    `json:"event"`
	Sentiment  Sentiment      `json:"sentiment"`
	Impact     Impact         `json:"impact"`
	Affected   []AffectedItem `json:"affected"`
	Disclaimer string         `json:"disclaimer"` // 强制非空，空卡不允许落库
	Verified   Verified       `json:"verified"`
}

// ImpactEvent 事件摘要。
type ImpactEvent struct {
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	Sources    []string  `json:"sources"`     // IntelItem id 列表
	OccurredAt time.Time `json:"occurred_at"` // UTC
}

// Sentiment 情感结论。engine 必须标注，不同引擎分数不可混比。
type Sentiment struct {
	Label  SentimentLabel  `json:"label"`
	Score  float64         `json:"score"` // -1 ~ 1；unknown 时为 0（不猜）
	Engine SentimentEngine `json:"engine"`
	Lang   string          `json:"lang"`
}

// Impact 影响面块。score 含四因子明细。
type Impact struct {
	Score     float64       `json:"score"`      // 0–100
	TTLHours  int           `json:"ttl_hours"`  // 6–24 可配；过期卡不得占据排行榜
	Factors   ImpactFactors `json:"factors"`
}

// ImpactFactors 影响面四因子（评审要求必须随 score 一起输出）。
type ImpactFactors struct {
	Sentiment    float64 `json:"sentiment"`
	SourceWeight float64 `json:"source_weight"`
	Relevance    float64 `json:"relevance"`
	Decay        float64 `json:"decay"`
}

// AffectedItem 受影响标的。每条结论必须有 path；confidence 必显（传导是假设不是因果）。
type AffectedItem struct {
	Symbol       string     `json:"symbol"`       // SPY / ^GSPC / EURUSD / BTC/USDT
	AssetClass   AssetClass `json:"asset_class"`
	Direction    Direction  `json:"direction"`
	Confidence   float64    `json:"confidence"` // 传导规则 ≤0.6
	Path         []string   `json:"path"`       // 影响路径文本链，无 path 视为违约
	RuleID       string     `json:"rule_id"`
	IsTransmission bool     `json:"is_transmission,omitempty"` // 是否为跨资产传导项
}

// Verified 事后校验回填。hit 为 null 表示未校验，不得默认解释为「不准」。
type Verified struct {
	CheckedAt  *time.Time `json:"checked_at"`
	ActualMove *float64   `json:"actual_move"`
	Hit        *bool      `json:"hit"`
}

// Validate 校验 ImpactCard。
func (x *ImpactCard) Validate() error {
	if err := ValidateIDFormat(x.ImpactID, PrefixImpactCard); err != nil {
		return err
	}
	if x.Disclaimer == "" {
		return errField("disclaimer", "强制非空（契约 §3）")
	}
	if !x.Sentiment.Label.Valid() {
		return errField("sentiment.label", "非法取值")
	}
	if !x.Sentiment.Engine.Valid() {
		return errField("sentiment.engine", "非法取值")
	}
	if x.Sentiment.Score < -1 || x.Sentiment.Score > 1 {
		return errField("sentiment.score", "须在 [-1,1]")
	}
	if x.Impact.TTLHours < 6 || x.Impact.TTLHours > 24 {
		return errField("impact.ttl_hours", "须在 6–24")
	}
	for i, a := range x.Affected {
		if !a.AssetClass.Valid() {
			return errField("affected["+itoa(i)+"].asset_class", "非法取值")
		}
		if !a.Direction.Valid() {
			return errField("affected["+itoa(i)+"].direction", "非法取值")
		}
		if len(a.Path) == 0 {
			return errField("affected["+itoa(i)+"].path", "每条结论必须有 path")
		}
		if a.IsTransmission && a.Confidence > 0.6 {
			return errField("affected["+itoa(i)+"].confidence", "跨资产传导置信度必须 ≤0.6（假设不是因果）")
		}
	}
	return nil
}

// EventCard 是 ③ MIA 的事件卡（契约 §0.3 ec_ 前缀）。
type EventCard struct {
	EventID         string    `json:"event_id"` // ec_ 前缀
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	SourceIntelIDs  []string  `json:"source_intel_ids"`
	SourceCount     int32     `json:"source_count"` // 按不同域名去重计数
	FirstSeenAt     time.Time `json:"first_seen_at"`
	LastSeenAt      time.Time `json:"last_seen_at"`
}

// TransmissionRule 是 ③ MIA 的跨资产传导规则（confidence 强制 ≤0.6）。
type TransmissionRule struct {
	RuleID      string    `json:"rule_id"` // rule_ 前缀
	Trigger     TriggerSpec `json:"trigger"`
	ToAsset     string    `json:"to_asset"`
	ToDirection Direction `json:"to_direction"`
	Confidence  float64   `json:"confidence"` // 必填且 ≤0.6
	Note        string    `json:"note"`       // 必填
}

// TriggerSpec 传导触发条件。
type TriggerSpec struct {
	Asset      string    `json:"asset"`
	Direction  Direction `json:"direction"`
	MinMovePct float64   `json:"min_move_pct"`
}

// Validate 校验传导规则（confidence ≤0.6 是诚实底线，标 0.9 是自欺欺人）。
func (r *TransmissionRule) Validate() error {
	if !r.ToDirection.Valid() {
		return errField("to_direction", "非法取值")
	}
	if r.Confidence <= 0 || r.Confidence > 0.6 {
		return errField("confidence", "传导规则置信度须在 (0, 0.6]")
	}
	if r.Note == "" {
		return errField("note", "必填")
	}
	return nil
}
