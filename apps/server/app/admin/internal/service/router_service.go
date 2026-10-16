package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

type RouterService struct {
	adminV1.RouterServiceHTTPServer

	log *log.Helper

	uc *biz.RouterUsecase
}

func NewRouterService(
	logger log.Logger,
	uc *biz.RouterUsecase,
) *RouterService {
	l := log.NewHelper(log.With(logger, "module", "router/service/admin-service"))
	return &RouterService{
		log: l,
		uc:  uc,
	}
}

func (s *RouterService) ListPermissionCode(ctx context.Context, _ *emptypb.Empty) (*adminV1.ListPermissionCodeResponse, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	return s.uc.ListPermissionCode(ctx, operator.UserId)
}

func (s *RouterService) ListRoute(ctx context.Context, _ *emptypb.Empty) (*adminV1.ListRouteResponse, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	return s.uc.ListRoute(ctx, operator.UserId)
}
