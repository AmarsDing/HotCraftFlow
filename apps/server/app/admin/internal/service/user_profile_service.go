package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"

	"framework/pkg/middleware/auth"
)

type UserProfileService struct {
	adminV1.UserProfileServiceHTTPServer

	userUC *biz.UserUsecase

	log *log.Helper
}

func NewUserProfileService(
	logger log.Logger,
	userUC *biz.UserUsecase,
) *UserProfileService {
	l := log.NewHelper(log.With(logger, "module", "user-profile/service/admin-service"))
	return &UserProfileService{
		log:    l,
		userUC: userUC,
	}
}

func (s *UserProfileService) GetUser(ctx context.Context, _ *emptypb.Empty) (*userV1.User, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.userUC.Get(ctx, operator.UserId)
	if err != nil {
		s.log.Errorf("查询用户失败[%s]", err.Error())
		return nil, authenticationV1.ErrorNotFound("user not found")
	}

	return user, err
}

func (s *UserProfileService) UpdateUser(ctx context.Context, req *userV1.UpdateUserRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter: data is required")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	req.Data.Id = trans.Ptr(operator.UserId)

	if err = s.userUC.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}
