package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	mapV1 "framework/api/gen/go/media/v1/map"
	algopublisher "framework/app/admin/internal/algo/publisher"
	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/publishadapters"
	"framework/pkg/middleware/auth"
	"framework/pkg/middleware/logging"
)

// MapService ⑤ MAP REST 服务（概要设计 §8.2 路由矩阵）。
//
// 🔴 服务层纪律：
//   - approver/operator/submitted_by 一律取自 JWT（auth.FromContext），绝不采信请求体身份字段。
//   - 每条状态变更路径都带 IP/UA 进审计（map_approval_records）。
//   - 🔴 credential 明文只在本层入口存在一瞬，传 biz 后不回显、不记日志（NFR-MAP-03）。
//   - biz 语义错误 → MapErrorReason 精确映射（mapBizError），前端按 reason 提示。
type MapService struct {
	mapV1.MapServiceHTTPServer

	log       *log.Helper
	gate      *bizpub.ApprovalGateUsecase
	intake    *bizpub.IntakeUsecase
	publisher *bizpub.PublishUsecase
	dispatch  *bizpub.DispatchUsecase
	channels  *bizpub.ChannelUsecase
	accounts  *bizpub.AccountUsecase
	catalog   *publishadapters.PlatformCatalog // 平台运营配置（requires_qualification 提示）
}

// NewMapService 构造。
func NewMapService(
	logger log.Logger,
	gate *bizpub.ApprovalGateUsecase,
	intake *bizpub.IntakeUsecase,
	publisher *bizpub.PublishUsecase,
	dispatch *bizpub.DispatchUsecase,
	channels *bizpub.ChannelUsecase,
	accounts *bizpub.AccountUsecase,
	catalog *publishadapters.PlatformCatalog,
) *MapService {
	return &MapService{
		log:       log.NewHelper(log.With(logger, "module", "map/service/admin-service")),
		gate:      gate,
		intake:    intake,
		publisher: publisher,
		dispatch:  dispatch,
		channels:  channels,
		accounts:  accounts,
		catalog:   catalog,
	}
}

// ==================== 身份与审计上下文 ====================

// mapActorFromContext 提取操作者（🔴 唯一身份来源：JWT）。
// V1 全部为 human；自动化身份调用路径（asynq worker 内部调用）后续以 NewAutomationActor 注入。
func mapActorFromContext(ctx context.Context) (bizpub.Actor, error) {
	payload, err := auth.FromContext(ctx)
	if err != nil {
		return bizpub.Actor{}, err
	}
	id := strings.TrimSpace(payload.GetUsername())
	if id == "" {
		id = fmt.Sprintf("u%d", payload.GetUserId())
	}
	return bizpub.NewHumanActor(id), nil
}

// mapAuditMeta 提取 IP/UA（复用日志中间件的 XFF 解析；非 HTTP 调用为空串）。
func mapAuditMeta(ctx context.Context) bizpub.AuditMeta {
	return bizpub.AuditMeta{
		IP:        logging.ClientIPFromContext(ctx),
		UserAgent: logging.UserAgentFromContext(ctx),
	}
}

// ==================== 渠道 ====================

// ListChannels 查询渠道列表。
func (s *MapService) ListChannels(ctx context.Context, req *pagination.PagingRequest) (*mapV1.ListChannelsResponse, error) {
	rows, total, err := s.channels.List(ctx, int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapBizError(err)
	}
	items := make([]*mapV1.MapChannel, 0, len(rows))
	for _, r := range rows {
		items = append(items, channelToDTO(r))
	}
	return &mapV1.ListChannelsResponse{Total: uint32(total), Items: items}, nil
}

// CreateChannel 创建渠道（能力快照服务端写死：适配器声明 / platforms.yaml）。
func (s *MapService) CreateChannel(ctx context.Context, req *mapV1.CreateChannelRequest) (*mapV1.MapChannel, error) {
	row, err := s.channels.Create(ctx, bizpub.CreateChannelInput{
		PlatformKey:    req.GetPlatformKey(),
		Slot:           req.GetSlot(),
		Region:         req.GetRegion(),
		Enabled:        req.GetEnabled(),
		Priority:       int(req.GetPriority()),
		AdapterVersion: req.GetAdapterVersion(),
	})
	if err != nil {
		return nil, mapBizError(err)
	}
	// 平台资质提示（douyin/kuaishou/xiaohongshu 企业资质；wechat_channels 无开放 API）——仅 Warn 级日志提示，不阻断建档。
	if s.catalog != nil {
		if conf, ok := s.catalog.ConfOf(row.PlatformKey); ok {
			switch {
			case !conf.Enabled:
				s.log.Warnf("[map] 渠道建于未启用平台 %s（note: %s）", row.PlatformKey, conf.Note)
			case conf.RequiresQualification:
				s.log.Warnf("[map] 渠道 %s:%s 需企业资质（%s），发布前请确认授权到位", row.PlatformKey, row.Slot, conf.Note)
			}
		}
	}
	return channelToDTO(row), nil
}

// ==================== 账号 ====================

// ListAccounts 查询账号列表（🔴 响应永不包含凭据本体，仅 has_credential 标记）。
func (s *MapService) ListAccounts(ctx context.Context, req *pagination.PagingRequest) (*mapV1.ListAccountsResponse, error) {
	rows, total, err := s.accounts.List(ctx, int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapBizError(err)
	}
	// 冗余渠道字段装配（行政后台页量级小，逐渠道查询可接受）。
	chCache := map[string]*bizpub.ChannelRow{}
	for _, r := range rows {
		ch, ok := chCache[r.ChannelID]
		if !ok {
			if got, gerr := s.channels.Get(ctx, r.ChannelID); gerr == nil {
				ch = got
			}
			chCache[r.ChannelID] = ch
		}
		if ch != nil {
			r.PlatformKey = ch.PlatformKey
			r.Slot = ch.Slot
		}
	}
	items := make([]*mapV1.MapAccount, 0, len(rows))
	for _, r := range rows {
		items = append(items, accountToDTO(r))
	}
	return &mapV1.ListAccountsResponse{Total: uint32(total), Items: items}, nil
}

// CreateAccount 创建账号（凭据明文仅经 TLS 传输一次，落库前加密；响应不回显）。
func (s *MapService) CreateAccount(ctx context.Context, req *mapV1.CreateAccountRequest) (*mapV1.MapAccount, error) {
	row, err := s.accounts.Create(ctx, bizpub.CreateAccountInput{
		ChannelID:      req.GetChannelId(),
		DisplayName:    req.GetDisplayName(),
		CredentialType: req.GetCredentialType(),
		Credential:     req.GetCredential(),
		Enabled:        req.GetEnabled(),
	})
	req.Credential = "" // 🔴 请求体内明文立刻擦除（防后续日志/中间件误录）
	if err != nil {
		return nil, mapBizError(err)
	}
	return accountToDTO(row), nil
}

// ToggleAccount 启停账号。
func (s *MapService) ToggleAccount(ctx context.Context, req *mapV1.ToggleAccountRequest) (*mapV1.MapAccount, error) {
	row, err := s.accounts.Toggle(ctx, req.GetId(), req.GetEnabled())
	if err != nil {
		return nil, mapBizError(err)
	}
	return accountToDTO(row), nil
}

// ReauthAccount 重新授权（凭据失效 → 覆盖新凭据 + 健康复位；🔴 凭据失效从不清旧凭据）。
func (s *MapService) ReauthAccount(ctx context.Context, req *mapV1.ReauthAccountRequest) (*emptypb.Empty, error) {
	err := s.accounts.Reauth(ctx, req.GetId(), req.GetCredentialType(), req.GetCredential())
	req.Credential = "" // 🔴 擦除明文
	if err != nil {
		return nil, mapBizError(err)
	}
	return &emptypb.Empty{}, nil
}

// ==================== 分发单元 ====================

// ListDispatchUnits 查询分发单元列表。
func (s *MapService) ListDispatchUnits(ctx context.Context, req *pagination.PagingRequest) (*mapV1.ListDispatchUnitsResponse, error) {
	rows, total, err := s.dispatch.ListUnits(ctx, int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapBizError(err)
	}
	items := make([]*mapV1.MapDispatchUnitItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, unitToDTO(r))
	}
	return &mapV1.ListDispatchUnitsResponse{Total: uint32(total), Items: items}, nil
}

// CreateDispatchUnit 创建分发单元（创建即记 submit 审计；可见性缺省强制 private）。
func (s *MapService) CreateDispatchUnit(ctx context.Context, req *mapV1.CreateDispatchUnitRequest) (*mapV1.MapDispatchUnitItem, error) {
	actor, err := mapActorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var scheduledFor *time.Time
	if req.GetScheduledFor() != nil {
		t := req.GetScheduledFor().AsTime()
		scheduledFor = &t
	}
	unit, err := s.intake.Create(ctx, bizpub.CreateUnitInput{
		VariantSetID:             req.GetVariantSetId(),
		VariantID:                req.GetVariantId(),
		Targets:                  req.GetTargets(),
		ScheduledFor:             scheduledFor,
		Visibility:               req.GetVisibilityDefault(),
		VisibilityOverrideReason: req.GetVisibilityOverrideReason(),
		SubmittedBy:              actor.ID,
		ActorType:                string(actor.Type),
		Meta:                     mapAuditMeta(ctx),
	})
	if err != nil {
		return nil, mapBizError(err)
	}
	return unitToDTO(unit), nil
}

// GetDispatchUnit 详情（含审批记录与回执）。
func (s *MapService) GetDispatchUnit(ctx context.Context, req *mapV1.GetDispatchUnitRequest) (*mapV1.MapDispatchUnitDetail, error) {
	detail, err := s.dispatch.GetUnitDetail(ctx, req.GetId())
	if err != nil {
		return nil, mapBizError(err)
	}
	records := make([]*mapV1.MapApprovalRecordItem, 0, len(detail.Records))
	for _, r := range detail.Records {
		records = append(records, recordToDTO(r))
	}
	receipts := make([]*mapV1.MapReceipt, 0, len(detail.Receipts))
	for _, r := range detail.Receipts {
		receipts = append(receipts, receiptToDTO(r))
	}
	return &mapV1.MapDispatchUnitDetail{
		Unit:     unitToDTO(detail.Unit),
		Records:  records,
		Receipts: receipts,
	}, nil
}

// SubmitDispatchUnit 重新提交（rejected → draft）。
func (s *MapService) SubmitDispatchUnit(ctx context.Context, req *mapV1.SubmitDispatchUnitRequest) (*mapV1.MapDispatchUnitItem, error) {
	actor, err := mapActorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	unit, err := s.gate.Submit(ctx, req.GetId(), actor, mapAuditMeta(ctx))
	if err != nil {
		return nil, mapBizError(err)
	}
	return unitToDTO(unit), nil
}

// ApproveDispatchUnit 🔴 审批通过（审批人取自 JWT；自批必败；与发布严格分离）。
func (s *MapService) ApproveDispatchUnit(ctx context.Context, req *mapV1.ApproveDispatchUnitRequest) (*mapV1.MapDispatchUnitItem, error) {
	actor, err := mapActorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	unit, err := s.gate.Approve(ctx, req.GetId(), actor, mapAuditMeta(ctx))
	if err != nil {
		return nil, mapBizError(err)
	}
	return unitToDTO(unit), nil
}

// RejectDispatchUnit 驳回（reason 必填）。
func (s *MapService) RejectDispatchUnit(ctx context.Context, req *mapV1.RejectDispatchUnitRequest) (*mapV1.MapDispatchUnitItem, error) {
	actor, err := mapActorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	unit, err := s.gate.Reject(ctx, req.GetId(), req.GetReason(), actor, mapAuditMeta(ctx))
	if err != nil {
		return nil, mapBizError(err)
	}
	return unitToDTO(unit), nil
}

// PublishDispatchUnit 🔴 发布（五校验 + 非 private 二次确认 + 幂等锁；operator 取自 JWT）。
func (s *MapService) PublishDispatchUnit(ctx context.Context, req *mapV1.PublishDispatchUnitRequest) (*mapV1.PublishDispatchUnitResponse, error) {
	actor, err := mapActorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	outcome, err := s.publisher.Publish(ctx, req.GetId(), actor, req.GetConfirmPublic(), mapAuditMeta(ctx))
	if err != nil {
		return nil, mapBizError(err)
	}
	receipts := make([]*mapV1.MapReceipt, 0, len(outcome.Receipts))
	for _, r := range outcome.Receipts {
		receipts = append(receipts, receiptToDTO(r))
	}
	return &mapV1.PublishDispatchUnitResponse{
		PublishState: outcome.Unit.PublishState,
		Receipts:     receipts,
		Warnings:     outcome.Warnings,
	}, nil
}

// CancelDispatchUnit 撤回。
func (s *MapService) CancelDispatchUnit(ctx context.Context, req *mapV1.CancelDispatchUnitRequest) (*mapV1.MapDispatchUnitItem, error) {
	actor, err := mapActorFromContext(ctx)
	if err != nil {
		return nil, err
	}
	unit, err := s.gate.Cancel(ctx, req.GetId(), req.GetReason(), actor, mapAuditMeta(ctx))
	if err != nil {
		return nil, mapBizError(err)
	}
	return unitToDTO(unit), nil
}

// ==================== 审批队列 ====================

// ListApprovalQueue 待审批队列（approval_state=draft；前端核心页面）。
func (s *MapService) ListApprovalQueue(ctx context.Context, req *pagination.PagingRequest) (*mapV1.ListApprovalQueueResponse, error) {
	rows, total, err := s.dispatch.ListApprovalQueue(ctx, int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapBizError(err)
	}
	items := make([]*mapV1.MapDispatchUnitItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, unitToDTO(r))
	}
	return &mapV1.ListApprovalQueueResponse{Total: uint32(total), Items: items}, nil
}

// ==================== 回执 / 日历 / 表现 ====================

// ListReceipts 回执列表（post_id/时间/可见性/操作者；前端 CSV 导出）。
func (s *MapService) ListReceipts(ctx context.Context, req *pagination.PagingRequest) (*mapV1.ListReceiptsResponse, error) {
	rows, total, err := s.dispatch.ListReceipts(ctx, int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapBizError(err)
	}
	items := make([]*mapV1.MapReceipt, 0, len(rows))
	for _, r := range rows {
		items = append(items, receiptToDTO(r))
	}
	return &mapV1.ListReceiptsResponse{Total: uint32(total), Items: items}, nil
}

// GetCalendar 矩阵日历（只读）。
func (s *MapService) GetCalendar(ctx context.Context, req *mapV1.GetCalendarRequest) (*mapV1.GetCalendarResponse, error) {
	if req.GetFrom() == nil || req.GetTo() == nil {
		return nil, mapV1.ErrorMapBadRequest("日历查询必须提供 from/to 时间窗")
	}
	rows, err := s.dispatch.ListCalendar(ctx, req.GetFrom().AsTime(), req.GetTo().AsTime())
	if err != nil {
		return nil, mapBizError(err)
	}
	items := make([]*mapV1.MapCalendarItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, &mapV1.MapCalendarItem{
			UnitId:        r.UnitID,
			Targets:       r.Targets,
			ScheduledFor:  tsPtr(r.ScheduledFor),
			ScheduleMode:  r.ScheduleMode,
			ApprovalState: r.ApprovalState,
			PublishState:  r.PublishState,
		})
	}
	return &mapV1.GetCalendarResponse{Items: items}, nil
}

// GetAnalytics 表现数据（无快照时返回诚实 note——绝不伪造数字）。
func (s *MapService) GetAnalytics(ctx context.Context, req *mapV1.GetAnalyticsRequest) (*mapV1.MapAnalyticsView, error) {
	snap, receipt, err := s.dispatch.GetAnalytics(ctx, req.GetId())
	if err != nil {
		return nil, mapBizError(err)
	}
	view := &mapV1.MapAnalyticsView{
		ReceiptId:   receipt.ReceiptID,
		PlatformKey: receipt.PlatformKey,
	}
	if snap == nil {
		// 🔴 诚实空态：拉取管道已落地（worker/map analytics），但总闸 feedback.enabled 默认关闭
		// 或该回执尚无快照（30 天窗/平台未声明 analytics 能力）。
		view.Note = "表现数据尚未回流（feedback.enabled 未开启或该回执尚无快照）；计数为 0 表示无数据而非真实为零"
		return view, nil
	}
	view.Views = snap.Views
	view.Likes = snap.Likes
	view.Comments = snap.Comments
	view.Shares = snap.Shares
	view.Favorites = snap.Favorites
	view.CompletionRate = snap.CompletionRate
	view.FetchedAt = timestamppb.New(snap.FetchedAt)
	// 🔴 冻结态诚实标注：回执出回流扫描窗（bizpub.FeedbackScanWindow）后 worker 停止
	// 追加快照（详细设计 §7.4），最新一行即窗口沿冻结值——不伪装新鲜，消费方按 fetched_at
	// 判断陈旧度。窗口常量与 worker 同源，避免两侧口径漂移。
	if receipt.PublishedAt.Before(time.Now().Add(-bizpub.FeedbackScanWindow)) {
		view.Note = fmt.Sprintf("该回执已超出 %d 天回流窗（published_at=%s），快照停止更新；fetched_at 为最后一轮拉取时间，计数为冻结值",
			int(bizpub.FeedbackScanWindow/(24*time.Hour)), receipt.PublishedAt.UTC().Format(time.RFC3339))
	}
	return view, nil
}

// ==================== DTO 转换 ====================

func channelToDTO(r *bizpub.ChannelRow) *mapV1.MapChannel {
	var capJSON string
	if r.Capability != nil {
		if b, err := json.Marshal(r.Capability); err == nil {
			capJSON = string(b)
		}
	}
	return &mapV1.MapChannel{
		ChannelId:      r.ChannelID,
		PlatformKey:    r.PlatformKey,
		Slot:           r.Slot,
		Region:         r.Region,
		Enabled:        r.Enabled,
		CapabilityJson: capJSON,
		AdapterVersion: derefStr(r.AdapterVersion),
		Priority:       int32(r.Priority),
		CreateTime:     tsVal(r.CreateTime),
	}
}

func accountToDTO(r *bizpub.AccountRow) *mapV1.MapAccount {
	return &mapV1.MapAccount{
		AccountId:           r.AccountID,
		ChannelId:           r.ChannelID,
		PlatformKey:         r.PlatformKey,
		Slot:                r.Slot,
		DisplayName:         r.DisplayName,
		CredentialType:      r.CredentialType,
		HasCredential:       r.HasCredential,
		Enabled:             r.Enabled,
		Health:              r.Health,
		ConsecutiveFailures: int32(r.ConsecutiveFailures),
		PausedUntil:         tsPtr(r.PausedUntil),
		Published_24H:       int32(r.Published24h),
		LastPublishedAt:     tsPtr(r.LastPublishedAt),
		LastError:           derefStr(r.LastError),
		CreateTime:          tsVal(r.CreateTime),
	}
}

func unitToDTO(r *bizpub.UnitRow) *mapV1.MapDispatchUnitItem {
	return &mapV1.MapDispatchUnitItem{
		UnitId:                   r.UnitID,
		VariantSetId:             derefStr(r.VariantSetID),
		VariantId:                derefStr(r.VariantID),
		Targets:                  r.Targets,
		ScheduledFor:             tsPtr(r.ScheduledFor),
		ScheduleMode:             r.ScheduleMode,
		VisibilityDefault:        r.VisibilityDefault,
		VisibilityOverrideReason: derefStr(r.VisibilityOverrideReason),
		ApprovalState:            r.ApprovalState,
		SubmittedBy:              r.SubmittedBy,
		Approver:                 derefStr(r.Approver),
		ApprovedAt:               tsPtr(r.ApprovedAt),
		RejectReason:             derefStr(r.RejectReason),
		PublishState:             r.PublishState,
		PostId:                   derefStr(r.PostID),
		PostUrl:                  derefStr(r.PostURL),
		PublishedAt:              tsPtr(r.PublishedAt),
		Operator:                 derefStr(r.Operator),
		CreateTime:               tsVal(r.CreateTime),
	}
}

func recordToDTO(r *bizpub.RecordEntry) *mapV1.MapApprovalRecordItem {
	return &mapV1.MapApprovalRecordItem{
		Id:                 r.ID,
		Action:             r.Action,
		Actor:              r.Actor,
		ActorType:          r.ActorType,
		FromState:          derefStr(r.FromState),
		ToState:            derefStr(r.ToState),
		Reason:             derefStr(r.Reason),
		VisibilityAtAction: derefStr(r.VisibilityAtAction),
		Ip:                 derefStr(r.IP),
		Result:             r.Result,
		CreateTime:         tsVal(r.CreateTime),
	}
}

func receiptToDTO(r *bizpub.ReceiptRow) *mapV1.MapReceipt {
	return &mapV1.MapReceipt{
		ReceiptId:      r.ReceiptID,
		DispatchUnitId: r.DispatchUnitID,
		AccountId:      derefStr(r.AccountID),
		PlatformKey:    r.PlatformKey,
		PostId:         derefStr(r.PostID),
		PostUrl:        derefStr(r.PostURL),
		Visibility:     r.Visibility,
		PublishedAt:    tsVal(r.PublishedAt),
		Operator:       r.Operator,
	}
}

// ==================== 错误映射（biz 语义错误 → MapErrorReason） ====================

// mapBizError biz/算法层错误 → mapV1 status 错误（精确 reason 映射；消息做 token 前缀清理）。
func mapBizError(err error) error {
	msg := cleanedMapMsg(err)
	switch {
	// —— 审批门（🔴 安全类，403/409 全留痕于 biz 层） ——
	case errors.Is(err, algopublisher.ErrNotApproved):
		return mapV1.ErrorMapNotApproved("%s", msg)
	case errors.Is(err, algopublisher.ErrSelfApproval):
		return mapV1.ErrorMapSelfApproval("%s", msg)
	case errors.Is(err, algopublisher.ErrAutomationCannotApprove),
		errors.Is(err, algopublisher.ErrAutomationCannotPublish):
		return mapV1.ErrorMapAutomationDenied("%s", msg)
	case errors.Is(err, algopublisher.ErrVisibilityNoReason):
		return mapV1.ErrorMapVisibilityNoReason("%s", msg)
	case errors.Is(err, algopublisher.ErrInvalidState):
		return mapV1.ErrorMapInvalidState("%s", msg)
	case errors.Is(err, algopublisher.ErrAccountPaused):
		return mapV1.ErrorMapAccountPaused("%s", msg)
	case errors.Is(err, algopublisher.ErrQuotaExceeded):
		return mapV1.ErrorMapQuotaExceeded("%s", msg)

	// —— biz 语义错误 ——
	case errors.Is(err, bizpub.ErrNotFound):
		return mapV1.ErrorMapNotFound("%s", msg)
	case errors.Is(err, bizpub.ErrChannelNotFound):
		return mapV1.ErrorMapChannelNotFound("%s", msg)
	case errors.Is(err, bizpub.ErrAdapterNotFound):
		return mapV1.ErrorMapAdapterNotFound("%s", msg)
	case errors.Is(err, bizpub.ErrCapabilityUnsupported):
		return mapV1.ErrorMapCapabilityUnsupported("%s", msg)
	case errors.Is(err, bizpub.ErrBadRequest):
		return mapV1.ErrorMapBadRequest("%s", msg)
	case errors.Is(err, bizpub.ErrRejectReasonRequired):
		return mapV1.ErrorMapRejectReasonRequired("%s", msg)
	case errors.Is(err, bizpub.ErrPublicConfirmRequired):
		return mapV1.ErrorMapPublicConfirmRequired("%s", msg)
	case errors.Is(err, bizpub.ErrAlreadyPublishing):
		return mapV1.ErrorMapAlreadyPublishing("%s", msg)
	case errors.Is(err, bizpub.ErrCredentialInvalid):
		return mapV1.ErrorMapCredentialInvalid("%s", msg)
	case errors.Is(err, bizpub.ErrUpstream):
		return mapV1.ErrorMapUpstreamError("%s", msg)
	default:
		return mapV1.ErrorMapInternal("%s", msg)
	}
}

// cleanedMapMsg 去掉「MAP_XXX: 」token 前缀（reason 已由 status 承载，message 保留人类可读部分）。
func cleanedMapMsg(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && i < 44 {
		if strings.HasPrefix(msg[:i], "MAP_") && !strings.ContainsAny(msg[:i], " /") {
			return msg[i+2:]
		}
	}
	return msg
}

// ==================== 时间/字符串小助手 ====================

func tsVal(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func tsPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
