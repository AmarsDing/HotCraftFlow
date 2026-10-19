// 表现回流 worker 单测（无 DB/无 asynq/无网络——全 fake 替代，同 schedule_worker 测试型制）。
//
// 用例矩阵：
//
//	happy path / 能力门 / 渠道停用 / 凭据失效
//	多槽位：无账号诚实跳过 / 账号锚定槽位 / 锚定槽位能力门 / 槽位已删 / 平台不一致 / 槽位停用
//	上游错误下一轮自然重拉 / 快照写失败不崩 / 扫描源失败如实上报
//	反馈总闸 / 30 天窗 / 无 post_id
//
// fake 谓词与 Ent 逐字对齐（published_at ≥ since + post_id 非空 + limit<=0 默认 64；
// ListChannels 分页钳制；ListChannelsByPlatform 无分页钳制）。
package mapworker

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/pkg/mediacontract"
)

// ---------- fakes ----------

type aReceiptsFake struct {
	mu      sync.Mutex
	rows    []*bizpub.ReceiptRow
	scanErr error
}

func (f *aReceiptsFake) CreateReceipt(_ context.Context, row *bizpub.ReceiptRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, row)
	return nil
}
func (f *aReceiptsFake) ListReceiptsByUnit(_ context.Context, unitID string) ([]*bizpub.ReceiptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*bizpub.ReceiptRow
	for _, r := range f.rows {
		if r.DispatchUnitID == unitID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *aReceiptsFake) ListReceipts(_ context.Context, _, _ int) ([]*bizpub.ReceiptRow, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows, len(f.rows), nil
}
func (f *aReceiptsFake) GetReceipt(_ context.Context, receiptID string) (*bizpub.ReceiptRow, error) {
	for _, r := range f.rows {
		if r.ReceiptID == receiptID {
			return r, nil
		}
	}
	return nil, bizpub.ErrNotFound
}
func (f *aReceiptsFake) CountReceiptsSince(context.Context, time.Time) (int, error) { return 0, nil }
func (f *aReceiptsFake) CountReceiptsByPlatformSince(context.Context, string, time.Time) (int, error) {
	return 0, nil
}

// ListReceiptsPublishedSince 谓词副本（与 Ent 逐字对齐：≥since + post_id 非空，
// published_at 倒序，limit<=0 默认 64——不是「不限」）。
func (f *aReceiptsFake) ListReceiptsPublishedSince(_ context.Context, since time.Time, limit int) ([]*bizpub.ReceiptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	if limit <= 0 {
		limit = 64
	}
	var out []*bizpub.ReceiptRow
	for _, r := range f.rows {
		if r.PublishedAt.Before(since) || r.PostID == nil || *r.PostID == "" {
			continue
		}
		out = append(out, r)
	}
	// published_at 倒序（新鲜优先）；时间相同按 ID 倒序保确定性
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
func (f *aReceiptsFake) ListReceiptsPublishedSinceBefore(_ context.Context, since, before time.Time, limit int) ([]*bizpub.ReceiptRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	if limit <= 0 {
		limit = 64
	}
	var out []*bizpub.ReceiptRow
	for _, r := range f.rows {
		if r.PublishedAt.Before(since) || (!before.IsZero() && !r.PublishedAt.Before(before)) ||
			r.PostID == nil || *r.PostID == "" {
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

type aPerformanceFake struct {
	mu      sync.Mutex
	snaps   []*bizpub.PerformanceSnapshotRow
	saveErr error
}

func (f *aPerformanceFake) SaveSnapshot(_ context.Context, row *bizpub.PerformanceSnapshotRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	f.snaps = append(f.snaps, row)
	return nil
}
func (f *aPerformanceFake) LatestByReceipt(_ context.Context, receiptID string) (*bizpub.PerformanceSnapshotRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *bizpub.PerformanceSnapshotRow
	for _, s := range f.snaps {
		if s.ReceiptID == receiptID && (best == nil || s.FetchedAt.After(best.FetchedAt)) {
			best = s
		}
	}
	return best, nil
}

type aChannelsFake struct {
	mu         sync.Mutex
	byPlatform map[string][]*bizpub.ChannelRow
}

func newAChannelsFake(channels ...*bizpub.ChannelRow) *aChannelsFake {
	f := &aChannelsFake{byPlatform: map[string][]*bizpub.ChannelRow{}}
	for _, ch := range channels {
		f.byPlatform[ch.PlatformKey] = append(f.byPlatform[ch.PlatformKey], ch)
	}
	return f
}
func (f *aChannelsFake) GetChannel(_ context.Context, channelID string) (*bizpub.ChannelRow, error) {
	for _, chs := range f.byPlatform {
		for _, ch := range chs {
			if ch.ChannelID == channelID {
				return ch, nil
			}
		}
	}
	return nil, bizpub.ErrNotFound
}

// ListChannels 分页副本（与 Ent 对齐：clampPage 护栏 + create_time 倒序 + 偏移/限长）。
func (f *aChannelsFake) ListChannels(_ context.Context, pageNo, pageSize int) ([]*bizpub.ChannelRow, int, error) {
	if pageNo < 1 {
		pageNo = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	all := make([]*bizpub.ChannelRow, 0)
	for _, chs := range f.byPlatform {
		all = append(all, chs...)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreateTime.Equal(all[j].CreateTime) {
			return all[i].CreateTime.After(all[j].CreateTime)
		}
		return all[i].ID > all[j].ID
	})
	total := len(all)
	start := (pageNo - 1) * pageSize
	if start >= total {
		return nil, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return all[start:end], total, nil
}

// ListChannelsByPlatform 直查副本（与 Ent 对齐：平台谓词 + slot/id 升序，无分页钳制）。
func (f *aChannelsFake) ListChannelsByPlatform(_ context.Context, platform string) ([]*bizpub.ChannelRow, error) {
	out := append([]*bizpub.ChannelRow(nil), f.byPlatform[platform]...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slot != out[j].Slot {
			return out[i].Slot < out[j].Slot
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func (f *aChannelsFake) GetChannelBySlot(_ context.Context, platform, slot string) (*bizpub.ChannelRow, error) {
	for _, ch := range f.byPlatform[platform] {
		if ch.Slot == slot {
			return ch, nil
		}
	}
	return nil, bizpub.ErrNotFound
}
func (f *aChannelsFake) CreateChannel(_ context.Context, row *bizpub.ChannelRow) error {
	f.byPlatform[row.PlatformKey] = append(f.byPlatform[row.PlatformKey], row)
	return nil
}

type aAccountsFake struct {
	mu             sync.Mutex
	byID           map[string]*bizpub.AccountRow
	byCh           map[string][]*bizpub.AccountRow
	setHealthCalls int
}

func newAAccountsFake(accounts ...*bizpub.AccountRow) *aAccountsFake {
	f := &aAccountsFake{byID: map[string]*bizpub.AccountRow{}, byCh: map[string][]*bizpub.AccountRow{}}
	for _, a := range accounts {
		f.byID[a.AccountID] = a
		f.byCh[a.ChannelID] = append(f.byCh[a.ChannelID], a)
	}
	return f
}
func (f *aAccountsFake) GetAccount(_ context.Context, accountID string) (*bizpub.AccountRow, error) {
	if a, ok := f.byID[accountID]; ok {
		return a, nil
	}
	return nil, bizpub.ErrNotFound
}
func (f *aAccountsFake) ListAccountsByChannel(_ context.Context, channelID string) ([]*bizpub.AccountRow, error) {
	return f.byCh[channelID], nil
}
func (f *aAccountsFake) CreateAccount(_ context.Context, row *bizpub.AccountRow) error {
	f.byID[row.AccountID] = row
	f.byCh[row.ChannelID] = append(f.byCh[row.ChannelID], row)
	return nil
}
func (f *aAccountsFake) ListAccounts(_ context.Context, _, _ int) ([]*bizpub.AccountRow, int, error) {
	var out []*bizpub.AccountRow
	for _, a := range f.byID {
		out = append(out, a)
	}
	return out, len(out), nil
}
func (f *aAccountsFake) SetEnabled(_ context.Context, accountID string, enabled bool) error {
	if a, ok := f.byID[accountID]; ok {
		a.Enabled = enabled
	}
	return nil
}
func (f *aAccountsFake) SetHealth(_ context.Context, accountID, health string, lastErr *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setHealthCalls++
	if a, ok := f.byID[accountID]; ok {
		a.Health = health
		if lastErr != nil {
			a.LastError = lastErr
		}
	}
	return nil
}
func (f *aAccountsFake) SetCredentialRef(_ context.Context, accountID, ref, credType string) error {
	if a, ok := f.byID[accountID]; ok {
		a.CredentialRef = &ref
		a.CredentialType = credType
	}
	return nil
}
func (f *aAccountsFake) IncrementFailures(_ context.Context, accountID string) (int, error) {
	if a, ok := f.byID[accountID]; ok {
		a.ConsecutiveFailures++
		return a.ConsecutiveFailures, nil
	}
	return 0, nil
}
func (f *aAccountsFake) ResetFailures(_ context.Context, accountID string) error {
	if a, ok := f.byID[accountID]; ok {
		a.ConsecutiveFailures = 0
	}
	return nil
}
func (f *aAccountsFake) PauseAccount(_ context.Context, accountID string, until time.Time, lastErr *string) error {
	if a, ok := f.byID[accountID]; ok {
		a.Health = "paused"
		a.PausedUntil = &until
	}
	return nil
}
func (f *aAccountsFake) TouchLastPublished(_ context.Context, accountID string, at time.Time) error {
	if a, ok := f.byID[accountID]; ok {
		a.LastPublishedAt = &at
		a.Published24h++
	}
	return nil
}

type aAdaptersFake struct {
	pubs map[string]bizpub.Publisher
}

func newAAdaptersFake(pubs ...bizpub.Publisher) *aAdaptersFake {
	f := &aAdaptersFake{pubs: map[string]bizpub.Publisher{}}
	for _, p := range pubs {
		f.pubs[p.PlatformKey()] = p
	}
	return f
}
func (f *aAdaptersFake) Get(key string) (bizpub.Publisher, bool) {
	p, ok := f.pubs[key]
	return p, ok
}

type aPublisherFake struct {
	key        string
	analytics  bool
	perf       *bizpub.Performance
	err        error
	fetchCalls int
	lastPostID string
}

func (p *aPublisherFake) PlatformKey() string { return p.key }
func (p *aPublisherFake) Capability() bizpub.Capability {
	return bizpub.Capability{Video: true, Analytics: p.analytics}
}
func (p *aPublisherFake) Publish(context.Context, bizpub.PublishRequest) (*bizpub.PublishResult, error) {
	return nil, nil
}
func (p *aPublisherFake) FetchAnalytics(_ context.Context, _ *bizpub.AccountRow, postID string) (*bizpub.Performance, error) {
	p.fetchCalls++
	p.lastPostID = postID
	return p.perf, p.err
}

type aDispatchFake struct {
	mu    sync.Mutex
	units map[string]*bizpub.UnitRow
}

func newADispatchFake(units ...*bizpub.UnitRow) *aDispatchFake {
	f := &aDispatchFake{units: map[string]*bizpub.UnitRow{}}
	for _, u := range units {
		f.units[u.UnitID] = u
	}
	return f
}
func (f *aDispatchFake) CreateUnit(_ context.Context, row *bizpub.UnitRow) error {
	f.units[row.UnitID] = row
	return nil
}
func (f *aDispatchFake) GetUnit(_ context.Context, unitID string) (*bizpub.UnitRow, error) {
	if u, ok := f.units[unitID]; ok {
		return u, nil
	}
	return nil, bizpub.ErrNotFound
}
func (f *aDispatchFake) UpdateStates(_ context.Context, unitID string, patch bizpub.UnitPatch) (*bizpub.UnitRow, error) {
	if u, ok := f.units[unitID]; ok {
		if patch.PublishState != nil {
			u.PublishState = *patch.PublishState
		}
		return u, nil
	}
	return nil, bizpub.ErrNotFound
}
func (f *aDispatchFake) ListUnits(context.Context, int, int) ([]*bizpub.UnitRow, int, error) {
	return nil, 0, nil
}
func (f *aDispatchFake) ListUnitsByApproval(context.Context, string, int, int) ([]*bizpub.UnitRow, int, error) {
	return nil, 0, nil
}
func (f *aDispatchFake) ListScheduledBetween(context.Context, time.Time, time.Time) ([]*bizpub.UnitRow, error) {
	return nil, nil
}
func (f *aDispatchFake) ListPublishDue(context.Context, time.Time, int) ([]*bizpub.UnitRow, error) {
	return nil, nil
}

// ---------- helpers ----------

func aBaseConf() *bizpub.Conf {
	c := bizpub.DefaultConf()
	c.Feedback.Enabled = true
	return &c
}

func aNow() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }

func aReceipt(id, unit, platform string, publishedAt time.Time, postID string) *bizpub.ReceiptRow {
	r := &bizpub.ReceiptRow{
		ReceiptID:      id,
		DispatchUnitID: unit,
		PlatformKey:    platform,
		Visibility:     "private",
		PublishedAt:    publishedAt,
		Operator:       "bob",
	}
	if postID != "" {
		r.PostID = &postID
	}
	return r
}

func aAccount(id, chID string) *bizpub.AccountRow {
	return &bizpub.AccountRow{
		AccountID:   id,
		ChannelID:   chID,
		DisplayName: id,
		Enabled:     true,
		Health:      "healthy",
	}
}

func aChannel(id, platform, slot string, analytics bool) *bizpub.ChannelRow {
	return &bizpub.ChannelRow{
		ChannelID:   id,
		PlatformKey: platform,
		Slot:        slot,
		Enabled:     true,
		Capability:  map[string]any{"video": true, "analytics": analytics},
	}
}

// ---------- tests ----------

// 拉取成功 → 快照落表（字段完整：回执关联/平台/指标/fetched_at）。
func TestAnalyticsPull_SavesSnapshot(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{
		key: "youtube", analytics: true,
		perf: &bizpub.Performance{Views: 1000, Likes: 88, Comments: 12, Shares: 5, Favorites: 30, CompletionRate: 0.62},
	}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1", VariantID: sp("v_1")})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 1 {
		t.Fatalf("want 1 snapshot, got %d", len(perf.snaps))
	}
	s := perf.snaps[0]
	if s.ReceiptID != "pr_1" || s.PlatformKey != "youtube" || s.Views != 1000 || s.Likes != 88 {
		t.Errorf("snapshot fields wrong: %+v", s)
	}
	if s.DispatchUnitID == nil || *s.DispatchUnitID != "du_1" {
		t.Errorf("dispatch_unit_id backfill missing")
	}
	if s.VariantID == nil || *s.VariantID != "v_1" {
		t.Errorf("variant_id backfill missing")
	}
	if s.FetchedAt.IsZero() {
		t.Errorf("fetched_at missing")
	}
	if pub.fetchCalls != 1 || pub.lastPostID != "post-123" {
		t.Errorf("FetchAnalytics call count=%d postID=%q", pub.fetchCalls, pub.lastPostID)
	}
}

// 平台未声明 analytics 能力 → 跳过（终态，不拉取）。
func TestAnalyticsPull_AnalyticsUnsupported_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", false) // analytics=false
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true} // 渠道快照关
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
	if pub.fetchCalls != 0 {
		t.Errorf("FetchAnalytics should not be called when analytics unsupported")
	}
}

// 渠道停用 → 跳过（终态）。
func TestAnalyticsPull_ChannelDisabled_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	ch.Enabled = false
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
}

// 凭据失效 → 账号置 credential_invalid + 不重复拉取（终态，凭据保留）。
func TestAnalyticsPull_CredentialInvalid_MarksAccount(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true, err: bizpub.ErrCredentialInvalid}
	adapters := newAAdaptersFake(pub)
	accounts := newAAccountsFake(acct)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), accounts, adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
	if acct.Health != "credential_invalid" {
		t.Errorf("account health=%q, want credential_invalid", acct.Health)
	}
	if accounts.setHealthCalls != 1 {
		t.Errorf("SetHealth calls=%d, want 1", accounts.setHealthCalls)
	}
}

// 多槽位平台且回执无发布账号 → 无槽位依据，诚实跳过（不臆测槽位）。
// （真实发布回执恒带 account_id；本用例覆盖历史/导入回执的兜底语义。）
func TestAnalyticsPull_MultiSlotPlatform_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch1 := aChannel("ch_1", "youtube", "main", true)
	ch2 := aChannel("ch_2", "youtube", "a1", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch1, ch2), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
	if pub.fetchCalls != 0 {
		t.Errorf("FetchAnalytics should not be called without slot evidence")
	}
}

// 多槽位平台 + 回执发布账号 → 经账号锚定发布槽位，照常拉取（不再误判为诚实 skipped）。
func TestAnalyticsPull_MultiSlot_UsesReceiptAccount_AnchorsSlot(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	acctID := "pa_shorts"
	rc.AccountID = &acctID
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	chMain := aChannel("ch_main", "youtube", "main", true)
	chShorts := aChannel("ch_shorts", "youtube", "shorts", true) // 两渠道同平台 → 多槽位
	acct := aAccount("pa_shorts", "ch_shorts")
	pub := &aPublisherFake{key: "youtube", analytics: true, perf: &bizpub.Performance{Views: 42}}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(chMain, chShorts), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 1 {
		t.Fatalf("want 1 snapshot (anchored slot), got %d", len(perf.snaps))
	}
	if perf.snaps[0].Views != 42 {
		t.Errorf("snapshot views=%d, want 42", perf.snaps[0].Views)
	}
	if pub.fetchCalls != 1 {
		t.Errorf("FetchAnalytics calls=%d, want 1", pub.fetchCalls)
	}
}

// 多槽位锚定后，渠道快照 analytics=false 优先于适配器声明 → 跳过。
// （回归：曾因 ch==nil 回退适配器声明而旁路槽位快照的能力门。）
func TestAnalyticsPull_MultiSlot_AnchoredSlotAnalyticsOff_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	acctID := "pa_shorts"
	rc.AccountID = &acctID
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	chMain := aChannel("ch_main", "youtube", "main", true)
	chShorts := aChannel("ch_shorts", "youtube", "shorts", false) // 槽位快照关 analytics
	acct := aAccount("pa_shorts", "ch_shorts")
	pub := &aPublisherFake{key: "youtube", analytics: true} // 适配器声明开——不得覆盖快照
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(chMain, chShorts), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots (slot capability off), got %d", len(perf.snaps))
	}
	if pub.fetchCalls != 0 {
		t.Errorf("FetchAnalytics calls=%d, want 0 (channel snapshot gate wins)", pub.fetchCalls)
	}
}

// 锚定槽位已删 → 终态跳过（不延后重试）。
func TestAnalyticsPull_MultiSlot_AnchoredChannelGone_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	acctID := "pa_ghost"
	rc.AccountID = &acctID
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	chMain := aChannel("ch_main", "youtube", "main", true)
	chShorts := aChannel("ch_shorts", "youtube", "shorts", true)
	acct := aAccount("pa_ghost", "ch_ghost") // 账号所属渠道不在仓储（发布后被删）
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(chMain, chShorts), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 || pub.fetchCalls != 0 {
		t.Fatalf("want honest skip, snaps=%d fetchCalls=%d", len(perf.snaps), pub.fetchCalls)
	}
}

// 锚定渠道平台与回执平台不一致（数据异常）→ 跳过，不猜测。
func TestAnalyticsPull_MultiSlot_AnchoredPlatformMismatch_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	acctID := "pa_other"
	rc.AccountID = &acctID
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	chMain := aChannel("ch_main", "youtube", "main", true)
	chShorts := aChannel("ch_shorts", "youtube", "shorts", true)
	chOther := aChannel("ch_other", "bilibili", "main", true) // 账号实际所属另一平台
	acct := aAccount("pa_other", "ch_other")
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(chMain, chShorts, chOther), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 || pub.fetchCalls != 0 {
		t.Fatalf("want honest skip on platform mismatch, snaps=%d fetchCalls=%d", len(perf.snaps), pub.fetchCalls)
	}
}

// 锚定渠道已停用 → 终态跳过。
func TestAnalyticsPull_MultiSlot_AnchoredChannelDisabled_Skips(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	acctID := "pa_shorts"
	rc.AccountID = &acctID
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	chMain := aChannel("ch_main", "youtube", "main", true)
	chShorts := aChannel("ch_shorts", "youtube", "shorts", true)
	chShorts.Enabled = false
	acct := aAccount("pa_shorts", "ch_shorts")
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(chMain, chShorts), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 || pub.fetchCalls != 0 {
		t.Fatalf("want honest skip on disabled anchored slot, snaps=%d fetchCalls=%d", len(perf.snaps), pub.fetchCalls)
	}
}

// 扫描源失败 → 整轮如实上报（asynq 按周期任务失败重投），不吞错。
func TestAnalyticsPull_ScanError_Propagates(t *testing.T) {
	conf := aBaseConf()
	receipts := &aReceiptsFake{scanErr: bizpub.ErrInternal}
	w := NewAnalyticsWorker(nil, conf, receipts, &aPerformanceFake{}, newADispatchFake(),
		newAChannelsFake(), newAAccountsFake(), newAAdaptersFake(), log.DefaultLogger)
	w.clock = func() time.Time { return aNow() }
	if err := w.pullAll(context.Background()); err == nil {
		t.Fatal("want scan error propagated, got nil")
	}
}

// 快照写入失败 → 该条 failed（平台数据本轮丢弃、下轮重拉），不崩整轮。
func TestAnalyticsPull_SaveError_MarksFailedNoCrash(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{saveErr: bizpub.ErrInternal}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true, perf: &bizpub.Performance{Views: 7}}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll should not error on save failure: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
	if pub.fetchCalls != 1 {
		t.Errorf("FetchAnalytics calls=%d, want 1（平台数据已拉到，仅落库失败）", pub.fetchCalls)
	}
}

// 回执指定账号（发布同账号）→ 数据口径一致。
func TestAnalyticsPull_UsesReceiptAccount(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	acctID := "pa_1"
	rc.AccountID = &acctID
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true,
		perf: &bizpub.Performance{Views: 999}}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 1 {
		t.Fatalf("want 1 snapshot, got %d", len(perf.snaps))
	}
}

// 上游错误（非终态）→ 本轮 failed，下轮自然重拉（无独立重试队列）。
func TestAnalyticsPull_UpstreamError_FailedButNoCrash(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true, err: bizpub.ErrInternal}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll should not error on per-receipt failure: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
}

// 反馈总闸关闭 → 整轮 no-op。
func TestAnalyticsPull_FeedbackDisabled_NoOp(t *testing.T) {
	conf := aBaseConf()
	conf.Feedback.Enabled = false
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "post-123")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(), newAAccountsFake(), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
	if pub.fetchCalls != 0 {
		t.Errorf("FetchAnalytics should not be called when disabled")
	}
}

// 30 天窗外回执不拉（谓词过滤）。
func TestAnalyticsPull_OldReceiptsSkipped(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	old := aReceipt("pr_old", "du_1", "youtube", now.Add(-31*24*time.Hour), "post-old")
	fresh := aReceipt("pr_new", "du_2", "youtube", now.Add(-time.Hour), "post-new")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{old, fresh}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true,
		perf: &bizpub.Performance{Views: 1}}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"}, &bizpub.UnitRow{UnitID: "du_2"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 1 {
		t.Fatalf("want 1 snapshot (only fresh receipt), got %d", len(perf.snaps))
	}
	if perf.snaps[0].ReceiptID != "pr_new" {
		t.Errorf("pulled receipt=%q, want pr_new", perf.snaps[0].ReceiptID)
	}
}

// 无 post_id 回执不拉（谓词过滤）。
func TestAnalyticsPull_NoPostID_Skipped(t *testing.T) {
	conf := aBaseConf()
	now := aNow()
	rc := aReceipt("pr_1", "du_1", "youtube", now.Add(-time.Hour), "")
	receipts := &aReceiptsFake{rows: []*bizpub.ReceiptRow{rc}}
	perf := &aPerformanceFake{}
	ch := aChannel("ch_1", "youtube", "main", true)
	acct := aAccount("pa_1", "ch_1")
	pub := &aPublisherFake{key: "youtube", analytics: true}
	adapters := newAAdaptersFake(pub)
	dispatch := newADispatchFake(&bizpub.UnitRow{UnitID: "du_1"})

	w := NewAnalyticsWorker(nil, conf, receipts, perf, dispatch, newAChannelsFake(ch), newAAccountsFake(acct), adapters, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	if err := w.pullAll(context.Background()); err != nil {
		t.Fatalf("pullAll: %v", err)
	}
	if len(perf.snaps) != 0 {
		t.Fatalf("want 0 snapshots, got %d", len(perf.snaps))
	}
}

// 静默测试桩：mediacontract 引用消除（契约 ID 生成器已由 worker 直接调用）。
var _ = mediacontract.NewPerformanceSnapID
