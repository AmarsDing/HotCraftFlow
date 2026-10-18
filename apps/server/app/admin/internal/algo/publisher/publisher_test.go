package publisher

import (
	"testing"
	"time"

	"framework/pkg/mediacontract"
)

func approvedUnit(submitter, approver string) *mediacontract.DispatchUnit {
	return &mediacontract.DispatchUnit{
		UnitID:      mediacontract.NewDispatchUnitID(),
		SubmittedBy: submitter,
		Approval:    mediacontract.Approval{State: mediacontract.ApprovalApproved, Approver: &approver},
		Visibility:  mediacontract.VisibilitySpec{Default: mediacontract.VisibilityPrivate},
	}
}

func TestGateThreeIronRules(t *testing.T) {
	human := Actor{ID: "u2", Type: mediacontract.ActorHuman}

	// 铁律 1：无 approve 不得 publish。
	draft := approvedUnit("u1", "u2")
	draft.Approval.State = mediacontract.ApprovalDraft
	if err := CanPublish(PublishCheck{Unit: draft, Operator: human}); err != ErrNotApproved {
		t.Fatalf("draft 状态发布应报 MAP_NOT_APPROVED，got %v", err)
	}

	// 铁律 2：禁自批。
	selfApproved := approvedUnit("u1", "u1")
	if err := CanPublish(PublishCheck{Unit: selfApproved, Operator: human}); err != ErrSelfApproval {
		t.Fatalf("自批发布应报 MAP_SELF_APPROVAL，got %v", err)
	}

	// 自动化不得发布。
	auto := Actor{ID: "bot", Type: mediacontract.ActorAutomation}
	if err := CanPublish(PublishCheck{Unit: approvedUnit("u1", "u2"), Operator: auto}); err != ErrAutomationCannotPublish {
		t.Fatalf("自动化发布应报 MAP_AUTOMATION_DENIED，got %v", err)
	}

	// 合法路径：approved + 他人审批 + human 操作 + private。
	if err := CanPublish(PublishCheck{Unit: approvedUnit("u1", "u2"), Operator: human}); err != nil {
		t.Fatalf("合法发布应通过，got %v", err)
	}
}

func TestGateVisibilityReason(t *testing.T) {
	human := Actor{ID: "u2", Type: mediacontract.ActorHuman}
	u := approvedUnit("u1", "u2")
	u.Visibility.Default = mediacontract.VisibilityPublic // 非 private 且无理由
	if err := CanPublish(PublishCheck{Unit: u, Operator: human}); err != ErrVisibilityNoReason {
		t.Fatalf("公开无理由应报 MAP_VISIBILITY_NO_REASON，got %v", err)
	}
	reason := "运营确认公开发布"
	u.Visibility.OverrideReason = &reason
	if err := CanPublish(PublishCheck{Unit: u, Operator: human}); err != nil {
		t.Fatalf("公开有理由应通过，got %v", err)
	}
}

func TestApproveSelfForbidden(t *testing.T) {
	draft := &mediacontract.DispatchUnit{
		SubmittedBy: "u1",
		Approval:    mediacontract.Approval{State: mediacontract.ApprovalDraft},
	}
	if err := CanApprove(draft, Actor{ID: "u1", Type: mediacontract.ActorHuman}); err != ErrSelfApproval {
		t.Fatalf("自己审批自己应报 MAP_SELF_APPROVAL，got %v", err)
	}
	if err := CanApprove(draft, Actor{ID: "u2", Type: mediacontract.ActorHuman}); err != nil {
		t.Fatalf("他人审批应通过，got %v", err)
	}
}

func TestApprovalStateMachine(t *testing.T) {
	// 合法转移。
	if s, err := NextApprovalState(mediacontract.ApprovalDraft, "approve"); err != nil || s != mediacontract.ApprovalApproved {
		t.Fatalf("draft→approved 应合法，got %s %v", s, err)
	}
	if s, err := NextApprovalState(mediacontract.ApprovalApproved, "publish"); err != nil || s != mediacontract.ApprovalPublished {
		t.Fatalf("approved→published 应合法，got %s %v", s, err)
	}
	// 非法转移：draft 直接 publish。
	if _, err := NextApprovalState(mediacontract.ApprovalDraft, "publish"); err != ErrInvalidState {
		t.Fatalf("draft 直接 publish 应拒绝，got %v", err)
	}
}

func TestCircuitBreaker(t *testing.T) {
	cfg := DefaultBreakerConfig()
	now := time.Now()
	// 3 次连续失败 → 暂停。
	d := EvaluateAccountFailure(3, 0.1, cfg, now)
	if !d.PauseAccount || d.Health != HealthPaused {
		t.Fatalf("连续 3 次失败应暂停账号，got %+v", d)
	}
	if d.PausedUntil.Before(now.Add(29 * time.Minute)) {
		t.Fatalf("暂停时长应约 30min")
	}
	// 平台级失败率超阈 → 暂停平台。
	d2 := EvaluateAccountFailure(1, 0.6, cfg, now)
	if !d2.PausePlatform {
		t.Fatalf("平台失败率 0.6 应暂停平台")
	}
}

func TestRateGovernance(t *testing.T) {
	cfg := DefaultRateConfig()
	// 账号间隔不足。
	if ok, why := CanPublishNow(60, 0, 0, cfg); ok || why != "account_min_interval" {
		t.Fatalf("账号间隔不足应阻断，got %v %s", ok, why)
	}
	// 全局 24h 超限。
	if ok, why := CanPublishNow(600, 0, 100, cfg); ok || why != "global_daily_cap" {
		t.Fatalf("全局超限应硬阻断，got %v %s", ok, why)
	}
	// 合法。
	if ok, _ := CanPublishNow(600, 5, 50, cfg); !ok {
		t.Fatalf("合法应放行")
	}
}
