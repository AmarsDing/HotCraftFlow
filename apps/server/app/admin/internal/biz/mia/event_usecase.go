package mia

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algomia "framework/app/admin/internal/algo/mia"
	"framework/pkg/mediacontract"
)

// EventUsecase 事件卡（FR-MIA-05）：自动聚类查询 + 人工拆分/合并干预。
type EventUsecase struct {
	repo EventCardRepo
	log  *log.Helper
}

// NewEventUsecase 构造事件卡用例。
func NewEventUsecase(repo EventCardRepo, logger log.Logger) *EventUsecase {
	return &EventUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "biz/mia/event")),
	}
}

// List 分页查询事件卡。
func (u *EventUsecase) List(ctx context.Context, q EventQuery) ([]*EventCardRow, int, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 500 {
		q.PageSize = 50
	}
	return u.repo.List(ctx, q)
}

// Get 按 event_id 查询单卡。
func (u *EventUsecase) Get(ctx context.Context, eventID string) (*EventCardRow, error) {
	return u.repo.Get(ctx, eventID)
}

// Split 人工拆分：把指定来源从父卡拆出为新卡（两卡均锁定，不再被自动聚类吞并）。
func (u *EventUsecase) Split(ctx context.Context, parentEventID string, splitIntelIDs []string) (*EventCardRow, *EventCardRow, error) {
	if len(splitIntelIDs) == 0 {
		return nil, nil, fmt.Errorf("split: source_intel_ids 不可为空")
	}
	parent, err := u.repo.Get(ctx, parentEventID)
	if err != nil {
		return nil, nil, err
	}
	if parent == nil {
		return nil, nil, fmt.Errorf("split: 事件卡 %s 不存在", parentEventID)
	}
	if len(parent.SourceIntelIDs)-len(splitIntelIDs) < 1 {
		return nil, nil, fmt.Errorf("split: 全部拆分会留下空父卡，请改用 merge")
	}
	// 仅拆分确实属于父卡的来源。
	keep := map[string]bool{}
	for _, id := range splitIntelIDs {
		keep[id] = true
	}
	var childSrcs []string
	rest := make([]string, 0, len(parent.SourceIntelIDs))
	for _, id := range parent.SourceIntelIDs {
		if keep[id] {
			childSrcs = append(childSrcs, id)
		} else {
			rest = append(rest, id)
		}
	}
	if len(childSrcs) == 0 {
		return nil, nil, fmt.Errorf("split: 指定来源均不属于该事件卡")
	}
	child := &EventCardRow{
		EventID:        mediacontract.NewEventCardID(),
		Title:          parent.Title,
		Summary:        parent.Summary,
		OccurredAt:     parent.OccurredAt,
		TopicID:        parent.TopicID,
		SourceIntelIDs: childSrcs,
		SourceCount:    len(childSrcs),
		Entities:       parent.Entities,
		Category:       parent.Category,
		Lang:           parent.Lang,
		MergedBy:       "manual",
		SplitLocked:    true,
	}
	// 父卡缩减来源并降 source_count。
	lockedParent := &EventCardRow{
		ID:             parent.ID,
		EventID:        parent.EventID,
		SourceIntelIDs: rest,
		SourceCount:    len(rest),
		SplitLocked:    true,
		MergedBy:       "manual",
	}
	if err := u.repo.ReplaceSourcesForSplit(ctx, lockedParent, child); err != nil {
		return nil, nil, err
	}
	u.log.WithContext(ctx).Infof("event split: parent=%s remain=%d, child=%s sources=%d",
		parent.EventID, len(rest), child.EventID, len(childSrcs))
	got, _ := u.repo.Get(ctx, parent.EventID)
	return got, child, nil
}

// Merge 人工合并：把后续卡合并到第一张（第一张为目标卡，锁定）。
func (u *EventUsecase) Merge(ctx context.Context, eventIDs []string) (*EventCardRow, error) {
	if len(eventIDs) < 2 {
		return nil, fmt.Errorf("merge: 至少需要两张事件卡")
	}
	targetID := eventIDs[0]
	const maxMerge = 64
	if len(eventIDs) > maxMerge {
		eventIDs = eventIDs[:maxMerge]
	}
	target, err := u.repo.Get(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("merge: 目标卡 %s 不存在", targetID)
	}
	// 合并保留目标卡的现象口径：occurred_at 不变（事件发生时间不因合并而改变）。
	absorbed := eventIDs[1:]
	if err := u.repo.MergeInto(ctx, targetID, absorbed); err != nil {
		return nil, err
	}
	got, _ := u.repo.Get(ctx, targetID)
	u.log.WithContext(ctx).Infof("event merged: target=%s absorbed=%v", targetID, absorbed)
	return got, nil
}

// ClusterMatch 自动聚类的候选比对结果。
type ClusterMatch struct {
	Card       *EventCardRow
	Similarity float64
}

// FindClusterMatch 在近窗口事件卡中找最相似（满足并入阈值）的一张。
// 相似度 = 0.5·实体重合度 + 0.3·标题语义近似 + 0.2·时间接近度（详细设计 §2.3）。
func (u *EventUsecase) FindClusterMatch(ctx context.Context, newTitle string, newEntities []mediacontract.Entity, occurredAt time.Time, windowHours int, similarityThreshold float64, recent []*EventCardRow, titleSim func(a, b string) float64) *ClusterMatch {
	if len(recent) == 0 {
		return nil
	}
	best := &ClusterMatch{}
	hoursWindow := float64(windowHours)
	if hoursWindow <= 0 {
		hoursWindow = 24
	}
	for _, card := range recent {
		if card.SplitLocked {
			continue // 人工干预锁定卡不再被自动聚类吞并
		}
		entityJ := algomia.EntitySetJaccard(newEntities, card.Entities)
		ts := 0.0
		if titleSim != nil {
			ts = clamp01(titleSim(newTitle, card.Title))
		}
		hoursApart := occurredAt.Sub(card.OccurredAt).Hours()
		if hoursApart < 0 {
			hoursApart = -hoursApart
		}
		if hoursApart > hoursWindow {
			continue
		}
		sim := algomia.EventSimilarity(entityJ, ts, hoursApart)
		if sim > best.Similarity || best.Card == nil {
			best = &ClusterMatch{Card: card, Similarity: sim}
		}
	}
	threshold := similarityThreshold
	if threshold <= 0 || threshold > 1 {
		threshold = algomia.EventMergeThreshold
	}
	if best == nil || best.Similarity < threshold {
		return nil
	}
	return best
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// sortBySourcesDesc 事件卡按来源数倒序（供列表默认排序参考）。
func sortBySourcesDesc(rows []*EventCardRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].SourceCount > rows[j].SourceCount })
}
