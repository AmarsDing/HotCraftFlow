package biz

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"
	entSQL "entgo.io/ent/dialect/sql"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"github.com/tx7do/go-utils/trans"

	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"
)

// ErrOperationPasswordNotSet 用户未设置独立操作密码（§2.6.6）。
// 调用方应据此返回 422 OPERATION_PASSWORD_NOT_SET，引导用户在个人中心设置。
// 定义在 biz 层以避免 service 引用 data 包；data 层以同名变量别名保持兼容。
var ErrOperationPasswordNotSet = errors.New("operation password not set")

// UserRepo 用户仓储端口，由 data 层 UserRepo 实现并经 wire.Bind 绑定。
// 方法集为 service/biz 各用例实际调用的并集。
type UserRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListUserResponse, error)
	Get(ctx context.Context, userId uint32) (*userV1.User, error)
	GetUserByUserName(ctx context.Context, userName string) (*userV1.User, error)
	// ListDeptIDs 批量查询用户部门（id → dept_id）；未分配部门的用户不出现在返回 map 中。
	// 供在线用户列表等场景补充部门关联（proto User DTO 的 OrgId 与 ent DeptID 字段名不匹配，copier 不自动映射）。
	ListDeptIDs(ctx context.Context, ids []uint32) (map[uint32]uint32, error)
	UserExists(ctx context.Context, req *userV1.UserExistsRequest) (*userV1.UserExistsResponse, error)
	Create(ctx context.Context, req *userV1.CreateUserRequest) (*userV1.User, error)
	Update(ctx context.Context, req *userV1.UpdateUserRequest) error
	Delete(ctx context.Context, userId uint32) error

	// 独立操作密码（§2.6.6 二次鉴权）
	SetOperationPassword(ctx context.Context, userID uint32, newPassword, oldPassword string) error
	VerifyOperationPassword(ctx context.Context, userID uint32, input string) (bool, error)

	// MFA（TOTP）
	GetMFAStatus(ctx context.Context, userID uint32) (enabled bool, secret string, err error)
	SetMFASecret(ctx context.Context, userID uint32, secret string) error
	EnableMFA(ctx context.Context, userID uint32) error
	DisableMFA(ctx context.Context, userID uint32) error

	// 密码策略时间戳
	UpdatePasswordTimestamps(ctx context.Context, userID uint32, expireDays int) error

	// UpdateLastLogin 登录成功后回写最后登录时间与来源 IP（登录链路专用）。
	UpdateLastLogin(ctx context.Context, userID uint32, ip string) error
}

// UserUsecase 用户业务用例。
// 持有 UserRepo 与 UserCredentialRepo（后者用于构造期播种超管初始密码）。
// 权限校验等编排逻辑留在 service 层组合调用，本层薄透传。
type UserUsecase struct {
	repo           UserRepo
	credentialRepo UserCredentialRepo
	log            *log.Helper
}

// NewUserUsecase 创建用户 Usecase。
// 构造期播种超级管理员（原 NewUserService 的副作用，保持同等时机：用户表为空时补建 admin）。
func NewUserUsecase(repo UserRepo, credentialRepo UserCredentialRepo, logger log.Logger) *UserUsecase {
	uc := &UserUsecase{
		repo:           repo,
		credentialRepo: credentialRepo,
		log:            log.NewHelper(log.With(logger, "module", "user/usecase/admin-service")),
	}

	ctx := context.Background()
	if count, _ := uc.repo.Count(ctx, []func(s *entSQL.Selector){}); count == 0 {
		_ = uc.CreateDefaultUser(ctx)
	}

	return uc
}

// List 分页查询用户列表
func (uc *UserUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListUserResponse, error) {
	return uc.repo.List(ctx, req)
}

// Get 按 ID 查询用户
func (uc *UserUsecase) Get(ctx context.Context, userId uint32) (*userV1.User, error) {
	return uc.repo.Get(ctx, userId)
}

// GetUserByUserName 按用户名查询用户
func (uc *UserUsecase) GetUserByUserName(ctx context.Context, userName string) (*userV1.User, error) {
	return uc.repo.GetUserByUserName(ctx, userName)
}

// UserExists 判断用户名是否已存在
func (uc *UserUsecase) UserExists(ctx context.Context, req *userV1.UserExistsRequest) (*userV1.UserExistsResponse, error) {
	return uc.repo.UserExists(ctx, req)
}

// Create 创建用户
func (uc *UserUsecase) Create(ctx context.Context, req *userV1.CreateUserRequest) (*userV1.User, error) {
	return uc.repo.Create(ctx, req)
}

// Update 更新用户
func (uc *UserUsecase) Update(ctx context.Context, req *userV1.UpdateUserRequest) error {
	return uc.repo.Update(ctx, req)
}

// Delete 删除用户
func (uc *UserUsecase) Delete(ctx context.Context, userId uint32) error {
	return uc.repo.Delete(ctx, userId)
}

// SetOperationPassword 设置独立操作密码
func (uc *UserUsecase) SetOperationPassword(ctx context.Context, userID uint32, newPassword, oldPassword string) error {
	return uc.repo.SetOperationPassword(ctx, userID, newPassword, oldPassword)
}

// VerifyOperationPassword 校验独立操作密码
func (uc *UserUsecase) VerifyOperationPassword(ctx context.Context, userID uint32, input string) (bool, error) {
	return uc.repo.VerifyOperationPassword(ctx, userID, input)
}

// GetMFAStatus 查询用户 MFA 状态与密钥
func (uc *UserUsecase) GetMFAStatus(ctx context.Context, userID uint32) (bool, string, error) {
	return uc.repo.GetMFAStatus(ctx, userID)
}

// SetMFASecret 写入 MFA 密钥
func (uc *UserUsecase) SetMFASecret(ctx context.Context, userID uint32, secret string) error {
	return uc.repo.SetMFASecret(ctx, userID, secret)
}

// EnableMFA 启用 MFA
func (uc *UserUsecase) EnableMFA(ctx context.Context, userID uint32) error {
	return uc.repo.EnableMFA(ctx, userID)
}

// DisableMFA 禁用 MFA
func (uc *UserUsecase) DisableMFA(ctx context.Context, userID uint32) error {
	return uc.repo.DisableMFA(ctx, userID)
}

// UpdatePasswordTimestamps 更新密码修改时间与过期时间
func (uc *UserUsecase) UpdatePasswordTimestamps(ctx context.Context, userID uint32, expireDays int) error {
	return uc.repo.UpdatePasswordTimestamps(ctx, userID, expireDays)
}

// CreateDefaultUser 创建默认用户，即超级用户
func (uc *UserUsecase) CreateDefaultUser(ctx context.Context) error {
	var err error

	if _, err = uc.repo.Create(ctx, &userV1.CreateUserRequest{
		Data: &userV1.User{
			Id:        trans.Ptr(uint32(1)),
			Username:  trans.Ptr("admin"),
			Realname:  trans.Ptr("大灰狼"),
			Nickname:  trans.Ptr("鹳狸猿"),
			Region:    trans.Ptr("中国"),
			Email:     trans.Ptr("admin@gmail.com"),
			Authority: userV1.UserAuthority_SYS_ADMIN.Enum(),
			Roles:     []string{"super"},
		},
	}); err != nil {
		uc.log.Errorf("create default user err: %v", err)
		return err
	}

	err = uc.credentialRepo.Create(ctx, &authenticationV1.CreateUserCredentialRequest{
		Data: &authenticationV1.UserCredential{
			UserId:         trans.Ptr(uint32(1)),
			IdentityType:   authenticationV1.IdentityType_USERNAME.Enum(),
			Identifier:     trans.Ptr("admin"),
			CredentialType: authenticationV1.CredentialType_PASSWORD_HASH.Enum(),
			Credential:     trans.Ptr("123456"),
			IsPrimary:      trans.Ptr(true),
			Status:         authenticationV1.UserCredentialStatus_ENABLED.Enum(),
		},
	})
	if err != nil {
		uc.log.Errorf("create default user credential err: %v", err)
		return err
	}

	return err
}
