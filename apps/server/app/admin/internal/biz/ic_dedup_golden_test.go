package biz

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// M-27 去重黄金集回放（dev 评审：「回放 100 条真实条目快照」的可执行骨架）
//
// 语料 testdata/ic/dedup_golden.jsonl 每行一条：
//
//	{"id":"g07","cluster":"C2","expect":"new|merge","level":"L1|L2",
//	 "no_fp":false,"url":"...","title":"...","text":"...","lang":"zh|en","note":"..."}
//
// 设计约束：
//   - expect=new  必须是该 cluster 标签的首次出现 → Resolve 应新建簇；
//   - expect=merge 必须引用已出现标签 → Resolve 应并入同一簇（跨标签并簇即误并）；
//   - 覆盖 L1（URL 归一等价类）与 L2（NFKC 标点/前缀后缀样板/同义改写/语序/数字改写）；
//   - 12 条负例两两隔离：任意不同标签的有效指纹对汉明必须 > 阈值（防跨簇误并回归）；
//   - L3 关闭（embedder=nil），测试自包含、确定性、无网络。
//
// 真实快照就位时：同构导出线上样本追加进 JSONL，此测试不变。
// =============================================================================

// goldenEntry 黄金集一行。
type goldenEntry struct {
	ID      string `json:"id"`
	Cluster string `json:"cluster"`
	Expect  string `json:"expect"`
	Level   string `json:"level"`
	NoFP    bool   `json:"no_fp"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Text    string `json:"text"`
	Lang    string `json:"lang"`
	Note    string `json:"note"`
}

func loadGoldenEntries(t *testing.T) []goldenEntry {
	t.Helper()
	f, err := os.Open("testdata/ic/dedup_golden.jsonl")
	if err != nil {
		t.Fatalf("open golden corpus: %v", err)
	}
	defer f.Close()
	var entries []goldenEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var e goldenEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("golden line %d invalid JSON: %v", line, err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan golden corpus: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("golden corpus is empty")
	}
	return entries
}

// ---------- 全内存回放替身（house stub 纪律：Replay 触及不到的方法一律 panic） ----------

type goldenIntelRepo struct {
	rows []*IntelItemRow
	byID map[string]*IntelItemRow
}

func newGoldenIntelRepo() *goldenIntelRepo {
	return &goldenIntelRepo{byID: map[string]*IntelItemRow{}}
}

func (s *goldenIntelRepo) add(row *IntelItemRow) {
	s.rows = append(s.rows, row)
	s.byID[row.IntelID] = row
}

func (s *goldenIntelRepo) GetByID(_ context.Context, id string) (*IntelItemRow, error) {
	return s.byID[id], nil
}

// FindRecentByURLKey 镜像修正后 SQL：排除自身 + 逆序首命中（窗口内最新）。
func (s *goldenIntelRepo) FindRecentByURLKey(_ context.Context, selfID, urlKey string, _ time.Duration) (*IntelItemRow, error) {
	for i := len(s.rows) - 1; i >= 0; i-- {
		if s.rows[i].IntelID == selfID {
			continue
		}
		if s.rows[i].Dedup.URLKey == urlKey {
			return s.rows[i], nil
		}
	}
	return nil, nil
}

// FindSimHashCandidates B-26 pigeonhole：四段任一等值即候选（仅有效指纹入倒排）。
func (s *goldenIntelRepo) FindSimHashCandidates(_ context.Context, segs [4]int16, _ time.Duration, limit int) ([]*IntelItemRow, error) {
	out := make([]*IntelItemRow, 0, 8)
	for _, r := range s.rows {
		if !r.FingerprintValid || r.Dedup.Fingerprint == "" {
			continue
		}
		for i := 0; i < 4; i++ {
			if r.FpSegments[i] == segs[i] {
				out = append(out, r)
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *goldenIntelRepo) AssignCluster(_ context.Context, intelIDs []string, clusterID string) error {
	for _, id := range intelIDs {
		if r, ok := s.byID[id]; ok {
			r.Dedup.ClusterID = clusterID
		}
	}
	return nil
}

// —— 以下方法 Replay 路径不触发，panic 防静默误用 ——

func (s *goldenIntelRepo) GetByDedupKey(context.Context, string) (*IntelItemRow, error) {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) List(context.Context, *IntelListQuery) ([]*IntelItemRow, int, error) {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) ListByClusterID(context.Context, string, int) ([]*IntelItemRow, error) {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) UpsertWithEvidenceTx(context.Context, *IntelItemUpsert, *EvidenceSnapshotUpsert) (string, bool, error) {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) UpdateAfterNormalize(context.Context, *IntelNormalizePatch) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) UpdateDedupKey(context.Context, string, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) UpdateGate(context.Context, string, string, []string) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) UpdateMetrics(context.Context, string, *IntelMetricsPatch) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) UpdateEmbedding(context.Context, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) SupplementText(context.Context, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) SetPublishState(context.Context, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) ListPendingPublish(context.Context, int) ([]*IntelItemRow, error) {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) MoveClusterMembers(context.Context, string, string) (int, error) {
	panic("golden stub: not used")
}
func (s *goldenIntelRepo) DeleteBefore(context.Context, time.Time, int) (int, error) {
	panic("golden stub: not used")
}

type goldenClusterRepo struct {
	byID       map[string]*IntelClusterRow
	created    []string
	intel      *goldenIntelRepo
	mergeCalls []string // 黄金集断言：整个回放不得发生跨簇并入
}

func newGoldenClusterRepo(intel *goldenIntelRepo) *goldenClusterRepo {
	return &goldenClusterRepo{byID: map[string]*IntelClusterRow{}, intel: intel}
}

func (s *goldenClusterRepo) Create(_ context.Context, row *IntelClusterRow) (*IntelClusterRow, error) {
	s.byID[row.ID] = row
	s.created = append(s.created, row.ID)
	return row, nil
}
func (s *goldenClusterRepo) GetByID(_ context.Context, id string) (*IntelClusterRow, error) {
	return s.byID[id], nil
}
func (s *goldenClusterRepo) TouchSeen(_ context.Context, id string, at time.Time) error {
	if cl, ok := s.byID[id]; ok {
		cl.LastSeenAt = at.UTC()
	}
	return nil
}

// RecomputeStats 与 data 层同口径：member_count 以条目实际指针为准。
func (s *goldenClusterRepo) RecomputeStats(_ context.Context, id string) error {
	cl, ok := s.byID[id]
	if !ok {
		return nil
	}
	var n int32
	for _, r := range s.intel.rows {
		if r.Dedup.ClusterID == id {
			n++
		}
	}
	cl.MemberCount = n
	return nil
}

// MergeTx S-3 跨簇并入：黄金集任何一次调用都意味着误并，记录后由断言兜底。
func (s *goldenClusterRepo) MergeTx(_ context.Context, fromID, toID, reason, _ string) error {
	s.mergeCalls = append(s.mergeCalls, fromID+"->"+toID+" ("+reason+")")
	from := s.byID[fromID]
	if from != nil {
		from.Status = "closed"
	}
	for _, r := range s.intel.rows {
		if r.Dedup.ClusterID == fromID {
			r.Dedup.ClusterID = toID
		}
	}
	return nil
}

func (s *goldenClusterRepo) List(context.Context, *IntelClusterListQuery) ([]*IntelClusterRow, int, error) {
	panic("golden stub: not used")
}
func (s *goldenClusterRepo) SetRepresentative(context.Context, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenClusterRepo) SetCategory(context.Context, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenClusterRepo) SetCentroidRef(context.Context, string, string) error {
	panic("golden stub: not used")
}
func (s *goldenClusterRepo) SplitTx(context.Context, string, []string, string, string) (string, error) {
	panic("golden stub: not used")
}
func (s *goldenClusterRepo) ListIdleOpen(context.Context, time.Time, int) ([]*IntelClusterRow, error) {
	panic("golden stub: not used")
}
func (s *goldenClusterRepo) Close(context.Context, string, time.Time) error {
	panic("golden stub: not used")
}

type goldenSampleRepo struct {
	rows []*DedupSampleRow
}

func (s *goldenSampleRepo) Create(_ context.Context, row *DedupSampleRow) error {
	s.rows = append(s.rows, row)
	return nil
}
func (s *goldenSampleRepo) ListByCluster(context.Context, string, int) ([]*DedupSampleRow, error) {
	panic("golden stub: not used")
}

// ---------- 回放 ----------

func TestDedupGoldenReplay(t *testing.T) {
	entries := loadGoldenEntries(t)
	ctx := context.Background()
	cfg := ResolveIcConfig(nil) // 默认基线：汉明 ≤3、窗口 72h
	logger := log.NewStdLogger(io.Discard)

	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &goldenSampleRepo{}
	uc := NewIcDedupUsecase(intel, clusters, samples, nil, cfg, logger)

	idf := LoadIDFTable("")
	replayAt := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

	labelCluster := map[string]string{} // 黄金标签 → 实际簇 ID
	fps := map[string]uint64{}          // id → 指纹（跨簇隔离性断言用）
	var mergeIDs []string
	var sampleOf func(id string) *DedupSampleRow
	sampleOf = func(id string) *DedupSampleRow {
		for _, r := range samples.rows {
			if r.IntelID == "it_golden_"+id {
				return r
			}
		}
		return nil
	}

	for _, e := range entries {
		e := e
		t.Run(e.ID+"/"+e.Cluster, func(t *testing.T) {
			urlKey, canonical, err := NormalizeURLKey(e.URL)
			if err != nil {
				t.Fatalf("normalize url: %v", err)
			}
			if canonical == "" {
				t.Fatal("canonical must not be empty")
			}
			sim := ComputeSimHash(e.Title, e.Text, e.Lang, idf)
			if e.NoFP {
				if sim.Valid {
					t.Fatalf("entry marked no_fp but fingerprint is valid (tokens=%d)", sim.Tokens)
				}
			} else if !sim.Valid {
				t.Fatalf("fingerprint invalid (tokens=%d) —— 指纹无效的条目 L2 永不命中，语料需加长正文", sim.Tokens)
			}
			if sim.Valid {
				fps[e.ID] = sim.Fingerprint
			}

			row := &IntelItemRow{SourceID: "golden-src", FingerprintValid: sim.Valid, FpSegments: sim.Segments}
			row.IntelID = "it_golden_" + e.ID
			row.Source.Platform = "weibo" // 元信息，不影响去重判定
			row.Source.Region = contract.RegionCN
			if e.Lang == "en" {
				row.Source.Region = contract.RegionIntl
			}
			row.Source.CollectedAt = replayAt
			row.Payload.Title = e.Title
			row.Payload.Text = e.Text
			row.Dedup.URLKey = urlKey
			row.Dedup.Fingerprint = sim.Hex
			intel.add(row)

			out, err := uc.Resolve(ctx, row, &sim)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}

			switch e.Expect {
			case "new":
				if _, dup := labelCluster[e.Cluster]; dup {
					t.Fatalf("expect=new 但标签 %s 已存在（语料顺序错误：首条必须 new）", e.Cluster)
				}
				if out.Action != "new_cluster" {
					t.Fatalf("expected new_cluster, got %s (cluster=%s level=%s matched=%s) —— 与其他条目误并？",
						out.Action, out.ClusterID, out.Level, out.MatchedIntelID)
				}
				labelCluster[e.Cluster] = out.ClusterID

			case "merge":
				want, ok := labelCluster[e.Cluster]
				if !ok {
					t.Fatalf("merge 引用了未出现标签 %s（语料顺序错误）", e.Cluster)
				}
				if out.Action != "merged" {
					t.Fatalf("expected merged, got %s —— 近重复未命中（指纹漂移？）", out.Action)
				}
				if out.ClusterID != want {
					t.Fatalf("并入错误簇 %s（应入 %s）—— 跨标签误并", out.ClusterID, want)
				}
				if e.Level != "" && out.Level != e.Level {
					t.Fatalf("去重级别 %s，期望 %s（判定路径漂移）", out.Level, e.Level)
				}
				if out.MatchedIntelID == "" {
					t.Fatal("merged 必须带 matched intel id（审计链）")
				}
				mergeIDs = append(mergeIDs, e.ID)

			default:
				t.Fatalf("expect 非法取值 %q（仅 new|merge）", e.Expect)
			}

			// 行上指针与 outcome 一致（落库正确性）
			if row.Dedup.ClusterID != out.ClusterID {
				t.Fatalf("行 cluster_id=%s 与 outcome=%s 不一致", row.Dedup.ClusterID, out.ClusterID)
			}
		})
	}

	// ---- 全局断言 ----

	if len(clusters.created) != len(labelCluster) {
		t.Fatalf("建簇数 %d ≠ 标签数 %d（有标签从未 new 或重复建簇）", len(clusters.created), len(labelCluster))
	}
	if len(clusters.mergeCalls) != 0 {
		t.Fatalf("黄金集发生跨簇并入（应为 0）：%v", clusters.mergeCalls)
	}

	// 抽样表：每条 merge 一条 auto_merge，级别与期望一致（M-27 评审抽样留痕定位）
	if len(samples.rows) != len(mergeIDs) {
		t.Fatalf("auto_merge 抽样 %d 条 ≠ merge 条目 %d 条", len(samples.rows), len(mergeIDs))
	}
	levelByID := map[string]string{}
	for _, e := range entries {
		if e.Level != "" {
			levelByID[e.ID] = e.Level
		}
	}
	for _, id := range mergeIDs {
		s := sampleOf(id)
		if s == nil {
			t.Fatalf("%s 缺 auto_merge 抽样", id)
		}
		if s.Action != "auto_merge" || s.Operator != "system:dedup" {
			t.Fatalf("%s 抽样字段异常: action=%s operator=%s", id, s.Action, s.Operator)
		}
		if want := levelByID[id]; want != "" && s.Level != want {
			t.Fatalf("%s 抽样级别 %s ≠ 期望 %s", id, s.Level, want)
		}
	}

	// 全量指针终态：每条目最终 cluster_id 必须等于其标签的簇
	for _, e := range entries {
		row := intel.byID["it_golden_"+e.ID]
		if row == nil || row.Dedup.ClusterID != labelCluster[e.Cluster] {
			t.Fatalf("%s 终态 cluster_id=%v ≠ 标签簇 %s", e.ID, row, labelCluster[e.Cluster])
		}
	}

	// 幂等回放（NATS at-least-once）：已带簇条目重放返回同簇，无副作用
	lastMerge := intel.byID["it_golden_"+mergeIDs[len(mergeIDs)-1]]
	out, err := uc.Resolve(ctx, lastMerge, &SimHashResult{})
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if out.Action != "merged" || out.ClusterID != lastMerge.Dedup.ClusterID {
		t.Fatalf("幂等回放漂移: %+v, row cluster=%s", out, lastMerge.Dedup.ClusterID)
	}
	if len(clusters.created) != len(labelCluster) {
		t.Fatal("幂等回放不得新建簇")
	}

	// ---- 跨簇隔离性（负例核心）：任意不同标签的有效指纹对汉明必须 > 阈值 ----
	threshold := int(cfg.SimhashThreshold)
	ids := make([]string, 0, len(fps))
	for id := range fps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	labelOf := map[string]string{}
	for _, e := range entries {
		labelOf[e.ID] = e.Cluster
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if labelOf[ids[i]] == labelOf[ids[j]] {
				continue
			}
			d := HammingDistance(fps[ids[i]], fps[ids[j]])
			if d <= threshold {
				t.Errorf("跨标签指纹过近: %s(%s) × %s(%s) hamming=%d ≤ %d —— 语料/算法回归，需调整文本或复查分词",
					ids[i], labelOf[ids[i]], ids[j], labelOf[ids[j]], d, threshold)
			}
		}
	}

	// 语料自检汇总：防后续增补破坏结构
	t.Logf("golden replay done: %d entries, %d clusters, %d merges (samples=%d), threshold=%d",
		len(entries), len(labelCluster), len(mergeIDs), len(samples.rows), threshold)
}

// TestDedupGoldenCorpusIntegrity 语料结构自检（独立快测，失败先于回放暴露）。
func TestDedupGoldenCorpusIntegrity(t *testing.T) {
	entries := loadGoldenEntries(t)
	seen := map[string]bool{}
	labels := map[string]int{}
	for i, e := range entries {
		if e.ID == "" || e.URL == "" || e.Text == "" || e.Cluster == "" {
			t.Fatalf("line %d: id/url/text/cluster 必填", i+1)
		}
		if seen[e.ID] {
			t.Fatalf("line %d: id %s 重复", i+1, e.ID)
		}
		seen[e.ID] = true
		labels[e.Cluster]++
		switch e.Expect {
		case "new":
			if labels[e.Cluster] != 1 {
				t.Fatalf("%s: expect=new 必须是标签 %s 的首次出现", e.ID, e.Cluster)
			}
			if e.Level != "" {
				t.Fatalf("%s: new 不得声明 level", e.ID)
			}
		case "merge":
			if labels[e.Cluster] < 2 {
				t.Fatalf("%s: merge 引用了未出现标签 %s", e.ID, e.Cluster)
			}
		default:
			t.Fatalf("%s: expect 非法取值 %q", e.ID, e.Expect)
		}
	}
	// new 与 merge 数量关系：merge 总数 == 条目数 - 标签数
	var merges int
	for _, e := range entries {
		if e.Expect == "merge" {
			merges++
		}
	}
	if merges != len(entries)-len(labels) {
		t.Fatalf("merge 数 %d ≠ %d-%d", merges, len(entries), len(labels))
	}
}
