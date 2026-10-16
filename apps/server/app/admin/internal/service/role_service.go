package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	userV1 "framework/api/gen/go/user/v1"

	"framework/pkg/middleware/auth"
)

type RoleService struct {
	adminV1.RoleServiceHTTPServer

	log *log.Helper

	roleUC     *biz.RoleUsecase
	authorizer biz.AuthorizerPort
}

func NewRoleService(logger log.Logger, roleUC *biz.RoleUsecase, authorizer biz.AuthorizerPort) *RoleService {
	l := log.NewHelper(log.With(logger, "module", "role/service/admin-service"))
	return &RoleService{
		log:        l,
		roleUC:     roleUC,
		authorizer: authorizer,
	}
}

func (s *RoleService) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListRoleResponse, error) {
	return s.roleUC.List(ctx, req)
}

func (s *RoleService) Get(ctx context.Context, req *userV1.GetRoleRequest) (*userV1.Role, error) {
	return s.roleUC.Get(ctx, req.GetId())
}

func (s *RoleService) Create(ctx context.Context, req *userV1.CreateRoleRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.roleUC.Create(ctx, req); err != nil {
		return nil, err
	}

	// 新建角色可能携带 API 绑定（apis 字段），Create 拿不到新 id 无法定点 SyncPolicies，全量重建兜底
	if len(req.Data.GetApis()) > 0 {
		if resetErr := s.authorizer.ResetPolicies(ctx); resetErr != nil {
			s.log.Errorf("reset policies after role create failed (non-fatal): %v", resetErr)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *RoleService) Update(ctx context.Context, req *userV1.UpdateRoleRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.roleUC.Update(ctx, req); err != nil {
		return nil, err
	}

	// 角色信息变更后同步 Casbin 策略（角色 code 变更会改变策略主体）
	roleID := req.GetId()
	if roleID == 0 {
		roleID = req.Data.GetId()
	}
	if roleID != 0 {
		if syncErr := s.authorizer.SyncPolicies(ctx, roleID); syncErr != nil {
			s.log.Errorf("sync policies for role %d failed (non-fatal): %v", roleID, syncErr)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *RoleService) Delete(ctx context.Context, req *userV1.DeleteRoleRequest) (*emptypb.Empty, error) {
	// 删除前查询角色 code，用于清理 Casbin 策略
	role, err := s.roleUC.Get(ctx, req.GetId())
	if err != nil {
		s.log.Warnf("get role %d before delete failed: %v", req.GetId(), err)
	}

	if err := s.roleUC.Delete(ctx, req); err != nil {
		return nil, err
	}

	if role != nil && role.GetCode() != "" {
		if delErr := s.authorizer.DeletePolicies(ctx, role.GetCode()); delErr != nil {
			s.log.Errorf("delete policies for role %s failed (non-fatal): %v", role.GetCode(), delErr)
		}
	}

	return &emptypb.Empty{}, nil
}
