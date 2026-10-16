package biz

import "context"

// AuthorizerPort 授权策略端口（biz 层禁止 import data，由 data.Authorizer 实现并经 wire.Bind 绑定）。
// service 层仅做策略同步编排：角色/资源变更后触发增量同步或删除。
// 鉴权引擎本体（Engine）仍由 server 装配层直接持有 data.Authorizer 使用，不经过本端口。
type AuthorizerPort interface {
	// ResetPolicies 全量重建策略（资源组/API 资源变更时调用）。
	ResetPolicies(ctx context.Context) error
	// SyncPolicies 增量同步单个角色的策略（角色信息/绑定变更时调用）。
	SyncPolicies(ctx context.Context, roleID uint32) error
	// DeletePolicies 删除角色的全部策略（角色删除时调用）。
	DeletePolicies(ctx context.Context, roleCode string) error
}
