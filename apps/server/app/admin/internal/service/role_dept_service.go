package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

// RoleDeptService 角色部门关联服务层。
type RoleDeptService struct {
	adminV1.RoleDeptServiceHTTPServer

	log *log.Helper

	uc *biz.RoleDeptUsecase
}

func NewRoleDeptService(logger log.Logger, uc *biz.RoleDeptUsecase) *RoleDeptService {
	l := log.NewHelper(log.With(logger, "module", "role-dept/service/admin-service"))
	return &RoleDeptService{
		log: l,
		uc:  uc,
	}
}

func (s *RoleDeptService) ListByRole(ctx context.Context, req *adminV1.ListRoleDeptByRoleRequest) (*adminV1.ListRoleDeptResponse, error) {
	return s.uc.ListByRole(ctx, req.GetRoleId())
}

func (s *RoleDeptService) BatchBind(ctx context.Context, req *adminV1.BatchBindRoleDeptRequest) (*emptypb.Empty, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	if err = s.uc.BatchBind(ctx, req.GetRoleId(), req.GetDeptIds(), operator.TenantId); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *RoleDeptService) UnbindByRole(ctx context.Context, req *adminV1.UnbindRoleDeptRequest) (*emptypb.Empty, error) {
	if err := s.uc.UnbindByRole(ctx, req.GetRoleId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
