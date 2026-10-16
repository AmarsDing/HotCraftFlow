package data

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/roleresourcegroup"
)

// RoleResourceGroupRepo 管理 ap_org_role_resource_group 关联表。
// 角色勾选资源组 → 展开为组内所有 API 资源 → 生成 Casbin 策略。
type RoleResourceGroupRepo struct {
	data *Data
	log  *log.Helper
}

func NewRoleResourceGroupRepo(data *Data, logger log.Logger) *RoleResourceGroupRepo {
	return &RoleResourceGroupRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "role-resource-group/repo/admin-service")),
	}
}

// ListResourceGroupIDsByRole 返回角色绑定的资源组 ID 列表。
func (r *RoleResourceGroupRepo) ListResourceGroupIDsByRole(ctx context.Context, roleID uint32) ([]uint32, error) {
	entities, err := r.data.db.Client().RoleResourceGroup.Query().
		Where(roleresourcegroup.RoleIDEQ(roleID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list role resource groups failed: %s", err.Error())
		return nil, fmt.Errorf("list role resource groups failed: %w", err)
	}

	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ResourceGroupID)
	}
	return ids, nil
}

// ListResourceGroupIDsByRoleIDs 批量查询多个角色绑定的资源组 ID 列表（去重，保序）。
// 实现 biz.RoleResourceGroupReader 窄接口，供 GetUserPermissions 聚合。
func (r *RoleResourceGroupRepo) ListResourceGroupIDsByRoleIDs(ctx context.Context, roleIDs []uint32) ([]uint32, error) {
	if len(roleIDs) == 0 {
		return []uint32{}, nil
	}
	entities, err := r.data.db.Client().RoleResourceGroup.Query().
		Where(roleresourcegroup.RoleIDIn(roleIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list resource group ids by role ids failed: %s", err.Error())
		return nil, fmt.Errorf("list resource group ids by role ids failed: %w", err)
	}

	seen := make(map[uint32]struct{}, len(entities))
	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		if _, ok := seen[e.ResourceGroupID]; ok {
			continue
		}
		seen[e.ResourceGroupID] = struct{}{}
		ids = append(ids, e.ResourceGroupID)
	}
	return ids, nil
}

// ListAllRoleBindings 返回所有角色→资源组绑定，用于启动时全量生成策略。
// 返回 map[roleID][]resourceGroupID。
func (r *RoleResourceGroupRepo) ListAllRoleBindings(ctx context.Context) (map[uint32][]uint32, error) {
	entities, err := r.data.db.Client().RoleResourceGroup.Query().All(ctx)
	if err != nil {
		r.log.Errorf("list all role resource group bindings failed: %s", err.Error())
		return nil, fmt.Errorf("list all role resource group bindings failed: %w", err)
	}

	bindings := make(map[uint32][]uint32)
	for _, e := range entities {
		bindings[e.RoleID] = append(bindings[e.RoleID], e.ResourceGroupID)
	}
	return bindings, nil
}

// SetRoleBindings 替换角色的资源组绑定（先删后插）。
func (r *RoleResourceGroupRepo) SetRoleBindings(ctx context.Context, roleID uint32, resourceGroupIDs []uint32) error {
	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx failed: %w", err)
	}

	if _, err := tx.RoleResourceGroup.Delete().
		Where(roleresourcegroup.RoleIDEQ(roleID)).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete old bindings failed: %w", err)
	}

	if len(resourceGroupIDs) > 0 {
		builders := make([]*ent.RoleResourceGroupCreate, 0, len(resourceGroupIDs))
		for _, gid := range resourceGroupIDs {
			builders = append(builders,
				tx.RoleResourceGroup.Create().
					SetRoleID(roleID).
					SetResourceGroupID(gid),
			)
		}
		if err := tx.RoleResourceGroup.CreateBulk(builders...).Exec(ctx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("bulk insert bindings failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx failed: %w", err)
	}
	return nil
}
