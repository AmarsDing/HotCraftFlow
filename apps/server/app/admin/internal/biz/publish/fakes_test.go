package publish

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"framework/pkg/mediacontract"
)

// ============================================================================
// 测试替身：biz 端口的内存实现（无 DB/无 Redis）。
// 只承载单测需要的最小语义；任何「看起来像真数据库」的行为都保持简单直白。
// ============================================================================

// ---------- Channels ----------

type fakeChannels struct {
	bySlot map[string]*ChannelRow
	byID   map[string]*ChannelRow
}

func newFakeChannels(rows ...*ChannelRow) *fakeChannels {
	f := &fakeChannels{bySlot: map[string]*ChannelRow{}, byID: map[string]*ChannelRow{}}
	for _, r := range rows {
		f.bySlot[r.PlatformKey+":"+r.Slot] = r
		f.byID[r.ChannelID] = r
	}
	return f
}

func (f *fakeChannels) CreateChannel(_ context.Context, row *ChannelRow) error {
	f.bySlot[row.PlatformKey+":"+row.Slot] = row
	f.byID[row.ChannelID] = row
	return nil
}

func (f *fakeChannels) GetChannelBySlot(_ context.Context, platform, slot string) (*ChannelRow, error) {
	if r, ok := f.bySlot[platform+":"+slot]; ok {
		return r, nil
	}
	return nil, ErrNotFound
}

func (f *fakeChannels) GetChannel(_ context.Context, channelID string) (*ChannelRow, error) {
	if r, ok := f.byID[channelID]; ok {
		return r, nil
	}
	return nil, ErrNotFound
}

// ListChannels 分页副本（与 Ent 实现对齐：clampPage 护栏 + create_time 倒序 + 偏移/限长）。
func (f *fakeChannels) ListChannels(_ context.Context, pageNo, pageSize int) ([]*ChannelRow, int, error) {
	if pageNo < 1 {
		pageNo = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	all := make([]*ChannelRow, 0, len(f.byID))
	for _, r := range f.byID {
		all = append(all, r)
	}
	// Ent 侧为 create_time 倒序；时间相同按 ID 倒序保确定性（DB 平局序未定义）。
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreateTime.Equal(all[j].CreateTime) {
			return all[i].CreateTime.After(all[j].CreateTime)
		}
		return all[i].ID > all[j].ID
	})
	total := len(all)
	start := (pageNo - 1) * pageSize
	if start >= total {
		return []*ChannelRow{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

// ListChannelsByPlatform 直查副本（与 Ent 实现对齐：平台谓词过滤、无分页钳制）。
func (f *fakeChannels) ListChannelsByPlatform(_ context.Context, platform string) ([]*ChannelRow, error) {
	out := make([]*ChannelRow, 0, len(f.byID))
	for _, r := range f.byID {
		if r.PlatformKey == platform {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ---------- Accounts ----------

type healthCall struct {
	health  string
	lastErr *string
}

type fakeAccounts struct {
	mu       sync.Mutex
	byID     map[string]*AccountRow
	failures map[string]int
	// 观测
	pausedCalls  map[string]*time.Time
	healthCalls  map[string][]healthCall
	lastPublTaps map[string]time.Time
}

func newFakeAccounts(rows ...*AccountRow) *fakeAccounts {
	f := &fakeAccounts{
		byID:         map[string]*AccountRow{},
		failures:     map[string]int{},
		pausedCalls:  map[string]*time.Time{},
		healthCalls:  map[string][]healthCall{},
		lastPublTaps: map[string]time.Time{},
	}
	for _, r := range rows {
		f.byID[r.AccountID] = r
		f.failures[r.AccountID] = r.ConsecutiveFailures
	}
	return f
}

func (f *fakeAccounts) CreateAccount(_ context.Context, row *AccountRow) error {
	f.byID[row.AccountID] = row
	return nil
}

func (f *fakeAccounts) GetAccount(_ context.Context, accountID string) (*AccountRow, error) {
	if r, ok := f.byID[accountID]; ok {
		return r, nil
	}
	return nil, ErrNotFound
}

func (f *fakeAccounts) ListAccounts(_ context.Context, _, _ int) ([]*AccountRow, int, error) {
	out := make([]*AccountRow, 0, len(f.byID))
	for _, r := range f.byID {
		out = append(out, r)
	}
	return out, len(out), nil
}

func (f *fakeAccounts) ListAccountsByChannel(_ context.Context, channelID string) ([]*AccountRow, error) {
	var out []*AccountRow
	for _, r := range f.byID {
		if r.ChannelID == channelID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}

func (f *fakeAccounts) SetEnabled(_ context.Context, accountID string, enabled bool) error {
	if r, ok := f.byID[accountID]; ok {
		r.Enabled = enabled
		return nil
	}
	return ErrNotFound
}

func (f *fakeAccounts) SetHealth(_ context.Context, accountID, health string, lastErr *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byID[accountID]
	if !ok {
		return ErrNotFound
	}
	r.Health = health
	if lastErr != nil {
		r.LastError = lastErr
	}
	if health == AccountHealthHealthy {
		r.PausedUntil = nil
	}
	f.healthCalls[accountID] = append(f.healthCalls[accountID], healthCall{health: health, lastErr: lastErr})
	return nil
}

func (f *fakeAccounts) SetCredentialRef(_ context.Context, accountID, ref, credType string) error {
	if r, ok := f.byID[accountID]; ok {
		r.CredentialRef = &ref
		r.CredentialType = credType
		r.HasCredential = true
		return nil
	}
	return ErrNotFound
}

func (f *fakeAccounts) IncrementFailures(_ context.Context, accountID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[accountID]; !ok {
		return 0, ErrNotFound
	}
	f.failures[accountID]++
	f.byID[accountID].ConsecutiveFailures = f.failures[accountID]
	return f.failures[accountID], nil
}

func (f *fakeAccounts) ResetFailures(_ context.Context, accountID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[accountID]; !ok {
		return ErrNotFound
	}
	f.failures[accountID] = 0
	f.byID[accountID].ConsecutiveFailures = 0
	return nil
}

func (f *fakeAccounts) PauseAccount(_ context.Context, accountID string, until time.Time, lastErr *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byID[accountID]
	if !ok {
		return ErrNotFound
	}
	r.Health = AccountHealthPaused
	r.PausedUntil = &until
	if lastErr != nil {
		r.LastError = lastErr
	}
	f.pausedCalls[accountID] = &until
	return nil
}

func (f *fakeAccounts) TouchLastPublished(_ context.Context, accountID string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byID[accountID]
	if !ok {
		return ErrNotFound
	}
	r.LastPublishedAt = &at
	r.Published24h++
	f.lastPublTaps[accountID] = at
	return nil
}

// ---------- Dispatch ----------

type fakeDispatch struct {
	mu     sync.Mutex
	units  map[string]*UnitRow
	onLock func() // 锁后重读钩子（双检场景：把 unit 置 published）
}

func newFakeDispatch(rows ...*UnitRow) *fakeDispatch {
	f := &fakeDispatch{units: map[string]*UnitRow{}}
	for _, r := range rows {
		f.units[r.UnitID] = cloneUnit(r)
	}
	return f
}

func cloneUnit(u *UnitRow) *UnitRow {
	cp := *u
	return &cp
}

func (f *fakeDispatch) CreateUnit(_ context.Context, row *UnitRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.units[row.UnitID] = cloneUnit(row)
	return nil
}

func (f *fakeDispatch) GetUnit(_ context.Context, unitID string) (*UnitRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.onLock != nil {
		f.onLock()
	}
	if r, ok := f.units[unitID]; ok {
		return cloneUnit(r), nil
	}
	return nil, ErrNotFound
}

func (f *fakeDispatch) UpdateStates(_ context.Context, unitID string, patch UnitPatch) (*UnitRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.units[unitID]
	if !ok {
		return nil, ErrNotFound
	}
	if patch.ApprovalState != nil {
		r.ApprovalState = *patch.ApprovalState
	}
	if patch.Approver != nil {
		r.Approver = patch.Approver
	}
	if patch.ApprovedAt != nil {
		r.ApprovedAt = patch.ApprovedAt
	}
	if patch.ClearApprover {
		r.Approver = nil
		r.ApprovedAt = nil
	}
	if patch.RejectReason != nil {
		r.RejectReason = patch.RejectReason
	}
	if patch.ClearRejectReason {
		r.RejectReason = nil
	}
	if patch.PublishState != nil {
		r.PublishState = *patch.PublishState
	}
	if patch.Operator != nil {
		r.Operator = patch.Operator
	}
	if patch.PublishedAt != nil {
		r.PublishedAt = patch.PublishedAt
	}
	if patch.PostID != nil {
		r.PostID = patch.PostID
	}
	if patch.PostURL != nil {
		r.PostURL = patch.PostURL
	}
	return cloneUnit(r), nil
}

func (f *fakeDispatch) ListUnits(_ context.Context, _, _ int) ([]*UnitRow, int, error) {
	out := make([]*UnitRow, 0, len(f.units))
	for _, r := range f.units {
		out = append(out, cloneUnit(r))
	}
	return out, len(out), nil
}

func (f *fakeDispatch) ListUnitsByApproval(_ context.Context, state string, _, _ int) ([]*UnitRow, int, error) {
	var out []*UnitRow
	for _, r := range f.units {
		if r.ApprovalState == state {
			out = append(out, cloneUnit(r))
		}
	}
	return out, len(out), nil
}

func (f *fakeDispatch) ListScheduledBetween(_ context.Context, from, to time.Time) ([]*UnitRow, error) {
	var out []*UnitRow
	for _, r := range f.units {
		if r.ScheduledFor != nil && !r.ScheduledFor.Before(from) && !r.ScheduledFor.After(to) {
			out = append(out, cloneUnit(r))
		}
	}
	return out, nil
}

// ListPublishDue 与 Ent 实现同谓词：local_worker + approved + pending/failed + scheduled_for ≤ before。
func (f *fakeDispatch) ListPublishDue(_ context.Context, before time.Time, limit int) ([]*UnitRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*UnitRow
	for _, r := range f.units {
		if r.ScheduledFor == nil || r.ScheduledFor.After(before) {
			continue
		}
		if r.ScheduleMode != "local_worker" || r.ApprovalState != string(mediacontract.ApprovalApproved) {
			continue
		}
		if r.PublishState != "pending" && r.PublishState != "failed" {
			continue
		}
		out = append(out, cloneUnit(r))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ---------- Records ----------

type fakeRecords struct {
	mu      sync.Mutex
	entries []*RecordEntry
}

func (f *fakeRecords) Record(_ context.Context, e *RecordEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeRecords) ListRecordsByUnit(_ context.Context, unitID string) ([]*RecordEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*RecordEntry
	for _, e := range f.entries {
		if e.DispatchUnitID == unitID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRecords) countByResult(result string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.entries {
		if e.Result == result {
			n++
		}
	}
	return n
}

// ---------- Receipts ----------

type fakeReceipts struct {
	mu          sync.Mutex
	rows        []*ReceiptRow
	unitIndex   map[string]map[string]bool // unitID → platform → exists（唯一索引语义）
	totalCount  int
	platformCnt map[string]int
	// 注入行为
	conflictOnCreate bool
	createCalls      int
}

func newFakeReceipts() *fakeReceipts {
	return &fakeReceipts{unitIndex: map[string]map[string]bool{}, platformCnt: map[string]int{}}
}

func (f *fakeReceipts) CreateReceipt(_ context.Context, row *ReceiptRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	if f.conflictOnCreate {
		return ErrReceiptConflict
	}
	plats, ok := f.unitIndex[row.DispatchUnitID]
	if !ok {
		plats = map[string]bool{}
		f.unitIndex[row.DispatchUnitID] = plats
	}
	if plats[row.PlatformKey] {
		return ErrReceiptConflict // 唯一索引 (dispatch_unit_id, platform_key)
	}
	plats[row.PlatformKey] = true
	f.rows = append(f.rows, row)
	return nil
}

func (f *fakeReceipts) ListReceiptsByUnit(_ context.Context, unitID string) ([]*ReceiptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*ReceiptRow
	for _, r := range f.rows {
		if r.DispatchUnitID == unitID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeReceipts) ListReceipts(_ context.Context, _, _ int) ([]*ReceiptRow, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows, len(f.rows), nil
}

func (f *fakeReceipts) GetReceipt(_ context.Context, receiptID string) (*ReceiptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.ReceiptID == receiptID {
			return r, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeReceipts) CountReceiptsSince(_ context.Context, _ time.Time) (int, error) {
	return f.totalCount, nil
}

func (f *fakeReceipts) CountReceiptsByPlatformSince(_ context.Context, platform string, _ time.Time) (int, error) {
	return f.platformCnt[platform], nil
}

// ListReceiptsPublishedSince 谓词副本（与 Ent 实现对齐：≥since 且 post_id 非空，
// published_at 倒序，limit<=0 默认 64——Ent 语义，fake 必须逐字复制）。
func (f *fakeReceipts) ListReceiptsPublishedSince(_ context.Context, since time.Time, limit int) ([]*ReceiptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit <= 0 {
		limit = 64
	}
	out := make([]*ReceiptRow, 0, len(f.rows))
	for _, r := range f.rows {
		if r.PublishedAt.Before(since) || r.PostID == nil || *r.PostID == "" {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PublishedAt.Equal(out[j].PublishedAt) {
			return out[i].PublishedAt.After(out[j].PublishedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListReceiptsPublishedSinceBefore 分页谓词副本（M6）：published_at ∈ [since, before)。
func (f *fakeReceipts) ListReceiptsPublishedSinceBefore(_ context.Context, since, before time.Time, limit int) ([]*ReceiptRow, error) {
	if !before.IsZero() {
		filtered := make([]*ReceiptRow, 0, len(f.rows))
		for _, r := range f.rows {
			if r.PublishedAt.Before(before) {
				filtered = append(filtered, r)
			}
		}
		saved := f.rows
		f.rows = filtered
		defer func() { f.rows = saved }()
	}
	return f.ListReceiptsPublishedSince(context.Background(), since, limit)
}

// ---------- Performance snapshots（FR-PUB-08） ----------

type fakePerformance struct {
	mu        sync.Mutex
	bySnap    map[string]*PerformanceSnapshotRow
	byReceipt map[string][]*PerformanceSnapshotRow
	saveErr   error
}

func newFakePerformance() *fakePerformance {
	return &fakePerformance{bySnap: map[string]*PerformanceSnapshotRow{}, byReceipt: map[string][]*PerformanceSnapshotRow{}}
}

func (f *fakePerformance) SaveSnapshot(_ context.Context, row *PerformanceSnapshotRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.bySnap[row.SnapshotID] = row
	f.byReceipt[row.ReceiptID] = append(f.byReceipt[row.ReceiptID], row)
	return nil
}

func (f *fakePerformance) LatestByReceipt(_ context.Context, receiptID string) (*PerformanceSnapshotRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.byReceipt[receiptID]
	var best *PerformanceSnapshotRow
	for _, s := range list {
		if best == nil || s.FetchedAt.After(best.FetchedAt) {
			best = s
		}
	}
	return best, nil
}

// ---------- Vault ----------

type fakeVault struct {
	mu      sync.Mutex
	byRef   map[string]Credential
	deletes int
	stores  int
}

func newFakeVault() *fakeVault {
	return &fakeVault{byRef: map[string]Credential{}}
}

func (f *fakeVault) Store(_ context.Context, accountID string, cred *Credential) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stores++
	ref := "fake:ref:" + accountID
	secret := make([]byte, len(cred.Secret))
	copy(secret, cred.Secret)
	f.byRef[ref] = Credential{Type: cred.Type, Secret: secret}
	return ref, nil
}

func (f *fakeVault) Load(_ context.Context, ref string) (*Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.byRef[ref]; ok {
		return &c, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeVault) Delete(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	return nil
}

// ---------- Locker ----------

type fakeLocker struct {
	mu      sync.Mutex
	held    map[string]bool
	denyAll bool
	locks   int
	unlocks int
}

func newFakeLocker() *fakeLocker { return &fakeLocker{held: map[string]bool{}} }

func (f *fakeLocker) Lock(_ context.Context, unitID string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locks++
	if f.denyAll || f.held[unitID] {
		return false, nil
	}
	f.held[unitID] = true
	return true, nil
}

func (f *fakeLocker) Unlock(_ context.Context, unitID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unlocks++
	delete(f.held, unitID)
	return nil
}

// ---------- Adapters / Publisher ----------

type fakeAdapters struct {
	pubs map[string]Publisher
}

func (f *fakeAdapters) Get(key string) (Publisher, bool) {
	p, ok := f.pubs[key]
	return p, ok
}

type publisherCall struct {
	Platform   string
	Visibility string
}

type fakePublisher struct {
	key string
	// 行为注入：nil 表示成功
	errOnPublish error
	publishRes   *PublishResult
	// 观测
	mu         *sync.Mutex
	order      *[]string // 共享顺序记录（多 target 串行断言用）
	calls      []publisherCall
	capability Capability
}

func newSuccessPublisher(key string, order *[]string, mu *sync.Mutex) *fakePublisher {
	return &fakePublisher{
		key:   key,
		mu:    mu,
		order: order,
		publishRes: &PublishResult{
			PostID:      "post_" + key,
			PostURL:     "https://example.test/" + key + "/post",
			Visibility:  "private",
			PublishedAt: time.Unix(1700000000, 0).UTC(),
		},
		capability: Capability{Video: true, NativeSchedule: true, TitleMaxLen: 100, DescMaxLen: 2000},
	}
}

func (p *fakePublisher) PlatformKey() string    { return p.key }
func (p *fakePublisher) Capability() Capability { return p.capability }
func (p *fakePublisher) FetchAnalytics(context.Context, *AccountRow, string) (*Performance, error) {
	return nil, errors.New("not implemented in fake")
}

func (p *fakePublisher) Publish(_ context.Context, req PublishRequest) (*PublishResult, error) {
	if p.mu != nil && p.order != nil {
		p.mu.Lock()
		*p.order = append(*p.order, p.key)
		p.calls = append(p.calls, publisherCall{Platform: p.key, Visibility: req.Visibility})
		p.mu.Unlock()
	}
	if p.errOnPublish != nil {
		return nil, p.errOnPublish
	}
	return p.publishRes, nil
}
