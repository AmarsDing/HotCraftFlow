package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/miatransmissionrule"
	"framework/pkg/mediacontract"
)

// MiaTransmissionRuleRepo 传导规则仓储（FR-MIA-08）。
type MiaTransmissionRuleRepo struct {
	data *Data
	log  *log.Helper
}

// NewMiaTransmissionRuleRepo 构造。
func NewMiaTransmissionRuleRepo(data *Data, logger log.Logger) bizmia.TransmissionRuleRepo {
	return &MiaTransmissionRuleRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "mia/transmission-rule/repo/admin-service")),
	}
}

func (r *MiaTransmissionRuleRepo) Create(ctx context.Context, row *bizmia.TransmissionRuleRow) (*bizmia.TransmissionRuleRow, error) {
	now := time.Now()
	e, err := r.data.db.Client().MiaTransmissionRule.Create().
		SetRuleID(row.RuleID).
		SetTrigger(triggerToJSON(row.Trigger)).
		SetNillableFromAsset(strPtrOrNil(row.FromAsset)).
		SetToAsset(row.ToAsset).
		SetToDirection(miatransmissionrule.ToDirection(row.ToDirection)).
		SetConfidence(row.Confidence).
		SetNote(row.Note).
		SetEnabled(row.Enabled).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create transmission rule failed (rule_id=%s): %s", row.RuleID, err.Error())
		return nil, adminV1.ErrorInternalServerError("create transmission rule failed")
	}
	return toBizTransmissionRule(e), nil
}

func (r *MiaTransmissionRuleRepo) Update(ctx context.Context, id int64, row *bizmia.TransmissionRuleRow) (*bizmia.TransmissionRuleRow, error) {
	e, err := r.data.db.Client().MiaTransmissionRule.UpdateOneID(int(id)).
		SetTrigger(triggerToJSON(row.Trigger)).
		SetNillableFromAsset(strPtrOrNil(row.FromAsset)).
		SetToAsset(row.ToAsset).
		SetToDirection(miatransmissionrule.ToDirection(row.ToDirection)).
		SetConfidence(row.Confidence).
		SetNote(row.Note).
		SetEnabled(row.Enabled).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update transmission rule failed (id=%d): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("update transmission rule failed")
	}
	return toBizTransmissionRule(e), nil
}

func (r *MiaTransmissionRuleRepo) Delete(ctx context.Context, id int64) error {
	if err := r.data.db.Client().MiaTransmissionRule.DeleteOneID(int(id)).Exec(ctx); err != nil {
		r.log.Errorf("delete transmission rule failed (id=%d): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("delete transmission rule failed")
	}
	return nil
}

func (r *MiaTransmissionRuleRepo) List(ctx context.Context, enabledOnly bool) ([]*bizmia.TransmissionRuleRow, error) {
	query := r.data.db.Client().MiaTransmissionRule.Query()
	if enabledOnly {
		query = query.Where(miatransmissionrule.EnabledEQ(true))
	}
	rows, err := query.
		Order(ent.Asc(miatransmissionrule.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list transmission rules failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query transmission rules failed")
	}
	out := make([]*bizmia.TransmissionRuleRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, toBizTransmissionRule(e))
	}
	return out, nil
}

func (r *MiaTransmissionRuleRepo) Count(ctx context.Context) (int, error) {
	n, err := r.data.db.Client().MiaTransmissionRule.Query().Count(ctx)
	if err != nil {
		r.log.Errorf("count transmission rules failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("count transmission rules failed")
	}
	return n, nil
}

// triggerToJSON 触发条件 → JSONB 列。
func triggerToJSON(t mediacontract.TriggerSpec) map[string]any {
	return map[string]any{
		"asset":        t.Asset,
		"direction":    string(t.Direction),
		"min_move_pct": t.MinMovePct,
	}
}

// triggerFromJSON JSONB 列 → 触发条件。
func triggerFromJSON(m map[string]any) mediacontract.TriggerSpec {
	var t mediacontract.TriggerSpec
	if v, ok := m["asset"].(string); ok {
		t.Asset = v
	}
	if v, ok := m["direction"].(string); ok {
		t.Direction = mediacontract.Direction(v)
	}
	if v, ok := m["min_move_pct"].(float64); ok {
		t.MinMovePct = v
	}
	return t
}

func toBizTransmissionRule(e *ent.MiaTransmissionRule) *bizmia.TransmissionRuleRow {
	return &bizmia.TransmissionRuleRow{
		ID:           int64(e.ID),
		RuleID:       e.RuleID,
		Trigger:      triggerFromJSON(e.Trigger),
		FromAsset:    derefStr(e.FromAsset),
		ToAsset:      e.ToAsset,
		ToDirection:  mediacontract.Direction(e.ToDirection),
		Confidence:   e.Confidence,
		Note:         e.Note,
		Enabled:      e.Enabled,
		HitCount:     e.HitCount,
		CorrectCount: e.CorrectCount,
	}
}
