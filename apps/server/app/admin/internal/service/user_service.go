package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"

	"framework/pkg/middleware/auth"
)

type UserService struct {
	adminV1.UserServiceHTTPServer

	log *log.Helper

	userUC           *biz.UserUsecase
	userCredentialUC *biz.UserCredentialUsecase
}

func NewUserService(
	logger log.Logger,
	userUC *biz.UserUsecase,
	userCredentialUC *biz.UserCredentialUsecase,
) *UserService {
	l := log.NewHelper(log.With(logger, "module", "user/service/admin-service"))
	svc := &UserService{
		log:              l,
		userUC:           userUC,
		userCredentialUC: userCredentialUC,
	}

	// 超级管理员播种已上移至 biz.NewUserUsecase 构造期完成（保持同等时机），此处不再重复。

	return svc
}

func (s *UserService) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListUserResponse, error) {
	return s.userUC.List(ctx, req)
}

func (s *UserService) Get(ctx context.Context, req *userV1.GetUserRequest) (*userV1.User, error) {
	user, err := s.userUC.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) GetUserByUserName(ctx context.Context, req *userV1.GetUserByUserNameRequest) (*userV1.User, error) {
	user, err := s.userUC.GetUserByUserName(ctx, req.GetUsername())
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) Create(ctx context.Context, req *userV1.CreateUserRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	// 获取操作者的用户信息
	operatorUser, err := s.userUC.Get(ctx, operator.UserId)
	if err != nil {
		return nil, err
	}

	// 校验操作者的权限
	if operatorUser.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("权限不够")
	}

	if req.Data.Authority == nil {
		req.Data.Authority = userV1.UserAuthority_CUSTOMER_USER.Enum()
	}

	if req.Data.Authority != nil {
		if operatorUser.GetAuthority() < req.Data.GetAuthority() {
			return nil, adminV1.ErrorForbidden("不能够创建同级用户或者比自己权限高的用户")
		}
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	// 创建用户
	var user *userV1.User
	if user, err = s.userUC.Create(ctx, req); err != nil {
		s.log.Error(err)
		return nil, err
	}

	if len(req.GetPassword()) == 0 {
		// 如果没有设置密码，则默认设置为 666666
		req.Password = trans.Ptr("666666")
	}

	if len(req.GetPassword()) > 0 {
		if err = s.userCredentialUC.Create(ctx, &authenticationV1.CreateUserCredentialRequest{
			Data: &authenticationV1.UserCredential{
				UserId:   user.Id,
				TenantId: user.TenantId,

				IdentityType: authenticationV1.IdentityType_USERNAME.Enum(),
				Identifier:   req.Data.Username,

				CredentialType: authenticationV1.CredentialType_PASSWORD_HASH.Enum(),
				Credential:     req.Password,

				IsPrimary: trans.Ptr(true),
				Status:    authenticationV1.UserCredentialStatus_ENABLED.Enum(),
			},
		}); err != nil {
			return nil, err
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *UserService) Update(ctx context.Context, req *userV1.UpdateUserRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	// 获取操作者的用户信息
	operatorUser, err := s.userUC.Get(ctx, operator.UserId)
	if err != nil {
		return nil, err
	}

	// 校验操作者的权限
	if operatorUser.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("权限不够")
	}

	if req.Data.Authority != nil {
		if operatorUser.GetAuthority() < req.Data.GetAuthority() {
			return nil, adminV1.ErrorForbidden("不能够赋权同级用户或者比自己权限高的用户")
		}
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	// 更新用户
	if err = s.userUC.Update(ctx, req); err != nil {
		s.log.Error(err)
		return nil, err
	}

	if len(req.GetPassword()) > 0 {
		if err = s.userCredentialUC.ResetCredential(ctx, &authenticationV1.ResetCredentialRequest{
			IdentityType:  authenticationV1.IdentityType_USERNAME,
			Identifier:    req.Data.GetUsername(),
			NewCredential: req.GetPassword(),
		}); err != nil {
			return nil, err
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *UserService) Delete(ctx context.Context, req *userV1.DeleteUserRequest) (*emptypb.Empty, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	// 获取操作者的用户信息
	operatorUser, err := s.userUC.Get(ctx, operator.UserId)
	if err != nil {
		return nil, err
	}

	// 校验操作者的权限
	if operatorUser.GetAuthority() != userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("权限不够")
	}

	// 获取将被删除的用户信息
	user, err := s.userUC.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	// 不能删除超级管理员
	if user.GetAuthority() == userV1.UserAuthority_SYS_ADMIN {
		return nil, adminV1.ErrorForbidden("闹哪样？不能删除超级管理员！")
	}

	if operatorUser.GetAuthority() == user.GetAuthority() {
		return nil, adminV1.ErrorForbidden("不能删除同级用户！")
	}

	// 删除用户
	err = s.userUC.Delete(ctx, req.GetId())

	return &emptypb.Empty{}, err
}

func (s *UserService) UserExists(ctx context.Context, req *userV1.UserExistsRequest) (*userV1.UserExistsResponse, error) {
	return s.userUC.UserExists(ctx, req)
}

// SetOperationPassword 设置当前登录用户的独立操作密码（§2.6.6 二次鉴权前置）。
// 与登录密码独立，AES-256-GCM 加密存储 6 位数字明文。
// 已设置过时必须校验旧操作密码；首次设置时 oldOperationPassword 可为空。
func (s *UserService) SetOperationPassword(ctx context.Context, req *adminV1.SetOperationPasswordRequest) (*emptypb.Empty, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.userUC.SetOperationPassword(ctx, operator.UserId, req.GetOperationPassword(), req.GetOldOperationPassword()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
