package biz

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/durationpb"

	"framework/app/admin/internal/conf"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 归一化管线（ic_normalize.go）套件级测试 + IcConfig 解析测试（S-35 覆盖率爬坡）
//
// 管线故意用「真身的真身」：真实 IcNormalizeUsecase + 真实 IcDedupUsecase +
// 真实 IcExtractUsecase + 真实 GovernanceGate，仅替换四壁（仓储/发布/短链），
// 断言落在同一条 Process 调用链上的端到端效果（S-35 按文件统计覆盖）。
// =============================================================================

// ---------- 归一化四壁替身 ----------

// normIntelRepo 复用黄金回放的内存仓储，仅放行 Process 触及的回写方法
// （黄金回放路径本身不调用它们，纪律不变：未被任何测试触及的方法保持 panic）。
type normIntelRepo struct {
	*goldenIntelRepo
	failDedupKey bool       // 模拟 dedup_key 唯一索引冲突（短链换键迟到的 L1 场景）
	gateWrites   [][]string // (intelID, gate)
	stateWrites  [][]string // (intelID, state)
	embedWrites  int
	normPatches  []*IntelNormalizePatch
	dedupKeySets int
}

func (s *normIntelRepo) UpdateAfterNormalize(_ context.Context, p *IntelNormalizePatch) error {
	r := s.byID[p.IntelID]
	if r == nil {
		return errors.New("normalize patch: intel not found")
	}
	if p.URLKey != nil {
		r.Dedup.URLKey = *p.URLKey
	}
	if p.FingerprintHex != nil {
		r.Dedup.Fingerprint = *p.FingerprintHex
	}
	r.FingerprintValid = p.FingerprintValid
	r.FpSegments = p.FpSegments
	r.Entities = p.Entities
	r.Category = contract.Category(p.Category)
	r.Workability = contract.IntelWorkability{Score: p.WorkabilityScore, Reason: p.WorkabilityReason}
	r.Payload.Title = p.Title
	r.Payload.Text = p.Text
	r.Payload.Lang = p.Lang
	s.normPatches = append(s.normPatches, p)
	return nil
}

func (s *normIntelRepo) UpdateGate(_ context.Context, id, gate string, _ []string) error {
	s.gateWrites = append(s.gateWrites, []string{id, gate})
	if r := s.byID[id]; r != nil {
		r.Compliance.Gate = contract.Gate(gate)
	}
	return nil
}

func (s *normIntelRepo) UpdateEmbedding(_ context.Context, id, embeddingID string) error {
	s.embedWrites++
	if r := s.byID[id]; r != nil {
		r.EmbeddingID = embeddingID
	}
	return nil
}

func (s *normIntelRepo) UpdateDedupKey(_ context.Context, id, urlKey, dedupKey string) error {
	if s.failDedupKey {
		return errors.New("duplicate dedup_key (unique index)")
	}
	s.dedupKeySets++
	if r := s.byID[id]; r != nil {
		r.Dedup.URLKey = urlKey
		r.DedupKey = dedupKey
	}
	return nil
}

func (s *normIntelRepo) SetPublishState(_ context.Context, id, state string) error {
	s.stateWrites = append(s.stateWrites, []string{id, state})
	if r := s.byID[id]; r != nil {
		r.PublishState = state
	}
	return nil
}

func (s *normIntelRepo) ListPendingPublish(_ context.Context, limit int) ([]*IntelItemRow, error) {
	out := make([]*IntelItemRow, 0, 4)
	for _, r := range s.rows {
		if r.PublishState == "pending" {
			out = append(out, r)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// normSourceRepo：buildGateContext 只读 GetByID。
type normSourceRepo struct{ byID map[string]*IntelSourceRow }

func (s *normSourceRepo) GetByID(_ context.Context, id string) (*IntelSourceRow, error) {
	return s.byID[id], nil
}

func (s *normSourceRepo) Create(context.Context, *IntelSourceCreate) (*IntelSourceRow, error) {
	panic("norm stub: not used")
}
func (s *normSourceRepo) Update(context.Context, string, *IntelSourcePatch) (*IntelSourceRow, error) {
	panic("norm stub: not used")
}
func (s *normSourceRepo) Delete(context.Context, string) (bool, error) { panic("norm stub: not used") }
func (s *normSourceRepo) List(context.Context, *IntelSourceListQuery) ([]*IntelSourceRow, int, error) {
	panic("norm stub: not used")
}
func (s *normSourceRepo) ListAllEnabled(context.Context) ([]*IntelSourceRow, error) {
	panic("norm stub: not used")
}
func (s *normSourceRepo) SetStatus(context.Context, string, string, *time.Time) error {
	panic("norm stub: not used")
}
func (s *normSourceRepo) RecordRunOutcome(context.Context, string, bool, string, time.Time, int32, time.Duration) error {
	panic("norm stub: not used")
}
func (s *normSourceRepo) Recompute7dCounts(context.Context, string, int32, int32) error {
	panic("norm stub: not used")
}

// normEvidenceRepo：buildGateContext 只读 GetLatestByIntelID。
type normEvidenceRepo struct{ byIntel map[string]*EvidenceSnapshotRow }

func (s *normEvidenceRepo) GetLatestByIntelID(_ context.Context, intelID string) (*EvidenceSnapshotRow, error) {
	return s.byIntel[intelID], nil
}

func (s *normEvidenceRepo) GetByID(context.Context, string) (*EvidenceSnapshotRow, error) {
	panic("norm stub: not used")
}
func (s *normEvidenceRepo) ListByIntelID(context.Context, string) ([]*EvidenceSnapshotRow, error) {
	panic("norm stub: not used")
}
func (s *normEvidenceRepo) LatestChainHash(context.Context, string) (string, error) {
	panic("norm stub: not used")
}
func (s *normEvidenceRepo) ListExpired(context.Context, time.Time, int) ([]*EvidenceSnapshotRow, error) {
	panic("norm stub: not used")
}
func (s *normEvidenceRepo) DeleteIDs(context.Context, []string) (int, error) {
	panic("norm stub: not used")
}

// normPublisher：记录发布主题；failOn 命中的 intelID 模拟 NATS 抖动；failAll 全拒
//（采集编排发布失败路径——msgID 由生产侧实时生成，测试无法预知）。
type normPublisher struct {
	calls   [][]string // (subject, intelID)
	failOn  map[string]bool
	failAll bool
}

func (s *normPublisher) PublishIntel(_ context.Context, subject string, _ *contract.EventEnvelope, msgID string) error {
	if s.failAll || s.failOn[msgID] {
		return errors.New("nats unavailable")
	}
	s.calls = append(s.calls, []string{subject, msgID})
	return nil
}

// normShortlink：可编程短链展开。
type normShortlink struct {
	finalByURL map[string]string
	err        error
}

func (s *normShortlink) Expand(_ context.Context, rawURL string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if f, ok := s.finalByURL[rawURL]; ok {
		return f, nil
	}
	return rawURL, nil
}

// ---------- 测试基座 ----------

var normAt = time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)

// normTestRow 造一条契约完整（Validate 可过）的原始条目，等价于采集落库后的读回。
func normTestRow(t *testing.T, url, sourceID string, method contract.CollectMethod, title, text string) *IntelItemRow {
	t.Helper()
	urlKey, canonical, err := NormalizeURLKey(url)
	if err != nil {
		t.Fatalf("normalize url: %v", err)
	}
	row := &IntelItemRow{SourceID: sourceID, PublishState: "pending"}
	row.IntelID = contract.NewIntelItemID() // 契约纪律：it_ + 26 位 ULID（Validate 硬卡）
	row.APIVersion = "1.0"
	row.Source.Platform = "weibo"
	row.Source.Region = contract.RegionCN
	row.Source.CollectMethod = method
	row.Source.URL = canonical
	row.Source.CollectedAt = normAt
	row.Payload.Title = title
	row.Payload.Text = text
	row.Payload.Lang = "" // 交给 S-25 判定
	row.Dedup.URLKey = urlKey
	row.DedupKey = ComputeDedupKey(urlKey, sourceID, 0)
	return row
}

type normFixture struct {
	intel     *normIntelRepo
	publisher *normPublisher
	uc        *IcNormalizeUsecase
}

func newNormalizeForTest(intel *normIntelRepo, shortlink ShortlinkExpanderPort, sources map[string]*IntelSourceRow, evidence map[string]*EvidenceSnapshotRow) *normFixture {
	logger := log.NewStdLogger(io.Discard)
	cfg := ResolveIcConfig(nil)
	clusters := newGoldenClusterRepo(intel.goldenIntelRepo)
	samples := &goldenSampleRepo{}
	dedup := NewIcDedupUsecase(intel.goldenIntelRepo, clusters, samples, nil, cfg, logger)
	extract := NewIcExtractUsecase(cfg, nil, logger)
	gate := newGateForTest(&stubRuleRepo{}, &stubAuthzRepo{})
	pub := &normPublisher{failOn: map[string]bool{}}
	uc := NewIcNormalizeUsecase(cfg, intel, &normSourceRepo{byID: sources}, &normEvidenceRepo{byIntel: evidence}, shortlink, extract, dedup, gate, pub, logger)
	return &normFixture{intel: intel, publisher: pub, uc: uc}
}

// TestNormalizeProcess 归一化主链路（评审 B-8 第 5-8 步）分场景回放。
func TestNormalizeProcess(t *testing.T) {
	entries := loadGoldenEntries(t)
	zh := entries[0] // g01：长中文正文，L2 指纹有效

	t.Run("gate pass → 正常发布 normalized", func(t *testing.T) {
		intel := &normIntelRepo{goldenIntelRepo: newGoldenIntelRepo()}
		row := normTestRow(t, zh.URL, "src-hot", contract.CollectHotlist, zh.Title, zh.Text)
		intel.add(row)
		fx := newNormalizeForTest(intel, nil, nil, nil)

		res, err := fx.uc.Process(context.Background(), &contract.RawIntelNotice{IntelID: row.IntelID}, "trace-01")
		if err != nil {
			t.Fatalf("process: %v", err)
		}
		if res == nil || res.Gate != "pass" {
			t.Fatalf("gate=%v, want pass", res)
		}
		if !res.Published {
			t.Fatal("publish 应成功（隔离四壁无故障）")
		}
		if res.ClusterID == "" {
			t.Fatal("首条应新建簇")
		}
		// S-25 语言判定 + L4 提取 + L2 指纹落库
		if row.Payload.Lang != "zh" {
			t.Fatalf("lang=%q, want zh（S-25 Unicode 判定）", row.Payload.Lang)
		}
		if row.Category == "" {
			t.Fatal("category 为空（L4 classify 未落库）")
		}
		if !row.FingerprintValid {
			t.Fatal("长文本应产出有效 SimHash（B-9）")
		}
		// 落库顺序物：normalize patch + gate 列 + publish state
		if len(intel.normPatches) != 1 {
			t.Fatalf("UpdateAfterNormalize 调用 %d 次 ≠ 1", len(intel.normPatches))
		}
		if len(intel.gateWrites) != 1 || intel.gateWrites[0][1] != "pass" {
			t.Fatalf("gate 回写异常: %v", intel.gateWrites)
		}
		if len(intel.stateWrites) != 1 || intel.stateWrites[0][1] != "published" {
			t.Fatalf("publish_state 回写异常: %v", intel.stateWrites)
		}
		// 发布主题与幂等键契约
		if len(fx.publisher.calls) != 1 || fx.publisher.calls[0][0] != contract.SubjectIntelNormalized {
			t.Fatalf("发布主题异常: %v", fx.publisher.calls)
		}
		if fx.publisher.calls[0][1] != row.IntelID {
			t.Fatalf("幂等键应为 intel_id: %v", fx.publisher.calls[0])
		}
	})

	t.Run("gate block → 发布 blocked 主题且仍落 published", func(t *testing.T) {
		intel := &normIntelRepo{goldenIntelRepo: newGoldenIntelRepo()}
		row := normTestRow(t, zh.URL, "src-crawl", contract.CollectCrawl, zh.Title, zh.Text)
		intel.add(row)
		// 取证快照：robots 已查且 disallow（S-7 → B-16 block）
		ev := map[string]*EvidenceSnapshotRow{row.IntelID: {RobotsChecked: true, RobotsAllowed: false}}
		fx := newNormalizeForTest(intel, nil, nil, ev)

		res, err := fx.uc.Process(context.Background(), &contract.RawIntelNotice{IntelID: row.IntelID}, "trace-02")
		if err != nil {
			t.Fatalf("process: %v", err)
		}
		if res.Gate != "block" {
			t.Fatalf("gate=%s, want block（robots disallow）", res.Gate)
		}
		if len(fx.publisher.calls) != 1 || fx.publisher.calls[0][0] != contract.SubjectIntelBlocked {
			t.Fatalf("block 条目必须走 blocked 主题: %v", fx.publisher.calls)
		}
		// block 也入簇留档（闸门在去重之前，审计可聚类同款来源）
		if res.ClusterID == "" {
			t.Fatal("block 条目亦应入簇留档")
		}
	})

	t.Run("短链换键冲突 → ForceL1Merge 并入先占簇", func(t *testing.T) {
		intel := &normIntelRepo{goldenIntelRepo: newGoldenIntelRepo()}
		fx := newNormalizeForTest(intel, nil, nil, nil)
		ctx := context.Background()

		// 先占者：最终 URL 已归一化入簇
		peer := normTestRow(t, "https://example.com/news/hot-123", "src-hot", contract.CollectHotlist, zh.Title, zh.Text)
		simPeer := ComputeSimHash(peer.Payload.Title, peer.Payload.Text, "zh", fx.uc.idf)
		peer.FingerprintValid = simPeer.Valid
		peer.FpSegments = simPeer.Segments
		peer.Dedup.Fingerprint = simPeer.Hex
		intel.add(peer)
		out, err := NewIcDedupUsecase(intel.goldenIntelRepo, newGoldenClusterRepo(intel.goldenIntelRepo), &goldenSampleRepo{}, nil, ResolveIcConfig(nil), log.NewStdLogger(io.Discard)).Resolve(ctx, peer, &simPeer)
		_ = out
		_ = err
		// 用固件里的 dedup 实例保证同一仓储视图
		peerOut, err := fx.uc.dedup.Resolve(ctx, peer, &simPeer)
		if err != nil {
			t.Fatalf("peer resolve: %v", err)
		}
		peerCluster := peerOut.ClusterID

		// 迟到的短链采样：同文不同壳（t.co 包装）
		intel.failDedupKey = true // dedup_key 唯一索引已被先占行命中
		cur := normTestRow(t, "https://t.co/abc123", "src-hot", contract.CollectHotlist, zh.Title, zh.Text)
		intel.add(cur)
		fx2 := newNormalizeForTest(intel, &normShortlink{finalByURL: map[string]string{
			"https://t.co/abc123": "https://example.com/news/hot-123",
		}}, nil, nil)

		res, err := fx2.uc.Process(ctx, &contract.RawIntelNotice{IntelID: cur.IntelID}, "trace-03")
		if err != nil {
			t.Fatalf("process: %v", err)
		}
		if res.ClusterID != peerCluster {
			t.Fatalf("短链展开撞上既有 url_key 应强制 L1 并入先占簇 %s，实际 %s", peerCluster, res.ClusterID)
		}
		if cur.Dedup.ClusterID != peerCluster {
			t.Fatalf("行级 cluster_id 未跟随强并: %s", cur.Dedup.ClusterID)
		}
		// 换键冲突场景不得把 url_key 回写成本行（键属于先占者）
		if len(intel.normPatches) != 1 || intel.normPatches[0].URLKey != nil {
			t.Fatalf("冲突路径不应回写 url_key: %+v", intel.normPatches[0].URLKey)
		}
	})

	t.Run("nats 抖动 → Published=false 且不回写 publish_state（B-25）", func(t *testing.T) {
		intel := &normIntelRepo{goldenIntelRepo: newGoldenIntelRepo()}
		row := normTestRow(t, zh.URL, "src-hot", contract.CollectHotlist, zh.Title, zh.Text)
		intel.add(row)
		fx := newNormalizeForTest(intel, nil, nil, nil)
		fx.publisher.failOn[row.IntelID] = true

		res, err := fx.uc.Process(context.Background(), &contract.RawIntelNotice{IntelID: row.IntelID}, "trace-04")
		if err != nil {
			t.Fatalf("publish 失败不得上抛（republish 打捞口径）: %v", err)
		}
		if res.Published {
			t.Fatal("nats 故障时 Published 必须为 false")
		}
		if len(intel.stateWrites) != 0 {
			t.Fatalf("发布未成功不得写 publish_state: %v", intel.stateWrites)
		}
	})

	t.Run("条目不存在 → nil,nil 由 worker 直接 ack", func(t *testing.T) {
		intel := &normIntelRepo{goldenIntelRepo: newGoldenIntelRepo()}
		fx := newNormalizeForTest(intel, nil, nil, nil)
		res, err := fx.uc.Process(context.Background(), &contract.RawIntelNotice{IntelID: "it_ghost"}, "trace-05")
		if err != nil || res != nil {
			t.Fatalf("幽灵条目应静默放行: res=%v err=%v", res, err)
		}
	})
}

// TestNormalizeRepublishPending B-25 打捞：pending 且归一化完整的条目补发。
func TestNormalizeRepublishPending(t *testing.T) {
	entries := loadGoldenEntries(t)
	zh := entries[0]
	idf := LoadIDFTable("")

	intel := &normIntelRepo{goldenIntelRepo: newGoldenIntelRepo()}
	fx := newNormalizeForTest(intel, nil, nil, nil)

	mkRow := func(id string, validFP bool, state string) *IntelItemRow {
		row := normTestRow(t, zh.URL+"/"+id, "src-hot", contract.CollectHotlist, zh.Title, zh.Text)
		if validFP {
			row.FingerprintValid = true
			row.Dedup.Fingerprint = ComputeSimHash(zh.Title, zh.Text, "zh", idf).Hex
			row.Category = contract.CategoryAHot
			row.Compliance.Gate = contract.GatePass
		}
		row.PublishState = state
		intel.add(row)
		return row
	}

	complete := mkRow("rp01", true, "pending")      // 应补发
	incomplete := mkRow("rp02", false, "pending")   // 半成品：跳过，交给重试链路
	already := mkRow("rp03", true, "published")     // 不在 pending 集合
	flaky := mkRow("rp04", true, "pending")         // 补发再抖动：跳过但不中断
	fx.publisher.failOn[flaky.IntelID] = true

	n, err := fx.uc.RepublishPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if n != 1 {
		t.Fatalf("补发成功数 %d ≠ 1（完整 pending 只有 rp01）", n)
	}
	if complete.PublishState != "published" {
		t.Fatalf("补发后状态: %s", complete.PublishState)
	}
	if incomplete.PublishState != "pending" || already.PublishState != "published" || flaky.PublishState != "pending" {
		t.Fatalf("三类行状态被误改: %s/%s/%s", incomplete.PublishState, already.PublishState, flaky.PublishState)
	}
	if len(fx.publisher.calls) != 1 || fx.publisher.calls[0][0] != contract.SubjectIntelNormalized {
		t.Fatalf("补发发布记录异常: %v", fx.publisher.calls)
	}
}

// =============================================================================
// IcConfig 解析（ResolveIcConfig：零值兜底 + proto 全覆盖覆写）
// =============================================================================

func TestIcConfigResolveDefaults(t *testing.T) {
	cfg := ResolveIcConfig(nil)
	if !cfg.Enabled || cfg.Concurrency != 8 || cfg.SimhashThreshold != 3 ||
		cfg.EmbeddingThreshold != 0.86 || cfg.DedupMergeWindow != 72*time.Hour {
		t.Fatalf("默认基线漂移: %+v", cfg)
	}
	if cfg.EmbeddingStoreEnabled {
		t.Fatal("缺省 store 未配应视为关闭（B-7 跳 L3）")
	}
	if cfg.IDFPath == "" || cfg.RobotsUserAgent == "" || cfg.RetentionSweepCron == "" {
		t.Fatalf("非 proto 开放字段必须有内置基线: %+v", cfg)
	}
}

func TestIcConfigResolveOverrides(t *testing.T) {
	bc := &conf.Bootstrap{Ic: &conf.Ic{
		Enabled:             true,
		Concurrency:         16,
		WorkabilityMinScore: 0.7,
		RawQueueCapacity:    9000,
		Dedup: &conf.Ic_Dedup{
			SimhashThreshold:     4,
			EmbeddingThreshold:   0.9,
			EmbeddingWindowHours: 12,
			MergeWindowHours:     120,
			Embedding:            &conf.Ic_Dedup_Embedding{Store: "pgvector", DailyCap: 8000},
		},
		Evidence:  &conf.Ic_Evidence{StorageDir: "/data/ev", RawMaxBytes: 1 << 20, RetainDays: 30},
		RateLimit: &conf.Ic_RateLimit{DefaultPerMin: 60, BackoffMax: durationpb.New(5 * time.Minute)},
		Breaker:   &conf.Ic_Breaker{FailThreshold: 3, Cooldown: durationpb.New(10 * time.Minute)},
		Robots:    &conf.Ic_Robots{UserAgent: "TestBot", Timeout: durationpb.New(3 * time.Second), CacheTtl: durationpb.New(time.Hour)},
		Shortlink: &conf.Ic_Shortlink{MaxRedirects: 3},
		Retention: &conf.Ic_Retention{IntelDays: 180, ClusterIdleCloseHours: 24, SweepCron: "0 4 * * *"},
		Llm:       &conf.Ic_Llm{Enabled: true, FallbackToRule: false},
	}}
	cfg := ResolveIcConfig(bc)

	if cfg.Concurrency != 16 || cfg.WorkabilityMinScore != 0.7 || cfg.RawQueueCapacity != 9000 {
		t.Fatalf("顶层字段覆写失败: %+v", cfg)
	}
	if cfg.SimhashThreshold != 4 || cfg.EmbeddingThreshold != 0.9 || cfg.EmbeddingWindowHours != 12 || cfg.DedupMergeWindow != 120*time.Hour {
		t.Fatalf("dedup 覆写失败: %+v", cfg)
	}
	if !cfg.EmbeddingStoreEnabled || cfg.EmbeddingDailyCap != 8000 {
		t.Fatalf("embedding store/cap 覆写失败: %+v", cfg)
	}
	if cfg.EvidenceStorageDir != "/data/ev" || cfg.EvidenceRawMaxBytes != 1<<20 || cfg.EvidenceRetainDays != 30 {
		t.Fatalf("evidence 覆写失败: %+v", cfg)
	}
	if cfg.RateLimitDefaultPerMin != 60 || cfg.RateLimitBackoffMax != 5*time.Minute {
		t.Fatalf("rate limit 覆写失败: %+v", cfg)
	}
	if cfg.BreakerFailThreshold != 3 || cfg.BreakerCooldown != 10*time.Minute {
		t.Fatalf("breaker 覆写失败: %+v", cfg)
	}
	if cfg.RobotsUserAgent != "TestBot" || cfg.RobotsTimeout != 3*time.Second || cfg.RobotsTTL != time.Hour {
		t.Fatalf("robots 覆写失败: %+v", cfg)
	}
	if cfg.ShortlinkMaxRedirects != 3 {
		t.Fatalf("shortlink 覆写失败: %+v", cfg)
	}
	if cfg.RetentionIntelDays != 180 || cfg.RetentionClusterIdleHours != 24 || cfg.RetentionSweepCron != "0 4 * * *" {
		t.Fatalf("retention 覆写失败: %+v", cfg)
	}
	if !cfg.LLMEnabled || cfg.LLMFallbackToRule {
		t.Fatalf("llm 覆写失败: %+v", cfg)
	}

	// 零值不覆写（只给部分字段，其余保持默认基线）
	partial := ResolveIcConfig(&conf.Bootstrap{Ic: &conf.Ic{Concurrency: 20}})
	if partial.Concurrency != 20 || partial.SimhashThreshold != 3 || partial.EvidenceRetainDays != 90 {
		t.Fatalf("零值守卫失效: %+v", partial)
	}
	// store=none 视为关闭（与缺省同义）
	none := ResolveIcConfig(&conf.Bootstrap{Ic: &conf.Ic{Dedup: &conf.Ic_Dedup{Embedding: &conf.Ic_Dedup_Embedding{Store: "none"}}}})
	if none.EmbeddingStoreEnabled {
		t.Fatal("store=none 须判定为关闭")
	}
}
