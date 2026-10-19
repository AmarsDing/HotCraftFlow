package tt

// ============================================================================
// 话题聚类打分与归并决策（评审 B-1/A-1 可编码方案 Step3/Step4/Step6）
//
// 侧壁约定：
//  - Step1 硬键归并在 ingest（M1 已落）；
//  - Step2 召回（实体倒排 + SimHash 四段桶 + 7d 时间窗，上限 200）在 repo 层，
//    本文件只吃召回产出的相似度分量；
//  - Step5 merge_lock 由 repo/工作器写 Topic.merge_lock；
//  - label 重选触发（每 6h 或 heat 变动 >10）同样在 worker，本文件只提供
//    选取规则本身（M-19/Step6 统一：gate=pass 优先 → workability 降序 →
//    互动降序 → collected_at 升序）。
//
// 跨语言（非同一语言脚本）：实体 Jaccard 与 SimHash 完全无效（评审原文），
// 唯一通道是 embedding；cos ≥ 0.72 才允许归入，否则一律新建。无 embedding
// 提供方时跨语言恒为新建——宁可多建也不乱并（误并污染 topic 曲线，无法回滚）。
// ============================================================================

// ClusterConf 聚类打分与决策阈值（A-1 终稿默认；网格搜索标定后入配置）。
type ClusterConf struct {
	// WJaccard / WSimHash / WCos 三路加权（Σ=1，0.45/0.25/0.30）。
	WJaccard float64
	WSimHash float64
	WCos     float64
	// ThetaMerge 自动归入阈值（≥0.82）。
	ThetaMerge float64
	// ThetaReview 人工复核阈值（0.70≤s<0.82 归入并置 needs_review）。
	ThetaReview float64
	// CrossLangCosMin 跨语言归入的 embedding 余弦下限（0.72）。
	CrossLangCosMin float64
}

// DefaultClusterConf 评审终稿默认值。
func DefaultClusterConf() ClusterConf {
	return ClusterConf{
		WJaccard: 0.45, WSimHash: 0.25, WCos: 0.30,
		ThetaMerge: 0.82, ThetaReview: 0.70, CrossLangCosMin: 0.72,
	}
}

// SimParts 一次候选话题对比的已算好相似度分量。
type SimParts struct {
	// EntityJaccard 实体集合 Jaccard（confidence≥0.6 的 (type,value) 过滤后）。
	EntityJaccard float64
	// SimHashSim = 1 - Hamming/64（标题 SimHash 相似度）。
	SimHashSim float64
	// Cosine embedding 余弦相似度（多语言模型）。
	Cosine float64
	// HasEmbedding 本次是否有 embedding 通道（无提供方/调用失败 → false）。
	HasEmbedding bool
	// SameLang 两文本是否同一语言脚本（非同一 → 仅 cos 通道有效）。
	SameLang bool
}

// ClusterDecision 归并决策（A-1 Step4 三段）。
type ClusterDecision string

const (
	// ClusterMerge s≥ThetaMerge：自动归入。
	ClusterMerge ClusterDecision = "merge"
	// ClusterReview ThetaReview≤s<ThetaMerge：归入候选但置 needs_review=true。
	ClusterReview ClusterDecision = "review"
	// ClusterNew s<ThetaReview：新建话题。
	ClusterNew ClusterDecision = "new"
)

// ScoreTopicPair 合成相似度并给出三段决策。
//
// 通道缺失处理（A-6 同构的权重重分配精神）：
//   - 无 embedding（HasEmbedding=false），同语言：0.45/0.25 两路按 Σw 重归一；
//   - 跨语言：实体与 SimHash 被判无效，不看加权分，仅按 cos≥CrossLangCosMin 判
//     merge/new（保守不设 review 档——复核通道对跨语言同样缺证据）;
//   - 跨语言且无 embedding：恒 new。
func ScoreTopicPair(p SimParts, c ClusterConf) (float64, ClusterDecision) {
	if !p.SameLang {
		if !p.HasEmbedding || p.Cosine < c.CrossLangCosMin {
			return p.Cosine, ClusterNew
		}
		return p.Cosine, ClusterMerge
	}

	wj, ws, wc := c.WJaccard, c.WSimHash, c.WCos
	var s float64
	if p.HasEmbedding {
		s = wj*p.EntityJaccard + ws*p.SimHashSim + wc*p.Cosine
	} else {
		sum := wj + ws
		if sum <= 0 {
			return 0, ClusterNew
		}
		s = (wj*p.EntityJaccard + ws*p.SimHashSim) / sum
	}

	switch {
	case s >= c.ThetaMerge:
		return s, ClusterMerge
	case s >= c.ThetaReview:
		return s, ClusterReview
	default:
		return s, ClusterNew
	}
}

// LabelCandidate label 选取候选（TopicSource 贡献的 IntelItem 投影）。
type LabelCandidate struct {
	Title       string
	GatePass    bool    // 契约闸门契约通过
	Workability float64 // B_material 可工作分（其余类目 0 也允许）
	Engagement  float64 // 互动量（likes+comments+shares+favorites，V1 likes+comments）
	CollectedAt int64   // Unix 秒（UTC）
}

// PickLabel M-19/Step6 选取规则：gate=pass 优先 → workability 降序 →
// 互动降序 → collected_at 升序（最早优先，稳定）。返回选中标题与是否含
// gate=pass 项；空候选返回 ("", false)。
func PickLabel(cands []LabelCandidate) (string, bool) {
	if len(cands) == 0 {
		return "", false
	}
	best := cands[0]
	for _, cur := range cands[1:] {
		if labelBeats(cur, best) {
			best = cur
		}
	}
	return best.Title, best.GatePass
}

// labelBeats 严格全序比较（false 方向勿用取反，平局一律后到者不输）。
func labelBeats(a, b LabelCandidate) bool {
	if a.GatePass != b.GatePass {
		return a.GatePass
	}
	if a.Workability != b.Workability {
		return a.Workability > b.Workability
	}
	if a.Engagement != b.Engagement {
		return a.Engagement > b.Engagement
	}
	return a.CollectedAt < b.CollectedAt // 升序：更早者优先
}
