package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
)

// =============================================================================
// ① IC · IntelSourceService —— 信源注册表 CRUD + 手动触发 + 合规规则 CRUD。
// 契约枚举词表校验在本层（source.proto 头部约定：service 层校验，proto 不承载 enum）。
// biz 哨兵错误 → ic_error.proto 错误码（HTTP 状态由生成代码锚定）。
// =============================================================================

// IcSourceService 信源与合规规则管理。
type IcSourceService struct {
	icV1.UnimplementedIntelSourceServiceServer

	sourceUC  *biz.IcSourceUsecase
	collectUC *biz.IcCollectUsecase
	rules     biz.ComplianceRuleRepo
	log       *log.Helper
}

// NewIcSourceService 构造。
func NewIcSourceService(
	sourceUC *biz.IcSourceUsecase,
	collectUC *biz.IcCollectUsecase,
	rules biz.ComplianceRuleRepo,
	logger log.Logger,
) *IcSourceService {
	return &IcSourceService{
		sourceUC: sourceUC, collectUC: collectUC, rules: rules,
		log: log.NewHelper(log.With(logger, "module", "ic/source/service")),
	}
}

// ---------- 信源 CRUD ----------

func (s *IcSourceService) ListIntelSources(ctx context.Context, req *icV1.ListIntelSourcesRequest) (*icV1.ListIntelSourcesResponse, error) {
	page, pageSize := icPageClamp(req.GetPage(), req.GetPageSize())
	q := &biz.IntelSourceListQuery{
		Page: page, PageSize: pageSize,
		Platform: req.GetPlatform(), Kind: req.GetKind(),
	}
	if req.GetEnabledOnly() {
		one := true
		q.Enabled = &one
	}
	rows, total, err := s.sourceUC.List(ctx, q)
	if err != nil {
		return nil, mapIcError(err)
	}
	out := &icV1.ListIntelSourcesResponse{Total: int32(total)}
	for _, r := range rows {
		out.Items = append(out.Items, sourceToProto(r))
	}
	return out, nil
}

func (s *IcSourceService) GetIntelSource(ctx context.Context, req *icV1.GetIntelSourceRequest) (*icV1.IntelSource, error) {
	row, err := s.sourceUC.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapIcError(err)
	}
	return sourceToProto(row), nil
}

func (s *IcSourceService) CreateIntelSource(ctx context.Context, req *icV1.CreateIntelSourceRequest) (*icV1.IntelSource, error) {
	in, err := sourceFromProto(req.GetData())
	if err != nil {
		return nil, err
	}
	row, err := s.sourceUC.Create(ctx, in)
	if err != nil {
		s.log.Warnf("reject source create (%s/%s×%s): %v", in.Platform, in.Kind, in.Method, err)
		return nil, mapIcError(err)
	}
	return sourceToProto(row), nil
}

func (s *IcSourceService) UpdateIntelSource(ctx context.Context, req *icV1.UpdateIntelSourceRequest) (*icV1.IntelSource, error) {
	d := req.GetData()
	if d == nil {
		return nil, icV1.ErrorIcConfigInvalid("data 必填")
	}
	// PATCH 语义：仅可写字段（kind×method 建后不可变——改采集方式 = 新信源，防历史断链）
	patch := &biz.IntelSourcePatch{}
	if d.GetConfigJson() != "" {
		cfg, err := parseSourceConfig(d.GetConfigJson())
		if err != nil {
			return nil, err
		}
		patch.Config = &cfg
	}
	if v := d.GetEnabled(); v {
		patch.Enabled = &v
	} else {
		off := false
		patch.Enabled = &off
	}
	if v := d.GetIntervalMinutes(); v > 0 {
		patch.IntervalMinutes = &v
	}
	if v := d.GetRateLimitPerMin(); v > 0 {
		patch.RateLimitPerMin = &v
	}
	if v := d.GetPriority(); v != 0 {
		patch.Priority = &v
	}
	if v := d.GetCredentialRef(); v != "" {
		patch.CredentialRef = &v
	}
	if v := d.GetWeight(); v > 0 {
		patch.Weight = &v
	}
	if d.GetRobotsExempt() {
		one, reason := true, d.GetRobotsExemptReason()
		patch.RobotsExempt, patch.RobotsExemptReason = &one, &reason
	}
	row, err := s.sourceUC.Update(ctx, req.GetId(), patch)
	if err != nil {
		return nil, mapIcError(err)
	}
	return sourceToProto(row), nil
}

func (s *IcSourceService) DeleteIntelSource(ctx context.Context, req *icV1.DeleteIntelSourceRequest) (*icV1.DeleteIntelSourceResponse, error) {
	ok, err := s.sourceUC.Delete(ctx, req.GetId())
	if err != nil {
		return nil, mapIcError(err)
	}
	return &icV1.DeleteIntelSourceResponse{Ok: ok}, nil
}

func (s *IcSourceService) ToggleIntelSource(ctx context.Context, req *icV1.ToggleIntelSourceRequest) (*icV1.ToggleIntelSourceResponse, error) {
	if err := s.sourceUC.Toggle(ctx, req.GetId(), req.GetEnabled()); err != nil {
		return nil, mapIcError(err)
	}
	return &icV1.ToggleIntelSourceResponse{Ok: true}, nil
}

// RunCollector 手动触发一轮采集（B-13；force=忽略限流/熔断）。
// 非 force 被熔断/限流跳过 → 429 可读提示（区别于「跑了但 0 新增」）。
func (s *IcSourceService) RunCollector(ctx context.Context, req *icV1.RunCollectorRequest) (*icV1.RunCollectorResponse, error) {
	runID, fin, err := s.collectUC.RunWithStats(ctx, req.GetSourceId(), "manual", req.GetForce())
	if err != nil {
		return nil, mapIcError(err)
	}
	if runID == "" && fin == nil {
		return nil, icV1.ErrorIcRateLimited("信源被熔断或限流跳过；人工补采请带 force=true")
	}
	return &icV1.RunCollectorResponse{
		Collected: fin.Fetched, Normalized: fin.Normalized, RunId: runID,
	}, nil
}

// ---------- 合规规则 CRUD（闸门判据；词表校验在本层，proto 头部约定） ----------

func (s *IcSourceService) ListComplianceRules(ctx context.Context, req *icV1.ListComplianceRulesRequest) (*icV1.ListComplianceRulesResponse, error) {
	page, pageSize := icPageClamp(req.GetPage(), req.GetPageSize())
	q := &biz.ComplianceRuleListQuery{Page: page, PageSize: pageSize, RuleType: req.GetRuleType()}
	if req.GetEnabledOnly() {
		one := true
		q.Enabled = &one
	}
	rows, total, err := s.rules.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := &icV1.ListComplianceRulesResponse{Total: int32(total)}
	for _, r := range rows {
		out.Items = append(out.Items, ruleToProto(r))
	}
	return out, nil
}

func (s *IcSourceService) CreateComplianceRule(ctx context.Context, req *icV1.CreateComplianceRuleRequest) (*icV1.ComplianceRule, error) {
	row, err := ruleFromProto(req.GetData())
	if err != nil {
		return nil, err
	}
	if uid, uerr := currentUserID(ctx); uerr == nil {
		row.CreateBy, row.UpdateBy = uid, uid
	}
	created, err := s.rules.Create(ctx, row)
	if err != nil {
		return nil, err
	}
	s.log.Infof("[audit] compliance rule created: id=%s type=%s action=%s", created.ID, created.RuleType, created.Action)
	return ruleToProto(created), nil
}

func (s *IcSourceService) UpdateComplianceRule(ctx context.Context, req *icV1.UpdateComplianceRuleRequest) (*icV1.ComplianceRule, error) {
	row, err := ruleFromProto(req.GetData())
	if err != nil {
		return nil, err
	}
	row.ID = req.GetId()
	if uid, uerr := currentUserID(ctx); uerr == nil {
		row.UpdateBy = uid
	}
	updated, err := s.rules.Update(ctx, row)
	if err != nil {
		return nil, err
	}
	s.log.Infof("[audit] compliance rule updated: id=%s type=%s action=%s", updated.ID, updated.RuleType, updated.Action)
	return ruleToProto(updated), nil
}

func (s *IcSourceService) DeleteComplianceRule(ctx context.Context, req *icV1.DeleteComplianceRuleRequest) (*icV1.DeleteComplianceRuleResponse, error) {
	ok, err := s.rules.Delete(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	s.log.Infof("[audit] compliance rule deleted: id=%s ok=%v", req.GetId(), ok)
	return &icV1.DeleteComplianceRuleResponse{Ok: ok}, nil
}

// ---------- 映射 ----------

// sourceToProto biz → proto（config map → JSON 原文；健康度由 7d 计数推导）。
func sourceToProto(r *biz.IntelSourceRow) *icV1.IntelSource {
	out := &icV1.IntelSource{
		Id: r.ID, Platform: r.Platform, Region: r.Region,
		Kind: r.Kind, Method: r.Method,
		ConfigSchemaHint: r.ConfigSchemaHint,
		Enabled:          r.Enabled, IntervalMinutes: r.IntervalMinutes,
		RateLimitPerMin: r.RateLimitPerMin, Priority: r.Priority,
		CredentialRef: r.CredentialRef, Weight: r.Weight,
		RobotsExempt: r.RobotsExempt, RobotsExemptReason: r.RobotsExemptReason,
		Status: r.Status, FailStreak: r.FailStreak,
		PausedUntil: icPBTime(r.PausedUntil),
		LastRunAt:   icPBTime(r.LastRunAt),
		CreateTime:  icPBTime(r.CreateTime), UpdateTime: icPBTime(r.UpdateTime),
		Health: icHealthView(r),
	}
	if len(r.Config) > 0 {
		if b, err := json.Marshal(r.Config); err == nil {
			out.ConfigJson = string(b)
		}
	}
	return out
}

// icHealthView NFR-IC-05 健康度推导：disabled → down；7d 无数据 → healthy（新源不误报）；
// 成功率 0 → down；<90% → degraded。
func icHealthView(r *biz.IntelSourceRow) *icV1.SourceHealth {
	total := r.SuccessCount7d + r.FailCount7d
	rate, status := 0.0, "healthy"
	switch {
	case r.Status == "disabled":
		status = "down"
	case total == 0:
		// 新源无样本，不罚站
	default:
		rate = float64(r.SuccessCount7d) / float64(total)
		switch {
		case rate == 0:
			status = "down"
		case rate < 0.9:
			status = "degraded"
		}
	}
	return &icV1.SourceHealth{
		Status: status, SuccessRate_7D: rate,
		LastSuccessAt: icPBTime(r.LastSuccessAt), LastError: r.LastError,
	}
}

// sourceFromProto proto → biz 创建入参（config_json 必须合法 JSON——proto 注释契约）。
func sourceFromProto(d *icV1.IntelSource) (*biz.IntelSourceCreate, error) {
	if d == nil {
		return nil, icV1.ErrorIcConfigInvalid("data 必填")
	}
	cfg, err := parseSourceConfig(d.GetConfigJson())
	if err != nil {
		return nil, err
	}
	return &biz.IntelSourceCreate{
		Platform: d.GetPlatform(), Region: d.GetRegion(),
		Kind: d.GetKind(), Method: d.GetMethod(),
		Config: cfg, ConfigSchemaHint: d.GetConfigSchemaHint(),
		Enabled: d.GetEnabled(), IntervalMinutes: d.GetIntervalMinutes(),
		RateLimitPerMin: d.GetRateLimitPerMin(), Priority: d.GetPriority(),
		CredentialRef: d.GetCredentialRef(), Weight: d.GetWeight(),
		RobotsExempt: d.GetRobotsExempt(), RobotsExemptReason: d.GetRobotsExemptReason(),
	}, nil
}

// parseSourceConfig config_json 原文 → map（空串 = 无配置；非法 JSON → 400）。
func parseSourceConfig(s string) (map[string]interface{}, error) {
	if s == "" {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, icV1.ErrorIcConfigInvalid("config_json 非法 JSON: %s", err.Error())
	}
	return m, nil
}

// 合规规则词表（proto 注释契约 + 闸门实现两侧一致的单一事实源）。
var (
	icRuleTypes   = map[string]bool{"sensitive_category": true, "rights_holder_blacklist": true, "authorization": true, "robots": true, "platform_tos": true}
	icRuleActions = map[string]bool{"pass": true, "review": true, "block": true}
	icRuleScopes  = map[string]bool{"global": true, "platform": true}
)

// ruleFromProto 词表校验 + 映射（scope=platform 时 platform 必填）。
func ruleFromProto(d *icV1.ComplianceRule) (*biz.ComplianceRuleRow, error) {
	if d == nil {
		return nil, icV1.ErrorIcConfigInvalid("data 必填")
	}
	if !icRuleTypes[d.GetRuleType()] {
		return nil, icV1.ErrorIcConfigInvalid("rule_type 非法取值: %s", d.GetRuleType())
	}
	if !icRuleActions[d.GetAction()] {
		return nil, icV1.ErrorIcConfigInvalid("action 非法取值: %s", d.GetAction())
	}
	scope := d.GetScope()
	if scope == "" {
		scope = "global"
	}
	if !icRuleScopes[scope] {
		return nil, icV1.ErrorIcConfigInvalid("scope 非法取值: %s", scope)
	}
	if scope == "platform" && d.GetPlatform() == "" {
		return nil, icV1.ErrorIcConfigInvalid("scope=platform 时 platform 必填")
	}
	if d.GetPattern() == "" {
		return nil, icV1.ErrorIcConfigInvalid("pattern 必填")
	}
	row := &biz.ComplianceRuleRow{
		RuleType: d.GetRuleType(), Pattern: d.GetPattern(), Action: d.GetAction(),
		Priority: d.GetPriority(), Enabled: d.GetEnabled(),
		Scope: scope, Platform: d.GetPlatform(), Remark: d.GetRemark(),
	}
	if t := d.GetEffectiveFrom(); t != nil {
		v := t.AsTime().UTC()
		row.EffectiveFrom = &v
	}
	if t := d.GetEffectiveUntil(); t != nil {
		v := t.AsTime().UTC()
		row.EffectiveUntil = &v
	}
	return row, nil
}

func ruleToProto(r *biz.ComplianceRuleRow) *icV1.ComplianceRule {
	return &icV1.ComplianceRule{
		Id: r.ID, RuleType: r.RuleType, Pattern: r.Pattern, Action: r.Action,
		Priority: r.Priority, Enabled: r.Enabled,
		EffectiveFrom: icPBTime(r.EffectiveFrom), EffectiveUntil: icPBTime(r.EffectiveUntil),
		HitCount: r.HitCount, Scope: r.Scope, Platform: r.Platform, Remark: r.Remark,
	}
}

// ---------- 跨服务共享小件（ic_intel/collector 两服务同用） ----------

// icPBTime *time.Time → timestamppb（nil-guard；统一 UTC）。
func icPBTime(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(t.UTC())
}

// icPageClamp 分页护栏（V1 上限 200，防全表拉取）。
func icPageClamp(page, pageSize int32) (int32, int32) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

// mapIcError biz 哨兵 → ic_error.proto 错误码（未知的原样穿透，middleware 兜底 500）。
func mapIcError(err error) error {
	switch {
	case errors.Is(err, biz.ErrIcSourceNotFound):
		return icV1.ErrorIcSrcNotFound("%s", err.Error())
	case errors.Is(err, biz.ErrIcKindMethod),
		errors.Is(err, biz.ErrIcIntervalFloor),
		errors.Is(err, biz.ErrIcRobotsExemptWhy),
		errors.Is(err, biz.ErrIcSidecarNeedsAuth),
		errors.Is(err, biz.ErrIcFetchMethod),
		errors.Is(err, biz.ErrIcPlatformUnknown):
		return icV1.ErrorIcConfigInvalid("%s", err.Error())
	case errors.Is(err, biz.ErrIcCollectorUnavailable):
		return icV1.ErrorIcUpstreamUnavailable("%s", err.Error())
	case errors.Is(err, biz.ErrRobotsDenied):
		return icV1.ErrorIcRobotsDisallowed("%s", err.Error())
	case errors.Is(err, biz.ErrURLBlocked):
		return icV1.ErrorIcUrlBlocked("%s", err.Error())
	case errors.Is(err, biz.ErrIcFetchRateLimited):
		return icV1.ErrorIcRateLimited("%s", err.Error())
	case errors.Is(err, biz.ErrIcFetchEmpty):
		return icV1.ErrorIcEmptyContent("%s", err.Error())
	default:
		return err
	}
}
