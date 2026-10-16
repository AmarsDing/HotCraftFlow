package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
)

// UserPermissionService 用户数据权限查询服务层。
// 供 06-query 等跨模块服务查询指定用户的数据权限范围（scope + 部门 + 资源组）。
type UserPermissionService struct {
	adminV1.UserPermissionServiceHTTPServer

	log *log.Helper

	uc *biz.DataPermissionUsecase
}

func NewUserPermissionService(logger log.Logger, uc *biz.DataPermissionUsecase) *UserPermissionService {
	l := log.NewHelper(log.With(logger, "module", "user-permission/service/admin-service"))
	return &UserPermissionService{
		log: l,
		uc:  uc,
	}
}

func (s *UserPermissionService) GetUserPermissions(ctx context.Context, req *adminV1.GetUserPermissionsRequest) (*adminV1.UserPermissions, error) {
	return s.uc.GetUserPermissions(ctx, req)
}
