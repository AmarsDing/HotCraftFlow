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

// DictService 字典服务层。
// 持有 *biz.DictUsecase（不直接持有 data repo），负责 HTTP 入口鉴权、
// operator 注入、DTO 组装；业务规则下沉到 Usecase。
type DictService struct {
	adminV1.DictServiceHTTPServer

	log *log.Helper

	uc *biz.DictUsecase
}

func NewDictService(logger log.Logger, uc *biz.DictUsecase) *DictService {
	l := log.NewHelper(log.With(logger, "module", "dict/service/admin-service"))
	return &DictService{
		log: l,
		uc:  uc,
	}
}

func (s *DictService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDictResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *DictService) Get(ctx context.Context, req *adminV1.GetDictRequest) (*adminV1.Dict, error) {
	return s.uc.Get(ctx, req)
}

func (s *DictService) Create(ctx context.Context, req *adminV1.CreateDictRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息并注入审计字段（HTTP 入口鉴权延伸，业务规则在 Usecase）
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

func (s *DictService) Update(ctx context.Context, req *adminV1.UpdateDictRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息并注入审计字段
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

func (s *DictService) Delete(ctx context.Context, req *adminV1.DeleteDictRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// ListCategories 查询字典分类列表（含条目数/消费方/内置标记，P2 分类实体化）
func (s *DictService) ListCategories(ctx context.Context, req *adminV1.ListDictCategoryRequest) (*adminV1.ListDictCategoryResponse, error) {
	return s.uc.ListCategories(ctx)
}

// CreateCategory 创建字典分类（仅非内置编码；内置分类由服务端种子自动注册）
func (s *DictService) CreateCategory(ctx context.Context, req *adminV1.CreateDictCategoryRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.CreateCategory(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// UpdateCategory 更新字典分类（名称/排序/备注；编码为身份不可改，内置分类用途锁定）
func (s *DictService) UpdateCategory(ctx context.Context, req *adminV1.UpdateDictCategoryRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}
	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.UpdateCategory(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// DeleteCategory 删除字典分类（仅允许非内置且空分类）
func (s *DictService) DeleteCategory(ctx context.Context, req *adminV1.DeleteDictCategoryRequest) (*emptypb.Empty, error) {
	if err := s.uc.DeleteCategory(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
