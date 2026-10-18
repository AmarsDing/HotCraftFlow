package publish

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
	"framework/pkg/mediacontract"
)

// draftUnit 构造草稿态单元（submitted_by=alice，默认 private）。
func draftUnit(unitID, submittedBy string) *UnitRow {
	return &UnitRow{
		UnitID:            unitID,
		Targets:           []string{"youtube:main"},
		ApprovalState:     string(mediacontract.ApprovalDraft),
		SubmittedBy:       submittedBy,
		PublishState:      "pending",
		VisibilityDefault: string(mediacontract.VisibilityPrivate),
		ScheduleMode:      "none",
	}
}

func newGate(conf *Conf, dispatch DispatchRepo, records ApprovalRepo) *ApprovalGateUsecase {
	return NewApprovalGateUsecase(conf, dispatch, records, log.DefaultLogger)
}

// 🔴 验收 §8-1：未审批 → 发布必败（MAP_NOT_APPROVED）且留 deny 审计。
func TestGate_PublishWithoutApprove_Denied(t *testing.T) {
	unit := draftUnit("du_t1", "alice")

	// 纯算法层（唯一发布前置校验）：
	cu := ToContractUnit(unit)
	err := algopublisher.CanPublish(algopublisher.PublishCheck{Unit: cu, Operator: NewHumanActor("bob")})
	if !errors.Is(err, algopublisher.ErrNotApproved) {
		t.Fatalf("期望 MAP_NOT_APPROVED，实际 %v", err)
	}

	// biz 包装层同语义：
	err = CheckPublish(unit, NewHumanActor("bob"), false, false)
	if !errors.Is(err, algopublisher.ErrNotApproved) {
		t.Fatalf("CheckPublish 期望 MAP_NOT_APPROVED，实际 %v", err)
	}
}

// 🔴 验收 §8-2：提交者自己审批 → 必败（MAP_SELF_APPROVAL）+ deny 留痕。
func TestGate_SelfApproval_Denied(t *testing.T) {
	unit := draftUnit("du_t2", "alice")
	dispatch := newFakeDispatch(unit)
	records := &fakeRecords{}
	gate := newGate(DefaultConfPtr(), dispatch, records)

	_, err := gate.Approve(context.Background(), unit.UnitID, NewHumanActor("alice"), AuditMeta{IP: "1.2.3.4"})
	if !errors.Is(err, algopublisher.ErrSelfApproval) {
		t.Fatalf("期望 MAP_SELF_APPROVAL，实际 %v", err)
	}
	// 状态未被偷改：
	got, _ := dispatch.GetUnit(context.Background(), unit.UnitID)
	if got.ApprovalState != string(mediacontract.ApprovalDraft) || got.Approver != nil {
		t.Fatalf("自批不该改状态: %+v", got)
	}
	if records.countByResult("deny") != 1 {
		t.Fatalf("期望 1 条 deny 审计，实际 %d", records.countByResult("deny"))
	}
}

// 🔴 验收 §8-3：自动化身份不得审批/发布（MAP_AUTOMATION_*）。
func TestGate_Automation_Denied(t *testing.T) {
	unit := draftUnit("du_t3", "alice")
	dispatch := newFakeDispatch(unit)
	records := &fakeRecords{}
	gate := newGate(DefaultConfPtr(), dispatch, records)

	_, err := gate.Approve(context.Background(), unit.UnitID, NewAutomationActor("bot-1"), AuditMeta{})
	if !errors.Is(err, algopublisher.ErrAutomationCannotApprove) {
		t.Fatalf("自动化审批期望 MAP_AUTOMATION_CANNOT_APPROVE，实际 %v", err)
	}

	// 自动化驳回同样禁止：
	_, err = gate.Reject(context.Background(), unit.UnitID, "bad", NewAutomationActor("bot-1"), AuditMeta{})
	if !errors.Is(err, algopublisher.ErrAutomationCannotApprove) {
		t.Fatalf("自动化驳回期望 MAP_AUTOMATION_CANNOT_APPROVE，实际 %v", err)
	}

	// 已批单元：自动化点发布必败。
	approved := draftUnit("du_t4", "alice")
	approved.ApprovalState = string(mediacontract.ApprovalApproved)
	approved.Approver = strPtr("bob")
	now := time.Now()
	approved.ApprovedAt = &now
	err = CheckPublish(approved, NewAutomationActor("bot-1"), false, false)
	if !errors.Is(err, algopublisher.ErrAutomationCannotPublish) {
		t.Fatalf("自动化发布期望 MAP_AUTOMATION_DENIED，实际 %v", err)
	}
}

// 🔴 验收 §8-4：非 private 且无 override_reason → 发布防线的最后一道必败。
func TestGate_PublishNonPrivateWithoutReason_Denied(t *testing.T) {
	unit := draftUnit("du_t5", "alice")
	unit.ApprovalState = string(mediacontract.ApprovalApproved)
	unit.Approver = strPtr("bob")
	unit.VisibilityDefault = string(mediacontract.VisibilityPublic)
	unit.VisibilityOverrideReason = nil // 🔴 关键：无理由

	err := CheckPublish(unit, NewHumanActor("carol"), false, false)
	if !errors.Is(err, algopublisher.ErrVisibilityNoReason) {
		t.Fatalf("期望 MAP_VISIBILITY_NO_REASON，实际 %v", err)
	}
}

// 🔴 验收 §8-5：默认私密恒成立——多目标建单全部落 private（NFR-MAP-01）。
func TestGate_DefaultVisibilityAlwaysPrivate(t *testing.T) {
	platforms := []string{"youtube", "bilibili", "x", "reddit", "tiktok"}
	var channels []*ChannelRow
	pubs := map[string]Publisher{}
	for i, p := range platforms {
		channels = append(channels, &ChannelRow{
			ChannelID:   "ch_" + p,
			PlatformKey: p,
			Slot:        "main",
			Enabled:     true,
			Capability:  capabilitySnapshot(Capability{NativeSchedule: i%2 == 0}),
		})
		pubs[p] = newSuccessPublisher(p, nil, nil)
	}
	intake := NewIntakeUsecase(DefaultConfPtr(), newFakeChannels(channels...),
		newFakeDispatch(), &fakeRecords{}, &fakeAdapters{pubs: pubs}, log.DefaultLogger)

	ctx := context.Background()
	for i := 0; i < 20; i++ {
		targets := []string{platforms[i%len(platforms)] + ":main", platforms[(i+1)%len(platforms)] + ":main"}
		unit, err := intake.Create(ctx, CreateUnitInput{Targets: targets, SubmittedBy: "alice"})
		if err != nil {
			t.Fatalf("建单失败 #%d: %v", i, err)
		}
		if unit.VisibilityDefault != string(mediacontract.VisibilityPrivate) {
			t.Fatalf("#%d 默认可见性必须恒为 private，实际 %q", i, unit.VisibilityDefault)
		}
	}
}

// 正常流转：approve → approved + approver 落库 + allow 审计；重复 approve 报状态错。
func TestGate_Approve_HappyPath(t *testing.T) {
	unit := draftUnit("du_t6", "alice")
	dispatch := newFakeDispatch(unit)
	records := &fakeRecords{}
	gate := newGate(DefaultConfPtr(), dispatch, records)

	updated, err := gate.Approve(context.Background(), unit.UnitID, NewHumanActor("bob"), AuditMeta{})
	if err != nil {
		t.Fatalf("审批通过失败: %v", err)
	}
	if updated.ApprovalState != string(mediacontract.ApprovalApproved) {
		t.Fatalf("期望 approved，实际 %s", updated.ApprovalState)
	}
	if updated.Approver == nil || *updated.Approver != "bob" {
		t.Fatalf("审批者未落库: %+v", updated.Approver)
	}
	// 重复审批 → MAP_INVALID_STATE（CanApprove 先去重）。
	if _, err := gate.Approve(context.Background(), unit.UnitID, NewHumanActor("carol"), AuditMeta{}); !errors.Is(err, algopublisher.ErrInvalidState) {
		t.Fatalf("重复审批期望 MAP_INVALID_STATE，实际 %v", err)
	}
	if records.countByResult("allow") != 1 {
		t.Fatalf("期望 1 条 allow 审计，实际 %d", records.countByResult("allow"))
	}
}

// Reject：reason 必填；已批单可驳回到 rejected；提交者可自驳（不等于自批）。
func TestGate_Reject_Rules(t *testing.T) {
	unit := draftUnit("du_t7", "alice")
	dispatch := newFakeDispatch(unit)
	records := &fakeRecords{}
	gate := newGate(DefaultConfPtr(), dispatch, records)

	if _, err := gate.Reject(context.Background(), unit.UnitID, "  ", NewHumanActor("bob"), AuditMeta{}); !errors.Is(err, ErrRejectReasonRequired) {
		t.Fatalf("空 reason 期望 MAP_REJECT_REASON_REQUIRED，实际 %v", err)
	}
	updated, err := gate.Reject(context.Background(), unit.UnitID, "口径不符", NewHumanActor("alice"), AuditMeta{})
	if err != nil {
		t.Fatalf("驳回失败: %v", err)
	}
	if updated.ApprovalState != string(mediacontract.ApprovalRejected) || updated.RejectReason == nil || *updated.RejectReason != "口径不符" {
		t.Fatalf("驳回状态/原因不正确: %+v", updated)
	}
}

// Submit：rejected → draft（清空 approver/reason）；draft 幂等；approved 拒绝。
func TestGate_Submit_Transitions(t *testing.T) {
	unit := draftUnit("du_t8", "alice")
	unit.ApprovalState = string(mediacontract.ApprovalRejected)
	unit.RejectReason = strPtr("口径不符")
	dispatch := newFakeDispatch(unit)
	records := &fakeRecords{}
	gate := newGate(DefaultConfPtr(), dispatch, records)

	updated, err := gate.Submit(context.Background(), unit.UnitID, NewHumanActor("alice"), AuditMeta{})
	if err != nil {
		t.Fatalf("重提失败: %v", err)
	}
	if updated.ApprovalState != string(mediacontract.ApprovalDraft) || updated.RejectReason != nil {
		t.Fatalf("重提后应回 draft 且清驳回原因: %+v", updated)
	}
	// draft 幂等直返。
	if _, err := gate.Submit(context.Background(), unit.UnitID, NewHumanActor("alice"), AuditMeta{}); err != nil {
		t.Fatalf("draft 重提应幂等: %v", err)
	}
	// approved 不可走 Submit。
	unit2 := draftUnit("du_t9", "alice")
	unit2.ApprovalState = string(mediacontract.ApprovalApproved)
	dispatch.CreateUnit(context.Background(), unit2)
	if _, err := gate.Submit(context.Background(), unit2.UnitID, NewHumanActor("alice"), AuditMeta{}); !errors.Is(err, algopublisher.ErrInvalidState) {
		t.Fatalf("approved 走 Submit 应报 MAP_INVALID_STATE，实际 %v", err)
	}
}

// Cancel：published/publishing 不可撤；approved → draft 清审批人。
func TestGate_Cancel_Rules(t *testing.T) {
	unit := draftUnit("du_t10", "alice")
	unit.ApprovalState = string(mediacontract.ApprovalApproved)
	unit.Approver = strPtr("bob")
	dispatch := newFakeDispatch(unit)
	records := &fakeRecords{}
	gate := newGate(DefaultConfPtr(), dispatch, records)

	updated, err := gate.Cancel(context.Background(), unit.UnitID, "方向调整", NewHumanActor("alice"), AuditMeta{})
	if err != nil {
		t.Fatalf("撤回失败: %v", err)
	}
	if updated.ApprovalState != string(mediacontract.ApprovalDraft) || updated.Approver != nil {
		t.Fatalf("撤回已批单元应回 draft 清审批人: %+v", updated)
	}

	pubbing := draftUnit("du_t11", "alice")
	pubbing.PublishState = "published"
	dispatch.CreateUnit(context.Background(), pubbing)
	if _, err := gate.Cancel(context.Background(), pubbing.UnitID, "", NewHumanActor("alice"), AuditMeta{}); !errors.Is(err, algopublisher.ErrInvalidState) {
		t.Fatalf("已发布不可撤回，实际 %v", err)
	}
}

func DefaultConfPtr() *Conf {
	c := DefaultConf()
	return &c
}
