package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	userV1 "framework/api/gen/go/user/v1"
)

// RoleRepo 角色仓储端口，由 data 层 RoleRepo 实现并经 wire.Bind 绑定。
type RoleRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListRoleResponse, error)
	Get(ctx context.Context, id uint32) (*userV1.Role, error)
	GetRoleByCode(ctx context.Context, code string) (*userV1.Role, error)
	Create(ctx context.Context, req *userV1.CreateRoleRequest) error
	Update(ctx context.Context, req *userV1.UpdateRoleRequest) error
	Delete(ctx context.Context, req *userV1.DeleteRoleRequest) error
	// EnsureMenus 确保角色-菜单关联存在（播种用，幂等）
	EnsureMenus(ctx context.Context, roleID uint32, menuIDs []uint32) error
	// EnsureApis 全量同步角色-API资源关联（播种用，幂等）
	EnsureApis(ctx context.Context, roleID uint32, apiIDs []uint32) error
	// EnsureUserRole 确保用户-角色关联存在（播种与导入用，幂等）
	EnsureUserRole(ctx context.Context, userID, roleID uint32) error
}

// RoleUsecase 角色业务用例（薄透传，策略同步编排留 service 层）。
type RoleUsecase struct {
	repo RoleRepo
	log  *log.Helper
}

func NewRoleUsecase(repo RoleRepo, logger log.Logger) *RoleUsecase {
	return &RoleUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "role/usecase/admin-service")),
	}
}

func (uc *RoleUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListRoleResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *RoleUsecase) Get(ctx context.Context, id uint32) (*userV1.Role, error) {
	return uc.repo.Get(ctx, id)
}

func (uc *RoleUsecase) GetRoleByCode(ctx context.Context, code string) (*userV1.Role, error) {
	return uc.repo.GetRoleByCode(ctx, code)
}

func (uc *RoleUsecase) Create(ctx context.Context, req *userV1.CreateRoleRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *RoleUsecase) Update(ctx context.Context, req *userV1.UpdateRoleRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *RoleUsecase) Delete(ctx context.Context, req *userV1.DeleteRoleRequest) error {
	return uc.repo.Delete(ctx, req)
}

func (uc *RoleUsecase) EnsureMenus(ctx context.Context, roleID uint32, menuIDs []uint32) error {
	return uc.repo.EnsureMenus(ctx, roleID, menuIDs)
}

// AddMenus 增量给角色补挂菜单（存量 ∪ 新增，去重后全量替换写回）。
// 与 EnsureMenus 的区别：EnsureMenus 以入参为准全量替换（会还原管理页调整），
// AddMenus 只增不删，用于 seed 差量补齐把新增菜单挂到已存在角色上。
// 返回实际新增的菜单数。
func (uc *RoleUsecase) AddMenus(ctx context.Context, roleID uint32, menuIDs []uint32) (int, error) {
	if len(menuIDs) == 0 {
		return 0, nil
	}
	role, err := uc.repo.Get(ctx, roleID)
	if err != nil {
		return 0, err
	}
	seen := make(map[uint32]bool, len(role.GetMenus())+len(menuIDs))
	merged := make([]uint32, 0, len(role.GetMenus())+len(menuIDs))
	for _, id := range role.GetMenus() {
		if !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	added := 0
	for _, id := range menuIDs {
		if !seen[id] {
			seen[id] = true
			merged = append(merged, id)
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	if err := uc.repo.EnsureMenus(ctx, roleID, merged); err != nil {
		return 0, err
	}
	return added, nil
}

func (uc *RoleUsecase) EnsureApis(ctx context.Context, roleID uint32, apiIDs []uint32) error {
	return uc.repo.EnsureApis(ctx, roleID, apiIDs)
}

func (uc *RoleUsecase) EnsureUserRole(ctx context.Context, userID, roleID uint32) error {
	return uc.repo.EnsureUserRole(ctx, userID, roleID)
}
