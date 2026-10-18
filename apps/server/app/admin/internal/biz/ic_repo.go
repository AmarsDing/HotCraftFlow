package biz

import (
	"context"
	"time"

	"framework/pkg/mediacontract"
)

// =============================================================================
// ① IC 情报收集 —— 领域视图与仓储端口（docs/systems/01-IC/03 §4、深度评审 B-5）
//
// 纪律：
//  - IntelItemRow 显式嵌入 mediacontract.IntelItem（评审 B-5：仓储层显式映射
//    ent → 契约，避免字段漂移；toContract 映射与测试在 data/ic_intel_repo.go）。
//  - 枚举一律 string（契约 §0.2：不用 proto enum，跨系统魔术值显式化）。
//  - 所有时间 UTC（契约 §0.3）。
// =============================================================================

// ---------- IntelItem ----------

// IntelItemRow 情报条目内部视图 = 契约负载 + 内部管理字段。
type IntelItemRow struct {
	mediacontract.IntelItem

	SourceID    string // 信源外键（契约不含）
	EvidenceID  string // 取证快照外键（ev_ 前缀）
	PublishState string // pending | published | failed（评审 B-25）

	CycleBucket      int64
	DedupKey         string
	FingerprintValid bool
	FpSegments       [4]int16 // SimHash 四段（B-26 pigeonhole 倒排）
	EmbeddingID      string

	PublishedAtSuspect bool // S-16 保留原值并标记
	RawSize            int64
	RawTruncated       bool
}

// IntelItemUpsert 采集落库入参（B-6 幂等键由 usecase 计算后传入）。
type IntelItemUpsert struct {
	Contract    mediacontract.IntelItem
	SourceID    string
	URLKey      string
	CycleBucket int64
	DedupKey    string

	RawSize      int64
	RawTruncated bool

	// PublishedAtSuspect S-16：上游发布时间不可信（未来时间/解析异常），保留原值并标记。
	PublishedAtSuspect bool
}

// IntelNormalizePatch 归一化阶段回写（URL key/SimHash/实体/分类/可加工性）。
type IntelNormalizePatch struct {
	IntelID          string
	URLKey           *string // 短链展开后可变（S-6）
	FingerprintHex   *string
	FingerprintValid bool
	FpSegments       [4]int16
	Entities         []mediacontract.Entity
	Category         string
	WorkabilityScore float64
	WorkabilityReason string
	Title            string // 归一化可能清洗标题
	Text             string
	Lang             string
}

// IntelMetricsPatch 榜单复抓/指标刷新。
type IntelMetricsPatch struct {
	Rank      int32
	RankTotal int32
	Views     int64
	Likes     int64
	Comments  int64
	AsOf      time.Time
}

// IntelListQuery 列表检索（02 §6.2；V1 全文降级 ILIKE，评审 B-19）。
type IntelListQuery struct {
	Page, PageSize int32

	Platform, Region, Category, ClusterID, CollectMethod, Gate string
	Keyword, Entity                                            string
	CollectedFrom, CollectedTo                                 time.Time // 零值 = 不限
}

type IntelClusterListQuery struct {
	Page, PageSize int32
	Region, Status string
}

type IntelItemRepo interface {
	GetByID(ctx context.Context, id string) (*IntelItemRow, error)
	GetByDedupKey(ctx context.Context, dedupKey string) (*IntelItemRow, error)
	// FindRecentByURLKey L1 去重：窗口内同 url_key 命中即合并。
	FindRecentByURLKey(ctx context.Context, urlKey string, window time.Duration) (*IntelItemRow, error)
	// FindSimHashCandidates B-26 pigeonhole：四段任一等值即候选，Penelope 在 Go 侧算。
	FindSimHashCandidates(ctx context.Context, segs [4]int16, window time.Duration, limit int) ([]*IntelItemRow, error)
	// List 检测列表页（含 total）。
	List(ctx context.Context, q *IntelListQuery) ([]*IntelItemRow, int, error)
	ListByClusterID(ctx context.Context, clusterID string, limit int) ([]*IntelItemRow, error)

	// UpsertWithEvidenceTx B-8：条目 + 取证快照同事务落库；
	// dedup_key 冲突走 ON CONFLICT 更新（NFR-IC-03 幂等），返回是否存在旧行。
	UpsertWithEvidenceTx(ctx context.Context, upsert *IntelItemUpsert, ev *EvidenceSnapshotUpsert) (intelID string, created bool, err error)

	// UpdateAfterNormalize 归一化回写（S-6 短链展开可更新 URL/url_key/dedup_key）。
	UpdateAfterNormalize(ctx context.Context, patch *IntelNormalizePatch) error
	// RekeyDedupKey 短链展开后重算 dedup_key。冲突（另一行已占该键）交由 usecase 走 L1 合并。
	UpdateDedupKey(ctx context.Context, intelID, urlKey, dedupKey string) error
	UpdateGate(ctx context.Context, intelID, gate string, reasons []string) error
	UpdateMetrics(ctx context.Context, intelID string, m *IntelMetricsPatch) error
	UpdateEmbedding(ctx context.Context, intelID, embeddingID string) error

	// SupplementText 手贴正文续跑（评审 C-12：UPDATE 既有条目，禁止新建）。
	SupplementText(ctx context.Context, intelID, text string) error

	SetPublishState(ctx context.Context, intelID, state string) error
	// ListPendingPublish B-25：republish 任务每 30s 打捞（≤1h 窗口由 SQL/调用参数控制）。
	ListPendingPublish(ctx context.Context, limit int) ([]*IntelItemRow, error)

	// AssignCluster 簇拆分/并入：批量改指针。
	AssignCluster(ctx context.Context, intelIDs []string, clusterID string) error
	// MoveClusterMembers 合簇时整簇迁移成员；返回迁移行数。
	MoveClusterMembers(ctx context.Context, fromClusterID, toClusterID string) (int, error)

	// DeleteBefore 保留期清扫（NFR-IC-08 intel 365d）。
	DeleteBefore(ctx context.Context, before time.Time, limit int) (int, error)
}

// ---------- 信源（S-12 熔断、S-20 参数下限校验在 service 层） ----------

type IntelSourceRow struct {
	ID         string
	Platform   string
	Region     string
	Kind       string // hotlist|keyword|creator|rss|competitor（S-20 词表）
	Method     string // hotlist|api|rss|crawl|sidecar（合法组合矩阵见 ic_source_usecase.go）
	Config     map[string]interface{}
	ConfigSchemaHint string

	Enabled         bool
	IntervalMinutes int32
	RateLimitPerMin int32
	Priority        int32
	CredentialRef   string
	Weight          float64 // 影响 MEA 评分权重（评审 M-29）

	RobotsExempt       bool // S-8：仅授权场景，必须给原因
	RobotsExemptReason string

	Status      string // active|paused|disabled
	PausedUntil *time.Time
	FailStreak  int32

	LastRunAt, LastSuccessAt *time.Time
	SuccessCount7d, FailCount7d int32
	LastError string

	CreateTime, UpdateTime *time.Time
}

type IntelSourceCreate struct {
	Platform, Region, Kind, Method string
	Config                         map[string]interface{}
	ConfigSchemaHint               string
	Enabled                        bool
	IntervalMinutes                int32
	RateLimitPerMin                int32
	Priority                       int32
	CredentialRef                  string
	Weight                         float64
	RobotsExempt                   bool
	RobotsExemptReason             string
}

// IntelSourcePatch PATCH 语义：指针字段 = nil 表示不修改。
type IntelSourcePatch struct {
	Config           *map[string]interface{}
	Enabled          *bool
	IntervalMinutes  *int32
	RateLimitPerMin  *int32
	Priority         *int32
	CredentialRef    *string
	Weight           *float64
	RobotsExempt     *bool
	RobotsExemptReason *string
}

type IntelSourceListQuery struct {
	Page, PageSize int32
	Platform, Kind, Method, Status string
	Enabled *bool
}

type IntelSourceRepo interface {
	Create(ctx context.Context, in *IntelSourceCreate) (*IntelSourceRow, error)
	GetByID(ctx context.Context, id string) (*IntelSourceRow, error)
	Update(ctx context.Context, id string, patch *IntelSourcePatch) (*IntelSourceRow, error)
	Delete(ctx context.Context, id string) (bool, error)
	List(ctx context.Context, q *IntelSourceListQuery) ([]*IntelSourceRow, int, error)
	// ListAllEnabled 调度器全量拉取（不含 disabled）。
	ListAllEnabled(ctx context.Context) ([]*IntelSourceRow, error)
	// SetStatus 启停/熔断（S-12：paused 带 paused_until）。
	SetStatus(ctx context.Context, id, status string, pausedUntil *time.Time) error
	// RecordRunOutcome 运行回执：成功重置 fail_streak；连续失败达阈值置 paused 30m（S-12）。
	RecordRunOutcome(ctx context.Context, id string, success bool, errMsg string, at time.Time, failThreshold int32, cooldown time.Duration) error
	// Recompute7dCounts 7 日滚动计数由健康任务从采集体量表回填（S-14）。
	Recompute7dCounts(ctx context.Context, id string, success, fail int32) error
}

// ---------- 平台凭据账号池（FR-INT-13 轮换，NFR-IC-06 加密存储） ----------

type CredentialAccountRow struct {
	ID            string
	Platform      string
	AccountKey    string
	CredentialRef string // 密文引用，绝不落日志
	Enabled       bool
	QuotaUsed     int64
	QuotaLimit    int64
	FailStreak    int32
	PausedUntil   *time.Time
	LastUsedAt    *time.Time
}

type CredentialAccountRepo interface {
	Create(ctx context.Context, row *CredentialAccountRow) (*CredentialAccountRow, error)
	GetByID(ctx context.Context, id string) (*CredentialAccountRow, error)
	ListByPlatform(ctx context.Context, platform string, onlyEnabled bool) ([]*CredentialAccountRow, error)
	Update(ctx context.Context, row *CredentialAccountRow) error
	// RecordUsage 成功刷新 last_used_at + quota；失败累加 fail_streak，超阈值暂停至 paused_until。
	RecordUsage(ctx context.Context, id string, ok bool, at time.Time, pausedUntil *time.Time) error
}

// ---------- 取证快照（FR-INT-11 不可裁剪；B-17 哈希链） ----------

type EvidenceSnapshotRow struct {
	ID              string
	IntelID         string
	CollectedAt     time.Time
	Method          string
	RequestURL      string
	FinalURL        string
	HTTPStatus      int32
	HTTPHeaders     map[string]string // 脱敏受限集合（日志纪律同）
	RawPath         string
	RawHash         string
	PrevHash        string // B-17：链式哈希 sha256(prev||raw||intel||ns)
	ChainHash       string
	RobotsChecked   bool
	RobotsAllowed   bool
	CrawlDelayMs    int32
	RetainUntil     time.Time // NFR-IC-08：90d
	RawSizeBytes    int64
}

type EvidenceSnapshotUpsert = EvidenceSnapshotRow

type EvidenceRepo interface {
	// GetByID / GetLatestByIntelID 不存在时返回 nil,nil。
	GetByID(ctx context.Context, id string) (*EvidenceSnapshotRow, error)
	GetLatestByIntelID(ctx context.Context, intelID string) (*EvidenceSnapshotRow, error)
	ListByIntelID(ctx context.Context, intelID string) ([]*EvidenceSnapshotRow, error)
	// LatestChainHash B-17：取该条目最近一次 chain_hash 作为下一条 prev_hash。
	LatestChainHash(ctx context.Context, intelID string) (string, error)
	// ListExpired retention 清扫：segment 先删文件再删行。
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*EvidenceSnapshotRow, error)
	DeleteIDs(ctx context.Context, ids []string) (int, error)
}

// ---------- 簇（S-2 空闲关闭、S-3 合并同事务、S-4 质心冻结） ----------

type IntelClusterRow struct {
	ID               string
	Label            string
	MemberCount      int32
	PlatformCount    int32
	RepresentativeID string
	Region           string
	Category         string // L4 理解层回填，可空
	FirstSeenAt      time.Time
	LastSeenAt       time.Time
	MergedBy         string // url|fingerprint|embedding|manual
	CentroidRef      string // Redis key；成员 >50 冻结（S-4）
	Status           string // open|closed
	ClosedAt         *time.Time
}

type IntelClusterRepo interface {
	Create(ctx context.Context, row *IntelClusterRow) (*IntelClusterRow, error)
	GetByID(ctx context.Context, id string) (*IntelClusterRow, error)
	List(ctx context.Context, q *IntelClusterListQuery) ([]*IntelClusterRow, int, error)

	// TouchSeen 新成员加入：first/last seen 刷新。
	TouchSeen(ctx context.Context, id string, at time.Time) error
	// RecomputeStats S-3：member_count / platform_count 以 ic_intel_items 实际为准。
	RecomputeStats(ctx context.Context, id string) error
	SetRepresentative(ctx context.Context, id, intelID string) error
	SetCategory(ctx context.Context, id, category string) error
	SetCentroidRef(ctx context.Context, id, ref string) error

	// MergeTx S-3：锁定两簇（按 ID 排序避免死锁）→ 迁移成员 → 事件落库 → from 关闭。
	MergeTx(ctx context.Context, fromID, toID, reason, operator string) error
	// SplitTx 劈出新簇（S-23）：新簇并入指定成员并重算两簇统计。
	SplitTx(ctx context.Context, fromID string, intelIDs []string, operator, label string) (newClusterID string, err error)

	// ListIdleOpen S-2 清扫：last_seen_at 早于阈值的 open 簇。
	ListIdleOpen(ctx context.Context, idleBefore time.Time, limit int) ([]*IntelClusterRow, error)
	Close(ctx context.Context, id string, at time.Time) error
}

// ---------- 去重抽样（M-27 黄金集回放评审必读） ----------

type DedupSampleRow struct {
	IntelID   string
	ClusterID string
	Action    string  // auto_merge|manual_split|manual_merge
	Level     string  // L1|L2|L3
	SimScore  float64
	Operator  string
}

type DedupSampleRepo interface {
	Create(ctx context.Context, row *DedupSampleRow) error
	ListByCluster(ctx context.Context, clusterID string, limit int) ([]*DedupSampleRow, error)
}

// ---------- 合规规则（FR-INT-12 无旁路；B-16 fail-closed；S-9 审计） ----------

type ComplianceRuleRow struct {
	ID             string
	RuleType       string // sensitive_category|rights_holder_blacklist|authorization|robots|platform_tos
	Pattern        string
	Action         string // pass|review|block
	Priority       int32
	Enabled        bool
	EffectiveFrom  *time.Time
	EffectiveUntil *time.Time
	HitCount       int64
	Scope          string // global|platform
	Platform       string
	Remark         string
	CreateBy, UpdateBy uint32
	CreateTime, UpdateTime *time.Time
}

type ComplianceRuleListQuery struct {
	Page, PageSize int32
	RuleType, Action, Scope string
	Enabled *bool
}

type ComplianceRuleRepo interface {
	Create(ctx context.Context, row *ComplianceRuleRow) (*ComplianceRuleRow, error)
	GetByID(ctx context.Context, id string) (*ComplianceRuleRow, error)
	Update(ctx context.Context, row *ComplianceRuleRow) (*ComplianceRuleRow, error)
	Delete(ctx context.Context, id string) (bool, error)
	List(ctx context.Context, q *ComplianceRuleListQuery) ([]*ComplianceRuleRow, int, error)
	// ListEffective 闸门取规则：enabled 且在生效窗口内，按 priority 降序（严格者优先胜出在 biz 判定）。
	ListEffective(ctx context.Context, now time.Time) ([]*ComplianceRuleRow, error)
	// IncrementHitCount 命中审计（Explainable RuleIDs 支撑）。
	IncrementHitCount(ctx context.Context, ids []string) error
}

// ---------- 授权台账（B-14 #12：authorization 规则的匹配源） ----------

type AuthorizationRecordRow struct {
	ID          string
	Subject     string // author|domain|asset
	SubjectKey  string
	Status      string // granted|unknown|denied
	Scope       string
	EvidenceRef string
	ValidUntil  *time.Time
	// S-9 审计：变更人（CreateBy/UpdateBy mixin）
	CreateBy, UpdateBy uint32
}

type AuthorizationRecordRepo interface {
	// Upsert 按 (subject, subject_key) 幂等写。
	Upsert(ctx context.Context, row *AuthorizationRecordRow) (*AuthorizationRecordRow, error)
	// Get 闸门查询：过期视为 unknown（fail-closed，B-16）。
	Get(ctx context.Context, subject, subjectKey string, now time.Time) (*AuthorizationRecordRow, error)
	List(ctx context.Context, subject string, page, pageSize int32) ([]*AuthorizationRecordRow, int, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// ---------- 运行记录与死信（S-13 / S-14 健康度来源） ----------

type CollectRunRow struct {
	ID          string
	SourceID    string
	Trigger     string // cron|manual|retry
	StartedAt   time.Time
	FinishedAt  *time.Time
	Fetched     int32
	Normalized  int32
	Merged      int32
	Blocked     int32
	Skipped     int32
	Status      string // running|succeeded|failed
	ErrorCode   string
}

type CollectRunFinish struct {
	Fetched, Normalized, Merged, Blocked, Skipped int32
	Status    string
	ErrorCode string
}

type CollectRunRepo interface {
	Create(ctx context.Context, sourceID, trigger string, startedAt time.Time) (string, error)
	Finish(ctx context.Context, id string, fin *CollectRunFinish) error
	List(ctx context.Context, sourceID string, page, pageSize int32) ([]*CollectRunRow, int, error)
	// CountOutcomes S-14：健康任务按窗口回填 success_count_7d / fail_count_7d。
	CountOutcomes(ctx context.Context, sourceID string, since time.Time) (success, fail int32, err error)
}

type DeadLetterRow struct {
	ID            string
	TaskType      string // collect|normalize|publish|replay
	Payload       map[string]interface{}
	ErrorCode     string
	ErrorMsg      string
	Attempt       int32
	FirstFailedAt time.Time
	LastFailedAt  time.Time
	Status        string // pending|replayed|discarded
	ReplayedBy    string
	ReplayedAt    *time.Time
}

type DeadLetterRepo interface {
	Create(ctx context.Context, row *DeadLetterRow) (string, error)
	GetByID(ctx context.Context, id string) (*DeadLetterRow, error)
	List(ctx context.Context, status string, page, pageSize int32) ([]*DeadLetterRow, int, error)
	MarkReplayed(ctx context.Context, id, operator string, at time.Time) error
	MarkDiscarded(ctx context.Context, id string) error
}
