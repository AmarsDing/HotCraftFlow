package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmeasset"
)

// MmeAssetRepo 素材仓储（FR-MIX-07 指纹去重）。
type MmeAssetRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeAssetRepo 构造。
func NewMmeAssetRepo(data *Data, logger log.Logger) bizmme.AssetRepo {
	return &MmeAssetRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/asset/repo"))}
}

func toBizMmeAsset(e *ent.MmeAsset) *bizmme.Asset {
	if e == nil {
		return nil
	}
	return &bizmme.Asset{
		ID:           e.ID,
		Fingerprint:  e.Fingerprint,
		SourceURL:    derefStr(e.SourceURL),
		LocalPath:    e.LocalPath,
		DurationSec:  e.DurationSec,
		Width:        int(e.Width),
		Height:       int(e.Height),
		SizeBytes:    e.SizeBytes,
		RefCount:     int(e.RefCount),
		MetaTitle:    derefStr(e.MetaTitle),
		SubtitlePath: derefStr(e.SubtitlePath),
		CreatedAt:    derefTime(e.CreateTime),
	}
}

func (r *MmeAssetRepo) Create(ctx context.Context, a *bizmme.Asset) error {
	now := time.Now()
	b := r.data.db.Client().MmeAsset.Create().
		SetNillableSourceURL(strPtrOrNil(a.SourceURL)).
		SetLocalPath(a.LocalPath).
		SetDurationSec(a.DurationSec).
		SetWidth(int32(a.Width)).
		SetHeight(int32(a.Height)).
		SetSizeBytes(a.SizeBytes).
		SetRefCount(int32(firstNonZero(a.RefCount, 1))).
		SetNillableMetaTitle(strPtrOrNil(a.MetaTitle)).
		SetNillableSubtitlePath(strPtrOrNil(a.SubtitlePath)).
		SetCreateTime(now).
		SetUpdateTime(now)
	if a.Fingerprint != "" {
		b = b.SetFingerprint(a.Fingerprint)
	}
	if a.ID != "" {
		b = b.SetID(a.ID)
	}
	e, err := b.Save(ctx)
	if err != nil {
		r.log.Errorf("create mme asset failed (fp=%s): %s", a.Fingerprint, err.Error())
		return adminV1.ErrorInternalServerError("create mme asset failed")
	}
	a.ID = e.ID
	a.CreatedAt = derefTime(e.CreateTime)
	return nil
}

func (r *MmeAssetRepo) Update(ctx context.Context, a *bizmme.Asset) error {
	_, err := r.data.db.Client().MmeAsset.UpdateOneID(a.ID).
		SetNillableSourceURL(strPtrOrNil(a.SourceURL)).
		SetLocalPath(a.LocalPath).
		SetDurationSec(a.DurationSec).
		SetWidth(int32(a.Width)).
		SetHeight(int32(a.Height)).
		SetSizeBytes(a.SizeBytes).
		SetNillableMetaTitle(strPtrOrNil(a.MetaTitle)).
		SetNillableSubtitlePath(strPtrOrNil(a.SubtitlePath)).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update mme asset %s failed: %s", a.ID, err.Error())
		return adminV1.ErrorInternalServerError("update mme asset failed")
	}
	return nil
}

func (r *MmeAssetRepo) Get(ctx context.Context, id string) (*bizmme.Asset, error) {
	e, err := r.data.db.Client().MmeAsset.Query().
		Where(mmeasset.IDEQ(id)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get mme asset %s failed: %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get mme asset failed")
	}
	return toBizMmeAsset(e), nil
}

func (r *MmeAssetRepo) GetByFingerprint(ctx context.Context, fingerprint string) (*bizmme.Asset, error) {
	e, err := r.data.db.Client().MmeAsset.Query().
		Where(mmeasset.FingerprintEQ(fingerprint)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get mme asset by fingerprint failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("get mme asset failed")
	}
	return toBizMmeAsset(e), nil
}

func (r *MmeAssetRepo) IncrRefCount(ctx context.Context, id string) (int, error) {
	// ref_count++ 原子加一；Degrade 到先读再写会丢并发增量，用 ent 的 Add。
	e, err := r.data.db.Client().MmeAsset.UpdateOneID(id).
		AddRefCount(1).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("incr ref_count %s failed: %s", id, err.Error())
		return 0, adminV1.ErrorInternalServerError("incr ref count failed")
	}
	return int(e.RefCount), nil
}

func firstNonZero(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}
