package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	userV1 "framework/api/gen/go/user/v1"
)

// PositionRepo 岗位仓储端口，由 data 层 PositionRepo 实现并经 wire.Bind 绑定。
type PositionRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListPositionResponse, error)
	Get(ctx context.Context, req *userV1.GetPositionRequest) (*userV1.Position, error)
	Create(ctx context.Context, req *userV1.CreatePositionRequest) error
	Update(ctx context.Context, req *userV1.UpdatePositionRequest) error
	Delete(ctx context.Context, req *userV1.DeletePositionRequest) error
}

// PositionUsecase 岗位业务用例（薄透传）。
type PositionUsecase struct {
	repo PositionRepo
	log  *log.Helper
}

func NewPositionUsecase(repo PositionRepo, logger log.Logger) *PositionUsecase {
	return &PositionUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "position/usecase/admin-service")),
	}
}

func (uc *PositionUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListPositionResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *PositionUsecase) Get(ctx context.Context, req *userV1.GetPositionRequest) (*userV1.Position, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *PositionUsecase) Create(ctx context.Context, req *userV1.CreatePositionRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *PositionUsecase) Update(ctx context.Context, req *userV1.UpdatePositionRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *PositionUsecase) Delete(ctx context.Context, req *userV1.DeletePositionRequest) error {
	return uc.repo.Delete(ctx, req)
}
