package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/collectrun"
	"framework/app/admin/internal/data/ent/deadletter"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 采集运行记录（审计 + S-14 健康度来源）与死信（S-13）
// =============================================================================

type CollectRunRepo struct {
	data *Data
	log  *log.Helper
}

func NewCollectRunRepo(data *Data, logger log.Logger) biz.CollectRunRepo {
	return &CollectRunRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/run/repo/admin-service")),
		data: data,
	}
}

func (r *CollectRunRepo) Create(ctx context.Context, sourceID, trigger string, startedAt time.Time) (string, error) {
	now := time.Now().UTC()
	id := contract.NewID(contract.PrefixCollectRun)
	_, err := r.data.db.Client().CollectRun.Create().
		SetID(id).
		SetSourceID(sourceID).
		SetTrigger(trigger).
		SetStartedAt(startedAt.UTC()).
		SetFetched(0).
		SetNormalized(0).
		SetMerged(0).
		SetBlocked(0).
		SetSkipped(0).
		SetStatus("running").
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create collect run failed (src=%s): %s", sourceID, err.Error())
		return "", adminV1.ErrorInternalServerError("create collect run failed")
	}
	return id, nil
}

func (r *CollectRunRepo) Finish(ctx context.Context, id string, fin *biz.CollectRunFinish) error {
	if fin == nil {
		return nil
	}
	builder := r.data.db.Client().CollectRun.UpdateOneID(id).
		SetFinishedAt(time.Now().UTC()).
		SetFetched(fin.Fetched).
		SetNormalized(fin.Normalized).
		SetMerged(fin.Merged).
		SetBlocked(fin.Blocked).
		SetSkipped(fin.Skipped).
		SetStatus(fin.Status).
		SetUpdateTime(time.Now().UTC())
	if fin.ErrorCode != "" {
		builder.SetErrorCode(fin.ErrorCode)
	}
	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf("finish collect run failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("finish collect run failed")
	}
	return nil
}

func (r *CollectRunRepo) List(ctx context.Context, sourceID string, page, pageSize int32) ([]*biz.CollectRunRow, int, error) {
	p, s := icPageClamp(page, pageSize)
	query := r.data.db.Client().CollectRun.Query()
	if sourceID != "" {
		query = query.Where(collectrun.SourceIDEQ(sourceID))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count collect runs failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("count collect runs failed")
	}
	entities, err := query.
		Order(ent.Desc(collectrun.FieldStartedAt)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list collect runs failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("list collect runs failed")
	}
	out := make([]*biz.CollectRunRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizCollectRun(e))
	}
	return out, total, nil
}

// CountOutcomes S-14：健康任务按窗口统计成功/失败轮次。
func (r *CollectRunRepo) CountOutcomes(ctx context.Context, sourceID string, since time.Time) (int32, int32, error) {
	since = since.UTC()
	success, err := r.data.db.Client().CollectRun.Query().
		Where(
			collectrun.SourceIDEQ(sourceID),
			collectrun.StartedAtGT(since),
			collectrun.StatusEQ("succeeded"),
		).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count success runs failed (src=%s): %s", sourceID, err.Error())
		return 0, 0, adminV1.ErrorInternalServerError("count collect runs failed")
	}
	fail, err := r.data.db.Client().CollectRun.Query().
		Where(
			collectrun.SourceIDEQ(sourceID),
			collectrun.StartedAtGT(since),
			collectrun.StatusEQ("failed"),
		).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count failed runs failed (src=%s): %s", sourceID, err.Error())
		return 0, 0, adminV1.ErrorInternalServerError("count collect runs failed")
	}
	return int32(success), int32(fail), nil
}

func toBizCollectRun(e *ent.CollectRun) *biz.CollectRunRow {
	if e == nil {
		return nil
	}
	return &biz.CollectRunRow{
		ID:         e.ID,
		SourceID:   e.SourceID,
		Trigger:    e.Trigger,
		StartedAt:  e.StartedAt.UTC(),
		FinishedAt: e.FinishedAt,
		Fetched:    e.Fetched,
		Normalized: e.Normalized,
		Merged:     e.Merged,
		Blocked:    e.Blocked,
		Skipped:    e.Skipped,
		Status:     e.Status,
		ErrorCode:  derefStr(e.ErrorCode),
	}
}

// ---------- 死信 ----------

type DeadLetterRepo struct {
	data *Data
	log  *log.Helper
}

func NewDeadLetterRepo(data *Data, logger log.Logger) biz.DeadLetterRepo {
	return &DeadLetterRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/deadletter/repo/admin-service")),
		data: data,
	}
}

func (r *DeadLetterRepo) Create(ctx context.Context, row *biz.DeadLetterRow) (string, error) {
	now := time.Now().UTC()
	// 契约前缀表未给 DL 分配前缀：内部实体用裸 ULID（评审 M-20 纪律）
	id := contract.ULID()
	err := r.data.db.Client().DeadLetter.Create().
		SetID(id).
		SetTaskType(row.TaskType).
		SetPayload(row.Payload).
		SetErrorCode(row.ErrorCode).
		SetAttempt(row.Attempt).
		SetFirstFailedAt(row.FirstFailedAt.UTC()).
		SetLastFailedAt(row.LastFailedAt.UTC()).
		SetStatus("pending").
		SetCreateTime(now).
		SetUpdateTime(now).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("create dead letter failed (type=%s, code=%s): %s", row.TaskType, row.ErrorCode, err.Error())
		return "", adminV1.ErrorInternalServerError("create dead letter failed")
	}
	return id, nil
}

func (r *DeadLetterRepo) GetByID(ctx context.Context, id string) (*biz.DeadLetterRow, error) {
	e, err := r.data.db.Client().DeadLetter.Query().
		Where(deadletter.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query dead letter failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("query dead letter failed")
	}
	return toBizDeadLetter(e), nil
}

func (r *DeadLetterRepo) List(ctx context.Context, status string, page, pageSize int32) ([]*biz.DeadLetterRow, int, error) {
	p, s := icPageClamp(page, pageSize)
	query := r.data.db.Client().DeadLetter.Query()
	if status != "" {
		query = query.Where(deadletter.StatusEQ(status))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count dead letters failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("count dead letters failed")
	}
	entities, err := query.
		Order(ent.Desc(deadletter.FieldLastFailedAt)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list dead letters failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("list dead letters failed")
	}
	out := make([]*biz.DeadLetterRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizDeadLetter(e))
	}
	return out, total, nil
}

// MarkReplayed 重放审计（S-13：operator 必填，由 service 层保证非空）。
func (r *DeadLetterRepo) MarkReplayed(ctx context.Context, id, operator string, at time.Time) error {
	if _, err := r.data.db.Client().DeadLetter.UpdateOneID(id).
		SetStatus("replayed").
		SetReplayedBy(operator).
		SetReplayedAt(at.UTC()).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return icV1.ErrorIcSrcNotFound("死信不存在")
		}
		r.log.Errorf("mark dead letter replayed failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("mark dead letter replayed failed")
	}
	return nil
}

func (r *DeadLetterRepo) MarkDiscarded(ctx context.Context, id string) error {
	if _, err := r.data.db.Client().DeadLetter.UpdateOneID(id).
		SetStatus("discarded").
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("mark dead letter discarded failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("mark dead letter discarded failed")
	}
	return nil
}

func toBizDeadLetter(e *ent.DeadLetter) *biz.DeadLetterRow {
	if e == nil {
		return nil
	}
	return &biz.DeadLetterRow{
		ID:            e.ID,
		TaskType:      e.TaskType,
		Payload:       e.Payload,
		ErrorCode:     e.ErrorCode,
		ErrorMsg:      derefStr(e.ErrorMsg),
		Attempt:       e.Attempt,
		FirstFailedAt: e.FirstFailedAt.UTC(),
		LastFailedAt:  e.LastFailedAt.UTC(),
		Status:        e.Status,
		ReplayedBy:    derefStr(e.ReplayedBy),
		ReplayedAt:    e.ReplayedAt,
	}
}
