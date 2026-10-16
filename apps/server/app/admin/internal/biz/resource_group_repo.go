package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
)

// ResourceGroupRepo 资源组仓储端口（只读），由 data 层实现。
// 资源组是数据权限（角色-资源组绑定）与分析任务 target_type=resource_group 的基础数据，
// 仅提供列表/详情只读接口；写操作由数据初始化/迁移承担，不开放 RPC。
type ResourceGroupRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListResourceGroupResponse, error)
	Get(ctx context.Context, id uint32) (*adminV1.ResourceGroup, error)
}

// ResourceGroupUsecase 资源组业务用例（只读）。
type ResourceGroupUsecase struct {
	repo ResourceGroupRepo
	log  *log.Helper
}

func NewResourceGroupUsecase(repo ResourceGroupRepo, logger log.Logger) *ResourceGroupUsecase {
	return &ResourceGroupUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "resource-group/usecase/admin-service")),
	}
}

func (uc *ResourceGroupUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListResourceGroupResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *ResourceGroupUsecase) Get(ctx context.Context, req *adminV1.GetResourceGroupRequest) (*adminV1.ResourceGroup, error) {
	if req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter: id is required")
	}
	return uc.repo.Get(ctx, req.GetId())
}
