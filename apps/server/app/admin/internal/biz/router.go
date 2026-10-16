package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	userV1 "framework/api/gen/go/user/v1"

	"framework/pkg/utils/slice"
)

// RouterUsecase 动态路由业务用例（用户-角色-菜单聚合逻辑自 service 层下移，行为不变）。
type RouterUsecase struct {
	menuRepo MenuRepo
	roleRepo RoleRepo
	userRepo UserRepo
	log      *log.Helper
}

func NewRouterUsecase(menuRepo MenuRepo, roleRepo RoleRepo, userRepo UserRepo, logger log.Logger) *RouterUsecase {
	return &RouterUsecase{
		menuRepo: menuRepo,
		roleRepo: roleRepo,
		userRepo: userRepo,
		log:      log.NewHelper(log.With(logger, "module", "router/usecase/admin-service")),
	}
}

// queryRoleMenus 使用RoleID查询菜单，即单个角色的菜单
func (uc *RouterUsecase) queryRoleMenus(ctx context.Context, roleId uint32) ([]uint32, error) {
	role, err := uc.roleRepo.Get(ctx, roleId)
	if err != nil {
		uc.log.Errorf("query role failed[%s]", err.Error())
		return nil, adminV1.ErrorInternalServerError("query role failed")
	}
	return role.GetMenus(), nil
}

// queryRolesMenus 使用Roles查询菜单，即多个角色的菜单
func (uc *RouterUsecase) queryRolesMenus(ctx context.Context, roles []string) ([]uint32, error) {
	var err error
	var menus []uint32
	var role *userV1.Role
	for _, code := range roles {
		if role, err = uc.roleRepo.GetRoleByCode(ctx, code); err != nil {
			uc.log.Errorf("query role failed [%s] [%s]", code, err.Error())
			continue
		}
		menus = slice.MergeAndDeduplicate(menus, role.GetMenus())
	}

	return menus, nil
}

// ListPermissionCode 查询当前用户全部按钮权限码。
func (uc *RouterUsecase) ListPermissionCode(ctx context.Context, userID uint32) (*adminV1.ListPermissionCodeResponse, error) {
	user, err := uc.userRepo.Get(ctx, userID)
	if err != nil {
		uc.log.Errorf("query user failed[%s]", err.Error())
		return nil, adminV1.ErrorInternalServerError("query user failed")
	}

	// 多角色的菜单
	roleMenus, err := uc.queryRolesMenus(ctx, user.GetRoles())
	if err != nil {
		return nil, err
	}

	// 只取按钮类型菜单（按钮的 meta.authority 携带权限码）
	menus, err := uc.menuRepo.ListByIDs(ctx, toInt32Slice(roleMenus), true)
	if err != nil {
		uc.log.Errorf("list permission code failed [%s]", err.Error())
		return nil, adminV1.ErrorInternalServerError("list permission code failed")
	}

	codes := make([]string, 0)
	for _, m := range menus {
		if m.GetMeta() == nil || len(m.GetMeta().GetAuthority()) == 0 {
			continue
		}
		codes = append(codes, m.GetMeta().GetAuthority()...)
	}

	return &adminV1.ListPermissionCodeResponse{
		Codes: codes,
	}, nil
}

func (uc *RouterUsecase) fillRouteItem(menus []*adminV1.Menu) []*adminV1.RouteItem {
	if len(menus) == 0 {
		return nil
	}

	var routers []*adminV1.RouteItem

	for _, v := range menus {
		if v.GetStatus() != adminV1.MenuStatus_ON {
			continue
		}
		if v.GetType() == adminV1.MenuType_BUTTON {
			continue
		}

		item := &adminV1.RouteItem{
			Path:      v.Path,
			Component: v.Component,
			Name:      v.Name,
			Redirect:  v.Redirect,
			Alias:     v.Alias,
			Meta:      v.Meta,
		}

		if len(v.Children) > 0 {
			item.Children = uc.fillRouteItem(v.Children)
		}

		routers = append(routers, item)
	}

	return routers
}

// ListRoute 查询当前用户的动态路由树。
func (uc *RouterUsecase) ListRoute(ctx context.Context, userID uint32) (*adminV1.ListRouteResponse, error) {
	user, err := uc.userRepo.Get(ctx, userID)
	if err != nil {
		uc.log.Errorf("query user failed[%s]", err.Error())
		return nil, adminV1.ErrorInternalServerError("query user failed")
	}

	// 多角色的菜单
	roleMenus, err := uc.queryRolesMenus(ctx, user.GetRoles())
	if err != nil {
		return nil, err
	}

	// 查询非按钮菜单（已按 meta.order 排序），并在内存构建树
	menus, err := uc.menuRepo.ListByIDs(ctx, toInt32Slice(roleMenus), false)
	if err != nil {
		uc.log.Errorf("list route failed [%s]", err.Error())
		return nil, adminV1.ErrorInternalServerError("list route failed")
	}

	tree := uc.menuRepo.BuildTree(menus)
	resp := &adminV1.ListRouteResponse{Items: uc.fillRouteItem(tree)}

	return resp, nil
}

// toInt32Slice 把 []uint32 转 []int32，菜单 ID 在 proto 中是 int32。
func toInt32Slice(in []uint32) []int32 {
	out := make([]int32, 0, len(in))
	for _, v := range in {
		out = append(out, int32(v))
	}
	return out
}
