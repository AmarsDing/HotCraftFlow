package mediacontract

// 本文件集中定义所有跨系统枚举。契约 §0.2 强制：枚举一律小写下划线字符串。
// 每个枚举都提供 Valid() 校验与 All() 全集，供生产者/消费者两侧断言。

// ---------- ① IC ----------

// Category 情报类别，决定下游路由（契约 §1 路由表）。
type Category string

const (
	CategoryAHot       Category = "A_hot"       // 正在上升的话题/事件 → TT
	CategoryBMaterial  Category = "B_material"  // 可直接加工的素材 → MME
	CategoryCMarket    Category = "C_market"    // 可能影响市场 → MIA
	CategoryDCompliance Category = "D_compliance" // 合规信号 → 人工 + 告警
)

func (c Category) Valid() bool {
	switch c {
	case CategoryAHot, CategoryBMaterial, CategoryCMarket, CategoryDCompliance:
		return true
	}
	return false
}

// CollectMethod 五路采集器（契约 §1 source.collect_method）。
type CollectMethod string

const (
	CollectHotlist CollectMethod = "hotlist" // 热榜聚合
	CollectAPI     CollectMethod = "api"     // 官方 API
	CollectRSS     CollectMethod = "rss"     // RSS 订阅
	CollectCrawl   CollectMethod = "crawl"   // 爬取
	CollectSidecar CollectMethod = "sidecar" // 录播侧车
)

func (m CollectMethod) Valid() bool {
	switch m {
	case CollectHotlist, CollectAPI, CollectRSS, CollectCrawl, CollectSidecar:
		return true
	}
	return false
}

// Gate 合规闸门结论（契约 §1 compliance.gate）。
type Gate string

const (
	GatePass   Gate = "pass"   // 放行
	GateReview Gate = "review" // 转人工
	GateBlock  Gate = "block"  // 阻断（不得进入自动流程）
)

func (g Gate) Valid() bool { return g == GatePass || g == GateReview || g == GateBlock }

// EntityType 实体六类（契约 §1 entities[].type）。
type EntityType string

const (
	EntityPerson EntityType = "person"
	EntityOrg    EntityType = "org"
	EntityPolicy EntityType = "policy"
	EntityTech   EntityType = "tech"
	EntityPlace  EntityType = "place"
	EntityTopic  EntityType = "topic"
)

func (t EntityType) Valid() bool {
	switch t {
	case EntityPerson, EntityOrg, EntityPolicy, EntityTech, EntityPlace, EntityTopic:
		return true
	}
	return false
}

// Region 区域（契约 §1 平台标识枚举表）。
type Region string

const (
	RegionCN     Region = "cn"     // 国内
	RegionIntl   Region = "intl"   // 国外单一市场
	RegionGlobal Region = "global" // 跨市场/全球话题
)

func (r Region) Valid() bool { return r == RegionCN || r == RegionIntl || r == RegionGlobal }

// ---------- ② TT ----------

// Stage 话题生命周期五阶段 + dormant（评审 B-6 新增的兜底态）。
type Stage string

const (
	StageLatent   Stage = "latent"    // 潜伏期
	StageRising   Stage = "rising"    // 上升期
	StagePeak     Stage = "peak"      // 峰值期
	StageDecaying Stage = "decaying"  // 衰退期
	StageLongTail Stage = "long_tail" // 长尾期
	StageDormant  Stage = "dormant"   // 休眠期（评审新增：48h 无更新）
)

func (s Stage) Valid() bool {
	switch s {
	case StageLatent, StageRising, StagePeak, StageDecaying, StageLongTail, StageDormant:
		return true
	}
	return false
}

// BurstMethod 突发检测方法（契约 §2 burst.methods_hit / §7 methods_hit）。
type BurstMethod string

const (
	BurstZScore    BurstMethod = "zscore"     // Z-score 残差（heat 通道）
	BurstVelocity  BurstMethod = "velocity"   // 双阈值增速（heat 通道）
	BurstRankJump  BurstMethod = "rank_jump"  // 榜单跃迁（rank 通道）
	BurstEngSpike  BurstMethod = "eng_spike"  // 互动突增（engagement 通道，评审新增）
	BurstFirstXPlat BurstMethod = "first_cross_platform" // 首现跨平台（rank 通道，评审新增）
)

func (m BurstMethod) Valid() bool {
	switch m {
	case BurstZScore, BurstVelocity, BurstRankJump, BurstEngSpike, BurstFirstXPlat:
		return true
	}
	return false
}

// Channel 返回该检测方法所属的数据通道（评审 A-12 通道化投票的核心）。
// 投票要求 ≥2 票且来自 ≥2 个不同通道。
func (m BurstMethod) Channel() BurstChannel {
	switch m {
	case BurstZScore, BurstVelocity:
		return ChannelHeat
	case BurstRankJump, BurstFirstXPlat:
		return ChannelRank
	case BurstEngSpike:
		return ChannelEngagement
	}
	return ChannelUnknown
}

// BurstChannel 突发检测的数据通道（用于保证投票独立性）。
type BurstChannel string

const (
	ChannelUnknown    BurstChannel = "unknown"
	ChannelHeat       BurstChannel = "heat"       // heat 时序
	ChannelRank       BurstChannel = "rank"       // 榜单名次
	ChannelEngagement BurstChannel = "engagement" // 互动量
)

// CandidateRoute 选题路由（契约 §6 TopicCandidate.route）。
type CandidateRoute string

const (
	RouteMME  CandidateRoute = "mme"  // 走 MME 加工
	RouteMIA  CandidateRoute = "mia"  // 走 MIA 分析
	RouteBoth CandidateRoute = "both" // 两者
)

func (r CandidateRoute) Valid() bool { return r == RouteMME || r == RouteMIA || r == RouteBoth }

// CandidateState 选题状态（契约 §6）。
type CandidateState string

const (
	CandidatePending  CandidateState = "pending"
	CandidateAdopted  CandidateState = "adopted"
	CandidateRejected CandidateState = "rejected"
	CandidateSnoozed  CandidateState = "snoozed"
)

func (s CandidateState) Valid() bool {
	switch s {
	case CandidatePending, CandidateAdopted, CandidateRejected, CandidateSnoozed:
		return true
	}
	return false
}

// AlertState 告警状态（契约 §7）。
type AlertState string

const (
	AlertNew   AlertState = "new"
	AlertAck   AlertState = "ack"
	AlertMuted AlertState = "muted"
)

func (s AlertState) Valid() bool { return s == AlertNew || s == AlertAck || s == AlertMuted }

// ---------- ③ MIA ----------

// SentimentLabel 情感标签（契约 §3 sentiment.label）。
type SentimentLabel string

const (
	SentimentPositive SentimentLabel = "positive"
	SentimentNeutral  SentimentLabel = "neutral"
	SentimentNegative SentimentLabel = "negative"
	SentimentUnknown  SentimentLabel = "unknown" // 引擎全失效时，禁止猜测
)

func (l SentimentLabel) Valid() bool {
	switch l {
	case SentimentPositive, SentimentNeutral, SentimentNegative, SentimentUnknown:
		return true
	}
	return false
}

// SentimentEngine 情感引擎（契约 §3 sentiment.engine）。不同引擎分数不可混比。
type SentimentEngine string

const (
	EngineFinBERT SentimentEngine = "finbert"
	EngineLLM     SentimentEngine = "llm"
	EngineNone    SentimentEngine = "none" // 不可用兜底
)

func (e SentimentEngine) Valid() bool {
	return e == EngineFinBERT || e == EngineLLM || e == EngineNone
}

// AssetClass 资产类别（契约 §3 affected[].asset_class）。
type AssetClass string

const (
	AssetEquity    AssetClass = "equity"
	AssetIndex     AssetClass = "index"
	AssetFX        AssetClass = "fx"
	AssetCommodity AssetClass = "commodity"
	AssetCrypto    AssetClass = "crypto"
)

func (a AssetClass) Valid() bool {
	switch a {
	case AssetEquity, AssetIndex, AssetFX, AssetCommodity, AssetCrypto:
		return true
	}
	return false
}

// Direction 方向（契约 §3 affected[].direction）。
type Direction string

const (
	DirectionBullish Direction = "bullish"
	DirectionBearish Direction = "bearish"
	DirectionNeutral Direction = "neutral"
)

func (d Direction) Valid() bool {
	return d == DirectionBullish || d == DirectionBearish || d == DirectionNeutral
}

// ---------- ④ MME ----------

// RenderState 变体渲染状态。
type RenderState string

const (
	RenderPending   RenderState = "pending"
	RenderRunning   RenderState = "running"
	RenderSucceeded RenderState = "succeeded"
	RenderFailed    RenderState = "failed"
)

func (s RenderState) Valid() bool {
	switch s {
	case RenderPending, RenderRunning, RenderSucceeded, RenderFailed:
		return true
	}
	return false
}

// CaptionStyle 字幕风格（契约 §4 captions.style）。
type CaptionStyle string

const (
	CaptionSimple  CaptionStyle = "simple"
	CaptionDynamic CaptionStyle = "dynamic"
)

func (s CaptionStyle) Valid() bool { return s == CaptionSimple || s == CaptionDynamic }

// ---------- ⑤ MAP ----------

// ApprovalState 审批状态机（契约 §5 approval.state）。
type ApprovalState string

const (
	ApprovalDraft    ApprovalState = "draft"
	ApprovalApproved ApprovalState = "approved"
	ApprovalRejected ApprovalState = "rejected"
	ApprovalPublished ApprovalState = "published" // 发布完成后流转
)

func (s ApprovalState) Valid() bool {
	switch s {
	case ApprovalDraft, ApprovalApproved, ApprovalRejected, ApprovalPublished:
		return true
	}
	return false
}

// Visibility 可见性（契约 §5 visibility.default，恒为 private 起步）。
type Visibility string

const (
	VisibilityPrivate   Visibility = "private"
	VisibilityUnlisted  Visibility = "unlisted"
	VisibilityPublic    Visibility = "public"
)

func (v Visibility) Valid() bool {
	return v == VisibilityPrivate || v == VisibilityUnlisted || v == VisibilityPublic
}

// ActorType 操作者类型（用于禁自批与自动化隔离）。
type ActorType string

const (
	ActorHuman      ActorType = "human"
	ActorAutomation ActorType = "automation"
)

func (a ActorType) Valid() bool { return a == ActorHuman || a == ActorAutomation }
