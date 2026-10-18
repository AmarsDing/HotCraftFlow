package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmerenderjob"
	"framework/app/admin/internal/data/ent/mmevariant"
)

// MmeRenderJobRepo 渲染任务仓储。
type MmeRenderJobRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeRenderJobRepo 构造。
func NewMmeRenderJobRepo(data *Data, logger log.Logger) bizmme.RenderJobRepo {
	return &MmeRenderJobRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/render-job/repo"))}
}

func toBizRenderJob(e *ent.MmeRenderJob) *bizmme.RenderJob {
	if e == nil {
		return nil
	}
	return &bizmme.RenderJob{
		ID:               e.ID,
		VariantID:        e.VariantID,
		State:            e.State,
		Progress:         e.Progress,
		StartedAt:        derefTime(e.StartedAt),
		FinishedAt:       derefTime(e.FinishedAt),
		WorkerID:         e.WorkerID,
		ErrorCode:        derefStr(e.ErrorCode),
		ErrorMsg:         derefStr(e.ErrorMsg),
		IntermediatePath: derefStr(e.IntermediatePath),
		Attempt:          int(e.Attempt),
		CreatedAt:        derefTime(e.CreateTime),
	}
}

func timePtrOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func (r *MmeRenderJobRepo) Create(ctx context.Context, j *bizmme.RenderJob) error {
	now := time.Now()
	b := r.data.db.Client().MmeRenderJob.Create().
		SetVariantID(j.VariantID).
		SetState(j.State).
		SetProgress(j.Progress).
		SetNillableStartedAt(timePtrOrNil(j.StartedAt)).
		SetNillableFinishedAt(timePtrOrNil(j.FinishedAt)).
		SetWorkerID(j.WorkerID).
		SetNillableErrorCode(strPtrOrNil(j.ErrorCode)).
		SetNillableErrorMsg(strPtrOrNil(j.ErrorMsg)).
		SetNillableIntermediatePath(strPtrOrNil(j.IntermediatePath)).
		SetAttempt(int32(j.Attempt)).
		SetCreateTime(now).
		SetUpdateTime(now)
	if j.ID != "" {
		b = b.SetID(j.ID)
	}
	e, err := b.Save(ctx)
	if err != nil {
		r.log.Errorf("create render job failed (variant=%s): %s", j.VariantID, err.Error())
		return adminV1.ErrorInternalServerError("create render job failed")
	}
	j.ID = e.ID
	j.CreatedAt = derefTime(e.CreateTime)
	return nil
}

func (r *MmeRenderJobRepo) Update(ctx context.Context, j *bizmme.RenderJob) error {
	_, err := r.data.db.Client().MmeRenderJob.UpdateOneID(j.ID).
		SetState(j.State).
		SetProgress(j.Progress).
		SetNillableStartedAt(timePtrOrNil(j.StartedAt)).
		SetNillableFinishedAt(timePtrOrNil(j.FinishedAt)).
		SetWorkerID(j.WorkerID).
		SetNillableErrorCode(strPtrOrNil(j.ErrorCode)).
		SetNillableErrorMsg(strPtrOrNil(j.ErrorMsg)).
		SetNillableIntermediatePath(strPtrOrNil(j.IntermediatePath)).
		SetAttempt(int32(j.Attempt)).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update render job %s failed: %s", j.ID, err.Error())
		return adminV1.ErrorInternalServerError("update render job failed")
	}
	return nil
}

func (r *MmeRenderJobRepo) Get(ctx context.Context, id string) (*bizmme.RenderJob, error) {
	e, err := r.data.db.Client().MmeRenderJob.Query().
		Where(mmerenderjob.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get render job %s failed: %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get render job failed")
	}
	return toBizRenderJob(e), nil
}

func (r *MmeRenderJobRepo) GetByVariant(ctx context.Context, variantID string) (*bizmme.RenderJob, error) {
	e, err := r.data.db.Client().MmeRenderJob.Query().
		Where(mmerenderjob.VariantIDEQ(variantID)).
		Order(ent.Desc(mmerenderjob.FieldCreateTime)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get render job by variant %s failed: %s", variantID, err.Error())
		return nil, adminV1.ErrorInternalServerError("get render job failed")
	}
	return toBizRenderJob(e), nil
}

func (r *MmeRenderJobRepo) List(ctx context.Context, q bizmme.JobQuery) ([]*bizmme.RenderJob, int, error) {
	query := r.data.db.Client().MmeRenderJob.Query()
	if q.State != "" {
		query = query.Where(mmerenderjob.StateEQ(q.State))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count render jobs failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query render jobs failed")
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	rows, err := query.
		Order(ent.Desc(mmerenderjob.FieldCreateTime)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list render jobs failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query render jobs failed")
	}
	out := make([]*bizmme.RenderJob, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizRenderJob(e))
	}
	return out, total, nil
}

func (r *MmeRenderJobRepo) ListBySet(ctx context.Context, setID string) ([]*bizmme.RenderJob, error) {
	// job → variant → set 两级 join：先取簇内 variant id 集合
	vIDs, err := r.data.db.Client().MmeVariant.Query().
		Where(mmevariant.SetIDEQ(setID)).
		IDs(ctx)
	if err != nil {
		r.log.Errorf("list variant ids of set %s failed: %s", setID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query set variants failed")
	}
	rows, err := r.data.db.Client().MmeRenderJob.Query().
		Where(mmerenderjob.VariantIDIn(vIDs...)).
		Order(ent.Asc(mmerenderjob.FieldCreateTime)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list render jobs by set %s failed: %s", setID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query render jobs failed")
	}
	out := make([]*bizmme.RenderJob, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizRenderJob(e))
	}
	return out, nil
}
