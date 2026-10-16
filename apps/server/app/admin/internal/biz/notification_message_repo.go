package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	entSQL "entgo.io/ent/dialect/sql"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	internalMessageV1 "framework/api/gen/go/internal_message/v1"
)

// NotificationMessageRepo 通知消息仓储端口，由 data 层 NotificationMessageRepo 实现并经 wire.Bind 绑定。
type NotificationMessageRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRequest) (*internalMessageV1.NotificationMessage, error)
	Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRequest) error
	Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRequest) error
	Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRequest) error
}

// NotificationMessageUsecase 通知消息业务用例（薄透传）。
type NotificationMessageUsecase struct {
	repo NotificationMessageRepo
	log  *log.Helper
}

func NewNotificationMessageUsecase(repo NotificationMessageRepo, logger log.Logger) *NotificationMessageUsecase {
	return &NotificationMessageUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "notification-message/usecase/admin-service")),
	}
}

func (uc *NotificationMessageUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *NotificationMessageUsecase) Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRequest) (*internalMessageV1.NotificationMessage, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *NotificationMessageUsecase) Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *NotificationMessageUsecase) Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *NotificationMessageUsecase) Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRequest) error {
	return uc.repo.Delete(ctx, req)
}
