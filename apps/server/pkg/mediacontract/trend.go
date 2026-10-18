package mediacontract

import "time"

// TrendSeries 是 ② TT 的核心契约（契约 §2）。
// 消费者：② TT 看板、④ MME（选题）、③ MIA（热度佐证）。
type TrendSeries struct {
	APIVersion    string        `json:"api_version"`
	TopicID       string        `json:"topic_id"` // tp_ 前缀
	Label         string        `json:"label"`
	Category      Category      `json:"category"`
	Stage         Stage         `json:"stage"`
	StageEvidence StageEvidence `json:"stage_evidence"`
	Heat          Heat          `json:"heat"`
	Series        []SeriesPoint `json:"series"` // ≤200 点，见契约 §2 截取规则
	Burst         Burst         `json:"burst"`
	Origin        Origin        `json:"origin"`
	Diagnostics   *Diagnostics  `json:"diagnostics,omitempty"` // 补丁级演进：数据不足标记
}

// StageEvidence 生命周期判据数值（缺数值的阶段标签不可信）。
type StageEvidence struct {
	Velocity       float64 `json:"velocity"`
	Accel          float64 `json:"accel"`
	DurationPoints int32   `json:"duration_points"` // 当前阶段已持续采样点数（点数，非时长）
}

// Heat 热度块。score 是相对可比分数 0–100，不是绝对热度。
// 评审 B-3/C-5：四因子取值（rank_pct/eng_pct/freshness/breadth）必须随 heat 一起输出，
// 否则 NFR-TT-05（任何分数可追溯到因子数值）违约。
type Heat struct {
	Score     float64 `json:"score"`     // 0–100，相对可比
	RankPct   float64 `json:"rank_pct"`  // 榜内相对位置因子取值
	EngPct    float64 `json:"eng_pct"`   // 互动分位因子取值（评审补登）
	EngRate   float64 `json:"eng_rate"`  // 互动率（仅展示，不进公式）
	Freshness float64 `json:"freshness"` // 新鲜度因子取值（评审补登）
	Velocity  float64 `json:"velocity"`  // 平滑后增速 v_ewma
	Accel     float64 `json:"accel"`     // 加速度
	Breadth   Breadth `json:"breadth"`
	Factors   HeatFactors `json:"factors"` // 权重快照（可解释性强制）
}

// Breadth 跨平台扩散度。
type Breadth struct {
	PlatformCount int32    `json:"platform_count"`
	Platforms     []string `json:"platforms"` // 滑动 24h 窗口内的 distinct 平台列表
}

// HeatFactors 热度权重快照。同时保留原始配置权重与本次实际生效权重（评审 A-6）。
// 缺失即视为违约（可解释性靠这个字段）。
type HeatFactors struct {
	WRank    float64 `json:"w_rank"`    // 原始配置权重
	WEng     float64 `json:"w_eng"`
	WFresh   float64 `json:"w_fresh"`
	WBreadth float64 `json:"w_breadth"`
	WRankEff    float64 `json:"w_rank_eff"`    // 因子缺失重分配后的生效权重
	WEngEff     float64 `json:"w_eng_eff"`
	WFreshEff   float64 `json:"w_fresh_eff"`
	WBreadthEff float64 `json:"w_breadth_eff"`
	Missing     []string `json:"missing,omitempty"` // 本次缺失的因子名
}

// SeriesPoint 时序点。t 一律 UTC 带 Z。
type SeriesPoint struct {
	T        string  `json:"t"` // RFC3339 UTC（时间桶对齐后的 t_bucket）
	Platform string  `json:"platform"`
	Rank     int32   `json:"rank"`
	Heat     float64 `json:"heat"`
}

// Burst 突发判定。methods_hit 至少 2 个且来自 ≥2 通道才为 true（评审 A-12）。
type Burst struct {
	IsBurst    bool               `json:"is_burst"`
	MethodsHit []BurstMethod      `json:"methods_hit"`
	Evidence   map[string]float64 `json:"evidence"` // 每个命中方法的具体数值
}

// Origin 溯源。
type Origin struct {
	FirstSeenAt   string `json:"first_seen_at"` // RFC3339 UTC
	FirstPlatform string `json:"first_platform"`
	FirstURL      string `json:"first_url"`
}

// Diagnostics 诊断信息（补丁级演进，api_version 不变）。
// 用于区分「真的 latent」与「数据不足的 latent」。
type Diagnostics struct {
	InsufficientPoints bool  `json:"insufficient_points"`
	NPoints            int32 `json:"n_points"`
}

// Validate 校验 TrendSeries。
func (x *TrendSeries) Validate() error {
	if err := ValidateIDFormat(x.TopicID, PrefixTopic); err != nil {
		return err
	}
	if !x.Stage.Valid() {
		return errField("stage", "非法取值")
	}
	if !x.Category.Valid() {
		return errField("category", "非法取值")
	}
	if len(x.Series) > 200 {
		return errField("series", "NATS 消息不得超过 200 点（契约 §2）")
	}
	if x.Burst.IsBurst && len(x.Burst.MethodsHit) < 2 {
		return errField("burst.methods_hit", "is_burst=true 需 ≥2 个命中方法")
	}
	for _, m := range x.Burst.MethodsHit {
		if !m.Valid() {
			return errField("burst.methods_hit", "非法方法 "+string(m))
		}
	}
	return nil
}

// TopicCandidate 是 ② TT → ④ MME / ③ MIA 的交接（契约 §6）。
type TopicCandidate struct {
	APIVersion      string         `json:"api_version"`
	CandidateID     string         `json:"candidate_id"` // tc_ 前缀
	TopicID         string         `json:"topic_id"`
	Route           CandidateRoute `json:"route"`
	Score           CandidateScore `json:"score"`
	SuggestedAngles []string       `json:"suggested_angles"`
	MaterialRefs    []string       `json:"material_refs,omitempty"`
	State           CandidateState `json:"state"`
	RejectReason    *string        `json:"reject_reason"`
}

// CandidateScore 选题评分。total 仅用于排序，不作为自动采纳依据。
type CandidateScore struct {
	Hot         float64 `json:"hot"`
	Freshness   float64 `json:"freshness"`
	Workability float64 `json:"workability"`
	Compliance  float64 `json:"compliance"`
	Total       float64 `json:"total"`
}

// Validate 校验 TopicCandidate。
func (x *TopicCandidate) Validate() error {
	if err := ValidateIDFormat(x.CandidateID, PrefixTopicCandidate); err != nil {
		return err
	}
	if !x.Route.Valid() {
		return errField("route", "非法取值")
	}
	if !x.State.Valid() {
		return errField("state", "非法取值")
	}
	return nil
}

// Alert 是突发告警（契约 §7）。
type Alert struct {
	APIVersion  string             `json:"api_version"`
	AlertID     string             `json:"alert_id"` // al_ 前缀
	TopicID     string             `json:"topic_id"`
	TriggeredAt time.Time          `json:"triggered_at"` // UTC
	MethodsHit  []BurstMethod      `json:"methods_hit"`
	Evidence    map[string]float64 `json:"evidence"` // 无数值的告警不允许发出
	Channels    []string           `json:"channels"`
	State       AlertState         `json:"state"`
}

// Validate 校验 Alert。
func (x *Alert) Validate() error {
	if err := ValidateIDFormat(x.AlertID, PrefixAlert); err != nil {
		return err
	}
	if !x.State.Valid() {
		return errField("state", "非法取值")
	}
	if len(x.MethodsHit) < 2 {
		return errField("methods_hit", "≥2 个方法命中才产生 Alert（契约 §7）")
	}
	for _, m := range x.MethodsHit {
		if !m.Valid() {
			return errField("methods_hit", "非法方法")
		}
		if _, ok := x.Evidence[string(m)]; !ok {
			return errField("evidence", "命中方法 "+string(m)+" 缺具体数值")
		}
	}
	return nil
}
