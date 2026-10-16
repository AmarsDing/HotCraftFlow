package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	authenticationV1 "framework/api/gen/go/authentication/v1"
)

type UserCredentialService struct {
	authenticationV1.UnimplementedUserCredentialServiceServer

	log *log.Helper

	userCredentialUC *biz.UserCredentialUsecase
}

func NewUserCredentialService(
	logger log.Logger,
	userCredentialUC *biz.UserCredentialUsecase,
) *UserCredentialService {
	l := log.NewHelper(log.With(logger, "module", "user-credential/service/admin-service"))
	return &UserCredentialService{
		log:              l,
		userCredentialUC: userCredentialUC,
	}
}

func (s *UserCredentialService) List(ctx context.Context, req *pagination.PagingRequest) (*authenticationV1.ListUserCredentialResponse, error) {
	return s.userCredentialUC.List(ctx, req)
}

func (s *UserCredentialService) Get(ctx context.Context, req *authenticationV1.GetUserCredentialRequest) (*authenticationV1.UserCredential, error) {
	return s.userCredentialUC.Get(ctx, req)
}

func (s *UserCredentialService) GetByIdentifier(ctx context.Context, req *authenticationV1.GetUserCredentialByIdentifierRequest) (*authenticationV1.UserCredential, error) {
	return s.userCredentialUC.GetByIdentifier(ctx, req)
}

func (s *UserCredentialService) Create(ctx context.Context, req *authenticationV1.CreateUserCredentialRequest) (*emptypb.Empty, error) {
	if err := s.userCredentialUC.Create(ctx, req); err != nil {
		// s.log.Info(err)
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *UserCredentialService) Update(ctx context.Context, req *authenticationV1.UpdateUserCredentialRequest) (*emptypb.Empty, error) {
	if err := s.userCredentialUC.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *UserCredentialService) Delete(ctx context.Context, req *authenticationV1.DeleteUserCredentialRequest) (*emptypb.Empty, error) {
	if err := s.userCredentialUC.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *UserCredentialService) VerifyCredential(ctx context.Context, req *authenticationV1.VerifyCredentialRequest) (*authenticationV1.VerifyCredentialResponse, error) {
	return s.userCredentialUC.VerifyCredential(ctx, req)
}

func (s *UserCredentialService) ChangeCredential(ctx context.Context, req *authenticationV1.ChangeCredentialRequest) (*emptypb.Empty, error) {
	err := s.userCredentialUC.ChangeCredential(ctx, req)
	return &emptypb.Empty{}, err
}

func (s *UserCredentialService) ResetCredential(ctx context.Context, req *authenticationV1.ResetCredentialRequest) (*emptypb.Empty, error) {
	err := s.userCredentialUC.ResetCredential(ctx, req)
	return &emptypb.Empty{}, err
}
