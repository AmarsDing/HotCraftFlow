package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	authnEngine "github.com/tx7do/kratos-authn/engine"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	authenticationV1 "framework/api/gen/go/authentication/v1"

	"framework/pkg/jwt"
	"framework/pkg/middleware/auth"
	"framework/pkg/middleware/logging"
)

type AuthenticationService struct {
	adminV1.AuthenticationServiceHTTPServer

	uc *biz.AuthenticationUsecase


	authenticator authnEngine.Authenticator

	log *log.Helper
}

func NewAuthenticationService(
	logger log.Logger,
	uc *biz.AuthenticationUsecase,
	authenticator authnEngine.Authenticator,
) *AuthenticationService {
	l := log.NewHelper(log.With(logger, "module", "authn/service/admin-service"))
	return &AuthenticationService{
		log:           l,
		uc:            uc,
		authenticator: authenticator,
	}
}

// Login 登录
func (s *AuthenticationService) Login(ctx context.Context, req *authenticationV1.LoginRequest) (*authenticationV1.LoginResponse, error) {
	switch req.GetGrantType() {
	case authenticationV1.GrantType_password.String():
		resp, err := s.uc.LoginByPassword(ctx, req, logging.ClientIPFromContext(ctx), logging.UserAgentFromContext(ctx))
		if err != nil {
			return nil, err
		}
		s.recordUserLoginUsage(ctx, req.GetUsername(), resp)
		return resp, nil

	case authenticationV1.GrantType_refresh_token.String():
		return s.doGrantTypeRefreshToken(ctx, req)

	case authenticationV1.GrantType_client_credentials.String():
		return s.doGrantTypeClientCredentials(ctx, req)

	default:
		return nil, authenticationV1.ErrorInvalidGrantType("invalid grant type")
	}
}


// recordUserLoginUsage skeleton no-op (license module removed).
func (s *AuthenticationService) recordUserLoginUsage(ctx context.Context, username string, resp *authenticationV1.LoginResponse) {
	_ = ctx
	_ = username
	_ = resp
}

// doGrantTypeRefreshToken 处理授权类型 - 刷新令牌
func (s *AuthenticationService) doGrantTypeRefreshToken(ctx context.Context, req *authenticationV1.LoginRequest) (*authenticationV1.LoginResponse, error) {
	// RefreshToken 已入 JWT 白名单（access token 过期后调用，上下文无用户身份），
	// 用户身份由 usecase 通过 refresh_token 记录反查
	return s.uc.LoginByRefreshToken(ctx, req, logging.ClientIPFromContext(ctx), logging.UserAgentFromContext(ctx))
}

// doGrantTypeClientCredentials 处理授权类型 - 客户端凭据
func (s *AuthenticationService) doGrantTypeClientCredentials(_ context.Context, _ *authenticationV1.LoginRequest) (*authenticationV1.LoginResponse, error) {
	return nil, authenticationV1.ErrorInvalidGrantType("invalid grant type")
}

// Logout 登出
func (s *AuthenticationService) Logout(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	if err = s.uc.Logout(ctx, operator.UserId); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// RefreshToken 刷新令牌
func (s *AuthenticationService) RefreshToken(ctx context.Context, req *authenticationV1.LoginRequest) (*authenticationV1.LoginResponse, error) {
	// 校验授权类型
	if req.GetGrantType() != authenticationV1.GrantType_refresh_token.String() {
		return nil, authenticationV1.ErrorInvalidGrantType("invalid grant type")
	}

	return s.doGrantTypeRefreshToken(ctx, req)
}

func (s *AuthenticationService) ValidateToken(_ context.Context, req *authenticationV1.ValidateTokenRequest) (*authenticationV1.ValidateTokenResponse, error) {
	ret, err := s.authenticator.AuthenticateToken(req.GetToken())
	if err != nil {
		return &authenticationV1.ValidateTokenResponse{
			IsValid: false,
		}, err
	}

	claims, err := jwt.NewUserTokenPayloadWithClaims(ret)
	if err != nil {
		return &authenticationV1.ValidateTokenResponse{
			IsValid: false,
		}, err
	}

	return &authenticationV1.ValidateTokenResponse{
		IsValid: true,
		Claim:   claims,
	}, nil
}

func (s *AuthenticationService) RegisterUser(ctx context.Context, req *authenticationV1.RegisterUserRequest) (*authenticationV1.RegisterUserResponse, error) {
	return s.uc.RegisterUser(ctx, req)
}

// ChangePassword 修改用户密码
func (s *AuthenticationService) ChangePassword(ctx context.Context, req *authenticationV1.ChangePasswordRequest) (*emptypb.Empty, error) {
	if err := s.uc.ChangePassword(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// VerifyMFA MFA 二步登录：校验 TOTP 验证码并签发访问令牌
func (s *AuthenticationService) VerifyMFA(ctx context.Context, req *authenticationV1.VerifyMFARequest) (*authenticationV1.LoginResponse, error) {
	resp, err := s.uc.VerifyMFA(ctx, req, logging.ClientIPFromContext(ctx), logging.UserAgentFromContext(ctx))
	if err != nil {
		return nil, err
	}
	s.recordUserLoginUsage(ctx, "", resp)
	return resp, nil
}

// EnableMFA 发起 MFA 绑定：生成 TOTP 密钥与 otpauth URL（仅落密钥，扫码后调 ConfirmMFA 正式启用）
func (s *AuthenticationService) EnableMFA(ctx context.Context, _ *emptypb.Empty) (*authenticationV1.EnableMFAResponse, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	return s.uc.EnableMFA(ctx, operator.UserId)
}

// DisableMFA 禁用 MFA：校验 TOTP 验证码后关闭
func (s *AuthenticationService) DisableMFA(ctx context.Context, req *authenticationV1.DisableMFARequest) (*emptypb.Empty, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.uc.DisableMFA(ctx, operator.UserId, req.GetMfaCode()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ConfirmMFA 确认 MFA 绑定：扫码后校验首个 TOTP 验证码，通过后正式启用
func (s *AuthenticationService) ConfirmMFA(ctx context.Context, req *authenticationV1.ConfirmMFARequest) (*emptypb.Empty, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.uc.ConfirmMFA(ctx, operator.UserId, req.GetMfaCode()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// WhoAmI 获取当前用户身份信息
func (s *AuthenticationService) WhoAmI(ctx context.Context, _ *emptypb.Empty) (*authenticationV1.WhoAmIResponse, error) {
	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	return &authenticationV1.WhoAmIResponse{
		UserId:    operator.GetUserId(),
		Username:  operator.GetUsername(),
		Authority: operator.GetAuthority(),
	}, nil
}
