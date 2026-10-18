package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
	"framework/pkg/mediacontract"
)

// ApprovalGateUsecase 审批门用例：Submit / Approve / Reject / Cancel 状态机 + 审计流水（FR-PUB-04）。
//
// 🔴 纪律：
//   - 纯判定逻辑在 algo/publisher（CanApprove/NextApprovalState），本用例负责读写与留痕。
//   - 越权尝试也必须写审计（result=deny），宁可多记不可漏记。
//   - 状态更新成功而审计写入失败时：状态不回滚（无分布式事务），Errorf 告警提示人工补查。
type ApprovalGateUsecase struct {
	conf     *Conf
	dispatch DispatchRepo
	records  ApprovalRepo
	clock    func() time.Time
	log      *log.Helper
}

// NewApprovalGateUsecase 构造审批门用例。
func NewApprovalGateUsecase(conf *Conf, dispatch DispatchRepo, records ApprovalRepo, logger log.Logger) *ApprovalGateUsecase {
	return &ApprovalGateUsecase{
		conf:     conf,
		dispatch: dispatch,
		records:  records,
		clock:    time.Now,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish-gate")),
	}
}

// Submit 重新提交（rejected → draft）。已在 draft 的幂等直返；approved/published 拒绝。
// 提交人不改写（submitted_by 保持原作者，禁自批判据不可被偷换）。
func (uc *ApprovalGateUsecase) Submit(ctx context.Context, unitID string, actor Actor, meta AuditMeta) (*UnitRow, error) {
	unit, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	switch unit.ApprovalState {
	case string(mediacontract.ApprovalDraft):
		return unit, nil // 幂等
	case string(mediacontract.ApprovalRejected):
		// fallthrough 到转移
	default:
		err := fmt.Errorf("%w: 当前状态 %s 不可提交", algopublisher.ErrInvalidState, unit.ApprovalState)
		uc.auditDeny(ctx, unit, AuditActionSubmit, actor, "", err, meta)
		return nil, err
	}

	to := string(mediacontract.ApprovalDraft)
	patch := UnitPatch{
		ApprovalState:     &to,
		ClearApprover:     true,
		ClearRejectReason: true,
	}
	updated, err := uc.dispatch.UpdateStates(ctx, unitID, patch)
	if err != nil {
		return nil, err
	}
	uc.record(ctx, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionSubmit,
		Actor:              actor.ID,
		ActorType:          string(actor.Type),
		FromState:          strPtr(unit.ApprovalState),
		ToState:            strPtr(to),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "allow",
	})
	return updated, nil
}

// Approve 审批通过（🔴 禁自批 / 禁自动化审批；重复审批报状态错误）。
func (uc *ApprovalGateUsecase) Approve(ctx context.Context, unitID string, approver Actor, meta AuditMeta) (*UnitRow, error) {
	unit, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	cu := ToContractUnit(unit)
	if err := algopublisher.CanApprove(cu, approver); err != nil {
		uc.auditDeny(ctx, unit, AuditActionApprove, approver, "", err, meta)
		return nil, err
	}
	if _, err := algopublisher.NextApprovalState(mediacontract.ApprovalState(unit.ApprovalState), AuditActionApprove); err != nil {
		uc.auditDeny(ctx, unit, AuditActionApprove, approver, "", err, meta)
		return nil, err
	}

	now := uc.clock()
	to := string(mediacontract.ApprovalApproved)
	patch := UnitPatch{
		ApprovalState:     &to,
		Approver:          strPtr(approver.ID),
		ApprovedAt:        &now,
		ClearRejectReason: true,
	}
	updated, err := uc.dispatch.UpdateStates(ctx, unitID, patch)
	if err != nil {
		return nil, err
	}
	uc.record(ctx, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionApprove,
		Actor:              approver.ID,
		ActorType:          string(approver.Type),
		FromState:          strPtr(unit.ApprovalState),
		ToState:            strPtr(to),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "allow",
	})
	return updated, nil
}

// Reject 驳回（reason 强制非空；🔴 自动化身份不得执行审批侧动作）。
// 允许提交者本人驳回自己的草稿（等价撤回语义，不构成自批——自批只针对「通过」）。
func (uc *ApprovalGateUsecase) Reject(ctx context.Context, unitID, reason string, actor Actor, meta AuditMeta) (*UnitRow, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: 驳回必须填写原因", ErrRejectReasonRequired)
	}
	unit, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	if actor.Type == mediacontract.ActorAutomation {
		err := fmt.Errorf("%w: 自动化身份不得执行驳回", algopublisher.ErrAutomationCannotApprove)
		uc.auditDeny(ctx, unit, AuditActionReject, actor, reason, err, meta)
		return nil, err
	}
	next, err := algopublisher.NextApprovalState(mediacontract.ApprovalState(unit.ApprovalState), AuditActionReject)
	if err != nil {
		err = fmt.Errorf("%w: 当前状态 %s 不可驳回", algopublisher.ErrInvalidState, unit.ApprovalState)
		uc.auditDeny(ctx, unit, AuditActionReject, actor, reason, err, meta)
		return nil, err
	}

	to := string(next)
	patch := UnitPatch{
		ApprovalState: &to,
		RejectReason:  strPtr(reason),
		ClearApprover: true,
	}
	updated, err := uc.dispatch.UpdateStates(ctx, unitID, patch)
	if err != nil {
		return nil, err
	}
	uc.record(ctx, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionReject,
		Actor:              actor.ID,
		ActorType:          string(actor.Type),
		FromState:          strPtr(unit.ApprovalState),
		ToState:            strPtr(to),
		Reason:             strPtr(reason),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "allow",
	})
	return updated, nil
}

// Cancel 撤回（FR-PUB-04 流转终止）：
//   - 已发布 / 发布中不可撤回（回执即事实，不可逆）。
//   - approved → draft（撤回已批内容，防止其被发布；审批者信息清空）。
//   - draft / rejected → 无状态变化，仅留痕（取消意图进审计）。
func (uc *ApprovalGateUsecase) Cancel(ctx context.Context, unitID, reason string, actor Actor, meta AuditMeta) (*UnitRow, error) {
	unit, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	if unit.PublishState == "publishing" || unit.PublishState == "published" {
		err := fmt.Errorf("%w: 发布状态 %s 不可撤回（回执即事实）", algopublisher.ErrInvalidState, unit.PublishState)
		uc.auditDeny(ctx, unit, AuditActionCancel, actor, reason, err, meta)
		return nil, err
	}

	from := unit.ApprovalState
	to := from
	if from == string(mediacontract.ApprovalApproved) {
		to = string(mediacontract.ApprovalDraft)
		patch := UnitPatch{
			ApprovalState: &to,
			ClearApprover: true,
		}
		if _, err := uc.dispatch.UpdateStates(ctx, unitID, patch); err != nil {
			return nil, err
		}
	}
	updated, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	uc.record(ctx, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionCancel,
		Actor:              actor.ID,
		ActorType:          string(actor.Type),
		FromState:          strPtr(from),
		ToState:            strPtr(to),
		Reason:             strPtrOrNil(strings.TrimSpace(reason)),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "allow",
	})
	return updated, nil
}

// CheckPublish 发布前置校验（详细设计 §3 五条校验；熔断/配额结论由调用方收集后注入）。
// 本方法在 auditDeny 上不做包装——发布用例自行组织审计（含具体拒绝语义）。
func CheckPublish(unit *UnitRow, operator Actor, accountPaused, quotaExceeded bool) error {
	return algopublisher.CanPublish(algopublisher.PublishCheck{
		Unit:          ToContractUnit(unit),
		Operator:      operator,
		AccountPaused: accountPaused,
		QuotaExceeded: quotaExceeded,
	})
}

// record 写审计流水；失败仅告警（不抢主流程语义，但 Errorf 提示人工补查）。
func (uc *ApprovalGateUsecase) record(ctx context.Context, e *RecordEntry) {
	recordAudit(ctx, uc.records, e, uc.log)
}

// auditDeny 越权/违规尝试留痕（result=deny）。
func (uc *ApprovalGateUsecase) auditDeny(ctx context.Context, unit *UnitRow, action string, actor Actor, reason string, cause error, meta AuditMeta) {
	uc.log.Warnf("[map-gate] deny action=%s unit=%s actor=%s(%s) cause=%v", action, unit.UnitID, actor.ID, actor.Type, cause)
	uc.record(ctx, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             action,
		Actor:              actor.ID,
		ActorType:          string(actor.Type),
		FromState:          strPtr(unit.ApprovalState),
		Reason:             strPtrOrNil(reason),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "deny",
	})
}

// recordAudit 是 gate / publish 两个用例共享的审计写入（含失败告警）。
func recordAudit(ctx context.Context, repo ApprovalRepo, e *RecordEntry, logger *log.Helper) {
	if err := repo.Record(ctx, e); err != nil {
		// 🔴 审计丢失是严重事件：状态已变更而流水缺失，必须报警人工补查。
		logger.Errorf("[map-audit] 审计流水写入失败 unit=%s action=%s actor=%s err=%v（请人工补查）",
			e.DispatchUnitID, e.Action, e.Actor, err)
	}
}

// IsGateError 判定错误是否来自审批门算法（service 层映射错误码用）。
func IsGateError(err error) bool {
	return errors.Is(err, algopublisher.ErrNotApproved) ||
		errors.Is(err, algopublisher.ErrSelfApproval) ||
		errors.Is(err, algopublisher.ErrAutomationCannotPublish) ||
		errors.Is(err, algopublisher.ErrAutomationCannotApprove) ||
		errors.Is(err, algopublisher.ErrVisibilityNoReason) ||
		errors.Is(err, algopublisher.ErrInvalidState) ||
		errors.Is(err, algopublisher.ErrAccountPaused) ||
		errors.Is(err, algopublisher.ErrQuotaExceeded)
}

func strPtr(s string) *string { return &s }

func strPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
