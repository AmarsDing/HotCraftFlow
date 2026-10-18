package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/miaeventcard"
	"framework/app/admin/internal/data/ent/miaimpactcard"
	"framework/pkg/mediacontract"
)

// MiaEventCardRepo 事件卡仓储（FR-MIA-05）。
type MiaEventCardRepo struct {
	data *Data
	log  *log.Helper
}

// NewMiaEventCardRepo 构造。
func NewMiaEventCardRepo(data *Data, logger log.Logger) bizmia.EventCardRepo {
	return &MiaEventCardRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "mia/event-card/repo/admin-service")),
	}
}

func (r *MiaEventCardRepo) Create(ctx context.Context, row *bizmia.EventCardRow) (*bizmia.EventCardRow, error) {
	now := time.Now()
	if row.SourceCount <= 0 {
		row.SourceCount = sourceCountOf(row.Domains, row.SourceIntelIDs)
	}
	e, err := r.data.db.Client().MiaEventCard.Create().
		SetEventID(row.EventID).
		SetTitle(row.Title).
		SetSummary(row.Summary).
		SetOccurredAt(row.OccurredAt.UTC()).
		SetNillableTopicID(strPtrOrNil(row.TopicID)).
		SetSourceIntelIds(row.SourceIntelIDs).
		SetSourceCount(row.SourceCount).
		SetDomains(row.Domains).
		SetEntities(entitiesToJSON(row.Entities)).
		SetCategory(miaeventcard.Category(row.Category)).
		SetLang(row.Lang).
		SetMergedBy(miaeventcard.MergedBy(firstNonEmptyStr(row.MergedBy, "auto"))).
		SetSplitLocked(row.SplitLocked).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create event card failed (event_id=%s): %s", row.EventID, err.Error())
		return nil, adminV1.ErrorInternalServerError("create event card failed")
	}
	return toBizEventCard(e), nil
}

func (r *MiaEventCardRepo) Get(ctx context.Context, eventID string) (*bizmia.EventCardRow, error) {
	e, err := r.data.db.Client().MiaEventCard.Query().
		Where(miaeventcard.EventIDEQ(eventID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get event card failed (event_id=%s): %s", eventID, err.Error())
		return nil, adminV1.ErrorInternalServerError("get event card failed")
	}
	return toBizEventCard(e), nil
}

func (r *MiaEventCardRepo) List(ctx context.Context, q bizmia.EventQuery) ([]*bizmia.EventCardRow, int, error) {
	query := r.data.db.Client().MiaEventCard.Query()
	if q.Category != "" {
		query = query.Where(miaeventcard.CategoryEQ(miaeventcard.Category(q.Category)))
	}
	if q.Keyword != "" {
		query = query.Where(miaeventcard.TitleContains(q.Keyword))
	}
	if q.OccurredAfter != nil {
		query = query.Where(miaeventcard.OccurredAtGTE(q.OccurredAfter.UTC()))
	}
	if q.OccurredBefore != nil {
		query = query.Where(miaeventcard.OccurredAtLTE(q.OccurredBefore.UTC()))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count event cards failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query event cards failed")
	}

	rows, err := query.
		Order(ent.Desc(miaeventcard.FieldOccurredAt), ent.Desc(miaeventcard.FieldID)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list event cards failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query event cards failed")
	}
	out := make([]*bizmia.EventCardRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizEventCard(e))
	}
	return out, total, nil
}

// ListRecent 拉取近 window 的事件卡供自动聚类比对（上限 200，防窗口内数据膨胀拖慢聚类）。
func (r *MiaEventCardRepo) ListRecent(ctx context.Context, window time.Duration) ([]*bizmia.EventCardRow, error) {
	since := time.Now().Add(-window).UTC()
	rows, err := r.data.db.Client().MiaEventCard.Query().
		Where(miaeventcard.OccurredAtGTE(since)).
		Order(ent.Desc(miaeventcard.FieldOccurredAt)).
		Limit(200).
		All(ctx)
	if err != nil {
		r.log.Errorf("list recent event cards failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query recent event cards failed")
	}
	out := make([]*bizmia.EventCardRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizEventCard(e))
	}
	return out, nil
}

// AbsorbSource 自动聚类合并：把新来源追加进既有事件卡。
// 发生时间取最早源时间；source_count 按去重域名重算。
// 读改写无锁：聚类消费串行度低，乐观接受最后一次写入（幂等消费兜底）。
func (r *MiaEventCardRepo) AbsorbSource(ctx context.Context, eventID, intelID, domain string, occurredAt time.Time) error {
	e, err := r.data.db.Client().MiaEventCard.Query().
		Where(miaeventcard.EventIDEQ(eventID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil // 卡已被人为删除，静默跳过
		}
		r.log.Errorf("absorb source lookup failed (event_id=%s): %s", eventID, err.Error())
		return adminV1.ErrorInternalServerError("absorb event source failed")
	}

	sources := appendUniqueStr(e.SourceIntelIds, intelID)
	domains := appendUniqueStr(e.Domains, domain)
	occurred := e.OccurredAt
	if occurredAt.Before(occurred) {
		occurred = occurredAt.UTC()
	}
	_, err = r.data.db.Client().MiaEventCard.UpdateOneID(e.ID).
		SetSourceIntelIds(sources).
		SetDomains(domains).
		SetSourceCount(sourceCountOf(domains, sources)).
		SetOccurredAt(occurred).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("absorb source failed (event_id=%s intel=%s): %s", eventID, intelID, err.Error())
		return adminV1.ErrorInternalServerError("absorb event source failed")
	}
	return nil
}

// ReplaceSourcesForSplit 人工拆分：事务内更新父卡 + 创建新卡，两卡均 split_locked。
func (r *MiaEventCardRepo) ReplaceSourcesForSplit(ctx context.Context, parent, newCard *bizmia.EventCardRow) error {
	return RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
		now := time.Now()
		if parent.SourceCount <= 0 {
			parent.SourceCount = sourceCountOf(parent.Domains, parent.SourceIntelIDs)
		}
		_, err := tx.MiaEventCard.UpdateOneID(int(parent.ID)).
			SetSourceIntelIds(parent.SourceIntelIDs).
			SetDomains(parent.Domains).
			SetSourceCount(parent.SourceCount).
			SetSplitLocked(true).
			SetMergedBy(miaeventcard.MergedByManual).
			SetUpdateTime(now).
			Save(ctx)
		if err != nil {
			return err
		}
		if newCard.SourceCount <= 0 {
			newCard.SourceCount = sourceCountOf(newCard.Domains, newCard.SourceIntelIDs)
		}
		builder := tx.MiaEventCard.Create().
			SetEventID(newCard.EventID).
			SetTitle(newCard.Title).
			SetSummary(newCard.Summary).
			SetOccurredAt(newCard.OccurredAt.UTC()).
			SetNillableTopicID(strPtrOrNil(newCard.TopicID)).
			SetSourceIntelIds(newCard.SourceIntelIDs).
			SetSourceCount(newCard.SourceCount).
			SetDomains(newCard.Domains).
			SetEntities(entitiesToJSON(newCard.Entities)).
			SetCategory(miaeventcard.Category(newCard.Category)).
			SetLang(newCard.Lang).
			SetMergedBy(miaeventcard.MergedByManual).
			SetSplitLocked(true).
			SetCreateTime(now).
			SetUpdateTime(now)
		_, err = builder.Save(ctx)
		return err
	})
}

// MergeInto 人工合并：事务内把 absorbed 卡的来源并入 target，两卡锁定，
// absorbed 卡删除；被并卡名下的影响卡改挂 target（影响卡自含标题/路径，不丢审计）。
func (r *MiaEventCardRepo) MergeInto(ctx context.Context, targetID string, absorbedIDs []string) error {
	return RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
		target, err := tx.MiaEventCard.Query().
			Where(miaeventcard.EventIDEQ(targetID)).
			First(ctx)
		if err != nil {
			return err
		}
		absorbed, err := tx.MiaEventCard.Query().
			Where(miaeventcard.EventIDIn(absorbedIDs...)).
			All(ctx)
		if err != nil {
			return err
		}

		sources := target.SourceIntelIds
		domains := target.Domains
		occurred := target.OccurredAt
		for _, a := range absorbed {
			for _, s := range a.SourceIntelIds {
				sources = appendUniqueStr(sources, s)
			}
			for _, d := range a.Domains {
				domains = appendUniqueStr(domains, d)
			}
			if a.OccurredAt.Before(occurred) {
				occurred = a.OccurredAt
			}
		}

		now := time.Now()
		if _, err := tx.MiaEventCard.UpdateOneID(target.ID).
			SetSourceIntelIds(sources).
			SetDomains(domains).
			SetSourceCount(sourceCountOf(domains, sources)).
			SetOccurredAt(occurred).
			SetSplitLocked(true).
			SetMergedBy(miaeventcard.MergedByManual).
			SetUpdateTime(now).
			Save(ctx); err != nil {
			return err
		}
		// 被并卡名下的影响卡改挂 target（自含字段不受影响）。
		if len(absorbedIDs) > 0 {
			if _, err := tx.MiaImpactCard.Update().
				Where(miaimpactcard.EventIDIn(absorbedIDs...)).
				SetEventID(targetID).
				SetUpdateTime(now).
				Save(ctx); err != nil {
				return err
			}
			if _, err := tx.MiaEventCard.Delete().
				Where(miaeventcard.EventIDIn(absorbedIDs...)).
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// sourceCountOf 转载源数：按不同域名去重计数（无域名信息时以来源数兜底）。
func sourceCountOf(domains, sources []string) int {
	if len(domains) > 0 {
		return len(domains)
	}
	if len(sources) > 0 {
		return len(sources)
	}
	return 1
}

func appendUniqueStr(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, s := range list {
		if s == v {
			return list
		}
	}
	return append(list, v)
}

func strPtrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// derefStr 复用 api_resource_repo.go 中同包实现，不重复定义。

func entitiesToJSON(entities []mediacontract.Entity) []map[string]any {
	out := make([]map[string]any, 0, len(entities))
	for _, e := range entities {
		out = append(out, map[string]any{
			"type":       string(e.Type),
			"value":      e.Value,
			"confidence": e.Confidence,
		})
	}
	return out
}

func entitiesFromJSON(rows []map[string]any) []mediacontract.Entity {
	out := make([]mediacontract.Entity, 0, len(rows))
	for _, row := range rows {
		var e mediacontract.Entity
		if v, ok := row["type"].(string); ok {
			e.Type = mediacontract.EntityType(v)
		}
		if v, ok := row["value"].(string); ok {
			e.Value = v
		}
		if v, ok := row["confidence"].(float64); ok {
			e.Confidence = v
		}
		if e.Value != "" {
			out = append(out, e)
		}
	}
	return out
}

func toBizEventCard(e *ent.MiaEventCard) *bizmia.EventCardRow {
	row := &bizmia.EventCardRow{
		ID:             int64(e.ID),
		EventID:        e.EventID,
		Title:          e.Title,
		Summary:        derefStr(e.Summary),
		OccurredAt:     e.OccurredAt,
		TopicID:        derefStr(e.TopicID),
		SourceIntelIDs: e.SourceIntelIds,
		SourceCount:    e.SourceCount,
		Domains:        e.Domains,
		Entities:       entitiesFromJSON(e.Entities),
		Category:       string(e.Category),
		Lang:           e.Lang,
		MergedBy:       string(e.MergedBy),
		SplitLocked:    e.SplitLocked,
	}
	if e.CreateTime != nil {
		row.CreatedAt = *e.CreateTime
	}
	return row
}
