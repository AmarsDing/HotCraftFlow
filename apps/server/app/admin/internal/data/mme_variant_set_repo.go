package data

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmevariantset"
)

// MmeVariantSetRepo 变体簇仓储。
type MmeVariantSetRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeVariantSetRepo 构造。
func NewMmeVariantSetRepo(data *Data, logger log.Logger) bizmme.VariantSetRepo {
	return &MmeVariantSetRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/variant-set/repo"))}
}

func diffPairsToJSON(pairs []bizmme.PairSimilarity) []map[string]any {
	if pairs == nil {
		return nil
	}
	raw, err := json.Marshal(pairs)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func diffPairsFromJSON(raw []map[string]any) []bizmme.PairSimilarity {
	if raw == nil {
		return nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out []bizmme.PairSimilarity
	if err = json.Unmarshal(buf, &out); err != nil {
		return nil
	}
	return out
}

func toBizVariantSet(e *ent.MmeVariantSet) *bizmme.VariantSet {
	if e == nil {
		return nil
	}
	return &bizmme.VariantSet{
		ID:                e.ID,
		TaskID:            e.TaskID,
		CandidateID:       derefStr(e.CandidateID),
		AssetID:           e.AssetID,
		TranscriptRef:     e.TranscriptRef,
		State:             e.State,
		VariantCount:      int(e.VariantCount),
		DiffMinSimilarity: e.DiffMinSimilarity,
		DiffWarn:          e.DiffWarn,
		DiffPairs:         diffPairsFromJSON(e.DiffPairs),
		TemplateID:        derefStr(e.TemplateID),
		CreatedAt:         derefTime(e.CreateTime),
	}
}

func (r *MmeVariantSetRepo) Create(ctx context.Context, s *bizmme.VariantSet) error {
	now := time.Now()
	b := r.data.db.Client().MmeVariantSet.Create().
		SetTaskID(s.TaskID).
		SetNillableCandidateID(strPtrOrNil(s.CandidateID)).
		SetAssetID(s.AssetID).
		SetTranscriptRef(s.TranscriptRef).
		SetState(s.State).
		SetVariantCount(int32(s.VariantCount)).
		SetDiffMinSimilarity(s.DiffMinSimilarity).
		SetDiffWarn(s.DiffWarn).
		SetNillableTemplateID(strPtrOrNil(s.TemplateID)).
		SetCreateTime(now).
		SetUpdateTime(now)
	if p := diffPairsToJSON(s.DiffPairs); p != nil {
		b = b.SetDiffPairs(p)
	}
	if s.ID != "" {
		b = b.SetID(s.ID)
	}
	e, err := b.Save(ctx)
	if err != nil {
		r.log.Errorf("create variant set failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("create variant set failed")
	}
	s.ID = e.ID
	s.CreatedAt = derefTime(e.CreateTime)
	return nil
}

func (r *MmeVariantSetRepo) Update(ctx context.Context, s *bizmme.VariantSet) error {
	b := r.data.db.Client().MmeVariantSet.UpdateOneID(s.ID).
		SetNillableCandidateID(strPtrOrNil(s.CandidateID)).
		SetAssetID(s.AssetID).
		SetTranscriptRef(s.TranscriptRef).
		SetState(s.State).
		SetVariantCount(int32(s.VariantCount)).
		SetDiffMinSimilarity(s.DiffMinSimilarity).
		SetDiffWarn(s.DiffWarn).
		SetNillableTemplateID(strPtrOrNil(s.TemplateID)).
		SetUpdateTime(time.Now())
	if p := diffPairsToJSON(s.DiffPairs); p != nil {
		b = b.SetDiffPairs(p)
	}
	if _, err := b.Save(ctx); err != nil {
		r.log.Errorf("update variant set %s failed: %s", s.ID, err.Error())
		return adminV1.ErrorInternalServerError("update variant set failed")
	}
	return nil
}

func (r *MmeVariantSetRepo) Get(ctx context.Context, id string) (*bizmme.VariantSet, error) {
	e, err := r.data.db.Client().MmeVariantSet.Query().
		Where(mmevariantset.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get variant set %s failed: %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get variant set failed")
	}
	return toBizVariantSet(e), nil
}

func (r *MmeVariantSetRepo) GetByTask(ctx context.Context, taskID string) (*bizmme.VariantSet, error) {
	e, err := r.data.db.Client().MmeVariantSet.Query().
		Where(mmevariantset.TaskIDEQ(taskID)).
		Order(ent.Desc(mmevariantset.FieldCreateTime)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get variant set by task %s failed: %s", taskID, err.Error())
		return nil, adminV1.ErrorInternalServerError("get variant set failed")
	}
	return toBizVariantSet(e), nil
}
