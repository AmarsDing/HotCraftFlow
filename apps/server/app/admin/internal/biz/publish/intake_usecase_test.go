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

// intakeFixture 建单测试的通用夹具（mock 渠道 + 已注册适配器）。
type intakeFixture struct {
	channels *fakeChannels
	dispatch *fakeDispatch
	records  *fakeRecords
	ads      *fakeAdapters
}

func newIntakeFixture() *intakeFixture {
	pubs := map[string]Publisher{}
	for _, p := range []string{"youtube", "bilibili", "x"} {
		pubs[p] = newSuccessPublisher(p, nil, nil)
	}
	return &intakeFixture{
		channels: newFakeChannels(
			&ChannelRow{ChannelID: "ch_yt", PlatformKey: "youtube", Slot: "main", Enabled: true,
				Capability: capabilitySnapshot(Capability{NativeSchedule: true})},
			&ChannelRow{ChannelID: "ch_bili", PlatformKey: "bilibili", Slot: "main", Enabled: true,
				Capability: capabilitySnapshot(Capability{NativeSchedule: true})},
			&ChannelRow{ChannelID: "ch_x", PlatformKey: "x", Slot: "main", Enabled: true,
				Capability: capabilitySnapshot(Capability{NativeSchedule: false})},
			&ChannelRow{ChannelID: "ch_x_off", PlatformKey: "x", Slot: "backup", Enabled: false},
		),
		dispatch: newFakeDispatch(),
		records:  &fakeRecords{},
		ads:      &fakeAdapters{pubs: pubs},
	}
}

func (fx *intakeFixture) intake(conf *Conf) *IntakeUsecase {
	return NewIntakeUsecase(conf, fx.channels, fx.dispatch, fx.records, fx.ads, log.DefaultLogger)
}

// 目标校验矩阵：空 / 格式非法 / 重复 / 渠道不存在 / 渠道停用 / 适配器未注册。
func TestIntake_TargetValidation(t *testing.T) {
	fx := newIntakeFixture()
	intake := fx.intake(DefaultConfPtr())
	ctx := context.Background()

	cases := []struct {
		name    string
		targets []string
		want    error
	}{
		{"空 targets", nil, ErrBadRequest},
		{"格式非法（无槽位）", []string{"youtube"}, ErrBadRequest},
		{"格式非法（空平台）", []string{":main"}, ErrBadRequest},
		{"重复目标", []string{"youtube:main", "youtube:main"}, ErrBadRequest},
		{"渠道不存在", []string{"youtube:second"}, ErrChannelNotFound},
		{"渠道停用", []string{"x:backup"}, ErrChannelNotFound},
		{"适配器未注册", []string{"reddit:main"}, ErrAdapterNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "适配器未注册" {
				fx.channels.CreateChannel(ctx, &ChannelRow{ChannelID: "ch_rd", PlatformKey: "reddit", Slot: "main", Enabled: true})
			}
			_, err := intake.Create(ctx, CreateUnitInput{Targets: tc.targets, SubmittedBy: "alice"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("期望 %v，实际 %v", tc.want, err)
			}
		})
	}
}

// 可见性规则：非法值 → BadRequest；非 private 无 reason → ErrVisibilityNoReason；
// public + reason + allow_public → 通过；allow_public=false 时 public 拒绝。
func TestIntake_VisibilityRules(t *testing.T) {
	fx := newIntakeFixture()
	intake := fx.intake(DefaultConfPtr())
	ctx := context.Background()
	targets := []string{"youtube:main"}

	if _, err := intake.Create(ctx, CreateUnitInput{Targets: targets, SubmittedBy: "a", Visibility: "friends"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("非法可见性期望 BadRequest，实际 %v", err)
	}
	if _, err := intake.Create(ctx, CreateUnitInput{Targets: targets, SubmittedBy: "a",
		Visibility: string(mediacontract.VisibilityUnlisted)}); !errors.Is(err, algopublisher.ErrVisibilityNoReason) {
		t.Fatalf("unlisted 无 reason 期望 MAP_VISIBILITY_NO_REASON，实际 %v", err)
	}
	u, err := intake.Create(ctx, CreateUnitInput{Targets: targets, SubmittedBy: "a",
		Visibility: string(mediacontract.VisibilityPublic), VisibilityOverrideReason: "品牌排期"})
	if err != nil {
		t.Fatalf("public + reason 应通过: %v", err)
	}
	if u.VisibilityDefault != string(mediacontract.VisibilityPublic) || u.VisibilityOverrideReason == nil {
		t.Fatalf("可见性未落库: %+v", u)
	}

	closed := DefaultConf()
	closed.Security.AllowPublic = false
	intake2 := fx.intake(&closed)
	if _, err := intake2.Create(ctx, CreateUnitInput{Targets: targets, SubmittedBy: "a",
		Visibility: string(mediacontract.VisibilityPublic), VisibilityOverrideReason: "r"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("allow_public=false 时 public 应拒绝，实际 %v", err)
	}
}

// 排期模式：全目标 native → native；有非 native + worker 未启用 → none；启用 → local_worker。
func TestIntake_ScheduleMode(t *testing.T) {
	fx := newIntakeFixture()
	intake := fx.intake(DefaultConfPtr())
	ctx := context.Background()
	future := time.Now().Add(4 * time.Hour)

	u, err := intake.Create(ctx, CreateUnitInput{
		Targets: []string{"youtube:main", "bilibili:main"}, ScheduledFor: &future, SubmittedBy: "a"})
	if err != nil || u.ScheduleMode != "native" {
		t.Fatalf("全 native 目标应为 native，实际 mode=%s err=%v", u.ScheduleMode, err)
	}

	u2, err := intake.Create(ctx, CreateUnitInput{
		Targets: []string{"youtube:main", "x:main"}, ScheduledFor: &future, SubmittedBy: "a"})
	if err != nil || u2.ScheduleMode != "none" {
		t.Fatalf("含非 native 且 worker 未启用应为 none，实际 mode=%s err=%v", u2.ScheduleMode, err)
	}

	workerOn := DefaultConf()
	workerOn.Schedule.LocalWorkerEnabled = true
	u3, err := fx.intake(&workerOn).Create(ctx, CreateUnitInput{
		Targets: []string{"youtube:main", "x:main"}, ScheduledFor: &future, SubmittedBy: "a"})
	if err != nil || u3.ScheduleMode != "local_worker" {
		t.Fatalf("worker 启用时应为 local_worker，实际 mode=%s err=%v", u3.ScheduleMode, err)
	}
}

// 建单即审计（submit）：SubmittedBy 记录 + allow 留痕。
func TestIntake_AuditOnCreate(t *testing.T) {
	fx := newIntakeFixture()
	u, err := fx.intake(DefaultConfPtr()).Create(context.Background(), CreateUnitInput{
		Targets: []string{"youtube:main"}, SubmittedBy: "alice", Meta: AuditMeta{IP: "10.0.0.1", UserAgent: "ua-x"}})
	if err != nil {
		t.Fatalf("建单失败: %v", err)
	}
	if u.SubmittedBy != "alice" {
		t.Fatalf("submitted_by 未记录: %q", u.SubmittedBy)
	}
	recs, _ := fx.records.ListRecordsByUnit(context.Background(), u.UnitID)
	if len(recs) != 1 || recs[0].Action != AuditActionSubmit || recs[0].Result != "allow" {
		t.Fatalf("期望 1 条 submit allow 审计，实际 %+v", recs)
	}
	if recs[0].IP == nil || *recs[0].IP != "10.0.0.1" {
		t.Fatalf("审计 IP 未记录: %+v", recs[0].IP)
	}
}
