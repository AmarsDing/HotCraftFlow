package tt

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ============================================================================
// 话题人工管理（Step5 merge_lock 写入路径 + needs_review 复核工作流）。
//
// 评审 B-1：
//  - 合并/拆分后置 merge_lock=true，自动聚类不再拆散（TopicSource.moved_by/moved_at
//    记录操作者与时间戳）；
//  - 拆分产生的新话题初始 merge_lock=true（人工产物，不再自动吸附）；
//  - 复核队列 = needs_review=true 的话题列表（Step4 review 档产物）；
//  - AckReview confirm：needs_review=false；split：引导前端拆分对话框。
// ============================================================================

// TopicAdminRepo 话题管理持久化端口（data/tt 实现）。
type TopicAdminRepo interface {
	// ListNeedsReviewTopics 复核队列（needs_review=true 且未归档）。
	ListNeedsReviewTopics(ctx context.Context, page, pageSize int) (items []TopicAdminView, total int64, err error)

	// GetTopicDetail 话题详情 + 来源列表（拆分对话框数据源）。
	GetTopicDetail(ctx context.Context, topicID string) (TopicDetailView, error)

	// SplitTopicTx 事务拆分：将指定 intel_id 从原话题移到新话题
	// （新话题初始 merge_lock=true，来源行 moved_by/moved_at 记录）。
	SplitTopicTx(ctx context.Context, in SplitTopicInput) (newTopicID string, err error)

	// MergeTopicsTx 事务合并：将源话题的全部来源移到目标话题
	// （目标话题 merge_lock=true；源话题 is_archived=true；来源行 moved_by/moved_at 记录）。
	MergeTopicsTx(ctx context.Context, in MergeTopicsInput) (mergedCount int, err error)

	// AckReviewConfirm 确认归并正确（needs_review=false）。
	AckReviewConfirm(ctx context.Context, topicID string) error
}

// TopicAdminView 话题列表行。
type TopicAdminView struct {
	TopicID       string
	Label         string
	Category      string
	Stage         string
	HeatScore     float64
	Velocity      float64
	PlatformCount int
	MergeLock     bool
	NeedsReview   bool
	IsArchived    bool
	FirstSeenAt   time.Time
	LastSeenAt    time.Time
	SourceCount   int
}

// TopicSourceView 来源条目视图。
type TopicSourceView struct {
	IntelID       string
	Platform      string
	ContributedAt time.Time
	Similarity    float64
	ClusterKey    string
	MovedBy       string
}

// TopicDetailView 话题详情 + 来源列表。
type TopicDetailView struct {
	Topic   TopicAdminView
	Sources []TopicSourceView
}

// SplitTopicInput 拆分入参。
type SplitTopicInput struct {
	TopicID  string   // 待拆分话题
	IntelIDs []string // 要拆出的来源条目（必须从原话题存在）
	Operator string   // 操作者（JWT user id；moved_by 落库）
	Reason   string   // 拆分原因（审计）
}

// MergeTopicsInput 合并入参。
type MergeTopicsInput struct {
	TargetTopicID  string   // 目标话题（保留）
	SourceTopicIDs []string // 待并入的话题（标记 is_archived）
	Operator       string
	Reason         string
}

// TopicAdminUsecase 话题管理编排。
type TopicAdminUsecase struct {
	repo TopicAdminRepo
}

// NewTopicAdminUsecase 构造。
func NewTopicAdminUsecase(repo TopicAdminRepo) *TopicAdminUsecase {
	return &TopicAdminUsecase{repo: repo}
}

// ListNeedsReview 复核队列。
func (uc *TopicAdminUsecase) ListNeedsReview(ctx context.Context, page, pageSize int) ([]TopicAdminView, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	return uc.repo.ListNeedsReviewTopics(ctx, page, pageSize)
}

// GetTopicDetail 话题详情。
func (uc *TopicAdminUsecase) GetTopicDetail(ctx context.Context, topicID string) (TopicDetailView, error) {
	if topicID == "" {
		return TopicDetailView{}, errors.New("tt: topic_id required")
	}
	return uc.repo.GetTopicDetail(ctx, topicID)
}

// SplitTopic 拆分话题。
func (uc *TopicAdminUsecase) SplitTopic(ctx context.Context, in SplitTopicInput) (string, error) {
	if in.TopicID == "" {
		return "", errors.New("tt: topic_id required")
	}
	if len(in.IntelIDs) == 0 {
		return "", errors.New("tt: intel_ids required (at least one source to split out)")
	}
	if in.Operator == "" {
		return "", errors.New("tt: operator required (from JWT)")
	}
	if in.Reason == "" {
		return "", errors.New("tt: reason required for audit")
	}
	return uc.repo.SplitTopicTx(ctx, in)
}

// MergeTopics 合并话题。
func (uc *TopicAdminUsecase) MergeTopics(ctx context.Context, in MergeTopicsInput) (int, error) {
	if in.TargetTopicID == "" {
		return 0, errors.New("tt: target_topic_id required")
	}
	if len(in.SourceTopicIDs) == 0 {
		return 0, errors.New("tt: source_topic_ids required")
	}
	if in.Operator == "" {
		return 0, errors.New("tt: operator required (from JWT)")
	}
	if in.Reason == "" {
		return 0, errors.New("tt: reason required for audit")
	}
	// 防护：目标话题不得在源列表中（自合并无意义）
	for _, srcID := range in.SourceTopicIDs {
		if srcID == in.TargetTopicID {
			return 0, fmt.Errorf("tt: target topic %s cannot be in source list", srcID)
		}
	}
	return uc.repo.MergeTopicsTx(ctx, in)
}

// AckReview 复核确认（confirm：needs_review=false；split：保持 true，引导拆分）。
func (uc *TopicAdminUsecase) AckReview(ctx context.Context, topicID, action string) error {
	if topicID == "" {
		return errors.New("tt: topic_id required")
	}
	switch action {
	case "confirm":
		return uc.repo.AckReviewConfirm(ctx, topicID)
	case "split":
		// 保持 needs_review=true，前端引导拆分对话框
		return nil
	default:
		return fmt.Errorf("tt: unknown action %q (must be confirm|split)", action)
	}
}
