package server

import "framework/pkg/serviceauth"

// serviceTokenRules admin 服务令牌访问规则表（uid==0 令牌的唯一合法操作面，拒绝默认）。
//
// 盘点来源（2026-08-20 服务令牌收敛）：
//   - voice（经 gateway 代理）：alias:export-hotwords
//   - video：device-ext-cameras / monitor-devices / monitor-assets / device-categories / credentials/{id}/secret（22 视频 §4.1 T1 凭证按需解析）
//   - opstools：monitor-assets / device-categories / assets/{id}/full / protocols/{binding_id} / protocol-targets/{protocol_type} / system_configs(ops.* 设置回源)
//   - aiops：assets/{id}/full / monitor-assets / alias:resolve / protocols/{binding_id}
//     / users / users/{id} / knowledge-base（POST）/ knowledge-base:search / statistics
//   - gateway license_verifier：licenses/verify
//   - gateway MintInternalJWT（API key 代签）：scp=["*"] 通配，等价绑定用户本人，无需登记
//
// 注意：
//   - GetFullConfig（protocols/{binding_id}）在 JWT 白名单中（collector 无令牌拉取），
//     中间件链整体跳过，登记仅对持令牌调用生效，无害保留。
//   - aiops 契约先行的 work-orders / cabinet-tree 路由尚不存在，落地时必须同步登记规则。
//   - 新增受保护路由若允许服务间调用，必须在此登记 (method, template, scope)，否则 403。
var serviceTokenRules = []serviceauth.Rule{
	// 资产别名（voice 热词导出 / aiops 别名解析）
	{Method: "GET", Path: "/api/v1/monitor/asset/alias:export-hotwords", Scope: serviceauth.ScopeAdminAssetAlias},
	{Method: "GET", Path: "/api/v1/monitor/asset/alias:resolve", Scope: serviceauth.ScopeAdminAssetAlias},

	// 资产只读（video 资产同步 / opstools 绑定选择 / aiops 资产查询）
	{Method: "GET", Path: "/api/v1/monitor/device-ext-cameras", Scope: serviceauth.ScopeAdminAssetRead},
	{Method: "GET", Path: "/api/v1/monitor/monitor-devices", Scope: serviceauth.ScopeAdminAssetRead},
	// 单条设备存在性核验（awos-admin 线路组 device_id 跨库引用校验，2026-09 审计修复 T4）
	{Method: "GET", Path: "/api/v1/monitor/monitor-devices/{id}", Scope: serviceauth.ScopeAdminAssetRead},
	{Method: "GET", Path: "/api/v1/monitor/monitor-assets", Scope: serviceauth.ScopeAdminAssetRead},
	{Method: "GET", Path: "/api/v1/monitor/device-categories", Scope: serviceauth.ScopeAdminAssetRead},
	{Method: "GET", Path: "/api/v1/monitor/assets/{id}/full", Scope: serviceauth.ScopeAdminAssetRead},

	// 凭证明文按需读取（video 播放/PTZ 凭证注入；uid=0 服务令牌豁免操作密码，审计落 credential_secret_view）
	{Method: "GET", Path: "/api/v1/monitor/credentials/{id}/secret", Scope: serviceauth.ScopeAdminCredentialSecret},

	// 协议绑定配置（opstools / aiops 持令牌调用）
	{Method: "GET", Path: "/api/v1/monitor/protocols/{binding_id}", Scope: serviceauth.ScopeAdminProtocolRead},
	// 协议目标批量聚合（opstools 终端/VNC 目标选择器，消除逐资产 N+1）
	{Method: "GET", Path: "/api/v1/monitor/protocol-targets/{protocol_type}", Scope: serviceauth.ScopeAdminProtocolRead},

	// 用户只读（aiops 审批人/通知对象解析）
	{Method: "GET", Path: "/api/v1/monitor/users", Scope: serviceauth.ScopeAdminUserRead},
	{Method: "GET", Path: "/api/v1/monitor/users/{id}", Scope: serviceauth.ScopeAdminUserRead},

	// 知识库（aiops 诊断知识沉淀与检索）
	{Method: "POST", Path: "/api/v1/monitor/knowledge-base", Scope: serviceauth.ScopeAdminKnowledgeWrite},
	{Method: "POST", Path: "/api/v1/monitor/knowledge-base/search", Scope: serviceauth.ScopeAdminKnowledgeRead},
	{Method: "GET", Path: "/api/v1/monitor/knowledge-base/statistics", Scope: serviceauth.ScopeAdminKnowledgeRead},
	// 知识库标签（aiops 知识沉淀 tags=[AI沉淀] 名称→tag_ids 预检/兜底创建）
	{Method: "GET", Path: "/api/v1/monitor/knowledge-tags", Scope: serviceauth.ScopeAdminKnowledgeRead},
	{Method: "POST", Path: "/api/v1/monitor/knowledge-tags", Scope: serviceauth.ScopeAdminKnowledgeWrite},

	// 许可证校验（gateway license_verifier 周期回验）
	{Method: "POST", Path: "/api/v1/monitor/licenses/verify", Scope: serviceauth.ScopeAdminLicenseVerify},

	// 系统配置只读（opstools OpsSettingsProvider 回源 ops.* 远程运维设置；拒绝默认下需显式登记）
	{Method: "GET", Path: "/api/v1/monitor/system_configs", Scope: serviceauth.ScopeAdminSystemConfigRead},
}
