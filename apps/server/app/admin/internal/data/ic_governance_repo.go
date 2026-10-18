package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/authorizationrecord"
	"framework/app/admin/internal/data/ent/compliancerule"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 合规规则（FR-INT-12 闸门无旁路）+ 授权台账（B-14 #12 authorization 规则数据源）
// =============================================================================

type ComplianceRuleRepo struct {
	data *Data
	log  *log.Helper
}

func NewComplianceRuleRepo(data *Data, logger log.Logger) biz.ComplianceRuleRepo {
	return &ComplianceRuleRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/rule/repo/admin-service")),
		data: data,
	}
}

func (r *ComplianceRuleRepo) Create(ctx context.Context, row *biz.ComplianceRuleRow) (*biz.ComplianceRuleRow, error) {
	now := time.Now().UTC()
	builder := r.data.db.Client().ComplianceRule.Create().
		SetID(contract.NewID(contract.PrefixRule)).
		SetRuleType(row.RuleType).
		SetPattern(row.Pattern).
		SetAction(row.Action).
		SetPriority(row.Priority).
		SetEnabled(row.Enabled).
		SetHitCount(0).
		SetScope(row.Scope).
		SetCreateTime(now).
		SetUpdateTime(now)
	if row.EffectiveFrom != nil {
		builder.SetEffectiveFrom(row.EffectiveFrom.UTC())
	}
	if row.EffectiveUntil != nil {
		builder.SetEffectiveUntil(row.EffectiveUntil.UTC())
	}
	if row.Platform != "" {
		builder.SetPlatform(row.Platform)
	}
	if row.Remark != "" {
		builder.SetRemark(row.Remark)
	}
	if row.CreateBy != 0 {
		builder.SetCreateBy(row.CreateBy)
	}
	if row.UpdateBy != 0 {
		builder.SetUpdateBy(row.UpdateBy)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("create compliance rule failed (type=%s): %s", row.RuleType, err.Error())
		return nil, adminV1.ErrorInternalServerError("create compliance rule failed")
	}
	return toBizComplianceRule(e), nil
}

func (r *ComplianceRuleRepo) GetByID(ctx context.Context, id string) (*biz.ComplianceRuleRow, error) {
	e, err := r.data.db.Client().ComplianceRule.Query().
		Where(compliancerule.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query compliance rule failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("query compliance rule failed")
	}
	return toBizComplianceRule(e), nil
}

func (r *ComplianceRuleRepo) Update(ctx context.Context, row *biz.ComplianceRuleRow) (*biz.ComplianceRuleRow, error) {
	builder := r.data.db.Client().ComplianceRule.UpdateOneID(row.ID).
		SetRuleType(row.RuleType).
		SetPattern(row.Pattern).
		SetAction(row.Action).
		SetPriority(row.Priority).
		SetEnabled(row.Enabled).
		SetScope(row.Scope).
		SetUpdateTime(time.Now().UTC())
	if row.EffectiveFrom != nil {
		builder.SetEffectiveFrom(row.EffectiveFrom.UTC())
	} else {
		builder.ClearEffectiveFrom()
	}
	if row.EffectiveUntil != nil {
		builder.SetEffectiveUntil(row.EffectiveUntil.UTC())
	} else {
		builder.ClearEffectiveUntil()
	}
	if row.Platform != "" {
		builder.SetPlatform(row.Platform)
	} else {
		builder.ClearPlatform()
	}
	if row.Remark != "" {
		builder.SetRemark(row.Remark)
	} else {
		builder.ClearRemark()
	}
	if row.UpdateBy != 0 {
		builder.SetUpdateBy(row.UpdateBy)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, icV1.ErrorIcSrcNotFound("规则不存在")
		}
		r.log.Errorf("update compliance rule failed (id=%s): %s", row.ID, err.Error())
		return nil, adminV1.ErrorInternalServerError("update compliance rule failed")
	}
	return toBizComplianceRule(e), nil
}

func (r *ComplianceRuleRepo) Delete(ctx context.Context, id string) (bool, error) {
	n, err := r.data.db.Client().ComplianceRule.Delete().
		Where(compliancerule.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete compliance rule failed (id=%s): %s", id, err.Error())
		return false, adminV1.ErrorInternalServerError("delete compliance rule failed")
	}
	return n > 0, nil
}

func (r *ComplianceRuleRepo) List(ctx context.Context, q *biz.ComplianceRuleListQuery) ([]*biz.ComplianceRuleRow, int, error) {
	p, s := 1, 20
	var opts biz.ComplianceRuleListQuery
	if q != nil {
		opts = *q
		p, s = icPageClamp(opts.Page, opts.PageSize)
	}
	query := r.data.db.Client().ComplianceRule.Query()
	if opts.RuleType != "" {
		query = query.Where(compliancerule.RuleTypeEQ(opts.RuleType))
	}
	if opts.Action != "" {
		query = query.Where(compliancerule.ActionEQ(opts.Action))
	}
	if opts.Scope != "" {
		query = query.Where(compliancerule.ScopeEQ(opts.Scope))
	}
	if opts.Enabled != nil {
		query = query.Where(compliancerule.EnabledEQ(*opts.Enabled))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count compliance rules failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("count compliance rules failed")
	}
	entities, err := query.
		Order(ent.Desc(compliancerule.FieldPriority), ent.Desc(compliancerule.FieldCreateTime)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list compliance rules failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("list compliance rules failed")
	}
	out := make([]*biz.ComplianceRuleRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizComplianceRule(e))
	}
	return out, total, nil
}

// ListEffective 在生效窗口内的启用规则，priority 降序（闸门严格者胜出的判定在 biz）。
func (r *ComplianceRuleRepo) ListEffective(ctx context.Context, now time.Time) ([]*biz.ComplianceRuleRow, error) {
	now = now.UTC()
	entities, err := r.data.db.Client().ComplianceRule.Query().
		Where(
			compliancerule.EnabledEQ(true),
			compliancerule.Or(
				compliancerule.EffectiveFromIsNil(),
				compliancerule.EffectiveFromLTE(now),
			),
			compliancerule.Or(
				compliancerule.EffectiveUntilIsNil(),
				compliancerule.EffectiveUntilGT(now),
			),
		).
		Order(ent.Desc(compliancerule.FieldPriority)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list effective compliance rules failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("list effective compliance rules failed")
	}
	out := make([]*biz.ComplianceRuleRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizComplianceRule(e))
	}
	return out, nil
}

func (r *ComplianceRuleRepo) IncrementHitCount(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.data.db.Client().ComplianceRule.Update().
		Where(compliancerule.IDIn(ids...)).
		AddHitCount(1).
		Exec(ctx); err != nil {
		r.log.Errorf("increment rule hit count failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("increment rule hit count failed")
	}
	return nil
}

func toBizComplianceRule(e *ent.ComplianceRule) *biz.ComplianceRuleRow {
	if e == nil {
		return nil
	}
	row := &biz.ComplianceRuleRow{
		ID:             e.ID,
		RuleType:       e.RuleType,
		Pattern:        e.Pattern,
		Action:         e.Action,
		Priority:       e.Priority,
		Enabled:        e.Enabled,
		EffectiveFrom:  e.EffectiveFrom,
		EffectiveUntil: e.EffectiveUntil,
		HitCount:       e.HitCount,
		Scope:          e.Scope,
		Platform:       derefStr(e.Platform),
		Remark:         derefStr(e.Remark),
		CreateTime:     e.CreateTime,
		UpdateTime:     e.UpdateTime,
	}
	if e.CreateBy != nil {
		row.CreateBy = *e.CreateBy
	}
	if e.UpdateBy != nil {
		row.UpdateBy = *e.UpdateBy
	}
	return row
}

// ---------- 授权台账 ----------

type AuthorizationRecordRepo struct {
	data *Data
	log  *log.Helper
}

func NewAuthorizationRecordRepo(data *Data, logger log.Logger) biz.AuthorizationRecordRepo {
	return &AuthorizationRecordRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/authorization/repo/admin-service")),
		data: data,
	}
}

func (r *AuthorizationRecordRepo) Upsert(ctx context.Context, row *biz.AuthorizationRecordRow) (*biz.AuthorizationRecordRow, error) {
	existing, err := r.data.db.Client().AuthorizationRecord.Query().
		Where(
			authorizationrecord.SubjectEQ(row.Subject),
			authorizationrecord.SubjectKeyEQ(row.SubjectKey),
		).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("query authorization record failed (subject=%s,key=%s): %s", row.Subject, row.SubjectKey, err.Error())
		return nil, adminV1.ErrorInternalServerError("query authorization record failed")
	}
	now := time.Now().UTC()
	if existing != nil {
		builder := r.data.db.Client().AuthorizationRecord.UpdateOneID(existing.ID).
			SetStatus(row.Status).
			SetUpdateTime(now)
		if row.Scope != "" {
			builder.SetScope(row.Scope)
		} else {
			builder.ClearScope()
		}
		if row.EvidenceRef != "" {
			builder.SetEvidenceRef(row.EvidenceRef)
		} else {
			builder.ClearEvidenceRef()
		}
		if row.ValidUntil != nil {
			builder.SetValidUntil(row.ValidUntil.UTC())
		} else {
			builder.ClearValidUntil()
		}
		e, uerr := builder.Save(ctx)
		if uerr != nil {
			r.log.Errorf("update authorization record failed (id=%s): %s", existing.ID, uerr.Error())
			return nil, adminV1.ErrorInternalServerError("update authorization record failed")
		}
		return toBizAuthorizationRecord(e), nil
	}
	builder := r.data.db.Client().AuthorizationRecord.Create().
		SetID(contract.ULID()).
		SetSubject(row.Subject).
		SetSubjectKey(row.SubjectKey).
		SetStatus(row.Status).
		SetCreateTime(now).
		SetUpdateTime(now)
	if row.Scope != "" {
		builder.SetScope(row.Scope)
	}
	if row.EvidenceRef != "" {
		builder.SetEvidenceRef(row.EvidenceRef)
	}
	if row.ValidUntil != nil {
		builder.SetValidUntil(row.ValidUntil.UTC())
	}
	e, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("create authorization record failed (subject=%s,key=%s): %s", row.Subject, row.SubjectKey, err.Error())
		return nil, adminV1.ErrorInternalServerError("create authorization record failed")
	}
	return toBizAuthorizationRecord(e), nil
}

// Get 闸门查询。B-16 fail-closed：过期记录视为 unknown（不落会假 granted）。
func (r *AuthorizationRecordRepo) Get(ctx context.Context, subject, subjectKey string, now time.Time) (*biz.AuthorizationRecordRow, error) {
	e, err := r.data.db.Client().AuthorizationRecord.Query().
		Where(
			authorizationrecord.SubjectEQ(subject),
			authorizationrecord.SubjectKeyEQ(subjectKey),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query authorization record failed (subject=%s,key=%s): %s", subject, subjectKey, err.Error())
		return nil, adminV1.ErrorInternalServerError("query authorization record failed")
	}
	row := toBizAuthorizationRecord(e)
	if row.ValidUntil != nil && row.ValidUntil.Before(now.UTC()) {
		row.Status = "unknown" // 到期失效：fail-closed（评审 B-16）
	}
	return row, nil
}

func (r *AuthorizationRecordRepo) List(ctx context.Context, subject string, page, pageSize int32) ([]*biz.AuthorizationRecordRow, int, error) {
	p, s := icPageClamp(page, pageSize)
	query := r.data.db.Client().AuthorizationRecord.Query()
	if subject != "" {
		query = query.Where(authorizationrecord.SubjectEQ(subject))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count authorization records failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("count authorization records failed")
	}
	entities, err := query.
		Order(ent.Desc(authorizationrecord.FieldCreateTime)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list authorization records failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("list authorization records failed")
	}
	out := make([]*biz.AuthorizationRecordRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizAuthorizationRecord(e))
	}
	return out, total, nil
}

func (r *AuthorizationRecordRepo) Delete(ctx context.Context, id string) (bool, error) {
	n, err := r.data.db.Client().AuthorizationRecord.Delete().
		Where(authorizationrecord.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete authorization record failed (id=%s): %s", id, err.Error())
		return false, adminV1.ErrorInternalServerError("delete authorization record failed")
	}
	return n > 0, nil
}

func toBizAuthorizationRecord(e *ent.AuthorizationRecord) *biz.AuthorizationRecordRow {
	if e == nil {
		return nil
	}
	return &biz.AuthorizationRecordRow{
		ID:          e.ID,
		Subject:     e.Subject,
		SubjectKey:  e.SubjectKey,
		Status:      e.Status,
		Scope:       derefStr(e.Scope),
		EvidenceRef: derefStr(e.EvidenceRef),
		ValidUntil:  e.ValidUntil,
	}
}
