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
	"framework/pkg/middleware/logging"
)

type AdminLoginRestrictionService struct {
	adminV1.AdminLoginRestrictionServiceHTTPServer

	log *log.Helper

	uc *biz.AdminLoginRestrictionUsecase
}

func NewAdminLoginRestrictionService(logger log.Logger, uc *biz.AdminLoginRestrictionUsecase) *AdminLoginRestrictionService {
	l := log.NewHelper(log.With(logger, "module", "admin-login-restriction/service/admin-service"))
	return &AdminLoginRestrictionService{
		log: l,
		uc:  uc,
	}
}

// loginRestrictionOperator 提取操作者上下文（JWT 身份 + 请求来源 IP），供自锁护栏/预演判定。
// 取不到身份的请求（内部调用）返回 nil 操作者，护栏自动跳过。
func loginRestrictionOperator(ctx context.Context) *biz.LoginRestrictionOperator {
	operator, err := auth.FromContext(ctx)
	if err != nil || operator == nil {
		return nil
	}
	return &biz.LoginRestrictionOperator{
		UserID:   operator.GetUserId(),
		Username: operator.GetUsername(),
		ClientIP: logging.ClientIPFromContext(ctx),
	}
}

func (s *AdminLoginRestrictionService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginRestrictionResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *AdminLoginRestrictionService) Get(ctx context.Context, req *adminV1.GetAdminLoginRestrictionRequest) (*adminV1.AdminLoginRestriction, error) {
	return s.uc.Get(ctx, req)
}

func (s *AdminLoginRestrictionService) Create(ctx context.Context, req *adminV1.CreateAdminLoginRestrictionRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req, loginRestrictionOperator(ctx)); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *AdminLoginRestrictionService) Update(ctx context.Context, req *adminV1.UpdateAdminLoginRestrictionRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req, loginRestrictionOperator(ctx)); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *AdminLoginRestrictionService) Delete(ctx context.Context, req *adminV1.DeleteAdminLoginRestrictionRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	if err := s.uc.Delete(ctx, req, loginRestrictionOperator(ctx)); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// Simulate 预演登录限制变更：不落库，返回变更后影响报告（操作者自锁判定/受影响在线会话/规则冲突）。
func (s *AdminLoginRestrictionService) Simulate(ctx context.Context, req *adminV1.SimulateAdminLoginRestrictionRequest) (*adminV1.SimulateAdminLoginRestrictionResponse, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	return s.uc.Simulate(ctx, req, loginRestrictionOperator(ctx))
}
