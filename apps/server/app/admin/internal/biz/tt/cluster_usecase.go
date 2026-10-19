package tt

import (
	"context"
	"fmt"
	"math"
	"time"
)

// ============================================================================
// 话题聚类编排（评审 B-1/A-1 Step2→Step4）：召回 → 打分 → 三段决策 → 落位。
//
// 域模型对齐（重要——TT 无独立"簇"实体）：
//  - Topic 即归并单元；「归入」= 向既有话题 AttachSource（TopicSource.similarity=s），
//    复用 ingest 已有的 TopicRepo.AttachSource（(topic_id,intel_id) 幂等）；
//  - Step1 硬键（TopicSource.cluster_key 精确命中）在 ingest 前置完成，
//    走到本层的一定是硬键未命中的条目；
//  - review 档 = 同样 AttachSource + 置话题 needs_review=true（独立小端口，
//    不扩 TopicRepo 避免既有 fake 连锁失效）；
//  - Step5 的 merge_lock/moved_by/moved_at 是人工合并/拆分路径的留痕，
//    自动归入不写 moved_by（schema 注释：空=自动聚类归入）。
//
// 侧壁：
//  - 候选集构造（活跃未锁 merge_lock=false、|collected−last_seen|≤7d、
//    实体倒排 ∪ SimHash 桶、上限 200）由 ClusterRepo 保证；
//  - embedding 为可选端口：nil 时打分走 A-6 式权重重归一（见 ScoreTopicPair）；
//  - 决策为 new 时本层只回传信号，建新话题仍走 ingest 既有路径（单一写入口）。
// ============================================================================

// ClusterRepo 聚类召回的只读端口（data/tt 实现：topics×sources×intel 三表联查）。
type ClusterRepo interface {
	// FindClusterCandidates Step2 召回：活跃未锁话题的近 7d 来源条目特征，
	// 实体倒排命中 ∨ 任一 SimHash 段等值，行数 ≤ limit（评审 A-1：200）。
	// 同一话题可返回多行（每个命中来源一行），argmax 天然取话题最优。
	FindClusterCandidates(ctx context.Context, q CandidateQuery) ([]ClusterCandidateRow, error)
}

// NeedsReviewStore review 档置标端口（窄接口：只为聚类 review 路径服务）。
type NeedsReviewStore interface {
	// MarkNeedsReview 置 topic.needs_review=true（幂等）。
	MarkNeedsReview(ctx context.Context, topicID string) error
}

// CandidateQuery Step2 召回查询参数。
type CandidateQuery struct {
	Entities  []string  // 规范化实体串（confidence≥0.6 过滤后）
	Segments  [4]int16  // 标题 SimHash 四段（B-26 pigeonhole）
	From, To  time.Time // 时间窗（A-1：|collected_at − last_seen_at|≤7d）
	Limit     int       // 上限 200
	ExcludeID string    // 重算场景排除来源条目自身（it_ ID；空=不排除）
}

// ClusterCandidateRow 召回产出的候选行 = 话题 × 其一条命中来源条目的特征
// （已含打分所需的全部字段，避免 N+1 回表）。
type ClusterCandidateRow struct {
	TopicID  string   // tp_ 前缀
	Lang     string   // 来源条目语言脚本
	Fp       uint64   // 来源条目标题 SimHash64（本包 TitleSimHash64 参数族）
	Entities []string // 来源条目规范化实体串
}

// ClusterInput 一次聚类请求（Step1 硬键未命中的条目特征）。
type ClusterInput struct {
	IntelID    string    // it_ 前缀（merge/review 时落 AttachSource）
	ClusterKey string    // 可空（IC dedup.cluster_id，归入时落 TopicSource.cluster_key）
	Platform   string    // 来源平台（A-5 breadth 统计维度）
	Entities   []string  // confidence≥0.6 过滤后的规范化实体
	Lang       string    // 语言脚本（跨语言判定依据）
	Fp         uint64    // 标题 SimHash64
	Embedding  []float32 // 多语言 embedding；nil 或空切片=无通道（逐对重归一）
	// CollectedAt 条目样本时间（UTC）。时间窗：CollectedAt±窗宽（A-1：|Δ|≤7d 双向）。
	CollectedAt time.Time
	// ExcludeID 重算本条目时排除自身（it_ ID；空=不排除）。
	ExcludeID string
	// EmbeddingByTopic 话题代表向量提供者（可选；多语言/既有缓存接线用）：
	// 返回该话题的来源代表向量。缺失或长度不齐 → 该对无 embedding 通道。
	EmbeddingByTopic func(ctx context.Context, topicID string) ([]float32, error)
}

// ClusterOutcome 编排结果。
type ClusterOutcome struct {
	Decision ClusterDecision // merge / review / new
	TopicID  string          // 最佳候选话题 tp_ ID（new 时空串）
	Score    float64         // 最佳相似度
	Scanned  int             // 参与打分的候选行数
}

// Embedder 多语言 embedding 端口（可选）。余弦在本层算，端口只取向量。
type Embedder interface {
	// EmbedItem 条目（或话题代表文本）的多语言向量；实现侧可缓存。
	EmbedItem(ctx context.Context, id string) ([]float32, error)
}

// ClusterConfExtra 编排层阈值（召回窗等；打分阈值在 ClusterConf）。
type ClusterConfExtra struct {
	RecallWindowDays float64 // A-1：±7d
	RecallLimit      int     // A-1：200
}

// DefaultClusterConfExtra 评审终稿默认。
func DefaultClusterConfExtra() ClusterConfExtra {
	return ClusterConfExtra{RecallWindowDays: 7, RecallLimit: 200}
}

// ClusterUsecase 聚类编排器。
type ClusterUsecase struct {
	recall ClusterRepo
	topics TopicRepo // 复用既有端口：AttachSource 幂等落位
	review NeedsReviewStore
	emb    Embedder // 可 nil
	conf   ClusterConf
	extra  ClusterConfExtra
}

// NewClusterUsecase 组装（emb 允许 nil：无提供方时打分自动重归一）。
// conf 必须过 Validate 红线（零值/漂移配置静默灾变，绝不允许带病构造）；
// 测试构造的退化配置请直接 &ClusterUsecase{} 或显式绕过（知情的旁路）。
func NewClusterUsecase(recall ClusterRepo, topics TopicRepo, review NeedsReviewStore,
	emb Embedder, conf ClusterConf, extra ClusterConfExtra) (*ClusterUsecase, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	if extra.RecallWindowDays <= 0 || extra.RecallLimit <= 0 {
		return nil, fmt.Errorf("tt: cluster recall window/limit must be > 0: %+v", extra)
	}
	return &ClusterUsecase{recall: recall, topics: topics, review: review, emb: emb, conf: conf, extra: extra}, nil
}

// Resolve 对一条硬键未命中的条目执行 Step2→Step4。决策为 merge/review 时
// 直接落位（AttachSource 幂等 + review 档置 needs_review），否则仅回传 new
// 信号由 ingest 建新话题。
func (u *ClusterUsecase) Resolve(ctx context.Context, in ClusterInput) (ClusterOutcome, error) {
	out := ClusterOutcome{Decision: ClusterNew}

	win := time.Duration(u.extra.RecallWindowDays*24) * time.Hour
	cands, err := u.recall.FindClusterCandidates(ctx, CandidateQuery{
		Entities:  in.Entities,
		Segments:  SimHashSegments(in.Fp),
		From:      in.CollectedAt.Add(-win),
		To:        in.CollectedAt.Add(win),
		Limit:     u.extra.RecallLimit,
		ExcludeID: in.ExcludeID,
	})
	if err != nil {
		return out, err
	}
	if len(cands) == 0 {
		return out, nil
	}

	// 全量候选打分（行数 ≤200，规模可控），argmax 取最优。
	var (
		best    ClusterCandidateRow
		bestS   = -1.0
		bestDec = ClusterNew
	)
	// 通道有效性按「对」判定（评审对抗裁决发现：空/不齐向量曾 cos≡0 仍占 0.30
	// 权重，使 merge 上限 0.70 永不可达）。逐对评估，无效对 → 该对按两路重归一。
	for _, c := range cands {
		var cos float64
		pairHasEmb := false
		if in.EmbeddingByTopic != nil {
			cv, err := in.EmbeddingByTopic(ctx, c.TopicID)
			if err != nil {
				return out, err
			}
			cos, pairHasEmb = cosFromPair(in.Embedding, cv)
		} else if u.emb != nil && len(in.Embedding) > 0 {
			cv, err := u.emb.EmbedItem(ctx, c.TopicID)
			if err != nil {
				return out, err
			}
			cos, pairHasEmb = cosFromPair(in.Embedding, cv)
		}
		s, dec := ScoreTopicPair(SimParts{
			EntityJaccard: JaccardEntities(in.Entities, c.Entities),
			SimHashSim:    SimHashSim(in.Fp, c.Fp),
			Cosine:        cos,
			HasEmbedding:  pairHasEmb,
			SameLang:      c.Lang == in.Lang,
		}, u.conf)
		if s > bestS {
			best, bestS, bestDec = c, s, dec
		}
	}

	out.Score = positiveOrZero(bestS)
	out.Scanned = len(cands)
	if bestDec == ClusterNew {
		return out, nil
	}
	out.Decision = bestDec
	out.TopicID = best.TopicID

	// Step4 落位：归入 = AttachSource（(topic_id,intel_id) 幂等，similarity=s）。
	if err := u.topics.AttachSource(ctx, best.TopicID, AttachSourceInput{
		IntelID:       in.IntelID,
		ClusterKey:    in.ClusterKey,
		Platform:      in.Platform,
		ContributedAt: in.CollectedAt,
		Similarity:    bestS,
	}); err != nil {
		return out, err
	}
	// review 档：置话题 needs_review=true（Step4：归入并置标）。
	if bestDec == ClusterReview {
		if err := u.review.MarkNeedsReview(ctx, best.TopicID); err != nil {
			return out, err
		}
	}
	return out, nil
}

// positiveOrZero argmax 轨迹值守门（候选必非空到此，防御 -1 哨兵外泄）。
func positiveOrZero(v float64) float64 {
	if v < 0 {
		return 0
	}
	return v
}

// cosine32 float32 余弦；零向量任一侧或长度不齐 → 0（无信息态不得产出相似度）。
func cosine32(a, b []float32) float64 {
	if len(a) == 0 || len(b) != len(a) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

// cosFromPair 判定「该对」的 embedding 通道是否真实可用（非空、等长、非零向量），
// 可用时返回余弦与 true；不可用返回 0/false，由打分器按两路重归一（A-6 同构）。
func cosFromPair(a, b []float32) (float64, bool) {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0, false
	}
	var dot, na, nb float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0, false
	}
	return dot / math.Sqrt(na*nb), true
}
