package serviceauth

// 服务令牌 scope 常量（最小权限划分）。
// 命名规则：<目标服务>:<资源域>:<动作>。
const (
	// ScopeWildcard 通配（仅 gateway 代签 SYS_ADMIN 绑定身份的 API key 使用，
	// 语义等价于该绑定用户本人操作，保留现行行为）。
	ScopeWildcard = "*"

	// ---- admin（资产中心/用户/知识库/告警聚合/许可证）----
	ScopeAdminAssetRead      = "admin:asset:read"      // 资产/设备/分类/摄像机扩展只读
	ScopeAdminAssetAlias     = "admin:asset:alias"     // 资产别名解析/热词导出
	ScopeAdminProtocolRead   = "admin:protocol:read"   // 协议绑定配置只读
	ScopeAdminUserRead       = "admin:user:read"       // 用户只读
	ScopeAdminKnowledgeRead  = "admin:knowledge:read"  // 知识库检索/统计
	ScopeAdminKnowledgeWrite = "admin:knowledge:write" // 知识库创建
	ScopeAdminLicenseVerify  = "admin:license:verify"  // 许可证校验
	ScopeAdminSystemConfigRead = "admin:system-config:read" // 系统配置只读（opstools 回源 ops.* 远程运维设置）
	ScopeAdminCredentialSecret = "admin:credential:secret" // 凭证明文按需读取（video 播放/PTZ 注入，22 视频 §4.1 T1）

	// ---- monitoralarm（告警事件）----
	ScopeMonitoralarmEventRead = "monitoralarm:event:read" // 告警事件详情只读（admin 工单 alarm_derived 建单前存在性校验）

	// ---- collector-snmp-ssh（监控采集）----
	ScopeCollectorDeviceRead   = "collector:device:read"   // 设备状态/历史指标只读（visualization 设备指标数据源）
	ScopeCollectorTaskRead     = "collector:task:read"     // 采集任务列表只读（visualization 任务统计数据源）

	// ---- monitoralarm（告警查询扩展）----
	ScopeMonitoralarmArrivalRead = "monitoralarm:arrival:read" // 数据到达监控只读（visualization arrival 数据源）

	// ---- linemonitor（线路监控）----
	ScopeLinemonitorTopologyRead = "linemonitor:topology:read" // 拓扑只读
	ScopeLinemonitorConfigRead   = "linemonitor:config:read"   // 配置备份元数据/diff/内容只读
	ScopeLinemonitorConfigBackup = "linemonitor:config:backup" // 手动触发设备配置备份
	ScopeLinemonitorFlowRead     = "linemonitor:flow:read"     // NetFlow/sFlow 流量分析只读（visualization 大屏流量数据源）

	// ---- awos-pb（AWOS 存储基座；authmw 当前仅校验合法平台身份，scope 预登记供后续细粒度化）----
	ScopeAwospbDCRead  = "awospb:dc:read"  // 数据中心配置集合只读（datacenter 字典加载）
	ScopeAwospbDCWrite = "awospb:dc:write" // 数据中心配置集合写（ingest 草稿项自动注册）
	ScopeAwospbItemsBulk = "awospb:items:bulk" // awos_items 历史批量写入（datacenter historian）

	// ---- awos-pb forecast 集合（25-8 预报服务 TAF 编发；通用集合 CRUD 经 PB REST，规则 deny 直连）----
	ScopeAwospbForecastRead  = "awospb:forecast:read"  // forecast 集合读
	ScopeAwospbForecastWrite = "awospb:forecast:write" // forecast 集合写

	// ---- awos-pb obsbook 集合（25-5 年月总簿；通用集合 CRUD 经 PB REST，规则 deny 直连）----
	ScopeAwospbObsbookRead  = "awospb:obsbook:read"  // obsbook 集合读
	ScopeAwospbObsbookWrite = "awospb:obsbook:write" // obsbook 集合写
)
