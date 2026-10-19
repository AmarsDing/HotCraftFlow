package publish

import (
	"context"
	"time"
)

// ============================================================================
// 持久化端口（biz 层禁止 import data / ent，由 data 层实现，wire.Bind 绑定；
// 模式对齐 biz/tt/ports.go）。
// ============================================================================

// ChannelRepo 渠道仓储端口。
type ChannelRepo interface {
	// CreateChannel 新建渠道（platform_key+slot 唯一冲突时返回约束错误）。
	CreateChannel(ctx context.Context, row *ChannelRow) error
	// GetChannelBySlot 按 platform:slot 查渠道。
	GetChannelBySlot(ctx context.Context, platform, slot string) (*ChannelRow, error)
	// GetChannel 按 channel_id 查渠道。
	GetChannel(ctx context.Context, channelID string) (*ChannelRow, error)
	// ListChannels 分页列出渠道（create_time 倒序），返回条目与总数。
	// 注意：data 层对 pageSize 有 UI 护栏（>200 钳到 200）——「取某平台全量渠道」的
	// 内部路径不得依赖本方法，必须用 ListChannelsByPlatform。
	ListChannels(ctx context.Context, pageNo, pageSize int) ([]*ChannelRow, int, error)
	// ListChannelsByPlatform 直查指定平台的全部渠道（无分页、无 pageSize 钳制）。
	// worker/map 回流解析专用：与 REST 分页列表（ListChannels）语义分离，
	// 渠道总数超过后台页大小上限时也不会被静默截断。
	ListChannelsByPlatform(ctx context.Context, platform string) ([]*ChannelRow, error)
}

// AccountRepo 账号仓储端口。
type AccountRepo interface {
	// CreateAccount 新建账号（返回自增主键写入 row.ID）。
	CreateAccount(ctx context.Context, row *AccountRow) error
	// GetAccount 按 account_id（pa_ 前缀）查询。
	GetAccount(ctx context.Context, accountID string) (*AccountRow, error)
	// ListAccounts 分页列出账号（create_time 倒序）。
	ListAccounts(ctx context.Context, pageNo, pageSize int) ([]*AccountRow, int, error)
	// ListAccountsByChannel 列出渠道下全部账号（指派发布账号用）。
	ListAccountsByChannel(ctx context.Context, channelID string) ([]*AccountRow, error)
	// SetEnabled 人工启停。
	SetEnabled(ctx context.Context, accountID string, enabled bool) error
	// SetHealth 设置健康状态；lastErr 为 nil 时不动 last_error（已脱敏的文案才允许传入）。
	SetHealth(ctx context.Context, accountID, health string, lastErr *string) error
	// SetCredentialRef 回写凭据引用与类型（vault.Store 成功后调用）。
	SetCredentialRef(ctx context.Context, accountID, ref, credType string) error
	// IncrementFailures 连续失败数 +1，返回新值（熔断判定输入）。
	IncrementFailures(ctx context.Context, accountID string) (int, error)
	// ResetFailures 连续失败数清零（发布成功/恢复时）。
	ResetFailures(ctx context.Context, accountID string) error
	// PauseAccount 熔断暂停：health=paused + paused_until（已脱敏的 lastErr 一并记录）。
	PauseAccount(ctx context.Context, accountID string, until time.Time, lastErr *string) error
	// TouchLastPublished 发布成功后刷新 last_published_at 并快递增 published_24h（近似滚动计数；
	// 配额判定的权威来源是 receipts 时间窗计数，见 QuotaChecker）。
	TouchLastPublished(ctx context.Context, accountID string, at time.Time) error
}

// DispatchRepo 分发单元仓储端口。
type DispatchRepo interface {
	// CreateUnit 新建分发单元。
	CreateUnit(ctx context.Context, row *UnitRow) error
	// UpdateStates 状态流转补丁（审批区/发布区字段；nil=不动）。
	UpdateStates(ctx context.Context, unitID string, patch UnitPatch) (*UnitRow, error)
	// GetUnit 按 unit_id（du_ 前缀）查询。
	GetUnit(ctx context.Context, unitID string) (*UnitRow, error)
	// ListUnits 分页列出（create_time 倒序）。
	ListUnits(ctx context.Context, pageNo, pageSize int) ([]*UnitRow, int, error)
	// ListUnitsByApproval 按审批状态分页列出（审批队列 draft 用）。
	ListUnitsByApproval(ctx context.Context, state string, pageNo, pageSize int) ([]*UnitRow, int, error)
	// ListScheduledBetween 日历视图：scheduled_for ∈ [from, to] 的单元（无 scheduled_for 不入日历）。
	ListScheduledBetween(ctx context.Context, from, to time.Time) ([]*UnitRow, error)
	// ListPublishDue 到点扫描（worker/map 30s 轮询）：local_worker 模式 + 已审批 +
	// pending/failed + scheduled_for ≤ before（before 已含拟人化偏移上界）；
	// scheduled_for 升序，限 limit 条。
	ListPublishDue(ctx context.Context, before time.Time, limit int) ([]*UnitRow, error)
}

// UnitPatch 状态流转补丁（审批区与发布区分离；nil 字段不触碰）。
type UnitPatch struct {
	// —— 审批区 ——
	ApprovalState     *string    // draft / approved / rejected
	Approver          *string    // 非 nil 时写入审批者（approve 用）
	ApprovedAt        *time.Time // 非 nil 时写入审批时间
	ClearApprover     bool       // 为 true 时清空 approver + approved_at（重提/驳回到草稿）
	RejectReason      *string    // 非 nil 时写入驳回原因
	ClearRejectReason bool       // 为 true 时清空驳回原因（approve 用）

	// —— 发布区 ——
	PublishState *string    // pending / publishing / published / failed
	Operator     *string    // 非 nil 时写入发布操作者
	PublishedAt  *time.Time // 非 nil 时写入发布时间
	PostID       *string
	PostURL      *string
}

// ApprovalRepo 审批审计仓储端口。
type ApprovalRepo interface {
	// Record 追加一条审计流水（append-only，更新/删除能力绝不提供）。
	Record(ctx context.Context, e *RecordEntry) error
	// ListRecordsByUnit 按 unit 列出全部流水（create_time 升序）。
	ListRecordsByUnit(ctx context.Context, unitID string) ([]*RecordEntry, error)
}

// ReceiptRepo 发布回执仓储端口。
type ReceiptRepo interface {
	// CreateReceipt 新增回执；(dispatch_unit_id, platform_key) 唯一冲突返回 ErrReceiptConflict（幂等兜底）。
	CreateReceipt(ctx context.Context, row *ReceiptRow) error
	// ListReceiptsByUnit 按 unit 列出回执。
	ListReceiptsByUnit(ctx context.Context, unitID string) ([]*ReceiptRow, error)
	// ListReceipts 分页列出（published_at 倒序）。
	ListReceipts(ctx context.Context, pageNo, pageSize int) ([]*ReceiptRow, int, error)
	// GetReceipt 按 receipt_id（pr_ 前缀）查询。
	GetReceipt(ctx context.Context, receiptID string) (*ReceiptRow, error)
	// CountReceiptsSince 统计 since 后的回执总数（24h 总量配额）。
	CountReceiptsSince(ctx context.Context, since time.Time) (int, error)
	// CountReceiptsByPlatformSince 统计指定平台 since 后的回执数（平台小时配额）。
	CountReceiptsByPlatformSince(ctx context.Context, platform string, since time.Time) (int, error)
	// ListReceiptsPublishedSince 回流扫描源（worker/map 分析拉取管道）：published_at ≥ since
	// 且 post_id 非空（无 post_id 无可拉取对象），published_at 倒序（新鲜数据优先），限 limit。
	// limit 语义（与 Ent 实现逐字对齐）：limit <= 0 时取默认 64，不是「不限」——
	// 传 0 期望无界的调用方会静默截断；需要更多请显式传值。fake 必须复制该默认。
	ListReceiptsPublishedSince(ctx context.Context, since time.Time, limit int) ([]*ReceiptRow, error)
	// ListReceiptsPublishedSinceBefore 分页变体（M6 修复）：published_at ∈ [since, before)。
	// worker 以「本页最旧 published_at」为下一页 before 游标翻页拉满窗口；before 零值=首页。
	ListReceiptsPublishedSinceBefore(ctx context.Context, since, before time.Time, limit int) ([]*ReceiptRow, error)
}

// PerformanceRepo 表现快照仓储端口（FR-PUB-08：读写两侧齐备——worker 追加时序快照，
// REST/dispatch 用例读最新一条；MM_PUB stream 建成后的跨系统事件发射另计）。
type PerformanceRepo interface {
	// SaveSnapshot 追加一条时序快照（append-only：表现数为时序数据，历史轮次保留供趋势观察）。
	SaveSnapshot(ctx context.Context, row *PerformanceSnapshotRow) error
	// LatestByReceipt 取回执最新一条快照；无数据返回 (nil, nil)。
	LatestByReceipt(ctx context.Context, receiptID string) (*PerformanceSnapshotRow, error)
}

// ============================================================================
// 横切端口
// ============================================================================

// CredentialVault 凭据保险库端口（FR-PUB-05 / NFR-MAP-03 / 详细设计 §5）。
// 密钥来自环境变量（不入库）；明文仅存在于 Load 返回后、Zero 调用前的函数作用域。
type CredentialVault interface {
	// Store 加密存储凭据，返回引用（形如 enc:v1:<b64>），密文/引用可入库。
	Store(ctx context.Context, accountID string, cred *Credential) (ref string, err error)
	// Load 按引用解密（🔴 调用方必须 defer cred.Zero()）。
	Load(ctx context.Context, ref string) (*Credential, error)
	// Delete 删除凭据（账号删除场景；🔴 凭据失效不调用——保留便于排查）。
	Delete(ctx context.Context, ref string) error
}

// PublishLocker 发布幂等锁端口（NFR-MAP-05，Redis SET NX；data 层实现，可 nil 降级为进程内锁）。
type PublishLocker interface {
	// Lock 获取 unit 的发布锁，ttl 内防重；已占用返回 (false, nil)。
	Lock(ctx context.Context, unitID string, ttl time.Duration) (bool, error)
	// Unlock 释放锁（尽力而为）。
	Unlock(ctx context.Context, unitID string) error
}
