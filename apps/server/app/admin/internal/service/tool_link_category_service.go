package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

// ToolLinkCategoryService 工具链接分类服务；写操作复用工具链接管理员门禁（super/gly）。
type ToolLinkCategoryService struct {
	adminV1.ToolLinkCategoryServiceHTTPServer

	log *log.Helper

	uc *biz.ToolLinkCategoryUsecase
}

func NewToolLinkCategoryService(logger log.Logger, uc *biz.ToolLinkCategoryUsecase) *ToolLinkCategoryService {
	l := log.NewHelper(log.With(logger, "module", "tool-link-category/service/admin-service"))
	return &ToolLinkCategoryService{
		log: l,
		uc:  uc,
	}
}

func (s *ToolLinkCategoryService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkCategoryResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *ToolLinkCategoryService) Get(ctx context.Context, req *adminV1.GetToolLinkCategoryRequest) (*adminV1.ToolLinkCategory, error) {
	return s.uc.Get(ctx, req)
}

func (s *ToolLinkCategoryService) Create(ctx context.Context, req *adminV1.CreateToolLinkCategoryRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = requireToolLinkAdmin(operator.GetRoles()); err != nil {
		return nil, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ToolLinkCategoryService) Update(ctx context.Context, req *adminV1.UpdateToolLinkCategoryRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = requireToolLinkAdmin(operator.GetRoles()); err != nil {
		return nil, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ToolLinkCategoryService) Delete(ctx context.Context, req *adminV1.DeleteToolLinkCategoryRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = requireToolLinkAdmin(operator.GetRoles()); err != nil {
		return nil, err
	}

	if err = s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
