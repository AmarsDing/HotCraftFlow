package biz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 采集编排覆盖（ic_collect.go / ic_fetch.go 的共享四壁与基座）：
//   B-8 第 1-4 步顺序纪律（立即出 ID → 证据同事务 → 仅 RawMeta 发消息）
//   治理：S-10 双桶限流 → S-12 熔断 → robots 三态（S-7/S-8）
//   断言全部落在内存四壁上，真身 = 真实 IcCollectUsecase。
// =============================================================================

// ---------- 四壁替身 ----------

// collectSourceRepo 信源仓储：GetByID 真读，SetStatus/RecordRunOutcome 留证，其余封死。
type collectSourceRepo struct {
	byID         map[string]*IntelSourceRow
	statusWrites [][]string // (id, status)
	outcomes     []collectOutcomeWrite
}

type collectOutcomeWrite struct {
	id      string
	success bool
	errMsg  string
}

func (s *collectSourceRepo) GetByID(_ context.Context, id string) (*IntelSourceRow, error) {
	return s.byID[id], nil
}

func (s *collectSourceRepo) SetStatus(_ context.Context, id, status string, _ *time.Time) error {
	s.statusWrites = append(s.statusWrites, []string{id, status})
	if r := s.byID[id]; r != nil {
		r.Status = status
	}
	return nil
}

func (s *collectSourceRepo) RecordRunOutcome(_ context.Context, id string, success bool, errMsg string, _ time.Time, _ int32, _ time.Duration) error {
	s.outcomes = append(s.outcomes, collectOutcomeWrite{id, success, errMsg})
	return nil
}

func (s *collectSourceRepo) Create(context.Context, *IntelSourceCreate) (*IntelSourceRow, error) {
	panic("collect stub: not used")
}
func (s *collectSourceRepo) Update(context.Context, string, *IntelSourcePatch) (*IntelSourceRow, error) {
	panic("collect stub: not used")
}
func (s *collectSourceRepo) Delete(context.Context, string) (bool, error) { panic("collect stub: not used") }
func (s *collectSourceRepo) List(context.Context, *IntelSourceListQuery) ([]*IntelSourceRow, int, error) {
	panic("collect stub: not used")
}
func (s *collectSourceRepo) ListAllEnabled(context.Context) ([]*IntelSourceRow, error) {
	panic("collect stub: not used")
}
func (s *collectSourceRepo) Recompute7dCounts(context.Context, string, int32, int32) error {
	panic("collect stub: not used")
}

// collectIntelRepo 条目仓储：GetByID / UpsertWithEvidenceTx（dedup_key 幂等）真实现。
type collectIntelRepo struct {
	byID       map[string]*IntelItemRow
	byDedupKey map[string]*IntelItemRow
	upserts    []*IntelItemUpsert
	evidences  []*EvidenceSnapshotRow
	failUpsert bool
	failGet    bool
}

func (s *collectIntelRepo) GetByID(_ context.Context, id string) (*IntelItemRow, error) {
	if s.failGet {
		return nil, errors.New("db read down")
	}
	return s.byID[id], nil
}

// UpsertWithEvidenceTx 镜像生产语义：dedup_key 已占 → 返回既有行 ID + created=false（幂等重放）。
func (s *collectIntelRepo) UpsertWithEvidenceTx(_ context.Context, up *IntelItemUpsert, ev *EvidenceSnapshotUpsert) (string, bool, error) {
	if s.failUpsert {
		return "", false, errors.New("db write down")
	}
	if old, ok := s.byDedupKey[up.DedupKey]; ok {
		s.evidences = append(s.evidences, ev) // 证据 append-only：重放也留痕
		return old.IntelID, false, nil
	}
	row := &IntelItemRow{
		IntelItem:          up.Contract,
		SourceID:           up.SourceID,
		EvidenceID:         ev.ID,
		PublishState:       "pending",
		CycleBucket:        up.CycleBucket,
		DedupKey:           up.DedupKey,
		PublishedAtSuspect: up.PublishedAtSuspect,
		RawSize:            up.RawSize,
		RawTruncated:       up.RawTruncated,
	}
	s.byID[row.IntelID] = row
	s.byDedupKey[up.DedupKey] = row
	s.upserts = append(s.upserts, up)
	s.evidences = append(s.evidences, ev)
	return row.IntelID, true, nil
}

func (s *collectIntelRepo) GetByDedupKey(context.Context, string) (*IntelItemRow, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) FindRecentByURLKey(context.Context, string, string, time.Duration) (*IntelItemRow, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) FindSimHashCandidates(context.Context, [4]int16, time.Duration, int) ([]*IntelItemRow, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) List(context.Context, *IntelListQuery) ([]*IntelItemRow, int, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) ListByClusterID(context.Context, string, int) ([]*IntelItemRow, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) UpdateAfterNormalize(context.Context, *IntelNormalizePatch) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) UpdateDedupKey(context.Context, string, string, string) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) UpdateGate(context.Context, string, string, []string) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) UpdateMetrics(context.Context, string, *IntelMetricsPatch) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) UpdateEmbedding(context.Context, string, string) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) SupplementText(context.Context, string, string) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) SetPublishState(context.Context, string, string) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) ListPendingPublish(context.Context, int) ([]*IntelItemRow, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) AssignCluster(context.Context, []string, string) error {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) MoveClusterMembers(context.Context, string, string) (int, error) {
	panic("collect stub: not used")
}
func (s *collectIntelRepo) DeleteBefore(context.Context, time.Time, int) (int, error) {
	panic("collect stub: not used")
}

// collectRunRepo 运行账本：Create 发号 + Finish 留证（含失败注入）。
type collectRunRepo struct {
	seq        int
	created    [][]string // (runID, sourceID, trigger)
	finished   map[string]*CollectRunFinish
	failCreate bool
}

func (s *collectRunRepo) Create(_ context.Context, sourceID, trigger string, _ time.Time) (string, error) {
	if s.failCreate {
		return "", errors.New("db write down")
	}
	s.seq++
	id := fmt.Sprintf("run_%02d", s.seq)
	s.created = append(s.created, []string{id, sourceID, trigger})
	return id, nil
}

func (s *collectRunRepo) Finish(_ context.Context, id string, fin *CollectRunFinish) error {
	if s.finished == nil {
		s.finished = map[string]*CollectRunFinish{}
	}
	s.finished[id] = fin
	return nil
}

func (s *collectRunRepo) List(context.Context, string, int32, int32) ([]*CollectRunRow, int, error) {
	panic("collect stub: not used")
}
func (s *collectRunRepo) CountOutcomes(context.Context, string, time.Time) (int32, int32, error) {
	panic("collect stub: not used")
}

// collectStubCollector 可编程采集器：返回预置条目或错误，记录调用与被采信源。
type collectStubCollector struct {
	method  string
	items   []*RawCollectItem
	err     error
	calls   int
	lastSrc *IntelSourceRow
}

func (s *collectStubCollector) Method() string { return s.method }

func (s *collectStubCollector) Collect(_ context.Context, src *IntelSourceRow) ([]*RawCollectItem, error) {
	s.calls++
	s.lastSrc = src
	if s.err != nil {
		return nil, s.err
	}
	return s.items, nil
}

// collectRegistry：方法分发表（空表 = 未注册路径）。
type collectRegistry struct{ by map[string]Collector }

func (s *collectRegistry) Get(method string) (Collector, bool) {
	c, ok := s.by[method]
	return c, ok
}

func (s *collectRegistry) EnabledMethods() []string { panic("collect stub: not used") }

// collectStore 证据落盘：hash 发号、截断口径与生产一致；failOnContains 命中片段内容即故障。
type collectStore struct {
	calls          int
	fragments      []*contract.RawFragment
	sizes          []int64
	failOnContains string
	seq            int
}

func (s *collectStore) Save(_ context.Context, intelID string, frag *contract.RawFragment, maxBytes int64) (string, string, int64, bool, error) {
	s.calls++
	s.fragments = append(s.fragments, frag)
	if s.failOnContains != "" && strings.Contains(string(frag.Content), s.failOnContains) {
		return "", "", 0, false, errors.New("evidence disk full")
	}
	s.seq++
	size := int64(len(frag.Content))
	truncated := size > maxBytes
	if truncated {
		size = maxBytes
	}
	s.sizes = append(s.sizes, size)
	return fmt.Sprintf("raw/2026/10/%s.bin", intelID), strings.Repeat(fmt.Sprintf("%x", s.seq%10), 64), size, truncated, nil
}

func (s *collectStore) Delete(context.Context, string) error { panic("collect stub: not used") }
func (s *collectStore) Export(context.Context, string, map[string]string) (string, string, error) {
	panic("collect stub: not used")
}

// collectLimiter 双桶限流：denials/errKeys 按 key 编程，调用留证（key=perMin）。
type collectLimiter struct {
	denials map[string]bool
	errKeys map[string]bool
	calls   []string
}

func (s *collectLimiter) Allow(_ context.Context, key string, perMin int32) (bool, error) {
	s.calls = append(s.calls, fmt.Sprintf("%s=%d", key, perMin))
	if s.errKeys[key] {
		return false, errors.New("redis down")
	}
	if s.denials[key] {
		return false, nil
	}
	return true, nil
}

// collectRobots 三态口：全字段编程（err 非 nil 时生产走「未声明」分支继续）。
type collectRobots struct {
	allowed, found bool
	delay          int32
	err            error
	calls          []string // method|url|exempt
}

func (s *collectRobots) Check(_ context.Context, method, rawURL string, exempt bool) (bool, int32, bool, error) {
	s.calls = append(s.calls, fmt.Sprintf("%s|%s|%v", method, rawURL, exempt))
	return s.allowed, s.delay, s.found, s.err
}

// collectEvidenceRepo 链头口：Only LatestChainHash 被编排消费。
type collectEvidenceRepo struct {
	chain    string
	chainErr error
}

func (s *collectEvidenceRepo) LatestChainHash(context.Context, string) (string, error) {
	return s.chain, s.chainErr
}

func (s *collectEvidenceRepo) GetByID(context.Context, string) (*EvidenceSnapshotRow, error) {
	panic("collect stub: not used")
}
func (s *collectEvidenceRepo) GetLatestByIntelID(context.Context, string) (*EvidenceSnapshotRow, error) {
	panic("collect stub: not used")
}
func (s *collectEvidenceRepo) ListByIntelID(context.Context, string) ([]*EvidenceSnapshotRow, error) {
	panic("collect stub: not used")
}
func (s *collectEvidenceRepo) ListExpired(context.Context, time.Time, int) ([]*EvidenceSnapshotRow, error) {
	panic("collect stub: not used")
}
func (s *collectEvidenceRepo) DeleteIDs(context.Context, []string) (int, error) {
	panic("collect stub: not used")
}

// ---------- 测试基座 ----------

type collectFixture struct {
	u         *IcCollectUsecase
	cfg       *IcConfig
	sources   *collectSourceRepo
	intel     *collectIntelRepo
	runs      *collectRunRepo
	reg       *collectRegistry
	store     *collectStore
	limiter   *collectLimiter
	robots    *collectRobots
	pub       *normPublisher
	evRepo    *collectEvidenceRepo
	collector *collectStubCollector
}

// newCollectFixture 以 method 建全壁基座（出版器复用归一化的 normPublisher：记录 + failAll）。
func newCollectFixture(method string) *collectFixture {
	f := &collectFixture{cfg: ResolveIcConfig(nil)}
	f.sources = &collectSourceRepo{byID: map[string]*IntelSourceRow{}}
	f.intel = &collectIntelRepo{byID: map[string]*IntelItemRow{}, byDedupKey: map[string]*IntelItemRow{}}
	f.runs = &collectRunRepo{}
	f.collector = &collectStubCollector{method: method, items: []*RawCollectItem{collectTestItem("https://weibo.com/hot/1")}}
	f.reg = &collectRegistry{by: map[string]Collector{method: f.collector}}
	f.store = &collectStore{}
	f.limiter = &collectLimiter{denials: map[string]bool{}, errKeys: map[string]bool{}}
	f.robots = &collectRobots{allowed: true, found: true, delay: 500}
	f.pub = &normPublisher{failOn: map[string]bool{}}
	f.evRepo = &collectEvidenceRepo{}
	f.rebuild()
	return f
}

// rebuild 换壁后重装真身（同一份 cfg 指针，配改即生效）。
// 注：nil 端口必须装空接口——typed-nil 指针进接口后 usecase 的 == nil 卫拒不住。
func (f *collectFixture) rebuild() {
	var limiter RateLimiterPort
	if f.limiter != nil {
		limiter = f.limiter
	}
	var robots RobotsCheckerPort
	if f.robots != nil {
		robots = f.robots
	}
	f.u = NewIcCollectUsecase(f.cfg, f.sources, f.intel, f.runs, f.reg, f.store,
		limiter, robots, f.pub, f.evRepo, log.NewStdLogger(io.Discard))
}

// collectTestSource 合法信源（method 可变；治理字段全部默认）。
func collectTestSource(method string) *IntelSourceRow {
	return &IntelSourceRow{
		ID: "src_test", Platform: "weibo", Region: "cn", Kind: "hotlist",
		Method: method, Enabled: true, Status: "active", IntervalMinutes: 30,
	}
}

// collectTestItem 契约完整的原始条目：片段头混进敏感头验脱敏。
func collectTestItem(rawURL string) *RawCollectItem {
	return &RawCollectItem{
		SourceURL: rawURL, Title: "测试标题", Text: "正文内容若干字", Lang: "zh",
		Rank: 1, RankTotal: 50, Views: 1000,
		Fragment: &contract.RawFragment{
			RequestURL: rawURL, FinalURL: rawURL, HTTPStatus: 200,
			Content: []byte("raw-payload-bytes"),
			Headers: map[string]string{
				"Content-Type": "text/html; charset=utf-8",
				"Set-Cookie":   "sid=SECRET",
				"X-Trace":      "drop-me",
			},
		},
	}
}

// =============================================================================
// RunWithStats 编排门序：熔断 → 分发表 → 限流 → 开账 → 采集 → 逐条 B-8。
// =============================================================================

func TestCollectRunWithStats(t *testing.T) {
	ctx := context.Background()

	t.Run("信源不存在", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		runID, stats, err := f.u.RunWithStats(ctx, "src_ghost", "cron", false)
		if runID != "" || stats != nil {
			t.Fatalf("幽灵信源不得开账: runID=%q stats=%v", runID, stats)
		}
		if !errors.Is(err, ErrIcSourceNotFound) {
			t.Fatalf("哨兵口径: %v", err)
		}
	})

	t.Run("熔断窗内跳过（跳过非失败）", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		future := time.Now().Add(time.Hour)
		src := collectTestSource("hotlist")
		src.Status, src.PausedUntil = "paused", &future
		f.sources.byID[src.ID] = src
		runID, stats, err := f.u.RunWithStats(ctx, src.ID, "cron", false)
		if err != nil || runID != "" || stats != nil {
			t.Fatalf("窗内跳过应返回零值: runID=%q stats=%v err=%v", runID, stats, err)
		}
		if len(f.runs.created) != 0 || len(f.sources.outcomes) != 0 {
			t.Fatal("跳过轮不得开账/回执")
		}
	})

	t.Run("disabled 跳过", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		src.Enabled = false
		f.sources.byID[src.ID] = src
		if _, _, err := f.u.RunWithStats(ctx, src.ID, "cron", false); err != nil {
			t.Fatalf("disabled 是跳过不是错误: %v", err)
		}
		if f.collector.calls != 0 {
			t.Fatal("disabled 信源不得触达采集器")
		}
	})

	t.Run("熔断窗已过拨回 active", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		past := time.Now().Add(-time.Minute)
		src := collectTestSource("hotlist")
		src.Status, src.PausedUntil = "paused", &past
		f.sources.byID[src.ID] = src
		runID, stats, err := f.u.RunWithStats(ctx, src.ID, "manual", false)
		if err != nil || stats.Status != "succeeded" || runID == "" {
			t.Fatalf("窗过应恢复执行: runID=%q stats=%+v err=%v", runID, stats, err)
		}
		if len(f.sources.statusWrites) != 1 || f.sources.statusWrites[0][1] != "active" {
			t.Fatalf("S-12 恢复语义失守: %v", f.sources.statusWrites)
		}
	})

	t.Run("采集器未注册", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		src.Method = "crawl" // 注册表只有 hotlist
		f.sources.byID[src.ID] = src
		if _, _, err := f.u.RunWithStats(ctx, src.ID, "cron", false); !errors.Is(err, ErrIcCollectorUnavailable) {
			t.Fatalf("哨兵口径: %v", err)
		}
	})

	t.Run("平台桶拒给整轮跳过", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		f.limiter.denials["ic:platform:weibo"] = true
		runID, stats, err := f.u.RunWithStats(ctx, src.ID, "cron", false)
		if err != nil || runID != "" || stats != nil {
			t.Fatalf("限流跳过应返回零值: %v", err)
		}
		// 平台级 > 信源级：平台桶先打，被拒即止步，信源桶不再消费。
		if len(f.limiter.calls) != 1 || f.limiter.calls[0] != "ic:platform:weibo=20" {
			t.Fatalf("双桶顺序/默认速率口径: %v", f.limiter.calls)
		}
		if f.collector.calls != 0 {
			t.Fatal("限流拒绝不得触达采集器")
		}
	})

	t.Run("信源桶拒 + 自定义速率口径", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		src.RateLimitPerMin = 7
		f.sources.byID[src.ID] = src
		f.limiter.denials["ic:source:src_test"] = true
		if _, _, err := f.u.RunWithStats(ctx, src.ID, "cron", false); err != nil {
			t.Fatal(err)
		}
		if len(f.limiter.calls) != 2 || f.limiter.calls[1] != "ic:source:src_test=7" {
			t.Fatalf("信源桶速率应取信源配置: %v", f.limiter.calls)
		}
	})

	t.Run("限流器故障错误上抛不开账", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		f.limiter.errKeys["ic:platform:weibo"] = true
		if _, _, err := f.u.RunWithStats(ctx, src.ID, "cron", false); err == nil {
			t.Fatal("限流器故障（redis down）必须上抛")
		}
		if len(f.runs.created) != 0 {
			t.Fatal("限流失败不得开账")
		}
	})

	t.Run("运行开账失败", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		f.runs.failCreate = true
		if _, _, err := f.u.RunWithStats(ctx, src.ID, "cron", false); err == nil {
			t.Fatal("开账失败必须上抛")
		}
		if len(f.runs.finished) != 0 || len(f.sources.outcomes) != 0 {
			t.Fatal("开账失败不得关账/回执（defer 未登记）")
		}
	})

	t.Run("采集失败关账 failed 并熔断回执", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		f.collector.err = errors.New("upstream 504")
		runID, stats, err := f.u.RunWithStats(ctx, src.ID, "retry", false)
		if err == nil || runID == "" {
			t.Fatalf("采集失败: runID=%q err=%v", runID, err)
		}
		if stats.Status != "failed" || stats.ErrorCode != "IC_COLLECT_FAILED" {
			t.Fatalf("账本状态: %+v", stats)
		}
		if len(f.sources.outcomes) != 1 || f.sources.outcomes[0].success {
			t.Fatalf("S-12 熔断失败计次失守: %v", f.sources.outcomes)
		}
		if got := f.runs.finished[runID]; got == nil || got.Status != "failed" {
			t.Fatalf("run 关账缺失或状态错: %+v", got)
		}
	})

	t.Run("混合账本 created/duplicate/skipped 且仅新作发布", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		fragFail := collectTestItem("https://weibo.com/hot/2")
		fragFail.Fragment.Content = []byte("FAILME")
		f.store.failOnContains = "FAILME"
		f.collector.items = []*RawCollectItem{
			collectTestItem("https://weibo.com/hot/1"),
			collectTestItem("https://weibo.com/hot/1"), // 周期内重复 → dedup_key 命中
			fragFail,                                    // 证据落盘失败 → skipped
			collectTestItem("https://weibo.com/hot/3"),
		}
		runID, stats, err := f.u.RunWithStats(ctx, src.ID, "cron", false)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Fetched != 4 || stats.Normalized != 2 || stats.Merged != 1 || stats.Skipped != 1 {
			t.Fatalf("账本计数: %+v", stats)
		}
		if len(f.pub.calls) != 2 || len(f.intel.byID) != 2 {
			t.Fatalf("仅 created 发布/落库: pub=%v rows=%d", f.pub.calls, len(f.intel.byID))
		}
		if len(f.sources.outcomes) != 1 || !f.sources.outcomes[0].success {
			t.Fatal("成功回执缺失：熔断连击无法归零")
		}
		if got := f.runs.finished[runID]; got == nil || got.Normalized != 2 {
			t.Fatalf("run 关账口径: %+v", got)
		}
	})

	t.Run("force 绕过熔断与限流（人工补采 B-13）", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		src.Enabled = false // force 连 disabled 都让跑
		f.sources.byID[src.ID] = src
		f.limiter.denials["ic:platform:weibo"] = true
		runID, stats, err := f.u.RunWithStats(ctx, src.ID, "manual", true)
		if err != nil || stats == nil || stats.Status != "succeeded" || runID == "" {
			t.Fatalf("force 轮: runID=%q stats=%+v err=%v", runID, stats, err)
		}
		if len(f.limiter.calls) != 0 {
			t.Fatalf("force 不得消费限流桶: %v", f.limiter.calls)
		}
	})

	t.Run("无限流器直通", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		f.limiter = nil
		f.rebuild()
		if _, stats, err := f.u.RunWithStats(ctx, src.ID, "cron", false); err != nil || stats.Status != "succeeded" {
			t.Fatalf("nil 限流器应放行: stats=%+v err=%v", stats, err)
		}
	})

	t.Run("RunSource 为无统计代理", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		f.sources.byID[src.ID] = src
		runID, err := f.u.RunSource(ctx, src.ID, "cron")
		if err != nil || runID == "" {
			t.Fatalf("RunSource: runID=%q err=%v", runID, err)
		}
		if len(f.runs.created) != 1 {
			t.Fatal("cron 触发必须开账")
		}
	})
}

// =============================================================================
// persistRawItem：B-8 第 2-4 步的合同级断言（顺序写死不可换）。
// =============================================================================

func TestCollectPersistRawItem(t *testing.T) {
	ctx := context.Background()

	t.Run("首创全链：ID 立即生成 + 证据同事务 + 仅 RawMeta 发布，再重放幂等", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		item := collectTestItem("https://weibo.com/hot/1?utm_source=x")

		intelID, outcome, err := f.u.persistRawItem(ctx, src, item)
		if err != nil || outcome != rawOutcomeCreated {
			t.Fatalf("首创: outcome=%v err=%v", outcome, err)
		}
		if !strings.HasPrefix(intelID, "it_") || len(intelID) != 29 {
			t.Fatalf("intel_id 契约格式: %q", intelID)
		}
		if len(f.intel.upserts) != 1 {
			t.Fatalf("upsert 恰好一次: %d", len(f.intel.upserts))
		}
		up := f.intel.upserts[0]
		if up.SourceID != src.ID || up.URLKey == "" || up.DedupKey == "" || up.CycleBucket <= 0 {
			t.Fatalf("B-6 幂等键面: %+v", up)
		}
		if up.RawSize != int64(len("raw-payload-bytes")) || up.RawTruncated || up.PublishedAtSuspect {
			t.Fatalf("raw 面口径: %+v", up)
		}
		c := up.Contract
		if c.IntelID != intelID || c.Category != contract.CategoryBMaterial {
			t.Fatalf("契约初值: id=%s cat=%s", c.IntelID, c.Category)
		}
		if c.Compliance.Gate != contract.GateReview || len(c.Compliance.Reasons) != 1 {
			t.Fatalf("未判先封（B-16 方向初值纪律）失守: %+v", c.Compliance)
		}
		if c.Metrics.AsOf == nil || c.Source.Evidence.RawRef == "" {
			t.Fatal("观测面/证据引用缺失")
		}

		ev := f.intel.evidences[0]
		if ev.IntelID != intelID || ev.RawPath == "" || len(ev.RawHash) < 64 {
			t.Fatalf("证据落盘面: %+v", ev)
		}
		if ev.PrevHash != strings.Repeat("0", 64) || ev.ChainHash == "" || ev.ChainHash == ev.PrevHash || ev.ChainHash == ev.RawHash {
			t.Fatalf("B-17 链首/链哈希口径: prev=%q chain=%q", ev.PrevHash, ev.ChainHash)
		}
		if ev.HTTPStatus != 200 || ev.HTTPHeaders["Content-Type"] == "" {
			t.Fatalf("证据头留痕: %+v", ev.HTTPHeaders)
		}
		if _, bad := ev.HTTPHeaders["Set-Cookie"]; bad {
			t.Fatal("NFR-IC-06：Set-Cookie 必须脱敏")
		}
		if _, bad := ev.HTTPHeaders["X-Trace"]; bad {
			t.Fatal("白名单外头部必须丢弃")
		}
		if ev.RobotsChecked || ev.RobotsAllowed {
			t.Fatalf("hotlist 路 robots N/A 三态: %+v", ev)
		}
		if !ev.RetainUntil.After(time.Now().Add(89 * 24 * time.Hour)) {
			t.Fatalf("保留期口径（默认 90d）: %v", ev.RetainUntil)
		}

		// B-21：消息体只有 RawMeta（路径/哈希/链），原始字节永不进主题。
		if len(f.pub.calls) != 1 || f.pub.calls[0][0] != contract.SubjectIntelRaw || f.pub.calls[0][1] != intelID {
			t.Fatalf("发布面: %v", f.pub.calls)
		}

		// 幂等重放：同 URL 再落一次 → duplicate、指向既有行、不重复发布。
		again, outcome2, err := f.u.persistRawItem(ctx, src, item)
		if err != nil || outcome2 != rawOutcomeDuplicate || again != intelID {
			t.Fatalf("重放: id=%q outcome=%v err=%v", again, outcome2, err)
		}
		if len(f.intel.byID) != 1 || len(f.intel.evidences) != 2 {
			t.Fatalf("重放语义: rows=%d evidences=%d（证据 append-only 重放也留痕）", len(f.intel.byID), len(f.intel.evidences))
		}
		if len(f.pub.calls) != 1 {
			t.Fatalf("重复条目不得重复发布: %v", f.pub.calls)
		}
	})

	t.Run("crawl 路 robots 三态透传与故障面", func(t *testing.T) {
		f := newCollectFixture("crawl")
		src := collectTestSource("crawl")
		if _, _, err := f.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/a/1")); err != nil {
			t.Fatal(err)
		}
		ev := f.intel.evidences[0]
		if !ev.RobotsChecked || !ev.RobotsAllowed || ev.CrawlDelayMs != 500 {
			t.Fatalf("robots 三态透传: %+v", ev)
		}
		if len(f.robots.calls) != 1 {
			t.Fatalf("crawl 路必须查 robots: %v", f.robots.calls)
		}

		// robots 检查故障 → 三态全 false（未声明纪律），流程不中断。
		f2 := newCollectFixture("crawl")
		f2.robots.allowed, f2.robots.found, f2.robots.err = false, false, errors.New("robots fetch timeout")
		if _, _, err := f2.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/a/2")); err != nil {
			t.Fatalf("robots 故障不得中断采集: %v", err)
		}
		ev2 := f2.intel.evidences[0]
		if ev2.RobotsChecked || ev2.RobotsAllowed {
			t.Fatal("B-16 方向：取不到 robots 视为未声明（全 false）")
		}

		// robots 端口缺省 → N/A。
		f3 := newCollectFixture("crawl")
		f3.robots = nil
		f3.rebuild()
		if _, _, err := f3.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/a/3")); err != nil {
			t.Fatal(err)
		}
		if f3.intel.evidences[0].RobotsChecked {
			t.Fatal("nil robots 端口应 N/A")
		}
	})

	t.Run("RSS 路不查 robots", func(t *testing.T) {
		f := newCollectFixture("rss")
		src := collectTestSource("rss")
		if _, _, err := f.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/rss/1")); err != nil {
			t.Fatal(err)
		}
		if len(f.robots.calls) != 0 {
			t.Fatalf("S-7：hotlist/rss 不属 robots 路由: %v", f.robots.calls)
		}
	})

	t.Run("S-16 发布时间不可信标记且保留原值", func(t *testing.T) {
		f := newCollectFixture("rss")
		src := collectTestSource("rss")
		item := collectTestItem("https://weibo.com/rss/2")
		future := time.Now().Add(time.Hour) // 超出 +5m 容差
		item.PublishedAt = &future
		if _, _, err := f.u.persistRawItem(ctx, src, item); err != nil {
			t.Fatal(err)
		}
		up := f.intel.upserts[0]
		if !up.PublishedAtSuspect {
			t.Fatal("未来发布时间必须打 suspect")
		}
		if up.Contract.Source.PublishedAt != item.PublishedAt {
			t.Fatal("S-16：标记不篡改——原值必须保留")
		}
	})

	t.Run("超长原文截断标记", func(t *testing.T) {
		f := newCollectFixture("api")
		f.cfg.EvidenceRawMaxBytes = 16
		src := collectTestSource("api")
		item := collectTestItem("https://weibo.com/api/1")
		item.Fragment.Content = []byte(strings.Repeat("Z", 64))
		if _, _, err := f.u.persistRawItem(ctx, src, item); err != nil {
			t.Fatal(err)
		}
		up := f.intel.upserts[0]
		if !up.RawTruncated {
			t.Fatal("超 EvidenceRawMaxBytes 必须打 RawTruncated")
		}
		if up.RawSize != 16 {
			t.Fatalf("落库 size 以截断后为准: %d", up.RawSize)
		}
	})

	t.Run("无原文片段占位落盘（链完整优先）", func(t *testing.T) {
		f := newCollectFixture("sidecar")
		src := collectTestSource("sidecar")
		item := collectTestItem("https://weibo.com/sc/1")
		item.Fragment = nil
		if _, _, err := f.u.persistRawItem(ctx, src, item); err != nil {
			t.Fatal(err)
		}
		if f.store.calls != 1 || f.store.fragments[0] == nil {
			t.Fatal("占位片段必须照常落盘")
		}
		if f.store.fragments[0].RequestURL != item.SourceURL {
			t.Fatalf("占位片段 URL 留痕: %+v", f.store.fragments[0])
		}
	})

	t.Run("FinalURL 以片段为准", func(t *testing.T) {
		f := newCollectFixture("crawl")
		src := collectTestSource("crawl")
		item := collectTestItem("https://weibo.com/a/9")
		item.Fragment.FinalURL = "https://weibo.com/a/9#landed"
		if _, _, err := f.u.persistRawItem(ctx, src, item); err != nil {
			t.Fatal(err)
		}
		if f.intel.evidences[0].FinalURL != "https://weibo.com/a/9#landed" {
			t.Fatalf("证据 FinalURL 应跟随片段落地: %q", f.intel.evidences[0].FinalURL)
		}
	})

	t.Run("哈希链非首条 prev 透传", func(t *testing.T) {
		f := newCollectFixture("api")
		f.evRepo.chain = strings.Repeat("ab", 32)
		src := collectTestSource("api")
		if _, _, err := f.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/api/9")); err != nil {
			t.Fatal(err)
		}
		ev := f.intel.evidences[0]
		if ev.PrevHash != strings.Repeat("ab", 32) || ev.ChainHash == "" || ev.ChainHash == ev.PrevHash {
			t.Fatalf("B-17 链序: %+v", ev)
		}
	})

	t.Run("链头查询故障上抛", func(t *testing.T) {
		f := newCollectFixture("api")
		f.evRepo.chainErr = errors.New("db read down")
		src := collectTestSource("api")
		if _, _, err := f.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/api/10")); err == nil {
			t.Fatal("链头故障必须上抛")
		}
		if len(f.intel.upserts) != 0 || len(f.pub.calls) != 0 {
			t.Fatal("证据链断裂不得落库/发布")
		}
	})

	t.Run("证据落盘失败全链收手", func(t *testing.T) {
		f := newCollectFixture("api")
		f.store.failOnContains = "FAILME"
		src := collectTestSource("api")
		item := collectTestItem("https://weibo.com/api/11")
		item.Fragment.Content = []byte("FAILME")
		if _, _, err := f.u.persistRawItem(ctx, src, item); err == nil {
			t.Fatal("落盘失败必须上抛")
		}
		if len(f.intel.upserts) != 0 || len(f.pub.calls) != 0 {
			t.Fatal("B-8 第 3 步失败：第 4 步发布不得发生")
		}
	})

	t.Run("发布失败返回错误但行已落库（B-25 重拓扑补发）", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		f.pub.failAll = true
		src := collectTestSource("hotlist")
		intelID, _, err := f.u.persistRawItem(ctx, src, collectTestItem("https://weibo.com/hot/9"))
		if err == nil {
			t.Fatal("NATS 故障必须上抛")
		}
		if intelID != "" {
			t.Fatal("发布失败不返回条目 ID（重放由重拓扑认领）")
		}
		if len(f.intel.byID) != 1 {
			t.Fatal("B-25：行已落库待重发——不得丢数据")
		}
	})

	t.Run("URL 无法解析直接报错", func(t *testing.T) {
		f := newCollectFixture("hotlist")
		src := collectTestSource("hotlist")
		if _, _, err := f.u.persistRawItem(ctx, src, collectTestItem("https://bad host.com/x")); err == nil {
			t.Fatal("非法 URL 必须报错（死信由 worker 认领）")
		}
	})
}
