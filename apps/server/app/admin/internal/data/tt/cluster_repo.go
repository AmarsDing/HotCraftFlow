package ttdata

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/entityindex"
	"framework/app/admin/internal/data/ent/topic"
	"framework/app/admin/internal/data/ent/topicsource"
	biztt "framework/app/admin/internal/biz/tt"
)

// EntClusterRepo biz/tt.ClusterRepo + NeedsReviewStore 的 Ent 实现。
//
// 纪律：
//   - 双通道召回：实体倒排（tt_entity_index）∪ SimHash 分段桶（tt_topic_sources.simhash_seg0-3）；
//   - 窗口过滤：候选话题的 contributed_at 必须在 [q.From, q.To]（A-1：|Δ|≤7d）；
//   - 锁定/归档话题不召回（merge_lock=false ∧ is_archived=false）；
//   - 同一话题可返回多行（每命中来源一行），由 biz 层 argmax 取最优。
type EntClusterRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntClusterRepo 构造。
func NewEntClusterRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntClusterRepo {
	return &EntClusterRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/tt-cluster-repo")),
	}
}

// FindClusterCandidates Step2 召回：实体倒排 ∪ SimHash 分段桶，返回候选话题 × 命中来源行。
func (r *EntClusterRepo) FindClusterCandidates(ctx context.Context, q biztt.CandidateQuery) ([]biztt.ClusterCandidateRow, error) {
	if q.Limit <= 0 {
		q.Limit = 200 // A-1 默认上限
	}

	// 通道①：实体倒排（entity_key IN q.Entities ∧ indexed_at ≥ q.From）
	topicIDsFromEntities := make(map[string]struct{})
	if len(q.Entities) > 0 {
		rows, err := r.cli.Client().EntityIndex.Query().
			Where(
				entityindex.EntityKeyIn(q.Entities...),
				entityindex.IndexedAtGTE(q.From),
				entityindex.IndexedAtLTE(q.To),
			).
			Select(entityindex.FieldTopicID).
			Strings(ctx)
		if err != nil {
			return nil, err
		}
		for _, tid := range rows {
			topicIDsFromEntities[tid] = struct{}{}
		}
	}

	// 通道②：SimHash 分段桶（任一段等值）
	topicIDsFromSimHash := make(map[string]struct{})
	if q.Segments != [4]int16{0, 0, 0, 0} {
		srcs, err := r.cli.Client().TopicSource.Query().
			Where(
				topicsource.Or(
					topicsource.SimhashSeg0(q.Segments[0]),
					topicsource.SimhashSeg1(q.Segments[1]),
					topicsource.SimhashSeg2(q.Segments[2]),
					topicsource.SimhashSeg3(q.Segments[3]),
				),
				topicsource.ContributedAtGTE(q.From),
				topicsource.ContributedAtLTE(q.To),
			).
			Select(topicsource.FieldTopicID, topicsource.FieldTitleSimhash).
			Limit(q.Limit).
			All(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range srcs {
			topicIDsFromSimHash[s.TopicID] = struct{}{}
		}
	}

	// 合并去重候选 topic_id 集合
	candidateTopicIDs := make(map[string]struct{}, len(topicIDsFromEntities)+len(topicIDsFromSimHash))
	for tid := range topicIDsFromEntities {
		candidateTopicIDs[tid] = struct{}{}
	}
	for tid := range topicIDsFromSimHash {
		candidateTopicIDs[tid] = struct{}{}
	}

	if len(candidateTopicIDs) == 0 {
		return nil, nil
	}

	// 过滤 merge_lock / is_archived，并取回话题特征（lang, fp, entities）
	topicIDSlice := make([]string, 0, len(candidateTopicIDs))
	for tid := range candidateTopicIDs {
		topicIDSlice = append(topicIDSlice, tid)
	}

	// 查询话题的最近来源（代表特征：lang, title_simhash, entities）
	// 注：同一话题可能有多个来源，这里取每个话题最近一条来源作为代表
	sources, err := r.cli.Client().TopicSource.Query().
		Where(
			topicsource.TopicIDIn(topicIDSlice...),
			topicsource.ContributedAtGTE(q.From),
			topicsource.ContributedAtLTE(q.To),
		).
		Order(ent.Desc(topicsource.FieldContributedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// 按 topic_id 分组，每组取最新一条
	topicLatestSource := make(map[string]*ent.TopicSource)
	for _, s := range sources {
		if existing, ok := topicLatestSource[s.TopicID]; !ok || s.ContributedAt.After(existing.ContributedAt) {
			topicLatestSource[s.TopicID] = s
		}
	}

	// 查询话题元数据（过滤 merge_lock / is_archived）
	topics, err := r.cli.Client().Topic.Query().
		Where(
			topic.TopicIDIn(topicIDSlice...),
			topic.MergeLockEQ(false),
			topic.IsArchivedEQ(false),
		).
		Select(topic.FieldTopicID, topic.FieldLabel).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// 组装候选行（每个话题一行，取代表来源的特征）
	out := make([]biztt.ClusterCandidateRow, 0, len(topics))
	for _, t := range topics {
		src, ok := topicLatestSource[t.TopicID]
		if !ok {
			continue // 该话题在窗口内无来源（可能仅实体索引命中但来源已超窗）
		}

		// 从 EntityIndex 查该话题的实体集合（近 7d）
		entities, err := r.cli.Client().EntityIndex.Query().
			Where(
				entityindex.TopicID(t.TopicID),
				entityindex.IndexedAtGTE(q.From),
			).
			Select(entityindex.FieldEntityKey).
			Strings(ctx)
		if err != nil {
			r.log.Warnf("query entities for topic %s failed: %v", t.TopicID, err)
			entities = []string{} // 降级：无实体通道
		}

		// 语言脚本：从来源平台推断（简化；完整实现需 IntelItem 回查）
		// TODO: 完整实现需在 TopicSource 加 lang 字段（当前 schema 未存）
		// 临时方案：从 title_simhash 是否有值推断（中文标题 bigram 特征）
		lang := "zh" // 默认中文
		if src.TitleSimhash == 0 {
			lang = "unknown"
		}

		out = append(out, biztt.ClusterCandidateRow{
			TopicID:  t.TopicID,
			Lang:     lang,
			Fp:       src.TitleSimhash,
			Entities: entities,
		})

		if len(out) >= q.Limit {
			break
		}
	}

	return out, nil
}

// MarkNeedsReview 置话题 needs_review=true（幂等）。
func (r *EntClusterRepo) MarkNeedsReview(ctx context.Context, topicID string) error {
	return r.cli.Client().Topic.Update().
		Where(topic.TopicID(topicID)).
		SetNeedsReview(true).
		Exec(ctx)
}

// IndexEntities 写入实体倒排（供 ingest 调用）。
// entities 为 confidence≥0.6 过滤后的规范化实体串列表。
func (r *EntClusterRepo) IndexEntities(ctx context.Context, topicID, intelID string, entities []string, indexedAt time.Time) error {
	if len(entities) == 0 {
		return nil
	}

	bulk := make([]*ent.EntityIndexCreate, 0, len(entities))
	for _, ek := range entities {
		bulk = append(bulk, r.cli.Client().EntityIndex.Create().
			SetEntityKey(ek).
			SetTopicID(topicID).
			SetIntelID(intelID).
			SetIndexedAt(indexedAt))
	}

	// 批量插入；唯一约束冲突（同 entity_key+topic_id+intel_id）忽略（幂等重放）
	err := r.cli.Client().EntityIndex.CreateBulk(bulk...).
		OnConflict().
		Ignore().
		Exec(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		return err
	}
	return nil
}

// CleanupExpiredEntities 清理超窗实体索引（7d 窗口；由独立清理任务周期调用）。
func (r *EntClusterRepo) CleanupExpiredEntities(ctx context.Context, before time.Time) (int, error) {
	n, err := r.cli.Client().EntityIndex.Delete().
		Where(entityindex.IndexedAtLT(before)).
		Exec(ctx)
	return n, err
}

var _ biztt.ClusterRepo = (*EntClusterRepo)(nil)
var _ biztt.NeedsReviewStore = (*EntClusterRepo)(nil)
