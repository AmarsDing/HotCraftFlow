package service

import (
	"context"
	"strconv"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	ttV1 "framework/api/gen/go/media/v1/tt"
	biztt "framework/app/admin/internal/biz/tt"
)

// TtTopicAdminService 话题人工管理 REST（M3 Step5 merge_lock + 复核工作流）。
type TtTopicAdminService struct {
	uc  *biztt.TopicAdminUsecase
	log *log.Helper
}

// NewTtTopicAdminService 构造。
func NewTtTopicAdminService(uc *biztt.TopicAdminUsecase, logger log.Logger) *TtTopicAdminService {
	return &TtTopicAdminService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "tt/topic-admin/service")),
	}
}

// ListNeedsReview 复核队列（needs_review=true 且未归档）。
func (s *TtTopicAdminService) ListNeedsReview(ctx context.Context, req *ttV1.ListNeedsReviewRequest) (*ttV1.ListNeedsReviewResponse, error) {
	items, total, err := s.uc.ListNeedsReview(ctx, int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, err
	}
	out := &ttV1.ListNeedsReviewResponse{Total: total}
	for _, item := range items {
		out.Items = append(out.Items, topicAdminViewToProto(item))
	}
	return out, nil
}

// GetTopicDetail 话题详情（含来源列表，拆分对话框数据源）。
func (s *TtTopicAdminService) GetTopicDetail(ctx context.Context, req *ttV1.GetTopicDetailRequest) (*ttV1.GetTopicDetailResponse, error) {
	detail, err := s.uc.GetTopicDetail(ctx, req.GetTopicId())
	if err != nil {
		return nil, err
	}
	out := &ttV1.GetTopicDetailResponse{
		Topic: topicAdminViewToProto(detail.Topic),
	}
	for _, src := range detail.Sources {
		out.Sources = append(out.Sources, topicSourceViewToProto(src))
	}
	return out, nil
}

// SplitTopic 拆分话题（置 merge_lock + moved_by/moved_at 审计）。
func (s *TtTopicAdminService) SplitTopic(ctx context.Context, req *ttV1.SplitTopicRequest) (*ttV1.SplitTopicResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	newTopicID, err := s.uc.SplitTopic(ctx, biztt.SplitTopicInput{
		TopicID:  req.GetTopicId(),
		IntelIDs: req.GetIntelIds(),
		Operator: strconv.FormatUint(uint64(userID), 10),
		Reason:   req.GetReason(),
	})
	if err != nil {
		s.log.Warnf("split topic %s failed (user=%d): %v", req.GetTopicId(), userID, err)
		return nil, err
	}
	return &ttV1.SplitTopicResponse{NewTopicId: newTopicID}, nil
}

// MergeTopics 合并话题（置 merge_lock + moved_by/moved_at 审计）。
func (s *TtTopicAdminService) MergeTopics(ctx context.Context, req *ttV1.MergeTopicsRequest) (*ttV1.MergeTopicsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	mergedCount, err := s.uc.MergeTopics(ctx, biztt.MergeTopicsInput{
		TargetTopicID:  req.GetTargetTopicId(),
		SourceTopicIDs: req.GetSourceTopicIds(),
		Operator:       strconv.FormatUint(uint64(userID), 10),
		Reason:         req.GetReason(),
	})
	if err != nil {
		s.log.Warnf("merge topics failed (user=%d): %v", userID, err)
		return nil, err
	}
	return &ttV1.MergeTopicsResponse{MergedCount: int32(mergedCount)}, nil
}

// AckReview 复核确认（confirm：needs_review=false；split：引导拆分对话框）。
func (s *TtTopicAdminService) AckReview(ctx context.Context, req *ttV1.AckReviewRequest) (*ttV1.AckReviewResponse, error) {
	if err := s.uc.AckReview(ctx, req.GetTopicId(), req.GetAction()); err != nil {
		return nil, err
	}
	return &ttV1.AckReviewResponse{Ok: true}, nil
}

// topicAdminViewToProto biz → proto 视图映射。
func topicAdminViewToProto(v biztt.TopicAdminView) *ttV1.TopicItem {
	return &ttV1.TopicItem{
		TopicId:       v.TopicID,
		Label:         v.Label,
		Category:      v.Category,
		Stage:         v.Stage,
		HeatScore:     v.HeatScore,
		Velocity:      v.Velocity,
		PlatformCount: int32(v.PlatformCount),
		MergeLock:     v.MergeLock,
		NeedsReview:   v.NeedsReview,
		IsArchived:    v.IsArchived,
		FirstSeenAt:   timestamppb.New(v.FirstSeenAt),
		LastSeenAt:    timestamppb.New(v.LastSeenAt),
		SourceCount:   int32(v.SourceCount),
	}
}

// topicSourceViewToProto biz → proto 视图映射。
func topicSourceViewToProto(v biztt.TopicSourceView) *ttV1.TopicSourceItem {
	return &ttV1.TopicSourceItem{
		IntelId:       v.IntelID,
		Platform:      v.Platform,
		ContributedAt: timestamppb.New(v.ContributedAt),
		Similarity:    v.Similarity,
		ClusterKey:    v.ClusterKey,
		MovedBy:       v.MovedBy,
	}
}
