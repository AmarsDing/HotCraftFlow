// Package publisher 实现 ⑤ MAP 媒体自动化发布矩阵的核心算法。
// 对应 docs/systems/05-MAP/03-详细设计.md §3（审批门）与 §6（熔断）。
// 包名用 publisher 而非 map（Go 关键字）。
package publisher

import (
	"errors"

	"framework/pkg/mediacontract"
)

// 审批门错误码（对应 MAP_* 错误码）。
var (
	ErrNotApproved             = errors.New("MAP_NOT_APPROVED")
	ErrSelfApproval            = errors.New("MAP_SELF_APPROVAL")
	ErrAutomationCannotPublish = errors.New("MAP_AUTOMATION_DENIED")
	ErrAutomationCannotApprove = errors.New("MAP_AUTOMATION_CANNOT_APPROVE")
	ErrVisibilityNoReason      = errors.New("MAP_VISIBILITY_NO_REASON")
	ErrInvalidState            = errors.New("MAP_INVALID_STATE")
	ErrAccountPaused           = errors.New("MAP_ACCOUNT_PAUSED")
	ErrQuotaExceeded           = errors.New("MAP_QUOTA_EXCEEDED")
)

// Actor 操作者。
type Actor struct {
	ID   string
	Type mediacontract.ActorType
}

// 审批门三条铁律（详细设计 §3.1），应用层校验 + 数据库约束双保险。
// 本文件实现应用层校验；数据库约束由迁移保证：
//   publish_state='published' 要求 approval_state='approved'。

// CanApprove 校验 Approve 操作（详细设计 §3.2 禁自批）。
// 规则：审批人 ≠ 提交者；自动化身份不得审批；当前状态必须 draft。
func CanApprove(unit *mediacontract.DispatchUnit, approver Actor) error {
	if approver.Type == mediacontract.ActorAutomation {
		return ErrAutomationCannotApprove
	}
	if approver.ID == unit.SubmittedBy {
		return ErrSelfApproval
	}
	if unit.Approval.State != mediacontract.ApprovalDraft {
		return ErrInvalidState
	}
	return nil
}

// PublishCheck 是发布前的全部校验输入。
type PublishCheck struct {
	Unit            *mediacontract.DispatchUnit
	Operator        Actor
	AccountPaused   bool // 熔断器结论（调用方查）
	QuotaExceeded   bool // 配额结论（调用方查）
}

// CanPublish 校验 Publish 操作（详细设计 §3 五条校验，任一失败即拒绝）。
// 这是**唯一**发布入口的前置校验。
func CanPublish(in PublishCheck) error {
	u := in.Unit
	// 校验 1：状态必须 approved。
	if u.Approval.State != mediacontract.ApprovalApproved {
		return ErrNotApproved
	}
	// 校验 2：禁自批 + 自动化不得发布。
	if u.Approval.Approver == nil || *u.Approval.Approver == "" || *u.Approval.Approver == u.SubmittedBy {
		return ErrSelfApproval
	}
	if in.Operator.Type == mediacontract.ActorAutomation {
		return ErrAutomationCannotPublish
	}
	// 校验 3：可见性——非 private 必须有 override_reason。
	if u.Visibility.Default != mediacontract.VisibilityPrivate && u.Visibility.OverrideReason == nil {
		return ErrVisibilityNoReason
	}
	// 校验 4：熔断。
	if in.AccountPaused {
		return ErrAccountPaused
	}
	// 校验 5：总量上限。
	if in.QuotaExceeded {
		return ErrQuotaExceeded
	}
	return nil
}

// NextApprovalState 审批状态机转移。
// 合法转移：draft → approved / rejected；approved → published。
func NextApprovalState(from mediacontract.ApprovalState, action string) (mediacontract.ApprovalState, error) {
	switch from {
	case mediacontract.ApprovalDraft:
		switch action {
		case "approve":
			return mediacontract.ApprovalApproved, nil
		case "reject":
			return mediacontract.ApprovalRejected, nil
		}
	case mediacontract.ApprovalApproved:
		if action == "publish" {
			return mediacontract.ApprovalPublished, nil
		}
	}
	return "", ErrInvalidState
}
