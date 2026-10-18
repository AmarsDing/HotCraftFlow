package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/narrativecoinmapping"
	"framework/pkg/mediacontract"
)

// MiaNarrativeMappingRepo 映射表 B 仓储（FR-MIA-02）。
type MiaNarrativeMappingRepo struct {
	data *Data
	log  *log.Helper
}

// NewMiaNarrativeMappingRepo 构造。
func NewMiaNarrativeMappingRepo(data *Data, logger log.Logger) bizmia.NarrativeMappingRepo {
	return &MiaNarrativeMappingRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "mia/narrative-mapping/repo/admin-service")),
	}
}

func (r *MiaNarrativeMappingRepo) Create(ctx context.Context, item *bizmia.NarrativeItem) (*bizmia.NarrativeItem, error) {
	now := time.Now()
	row, err := r.data.db.Client().NarrativeCoinMapping.Create().
		SetNarrative(item.Narrative).
		SetKeywords(item.Keywords).
		SetCoins(item.Coins).
		SetDefaultSentimentHint(narrativecoinmapping.DefaultSentimentHint(item.DefaultSentimentHint)).
		SetTTLHours(item.TTLHours).
		SetConfidence(item.Confidence).
		SetEnabled(item.Enabled).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create narrative mapping failed (narrative=%s): %s", item.Narrative, err.Error())
		return nil, adminV1.ErrorInternalServerError("create narrative mapping failed")
	}
	return toBizNarrativeMapping(row), nil
}

func (r *MiaNarrativeMappingRepo) Update(ctx context.Context, id int64, item *bizmia.NarrativeItem) (*bizmia.NarrativeItem, error) {
	row, err := r.data.db.Client().NarrativeCoinMapping.UpdateOneID(int(id)).
		SetNarrative(item.Narrative).
		SetKeywords(item.Keywords).
		SetCoins(item.Coins).
		SetDefaultSentimentHint(narrativecoinmapping.DefaultSentimentHint(item.DefaultSentimentHint)).
		SetTTLHours(item.TTLHours).
		SetConfidence(item.Confidence).
		SetEnabled(item.Enabled).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update narrative mapping failed (id=%d): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("update narrative mapping failed")
	}
	return toBizNarrativeMapping(row), nil
}

func (r *MiaNarrativeMappingRepo) Delete(ctx context.Context, id int64) error {
	if err := r.data.db.Client().NarrativeCoinMapping.DeleteOneID(int(id)).Exec(ctx); err != nil {
		r.log.Errorf("delete narrative mapping failed (id=%d): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("delete narrative mapping failed")
	}
	return nil
}

func (r *MiaNarrativeMappingRepo) Get(ctx context.Context, id int64) (*bizmia.NarrativeItem, error) {
	row, err := r.data.db.Client().NarrativeCoinMapping.Get(ctx, int(id))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get narrative mapping failed (id=%d): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("get narrative mapping failed")
	}
	return toBizNarrativeMapping(row), nil
}

func (r *MiaNarrativeMappingRepo) List(ctx context.Context, q bizmia.NarrativeQuery) ([]*bizmia.NarrativeItem, int, error) {
	query := r.data.db.Client().NarrativeCoinMapping.Query()
	if q.Keyword != "" {
		query = query.Where(narrativecoinmapping.NarrativeContains(q.Keyword))
	}
	if q.EnabledOnly {
		query = query.Where(narrativecoinmapping.EnabledEQ(true))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count narrative mappings failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query narrative mappings failed")
	}

	rows, err := query.
		Order(ent.Desc(narrativecoinmapping.FieldUpdateTime), ent.Desc(narrativecoinmapping.FieldID)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list narrative mappings failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query narrative mappings failed")
	}
	out := make([]*bizmia.NarrativeItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizNarrativeMapping(row))
	}
	return out, total, nil
}

func (r *MiaNarrativeMappingRepo) ListEnabled(ctx context.Context) ([]*bizmia.NarrativeItem, error) {
	rows, err := r.data.db.Client().NarrativeCoinMapping.Query().
		Where(narrativecoinmapping.EnabledEQ(true)).
		Order(ent.Asc(narrativecoinmapping.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list enabled narrative mappings failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query narrative mappings failed")
	}
	out := make([]*bizmia.NarrativeItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizNarrativeMapping(row))
	}
	return out, nil
}

// UpsertByKey CSV 导入幂等：按唯一键 narrative 查改或新建。
func (r *MiaNarrativeMappingRepo) UpsertByKey(ctx context.Context, item *bizmia.NarrativeItem) (bool, error) {
	existing, err := r.data.db.Client().NarrativeCoinMapping.Query().
		Where(narrativecoinmapping.NarrativeEQ(item.Narrative)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("upsert narrative mapping lookup failed (narrative=%s): %s", item.Narrative, err.Error())
		return false, adminV1.ErrorInternalServerError("upsert narrative mapping failed")
	}
	if existing == nil {
		if _, err := r.Create(ctx, item); err != nil {
			return false, err
		}
		return true, nil
	}
	_, err = r.data.db.Client().NarrativeCoinMapping.UpdateOneID(existing.ID).
		SetKeywords(item.Keywords).
		SetCoins(item.Coins).
		SetDefaultSentimentHint(narrativecoinmapping.DefaultSentimentHint(item.DefaultSentimentHint)).
		SetTTLHours(item.TTLHours).
		SetConfidence(item.Confidence).
		SetEnabled(item.Enabled).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("upsert narrative mapping update failed (id=%d): %s", existing.ID, err.Error())
		return false, adminV1.ErrorInternalServerError("upsert narrative mapping failed")
	}
	return false, nil
}

func (r *MiaNarrativeMappingRepo) Count(ctx context.Context) (int, error) {
	n, err := r.data.db.Client().NarrativeCoinMapping.Query().Count(ctx)
	if err != nil {
		r.log.Errorf("count narrative mappings failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("count narrative mappings failed")
	}
	return n, nil
}

func toBizNarrativeMapping(e *ent.NarrativeCoinMapping) *bizmia.NarrativeItem {
	item := &bizmia.NarrativeItem{
		ID:                   int64(e.ID),
		Narrative:            e.Narrative,
		Keywords:             e.Keywords,
		Coins:                e.Coins,
		DefaultSentimentHint: mediacontract.Direction(e.DefaultSentimentHint),
		TTLHours:             e.TTLHours,
		Confidence:           e.Confidence,
		Enabled:              e.Enabled,
	}
	if e.UpdateTime != nil {
		item.UpdatedAt = *e.UpdateTime
	}
	return item
}
