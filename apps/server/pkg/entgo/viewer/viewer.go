package viewer

import (
	"context"

	userV1 "framework/api/gen/go/user/v1"
)

// Viewer describes the query/mutation viewer-context.
type Viewer interface {
	// Admin 是否是管理员
	Admin() bool

	// SystemAdmin 系统管理员
	SystemAdmin() bool

	// TenantAdmin 租户管理员
	TenantAdmin() bool

	// Tenant 返回租户ID
	Tenant() (uint32, bool)

	// GetUserID 返回当前用户ID（用于 scope=SELF 时按 user_id 字段过滤）
	// 返回 false 表示未注入（如系统内部任务）
	GetUserID() (uint32, bool)

	// GetDeptIDs 返回可访问的部门 ID 列表，用于数据权限过滤：
	//   - nil   表示无限制（管理员或 scope=ALL）
	//   - 空切片 表示仅访问自己的数据（scope=SELF）
	//   - 非空   表示仅访问这些部门的数据（scope=DEPT/CUSTOM）
	GetDeptIDs() ([]uint32, bool)
}

// UserViewer describes a user-viewer.
type UserViewer struct {
	UserID    *uint32              // 当前用户ID
	TenantId  *uint32              // Tenant ID
	Authority userV1.UserAuthority // 用户权限级别
	DeptIDs   []uint32             // 可访问部门列表（nil=无限制，空切片=仅SELF）
}

func (v UserViewer) Admin() bool {
	return v.SystemAdmin() || v.TenantAdmin()
}

func (v UserViewer) SystemAdmin() bool {
	return v.Authority == userV1.UserAuthority_SYS_ADMIN
}

func (v UserViewer) TenantAdmin() bool {
	return v.Authority == userV1.UserAuthority_TENANT_ADMIN
}

func (v UserViewer) Tenant() (uint32, bool) {
	if v.TenantId != nil {
		return *v.TenantId, true
	}
	return 0, false
}

// GetUserID 返回当前用户ID。
func (v UserViewer) GetUserID() (uint32, bool) {
	if v.UserID != nil {
		return *v.UserID, true
	}
	return 0, false
}

// GetDeptIDs 返回可访问的部门 ID 列表。
// nil 表示无限制（返回 false），非 nil 表示按列表过滤（返回 true）。
func (v UserViewer) GetDeptIDs() ([]uint32, bool) {
	if v.DeptIDs == nil {
		return nil, false
	}
	return v.DeptIDs, true
}

type ctxKey struct{}

// FromContext returns the Viewer stored in a context.
func FromContext(ctx context.Context) Viewer {
	v, _ := ctx.Value(ctxKey{}).(Viewer)
	return v
}

// NewContext returns a copy of parent context with the given Viewer attached with it.
func NewContext(parent context.Context, v Viewer) context.Context {
	return context.WithValue(parent, ctxKey{}, v)
}
