package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/entityassetmapping"
	"framework/pkg/mediacontract"
)

// MiaEntityMappingRepo 映射表 A 仓储（FR-MIA-01）。
type MiaEntityMappingRepo struct {
	data *Data
	log  *log.Helper
}

// NewMiaEntityMappingRepo 构造。
func NewMiaEntityMappingRepo(data *Data, logger log.Logger) bizmia.EntityMappingRepo {
	return &MiaEntityMappingRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "mia/entity-mapping/repo/admin-service")),
	}
}

func (r *MiaEntityMappingRepo) Create(ctx context.Context, item *bizmia.MappingItem) (*bizmia.MappingItem, error) {
	now := time.Now()
	row, err := r.data.db.Client().EntityAssetMapping.Create().
		SetEntityName(item.EntityName).
		SetEntityAlias(item.EntityAlias).
		SetEntityType(entityassetmapping.EntityType(item.EntityType)).
		SetAssetClass(entityassetmapping.AssetClass(item.AssetClass)).
		SetSymbol(item.Symbol).
		SetDirectionBias(entityassetmapping.DirectionBias(item.DirectionBias)).
		SetConfidence(item.Confidence).
		SetSource(firstNonEmptyStr(item.Source, "manual")).
		SetNotes(item.Notes).
		SetEnabled(item.Enabled).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create entity mapping failed (entity=%s symbol=%s): %s", item.EntityName, item.Symbol, err.Error())
		return nil, adminV1.ErrorInternalServerError("create entity mapping failed")
	}
	return toBizEntityMapping(row), nil
}

func (r *MiaEntityMappingRepo) Update(ctx context.Context, id int64, item *bizmia.MappingItem) (*bizmia.MappingItem, error) {
	row, err := r.data.db.Client().EntityAssetMapping.UpdateOneID(int(id)).
		SetEntityName(item.EntityName).
		SetEntityAlias(item.EntityAlias).
		SetEntityType(entityassetmapping.EntityType(item.EntityType)).
		SetAssetClass(entityassetmapping.AssetClass(item.AssetClass)).
		SetSymbol(item.Symbol).
		SetDirectionBias(entityassetmapping.DirectionBias(item.DirectionBias)).
		SetConfidence(item.Confidence).
		SetSource(firstNonEmptyStr(item.Source, "manual")).
		SetNotes(item.Notes).
		SetEnabled(item.Enabled).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update entity mapping failed (id=%d): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("update entity mapping failed")
	}
	return toBizEntityMapping(row), nil
}

func (r *MiaEntityMappingRepo) Delete(ctx context.Context, id int64) error {
	if err := r.data.db.Client().EntityAssetMapping.DeleteOneID(int(id)).Exec(ctx); err != nil {
		r.log.Errorf("delete entity mapping failed (id=%d): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("delete entity mapping failed")
	}
	return nil
}

func (r *MiaEntityMappingRepo) Get(ctx context.Context, id int64) (*bizmia.MappingItem, error) {
	row, err := r.data.db.Client().EntityAssetMapping.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get entity mapping failed (id=%d): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get entity mapping failed")
	}
	return toBizEntityMapping(row), nil
}

func (r *MiaEntityMappingRepo) List(ctx context.Context, q bizmia.MappingQuery) ([]*bizmia.MappingItem, int, error) {
	query := r.data.db.Client().EntityAssetMapping.Query()
	query = applyEntityMappingFilters(query, q)

	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count entity mappings failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query entity mappings failed")
	}

	rows, err := query.
		Order(ent.Desc(entityassetmapping.FieldUpdateTime), ent.Desc(entityassetmapping.FieldID)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list entity mappings failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query entity mappings failed")
	}
	out := make([]*bizmia.MappingItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizEntityMapping(row))
	}
	return out, total, nil
}

func (r *MiaEntityMappingRepo) ListEnabled(ctx context.Context) ([]*bizmia.MappingItem, error) {
	rows, err := r.data.db.Client().EntityAssetMapping.Query().
		Where(entityassetmapping.EnabledEQ(true)).
		Order(ent.Asc(entityassetmapping.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list enabled entity mappings failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query entity mappings failed")
	}
	out := make([]*bizmia.MappingItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizEntityMapping(row))
	}
	return out, nil
}

// UpsertByKey CSV 导入幂等：按唯一键 (entity_name, asset_class, symbol) 查改或新建。
func (r *MiaEntityMappingRepo) UpsertByKey(ctx context.Context, item *bizmia.MappingItem) (bool, error) {
	existing, err := r.data.db.Client().EntityAssetMapping.Query().
		Where(
			entityassetmapping.EntityNameEQ(item.EntityName),
			entityassetmapping.AssetClassEQ(entityassetmapping.AssetClass(item.AssetClass)),
			entityassetmapping.SymbolEQ(item.Symbol),
		).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("upsert entity mapping lookup failed (entity=%s symbol=%s): %s", item.EntityName, item.Symbol, err.Error())
		return false, adminV1.ErrorInternalServerError("upsert entity mapping failed")
	}
	if existing == nil {
		if _, err := r.Create(ctx, item); err != nil {
			return false, err
		}
		return true, nil
	}
	now := time.Now()
	_, err = r.data.db.Client().EntityAssetMapping.UpdateOneID(existing.ID).
		SetEntityAlias(item.EntityAlias).
		SetEntityType(entityassetmapping.EntityType(item.EntityType)).
		SetDirectionBias(entityassetmapping.DirectionBias(item.DirectionBias)).
		SetConfidence(item.Confidence).
		SetSource(firstNonEmptyStr(item.Source, "import")).
		SetNotes(item.Notes).
		SetEnabled(item.Enabled).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("upsert entity mapping update failed (id=%d): %s", existing.ID, err.Error())
		return false, adminV1.ErrorInternalServerError("upsert entity mapping failed")
	}
	return false, nil
}

func (r *MiaEntityMappingRepo) Count(ctx context.Context) (int, error) {
	n, err := r.data.db.Client().EntityAssetMapping.Query().Count(ctx)
	if err != nil {
		r.log.Errorf("count entity mappings failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("count entity mappings failed")
	}
	return n, nil
}

// applyEntityMappingFilters 列表公共过滤条件。
func applyEntityMappingFilters(query *ent.EntityAssetMappingQuery, q bizmia.MappingQuery) *ent.EntityAssetMappingQuery {
	if q.Keyword != "" {
		query = query.Where(entityassetmapping.Or(
			entityassetmapping.EntityNameContains(q.Keyword),
			entityassetmapping.SymbolContains(q.Keyword),
		))
	}
	if et := mediacontract.EntityType(q.EntityType); et.Valid() {
		query = query.Where(entityassetmapping.EntityTypeEQ(entityassetmapping.EntityType(et)))
	}
	if ac := mediacontract.AssetClass(q.AssetClass); ac.Valid() {
		query = query.Where(entityassetmapping.AssetClassEQ(entityassetmapping.AssetClass(ac)))
	}
	if q.EnabledOnly {
		query = query.Where(entityassetmapping.EnabledEQ(true))
	}
	return query
}

func toBizEntityMapping(e *ent.EntityAssetMapping) *bizmia.MappingItem {
	item := &bizmia.MappingItem{
		ID:            int64(e.ID),
		EntityName:    e.EntityName,
		EntityAlias:   e.EntityAlias,
		EntityType:    mediacontract.EntityType(e.EntityType),
		AssetClass:    mediacontract.AssetClass(e.AssetClass),
		Symbol:        e.Symbol,
		DirectionBias: mediacontract.Direction(e.DirectionBias),
		Confidence:    e.Confidence,
		Source:        e.Source,
		Notes:         e.Notes,
		Enabled:       e.Enabled,
	}
	if e.UpdateTime != nil {
		item.UpdatedAt = *e.UpdateTime
	}
	return item
}

func firstNonEmptyStr(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
