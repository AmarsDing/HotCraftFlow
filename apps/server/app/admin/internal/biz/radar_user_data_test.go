package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	kratoserrors "github.com/go-kratos/kratos/v2/errors"
)

// ── 内存 mock ──

type fakeWatchRepo struct {
	watches map[uint32][]*RadarWatch // userID -> watches
	nextID  uint32
}

func newFakeWatchRepo() *fakeWatchRepo {
	return &fakeWatchRepo{watches: map[uint32][]*RadarWatch{}, nextID: 1}
}

func (f *fakeWatchRepo) ListByUser(_ context.Context, userID uint32) ([]*RadarWatch, error) {
	out := make([]*RadarWatch, 0, len(f.watches[userID]))
	out = append(out, f.watches[userID]...)
	return out, nil
}

func (f *fakeWatchRepo) find(userID uint32, fullName string) *RadarWatch {
	for _, w := range f.watches[userID] {
		if w.FullName == fullName {
			return w
		}
	}
	return nil
}

// GetByFullName 返回副本：真实 ent 每次查询都是新对象，
// 副本语义避免调用方（如 SyncStars 计算 grown 时）意外共享指针被并发更新污染。
func (f *fakeWatchRepo) GetByFullName(_ context.Context, userID uint32, fullName string) (*RadarWatch, error) {
	w := f.find(userID, fullName)
	if w == nil {
		return nil, nil
	}
	cp := *w
	return &cp, nil
}

func (f *fakeWatchRepo) Upsert(_ context.Context, userID uint32, fullName string, baselineStars int64) (*RadarWatch, error) {
	if w := f.find(userID, fullName); w != nil {
		return w, nil
	}
	w := &RadarWatch{
		ID: f.nextID, FullName: fullName, BaselineStars: baselineStars,
		CreateTime: time.Now(), UpdateTime: time.Now(),
	}
	f.nextID++
	f.watches[userID] = append(f.watches[userID], w)
	return w, nil
}

func (f *fakeWatchRepo) Delete(_ context.Context, userID uint32, id uint32) (bool, error) {
	list := f.watches[userID]
	for i, w := range list {
		if w.ID == id {
			f.watches[userID] = append(list[:i], list[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeWatchRepo) UpdateStars(_ context.Context, userID uint32, fullName string, stars, delta int64, sparkline []int64) error {
	w := f.find(userID, fullName)
	if w == nil {
		return nil
	}
	w.Stars, w.StarsDelta, w.Sparkline = stars, delta, sparkline
	return nil
}

func (f *fakeWatchRepo) SetUnreadCount(_ context.Context, userID uint32, fullName string, n uint32) error {
	w := f.find(userID, fullName)
	if w == nil {
		return nil
	}
	w.UnreadCount = n
	return nil
}

type fakeEventRepo struct {
	events  map[uint32][]*RadarWatchEvent
	nextID  uint32
	readLog [][]uint32
}

func newFakeEventRepo() *fakeEventRepo {
	return &fakeEventRepo{events: map[uint32][]*RadarWatchEvent{}, nextID: 1}
}

func (f *fakeEventRepo) ListByUser(_ context.Context, userID uint32, unreadOnly bool) ([]*RadarWatchEvent, error) {
	out := make([]*RadarWatchEvent, 0)
	for _, e := range f.events[userID] {
		if unreadOnly && e.Read {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeEventRepo) CreateBatch(_ context.Context, userID uint32, events []*RadarWatchEvent) error {
	for _, e := range events {
		cp := *e
		cp.ID = f.nextID
		f.nextID++
		f.events[userID] = append(f.events[userID], &cp)
	}
	return nil
}

func (f *fakeEventRepo) MarkRead(_ context.Context, userID uint32, ids []uint32) (int, error) {
	f.readLog = append(f.readLog, ids)
	n := 0
	idSet := map[uint32]struct{}{}
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, e := range f.events[userID] {
		if _, ok := idSet[e.ID]; ok && !e.Read {
			e.Read = true
			n++
		}
	}
	return n, nil
}

func (f *fakeEventRepo) CountUnreadByRepo(_ context.Context, userID uint32, fullName string) (uint32, error) {
	var n uint32
	for _, e := range f.events[userID] {
		if e.FullName == fullName && !e.Read {
			n++
		}
	}
	return n, nil
}

func (f *fakeEventRepo) DeleteOlderThan(_ context.Context, _ time.Time) (int, error) {
	return 0, nil
}

func newWatchUsecase() (*RadarWatchUsecase, *fakeWatchRepo, *fakeEventRepo) {
	wr, er := newFakeWatchRepo(), newFakeEventRepo()
	uc := NewRadarWatchUsecase(wr, er, log.NewStdLogger(io.Discard))
	return uc, wr, er
}

func isBadRequest(err error) bool {
	return kratoserrors.Code(err) == 400
}

// ── normalizeFullName ──

func TestNormalizeFullName(t *testing.T) {
	valid := []string{"owner/repo", "a/b", "foo-bar/baz_qux.v2", "org2026/tool"}
	for _, name := range valid {
		if got, err := normalizeFullName(name); err != nil || got != name {
			t.Errorf("normalizeFullName(%q) = %q, %v; want %q, nil", name, got, err, name)
		}
	}

	invalid := []string{
		"", " ", "owner", "/repo", "owner/", "ow ner/repo", "owner/re po",
		"owner/.repo", "owner/repo.", "-a/b", "a/-b", "-/b", "a/-",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/repo", // owner 41 > 39
		"owner/" + string(make([]byte, 101)),                 // repo 101 > 100（内容无所谓，长度超了）
	}
	for _, name := range invalid {
		if _, err := normalizeFullName(name); err == nil {
			t.Errorf("normalizeFullName(%q) want error, got nil", name)
		} else if !isBadRequest(err) {
			t.Errorf("normalizeFullName(%q) err code = %d, want 400", name, kratoserrors.Code(err))
		}
	}
}

// ── Add / Remove ──

func TestWatchAddValidatesAndIdempotent(t *testing.T) {
	uc, _, _ := newWatchUsecase()
	ctx := context.Background()

	if _, err := uc.Add(ctx, 7, "not-a-repo"); err == nil {
		t.Fatal("Add with invalid full_name want error")
	}

	w1, err := uc.Add(ctx, 7, "golang/go")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	w2, err := uc.Add(ctx, 7, "golang/go")
	if err != nil {
		t.Fatalf("Add again: %v", err)
	}
	if w1.ID != w2.ID {
		t.Errorf("Add not idempotent: %d vs %d", w1.ID, w2.ID)
	}
}

func TestWatchPerUserIsolation(t *testing.T) {
	uc, _, _ := newWatchUsecase()
	ctx := context.Background()

	if _, err := uc.Add(ctx, 1, "a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Add(ctx, 2, "a/b"); err != nil {
		t.Fatal(err)
	}

	l1, _ := uc.List(ctx, 1)
	l2, _ := uc.List(ctx, 2)
	if len(l1) != 1 || len(l2) != 1 {
		t.Fatalf("isolation broken: user1=%d user2=%d", len(l1), len(l2))
	}

	ok, err := uc.Remove(ctx, 1, l1[0].ID)
	if err != nil || !ok {
		t.Fatalf("Remove user1: ok=%v err=%v", ok, err)
	}
	// user2 的盯盘不受影响
	l2, _ = uc.List(ctx, 2)
	if len(l2) != 1 {
		t.Errorf("user2 watch lost after user1 remove: %d", len(l2))
	}
	// 重复删除返回 false
	ok, _ = uc.Remove(ctx, 1, l1[0].ID)
	if ok {
		t.Error("re-remove want false")
	}
}

// ── MarkEventsRead ──

func TestMarkEventsReadEmptyIDsRejected(t *testing.T) {
	uc, _, _ := newWatchUsecase()
	if _, err := uc.MarkEventsRead(context.Background(), 1, nil); err == nil {
		t.Fatal("empty ids must be rejected (would otherwise mark ALL read)")
	} else if !isBadRequest(err) {
		t.Errorf("err code = %d, want 400", kratoserrors.Code(err))
	}
}

func TestMarkEventsReadOnlyGivenIDs(t *testing.T) {
	uc, wr, er := newWatchUsecase()
	ctx := context.Background()

	w, _ := wr.Upsert(ctx, 1, "a/b", 0)
	_ = er.CreateBatch(ctx, 1, []*RadarWatchEvent{{FullName: "a/b", Kind: "stars", Message: "+60"}})
	_ = er.CreateBatch(ctx, 1, []*RadarWatchEvent{{FullName: "a/b", Kind: "stars", Message: "+80"}})

	events, _ := uc.ListEvents(ctx, 1, true)
	if len(events) != 2 {
		t.Fatalf("unread want 2, got %d", len(events))
	}
	n, err := uc.MarkEventsRead(ctx, 1, []uint32{events[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("marked = %d, want 1", n)
	}
	unread, _ := uc.ListEvents(ctx, 1, true)
	if len(unread) != 1 {
		t.Errorf("unread after mark = %d, want 1", len(unread))
	}
	_ = w
}

// ── SyncStars 事件阈值 ──

func TestSyncStarsThresholdEvent(t *testing.T) {
	uc, wr, er := newWatchUsecase()
	ctx := context.Background()

	if _, err := wr.Upsert(ctx, 1, "a/b", 1000); err != nil {
		t.Fatal(err)
	}

	// 低于阈值（50）：不生成事件（基线 1000 → 1030，delta=30）
	if err := uc.SyncStars(ctx, 1, "a/b", 1030, []int64{30}, 50); err != nil {
		t.Fatal(err)
	}
	if n, _ := er.CountUnreadByRepo(ctx, 1, "a/b"); n != 0 {
		t.Errorf("no event want unread 0, got %d", n)
	}

	// 跨阈值：生成事件且未读数 +1（delta 100→200，增量 100 >= 50）
	if err := uc.SyncStars(ctx, 1, "a/b", 1200, []int64{30, 100, 200}, 50); err != nil {
		t.Fatal(err)
	}
	if n, _ := er.CountUnreadByRepo(ctx, 1, "a/b"); n != 1 {
		t.Errorf("threshold event want unread 1, got %d", n)
	}
	w := wr.find(1, "a/b")
	if w.UnreadCount != 1 || w.Stars != 1200 || w.StarsDelta != 200 {
		t.Errorf("watch state wrong: %+v", w)
	}

	// 对未盯盘仓库静默无操作
	if err := uc.SyncStars(ctx, 1, "x/y", 1, nil, 50); err != nil {
		t.Errorf("sync unwatched repo: %v", err)
	}
}

// ── Notify 用例 ──

func TestNotifyUsecase(t *testing.T) {
	er := newFakeEventRepo()
	uc := NewRadarNotifyUsecase(er)
	ctx := context.Background()

	if _, err := uc.MarkNotificationsRead(ctx, 1, nil); err == nil {
		t.Fatal("empty ids must be rejected")
	}
	if n, _ := uc.UnreadCount(ctx, 1); n != 0 {
		t.Errorf("unread want 0, got %d", n)
	}
	_ = er.CreateBatch(ctx, 1, []*RadarWatchEvent{{FullName: "a/b", Kind: "stars", Message: "+60"}})
	if n, _ := uc.UnreadCount(ctx, 1); n != 1 {
		t.Errorf("unread want 1, got %d", n)
	}
}

// ── Config 用例 ──

type fakeConfigRepo struct {
	saved *RadarUserConfigData
}

func (f *fakeConfigRepo) Get(_ context.Context, _ uint32) (*RadarUserConfigData, error) {
	return f.saved, nil
}

func (f *fakeConfigRepo) Save(_ context.Context, _ uint32, cfg *RadarUserConfigData) error {
	f.saved = cfg
	return nil
}

func TestConfigDefaultsAndNormalize(t *testing.T) {
	repo := &fakeConfigRepo{}
	uc := NewRadarConfigUsecase(repo, log.NewStdLogger(io.Discard))
	ctx := context.Background()

	cfg, err := uc.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.GitHubToken != "" || cfg.LLMEnabled {
		t.Errorf("default config wrong: %+v", cfg)
	}
	if cfg.LLMBaseURL != "https://api.openai.com/v1" || cfg.LLMModel != "gpt-4o-mini" {
		t.Errorf("default llm wrong: %q %q", cfg.LLMBaseURL, cfg.LLMModel)
	}

	out, err := uc.Set(ctx, &RadarUserConfigData{
		GitHubToken: "  ghp_xxxx  ", LLMEnabled: true,
		LLMBaseURL: " https://example.com ", LLMModel: " gpt-4 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.GitHubToken != "ghp_xxxx" || out.LLMBaseURL != "https://example.com" || out.LLMModel != "gpt-4" {
		t.Errorf("normalize wrong: %+v", out)
	}
}

