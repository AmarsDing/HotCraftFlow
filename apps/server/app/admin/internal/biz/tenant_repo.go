package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	userV1 "framework/api/gen/go/user/v1"
)

// TenantRepo 租户仓储端口，由 data 层 TenantRepo 实现并经 wire.Bind 绑定。
type TenantRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListTenantResponse, error)
	Get(ctx context.Context, req *userV1.GetTenantRequest) (*userV1.Tenant, error)
	Create(ctx context.Context, req *userV1.CreateTenantRequest) error
	Update(ctx context.Context, req *userV1.UpdateTenantRequest) error
	Delete(ctx context.Context, req *userV1.DeleteTenantRequest) error
	GetTenantByTenantCode(ctx context.Context, code string) (*userV1.Tenant, error)
}

// TenantUsecase 租户业务用例（薄透传）。
type TenantUsecase struct {
	repo TenantRepo
	log  *log.Helper
}

func NewTenantUsecase(repo TenantRepo, logger log.Logger) *TenantUsecase {
	return &TenantUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "tenant/usecase/admin-service")),
	}
}

func (uc *TenantUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListTenantResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *TenantUsecase) Get(ctx context.Context, req *userV1.GetTenantRequest) (*userV1.Tenant, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *TenantUsecase) Create(ctx context.Context, req *userV1.CreateTenantRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *TenantUsecase) Update(ctx context.Context, req *userV1.UpdateTenantRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *TenantUsecase) Delete(ctx context.Context, req *userV1.DeleteTenantRequest) error {
	return uc.repo.Delete(ctx, req)
}

func (uc *TenantUsecase) GetTenantByTenantCode(ctx context.Context, code string) (*userV1.Tenant, error) {
	return uc.repo.GetTenantByTenantCode(ctx, code)
}
