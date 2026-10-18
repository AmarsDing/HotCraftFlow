package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/credentialaccount"
	"framework/app/admin/internal/data/ent/intelsource"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 信源（S-12 熔断 / S-18 与 S-20 校验在 service 层）+ 平台凭据账号池（FR-INT-13）
// =============================================================================

type IntelSourceRepo struct {
	data *Data
	log  *log.Helper
}

func NewIntelSourceRepo(data *Data, logger log.Logger) biz.IntelSourceRepo {
	return &IntelSourceRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/source/repo/admin-service")),
		data: data,
	}
}

func (r *IntelSourceRepo) Create(ctx context.Context, in *biz.IntelSourceCreate) (*biz.IntelSourceRow, error) {
	now := time.Now().UTC()
	builder := r.data.db.Client().IntelSource.Create().
		SetID(contract.NewID(contract.PrefixIntelSource)).
		SetPlatform(in.Platform).
		SetRegion(in.Region).
		SetKind(in.Kind).
		SetMethod(in.Method).
		SetConfig(in.Config).
		SetEnabled(in.Enabled).
		SetIntervalMinutes(in.IntervalMinutes).
		SetRateLimitPerMin(in.RateLimitPerMin).
		SetPriority(in.Priority).
		SetWeight(in.Weight).
		SetRobotsExempt(in.RobotsExempt).
		SetStatus("active").
		SetFailStreak(0).
		SetSuccessCount7d(0).
		SetFailCount7d(0).
		SetCreateTime(now).
		SetUpdateTime(now)
	if in.ConfigSchemaHint != "" {
		builder.SetConfigSchemaHint(in.ConfigSchemaHint)
	}
	if in.CredentialRef != "" {
		builder.SetCredentialRef(in.CredentialRef)
	}
	if in.RobotsExemptReason != "" {
		builder.SetRobotsExemptReason(in.RobotsExemptReason)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, icV1.ErrorIcConfigInvalid("信源配置冲突")
		}
		r.log.Errorf("create intel source failed (platform=%s/%s): %s", in.Platform, in.Method, err.Error())
		return nil, adminV1.ErrorInternalServerError("create intel source failed")
	}
	return toBizIntelSource(e), nil
}

func (r *IntelSourceRepo) GetByID(ctx context.Context, id string) (*biz.IntelSourceRow, error) {
	e, err := r.data.db.Client().IntelSource.Query().
		Where(intelsource.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query intel source failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("query intel source failed")
	}
	return toBizIntelSource(e), nil
}

func (r *IntelSourceRepo) Update(ctx context.Context, id string, patch *biz.IntelSourcePatch) (*biz.IntelSourceRow, error) {
	builder := r.data.db.Client().IntelSource.UpdateOneID(id).
		SetUpdateTime(time.Now().UTC())
	if patch == nil {
		return r.GetByID(ctx, id)
	}
	if patch.Config != nil {
		builder.SetConfig(*patch.Config)
	}
	if patch.Enabled != nil {
		builder.SetEnabled(*patch.Enabled)
	}
	if patch.IntervalMinutes != nil {
		builder.SetIntervalMinutes(*patch.IntervalMinutes)
	}
	if patch.RateLimitPerMin != nil {
		builder.SetRateLimitPerMin(*patch.RateLimitPerMin)
	}
	if patch.Priority != nil {
		builder.SetPriority(*patch.Priority)
	}
	if patch.CredentialRef != nil {
		builder.SetCredentialRef(*patch.CredentialRef)
	}
	if patch.Weight != nil {
		builder.SetWeight(*patch.Weight)
	}
	if patch.RobotsExempt != nil {
		builder.SetRobotsExempt(*patch.RobotsExempt)
	}
	if patch.RobotsExemptReason != nil {
		builder.SetRobotsExemptReason(*patch.RobotsExemptReason)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, icV1.ErrorIcSrcNotFound("信源不存在")
		}
		r.log.Errorf("update intel source failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("update intel source failed")
	}
	return toBizIntelSource(e), nil
}

func (r *IntelSourceRepo) Delete(ctx context.Context, id string) (bool, error) {
	n, err := r.data.db.Client().IntelSource.Delete().
		Where(intelsource.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete intel source failed (id=%s): %s", id, err.Error())
		return false, adminV1.ErrorInternalServerError("delete intel source failed")
	}
	return n > 0, nil
}

func (r *IntelSourceRepo) List(ctx context.Context, q *biz.IntelSourceListQuery) ([]*biz.IntelSourceRow, int, error) {
	p, s := 1, 20
	var opts biz.IntelSourceListQuery
	if q != nil {
		opts = *q
	}
	p, s = icPageClamp(opts.Page, opts.PageSize)

	query := r.data.db.Client().IntelSource.Query()
	if opts.Platform != "" {
		query = query.Where(intelsource.PlatformEQ(opts.Platform))
	}
	if opts.Kind != "" {
		query = query.Where(intelsource.KindEQ(opts.Kind))
	}
	if opts.Method != "" {
		query = query.Where(intelsource.MethodEQ(opts.Method))
	}
	if opts.Status != "" {
		query = query.Where(intelsource.StatusEQ(opts.Status))
	}
	if opts.Enabled != nil {
		query = query.Where(intelsource.EnabledEQ(*opts.Enabled))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count intel sources failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("count intel sources failed")
	}
	entities, err := query.
		Order(ent.Asc(intelsource.FieldPriority), ent.Desc(intelsource.FieldCreateTime)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list intel sources failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("list intel sources failed")
	}
	out := make([]*biz.IntelSourceRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizIntelSource(e))
	}
	return out, total, nil
}

// ListAllEnabled 调度器全量拉取：enabled=真 且 未 disabled（B-11 interval_minutes 唯一真值）。
func (r *IntelSourceRepo) ListAllEnabled(ctx context.Context) ([]*biz.IntelSourceRow, error) {
	entities, err := r.data.db.Client().IntelSource.Query().
		Where(intelsource.EnabledEQ(true), intelsource.StatusNEQ("disabled")).
		All(ctx)
	if err != nil {
		r.log.Errorf("list enabled intel sources failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("list enabled intel sources failed")
	}
	out := make([]*biz.IntelSourceRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizIntelSource(e))
	}
	return out, nil
}

func (r *IntelSourceRepo) SetStatus(ctx context.Context, id, status string, pausedUntil *time.Time) error {
	builder := r.data.db.Client().IntelSource.UpdateOneID(id).
		SetStatus(status).
		SetUpdateTime(time.Now().UTC())
	if pausedUntil != nil {
		builder.SetPausedUntil(pausedUntil.UTC())
	} else {
		builder.ClearPausedUntil()
	}
	// 恢复运行时清零失败计数（S-12）
	if status == "active" {
		builder.SetFailStreak(0)
	}
	if _, err := builder.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return icV1.ErrorIcSrcNotFound("信源不存在")
		}
		r.log.Errorf("set source status failed (id=%s,status=%s): %s", id, status, err.Error())
		return adminV1.ErrorInternalServerError("set source status failed")
	}
	return nil
}

// RecordRunOutcome 运行回执（S-12 熔断器：连续失败达阈值 → paused + cooldown）。
func (r *IntelSourceRepo) RecordRunOutcome(ctx context.Context, id string, success bool, errMsg string, at time.Time, failThreshold int32, cooldown time.Duration) error {
	e, err := r.data.db.Client().IntelSource.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return icV1.ErrorIcSrcNotFound("信源不存在")
		}
		r.log.Errorf("get source for outcome failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("record run outcome failed")
	}
	builder := r.data.db.Client().IntelSource.UpdateOneID(id).
		SetLastRunAt(at.UTC()).
		SetUpdateTime(time.Now().UTC())
	if success {
		builder.SetFailStreak(0).
			SetLastSuccessAt(at.UTC()).
			AddSuccessCount7d(1).
			ClearLastError().
			ClearPausedUntil()
		// 熔断自动恢复：成功后若因暂停中，解除暂停（S-12）
		if e.Status == "paused" {
			builder.SetStatus("active")
		}
	} else {
		streak := e.FailStreak + 1
		builder.SetFailStreak(streak).AddFailCount7d(1)
		const maxErrLen = 512 // 错误截断：免占满列 + 日志脱敏一致性
		if len(errMsg) > maxErrLen {
			errMsg = errMsg[:maxErrLen]
		}
		builder.SetLastError(errMsg)
		if failThreshold > 0 && streak >= failThreshold {
			until := at.UTC().Add(cooldown)
			builder.SetStatus("paused").SetPausedUntil(until)
			r.log.Warnf("source circuit opened (id=%s, streak=%d, until=%s)", id, streak, until.Format(time.RFC3339))
		}
	}
	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf("record run outcome failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("record run outcome failed")
	}
	return nil
}

func (r *IntelSourceRepo) Recompute7dCounts(ctx context.Context, id string, success, fail int32) error {
	if _, err := r.data.db.Client().IntelSource.UpdateOneID(id).
		SetSuccessCount7d(success).
		SetFailCount7d(fail).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("recompute 7d counts failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("recompute 7d counts failed")
	}
	return nil
}

func toBizIntelSource(e *ent.IntelSource) *biz.IntelSourceRow {
	if e == nil {
		return nil
	}
	return &biz.IntelSourceRow{
		ID:                 e.ID,
		Platform:           e.Platform,
		Region:             e.Region,
		Kind:               e.Kind,
		Method:             e.Method,
		Config:             e.Config,
		ConfigSchemaHint:   derefStr(e.ConfigSchemaHint),
		Enabled:            e.Enabled,
		IntervalMinutes:    e.IntervalMinutes,
		RateLimitPerMin:    e.RateLimitPerMin,
		Priority:           e.Priority,
		CredentialRef:      derefStr(e.CredentialRef),
		Weight:             e.Weight,
		RobotsExempt:       e.RobotsExempt,
		RobotsExemptReason: derefStr(e.RobotsExemptReason),
		Status:             e.Status,
		PausedUntil:        e.PausedUntil,
		FailStreak:         e.FailStreak,
		LastRunAt:          e.LastRunAt,
		LastSuccessAt:      e.LastSuccessAt,
		SuccessCount7d:     e.SuccessCount7d,
		FailCount7d:        e.FailCount7d,
		LastError:          derefStr(e.LastError),
		CreateTime:         e.CreateTime,
		UpdateTime:         e.UpdateTime,
	}
}

// ---------- 平台凭据账号池 ----------

// CredentialAccountRepo 凭据行只存密文引用（NFR-IC-06：绝不落明文、绝不进日志）。
type CredentialAccountRepo struct {
	data *Data
	log  *log.Helper
}

func NewCredentialAccountRepo(data *Data, logger log.Logger) biz.CredentialAccountRepo {
	return &CredentialAccountRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/credential/repo/admin-service")),
		data: data,
	}
}

func (r *CredentialAccountRepo) Create(ctx context.Context, row *biz.CredentialAccountRow) (*biz.CredentialAccountRow, error) {
	id := row.ID
	if id == "" {
		id = contract.NewID(contract.PrefixCredentialAcct)
	}
	now := time.Now().UTC()
	builder := r.data.db.Client().CredentialAccount.Create().
		SetID(id).
		SetPlatform(row.Platform).
		SetAccountKey(row.AccountKey).
		SetCredentialRef(row.CredentialRef).
		SetEnabled(row.Enabled).
		SetQuotaUsed(row.QuotaUsed).
		SetQuotaLimit(row.QuotaLimit).
		SetFailStreak(row.FailStreak).
		SetCreateTime(now).
		SetUpdateTime(now)
	if row.PausedUntil != nil {
		builder.SetPausedUntil(row.PausedUntil.UTC())
	}
	if row.LastUsedAt != nil {
		builder.SetLastUsedAt(row.LastUsedAt.UTC())
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, icV1.ErrorIcConfigInvalid("该平台账号已存在")
		}
		r.log.Errorf("create credential account failed (platform=%s, key=%s): %s", row.Platform, row.AccountKey, err.Error())
		return nil, adminV1.ErrorInternalServerError("create credential account failed")
	}
	return toBizCredentialAccount(e), nil
}

func (r *CredentialAccountRepo) GetByID(ctx context.Context, id string) (*biz.CredentialAccountRow, error) {
	e, err := r.data.db.Client().CredentialAccount.Query().
		Where(credentialaccount.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query credential account failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("query credential account failed")
	}
	return toBizCredentialAccount(e), nil
}

func (r *CredentialAccountRepo) ListByPlatform(ctx context.Context, platform string, onlyEnabled bool) ([]*biz.CredentialAccountRow, error) {
	query := r.data.db.Client().CredentialAccount.Query().
		Where(credentialaccount.PlatformEQ(platform))
	if onlyEnabled {
		query = query.Where(credentialaccount.EnabledEQ(true))
	}
	entities, err := query.Order(ent.Asc(credentialaccount.FieldLastUsedAt)).All(ctx)
	if err != nil {
		r.log.Errorf("list credential accounts failed (platform=%s): %s", platform, err.Error())
		return nil, adminV1.ErrorInternalServerError("list credential accounts failed")
	}
	out := make([]*biz.CredentialAccountRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizCredentialAccount(e))
	}
	return out, nil
}

func (r *CredentialAccountRepo) Update(ctx context.Context, row *biz.CredentialAccountRow) error {
	builder := r.data.db.Client().CredentialAccount.UpdateOneID(row.ID).
		SetCredentialRef(row.CredentialRef).
		SetEnabled(row.Enabled).
		SetQuotaLimit(row.QuotaLimit).
		SetUpdateTime(time.Now().UTC())
	if row.PausedUntil != nil {
		builder.SetPausedUntil(row.PausedUntil.UTC())
	} else {
		builder.ClearPausedUntil()
	}
	if _, err := builder.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return icV1.ErrorIcSrcNotFound("账号不存在")
		}
		r.log.Errorf("update credential account failed (id=%s): %s", row.ID, err.Error())
		return adminV1.ErrorInternalServerError("update credential account failed")
	}
	return nil
}

// RecordUsage 使用回执（FR-INT-13 轮换依据；失败累加 fail_streak）。
func (r *CredentialAccountRepo) RecordUsage(ctx context.Context, id string, ok bool, at time.Time, pausedUntil *time.Time) error {
	builder := r.data.db.Client().CredentialAccount.UpdateOneID(id).
		SetUpdateTime(time.Now().UTC())
	if ok {
		builder.SetLastUsedAt(at.UTC()).
			AddQuotaUsed(1).
			SetFailStreak(0)
	} else {
		builder.AddFailStreak(1)
	}
	if pausedUntil != nil {
		builder.SetPausedUntil(pausedUntil.UTC())
	}
	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf("record credential usage failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("record credential usage failed")
	}
	return nil
}

func toBizCredentialAccount(e *ent.CredentialAccount) *biz.CredentialAccountRow {
	if e == nil {
		return nil
	}
	return &biz.CredentialAccountRow{
		ID:            e.ID,
		Platform:      e.Platform,
		AccountKey:    e.AccountKey,
		CredentialRef: e.CredentialRef,
		Enabled:       e.Enabled,
		QuotaUsed:     e.QuotaUsed,
		QuotaLimit:    e.QuotaLimit,
		FailStreak:    e.FailStreak,
		PausedUntil:   e.PausedUntil,
		LastUsedAt:    e.LastUsedAt,
	}
}
