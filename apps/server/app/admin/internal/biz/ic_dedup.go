package biz

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 三级去重与簇管理（03 §4.1-4.4 + 评审 B-6/B-7/B-9/B-26、S-2/S-3）
// 短路顺序 L1 → L2 → L3；命中即并入簇；未命中新建簇。
// L3 准入（B-7 修正后）：①L1/L2 未命中 ②tokens≥12 ③日配额未耗尽 —— 无类别条件
// （去重层不做类别判断，D_compliance 归治理层）。
// =============================================================================

// 合并窗口从 cfg.DedupMergeWindow 取（conf: ic.dedup.merge_window_hours，默认 72h，
// 推导 = 48h 闲时闭簇 S-2 + 一个采集周期余量；03 §4.4）。

// DedupOutcome 去重结论（归一化 usecase 负责把 ClusterID/EmbeddingID 落库）。
type DedupOutcome struct {
	Action         string // new_cluster | merged
	ClusterID      string
	Level          string  // L1 | L2 | L3 | ""（new_cluster 时为空）
	SimScore       float64 // L2: 1-hamming/64；L3: 余弦相似度
	MatchedIntelID string

	EmbeddingID string // L3 成功取向量时返回（调用方 UpdateEmbedding 落库）
	L3Skipped   string // "" | disabled | tokens | cap | unavailable（观测）
}

// IcDedupUsecase 编排。
type IcDedupUsecase struct {
	intel    IntelItemRepo
	clusters IntelClusterRepo
	samples  DedupSampleRepo
	embedder EmbedderPort // 可 nil（store=none 降级态）
	cfg      *IcConfig
	log      *log.Helper
}

func NewIcDedupUsecase(
	intel IntelItemRepo,
	clusters IntelClusterRepo,
	samples DedupSampleRepo,
	embedder EmbedderPort,
	cfg *IcConfig,
	logger log.Logger,
) *IcDedupUsecase {
	return &IcDedupUsecase{
		intel: intel, clusters: clusters, samples: samples, embedder: embedder, cfg: cfg,
		log: log.NewHelper(log.With(logger, "module", "ic/dedup/biz/admin-service")),
	}
}

// Resolve 对一条已落库待归一化的条目执行三级去重。
// 幂等纪律：已带 cluster_id 的重放消息直接返回现值（NATS at-least-once + MsgId 双保险）。
func (u *IcDedupUsecase) Resolve(ctx context.Context, row *IntelItemRow, sim *SimHashResult) (*DedupOutcome, error) {
	if row.Dedup.ClusterID != "" {
		return &DedupOutcome{Action: "merged", ClusterID: row.Dedup.ClusterID}, nil
	}
	self := row.IntelID

	// ---- L1：url_key 窗口命中（03 §4.1；短链已在此前展开重算 key）----
	// selfID 排除自身是必要条件：现役行在窗内且最新，SQL 不排除则命中恒为自身（M-27）。
	if hit, err := u.intel.FindRecentByURLKey(ctx, self, row.Dedup.URLKey, u.cfg.DedupMergeWindow); err != nil {
		return nil, err
	} else if hit != nil && hit.IntelID != self {
		// S-2：L1 命中只并入开簇——命中行的簇若已 idle-close，并入等于把新条目
		// 埋进下游按 status=open 过滤永远看不见的闭簇（评审坐实：48h 闭簇窗 <
		// 72h 合并窗，同 URL 迟到重采必踩）。闭簇时按未命中继续走 L2/L3。
		if l1ClusterID, l1Open := u.openClusterOf(ctx, hit); l1Open {
			out, merr := u.mergeIntoCluster(ctx, row, l1ClusterID, hit.IntelID, "L1", 1.0)
			if merr != nil {
				return nil, merr
			}
			if out != nil {
				return out, nil
			}
		}
		// out=nil：对端竞态不可见；l1Open=false：闭簇——均按未命中继续走 L2/L3
	}

	// ---- L2：SimHash 四段倒排召回 + 汉明判定（B-9/B-26）----
	if sim != nil && sim.Valid {
		out, err := u.resolveL2(ctx, row, sim)
		if err != nil {
			return nil, err
		}
		if out != nil {
			return out, nil
		}
	}

	// ---- L3：语义相似（B-7 准入三条件；成本控制短路）----
	var embeddingID string
	l3Out, l3Err := u.resolveL3(ctx, row, sim)
	switch {
	case l3Err == nil && l3Out != nil:
		return l3Out, nil // 命中
	case errors.Is(l3Err, errSkipL3NoMatch):
		embeddingID = l3Out.EmbeddingID // 跑了但没命中：向量已写，带上引用
	case l3Err != nil:
		return nil, l3Err
	}
	skipped := u.l3SkipReason(ctx, sim)
	if embeddingID != "" {
		skipped = "" // 实际执行过，不算跳过
	}

	// ---- 未命中：新建簇（03 §4.4）----
	cluster, err := u.clusters.Create(ctx, &IntelClusterRow{
		ID:               contract.NewClusterID(),
		Label:            truncateRunes(row.Payload.Title, 64),
		MemberCount:      1,
		PlatformCount:    1,
		RepresentativeID: self,
		Region:           string(row.Source.Region),
		FirstSeenAt:      row.Source.CollectedAt.UTC(),
		LastSeenAt:       row.Source.CollectedAt.UTC(),
		Status:           "open",
	})
	if err != nil {
		return nil, err
	}
	if err := u.intel.AssignCluster(ctx, []string{self}, cluster.ID); err != nil {
		return nil, err
	}
	return &DedupOutcome{Action: "new_cluster", ClusterID: cluster.ID, EmbeddingID: embeddingID, L3Skipped: skipped}, nil
}

// l3SkipReason 观测口径（Prometheus 计数标签）。
func (u *IcDedupUsecase) l3SkipReason(ctx context.Context, sim *SimHashResult) string {
	switch {
	case u.embedder == nil || !u.embedder.Enabled() || !u.cfg.EmbeddingStoreEnabled:
		return "disabled"
	case sim == nil || sim.Tokens < 12: // B-7 ②
		return "tokens"
	case u.embedder.DailyRemaining(ctx) <= 0: // B-7 ③
		return "cap"
	}
	return "unavailable"
}

// l3Eligible B-7 准入判定。
func (u *IcDedupUsecase) l3Eligible(ctx context.Context, sim *SimHashResult) bool {
	return u.embedder != nil && u.embedder.Enabled() && u.cfg.EmbeddingStoreEnabled &&
		sim != nil && sim.Tokens >= 12 && u.embedder.DailyRemaining(ctx) > 0
}

// resolveL2 L2 判定：召回候选去自引用，逐条算汉明；命中不同开簇时按成员最多为主并簇（03 §4.4）。
func (u *IcDedupUsecase) resolveL2(ctx context.Context, row *IntelItemRow, sim *SimHashResult) (*DedupOutcome, error) {
	cands, err := u.intel.FindSimHashCandidates(ctx, sim.Segments, u.cfg.DedupMergeWindow, 200)
	if err != nil {
		return nil, err
	}
	threshold := int(u.cfg.SimhashThreshold)
	// clusterID(open 或空) → 最优候选（汉明最小）
	best := map[string]*IntelItemRow{}
	bestDist := map[string]int{}
	for _, c := range cands {
		if c.IntelID == row.IntelID {
			continue
		}
		fp, perr := strconv.ParseUint(c.Dedup.Fingerprint, 16, 64)
		if perr != nil {
			continue
		}
		d := HammingDistance(sim.Fingerprint, fp)
		if d > threshold {
			continue
		}
		cid, ok := u.openClusterOf(ctx, c)
		if !ok {
			continue // 闭簇不吸纳新成员（S-2）
		}
		if cur, seen := bestDist[cid]; !seen || d < cur {
			best[cid], bestDist[cid] = c, d
		}
	}
	if len(best) == 0 {
		return nil, nil
	}
	target, matched, matchDist := u.pickDominantCluster(ctx, best, bestDist)
	if target == "" {
		return nil, nil
	}
	out, err := u.mergeIntoCluster(ctx, row, target, matched.IntelID, "L2", 1.0-float64(matchDist)/64.0)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil // 对端竞态不可见 → 按未命中交回 L3
	}
	// 跨簇并入（其余命中簇 → 主簇；S-3 同事务 + 事件落库在 data 层）
	for cid := range best {
		if cid == "" || cid == target {
			continue
		}
		if err := u.clusters.MergeTx(ctx, cid, target, "dedup:L2 multi-cluster collision", "system:dedup"); err != nil {
			return nil, err
		}
		u.log.Infof("merged cluster %s into %s (L2 collision)", cid, target)
	}
	return out, nil
}

// resolveL3 L3 语义相似：写向量 → 召回 → 并入。
func (u *IcDedupUsecase) resolveL3(ctx context.Context, row *IntelItemRow, sim *SimHashResult) (*DedupOutcome, error) {
	if !u.l3Eligible(ctx, sim) {
		return nil, nil
	}
	corpus := row.Payload.Title
	if row.Payload.Text != "" {
		corpus += "\n" + row.Payload.Text
	}
	embeddingID, err := u.embedder.Embed(ctx, row.IntelID, corpus)
	if err != nil {
		// B-7 配额耗尽/服务故障：跳过 L3 是设计语义（不是错误），仅观测。
		u.log.Warnf("embed failed, skip L3 (intel=%s): %s", row.IntelID, err.Error())
		return nil, nil
	}
	neighbors, err := u.embedder.SearchSimilar(ctx, row.IntelID, u.cfg.EmbeddingThreshold, u.cfg.EmbeddingWindowHours)
	if err != nil {
		u.log.Warnf("embedding search failed, treat as miss (intel=%s): %s", row.IntelID, err.Error())
		return &DedupOutcome{EmbeddingID: embeddingID}, errSkipL3NoMatch
	}
	for _, n := range neighbors {
		peer, gerr := u.intel.GetByID(ctx, n.IntelID)
		if gerr != nil {
			return nil, gerr
		}
		if peer == nil {
			continue
		}
		cid, ok := u.openClusterOf(ctx, peer)
		if !ok {
			continue
		}
		out, merr := u.mergeIntoCluster(ctx, row, cid, peer.IntelID, "L3", n.Score)
		if merr != nil {
			return nil, merr
		}
		if out == nil {
			// 对端竞态不可见（peer 在两次读之间被清扫/删除）：与 L2 同口径按未命中
			// 继续扫描下一邻居——不得解引用 out（评审坐实 nil deref panic）。
			continue
		}
		out.EmbeddingID = embeddingID
		return out, nil
	}
	return &DedupOutcome{EmbeddingID: embeddingID}, errSkipL3NoMatch
}

// errSkipL3NoMatch 内部流控哨兵：L3 跑了但没命中（≠ 错误）。
var errSkipL3NoMatch = icErrorSentinel("l3 no match")

type icSentinel string

func (s icSentinel) Error() string   { return string(s) }
func icErrorSentinel(s string) error { return icSentinel(s) }

// openClusterOf 取候选条目的开簇；空指针候选（尚未分配簇）按「开簇待建」返回空串 + true。
func (u *IcDedupUsecase) openClusterOf(ctx context.Context, item *IntelItemRow) (string, bool) {
	cid := item.Dedup.ClusterID
	if cid == "" {
		return "", true
	}
	cl, err := u.clusters.GetByID(ctx, cid)
	if err != nil || cl == nil || cl.Status != "open" {
		return "", false
	}
	return cid, true
}

// pickDominantCluster 命中多簇时取成员最多为主（03 §4.4）；成员数并列取簇 ID 小者（确定性）。
func (u *IcDedupUsecase) pickDominantCluster(ctx context.Context, best map[string]*IntelItemRow, bestDist map[string]int) (target string, matched *IntelItemRow, matchDist int) {
	var targetMembers int32 = -1
	for cid, cand := range best {
		var members int32
		if cid != "" {
			if cl, err := u.clusters.GetByID(ctx, cid); err == nil && cl != nil {
				members = cl.MemberCount
			}
		}
		if members > targetMembers || (members == targetMembers && (target == "" || cid < target)) {
			target, matched, matchDist, targetMembers = cid, cand, bestDist[cid], members
		}
	}
	return target, matched, matchDist
}

// mergeIntoCluster 并入指定簇（簇为空 → 先建簇把双方一起纳入）。
// 簇成员刷新与统计以 DB 实际为准（RecomputeStats），抽样留审计。
func (u *IcDedupUsecase) mergeIntoCluster(ctx context.Context, row *IntelItemRow, clusterID, matchedIntelID, level string, simScore float64) (*DedupOutcome, error) {
	if clusterID == "" {
		// 命中的条目自己还没簇（归一化乱序）：新建簇，双方一起纳入。
		if matchedIntelID != "" {
			peer, err := u.intel.GetByID(ctx, matchedIntelID)
			if err != nil {
				return nil, err
			}
			if peer != nil {
				cluster, cerr := u.clusters.Create(ctx, &IntelClusterRow{
					ID:               contract.NewClusterID(),
					Label:            truncateRunes(peer.Payload.Title, 64),
					MemberCount:      2,
					PlatformCount:    1,
					RepresentativeID: peer.IntelID,
					Region:           string(row.Source.Region),
					FirstSeenAt:      minTime(peer.Source.CollectedAt, row.Source.CollectedAt),
					LastSeenAt:       maxTime(peer.Source.CollectedAt, row.Source.CollectedAt),
					Status:           "open",
				})
				if cerr != nil {
					return nil, cerr
				}
				clusterID = cluster.ID
				if err = u.intel.AssignCluster(ctx, []string{peer.IntelID}, clusterID); err != nil {
					return nil, err
				}
			}
		}
		if clusterID == "" {
			// 对端不可见（极罕见竞态）：降级为新建簇。
			return nil, nil
		}
	}
	if err := u.intel.AssignCluster(ctx, []string{row.IntelID}, clusterID); err != nil {
		return nil, err
	}
	if err := u.clusters.TouchSeen(ctx, clusterID, row.Source.CollectedAt.UTC()); err != nil {
		return nil, err
	}
	if err := u.clusters.RecomputeStats(ctx, clusterID); err != nil {
		return nil, err
	}
	if err := u.samples.Create(ctx, &DedupSampleRow{
		IntelID: row.IntelID, ClusterID: clusterID,
		Action: "auto_merge", Level: level, SimScore: simScore, Operator: "system:dedup",
	}); err != nil {
		u.log.Warnf("dedup sample write failed (non-blocking): %s", err.Error())
	}
	return &DedupOutcome{Action: "merged", ClusterID: clusterID, Level: level, SimScore: simScore, MatchedIntelID: matchedIntelID}, nil
}

// ForceL1Merge 外部触发的 L1 合并（短链展开换 key 撞车场景：本行被判定为同 URL 迟到采样）。
// S-2 同 L1 主路径：peer 所在簇已闭则不并入（返回 merged=false 由调用方按未合并继续）。
func (u *IcDedupUsecase) ForceL1Merge(ctx context.Context, row *IntelItemRow, peer *IntelItemRow) (*DedupOutcome, error) {
	cid, open := u.openClusterOf(ctx, peer)
	if !open {
		return nil, nil
	}
	return u.mergeIntoCluster(ctx, row, cid, peer.IntelID, "L1", 1.0)
}

// ---------- 人工调参（S-23：Split/Merge API 的 biz 入口） ----------

// MergeClusters 人工并簇（审计：operator 必填）。
func (u *IcDedupUsecase) MergeClusters(ctx context.Context, fromID, toID, operator string) error {
	if fromID == toID {
		return nil
	}
	if operator == "" {
		operator = "unknown"
	}
	if err := u.clusters.MergeTx(ctx, fromID, toID, "manual", operator); err != nil {
		return err
	}
	return u.samples.Create(ctx, &DedupSampleRow{
		ClusterID: toID, Action: "manual_merge", Level: "", Operator: operator,
	})
}

// SplitCluster 人工劈簇（S-23；member 移入新簇并重算两簇统计在 data 层）。
func (u *IcDedupUsecase) SplitCluster(ctx context.Context, fromID string, intelIDs []string, operator, label string) (string, error) {
	if len(intelIDs) == 0 {
		return "", ErrIcInvalidArgument
	}
	if operator == "" {
		operator = "unknown"
	}
	// SplitTx 事务内已为每个成员写 manual_split 审计样本（M-27 黄金集评审依据，
	// ic_cluster_repo.go:336-350）；此处不再重复写——否则一次 split 每成员双份，
	// 黄金集回放按样本统计人工干预量时翻倍失真。
	return u.clusters.SplitTx(ctx, fromID, intelIDs, operator, label)
}

var ErrIcInvalidArgument = icSentinel("invalid argument")

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
