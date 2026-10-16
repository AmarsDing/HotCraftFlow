package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	entSQL "entgo.io/ent/dialect/sql"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	internalMessageV1 "framework/api/gen/go/internal_message/v1"
)

// NotificationMessageRecipientRepo 通知消息收件人仓储端口，由 data 层 NotificationMessageRecipientRepo 实现并经 wire.Bind 绑定。
type NotificationMessageRecipientRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageRecipientResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRecipientRequest) (*internalMessageV1.NotificationMessageRecipient, error)
	Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRecipientRequest) error
	Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRecipientRequest) error
	Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRecipientRequest) error

	// ListMy 查询当前登录用户的通知收件箱（收件记录并联消息标题/内容）。
	ListMy(ctx context.Context, userID uint32, req *internalMessageV1.ListMyNotificationRequest) (*internalMessageV1.ListMyNotificationResponse, error)
	// CountMyUnread 查询当前登录用户未读通知数。
	CountMyUnread(ctx context.Context, userID uint32) (uint32, error)
	// MarkMyRead 标记当前登录用户通知已读（recipientIDs 为空表示全部已读）。
	MarkMyRead(ctx context.Context, userID uint32, recipientIDs []uint32) error
}

// NotificationMessageRecipientUsecase 通知消息收件人业务用例（薄透传）。
type NotificationMessageRecipientUsecase struct {
	repo NotificationMessageRecipientRepo
	log  *log.Helper
}

func NewNotificationMessageRecipientUsecase(repo NotificationMessageRecipientRepo, logger log.Logger) *NotificationMessageRecipientUsecase {
	return &NotificationMessageRecipientUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "notification-message-recipient/usecase/admin-service")),
	}
}

func (uc *NotificationMessageRecipientUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageRecipientResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *NotificationMessageRecipientUsecase) Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRecipientRequest) (*internalMessageV1.NotificationMessageRecipient, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *NotificationMessageRecipientUsecase) Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRecipientRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *NotificationMessageRecipientUsecase) Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRecipientRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *NotificationMessageRecipientUsecase) Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRecipientRequest) error {
	return uc.repo.Delete(ctx, req)
}

func (uc *NotificationMessageRecipientUsecase) ListMy(ctx context.Context, userID uint32, req *internalMessageV1.ListMyNotificationRequest) (*internalMessageV1.ListMyNotificationResponse, error) {
	return uc.repo.ListMy(ctx, userID, req)
}

func (uc *NotificationMessageRecipientUsecase) CountMyUnread(ctx context.Context, userID uint32) (uint32, error) {
	return uc.repo.CountMyUnread(ctx, userID)
}

func (uc *NotificationMessageRecipientUsecase) MarkMyRead(ctx context.Context, userID uint32, recipientIDs []uint32) error {
	return uc.repo.MarkMyRead(ctx, userID, recipientIDs)
}
