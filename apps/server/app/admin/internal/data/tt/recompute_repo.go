package ttdata

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/topic"
	biztt "framework/app/admin/internal/biz/tt"
)

// EntRecomputeRepo biz/tt.RecomputeRepo 的 Ent 实现。
//
// recompute_lock 语义（S-23/A-14）：
//   - 非空 = 该话题正在重算；生命周期扫描集排除（ListActiveTopics 暂不滤——
//     锁陈旧阈值 2×interval 内评估一轮无害，M4 告警评估须读该键抑制）。
//   - 置锁为条件写：无锁或锁龄 < staleBefore 才成功（崩溃残留锁由陈旧阈值自愈）。
type EntRecomputeRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntRecomputeRepo 构造。
func NewEntRecomputeRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntRecomputeRepo {
	return &EntRecomputeRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/tt-recompute-repo")),
	}
}

// SetRecomputeLock 置/清 recompute_lock。
// at=nil → 无条件清除；at≠nil → 条件置锁（无锁或锁早于 staleBefore 才成功）。
// 返回 false = 他人持有效锁（调用方跳过本轮）。
func (r *EntRecomputeRepo) SetRecomputeLock(ctx context.Context, topicID string, at *time.Time, staleBefore time.Time) (bool, error) {
	if at == nil {
		_, err := r.cli.Client().Topic.Update().
			Where(topic.TopicID(topicID)).
			ClearRecomputeLock().
			Save(ctx)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	// 条件置锁：WHERE topic_id AND (recompute_lock IS NULL OR recompute_lock < staleBefore)
	n, err := r.cli.Client().Topic.Update().
		Where(
			topic.TopicID(topicID),
			topic.Or(
				topic.RecomputeLockIsNil(),
				topic.RecomputeLockLT(staleBefore),
			),
		).
		SetRecomputeLock(at.UTC()).
		Save(ctx)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil // 他人持有效锁
	}
	r.log.Infow("recompute_lock_acquired", "topic_id", topicID, "at", at.UTC().Format(time.RFC3339))
	return true, nil
}

var _ biztt.RecomputeRepo = (*EntRecomputeRepo)(nil)
