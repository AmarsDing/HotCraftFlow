package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

type MenuService struct {
	adminV1.MenuServiceHTTPServer

	log *log.Helper

	uc *biz.MenuUsecase
}

func NewMenuService(logger log.Logger, uc *biz.MenuUsecase) *MenuService {
	l := log.NewHelper(log.With(logger, "module", "menu/service/admin-service"))
	return &MenuService{
		log: l,
		uc:  uc,
	}
}

func (s *MenuService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListMenuResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *MenuService) Get(ctx context.Context, req *adminV1.GetMenuRequest) (*adminV1.Menu, error) {
	return s.uc.Get(ctx, req)
}

func (s *MenuService) Create(ctx context.Context, req *adminV1.CreateMenuRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {

		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *MenuService) Update(ctx context.Context, req *adminV1.UpdateMenuRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// GetImpact 查询菜单影响面（删除/禁用前的前端二次确认数据）。
func (s *MenuService) GetImpact(ctx context.Context, req *adminV1.GetMenuRequest) (*adminV1.MenuImpactResponse, error) {
	return s.uc.GetImpact(ctx, req)
}

// ListRoleRefs 查询全部菜单的角色引用关系（列表徽标 + 角色清单抽屉）。
func (s *MenuService) ListRoleRefs(ctx context.Context, _ *emptypb.Empty) (*adminV1.MenuRoleRefListResponse, error) {
	return s.uc.ListRoleRefs(ctx)
}

func (s *MenuService) Delete(ctx context.Context, req *adminV1.DeleteMenuRequest) (*emptypb.Empty, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.OperatorId = trans.Ptr(operator.UserId)

	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
