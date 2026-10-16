package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

// DataPermissionService 数据权限规则服务层。
type DataPermissionService struct {
	adminV1.DataPermissionServiceHTTPServer

	log *log.Helper

	uc *biz.DataPermissionUsecase
}

func NewDataPermissionService(logger log.Logger, uc *biz.DataPermissionUsecase) *DataPermissionService {
	l := log.NewHelper(log.With(logger, "module", "data-permission/service/admin-service"))
	return &DataPermissionService{
		log: l,
		uc:  uc,
	}
}

func (s *DataPermissionService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDataPermissionResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *DataPermissionService) Get(ctx context.Context, req *adminV1.GetDataPermissionRequest) (*adminV1.DataPermission, error) {
	return s.uc.Get(ctx, req)
}

func (s *DataPermissionService) Create(ctx context.Context, req *adminV1.CreateDataPermissionRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	// 数据权限规则表无 create_by 字段（schema Mixin 未含 CreateBy），tenant_id 由 repo 处理
	req.Data.TenantId = operator.TenantId

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *DataPermissionService) Update(ctx context.Context, req *adminV1.UpdateDataPermissionRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	req.Data.TenantId = operator.TenantId

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *DataPermissionService) Delete(ctx context.Context, req *adminV1.DeleteDataPermissionRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
