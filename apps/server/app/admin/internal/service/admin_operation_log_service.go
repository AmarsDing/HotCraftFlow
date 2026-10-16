package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
)

type AdminOperationLogService struct {
	adminV1.AdminOperationLogServiceHTTPServer

	log *log.Helper

	uc *biz.AdminOperationLogUsecase
}

func NewAdminOperationLogService(
	logger log.Logger,
	uc *biz.AdminOperationLogUsecase,
) *AdminOperationLogService {
	l := log.NewHelper(log.With(logger, "module", "admin-operation-log/service/admin-service"))
	return &AdminOperationLogService{
		log: l,
		uc:  uc,
	}
}

func (s *AdminOperationLogService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminOperationLogResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *AdminOperationLogService) Get(ctx context.Context, req *adminV1.GetAdminOperationLogRequest) (*adminV1.AdminOperationLog, error) {
	return s.uc.Get(ctx, req)
}

func (s *AdminOperationLogService) Clean(ctx context.Context, req *adminV1.CleanAdminOperationLogRequest) (*adminV1.CleanAdminOperationLogResponse, error) {
	return s.uc.Clean(ctx, req)
}
