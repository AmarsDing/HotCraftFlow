package ttdata

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	biztt "framework/app/admin/internal/biz/tt"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/topic"
)

// defaultLifecycleScanLimit ListActiveTopics 的兜底批量（limit≤0 时用）。
// 15min 评估周期内单批可扫完的量级；调用方分片/续扫按 topic_id 升序游标。
const defaultLifecycleScanLimit = 500

// EntLifecycleRepo biz/tt.LifecycleRepo 的 Ent 实现。
//
// 纪律：
//   - 扫描集 = stage≠dormant 且未归档且未 merge_lock（A-8 评估对象；
//     锁定话题的人工阶段由人工路径独占，评估器不动）。
//   - UpdateStage 是乐观条件写：WHERE stage != 目标阶段——并发评估器/人工
//     抢先落库后本进程拿到 n=0 → changed=false，与 biz 层 SaveIfNotAdvanced
//     同款语义，重复评估不产生写风暴。
//   - stage_evidence 暂无列（schema 后续扩展）；证据先结构化日志留痕，
//     落库字段由 biz 层契约保持（加列后本实现补 Set 即可，无接口变更）。
type EntLifecycleRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntLifecycleRepo 构造。
func NewEntLifecycleRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntLifecycleRepo {
	return &EntLifecycleRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/tt-lifecycle-repo")),
	}
}

// ListActiveTopics 活跃话题行（topic_id 升序，稳定顺序利于断点续扫）。
func (r *EntLifecycleRepo) ListActiveTopics(ctx context.Context, limit int) ([]biztt.LifecycleTopicRow, error) {
	if limit <= 0 {
		limit = defaultLifecycleScanLimit
	}
	rows, err := r.cli.Client().Topic.Query().
		Where(
			topic.IsArchivedEQ(false),
			topic.MergeLockEQ(false),
			topic.StageNEQ(topic.StageDormant),
		).
		Select(topic.FieldTopicID, topic.FieldStage, topic.FieldStageSince,
			topic.FieldStageDurationPoints, topic.FieldFirstSeenAt).
		Order(ent.Asc(topic.FieldTopicID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]biztt.LifecycleTopicRow, 0, len(rows))
	for _, t := range rows {
		row := biztt.LifecycleTopicRow{
			TopicID:             t.TopicID,
			Stage:               biztt.Stage(t.Stage),
			StageSince:          t.StageSince.UTC(), // Optional 未配 Nillable：NULL 读为零值
			StageDurationPoints: t.StageDurationPoints,
			FirstSeenAt:         t.FirstSeenAt.UTC(), // A-8 AgeDays 锚点（H4）
		}
		out = append(out, row)
	}
	return out, nil
}

// GetFirstSeenAt 读主题首见时间（A-8 AgeDays 锚点）。未找到返回零值 + nil，
// 组装侧退化为最新样本时间（AgeDays≈0，保守不迁老话题分支）。
func (r *EntLifecycleRepo) GetFirstSeenAt(ctx context.Context, topicID string) (time.Time, error) {
	t, err := r.cli.Client().Topic.Query().
		Where(topic.TopicID(topicID)).
		Select(topic.FieldFirstSeenAt).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return t.FirstSeenAt.UTC(), nil
}

// UpdateStage 条件更新阶段三元组 + stage_evidence JSON 落库（A-8 证据可追溯）。
// changed=false 表示目标阶段已是 in.Stage（含并发抢先），调用方按无变更处理。
func (r *EntLifecycleRepo) UpdateStage(ctx context.Context, topicID string, in biztt.StageUpdate) (bool, error) {
	// 解析 evidence JSON（biz 层 buildStageEvidence 产物）为 map 落库
	evidenceMap := map[string]any{}
	if in.Evidence != "" {
		// 简易解析（完整 JSON 解析由调用方保证；此处仅为落库形态转换）
		// 格式：{"rule":N,"from":"X","to":"Y","heat":H,"v":V,...}
		evidenceMap["raw"] = in.Evidence
	}

	n, err := r.cli.Client().Topic.Update().
		Where(
			topic.TopicID(topicID),
			topic.StageNEQ(topic.Stage(in.Stage)),
		).
		SetStage(topic.Stage(in.Stage)).
		SetStageSince(in.StageSince.UTC()).
		SetStageDurationPoints(in.StageDurationPoints).
		SetStageEvidence(evidenceMap).
		Save(ctx)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	r.log.Infow("stage_transition",
		"topic_id", topicID,
		"to", string(in.Stage),
		"since", in.StageSince.UTC().Format(time.RFC3339),
		"evidence", in.Evidence,
	)
	return true, nil
}

var _ biztt.LifecycleRepo = (*EntLifecycleRepo)(nil)
