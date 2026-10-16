package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	entSQL "entgo.io/ent/dialect/sql"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	internalMessageV1 "framework/api/gen/go/internal_message/v1"
)

// NotificationMessageCategoryRepo 通知消息分类仓储端口，由 data 层 NotificationMessageCategoryRepo 实现并经 wire.Bind 绑定。
type NotificationMessageCategoryRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageCategoryResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageCategoryRequest) (*internalMessageV1.NotificationMessageCategory, error)
	Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageCategoryRequest) error
	Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageCategoryRequest) error
	Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageCategoryRequest) error
}

// NotificationMessageCategoryUsecase 通知消息分类业务用例（薄透传）。
type NotificationMessageCategoryUsecase struct {
	repo NotificationMessageCategoryRepo
	log  *log.Helper
}

func NewNotificationMessageCategoryUsecase(repo NotificationMessageCategoryRepo, logger log.Logger) *NotificationMessageCategoryUsecase {
	return &NotificationMessageCategoryUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "notification-message-category/usecase/admin-service")),
	}
}

func (uc *NotificationMessageCategoryUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageCategoryResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *NotificationMessageCategoryUsecase) Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageCategoryRequest) (*internalMessageV1.NotificationMessageCategory, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *NotificationMessageCategoryUsecase) Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageCategoryRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *NotificationMessageCategoryUsecase) Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageCategoryRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *NotificationMessageCategoryUsecase) Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageCategoryRequest) error {
	return uc.repo.Delete(ctx, req)
}
