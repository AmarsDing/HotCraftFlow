package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	userV1 "framework/api/gen/go/user/v1"

	"framework/pkg/middleware/auth"
)

// tenantCodePattern 租户编码格式：字母/数字/中划线/下划线，1-64 位（与 ent schema MaxLen(64) 对齐）。
var tenantCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validateTenantPayload 租户创建/更新的公共入参校验（updateMask 场景仅校验出现的字段）。
func validateTenantPayload(data *userV1.Tenant, isCreate bool) error {
	if isCreate {
		if strings.TrimSpace(data.GetName()) == "" {
			return userV1.ErrorBadRequest("租户名称不能为空")
		}
		if strings.TrimSpace(data.GetCode()) == "" {
			return userV1.ErrorBadRequest("租户编码不能为空")
		}
	}
	if data.Name != nil && strings.TrimSpace(data.GetName()) == "" {
		return userV1.ErrorBadRequest("租户名称不能为空")
	}
	if data.Code != nil {
		if strings.TrimSpace(data.GetCode()) == "" {
			return userV1.ErrorBadRequest("租户编码不能为空")
		}
		if !tenantCodePattern.MatchString(data.GetCode()) {
			return userV1.ErrorBadRequest("租户编码仅支持字母、数字、中划线、下划线，长度1-64位")
		}
	}
	return nil
}

type TenantService struct {
	adminV1.TenantServiceHTTPServer

	log *log.Helper

	uc *biz.TenantUsecase
}

func NewTenantService(logger log.Logger, uc *biz.TenantUsecase) *TenantService {
	l := log.NewHelper(log.With(logger, "module", "tenant/service/admin-service"))
	return &TenantService{
		log: l,
		uc:  uc,
	}
}

func (s *TenantService) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListTenantResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *TenantService) Get(ctx context.Context, req *userV1.GetTenantRequest) (*userV1.Tenant, error) {
	return s.uc.Get(ctx, req)
}

func (s *TenantService) Create(ctx context.Context, req *userV1.CreateTenantRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	// 校验操作者权限：仅系统管理员可创建租户
	if operator.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("权限不够")
	}

	if err = validateTenantPayload(req.Data, true); err != nil {
		return nil, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *TenantService) Update(ctx context.Context, req *userV1.UpdateTenantRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	// 校验操作者权限：仅系统管理员可修改租户
	if operator.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("权限不够")
	}

	if err = validateTenantPayload(req.Data, false); err != nil {
		return nil, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *TenantService) Delete(ctx context.Context, req *userV1.DeleteTenantRequest) (*emptypb.Empty, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	// 校验操作者权限：仅系统管理员可删除租户
	if operator.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("权限不够")
	}

	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
