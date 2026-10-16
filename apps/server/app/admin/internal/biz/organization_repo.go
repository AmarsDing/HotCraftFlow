package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	userV1 "framework/api/gen/go/user/v1"
)

// OrganizationRepo 组织仓储端口，由 data 层 OrganizationRepo 实现并经 wire.Bind 绑定。
type OrganizationRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListOrganizationResponse, error)
	Get(ctx context.Context, req *userV1.GetOrganizationRequest) (*userV1.Organization, error)
	Create(ctx context.Context, req *userV1.CreateOrganizationRequest) error
	Update(ctx context.Context, req *userV1.UpdateOrganizationRequest) error
	Delete(ctx context.Context, req *userV1.DeleteOrganizationRequest) error
}

// OrganizationUsecase 组织业务用例（薄透传）。
type OrganizationUsecase struct {
	repo OrganizationRepo
	log  *log.Helper
}

func NewOrganizationUsecase(repo OrganizationRepo, logger log.Logger) *OrganizationUsecase {
	return &OrganizationUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "organization/usecase/admin-service")),
	}
}

func (uc *OrganizationUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListOrganizationResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *OrganizationUsecase) Get(ctx context.Context, req *userV1.GetOrganizationRequest) (*userV1.Organization, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *OrganizationUsecase) Create(ctx context.Context, req *userV1.CreateOrganizationRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *OrganizationUsecase) Update(ctx context.Context, req *userV1.UpdateOrganizationRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *OrganizationUsecase) Delete(ctx context.Context, req *userV1.DeleteOrganizationRequest) error {
	return uc.repo.Delete(ctx, req)
}
