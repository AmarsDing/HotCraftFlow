package data

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmevariant"
)

// MmeVariantRepo 单变体仓储。
type MmeVariantRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeVariantRepo 构造。
func NewMmeVariantRepo(data *Data, logger log.Logger) bizmme.VariantRepo {
	return &MmeVariantRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/variant/repo"))}
}

func copyMapToJSON(copy map[string]bizmme.PlatformCopy) map[string]any {
	if copy == nil {
		return nil
	}
	raw, err := json.Marshal(copy)
	if err != nil {
		return nil
	}
	out := map[string]any{}
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func copyMapFromJSON(raw map[string]any) map[string]bizmme.PlatformCopy {
	if raw == nil {
		return nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	out := map[string]bizmme.PlatformCopy{}
	if err = json.Unmarshal(buf, &out); err != nil {
		return nil
	}
	return out
}

func toBizMmeVariant(e *ent.MmeVariant) *bizmme.Variant {
	if e == nil {
		return nil
	}
	return &bizmme.Variant{
		ID:             e.ID,
		SetID:          e.SetID,
		ContentVersion: e.ContentVersion,
		Aspect:         e.Aspect,
		HookID:         e.HookID,
		CoverID:        e.CoverID,
		EDLRef:         e.EdlRef,
		DurationSec:    e.DurationSec,
		VideoPath:      e.VideoPath,
		CoverPath:      e.CoverPath,
		CaptionStyle:   e.CaptionStyle,
		SrtPath:        e.SrtPath,
		Copy:           copyMapFromJSON(e.CopyJSON),
		CopyDegraded:   e.CopyDegraded,
		ManifestPath:   e.ManifestPath,
		QCBoundaryOK:   e.QcBoundaryOk,
		QCSubtitleOK:   e.QcSubtitleOk,
		QCScore:        e.QcScore,
		RenderState:    e.RenderState,
		RenderError:    derefStr(e.RenderError),
		Selected:       e.Selected,
		ReviewState:    e.ReviewState,
		ReviewReason:   derefStr(e.ReviewReason),
	}
}

func (r *MmeVariantRepo) setVariantFields(b *ent.MmeVariantCreate, v *bizmme.Variant) *ent.MmeVariantCreate {
	b = b.SetSetID(v.SetID).
		SetContentVersion(v.ContentVersion).
		SetAspect(v.Aspect).
		SetHookID(v.HookID).
		SetCoverID(v.CoverID).
		SetEdlRef(v.EDLRef).
		SetDurationSec(v.DurationSec).
		SetVideoPath(v.VideoPath).
		SetCoverPath(v.CoverPath).
		SetCaptionStyle(v.CaptionStyle).
		SetSrtPath(v.SrtPath).
		SetCopyDegraded(v.CopyDegraded).
		SetManifestPath(v.ManifestPath).
		SetQcBoundaryOk(v.QCBoundaryOK).
		SetQcSubtitleOk(v.QCSubtitleOK).
		SetQcScore(v.QCScore).
		SetRenderState(v.RenderState).
		SetNillableRenderError(strPtrOrNil(v.RenderError)).
		SetSelected(v.Selected).
		SetReviewState(v.ReviewState).
		SetNillableReviewReason(strPtrOrNil(v.ReviewReason))
	if c := copyMapToJSON(v.Copy); c != nil {
		b = b.SetCopyJSON(c)
	}
	if v.ID != "" {
		b = b.SetID(v.ID)
	}
	return b
}

func (r *MmeVariantRepo) CreateBatch(ctx context.Context, vs []*bizmme.Variant) error {
	if len(vs) == 0 {
		return nil
	}
	now := time.Now()
	builders := make([]*ent.MmeVariantCreate, 0, len(vs))
	for _, v := range vs {
		b := r.setVariantFields(r.data.db.Client().MmeVariant.Create(), v).
			SetCreateTime(now).
			SetUpdateTime(now)
		builders = append(builders, b)
	}
	if err := r.data.db.Client().MmeVariant.CreateBulk(builders...).Exec(ctx); err != nil {
		r.log.Errorf("create variants batch failed (n=%d): %s", len(vs), err.Error())
		return adminV1.ErrorInternalServerError("create variants failed")
	}
	return nil
}

func (r *MmeVariantRepo) Update(ctx context.Context, v *bizmme.Variant) error {
	b := r.data.db.Client().MmeVariant.UpdateOneID(v.ID).
		SetContentVersion(v.ContentVersion).
		SetAspect(v.Aspect).
		SetHookID(v.HookID).
		SetCoverID(v.CoverID).
		SetEdlRef(v.EDLRef).
		SetDurationSec(v.DurationSec).
		SetVideoPath(v.VideoPath).
		SetCoverPath(v.CoverPath).
		SetCaptionStyle(v.CaptionStyle).
		SetSrtPath(v.SrtPath).
		SetCopyDegraded(v.CopyDegraded).
		SetManifestPath(v.ManifestPath).
		SetQcBoundaryOk(v.QCBoundaryOK).
		SetQcSubtitleOk(v.QCSubtitleOK).
		SetQcScore(v.QCScore).
		SetRenderState(v.RenderState).
		SetSelected(v.Selected).
		SetReviewState(v.ReviewState).
		SetUpdateTime(time.Now())
	if v.RenderError == "" {
		b = b.ClearRenderError()
	} else {
		b = b.SetRenderError(v.RenderError)
	}
	if v.ReviewReason == "" {
		b = b.ClearReviewReason()
	} else {
		b = b.SetReviewReason(v.ReviewReason)
	}
	if c := copyMapToJSON(v.Copy); c != nil {
		b = b.SetCopyJSON(c)
	}
	if _, err := b.Save(ctx); err != nil {
		r.log.Errorf("update variant %s failed: %s", v.ID, err.Error())
		return adminV1.ErrorInternalServerError("update variant failed")
	}
	return nil
}

func (r *MmeVariantRepo) Get(ctx context.Context, id string) (*bizmme.Variant, error) {
	e, err := r.data.db.Client().MmeVariant.Query().
		Where(mmevariant.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get variant %s failed: %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get variant failed")
	}
	return toBizMmeVariant(e), nil
}

func (r *MmeVariantRepo) ListBySet(ctx context.Context, setID string) ([]*bizmme.Variant, error) {
	rows, err := r.data.db.Client().MmeVariant.Query().
		Where(mmevariant.SetIDEQ(setID)).
		Order(ent.Asc(mmevariant.FieldContentVersion), ent.Asc(mmevariant.FieldAspect), ent.Asc(mmevariant.FieldHookID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list variants by set %s failed: %s", setID, err.Error())
		return nil, adminV1.ErrorInternalServerError("list variants failed")
	}
	out := make([]*bizmme.Variant, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizMmeVariant(e))
	}
	return out, nil
}

func (r *MmeVariantRepo) UpdateSelection(ctx context.Context, ids []string, selected bool) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.data.db.Client().MmeVariant.Update().
		Where(mmevariant.IDIn(ids...)).
		SetSelected(selected).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update selection (n=%d,%v) failed: %s", len(ids), selected, err.Error())
		return adminV1.ErrorInternalServerError("update selection failed")
	}
	return nil
}

func (r *MmeVariantRepo) DeleteUnrenderedBySet(ctx context.Context, setID string) error {
	_, err := r.data.db.Client().MmeVariant.Delete().
		Where(
			mmevariant.SetIDEQ(setID),
			mmevariant.RenderStateIn(bizmme.RenderPending, bizmme.RenderFailed),
		).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete unrendered variants of set %s failed: %s", setID, err.Error())
		return adminV1.ErrorInternalServerError("delete unrendered variants failed")
	}
	return nil
}

func (r *MmeVariantRepo) CountBySet(ctx context.Context, setID string) (total int, selected int, err error) {
	total, err = r.data.db.Client().MmeVariant.Query().
		Where(mmevariant.SetIDEQ(setID)).
		Count(ctx)
	if err != nil {
		return 0, 0, adminV1.ErrorInternalServerError("count variants failed")
	}
	selected, err = r.data.db.Client().MmeVariant.Query().
		Where(mmevariant.SetIDEQ(setID), mmevariant.SelectedEQ(true)).
		Count(ctx)
	if err != nil {
		return 0, 0, adminV1.ErrorInternalServerError("count selected variants failed")
	}
	return total, selected, nil
}
