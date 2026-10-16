package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	entSQL "entgo.io/ent/dialect/sql"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	internalMessageV1 "framework/api/gen/go/internal_message/v1"
)

// PrivateMessageRepo 私信仓储端口，由 data 层 PrivateMessageRepo 实现并经 wire.Bind 绑定。
type PrivateMessageRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListPrivateMessageResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *internalMessageV1.GetPrivateMessageRequest) (*internalMessageV1.PrivateMessage, error)
	Create(ctx context.Context, req *internalMessageV1.CreatePrivateMessageRequest) error
	Update(ctx context.Context, req *internalMessageV1.UpdatePrivateMessageRequest) error
	Delete(ctx context.Context, req *internalMessageV1.DeletePrivateMessageRequest) error
}

// PrivateMessageUsecase 私信业务用例（薄透传）。
type PrivateMessageUsecase struct {
	repo PrivateMessageRepo
	log  *log.Helper
}

func NewPrivateMessageUsecase(repo PrivateMessageRepo, logger log.Logger) *PrivateMessageUsecase {
	return &PrivateMessageUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "private-message/usecase/admin-service")),
	}
}

func (uc *PrivateMessageUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListPrivateMessageResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *PrivateMessageUsecase) Get(ctx context.Context, req *internalMessageV1.GetPrivateMessageRequest) (*internalMessageV1.PrivateMessage, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *PrivateMessageUsecase) Create(ctx context.Context, req *internalMessageV1.CreatePrivateMessageRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *PrivateMessageUsecase) Update(ctx context.Context, req *internalMessageV1.UpdatePrivateMessageRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *PrivateMessageUsecase) Delete(ctx context.Context, req *internalMessageV1.DeletePrivateMessageRequest) error {
	return uc.repo.Delete(ctx, req)
}
