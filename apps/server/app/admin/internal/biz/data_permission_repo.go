package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
	userV1 "framework/api/gen/go/user/v1"
	"framework/pkg/entgo/viewer"
	"framework/pkg/middleware/auth"
)

// DataPermissionRepo 数据权限规则仓储端口，由 data 层实现。
// 除标准 CRUD 外，提供按 role_id 批量查询规则、以及构建 viewer 所需的用户角色/部门查询。
type DataPermissionRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDataPermissionResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *adminV1.GetDataPermissionRequest) (*adminV1.DataPermission, error)
	Create(ctx context.Context, req *adminV1.CreateDataPermissionRequest) error
	Update(ctx context.Context, req *adminV1.UpdateDataPermissionRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteDataPermissionRequest) error

	// GetByRoleIDs 查询多个角色的数据权限规则（仅返回启用的）。
	GetByRoleIDs(ctx context.Context, roleIDs []uint32) ([]*adminV1.DataPermission, error)
	// GetRoleIDsByUserID 查询用户绑定的角色 ID 列表（通过 user_role 表）。
	GetRoleIDsByUserID(ctx context.Context, userID uint32) ([]uint32, error)
	// GetUserDeptID 查询用户所属部门 ID（通过 user 表 dept_id 字段）。
	GetUserDeptID(ctx context.Context, userID uint32) (uint32, error)
	// GetUserAuthority 查询用户授权等级（SYS_ADMIN/TENANT_ADMIN/...），不存在时返回空字符串。
	GetUserAuthority(ctx context.Context, userID uint32) (string, error)
}

// RoleDeptRepo 角色部门关联仓储端口，由 data 层实现。
type RoleDeptRepo interface {
	// ListByRole 查询角色绑定的部门列表。
	ListByRole(ctx context.Context, roleID uint32) (*adminV1.ListRoleDeptResponse, error)
	// GetDeptIDsByRoleIDs 查询多个角色绑定的部门 ID 列表（去重）。
	GetDeptIDsByRoleIDs(ctx context.Context, roleIDs []uint32) ([]uint32, error)
	// BatchBind 批量绑定部门到角色（先清空旧绑定再插入新绑定，幂等）。
	BatchBind(ctx context.Context, roleID uint32, deptIDs []uint32, tenantID *uint32) error
	// UnbindByRole 解绑角色的所有部门。
	UnbindByRole(ctx context.Context, roleID uint32) error
}

// RoleResourceGroupReader 角色资源组绑定查询端口（窄接口），由 data 层 RoleResourceGroupRepo 实现。
// 供 GetUserPermissions 聚合用户可访问资源组 ID 列表（06-query 数据权限过滤链路）。
type RoleResourceGroupReader interface {
	// ListResourceGroupIDsByRoleIDs 查询多个角色绑定的资源组 ID 列表（去重）。
	ListResourceGroupIDsByRoleIDs(ctx context.Context, roleIDs []uint32) ([]uint32, error)
}

// DataPermissionUsecase 数据权限业务用例。
// 持有 DataPermissionRepo 和 RoleDeptRepo，提供标准 CRUD 和 BuildViewer（供中间件调用）。
type DataPermissionUsecase struct {
	repo       DataPermissionRepo
	roleDept   RoleDeptRepo
	rgReader   RoleResourceGroupReader
	log        *log.Helper
}

// NewDataPermissionUsecase 创建数据权限 Usecase。
func NewDataPermissionUsecase(repo DataPermissionRepo, roleDept RoleDeptRepo, rgReader RoleResourceGroupReader, logger log.Logger) *DataPermissionUsecase {
	return &DataPermissionUsecase{
		repo:     repo,
		roleDept: roleDept,
		rgReader: rgReader,
		log:      log.NewHelper(log.With(logger, "module", "data-permission/usecase/admin-service")),
	}
}

// BuildViewer 根据用户身份构建数据权限 viewer，供数据权限中间件调用。
//
// 聚合规则（按最宽 scope 优先）：
//   - SYS_ADMIN / TENANT_ADMIN → DeptIDs=nil（无限制）
//   - 任一角色 scope=ALL        → DeptIDs=nil（无限制）
//   - 任一角色 scope=CUSTOM     → DeptIDs=角色绑定的部门并集
//   - 任一角色 scope=DEPT       → DeptIDs=[用户部门]
//   - 仅 scope=SELF             → DeptIDs=[]（空切片，仅按 UserID 过滤）
//   - 无任何规则                 → 默认 SELF（DeptIDs=[]）
//
// UserID 始终注入，用于 scope=SELF 时按 user_id 字段过滤。
func (uc *DataPermissionUsecase) BuildViewer(
	ctx context.Context,
	userID uint32,
	authority userV1.UserAuthority,
	tenantId *uint32,
) (viewer.Viewer, error) {
	// 管理员无限制
	if authority == userV1.UserAuthority_SYS_ADMIN || authority == userV1.UserAuthority_TENANT_ADMIN {
		return viewer.UserViewer{
			UserID:    trans.Ptr(userID),
			Authority: authority,
			TenantId:  tenantId,
			DeptIDs:   nil, // nil 表示无限制
		}, nil
	}

	// 普通用户：查询角色 → 查询数据权限规则 → 聚合 scope
	roleIDs, err := uc.repo.GetRoleIDsByUserID(ctx, userID)
	if err != nil {
		uc.log.Errorf("BuildViewer: query role ids failed for user %d: %v", userID, err)
		return nil, err
	}

	// 无角色 → 默认 SELF
	if len(roleIDs) == 0 {
		return viewer.UserViewer{
			UserID:    trans.Ptr(userID),
			Authority: authority,
			TenantId:  tenantId,
			DeptIDs:   []uint32{}, // 空切片表示仅 SELF
		}, nil
	}

	perms, err := uc.repo.GetByRoleIDs(ctx, roleIDs)
	if err != nil {
		uc.log.Errorf("BuildViewer: query data permissions failed for roles %v: %v", roleIDs, err)
		return nil, err
	}

	// 聚合 scope：取最宽
	hasAll := false
	hasCustom := false
	hasDept := false
	for _, p := range perms {
		switch p.GetScope() {
		case "ALL":
			hasAll = true
		case "CUSTOM":
			hasCustom = true
		case "DEPT":
			hasDept = true
		}
	}

	var deptIDs []uint32
	switch {
	case hasAll:
		deptIDs = nil // 无限制
	case hasCustom:
		// 自定义部门：查询角色绑定的部门并集
		customDeptIDs, err := uc.roleDept.GetDeptIDsByRoleIDs(ctx, roleIDs)
		if err != nil {
			uc.log.Errorf("BuildViewer: query custom dept ids failed for roles %v: %v", roleIDs, err)
			return nil, err
		}
		// 若同时有 DEPT scope，追加用户部门
		if hasDept {
			userDeptID, err := uc.repo.GetUserDeptID(ctx, userID)
			if err != nil {
				uc.log.Errorf("BuildViewer: query user dept id failed for user %d: %v", userID, err)
				return nil, err
			}
			if userDeptID > 0 {
				customDeptIDs = append(customDeptIDs, userDeptID)
			}
		}
		deptIDs = dedupUint32(customDeptIDs)
	case hasDept:
		// 仅 DEPT：用户部门
		userDeptID, err := uc.repo.GetUserDeptID(ctx, userID)
		if err != nil {
			uc.log.Errorf("BuildViewer: query user dept id failed for user %d: %v", userID, err)
			return nil, err
		}
		if userDeptID > 0 {
			deptIDs = []uint32{userDeptID}
		} else {
			deptIDs = []uint32{}
		}
	default:
		// 仅 SELF 或无规则
		deptIDs = []uint32{}
	}

	return viewer.UserViewer{
		UserID:    trans.Ptr(userID),
		Authority: authority,
		TenantId:  tenantId,
		DeptIDs:   deptIDs,
	}, nil
}

// GetUserPermissions 聚合指定用户的数据权限范围，供 06-query 等跨模块服务做数据过滤。
//
// 聚合规则（与 BuildViewer 口径一致）：
//   - SYS_ADMIN / TENANT_ADMIN → Unrestricted=true
//   - 任一角色 scope=ALL        → Unrestricted=true
//   - 否则返回最宽 scope（CUSTOM > DEPT > SELF）+ 对应 DeptIDs
//
// ResourceGroupIds 始终返回角色绑定的资源组并集（与 scope 无关），
// 供调用方按资源组白名单过滤设备。
func (uc *DataPermissionUsecase) GetUserPermissions(ctx context.Context, req *adminV1.GetUserPermissionsRequest) (*adminV1.UserPermissions, error) {
	userID := req.GetId()
	if userID == 0 {
		// 服务身份（网关 API Key 代签内部 JWT，uid 固定 0）：用户表无此 id，
		// 按调用方 JWT authority 裁定——SYS_ADMIN/TENANT_ADMIN 视为 Unrestricted，
		// 与实名管理员同口径；其余 fail-closed。
		// 根修：下游 linemonitor/monitorquery 等经网关服务账号（X-API-Key）调用时
		// uid=0 命中本分支原 400 → 上游 500 的系统性故障（2026-08-15 P3 复盘）。
		if caller, cerr := auth.FromContext(ctx); cerr == nil {
			if a := caller.GetAuthority(); a == userV1.UserAuthority_SYS_ADMIN || a == userV1.UserAuthority_TENANT_ADMIN {
				return &adminV1.UserPermissions{
					UserId:           0,
					Scope:            "ALL",
					Unrestricted:     true,
					DeptIds:          []uint32{},
					ResourceGroupIds: []uint32{},
				}, nil
			}
		}
		return nil, adminV1.ErrorBadRequest("user id is required")
	}

	resp := &adminV1.UserPermissions{
		UserId:           userID,
		Scope:            "SELF",
		DeptIds:          []uint32{},
		ResourceGroupIds: []uint32{},
	}

	// 管理员无限制
	authority, err := uc.repo.GetUserAuthority(ctx, userID)
	if err != nil {
		uc.log.Errorf("GetUserPermissions: query authority failed for user %d: %v", userID, err)
		return nil, err
	}
	if authority == "SYS_ADMIN" || authority == "TENANT_ADMIN" {
		resp.Unrestricted = true
		resp.Scope = "ALL"
		return resp, nil
	}

	roleIDs, err := uc.repo.GetRoleIDsByUserID(ctx, userID)
	if err != nil {
		uc.log.Errorf("GetUserPermissions: query role ids failed for user %d: %v", userID, err)
		return nil, err
	}
	if len(roleIDs) == 0 {
		return resp, nil
	}

	// 资源组并集（无论 scope 宽窄都返回）
	rgIDs, err := uc.rgReader.ListResourceGroupIDsByRoleIDs(ctx, roleIDs)
	if err != nil {
		uc.log.Errorf("GetUserPermissions: query resource groups failed for roles %v: %v", roleIDs, err)
		return nil, err
	}
	resp.ResourceGroupIds = rgIDs

	perms, err := uc.repo.GetByRoleIDs(ctx, roleIDs)
	if err != nil {
		uc.log.Errorf("GetUserPermissions: query data permissions failed for roles %v: %v", roleIDs, err)
		return nil, err
	}

	hasAll, hasCustom, hasDept := false, false, false
	for _, p := range perms {
		switch p.GetScope() {
		case "ALL":
			hasAll = true
		case "CUSTOM":
			hasCustom = true
		case "DEPT":
			hasDept = true
		}
	}

	switch {
	case hasAll:
		resp.Unrestricted = true
		resp.Scope = "ALL"
		resp.DeptIds = nil
	case hasCustom:
		resp.Scope = "CUSTOM"
		customDeptIDs, err := uc.roleDept.GetDeptIDsByRoleIDs(ctx, roleIDs)
		if err != nil {
			uc.log.Errorf("GetUserPermissions: query custom dept ids failed for roles %v: %v", roleIDs, err)
			return nil, err
		}
		if hasDept {
			userDeptID, err := uc.repo.GetUserDeptID(ctx, userID)
			if err != nil {
				uc.log.Errorf("GetUserPermissions: query user dept id failed for user %d: %v", userID, err)
				return nil, err
			}
			if userDeptID > 0 {
				customDeptIDs = append(customDeptIDs, userDeptID)
			}
		}
		resp.DeptIds = dedupUint32(customDeptIDs)
	case hasDept:
		resp.Scope = "DEPT"
		userDeptID, err := uc.repo.GetUserDeptID(ctx, userID)
		if err != nil {
			uc.log.Errorf("GetUserPermissions: query user dept id failed for user %d: %v", userID, err)
			return nil, err
		}
		if userDeptID > 0 {
			resp.DeptIds = []uint32{userDeptID}
		}
	default:
		// SELF：DeptIds 为空，调用方按 user_id 过滤
	}

	return resp, nil
}

// dedupUint32 去重 uint32 切片（保序）。
func dedupUint32(ids []uint32) []uint32 {	if len(ids) == 0 {
		return []uint32{}
	}
	seen := make(map[uint32]struct{}, len(ids))
	out := make([]uint32, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// List 分页查询数据权限规则列表
func (uc *DataPermissionUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDataPermissionResponse, error) {
	return uc.repo.List(ctx, req)
}

// Get 查询单个数据权限规则
func (uc *DataPermissionUsecase) Get(ctx context.Context, req *adminV1.GetDataPermissionRequest) (*adminV1.DataPermission, error) {
	return uc.repo.Get(ctx, req)
}

// Create 创建数据权限规则
func (uc *DataPermissionUsecase) Create(ctx context.Context, req *adminV1.CreateDataPermissionRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	return uc.repo.Create(ctx, req)
}

// Update 更新数据权限规则
func (uc *DataPermissionUsecase) Update(ctx context.Context, req *adminV1.UpdateDataPermissionRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	return uc.repo.Update(ctx, req)
}

// Delete 删除数据权限规则
func (uc *DataPermissionUsecase) Delete(ctx context.Context, req *adminV1.DeleteDataPermissionRequest) error {
	return uc.repo.Delete(ctx, req)
}

// RoleDeptUsecase 角色部门关联业务用例。
type RoleDeptUsecase struct {
	repo RoleDeptRepo
	log  *log.Helper
}

// NewRoleDeptUsecase 创建角色部门 Usecase。
func NewRoleDeptUsecase(repo RoleDeptRepo, logger log.Logger) *RoleDeptUsecase {
	return &RoleDeptUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "role-dept/usecase/admin-service")),
	}
}

// ListByRole 查询角色绑定的部门列表
func (uc *RoleDeptUsecase) ListByRole(ctx context.Context, roleID uint32) (*adminV1.ListRoleDeptResponse, error) {
	return uc.repo.ListByRole(ctx, roleID)
}

// BatchBind 批量绑定部门到角色
func (uc *RoleDeptUsecase) BatchBind(ctx context.Context, roleID uint32, deptIDs []uint32, tenantID *uint32) error {
	if roleID == 0 {
		return adminV1.ErrorBadRequest("role_id is required")
	}
	// 单独事务包裹由 repo 实现（先删后插）
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return uc.repo.BatchBind(ctx, roleID, deptIDs, tenantID)
}

// UnbindByRole 解绑角色的所有部门
func (uc *RoleDeptUsecase) UnbindByRole(ctx context.Context, roleID uint32) error {
	if roleID == 0 {
		return adminV1.ErrorBadRequest("role_id is required")
	}
	return uc.repo.UnbindByRole(ctx, roleID)
}
