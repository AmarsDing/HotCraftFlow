package data

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mmeanalysiscache"
)

// MmeAnalysisCacheRepo 4 级分析缓存仓储（FR-MIX-02）。
type MmeAnalysisCacheRepo struct {
	data *Data
	log  *log.Helper
}

// NewMmeAnalysisCacheRepo 构造。
func NewMmeAnalysisCacheRepo(data *Data, logger log.Logger) bizmme.CacheRepo {
	return &MmeAnalysisCacheRepo{data: data, log: log.NewHelper(log.With(logger, "module", "mme/cache/repo"))}
}

func toBizCacheEntry(e *ent.MmeAnalysisCache) *bizmme.CacheEntry {
	if e == nil {
		return nil
	}
	return &bizmme.CacheEntry{
		ID:            int64(e.ID),
		CacheKey:      e.CacheKey,
		Kind:          e.Kind,
		AssetID:       e.AssetID,
		PayloadPath:   e.PayloadPath,
		PayloadMeta:   e.PayloadMeta,
		Engine:        e.Engine,
		EngineVersion: e.EngineVersion,
		PromptVersion: e.PromptVersion,
		HitCount:      e.HitCount,
		SizeBytes:     e.SizeBytes,
		ExpiresAt:     e.ExpiresAt.UTC(),
	}
}

func (r *MmeAnalysisCacheRepo) Get(ctx context.Context, key string) (*bizmme.CacheEntry, error) {
	e, err := r.data.db.Client().MmeAnalysisCache.Query().
		Where(mmeanalysiscache.CacheKeyEQ(key)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get cache %s failed: %s", key, err.Error())
		return nil, adminV1.ErrorInternalServerError("get cache failed")
	}
	return toBizCacheEntry(e), nil
}

func (r *MmeAnalysisCacheRepo) Upsert(ctx context.Context, e *bizmme.CacheEntry) error {
	err := r.data.db.Client().MmeAnalysisCache.Create().
		SetCacheKey(e.CacheKey).
		SetKind(e.Kind).
		SetAssetID(e.AssetID).
		SetPayloadPath(e.PayloadPath).
		SetPayloadMeta(e.PayloadMeta).
		SetEngine(e.Engine).
		SetEngineVersion(e.EngineVersion).
		SetPromptVersion(e.PromptVersion).
		SetHitCount(e.HitCount).
		SetSizeBytes(e.SizeBytes).
		SetExpiresAt(e.ExpiresAt).
		OnConflictColumns(mmeanalysiscache.FieldCacheKey).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		r.log.Errorf("upsert cache %s failed: %s", e.CacheKey, err.Error())
		return adminV1.ErrorInternalServerError("upsert cache failed")
	}
	return nil
}

func (r *MmeAnalysisCacheRepo) IncrHit(ctx context.Context, key string) error {
	_, err := r.data.db.Client().MmeAnalysisCache.Update().
		Where(mmeanalysiscache.CacheKeyEQ(key)).
		AddHitCount(1).
		Save(ctx)
	if err != nil {
		r.log.Warnf("incr cache hit %s: %s", key, err.Error())
	}
	return nil // 观测计数失败不阻断主流程
}

func (r *MmeAnalysisCacheRepo) TotalBytes(ctx context.Context) (int64, error) {
	// ent 无 Sum 聚合接口的场合用原生 SQL
	var sum []struct{ Total int64 `json:"total"` }
	err := r.data.db.Client().MmeAnalysisCache.Query().
		Aggregate(func(s *sql.Selector) string {
			return sql.As(sql.Sum(mmeanalysiscache.FieldSizeBytes), "total")
		}).
		Scan(ctx, &sum)
	if err != nil || len(sum) == 0 {
		return 0, err
	}
	return sum[0].Total, nil
}
