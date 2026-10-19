package biz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 去重 L3（语义向量）与人工调参（S-23）测试 —— S-35 dedup ≥85% 攻坚
//
// 与黄金回放（确定性语料证行为）互补：本文件用「受控指纹 + 可编程向量桩」
// 直接驱动 resolveL3 准入/命中/降级、resolveL2 多簇碰撞、空簇旁支、
// MergeClusters/SplitCluster 审计样本等黄金语料构造代价过高的分支。
// =============================================================================

// stubEmbedder 可编程向量端口（B-7 三条件的逐项演绎）。
type stubEmbedder struct {
	enabled        bool
	dailyRemaining int32
	embedErr       error
	searchErr      error
	neighbors      []EmbeddingNeighbor
	embedCalls     int
	searchCalls    int
}

func (s *stubEmbedder) Embed(_ context.Context, intelID, _ string) (string, error) {
	s.embedCalls++
	if s.embedErr != nil {
		return "", s.embedErr
	}
	return "emb_" + intelID, nil
}
func (s *stubEmbedder) SearchSimilar(_ context.Context, _ string, _ float64, _ int32) ([]EmbeddingNeighbor, error) {
	s.searchCalls++
	return s.neighbors, s.searchErr
}
func (s *stubEmbedder) Enabled() bool                          { return s.enabled }
func (s *stubEmbedder) DailyRemaining(context.Context) int32   { return s.dailyRemaining }

// l3Cfg 打开 B-7 开关的幂等配置（默认 yaml 基线 store 关闭 → 手工拨正）。
func l3Cfg() *IcConfig {
	cfg := ResolveIcConfig(nil)
	cfg.EmbeddingStoreEnabled = true
	return cfg
}

// liteRow 受控指纹行：绕过语料校准，直接给定 url_key / 指纹 / 段（可精确摆汉明距离）。
func liteRow(id, urlKey, fpHex string, segs [4]int16) *IntelItemRow {
	row := &IntelItemRow{SourceID: "src-lite", FingerprintValid: true, FpSegments: segs}
	row.IntelID = "it_lite_" + id
	row.APIVersion = "1.0"
	row.Source.Platform = "weibo"
	row.Source.Region = contract.RegionCN
	row.Source.CollectMethod = contract.CollectHotlist
	row.Source.URL = "https://example.com/" + id
	row.Source.CollectedAt = normAt
	row.Payload.Title = "受控指纹行 " + id
	row.Payload.Text = "受控正文 " + id
	row.Dedup.URLKey = urlKey
	row.Dedup.Fingerprint = fpHex
	return row
}

// liteSim 与 liteRow 配对的 sim 视图（主持调用方 Resolve 入参）。
func liteSim(fpHex string, segs [4]int16, tokens int) *SimHashResult {
	fp, err := strconv.ParseUint(fpHex, 16, 64)
	if err != nil {
		return nil
	}
	return &SimHashResult{Fingerprint: fp, Hex: fpHex, Segments: segs, Valid: true, Tokens: tokens, Lang: "zh"}
}

func TestDedupL3(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)
	segs := [4]int16{1, 2, 3, 4}

	// 通用布景：先占者在开簇中，等待 L3 邻居命中
	setupClusteredPeer := func(t *testing.T, intel *goldenIntelRepo, clusters *goldenClusterRepo, samples *goldenSampleRepo) (peerID, clusterID string) {
		t.Helper()
		peer := liteRow("peer", "k_peer", "0000000011111111", [4]int16{9, 9, 9, 9})
		intel.add(peer)
		uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), logger)
		out, err := uc.Resolve(ctx, peer, liteSim("0000000011111111", [4]int16{9, 9, 9, 9}, 5))
		if err != nil || out.Action != "new_cluster" {
			t.Fatalf("peer 建簇失败: out=%+v err=%v", out, err)
		}
		return peer.IntelID, out.ClusterID
	}

	t.Run("L3 邻居命中 → merged 级别 L3，向量引用随出参", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		peerID, wantCluster := setupClusteredPeer(t, intel, clusters, samples)

		emb := &stubEmbedder{enabled: true, dailyRemaining: 100,
			neighbors: []EmbeddingNeighbor{{IntelID: peerID, Score: 0.91}}}
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), logger)

		row := liteRow("cur", "k_cur", "0000000022222222", segs)
		intel.add(row)
		out, err := uc.Resolve(ctx, row, liteSim("0000000022222222", segs, 50))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if out.Action != "merged" || out.Level != "L3" || out.ClusterID != wantCluster {
			t.Fatalf("L3 应并入邻居开簇: %+v (want cluster %s)", out, wantCluster)
		}
		if out.SimScore != 0.91 || out.MatchedIntelID != peerID {
			t.Fatalf("L3 相似度/对端未随出参: %+v", out)
		}
		if out.EmbeddingID != "emb_"+row.IntelID {
			t.Fatalf("命中也须带回向量引用（调用方 UpdateEmbedding）: %q", out.EmbeddingID)
		}
		if emb.embedCalls != 1 || emb.searchCalls != 1 {
			t.Fatalf("embedder 调用计数异常: %d/%d", emb.embedCalls, emb.searchCalls)
		}
		// L3 合并也留抽样（审计链一致）
		var last *DedupSampleRow
		for _, s := range samples.rows {
			if s.IntelID == row.IntelID {
				last = s
			}
		}
		if last == nil || last.Level != "L3" {
			t.Fatalf("缺 L3 auto_merge 抽样: %+v", last)
		}
	})

	t.Run("L3 跑了没命中 → 新簇但带回向量引用，不算跳过", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		emb := &stubEmbedder{enabled: true, dailyRemaining: 100} // neighbors 空
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), logger)

		row := liteRow("solo", "k_solo", "0000000033333333", segs)
		intel.add(row)
		out, err := uc.Resolve(ctx, row, liteSim("0000000033333333", segs, 50))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if out.Action != "new_cluster" || out.ClusterID == "" {
			t.Fatalf("无邻居应新簇: %+v", out)
		}
		if out.EmbeddingID != "emb_"+row.IntelID {
			t.Fatalf("跑过 L3 即使未命中也要回引用: %q", out.EmbeddingID)
		}
		if out.L3Skipped != "" {
			t.Fatalf("跑过 L3 不算跳过: %q", out.L3Skipped)
		}
	})

	t.Run("B-7 配额日尽 → L3 整级短路，embed 零调用", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		emb := &stubEmbedder{enabled: true, dailyRemaining: 0}
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), logger)

		row := liteRow("cap", "k_cap", "0000000044444444", segs)
		intel.add(row)
		out, err := uc.Resolve(ctx, row, liteSim("0000000044444444", segs, 50))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if emb.embedCalls != 0 {
			t.Fatalf("配额耗尽仍调用了 embed（B-7 硬熔断失守）: %d", emb.embedCalls)
		}
		if out.L3Skipped != "cap" {
			t.Fatalf("观测口径应为 cap: %q", out.L3Skipped)
		}
	})

	t.Run("B-7 语料过短 → tokens 短路", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		emb := &stubEmbedder{enabled: true, dailyRemaining: 100}
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), logger)

		row := liteRow("thin", "k_thin", "0000000055555555", segs)
		intel.add(row)
		out, _ := uc.Resolve(ctx, row, liteSim("0000000055555555", segs, 5))
		if out.L3Skipped != "tokens" {
			t.Fatalf("观测口径应为 tokens: %q", out.L3Skipped)
		}
		if emb.embedCalls != 0 {
			t.Fatal("tokens 短路先于 embed 调用")
		}
	})

	t.Run("store 配置关闭 → disabled，embed 零调用（即便实现可用）", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		emb := &stubEmbedder{enabled: true, dailyRemaining: 100}
		cfgOff := ResolveIcConfig(nil) // EmbeddingStoreEnabled=false
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, cfgOff, logger)

		row := liteRow("off", "k_off", "0000000066666666", segs)
		intel.add(row)
		out, _ := uc.Resolve(ctx, row, liteSim("0000000066666666", segs, 50))
		if out.L3Skipped != "disabled" {
			t.Fatalf("观测口径应为 disabled: %q", out.L3Skipped)
		}
		if emb.embedCalls != 0 {
			t.Fatal("配置关闭时不得调 embed（B-7 junction guarded by config）")
		}
	})

	t.Run("embed 服务故障 → 跳过是设计语义，观测 unavailable", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		emb := &stubEmbedder{enabled: true, dailyRemaining: 100, embedErr: errors.New("vector service down")}
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), logger)

		row := liteRow("sick", "k_sick", "0000000077777777", segs)
		intel.add(row)
		out, err := uc.Resolve(ctx, row, liteSim("0000000077777777", segs, 50))
		if err != nil {
			t.Fatalf("embed 故障不得上抛（B-7）: %v", err)
		}
		if out.Action != "new_cluster" || out.L3Skipped != "unavailable" {
			t.Fatalf("故障降级路径异常: %+v", out)
		}
	})

	t.Run("L3 邻居是闭簇成员 → 跳过该邻居照常新簇（S-2）", func(t *testing.T) {
		intel := newGoldenIntelRepo()
		clusters := newGoldenClusterRepo(intel)
		samples := &goldenSampleRepo{}
		// 手工摆一个闭簇 + 成员
		closed := &IntelClusterRow{ID: "cl_l3_closed", Status: "closed", MemberCount: 1}
		clusters.byID[closed.ID] = closed
		peer := liteRow("inclosed", "k_inclosed", "0000000088888888", [4]int16{8, 8, 8, 8})
		peer.Dedup.ClusterID = closed.ID
		intel.add(peer)

		emb := &stubEmbedder{enabled: true, dailyRemaining: 100,
			neighbors: []EmbeddingNeighbor{{IntelID: peer.IntelID, Score: 0.95}}}
		uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), logger)

		row := liteRow("guest", "k_guest", "0000000099999999", segs)
		intel.add(row)
		out, err := uc.Resolve(ctx, row, liteSim("0000000099999999", segs, 50))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if out.Action != "new_cluster" || out.ClusterID == closed.ID {
			t.Fatalf("闭簇不得吸纳（S-2）: %+v", out)
		}
	})
}

// TestDedupL2MultiCluster L2 多簇碰撞：受控指纹摆两个近邻分属不同开簇 →
// 主簇取成员多者（并列入簇 ID 小者），其余簇 MergeTx 并入（03 §4.4）。
func TestDedupL2MultiCluster(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)

	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &goldenSampleRepo{}
	uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), logger)

	segs := [4]int16{5, 5, 5, 5}
	mkClustered := func(id, fpHex, clusterPrefix string, extraMembers int32) string {
		clusterID := "cl_" + clusterPrefix
		clusters.byID[clusterID] = &IntelClusterRow{ID: clusterID, Status: "open", MemberCount: 1 + extraMembers}
		peer := liteRow(id, "k_"+id, fpHex, segs)
		peer.Dedup.ClusterID = clusterID
		intel.add(peer)
		return clusterID
	}
	// 当前 row fp=...0000；A 距 1 bit，B 距 2 bits —— 阈值 3 双双命中
	clA := mkClustered("pa", "0000000000000001", "aaa", 0)
	clB := mkClustered("pb", "0000000000000003", "bbb", 4) // 成员更多 → 主簇

	row := liteRow("cur", "k_cur", "0000000000000000", segs)
	intel.add(row)
	out, err := uc.Resolve(ctx, row, liteSim("0000000000000000", segs, 50))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out.Action != "merged" || out.ClusterID != clB {
		t.Fatalf("应并入成员更多的主簇 %s: %+v", clB, out)
	}
	if len(clusters.mergeCalls) != 1 || clusters.mergeCalls[0] != fmt.Sprintf("%s->%s (dedup:L2 multi-cluster collision)", clA, clB) {
		t.Fatalf("次簇应 MergeTx 并入主簇: %v", clusters.mergeCalls)
	}
}

// TestDedupL1PeerNoCluster L1 命中的对端还没有簇（同 URL 乱序归一化）：
// 新建联合簇把双方一并纳入（mergeIntoCluster 空簇旁支。
// 注：resolveL2 对空簇候选有早退卫士 target=="" → 该旁支只能由 L1 路径驱动）。
func TestDedupL1PeerNoCluster(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)

	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &goldenSampleRepo{}
	uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), logger)

	segs := [4]int16{7, 7, 7, 7}
	orphan := liteRow("orphan", "k_shared", "0000000000000002", segs) // 同 url_key、尚无簇
	intel.add(orphan)

	row := liteRow("cur", "k_shared", "0000000000000000", segs)
	intel.add(row)
	out, err := uc.Resolve(ctx, row, liteSim("0000000000000000", segs, 50))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out.Action != "merged" || out.Level != "L1" || out.ClusterID == "" {
		t.Fatalf("对端无簇也应 L1 合并（新建联合簇）: %+v", out)
	}
	if orphan.Dedup.ClusterID != out.ClusterID || row.Dedup.ClusterID != out.ClusterID {
		t.Fatalf("双方须同入新簇: orphan=%s row=%s out=%s",
			orphan.Dedup.ClusterID, row.Dedup.ClusterID, out.ClusterID)
	}
	cl := clusters.byID[out.ClusterID]
	if cl == nil || cl.RepresentativeID != orphan.IntelID {
		t.Fatalf("联合簇代表应为先到者: %+v", cl)
	}
	if cl.FirstSeenAt != normAt || cl.LastSeenAt != normAt {
		t.Fatalf("联合簇时间窗应覆盖双端: %+v", cl)
	}
}

// TestDedupForceL1MergeGhostPeer 竞态兜底：短链撞车宣称的对端在仓储里已不可见
// （保留期清扫/半事务），mergeIntoCluster 返回 nil,nil——调用方按未命中继续。
func TestDedupForceL1MergeGhostPeer(t *testing.T) {
	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &goldenSampleRepo{}
	uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), log.NewStdLogger(io.Discard))

	row := liteRow("cur", "k_cur", "0000000000000000", [4]int16{1, 1, 1, 1})
	intel.add(row)
	ghost := liteRow("ghost", "k_ghost", "0000000000000004", [4]int16{4, 4, 4, 4}) // 未入仓储

	out, err := uc.ForceL1Merge(context.Background(), row, ghost)
	if err != nil {
		t.Fatalf("幽灵对端不得报错（竞态按未命中处理）: %v", err)
	}
	if out != nil {
		t.Fatalf("幽灵对端应返回 nil 出参: %+v", out)
	}
	if row.Dedup.ClusterID != "" {
		t.Fatalf("行不得被幽灵簇污染: %s", row.Dedup.ClusterID)
	}
}

// TestDedupManualOps 人工调参（S-23）：并簇/劈簇的操作者审计样本。
func TestDedupManualOps(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)

	newRepo := func() (*goldenIntelRepo, *adminClusterRepo, *goldenSampleRepo) {
		intel := newGoldenIntelRepo()
		base := newGoldenClusterRepo(intel)
		base.byID["cl_src"] = &IntelClusterRow{ID: "cl_src", Status: "open", MemberCount: 2}
		base.byID["cl_dst"] = &IntelClusterRow{ID: "cl_dst", Status: "open", MemberCount: 1}
		samples := &goldenSampleRepo{}
		return intel, &adminClusterRepo{goldenClusterRepo: base, samples: samples}, samples
	}

	t.Run("MergeClusters 落审计样本", func(t *testing.T) {
		_, clusters, samples := newRepo()
		uc := NewIcDedupUsecase(nil, clusters, samples, nil, ResolveIcConfig(nil), logger)
		if err := uc.MergeClusters(ctx, "cl_src", "cl_dst", "alice"); err != nil {
			t.Fatalf("merge: %v", err)
		}
		if len(clusters.mergeCalls) != 1 || clusters.mergeCalls[0] != "cl_src->cl_dst (manual)" {
			t.Fatalf("MergeTx 记录异常: %v", clusters.mergeCalls)
		}
		if len(samples.rows) != 1 || samples.rows[0].Action != "manual_merge" || samples.rows[0].Operator != "alice" {
			t.Fatalf("manual_merge 样本异常: %+v", samples.rows)
		}
	})

	t.Run("MergeClusters 自并早退 + 操作者缺省兜底", func(t *testing.T) {
		_, clusters, samples := newRepo()
		uc := NewIcDedupUsecase(nil, clusters, samples, nil, ResolveIcConfig(nil), logger)
		if err := uc.MergeClusters(ctx, "cl_src", "cl_src", "alice"); err != nil {
			t.Fatalf("自并应幂等返回 nil: %v", err)
		}
		if len(clusters.mergeCalls) != 0 {
			t.Fatalf("自并不得触发 MergeTx: %v", clusters.mergeCalls)
		}
		if err := uc.MergeClusters(ctx, "cl_src", "cl_dst", ""); err != nil {
			t.Fatalf("空操作者应被兜底: %v", err)
		}
		if samples.rows[0].Operator != "unknown" {
			t.Fatalf("操作者缺省应为 unknown: %q", samples.rows[0].Operator)
		}
	})

	t.Run("SplitCluster 劈出成员入新簇 + 逐员样本", func(t *testing.T) {
		intel, clusters, samples := newRepo()
		m1 := liteRow("m1", "k_m1", "0000000000000001", [4]int16{1, 1, 1, 1})
		m2 := liteRow("m2", "k_m2", "0000000000000002", [4]int16{2, 2, 2, 2})
		stay := liteRow("stay", "k_stay", "0000000000000004", [4]int16{4, 4, 4, 4})
		for _, m := range []*IntelItemRow{m1, m2, stay} {
			m.Dedup.ClusterID = "cl_src"
			intel.add(m)
		}
		uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), logger)

		newID, err := uc.SplitCluster(ctx, "cl_src", []string{m1.IntelID, m2.IntelID}, "bob", "人工复核流出")
		if err != nil {
			t.Fatalf("split: %v", err)
		}
		if newID == "" || m1.Dedup.ClusterID != newID || m2.Dedup.ClusterID != newID {
			t.Fatalf("劈出成员未入新簇 %s: %s/%s", newID, m1.Dedup.ClusterID, m2.Dedup.ClusterID)
		}
		if stay.Dedup.ClusterID != "cl_src" {
			t.Fatalf("留守成员被误挪: %s", stay.Dedup.ClusterID)
		}
		if clusters.byID[newID] == nil || clusters.byID[newID].Label != "人工复核流出" {
			t.Fatalf("新簇行异常: %+v", clusters.byID[newID])
		}
		if len(samples.rows) != 2 || samples.rows[0].Action != "manual_split" || samples.rows[0].Operator != "bob" {
			t.Fatalf("manual_split 样本异常: %+v", samples.rows)
		}
	})

	t.Run("SplitCluster 空成员 → 参数错误早退", func(t *testing.T) {
		_, clusters, samples := newRepo()
		uc := NewIcDedupUsecase(nil, clusters, samples, nil, ResolveIcConfig(nil), logger)
		if _, err := uc.SplitCluster(ctx, "cl_src", nil, "bob", "x"); !errors.Is(err, ErrIcInvalidArgument) {
			t.Fatalf("空劈簇应报参数错误: %v", err)
		}
		if len(samples.rows) != 0 {
			t.Fatal("早退不得写样本")
		}
	})
}

// adminClusterRepo 人工调参需要 SplitTx 行为（黄金回放用不到 → 那里保持 panic）。
// samples 字段：SplitTx 事务内写 manual_split 审计（镜像生产 ic_cluster_repo.go:336-350），
// biz 层不再重复写——stub 必须承托同一语义，否则样本断言落空。
type adminClusterRepo struct {
	*goldenClusterRepo
	samples   *goldenSampleRepo
	failSplit bool
}

func (s *adminClusterRepo) SplitTx(_ context.Context, fromID string, intelIDs []string, operator, label string) (string, error) {
	if s.failSplit {
		return "", errors.New("split tx down")
	}
	newID := "cl_split_" + strconv.Itoa(len(s.byID)+1)
	s.byID[newID] = &IntelClusterRow{
		ID: newID, Label: label, Status: "open", MergedBy: "manual",
		FirstSeenAt: normAt, LastSeenAt: normAt,
	}
	s.created = append(s.created, newID)
	var moved int32
	for _, r := range s.intel.rows {
		if r.Dedup.ClusterID == fromID {
			for _, id := range intelIDs {
				if r.IntelID == id {
					r.Dedup.ClusterID = newID
					moved++
				}
			}
		}
	}
	s.byID[newID].MemberCount = moved
	if src := s.byID[fromID]; src != nil {
		src.MemberCount -= moved
	}
	// 镜像生产 SplitTx：事务内为每个劈出成员写一条 manual_split 审计样本
	// （M-27 黄金集评审依据）；biz 层不再重复写。
	if s.samples != nil {
		for _, id := range intelIDs {
			s.samples.rows = append(s.samples.rows, &DedupSampleRow{
				IntelID: id, ClusterID: newID, Action: "manual_split", Operator: operator,
			})
		}
	}
	return newID, nil
}

var _ = time.UTC // 保持 import 纪律（normAt 复用自 ic_normalize_test.go）

// =============================================================================
// 错误路径与哨兵（S-35 dedup ≥85%）：仓储故障上抛、脏指纹跳过、样本写失败不阻塞
// =============================================================================

// TestDedupSentinels 哨兵语义文本（监控/死信里的可读性也是契约）。
func TestDedupSentinels(t *testing.T) {
	if ErrIcInvalidArgument.Error() != "invalid argument" {
		t.Fatalf("哨兵文本漂移: %q", ErrIcInvalidArgument.Error())
	}
	if icErrorSentinel("probe").Error() != "probe" {
		t.Fatal("icErrorSentinel 包装失真")
	}
	if !errors.Is(errSkipL3NoMatch, errSkipL3NoMatch) {
		t.Fatal("同一哨兵 errors.Is 必须命中（L3 流控依赖）")
	}
	if errors.Is(errSkipL3NoMatch, ErrIcInvalidArgument) {
		t.Fatal("不同哨兵不得互相命中")
	}
}

// TestDedupMinMaxTime 时间窗求值两序对称。
func TestDedupMinMaxTime(t *testing.T) {
	a, b := normAt, normAt.Add(time.Hour)
	if minTime(a, b) != a || minTime(b, a) != a {
		t.Fatal("minTime 两序不对称")
	}
	if maxTime(a, b) != b || maxTime(b, a) != b {
		t.Fatal("maxTime 两序不对称")
	}
}

// errIntelRepo 仓储故障开关（golden 回放用不到 → 单独包装）。
type errIntelRepo struct {
	*goldenIntelRepo
	failURLKey bool
	failSim    bool
	failGet    bool
}

func (s *errIntelRepo) FindRecentByURLKey(context.Context, string, string, time.Duration) (*IntelItemRow, error) {
	if s.failURLKey {
		return nil, errors.New("urlkey index down")
	}
	return s.goldenIntelRepo.FindRecentByURLKey(context.Background(), "", "", 0)
}
func (s *errIntelRepo) FindSimHashCandidates(context.Context, [4]int16, time.Duration, int) ([]*IntelItemRow, error) {
	if s.failSim {
		return nil, errors.New("simhash inverted index down")
	}
	return s.goldenIntelRepo.FindSimHashCandidates(context.Background(), [4]int16{}, 0, 0)
}
func (s *errIntelRepo) GetByID(_ context.Context, id string) (*IntelItemRow, error) {
	if s.failGet {
		return nil, errors.New("intel row read down")
	}
	return s.goldenIntelRepo.GetByID(context.Background(), id)
}

// TestDedupResolveRepoFailures 仓储故障逐层上抛（死信纪律：硬错误才 retry/死信）。
func TestDedupResolveRepoFailures(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)
	segs := [4]int16{5, 5, 5, 5}

	t.Run("url_key 召回故障 → 直接上抛", func(t *testing.T) {
		intel := &errIntelRepo{goldenIntelRepo: newGoldenIntelRepo(), failURLKey: true}
		uc := NewIcDedupUsecase(intel, newGoldenClusterRepo(intel.goldenIntelRepo), &goldenSampleRepo{}, nil, ResolveIcConfig(nil), logger)
		row := liteRow("cur", "k_cur", "0000000000000000", segs)
		if _, err := uc.Resolve(ctx, row, liteSim("0000000000000000", segs, 50)); err == nil {
			t.Fatal("L1 召回故障必须上抛（worker 据此写死信）")
		}
	})

	t.Run("simhash 召回故障 → 直接上抛", func(t *testing.T) {
		intel := &errIntelRepo{goldenIntelRepo: newGoldenIntelRepo(), failSim: true}
		uc := NewIcDedupUsecase(intel, newGoldenClusterRepo(intel.goldenIntelRepo), &goldenSampleRepo{}, nil, ResolveIcConfig(nil), logger)
		row := liteRow("cur", "k_cur", "0000000000000000", segs)
		if _, err := uc.Resolve(ctx, row, liteSim("0000000000000000", segs, 50)); err == nil {
			t.Fatal("L2 召回故障必须上抛")
		}
	})

	t.Run("对端读故障（ForceL1Merge 内部）→ 上抛", func(t *testing.T) {
		intel := &errIntelRepo{goldenIntelRepo: newGoldenIntelRepo(), failGet: true}
		uc := NewIcDedupUsecase(intel, newGoldenClusterRepo(intel.goldenIntelRepo), &goldenSampleRepo{}, nil, ResolveIcConfig(nil), logger)
		row := liteRow("cur", "k_cur", "0000000000000000", segs)
		ghost := liteRow("ghost", "k_ghost", "0000000000000004", [4]int16{4, 4, 4, 4})
		if _, err := uc.ForceL1Merge(ctx, row, ghost); err == nil {
			t.Fatal("对端读故障必须上抛（区分竞态不可见与仓储故障）")
		}
	})
}

// TestDedupL2CandidateDirtyFingerprint 脏数据纪律：入库行指纹不可解析 →
// 跳过该候选（不让一条坏行瘫痪整个倒排桶）。
func TestDedupL2CandidateDirtyFingerprint(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)

	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &goldenSampleRepo{}
	uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), logger)

	segs := [4]int16{6, 6, 6, 6}
	dirty := liteRow("dirty", "k_dirty", "不是十六进制!", segs) // 入库异常行
	intel.add(dirty)

	row := liteRow("cur", "k_cur", "0000000000000000", segs)
	intel.add(row)
	out, err := uc.Resolve(ctx, row, liteSim("0000000000000000", segs, 50))
	if err != nil {
		t.Fatalf("脏指纹候选必须被跳过，不得污染判定: %v", err)
	}
	if out.Action != "new_cluster" {
		t.Fatalf("坏候选不得参与合并: %+v", out)
	}
}

// TestDedupSampleWriteFailure 抽样留痕失败不阻塞合并（审计尽力而为）。
func TestDedupSampleWriteFailure(t *testing.T) {
	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &failSampleRepo{goldenSampleRepo: &goldenSampleRepo{}}
	uc := NewIcDedupUsecase(intel, clusters, samples, nil, ResolveIcConfig(nil), log.NewStdLogger(io.Discard))

	// 先建簇，再 L1 入簇
	peer := liteRow("peer", "k_shared", "0000000011111111", [4]int16{9, 9, 9, 9})
	intel.add(peer)
	ctx := context.Background()
	_, err := uc.Resolve(ctx, peer, liteSim("0000000011111111", [4]int16{9, 9, 9, 9}, 5))
	if err != nil {
		t.Fatalf("peer 建簇: %v", err)
	}
	row := liteRow("cur", "k_shared", "0000000000000000", [4]int16{7, 7, 7, 7})
	intel.add(row)
	out, err := uc.Resolve(ctx, row, liteSim("0000000000000000", [4]int16{7, 7, 7, 7}, 5))
	if err != nil {
		t.Fatalf("样本写故障不得上抛: %v", err)
	}
	if out.Action != "merged" || out.ClusterID != peer.Dedup.ClusterID {
		t.Fatalf("合并结果不受样本故障影响: %+v", out)
	}
}

type failSampleRepo struct{ *goldenSampleRepo }

func (s *failSampleRepo) Create(context.Context, *DedupSampleRow) error {
	return errors.New("sample table down")
}

// TestDedupSplitTxFailure SplitTx 故障上抛且不写样本（部分失败不得留假审计）。
func TestDedupSplitTxFailure(t *testing.T) {
	intel := newGoldenIntelRepo()
	base := newGoldenClusterRepo(intel)
	base.byID["cl_src"] = &IntelClusterRow{ID: "cl_src", Status: "open", MemberCount: 2}
	clusters := &adminClusterRepo{goldenClusterRepo: base}
	clusters.failSplit = true
	samples := &goldenSampleRepo{}
	uc := NewIcDedupUsecase(nil, clusters, samples, nil, ResolveIcConfig(nil), log.NewStdLogger(io.Discard))

	if _, err := uc.SplitCluster(context.Background(), "cl_src", []string{"it_lite_x"}, "bob", "x"); err == nil {
		t.Fatal("SplitTx 故障必须上抛")
	}
	if len(samples.rows) != 0 {
		t.Fatal("失败路径不得写 manual_split 样本（假审计）")
	}
}

// TestDedupL3GhostNeighbor L3 邻居已不可见（保留期清扫竞态）→ 跳过照常新簇。
func TestDedupL3GhostNeighbor(t *testing.T) {
	intel := newGoldenIntelRepo()
	clusters := newGoldenClusterRepo(intel)
	samples := &goldenSampleRepo{}
	segs := [4]int16{3, 3, 3, 3}
	emb := &stubEmbedder{enabled: true, dailyRemaining: 100,
		neighbors: []EmbeddingNeighbor{{IntelID: "it_ghost_neighbor", Score: 0.99}}}
	uc := NewIcDedupUsecase(intel, clusters, samples, emb, l3Cfg(), log.NewStdLogger(io.Discard))

	row := liteRow("cur", "k_cur", "00000000aaaaaaaa", segs)
	intel.add(row)
	out, err := uc.Resolve(context.Background(), row, liteSim("00000000aaaaaaaa", segs, 50))
	if err != nil {
		t.Fatalf("幽灵邻居不得上抛: %v", err)
	}
	if out.Action != "new_cluster" {
		t.Fatalf("幽灵邻居不得参与合并: %+v", out)
	}
}
