package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	internalMessageV1 "framework/api/gen/go/internal_message/v1"

	"framework/pkg/middleware/auth"
)

type NotificationMessageRecipientService struct {
	adminV1.NotificationMessageRecipientServiceHTTPServer

	log *log.Helper

	uc *biz.NotificationMessageRecipientUsecase
}

func NewNotificationMessageRecipientService(logger log.Logger, uc *biz.NotificationMessageRecipientUsecase) *NotificationMessageRecipientService {
	l := log.NewHelper(log.With(logger, "module", "notification-message-recipient/service/admin-service"))
	return &NotificationMessageRecipientService{
		log: l,
		uc:  uc,
	}
}

func (s *NotificationMessageRecipientService) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageRecipientResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *NotificationMessageRecipientService) Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRecipientRequest) (*internalMessageV1.NotificationMessageRecipient, error) {
	return s.uc.Get(ctx, req)
}

func (s *NotificationMessageRecipientService) Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRecipientRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *NotificationMessageRecipientService) Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRecipientRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *NotificationMessageRecipientService) Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRecipientRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListMy 查询当前登录用户的通知收件箱（收件记录并联消息标题/内容）。
func (s *NotificationMessageRecipientService) ListMy(ctx context.Context, req *internalMessageV1.ListMyNotificationRequest) (*internalMessageV1.ListMyNotificationResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	return s.uc.ListMy(ctx, operator.UserId, req)
}

// CountMyUnread 查询当前登录用户未读通知数。
func (s *NotificationMessageRecipientService) CountMyUnread(ctx context.Context, _ *emptypb.Empty) (*internalMessageV1.CountMyUnreadResponse, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	count, err := s.uc.CountMyUnread(ctx, operator.UserId)
	if err != nil {
		return nil, err
	}

	return &internalMessageV1.CountMyUnreadResponse{Count: count}, nil
}

// MarkMyRead 标记当前登录用户通知已读（recipient_ids 为空表示全部已读）。
func (s *NotificationMessageRecipientService) MarkMyRead(ctx context.Context, req *internalMessageV1.MarkMyNotificationReadRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	if err = s.uc.MarkMyRead(ctx, operator.UserId, req.GetRecipientIds()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
