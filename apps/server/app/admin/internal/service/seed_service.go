package service

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"
)

// SeedService boots platform menus/roles/users for the skeleton.
type SeedService struct {
	log *log.Helper

	menuUC           *biz.MenuUsecase
	roleUC           *biz.RoleUsecase
	userUC           *biz.UserUsecase
	userCredentialUC *biz.UserCredentialUsecase
	apiResourceUC    *biz.ApiResourceUsecase
	departmentUC     *biz.DepartmentUsecase
	organizationUC   *biz.OrganizationUsecase
	positionUC       *biz.PositionUsecase
	dictUC           *biz.DictUsecase
	authorizer       biz.AuthorizerPort
}

func NewSeedService(
	logger log.Logger,
	menuUC *biz.MenuUsecase,
	roleUC *biz.RoleUsecase,
	userUC *biz.UserUsecase,
	userCredentialUC *biz.UserCredentialUsecase,
	apiResourceUC *biz.ApiResourceUsecase,
	departmentUC *biz.DepartmentUsecase,
	organizationUC *biz.OrganizationUsecase,
	positionUC *biz.PositionUsecase,
	dictUC *biz.DictUsecase,
	authorizer biz.AuthorizerPort,
) *SeedService {
	svc := &SeedService{
		log:              log.NewHelper(log.With(logger, "module", "seed/service/admin-service")),
		menuUC:           menuUC,
		roleUC:           roleUC,
		userUC:           userUC,
		userCredentialUC: userCredentialUC,
		apiResourceUC:    apiResourceUC,
		departmentUC:     departmentUC,
		organizationUC:   organizationUC,
		positionUC:       positionUC,
		dictUC:           dictUC,
		authorizer:       authorizer,
	}
	ctx := context.Background()
	if err := svc.seedAll(ctx); err != nil {
		svc.log.Errorf("seed data failed: %v", err)
	}
	return svc
}

type seedMenuRecord struct {
	id        int32
	parentID  int32
	mType     adminV1.MenuType
	path      string
	redirect  string
	name      string
	component string
	title     string
	icon      string
	order     int32
}

func (s *SeedService) seedAll(ctx context.Context) error {
	menuCount, err := s.menuUC.CountAll(ctx)
	if err != nil {
		return err
	}
	if menuCount == 0 {
		if err := s.seedMenus(ctx); err != nil {
			return err
		}
	} else {
		s.log.Infof("menus already seeded (%d rows), skip full menu seed", menuCount)
	}

	if err := s.menuUC.ResetSequence(ctx); err != nil {
		s.log.Errorf("reset menu id sequence failed: %v", err)
	}

	apiIds := s.allApiIds(ctx)
	if err := s.seedRoles(ctx, apiIds); err != nil {
		return err
	}
	if err := s.seedOrganizations(ctx); err != nil {
		s.log.Errorf("seed organizations failed: %v", err)
	}
	if err := s.seedDepartments(ctx); err != nil {
		return err
	}
	if err := s.seedPositions(ctx); err != nil {
		s.log.Errorf("seed positions failed: %v", err)
	}
	if err := s.seedUsers(ctx); err != nil {
		return err
	}
	_ = s.seedUserRoles(ctx)

	if err := s.dictUC.SeedDictCategories(ctx); err != nil {
		s.log.Errorf("seed dict categories failed: %v", err)
	}

	if s.authorizer != nil {
		if err := s.authorizer.ResetPolicies(ctx); err != nil {
			s.log.Errorf("reset policies after seed failed: %v", err)
		}
	}
	return nil
}

func platformMenuSeedRecords() []seedMenuRecord {
	return []seedMenuRecord{
		{id: 1, mType: adminV1.MenuType_MENU, path: "/dashboard", name: "Dashboard", component: "DashboardPage", title: "仪表盘", icon: "dashboard", order: 1},
		{id: 10, mType: adminV1.MenuType_FOLDER, path: "/opm", redirect: "/opm/organization", name: "Opm", title: "组织权限", icon: "corporate_fare", order: 10},
		{id: 11, parentID: 10, mType: adminV1.MenuType_MENU, path: "/opm/organization", name: "OpmOrganization", component: "opm/OrganizationPage", title: "组织", icon: "apartment", order: 1},
		{id: 12, parentID: 10, mType: adminV1.MenuType_MENU, path: "/opm/department", name: "OpmDepartment", component: "opm/DepartmentPage", title: "部门", icon: "account_tree", order: 2},
		{id: 13, parentID: 10, mType: adminV1.MenuType_MENU, path: "/opm/position", name: "OpmPosition", component: "opm/PositionPage", title: "岗位", icon: "badge", order: 3},
		{id: 14, parentID: 10, mType: adminV1.MenuType_MENU, path: "/opm/user", name: "OpmUser", component: "opm/UserPage", title: "用户", icon: "people", order: 4},
		{id: 21, parentID: 10, mType: adminV1.MenuType_MENU, path: "/permission/role", name: "PermissionRole", component: "permission/RolePage", title: "角色", icon: "admin_panel_settings", order: 5},
		{id: 22, parentID: 10, mType: adminV1.MenuType_MENU, path: "/permission/menu", name: "PermissionMenu", component: "permission/MenuPage", title: "菜单", icon: "menu", order: 6},
		{id: 102, parentID: 10, mType: adminV1.MenuType_MENU, path: "/opm/post", name: "OpmPost", component: "opm/PostPage", title: "岗位编制", icon: "work", order: 7},
		{id: 30, mType: adminV1.MenuType_FOLDER, path: "/system", redirect: "/system/dict", name: "System", title: "系统", icon: "settings", order: 30},
		{id: 31, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/dict", name: "SystemDict", component: "system/DictPage", title: "字典", icon: "book", order: 1},
		{id: 32, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/file", name: "SystemFile", component: "system/FilePage", title: "文件", icon: "folder", order: 2},
		{id: 33, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/task", name: "SystemTask", component: "system/TaskPage", title: "任务", icon: "schedule", order: 3},
		{id: 34, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/api-resource", name: "SystemApiResource", component: "system/ApiResourcePage", title: "API 资源", icon: "api", order: 4},
		{id: 37, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/login-restriction", name: "SystemLoginRestriction", component: "system/LoginRestrictionPage", title: "登录限制", icon: "gpp_maybe", order: 5},
		{id: 38, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/config", name: "SystemConfig", component: "system/SystemConfigPage", title: "系统配置", icon: "tune", order: 6},
		{id: 43, parentID: 30, mType: adminV1.MenuType_MENU, path: "/system/online-user", name: "SystemOnlineUser", component: "system/OnlineUserPage", title: "在线用户", icon: "sensors", order: 7},
		{id: 40, mType: adminV1.MenuType_MENU, path: "/tenant", name: "Tenant", component: "tenant/TenantPage", title: "租户", icon: "Apartment", order: 40},
		{id: 50, mType: adminV1.MenuType_FOLDER, path: "/log", redirect: "/log/login", name: "Log", title: "日志", icon: "history", order: 50},
		{id: 51, parentID: 50, mType: adminV1.MenuType_MENU, path: "/log/login", name: "LogLogin", component: "log/LoginLogPage", title: "登录日志", icon: "login", order: 1},
		{id: 52, parentID: 50, mType: adminV1.MenuType_MENU, path: "/log/operation", name: "LogOperation", component: "log/OperationLogPage", title: "操作日志", icon: "list_alt", order: 2},
		{id: 60, mType: adminV1.MenuType_MENU, path: "/github-radar", name: "GithubRadar",
			component: "github-radar/RadarPage", title: "github热门追踪", icon: "travel_explore", order: 20},
	}
}

func (s *SeedService) seedMenus(ctx context.Context) error {
	for _, r := range platformMenuSeedRecords() {
		if err := s.upsertSeedMenu(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *SeedService) upsertSeedMenu(ctx context.Context, r seedMenuRecord) error {
	menu := &adminV1.Menu{
		Id:        trans.Ptr(r.id),
		Type:      r.mType.Enum(),
		Path:      trans.Ptr(r.path),
		Name:      trans.Ptr(r.name),
		Component: trans.Ptr(r.component),
		Status:    adminV1.MenuStatus_ON.Enum(),
		Redirect:  trans.Ptr(r.redirect),
		Meta: &adminV1.RouteMeta{
			Title: trans.Ptr(r.title),
			Icon:  trans.Ptr(r.icon),
			Order: trans.Ptr(r.order),
		},
	}
	if r.parentID != 0 {
		menu.ParentId = trans.Ptr(r.parentID)
	}
	req := &adminV1.UpdateMenuRequest{
		Id:           uint32(r.id),
		Data:         menu,
		AllowMissing: trans.Ptr(true),
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{
			"parent_id", "type", "path", "redirect", "name", "component", "status",
			"meta.title", "meta.icon", "meta.order",
		}},
	}
	if err := s.menuUC.Update(ctx, req); err != nil {
		s.log.Errorf("seed menu upsert [%d %s] failed: %v", r.id, r.title, err)
		return err
	}
	return nil
}

// allApiIds 返回全部 API 资源 ID。
// 必须真实返回：种子超级管理员角色凭此绑定全部接口权限；若返回空，
// 启用 casbin 授权引擎后 super 也会 403（等于全站不可用）。
func (s *SeedService) allApiIds(ctx context.Context) []uint32 {
	resp, err := s.apiResourceUC.List(ctx, &pagination.PagingRequest{NoPaging: trans.Ptr(true)})
	if err != nil || resp == nil {
		s.log.Warnf("list api resources for role seeding failed: %v", err)
		return nil
	}
	ids := make([]uint32, 0, len(resp.GetItems()))
	for _, it := range resp.GetItems() {
		if it.GetId() != 0 {
			ids = append(ids, it.GetId())
		}
	}
	return ids
}

func (s *SeedService) seedRoles(ctx context.Context, apiIds []uint32) error {
	menus, err := s.menuUC.ListAll(ctx)
	if err != nil {
		return err
	}
	menuIDs := make([]uint32, 0, len(menus))
	for _, m := range menus {
		if m.GetId() != 0 {
			menuIDs = append(menuIDs, uint32(m.GetId()))
		}
	}
	if role, err := s.roleUC.GetRoleByCode(ctx, "super"); err == nil && role != nil {
		// 已存在：补齐新增接口/菜单权限（新增 API 资源后 super 不应 403）。
		// 只在有增量时更新，避免每次启动都全量重写关联表。
		if len(apiIds) > 0 && !containsAll(role.GetApis(), apiIds) {
			merged := mergeUint32(role.GetApis(), apiIds)
			menus := mergeUint32(role.GetMenus(), menuIDs)
			if err := s.roleUC.Update(ctx, &userV1.UpdateRoleRequest{Data: &userV1.Role{
				Id:    trans.Ptr(role.GetId()),
				Name:  trans.Ptr(role.GetName()),
				Code:  trans.Ptr(role.GetCode()),
				Menus: menus,
				Apis:  merged,
			}}); err != nil {
				s.log.Warnf("refresh super role api bindings failed: %v", err)
			} else {
				s.log.Infof("super role api bindings refreshed: %d -> %d", len(role.GetApis()), len(merged))
			}
		}
		return nil
	}
	if err := s.roleUC.Create(ctx, &userV1.CreateRoleRequest{Data: &userV1.Role{
		Name:   trans.Ptr("超级管理员"),
		Code:   trans.Ptr("super"),
		Status: trans.Ptr("ON"),
		Remark: trans.Ptr("framework skeleton"),
		Menus:  menuIDs,
		Apis:   apiIds,
	}}); err != nil {
		return fmt.Errorf("seed super role: %w", err)
	}
	return nil
}

// containsAll 判断 base 是否覆盖全部 want。
func containsAll(base, want []uint32) bool {
	set := make(map[uint32]struct{}, len(base))
	for _, v := range base {
		set[v] = struct{}{}
	}
	for _, v := range want {
		if _, ok := set[v]; !ok {
			return false
		}
	}
	return true
}

// mergeUint32 合并去重（保持 base 顺序优先）。
func mergeUint32(base, extra []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(base)+len(extra))
	out := make([]uint32, 0, len(base)+len(extra))
	for _, v := range base {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range extra {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func (s *SeedService) seedOrganizations(ctx context.Context) error {
	if _, err := s.organizationUC.Get(ctx, &userV1.GetOrganizationRequest{Id: 1}); err == nil {
		return nil
	}
	return s.organizationUC.Create(ctx, &userV1.CreateOrganizationRequest{Data: &userV1.Organization{
		Id:     trans.Ptr(uint32(1)),
		Name:   trans.Ptr("默认组织"),
		Status: trans.Ptr("ON"),
	}})
}

func (s *SeedService) seedDepartments(ctx context.Context) error {
	if _, err := s.departmentUC.Get(ctx, &userV1.GetDepartmentRequest{Id: 1}); err == nil {
		return nil
	}
	return s.departmentUC.Create(ctx, &userV1.CreateDepartmentRequest{Data: &userV1.Department{
		Id:             trans.Ptr(uint32(1)),
		Name:           trans.Ptr("默认部门"),
		OrganizationId: trans.Ptr(int32(1)),
		Status:         trans.Ptr("ON"),
	}})
}

func (s *SeedService) seedPositions(ctx context.Context) error {
	if _, err := s.positionUC.Get(ctx, &userV1.GetPositionRequest{Id: 1}); err == nil {
		return nil
	}
	return s.positionUC.Create(ctx, &userV1.CreatePositionRequest{Data: &userV1.Position{
		Id:     trans.Ptr(uint32(1)),
		Name:   trans.Ptr("管理员"),
		Code:   trans.Ptr("admin"),
		Status: trans.Ptr("ON"),
	}})
}

func (s *SeedService) seedUsers(ctx context.Context) error {
	if _, err := s.userUC.GetUserByUserName(ctx, "admin"); err == nil {
		return nil
	}
	user, err := s.userUC.Create(ctx, &userV1.CreateUserRequest{Data: &userV1.User{
		Username:  trans.Ptr("admin"),
		Realname:  trans.Ptr("管理员"),
		Nickname:  trans.Ptr("admin"),
		Authority: userV1.UserAuthority_SYS_ADMIN.Enum(),
		Status:    userV1.UserStatus_ON.Enum(),
		Roles:     []string{"super"},
		OrgId:     trans.Ptr(uint32(1)),
		PositionId: trans.Ptr(uint32(1)),
	}})
	if err != nil {
		return err
	}
	return s.userCredentialUC.Create(ctx, &authenticationV1.CreateUserCredentialRequest{Data: &authenticationV1.UserCredential{
		UserId:         user.Id,
		IdentityType:   authenticationV1.IdentityType_USERNAME.Enum(),
		Identifier:     trans.Ptr("admin"),
		CredentialType: authenticationV1.CredentialType_PASSWORD_HASH.Enum(),
		Credential:     trans.Ptr("123456"),
		IsPrimary:      trans.Ptr(true),
		Status:         authenticationV1.UserCredentialStatus_ENABLED.Enum(),
	}})
}

func (s *SeedService) seedUserRoles(ctx context.Context) error {
	return nil
}
