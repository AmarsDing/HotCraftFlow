package publish

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
	"framework/pkg/mediacontract"
)

// CreateUnitInput 建单输入（来自 API；SubmittedBy 由服务端从 JWT 提取，绝不信任请求体）。
type CreateUnitInput struct {
	VariantSetID string
	VariantID    string
	Targets      []string // ["platform:slot", ...]，必填
	ScheduledFor *time.Time
	Visibility   string // 缺省强制 private（NFR-MAP-01）
	VisibilityOverrideReason string
	SubmittedBy  string
	Meta         AuditMeta
}

// IntakeUsecase 建单用例（FR-PUB-01/03/06 的入口侧）。
//
// 🔴 安全默认值：visibility 缺失 → private；非 private 且无 override_reason →
// 当场拒绝（与其等到发布门才爆，不如建单即提示）。
// 🔴 目标平台必须注册适配器——未命中即 MAP_ADAPTER_NOT_FOUND 明确报错（ADR-MAP-05
// 拒绝静默降级；YouTube 等真实平台适配器未实装前，诚实报错胜过假装可发）。
type IntakeUsecase struct {
	conf      *Conf
	channels  ChannelRepo
	dispatch  DispatchRepo
	records   ApprovalRepo
	adapters  AdapterLookup
	clock     func() time.Time
	log       *log.Helper
}

// NewIntakeUsecase 构造建单用例。
func NewIntakeUsecase(
	conf *Conf,
	channels ChannelRepo,
	dispatch DispatchRepo,
	records ApprovalRepo,
	adapters AdapterLookup,
	logger log.Logger,
) *IntakeUsecase {
	return &IntakeUsecase{
		conf:     conf,
		channels: channels,
		dispatch: dispatch,
		records:  records,
		adapters: adapters,
		clock:    time.Now,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish-intake")),
	}
}

// Create 创建分发单元（创建即记 submit 审计）。
func (uc *IntakeUsecase) Create(ctx context.Context, in CreateUnitInput) (*UnitRow, error) {
	// 1) targets 必填且逐个合法 + 渠道存在且启用 + 适配器已注册。
	if len(in.Targets) == 0 {
		return nil, fmt.Errorf("%w: targets 必填（platform:slot，至少一个）", ErrBadRequest)
	}
	seen := make(map[string]struct{}, len(in.Targets))
	allNative := true
	for _, raw := range in.Targets {
		t, err := ParseTarget(raw)
		if err != nil {
			return nil, err
		}
		key := t.String()
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("%w: 重复的目标 %q", ErrBadRequest, key)
		}
		seen[key] = struct{}{}

		ch, err := uc.channels.GetChannelBySlot(ctx, t.Platform, t.Slot)
		if err != nil {
			return nil, fmt.Errorf("%w: 目标 %s 的渠道不存在", ErrChannelNotFound, key)
		}
		if !ch.Enabled {
			return nil, fmt.Errorf("%w: 目标 %s 的渠道已停用", ErrChannelNotFound, key)
		}
		if _, ok := uc.adapters.Get(t.Platform); !ok {
			return nil, fmt.Errorf("%w: 平台 %s 适配器未注册（明确报错，不静默降级）", ErrAdapterNotFound, t.Platform)
		}
		if !CapabilityOf(ch, nil).NativeSchedule {
			allNative = false
		}
	}

	// 2) 可见性（🔴 默认恒 private；非 private 必须给 override_reason）。
	visibility := strings.TrimSpace(in.Visibility)
	if visibility == "" {
		visibility = string(mediacontract.VisibilityPrivate)
	}
	if !mediacontract.Visibility(visibility).Valid() {
		return nil, fmt.Errorf("%w: 非法可见性 %q（仅 private/unlisted/public）", ErrBadRequest, visibility)
	}
	reason := strings.TrimSpace(in.VisibilityOverrideReason)
	if visibility != string(mediacontract.VisibilityPrivate) {
		if reason == "" {
			return nil, fmt.Errorf("%w: 非 private 可见性必须填写 override_reason", algopublisher.ErrVisibilityNoReason)
		}
		if visibility == string(mediacontract.VisibilityPublic) && !uc.conf.Security.AllowPublic {
			return nil, fmt.Errorf("%w: 当前配置禁止 public 发布（security.allow_public=false）", ErrBadRequest)
		}
	}

	// 3) 排期模式（V1 诚实降级：全目标支持平台原生排期 → native；
	// 否则 local_worker 启用时 → local_worker（M4 asynq worker 后置，先记账）；
	// 否则 none——scheduled_for 仅作展示意图，发布时间以人工点击为准）。
	scheduleMode := "none"
	if in.ScheduledFor != nil {
		switch {
		case allNative:
			scheduleMode = "native"
		case uc.conf.Schedule.LocalWorkerEnabled:
			scheduleMode = "local_worker"
		default:
			scheduleMode = "none"
		}
	}

	unit := &UnitRow{
		UnitID:                   mediacontract.NewDispatchUnitID(),
		Targets:                  in.Targets,
		ScheduleMode:             scheduleMode,
		VisibilityDefault:        visibility,
		VisibilityOverrideReason: strPtrOrNil(reason),
		ApprovalState:            string(mediacontract.ApprovalDraft),
		SubmittedBy:              in.SubmittedBy,
		PublishState:             "pending",
	}
	if in.VariantSetID != "" {
		unit.VariantSetID = strPtr(in.VariantSetID)
	}
	if in.VariantID != "" {
		unit.VariantID = strPtr(in.VariantID)
	}
	if in.ScheduledFor != nil {
		t := in.ScheduledFor.UTC()
		unit.ScheduledFor = &t
	}
	if err := uc.dispatch.CreateUnit(ctx, unit); err != nil {
		return nil, err
	}

	// 4) 创建即审计（submit）。
	recordAudit(ctx, uc.records, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionSubmit,
		Actor:              in.SubmittedBy,
		ActorType:          string(mediacontract.ActorHuman),
		ToState:            strPtr(unit.ApprovalState),
		Reason:             strPtrOrNil(reason),
		VisibilityAtAction: strPtr(visibility),
		IP:                 strPtrOrNil(in.Meta.IP),
		UserAgent:          strPtrOrNil(in.Meta.UserAgent),
		Result:             "allow",
	}, uc.log)
	return unit, nil
}
