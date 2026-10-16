package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	userV1 "framework/api/gen/go/user/v1"
)

// DepartmentRepo 部门仓储端口，由 data 层 DepartmentRepo 实现并经 wire.Bind 绑定。
type DepartmentRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListDepartmentResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *userV1.GetDepartmentRequest) (*userV1.Department, error)
	Create(ctx context.Context, req *userV1.CreateDepartmentRequest) error
	Update(ctx context.Context, req *userV1.UpdateDepartmentRequest) error
	Delete(ctx context.Context, req *userV1.DeleteDepartmentRequest) error
}

// DepartmentUsecase 部门业务用例（薄透传）。
type DepartmentUsecase struct {
	repo DepartmentRepo
	log  *log.Helper
}

func NewDepartmentUsecase(repo DepartmentRepo, logger log.Logger) *DepartmentUsecase {
	return &DepartmentUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "department/usecase/admin-service")),
	}
}

func (uc *DepartmentUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListDepartmentResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *DepartmentUsecase) IsExist(ctx context.Context, id uint32) (bool, error) {
	return uc.repo.IsExist(ctx, id)
}

func (uc *DepartmentUsecase) Get(ctx context.Context, req *userV1.GetDepartmentRequest) (*userV1.Department, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *DepartmentUsecase) Create(ctx context.Context, req *userV1.CreateDepartmentRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *DepartmentUsecase) Update(ctx context.Context, req *userV1.UpdateDepartmentRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *DepartmentUsecase) Delete(ctx context.Context, req *userV1.DeleteDepartmentRequest) error {
	return uc.repo.Delete(ctx, req)
}
