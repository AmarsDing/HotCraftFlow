package ttdata

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/topic"
	"framework/app/admin/internal/data/ent/topicsource"
	biztt "framework/app/admin/internal/biz/tt"
	"framework/pkg/mediacontract"
)

// EntTopicAdminRepo biz/tt.TopicAdminRepo 的 Ent 实现。
//
// 纪律：
//   - 合并/拆分事务化：来源行 UPDATE + 话题行 UPDATE 必须在同一事务；
//   - 拆分产生的新话题初始 merge_lock=true（人工产物，不再自动吸附）；
//   - 合并后源话题 is_archived=true（保留历史，软删）；
//   - 所有 moved_by/moved_at 落库（B-1 Step5 审计链）。
type EntTopicAdminRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntTopicAdminRepo 构造。
func NewEntTopicAdminRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntTopicAdminRepo {
	return &EntTopicAdminRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/tt-topic-admin-repo")),
	}
}

// ListNeedsReviewTopics 复核队列（needs_review=true 且未归档）。
func (r *EntTopicAdminRepo) ListNeedsReviewTopics(ctx context.Context, page, pageSize int) ([]biztt.TopicAdminView, int64, error) {
	q := r.cli.Client().Topic.Query().
		Where(
			topic.NeedsReviewEQ(true),
			topic.IsArchivedEQ(false),
		)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := q.
		Order(ent.Desc(topic.FieldLastSeenAt)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	items := make([]biztt.TopicAdminView, 0, len(rows))
	for _, t := range rows {
		// 统计来源数（二级查询，量级可控）
		count, err := r.cli.Client().TopicSource.Query().
			Where(topicsource.TopicID(t.TopicID)).
			Count(ctx)
		if err != nil {
			r.log.Warnf("count sources for topic %s failed: %v", t.TopicID, err)
			count = 0
		}

		items = append(items, biztt.TopicAdminView{
			TopicID:       t.TopicID,
			Label:         t.Label,
			Category:      string(t.Category),
			Stage:         string(t.Stage),
			HeatScore:     t.HeatScore,
			Velocity:      t.Velocity,
			PlatformCount: t.PlatformCount,
			MergeLock:     t.MergeLock,
			NeedsReview:   t.NeedsReview,
			IsArchived:    t.IsArchived,
			FirstSeenAt:   t.FirstSeenAt.UTC(),
			LastSeenAt:    t.LastSeenAt.UTC(),
			SourceCount:   count,
		})
	}
	return items, int64(total), nil
}

// GetTopicDetail 话题详情 + 来源列表。
func (r *EntTopicAdminRepo) GetTopicDetail(ctx context.Context, topicID string) (biztt.TopicDetailView, error) {
	var out biztt.TopicDetailView

	t, err := r.cli.Client().Topic.Query().
		Where(topic.TopicID(topicID)).
		First(ctx)
	if err != nil {
		return out, err
	}

	sources, err := r.cli.Client().TopicSource.Query().
		Where(topicsource.TopicID(topicID)).
		Order(ent.Desc(topicsource.FieldContributedAt)).
		All(ctx)
	if err != nil {
		return out, err
	}

	out.Topic = biztt.TopicAdminView{
		TopicID:       t.TopicID,
		Label:         t.Label,
		Category:      string(t.Category),
		Stage:         string(t.Stage),
		HeatScore:     t.HeatScore,
		Velocity:      t.Velocity,
		PlatformCount: t.PlatformCount,
		MergeLock:     t.MergeLock,
		NeedsReview:   t.NeedsReview,
		IsArchived:    t.IsArchived,
		FirstSeenAt:   t.FirstSeenAt.UTC(),
		LastSeenAt:    t.LastSeenAt.UTC(),
		SourceCount:   len(sources),
	}

	out.Sources = make([]biztt.TopicSourceView, 0, len(sources))
	for _, s := range sources {
		movedBy := ""
		if s.MovedBy != nil {
			movedBy = *s.MovedBy
		}
		clusterKey := ""
		if s.ClusterKey != nil {
			clusterKey = *s.ClusterKey
		}
		out.Sources = append(out.Sources, biztt.TopicSourceView{
			IntelID:       s.IntelID,
			Platform:      s.Platform,
			ContributedAt: s.ContributedAt.UTC(),
			Similarity:    s.Similarity,
			ClusterKey:    clusterKey,
			MovedBy:       movedBy,
		})
	}
	return out, nil
}

// SplitTopicTx 事务拆分话题。
func (r *EntTopicAdminRepo) SplitTopicTx(ctx context.Context, in biztt.SplitTopicInput) (string, error) {
	newTopicID := mediacontract.NewID("tp")
	now := time.Now().UTC()

	err := runInTx(ctx, r.cli.Client(), func(tx *ent.Tx) error {
		// 1. 校验原话题存在且未归档
		srcTopic, err := tx.Topic.Query().
			Where(topic.TopicID(in.TopicID)).
			First(ctx)
		if err != nil {
			return err
		}
		if srcTopic.IsArchived {
			return errors.New("tt: cannot split archived topic")
		}

		// 2. 校验所有 intel_id 属于原话题
		sources, err := tx.TopicSource.Query().
			Where(
				topicsource.TopicID(in.TopicID),
				topicsource.IntelIDIn(in.IntelIDs...),
			).
			All(ctx)
		if err != nil {
			return err
		}
		if len(sources) != len(in.IntelIDs) {
			return errors.New("tt: some intel_ids not found in source topic")
		}

		// 3. 新建子话题（继承原话题 label/category/region，merge_lock=true）
		firstSource := sources[0]
		newTopic := tx.Topic.Create().
			SetTopicID(newTopicID).
			SetLabel(srcTopic.Label + " (拆分)").
			SetCategory(srcTopic.Category).
			SetRegion(srcTopic.Region).
			SetStage(srcTopic.Stage).
			SetStageSince(now).
			SetStageDurationPoints(0).
			SetHeatScore(srcTopic.HeatScore).
			SetVelocity(srcTopic.Velocity).
			SetFirstSeenAt(firstSource.ContributedAt).
			SetLastSeenAt(firstSource.ContributedAt).
			SetMergeLock(true). // 人工产物
			SetRepresentativeIntelID(firstSource.IntelID).
			SetOrigin(map[string]interface{}{
				"split_from": in.TopicID,
				"reason":     in.Reason,
				"operator":   in.Operator,
			})
		if _, err := newTopic.Save(ctx); err != nil {
			return err
		}

		// 4. 移动来源行（更新 topic_id + moved_by/moved_at）
		for _, src := range sources {
			err := tx.TopicSource.Update().
				Where(topicsource.ID(src.ID)).
				SetTopicID(newTopicID).
				SetMovedBy(in.Operator).
				SetMovedAt(now).
				Exec(ctx)
			if err != nil {
				return err
			}
		}

		// 5. 原话题 merge_lock=true（拆分后锁定，防自动聚类干扰）
		err = tx.Topic.Update().
			Where(topic.TopicID(in.TopicID)).
			SetMergeLock(true).
			Exec(ctx)
		return err
	})

	if err != nil {
		return "", err
	}

	r.log.Infow("topic_split",
		"src_topic_id", in.TopicID,
		"new_topic_id", newTopicID,
		"moved_sources", len(in.IntelIDs),
		"operator", in.Operator,
		"reason", in.Reason,
	)
	return newTopicID, nil
}

// MergeTopicsTx 事务合并话题。
func (r *EntTopicAdminRepo) MergeTopicsTx(ctx context.Context, in biztt.MergeTopicsInput) (int, error) {
	now := time.Now().UTC()
	mergedCount := 0

	err := runInTx(ctx, r.cli.Client(), func(tx *ent.Tx) error {
		// 1. 校验目标话题存在且未归档
		target, err := tx.Topic.Query().
			Where(topic.TopicID(in.TargetTopicID)).
			First(ctx)
		if err != nil {
			return err
		}
		if target.IsArchived {
			return errors.New("tt: cannot merge into archived topic")
		}

		// 2. 逐个处理源话题
		for _, srcTopicID := range in.SourceTopicIDs {
			src, err := tx.Topic.Query().
				Where(topic.TopicID(srcTopicID)).
				First(ctx)
			if err != nil {
				return err
			}
			if src.IsArchived {
				continue // 幂等：已归档跳过
			}

			// 3. 移动全部来源到目标话题
			n, err := tx.TopicSource.Update().
				Where(topicsource.TopicID(srcTopicID)).
				SetTopicID(in.TargetTopicID).
				SetMovedBy(in.Operator).
				SetMovedAt(now).
				Save(ctx)
			if err != nil {
				return err
			}
			mergedCount += n

			// 4. 源话题标记归档（保留历史，软删）
			err = tx.Topic.Update().
				Where(topic.TopicID(srcTopicID)).
				SetIsArchived(true).
				SetMergeLock(true). // 锁定归档话题
				Exec(ctx)
			if err != nil {
				return err
			}
		}

		// 5. 目标话题 merge_lock=true（人工合并后锁定）
		err = tx.Topic.Update().
			Where(topic.TopicID(in.TargetTopicID)).
			SetMergeLock(true).
			Exec(ctx)
		return err
	})

	if err != nil {
		return 0, err
	}

	r.log.Infow("topics_merged",
		"target_topic_id", in.TargetTopicID,
		"source_topic_ids", in.SourceTopicIDs,
		"merged_sources", mergedCount,
		"operator", in.Operator,
		"reason", in.Reason,
	)
	return mergedCount, nil
}

// AckReviewConfirm 确认归并正确（needs_review=false）。
func (r *EntTopicAdminRepo) AckReviewConfirm(ctx context.Context, topicID string) error {
	return r.cli.Client().Topic.Update().
		Where(topic.TopicID(topicID)).
		SetNeedsReview(false).
		Exec(ctx)
}

var _ biztt.TopicAdminRepo = (*EntTopicAdminRepo)(nil)
