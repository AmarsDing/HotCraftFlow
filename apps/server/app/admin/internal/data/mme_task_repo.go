package data

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmemontagetask"
)

// MmeTaskRepo 加工任务仓储（④ MME）。
type MmeTaskRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeTaskRepo 构造。
func NewMmeTaskRepo(data *Data, logger log.Logger) bizmme.TaskRepo {
	return &MmeTaskRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/task/repo"))}
}

func mmeHighlightsToJSON(hs []bizmme.Highlight) []map[string]any {
	if hs == nil {
		return nil
	}
	raw, err := json.Marshal(hs)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func mmeHighlightsFromJSON(raw []map[string]any) []bizmme.Highlight {
	if raw == nil {
		return nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out []bizmme.Highlight
	if err = json.Unmarshal(buf, &out); err != nil {
		return nil
	}
	return out
}

func toBizMmeTask(e *ent.MmeMontageTask) *bizmme.Task {
	if e == nil {
		return nil
	}
	return &bizmme.Task{
		ID:            e.ID,
		Title:         e.Title,
		SourceType:    e.SourceType,
		SourceRef:     e.SourceRef,
		CandidateID:   derefStr(e.CandidateID),
		IntelID:       derefStr(e.IntelID),
		AssetID:       derefStr(e.AssetID),
		UserIntent:    derefStr(e.UserIntent),
		State:         e.State,
		PromptVersion: e.PromptVersion,
		ASREngine:     e.AsrEngine,
		ErrorMsg:      derefStr(e.ErrorMsg),
		Highlights:    mmeHighlightsFromJSON(e.Highlights),
		Progress:      e.Progress,
		CreatedAt:     e.CreateTime.UTC(),
		UpdatedAt:     e.UpdateTime.UTC(),
	}
}

func (r *MmeTaskRepo) Create(ctx context.Context, t *bizmme.Task) error {
	now := time.Now()
	b := r.data.db.Client().MmeMontageTask.Create().
		SetTitle(t.Title).
		SetSourceType(t.SourceType).
		SetSourceRef(t.SourceRef).
		SetCandidateID(t.CandidateID).
		SetIntelID(t.IntelID).
		SetAssetID(t.AssetID).
		SetUserIntent(t.UserIntent).
		SetState(t.State).
		SetPromptVersion(t.PromptVersion).
		SetAsrEngine(t.ASREngine).
		SetErrorMsg(t.ErrorMsg).
		SetProgress(t.Progress).
		SetCreateTime(now).
		SetUpdateTime(now)
	if h := mmeHighlightsToJSON(t.Highlights); h != nil {
		b = b.SetHighlights(h)
	}
	if t.ID != "" {
		b = b.SetID(t.ID)
	}
	e, err := b.Save(ctx)
	if err != nil {
		r.log.Errorf("create mme task failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("create mme task failed")
	}
	t.ID = e.ID
	t.CreatedAt = e.CreateTime.UTC()
	return nil
}

func (r *MmeTaskRepo) Update(ctx context.Context, t *bizmme.Task) error {
	b := r.data.db.Client().MmeMontageTask.UpdateOneID(t.ID).
		SetTitle(t.Title).
		SetSourceType(t.SourceType).
		SetSourceRef(t.SourceRef).
		SetCandidateID(t.CandidateID).
		SetIntelID(t.IntelID).
		SetAssetID(t.AssetID).
		SetUserIntent(t.UserIntent).
		SetState(t.State).
		SetPromptVersion(t.PromptVersion).
		SetAsrEngine(t.ASREngine).
		SetErrorMsg(t.ErrorMsg).
		SetProgress(t.Progress).
		SetUpdateTime(time.Now())
	if h := mmeHighlightsToJSON(t.Highlights); h != nil {
		b = b.SetHighlights(h)
	}
	if _, err := b.Save(ctx); err != nil {
		r.log.Errorf("update mme task %s failed: %s", t.ID, err.Error())
		return adminV1.ErrorInternalServerError("update mme task failed")
	}
	return nil
}

func (r *MmeTaskRepo) Get(ctx context.Context, id string) (*bizmme.Task, error) {
	e, err := r.data.db.Client().MmeMontageTask.Query().
		Where(mmemontagetask.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get mme task %s failed: %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get mme task failed")
	}
	return toBizMmeTask(e), nil
}

func (r *MmeTaskRepo) List(ctx context.Context, q bizmme.TaskQuery) ([]*bizmme.Task, int, error) {
	query := r.data.db.Client().MmeMontageTask.Query()
	if q.State != "" {
		query = query.Where(mmemontagetask.StateEQ(q.State))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count mme tasks failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query mme tasks failed")
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
	rows, err := query.
		Order(ent.Desc(mmemontagetask.FieldCreateTime)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list mme tasks failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query mme tasks failed")
	}
	out := make([]*bizmme.Task, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizMmeTask(e))
	}
	return out, total, nil
}

func (r *MmeTaskRepo) Delete(ctx context.Context, id string) error {
	if err := r.data.db.Client().MmeMontageTask.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		r.log.Errorf("delete mme task %s failed: %s", id, err.Error())
		return adminV1.ErrorInternalServerError("delete mme task failed")
	}
	return nil
}
