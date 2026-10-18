package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/miaimpactcard"
	"framework/pkg/mediacontract"
)

// MiaImpactCardRepo 影响卡仓储（FR-MIA-06）。
// 🔴 三项硬校验在 biz 层（Create 前）已完成；本层只做持久化。
type MiaImpactCardRepo struct {
	data *Data
	log  *log.Helper
}

// NewMiaImpactCardRepo 构造。
func NewMiaImpactCardRepo(data *Data, logger log.Logger) bizmia.ImpactCardRepo {
	return &MiaImpactCardRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "mia/impact-card/repo/admin-service")),
	}
}

func (r *MiaImpactCardRepo) Create(ctx context.Context, row *bizmia.ImpactCardRow) (*bizmia.ImpactCardRow, error) {
	now := time.Now()
	e, err := r.data.db.Client().MiaImpactCard.Create().
		SetImpactID(row.ImpactID).
		SetEventID(row.EventID).
		SetNillableTopicID(strPtrOrNil(row.TopicID)).
		SetTitle(row.Title).
		SetSummary(row.Summary).
		SetLang(row.Lang).
		SetSentimentLabel(miaimpactcard.SentimentLabel(row.SentimentLabel)).
		SetSentimentScore(row.SentimentScore).
		SetSentimentEngine(miaimpactcard.SentimentEngine(row.SentimentEngine)).
		SetImpactScore(row.ImpactScore).
		SetDirection(miaimpactcard.Direction(row.Direction)).
		SetTTLHours(row.TTLHours).
		SetExpiresAt(row.ExpiresAt.UTC()).
		SetFactors(impactFactorsToJSON(row.Factors)).
		SetAffected(row.Affected).
		SetNoMapping(row.NoMapping).
		SetDisclaimer(row.Disclaimer).
		SetNillableRuleVersion(strPtrOrNil(row.RuleVersion)).
		SetWeightsSnapshot(row.WeightsSnapshot).
		SetOccurredAt(row.OccurredAt.UTC()).
		SetPushed(false).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create impact card failed (impact_id=%s): %s", row.ImpactID, err.Error())
		return nil, adminV1.ErrorInternalServerError("create impact card failed")
	}
	return toBizImpactCard(e), nil
}

func (r *MiaImpactCardRepo) Get(ctx context.Context, impactID string) (*bizmia.ImpactCardRow, error) {
	e, err := r.data.db.Client().MiaImpactCard.Query().
		Where(miaimpactcard.ImpactIDEQ(impactID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get impact card failed (impact_id=%s): %s", impactID, err.Error())
		return nil, adminV1.ErrorInternalServerError("get impact card failed")
	}
	return toBizImpactCard(e), nil
}

// List 影响卡列表。默认过滤已过期（TTL 纪律：过期卡不占排行榜但不删除）。
// 排序：影响分降序优先（排行榜语义），同分按发生时间倒序。
func (r *MiaImpactCardRepo) List(ctx context.Context, q bizmia.ImpactQuery) ([]*bizmia.ImpactCardRow, int, error) {
	query := r.data.db.Client().MiaImpactCard.Query()
	if !q.IncludeExpired {
		query = query.Where(miaimpactcard.ExpiresAtGT(time.Now().UTC()))
	}
	if d := mediacontract.Direction(q.Direction); d.Valid() {
		query = query.Where(miaimpactcard.DirectionEQ(miaimpactcard.Direction(d)))
	}
	if q.MinScore > 0 {
		query = query.Where(miaimpactcard.ImpactScoreGTE(q.MinScore))
	}
	if q.OccurredAfter != nil {
		query = query.Where(miaimpactcard.OccurredAtGTE(q.OccurredAfter.UTC()))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count impact cards failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query impact cards failed")
	}

	rows, err := query.
		Order(ent.Desc(miaimpactcard.FieldImpactScore), ent.Desc(miaimpactcard.FieldOccurredAt), ent.Desc(miaimpactcard.FieldID)).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list impact cards failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("query impact cards failed")
	}
	out := make([]*bizmia.ImpactCardRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizImpactCard(e))
	}
	return out, total, nil
}

func (r *MiaImpactCardRepo) MarkPushed(ctx context.Context, impactID string) error {
	err := r.data.db.Client().MiaImpactCard.Update().
		Where(miaimpactcard.ImpactIDEQ(impactID)).
		SetPushed(true).
		SetUpdateTime(time.Now()).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("mark impact card pushed failed (impact_id=%s): %s", impactID, err.Error())
		return adminV1.ErrorInternalServerError("mark impact card pushed failed")
	}
	return nil
}

// impactFactorsToJSON 四因子 → JSON 列。
func impactFactorsToJSON(f mediacontract.ImpactFactors) map[string]any {
	return map[string]any{
		"sentiment":     f.Sentiment,
		"source_weight": f.SourceWeight,
		"relevance":     f.Relevance,
		"decay":         f.Decay,
	}
}

// impactFactorsFromJSON JSON 列 → 四因子（字段缺失容忍零值）。
func impactFactorsFromJSON(m map[string]any) mediacontract.ImpactFactors {
	num := func(k string) float64 {
		if v, ok := m[k].(float64); ok {
			return v
		}
		return 0
	}
	return mediacontract.ImpactFactors{
		Sentiment:    num("sentiment"),
		SourceWeight: num("source_weight"),
		Relevance:    num("relevance"),
		Decay:        num("decay"),
	}
}

func toBizImpactCard(e *ent.MiaImpactCard) *bizmia.ImpactCardRow {
	row := &bizmia.ImpactCardRow{
		ID:              int64(e.ID),
		ImpactID:        e.ImpactID,
		EventID:         e.EventID,
		TopicID:         derefStr(e.TopicID),
		Title:           e.Title,
		Summary:         derefStr(e.Summary),
		Lang:            e.Lang,
		SentimentLabel:  mediacontract.SentimentLabel(e.SentimentLabel),
		SentimentScore:  e.SentimentScore,
		SentimentEngine: mediacontract.SentimentEngine(e.SentimentEngine),
		ImpactScore:     e.ImpactScore,
		Direction:       mediacontract.Direction(e.Direction),
		TTLHours:        e.TTLHours,
		ExpiresAt:       e.ExpiresAt,
		Factors:         impactFactorsFromJSON(e.Factors),
		Affected:        e.Affected,
		NoMapping:       e.NoMapping,
		Disclaimer:      e.Disclaimer,
		RuleVersion:     derefStr(e.RuleVersion),
		WeightsSnapshot: e.WeightsSnapshot,
		OccurredAt:      e.OccurredAt,
		Pushed:          e.Pushed,
	}
	if e.CreateTime != nil {
		row.CreatedAt = *e.CreateTime
	}
	return row
}
