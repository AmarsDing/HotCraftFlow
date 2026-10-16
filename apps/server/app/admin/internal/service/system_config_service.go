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

// SystemConfigService 系统参数配置服务层。
// 持有 *biz.SystemConfigUsecase，负责 HTTP 入口鉴权、operator 注入；
// 缓存与业务规则（seed、刷新）下沉到 Usecase。
type SystemConfigService struct {
	adminV1.SystemConfigServiceHTTPServer

	log *log.Helper

	uc *biz.SystemConfigUsecase
}

func NewSystemConfigService(logger log.Logger, uc *biz.SystemConfigUsecase) *SystemConfigService {
	l := log.NewHelper(log.With(logger, "module", "system-config/service/admin-service"))
	return &SystemConfigService{
		log: l,
		uc:  uc,
	}
}

func (s *SystemConfigService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListSystemConfigResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *SystemConfigService) Get(ctx context.Context, req *adminV1.GetSystemConfigRequest) (*adminV1.SystemConfig, error) {
	return s.uc.Get(ctx, req)
}

func (s *SystemConfigService) Create(ctx context.Context, req *adminV1.CreateSystemConfigRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

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

func (s *SystemConfigService) Update(ctx context.Context, req *adminV1.UpdateSystemConfigRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

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

func (s *SystemConfigService) Delete(ctx context.Context, req *adminV1.DeleteSystemConfigRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
