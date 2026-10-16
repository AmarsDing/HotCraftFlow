package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/roledept"

	adminV1 "framework/api/gen/go/admin/v1"
)

// roleDeptRepo 未导出，实现 biz.RoleDeptRepo 接口。
type roleDeptRepo struct {
	data *Data
	log  *log.Helper
}

// NewRoleDeptRepo 返回 biz.RoleDeptRepo 接口。
func NewRoleDeptRepo(data *Data, logger log.Logger) biz.RoleDeptRepo {
	return &roleDeptRepo{
		log:  log.NewHelper(log.With(logger, "module", "role-dept/repo/admin-service")),
		data: data,
	}
}

// toDTO 将 ent.RoleDept 转换为 adminV1.RoleDept DTO。
func (r *roleDeptRepo) toDTO(e *ent.RoleDept) *adminV1.RoleDept {
	if e == nil {
		return nil
	}
	dto := &adminV1.RoleDept{
		Id:       trans.Ptr(e.ID),
		RoleId:   trans.Ptr(e.RoleID),
		DeptId:   trans.Ptr(e.DeptID),
		TenantId: e.TenantID,
	}
	if e.CreateTime != nil {
		dto.CreateTime = timeutil.TimeToTimestamppb(e.CreateTime)
	}
	return dto
}

// ListByRole 查询角色绑定的部门列表。
func (r *roleDeptRepo) ListByRole(ctx context.Context, roleID uint32) (*adminV1.ListRoleDeptResponse, error) {
	if roleID == 0 {
		return &adminV1.ListRoleDeptResponse{}, nil
	}
	entities, err := r.data.db.Client().RoleDept.Query().
		Where(roledept.RoleIDEQ(roleID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query role dept list failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query role dept list failed")
	}
	dtos := make([]*adminV1.RoleDept, 0, len(entities))
	for _, e := range entities {
		dtos = append(dtos, r.toDTO(e))
	}
	return &adminV1.ListRoleDeptResponse{
		Total: uint32(len(entities)),
		Items: dtos,
	}, nil
}

// GetDeptIDsByRoleIDs 查询多个角色绑定的部门 ID 列表（去重）。
func (r *roleDeptRepo) GetDeptIDsByRoleIDs(ctx context.Context, roleIDs []uint32) ([]uint32, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	entities, err := r.data.db.Client().RoleDept.Query().
		Where(roledept.RoleIDIn(roleIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query dept ids by role ids failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query dept ids failed")
	}
	seen := make(map[uint32]struct{}, len(entities))
	deptIDs := make([]uint32, 0, len(entities))
	for _, e := range entities {
		if _, ok := seen[e.DeptID]; ok {
			continue
		}
		seen[e.DeptID] = struct{}{}
		deptIDs = append(deptIDs, e.DeptID)
	}
	return deptIDs, nil
}

// BatchBind 批量绑定部门到角色（先清空旧绑定再插入新绑定，幂等）。
// 使用事务保证原子性。
func (r *roleDeptRepo) BatchBind(ctx context.Context, roleID uint32, deptIDs []uint32, tenantID *uint32) error {
	if roleID == 0 {
		return adminV1.ErrorBadRequest("role_id is required")
	}

	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("begin tx failed")
	}

	// 1. 清空旧绑定
	if _, err := tx.RoleDept.Delete().
		Where(roledept.RoleIDEQ(roleID)).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("clear old bindings failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("clear old bindings failed")
	}

	// 2. 插入新绑定（去重）
	seen := make(map[uint32]struct{}, len(deptIDs))
	now := time.Now()
	for _, deptID := range deptIDs {
		if deptID == 0 {
			continue
		}
		if _, ok := seen[deptID]; ok {
			continue
		}
		seen[deptID] = struct{}{}
		builder := tx.RoleDept.Create().
			SetRoleID(roleID).
			SetDeptID(deptID).
			SetCreateTime(now)
		if tenantID != nil {
			builder.SetNillableTenantID(tenantID)
		}
		if err := builder.Exec(ctx); err != nil {
			_ = tx.Rollback()
			r.log.Errorf("insert binding failed (role=%d, dept=%d): %s", roleID, deptID, err.Error())
			return adminV1.ErrorInternalServerError("insert binding failed")
		}
	}

	if err := tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("commit failed")
	}
	return nil
}

// UnbindByRole 解绑角色的所有部门。
func (r *roleDeptRepo) UnbindByRole(ctx context.Context, roleID uint32) error {
	if roleID == 0 {
		return adminV1.ErrorBadRequest("role_id is required")
	}
	if _, err := r.data.db.Client().RoleDept.Delete().
		Where(roledept.RoleIDEQ(roleID)).
		Exec(ctx); err != nil {
		r.log.Errorf("unbind by role failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("unbind failed")
	}
	return nil
}

// 确保编译期接口实现：roleDeptRepo 实现 biz.RoleDeptRepo。
var _ biz.RoleDeptRepo = (*roleDeptRepo)(nil)
