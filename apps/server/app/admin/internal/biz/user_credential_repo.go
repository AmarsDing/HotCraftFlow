package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	authenticationV1 "framework/api/gen/go/authentication/v1"
)

// UserCredentialRepo 用户凭据仓储端口，由 data 层 UserCredentialRepo 实现并经 wire.Bind 绑定。
type UserCredentialRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*authenticationV1.ListUserCredentialResponse, error)
	Get(ctx context.Context, req *authenticationV1.GetUserCredentialRequest) (*authenticationV1.UserCredential, error)
	GetByIdentifier(ctx context.Context, req *authenticationV1.GetUserCredentialByIdentifierRequest) (*authenticationV1.UserCredential, error)
	Create(ctx context.Context, req *authenticationV1.CreateUserCredentialRequest) error
	Update(ctx context.Context, req *authenticationV1.UpdateUserCredentialRequest) error
	Delete(ctx context.Context, id uint32) error
	VerifyCredential(ctx context.Context, req *authenticationV1.VerifyCredentialRequest) (*authenticationV1.VerifyCredentialResponse, error)
	ChangeCredential(ctx context.Context, req *authenticationV1.ChangeCredentialRequest) error
	ResetCredential(ctx context.Context, req *authenticationV1.ResetCredentialRequest) error
}

// UserCredentialUsecase 用户凭据业务用例（薄透传，校验编排留 service 层）。
type UserCredentialUsecase struct {
	repo UserCredentialRepo
	log  *log.Helper
}

func NewUserCredentialUsecase(repo UserCredentialRepo, logger log.Logger) *UserCredentialUsecase {
	return &UserCredentialUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "user-credential/usecase/admin-service")),
	}
}

func (uc *UserCredentialUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*authenticationV1.ListUserCredentialResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *UserCredentialUsecase) Get(ctx context.Context, req *authenticationV1.GetUserCredentialRequest) (*authenticationV1.UserCredential, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *UserCredentialUsecase) GetByIdentifier(ctx context.Context, req *authenticationV1.GetUserCredentialByIdentifierRequest) (*authenticationV1.UserCredential, error) {
	return uc.repo.GetByIdentifier(ctx, req)
}

func (uc *UserCredentialUsecase) Create(ctx context.Context, req *authenticationV1.CreateUserCredentialRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *UserCredentialUsecase) Update(ctx context.Context, req *authenticationV1.UpdateUserCredentialRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *UserCredentialUsecase) Delete(ctx context.Context, id uint32) error {
	return uc.repo.Delete(ctx, id)
}

func (uc *UserCredentialUsecase) VerifyCredential(ctx context.Context, req *authenticationV1.VerifyCredentialRequest) (*authenticationV1.VerifyCredentialResponse, error) {
	return uc.repo.VerifyCredential(ctx, req)
}

func (uc *UserCredentialUsecase) ChangeCredential(ctx context.Context, req *authenticationV1.ChangeCredentialRequest) error {
	return uc.repo.ChangeCredential(ctx, req)
}

func (uc *UserCredentialUsecase) ResetCredential(ctx context.Context, req *authenticationV1.ResetCredentialRequest) error {
	return uc.repo.ResetCredential(ctx, req)
}
