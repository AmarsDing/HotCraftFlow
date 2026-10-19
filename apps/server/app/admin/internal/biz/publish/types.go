// Package publish 实现 ⑤ MAP 媒体自动化发布矩阵的业务层（biz）。
// 对应 docs/systems/05-MAP/{02-概要设计,03-详细设计}.md。
//
// 包名用 publish（map 是 Go 关键字；目录名 biz/publish 与职责「发布」吻合）。
//
// 纪律：
//   - 本包不 import data / ent：所有持久化走 ports.go 定义的端口，由 data 层实现并 wire.Bind。
//   - 🔴 三条铁律（无 approve 不得 publish / 禁自批 / 默认私密）由 algo/publisher 纯算法承载，
//     本包负责收集熔断/配额结论喂给算法，并写审计流水。
//   - biz 层类型（UnitRow/AccountRow/...）为业务视图，与 ent 实体解耦，便于 fake 测试。
package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"framework/pkg/mediacontract"

	algopublisher "framework/app/admin/internal/algo/publisher"
)

// Actor 操作者（复用审批门算法定义，ID 为业务标识，Type ∈ human/automation）。
type Actor = algopublisher.Actor

// NewHumanActor 构造人类操作者。
func NewHumanActor(id string) Actor { return Actor{ID: id, Type: mediacontract.ActorHuman} }

// NewAutomationActor 构造自动化身份（🔴 不得审批/发布；仅供系统内部流程显式标注）。
func NewAutomationActor(id string) Actor { return Actor{ID: id, Type: mediacontract.ActorAutomation} }

// ============================================================================
// 错误（biz 层对外语义错误；审批门五校验的算法错误在 algo/publisher 定义，直接透出，
// 由 service 层统一映射到 MapErrorReason）。
// ============================================================================

var (
	// ErrChannelNotFound 渠道（platform:slot）不存在或已停用。
	ErrChannelNotFound = errors.New("MAP_CHANNEL_NOT_FOUND")
	// ErrAdapterNotFound 平台适配器未注册（ADR-MAP-05：明确报错，不静默降级）。
	ErrAdapterNotFound = errors.New("MAP_ADAPTER_NOT_FOUND")
	// ErrCapabilityUnsupported 平台不支持所请求能力。
	ErrCapabilityUnsupported = errors.New("MAP_CAPABILITY_UNSUPPORTED")
	// ErrNotFound 资源不存在（unit/account/receipt/channel）。
	ErrNotFound = errors.New("MAP_NOT_FOUND")
	// ErrBadRequest 请求参数错误。
	ErrBadRequest = errors.New("MAP_BAD_REQUEST")
	// ErrRejectReasonRequired 驳回必须填原因（概要设计 §4.2）。
	ErrRejectReasonRequired = errors.New("MAP_REJECT_REASON_REQUIRED")
	// ErrPublicConfirmRequired 非 private 发布必须显式 confirm_public（ADR-MAP-03 二次确认）。
	ErrPublicConfirmRequired = errors.New("MAP_PUBLIC_CONFIRM_REQUIRED")
	// ErrAlreadyPublishing 幂等锁冲突：同一 unit 正在发布。
	ErrAlreadyPublishing = errors.New("MAP_ALREADY_PUBLISHING")
	// ErrCredentialInvalid 凭据失效（不清凭据，引导重新授权）。
	ErrCredentialInvalid = errors.New("MAP_CREDENTIAL_INVALID")
	// ErrReceiptConflict 回执唯一索引冲突（幂等 DB 兜底的正常信号，非用户错误）。
	ErrReceiptConflict = errors.New("MAP_RECEIPT_CONFLICT")
	// ErrInternal 内部错误。
	ErrInternal = errors.New("MAP_INTERNAL")
	// ErrUpstream 平台返回错误（消息已脱敏转译，不含凭据/堆栈原文）。
	ErrUpstream = errors.New("MAP_UPSTREAM_ERROR")
)

// ============================================================================
// Target：DispatchUnit.targets 的 platform:slot 元素解析。
// ============================================================================

// Target 一个发布目标（platform_key:slot，如 youtube:main）。
type Target struct {
	Platform string
	Slot     string
}

// ParseTarget 解析 "platform:slot"。槽位缺省视为 "main"？——保守起见必须显式，
// 避免「以为发到 main 实际发错槽位」。格式非法返回 ErrBadRequest。
func ParseTarget(s string) (Target, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Target{}, fmt.Errorf("%w: 非法 target %q，期望 platform:slot", ErrBadRequest, s)
	}
	return Target{Platform: parts[0], Slot: parts[1]}, nil
}

func (t Target) String() string { return t.Platform + ":" + t.Slot }

// ============================================================================
// 业务视图行（biz 拥有的类型，与 ent 解耦；data 层负责映射）。
// ============================================================================

// ChannelRow 平台渠道（map_publish_channels 业务视图）。
type ChannelRow struct {
	ID             uint32
	ChannelID      string
	PlatformKey    string
	Slot           string
	Region         string
	Enabled        bool
	Capability     map[string]any
	AdapterVersion *string
	Priority       int
	CreateTime     time.Time
}

// AccountRow 账号（map_publish_accounts 业务视图；🔴 不含凭据明文/密文）。
type AccountRow struct {
	ID                  uint32
	AccountID           string
	ChannelID           string
	DisplayName         string
	HasCredential       bool
	CredentialRef       *string // 仅引用（enc:v1:...），发布时经 vault 解析，绝不进日志
	CredentialType      string  // oauth / cookie / api_key，可空串
	Enabled             bool
	Health              string // healthy / degraded / paused / credential_invalid
	ConsecutiveFailures int
	PausedUntil         *time.Time
	Published24h        int
	LastPublishedAt     *time.Time
	LastError           *string // 已脱敏
	CreateTime          time.Time

	// 冗余渠道字段（查询组装时由 service/usecase 注入）
	PlatformKey string
	Slot        string
}

// Paused 当前是否处于熔断暂停（未过期）。
func (a *AccountRow) Paused(now time.Time) bool {
	return a.Health == AccountHealthPaused && a.PausedUntil != nil && a.PausedUntil.After(now)
}

// 账号健康状态枚举（与 ent NamedValues 一致）。
const (
	AccountHealthHealthy           = "healthy"
	AccountHealthDegraded          = "degraded"
	AccountHealthPaused            = "paused"
	AccountHealthCredentialInvalid = "credential_invalid"
)

// UnitRow 分发单元（map_dispatch_units 业务视图）。
type UnitRow struct {
	ID                       uint32
	UnitID                   string
	VariantSetID             *string
	VariantID                *string
	Targets                  []string
	ScheduledFor             *time.Time
	ScheduleMode             string // native / local_worker / none
	VisibilityDefault        string // private / unlisted / public
	VisibilityOverrideReason *string
	ApprovalState            string // draft / approved / rejected
	SubmittedBy              string
	Approver                 *string
	ApprovedAt               *time.Time
	RejectReason             *string
	PublishState             string // pending / publishing / published / failed
	PostID                   *string
	PostURL                  *string
	PublishedAt              *time.Time
	Operator                 *string
	FeedbackRef              *string
	CreateTime               time.Time
}

// ParsedTargets 解析全部 targets；任一非法则整体报错（建单时已校验，此为防御）。
func (u *UnitRow) ParsedTargets() ([]Target, error) {
	out := make([]Target, 0, len(u.Targets))
	for _, s := range u.Targets {
		t, err := ParseTarget(s)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// ToContract 转为跨系统契约形状，供 algo/publisher 审批门纯算法消费。
func ToContractUnit(u *UnitRow) *mediacontract.DispatchUnit {
	cu := &mediacontract.DispatchUnit{
		UnitID:      u.UnitID,
		SubmittedBy: u.SubmittedBy,
		Visibility: mediacontract.VisibilitySpec{
			Default:        mediacontract.Visibility(u.VisibilityDefault),
			OverrideReason: u.VisibilityOverrideReason,
		},
		Approval: mediacontract.Approval{
			State:    mediacontract.ApprovalState(u.ApprovalState),
			Approver: u.Approver,
			At:       u.ApprovedAt,
		},
		Publish: mediacontract.Publish{
			PostID:   u.PostID,
			URL:      u.PostURL,
			At:       u.PublishedAt,
			Operator: u.Operator,
		},
	}
	return cu
}

// ReceiptRow 发布回执（map_publish_receipts 业务视图）。
type ReceiptRow struct {
	ID                 uint32
	ReceiptID          string
	DispatchUnitID     string
	AccountID          *string
	PlatformKey        string
	PostID             *string
	PostURL            *string
	Visibility         string
	PublishedAt        time.Time
	Operator           string
	RequestPayloadHash *string // 发布请求体 SHA-256（防篡改存证，🔴 哈希输入不含凭据）
}

// RecordEntry 审批/发布审计流水条目（map_approval_records 业务视图）。
type RecordEntry struct {
	ID                 uint32
	DispatchUnitID     string
	Action             string // submit / approve / reject / publish / cancel
	Actor              string
	ActorType          string // human / automation
	FromState          *string
	ToState            *string
	Reason             *string
	VisibilityAtAction *string
	IP                 *string
	UserAgent          *string
	Result             string // allow / deny（deny=越权尝试留痕）
	CreateTime         time.Time
}

// 审计动作常量。
const (
	AuditActionSubmit  = "submit"
	AuditActionApprove = "approve"
	AuditActionReject  = "reject"
	AuditActionPublish = "publish"
	AuditActionCancel  = "cancel"
)

// AuditMeta 由 service 层从 HTTP 请求提取的审计上下文。
type AuditMeta struct {
	IP        string
	UserAgent string
}

// ResolvedTarget target 解析 + 账号指派后的可执行目标。
type ResolvedTarget struct {
	Target  Target
	Channel *ChannelRow
	Account *AccountRow
}

// PerformanceSnapshotRow 表现回流快照（map_performance_snapshots 业务视图）。
type PerformanceSnapshotRow struct {
	ID             uint32
	SnapshotID     string
	ReceiptID      string
	PlatformKey    string
	DispatchUnitID *string // 回指分发单元（发布→表现→反哺选题闭环字段；可空）
	VariantID      *string // 回指 MME 变体（同一闭环；worker 经单元回填，可空）
	Views          int64
	Likes          int64
	Comments       int64
	Shares         int64
	Favorites      int64
	CompletionRate float64
	FetchedAt      time.Time
}

// ============================================================================
// 适配器端口类型（详细设计 §2，PublishRequest 中不得出现凭据字段——凭据经
// Credential 独立传递且仅在发布时解密；详见 §5「凭据不进 DispatchUnit 契约」）。
// ============================================================================

// Capability 平台能力声明（FR-PUB-01；声明式，不支持即明确报错）。
// json tags 与 capability 快照/platforms.yaml 键名一致（capabilityFromSnapshot 同源键）。
type Capability struct {
	Video          bool     `json:"video"`
	ImageText      bool     `json:"image_text"`
	NativeSchedule bool     `json:"native_schedule"`
	DraftState     bool     `json:"draft_state"`
	Analytics      bool     `json:"analytics"`
	MaxDurationSec int      `json:"max_duration_sec"`
	MaxSizeBytes   int64    `json:"max_size_bytes"`
	AspectRatios   []string `json:"aspect_ratios"`
	TitleMaxLen    int      `json:"title_max_len"`
	DescMaxLen     int      `json:"desc_max_len"`
}

// Credential 解密后的凭据（🔴 仅内存，用完必须 Zero() 清零；绝不进日志）。
type Credential struct {
	Type   string // oauth / cookie / api_key
	Secret []byte // 明文（仅函数作用域）
}

// Zero 内存清零（详细设计 §5：用完即清）。
func (c *Credential) Zero() {
	if c == nil {
		return
	}
	for i := range c.Secret {
		c.Secret[i] = 0
	}
	c.Secret = nil
}

// PublishRequest 一次发布的输入。媒体文件在 V1 由 MME 工件路径占位（联调时接入对象存储签名）。
type PublishRequest struct {
	Account      *AccountRow
	Credential   *Credential // 可为 nil（mock 平台不需要凭据的场景）
	Title        string
	Desc         string
	Tags         []string
	Visibility   string // private / unlisted / public
	ScheduledFor *time.Time
	VideoPath    string // 已转码媒体路径（V1 占位）
	CoverPath    string
}

// PublishResult 平台发布回执原始结果。
type PublishResult struct {
	PostID      string
	PostURL     string
	Visibility  string
	PublishedAt time.Time
	Raw         map[string]any // 平台原始响应（脱敏后落 receipt）
}

// Performance 表现数据（FR-PUB-08）。
type Performance struct {
	Views          int64
	Likes          int64
	Comments       int64
	Shares         int64
	Favorites      int64
	CompletionRate float64
	FetchedAt      time.Time
}

// Publisher 平台适配器接口（详细设计 §2）。publishadapters 实现。
type Publisher interface {
	PlatformKey() string
	Capability() Capability
	Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)
	FetchAnalytics(ctx context.Context, acct *AccountRow, postID string) (*Performance, error)
}

// AdapterLookup 适配器注册表查询端口（publishadapters 实现并注入）。
type AdapterLookup interface {
	// Get 按平台键取适配器。未注册必须明确返回 ErrAdapterNotFound（缺失错误由调用方包装）。
	Get(key string) (Publisher, bool)
}

// capabilityFromSnapshot converts channel capability snapshot JSON into Capability（best-effort）.
func capabilityFromSnapshot(snap map[string]any) Capability {
	var c Capability
	if snap == nil {
		return c
	}
	c.Video = boolFrom(snap["video"])
	c.ImageText = boolFrom(snap["image_text"])
	c.NativeSchedule = boolFrom(snap["native_schedule"])
	c.DraftState = boolFrom(snap["draft_state"])
	c.Analytics = boolFrom(snap["analytics"])
	c.MaxDurationSec = intFrom(snap["max_duration_sec"])
	c.MaxSizeBytes = int64From(snap["max_size_bytes"])
	c.TitleMaxLen = intFrom(snap["title_max_len"])
	c.DescMaxLen = intFrom(snap["desc_max_len"])
	if arr, ok := snap["aspect_ratios"].([]any); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok {
				c.AspectRatios = append(c.AspectRatios, s)
			}
		}
	}
	return c
}

// CapabilityOf 渠道能力视图：优先取渠道 capability 快照（运营可调），快照缺失回退适配器声明。
func CapabilityOf(ch *ChannelRow, pub Publisher) Capability {
	if ch != nil && len(ch.Capability) > 0 {
		return capabilityFromSnapshot(ch.Capability)
	}
	if pub != nil {
		return pub.Capability()
	}
	return Capability{}
}

func boolFrom(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func intFrom(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

func int64From(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}
