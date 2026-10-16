package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
)

type AdminLoginLogService struct {
	adminV1.AdminLoginLogServiceHTTPServer

	log *log.Helper

	uc *biz.AdminLoginLogUsecase
}

func NewAdminLoginLogService(logger log.Logger, uc *biz.AdminLoginLogUsecase) *AdminLoginLogService {
	l := log.NewHelper(log.With(logger, "module", "admin-login-log/service/admin-service"))
	return &AdminLoginLogService{
		log: l,
		uc:  uc,
	}
}

func (s *AdminLoginLogService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginLogResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *AdminLoginLogService) Get(ctx context.Context, req *adminV1.GetAdminLoginLogRequest) (*adminV1.AdminLoginLog, error) {
	return s.uc.Get(ctx, req)
}

func (s *AdminLoginLogService) Clean(ctx context.Context, req *adminV1.CleanAdminLoginLogRequest) (*adminV1.CleanAdminLoginLogResponse, error) {
	return s.uc.Clean(ctx, req)
}
