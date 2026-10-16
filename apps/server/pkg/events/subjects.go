package events

// 全站 NATS subject 常量（按域分组）。
//
// 命名规范：<domain>.<entity>.<verb>。新增主题必须在此登记并注明生产者/消费者，
// 禁止在服务内部重新声明同值常量（历史重复声明已收敛，见各服务迁移提交）。
//
// 生产者 → 消费者 对照见各常量注释；stream 归属见 streams.go。

// ---- 采集监控域（collector-snmp-ssh / pkg/collector 发布，stream: MONITOR）----
const (
	// SubjectMonitorDataCollect 采集指标数据。生产者 pkg/collector；消费者 monitoralarm（规则匹配）、visualization（节点摘要）。
	SubjectMonitorDataCollect = "monitor.data.collect"
	// SubjectDeviceStatus 设备在线状态变更。生产者 pkg/collector；消费者 monitoralarm、visualization。
	SubjectDeviceStatus = "device.status"
	// SubjectCollectorSnmpTrap SNMP Trap 事件。生产者 collector-snmp-ssh；消费者 monitoralarm（转告警）、linemonitor（配置事件）。
	SubjectCollectorSnmpTrap = "collector.snmp.trap"
)

// ---- 告警域（monitoralarm 发布，stream: ALARM_EVENTS）----
const (
	// SubjectAlarmEvents 告警事件（信封版本 v2.0）。生产者 monitoralarm（唯一）；
	// 消费者 moniternotice、aiops、remediation、visualization、monitoralarm 通知分发器。
	SubjectAlarmEvents = "alarm.events"
)

// ---- 通知域（moniternotice 发布，stream: NOTICE）----
const (
	// SubjectNoticeSiteMsg 站内消息。生产者 moniternotice；消费者 admin（写站内消息表）。
	SubjectNoticeSiteMsg = "notice.site_msg"
)

// ---- 线路监控域（linemonitor 发布，stream: LINE）----
const (
	// SubjectLineStatus 线路实时状态快照。消费者 monitoralarm、visualization。
	SubjectLineStatus = "line.status"
	// SubjectLineEvents 线路事件（创建/恢复）。消费者 monitoralarm、visualization。
	SubjectLineEvents = "line.events"
	// SubjectLineDiscoveryStatus 发现主机四态快照（拓扑大屏）。消费者 visualization。
	SubjectLineDiscoveryStatus = "line.discovery.status"
	// SubjectLineInventoryDraft 连线盘点草稿批次事件（31-连线盘点 P2：周期扫描/每日对账
	// 产出待确认草稿时发布，批次级一条含计数与样本）。消费者 framework-web 提醒/IT 监控采集页。
	SubjectLineInventoryDraft = "line.inventory.draft"
)

// ---- 气象域（metar/radar/satellite/windfield 发布，stream: WEATHER）----
const (
	SubjectWeatherMetarData   = "weather.metar.data"   // 新报文。消费者 visualization、monitoralarm（到达监控）
	SubjectWeatherMetarStatus = "weather.metar.status" // 拉取任务状态。消费者 visualization、monitoralarm

	SubjectWeatherTafData   = "weather.taf.data"   // 新 TAF 报文。消费者 visualization、monitoralarm（到达监控）
	SubjectWeatherTafStatus = "weather.taf.status" // TAF 拉取任务状态。消费者 visualization、monitoralarm

	// ---- 25-8 预报服务 TAF 编发（forecast 服务发布；与 weather.taf.* 外部采集分流；沿用 awos.* AWOS 域前缀治理）----
	SubjectWeatherForecastData   = "awos.taf.data"   // 本场新发布 TAF（新报/修订/取消）。消费者 visualization ws 透传、7-报文查询留档
	SubjectWeatherForecastRemind = "awos.taf.remind" // 漏发/未入库提醒。消费者 visualization ws 透传（右下角弹窗+声音）
	SubjectWeatherForecastArchive = "awos.taf.archive" // 入库确认（回报匹配成功）。消费者 visualization ws 透传（提示音）

	SubjectWeatherRadarData      = "weather.radar.data"      // 新数据文件。消费者 visualization
	SubjectWeatherRadarStatus    = "weather.radar.status"    // 拉取任务状态。消费者 visualization、monitoralarm
	SubjectWeatherRadarFreshness = "weather.radar.freshness" // 时效性告警。消费者 monitoralarm（转 alarm_events type=radar_delay）
	SubjectWeatherRadarProduct   = "weather.radar.product"   // 派生产品就绪。消费者 UE5/framework-web
	// SubjectRadarEchoAlert 回波告警（词表归 WEATHER stream 但主题不在 weather.> 下，stream 声明需单列）。
	SubjectRadarEchoAlert = "radar.echo.alert" // 消费者 monitoralarm

	SubjectWeatherSatelliteData   = "weather.satellite.data"   // 新云图。消费者 visualization、monitoralarm（到达监控）
	SubjectWeatherSatelliteStatus = "weather.satellite.status" // 拉取状态。消费者 visualization、monitoralarm
	SubjectWeatherSatelliteAlert  = "weather.satellite.alert"  // 拉取异常告警。消费者 monitoralarm

	SubjectWeatherWindFieldData    = "weather.windfield.data"    // 新风场文件。消费者 visualization、monitoralarm（到达监控）
	SubjectWeatherWindFieldStatus  = "weather.windfield.status"  // 拉取状态。消费者 visualization、monitoralarm
	SubjectWeatherWindFieldArrival = "weather.windfield.arrival" // 到达 slot 事件。消费者 monitoralarm（T8 告警桥）
	SubjectWeatherWindFieldProduct = "weather.windfield.product" // 派生产品就绪。消费者 UE5/framework-web

	// 风切变（30）。生产者 app/windshear。frame 给画面，alert 只在 raise/clear 时发给 monitoralarm。
	// 主题在 weather.> 下，不改 streams.go。
	SubjectWeatherWindShearFrame = "weather.windshear.frame"
	SubjectWeatherWindShearAlert = "weather.windshear.alert"
)

// ---- 航迹域（flight 发布，stream: FLIGHT）----
const (
	SubjectFlightData   = "flight.data"   // 航班位置批次。消费者 visualization、monitoralarm（到达监控）
	SubjectFlightStatus = "flight.status" // 轮询状态。消费者 visualization、monitoralarm
)

// ---- 运维工具域（opstools 发布，stream: OPSTOOLS）----
const (
	SubjectOpstoolsInspectionCompleted = "opstools.inspection.completed" // 巡检完成。消费者 monitoralarm
	SubjectOpstoolsDiscoveryCompleted  = "opstools.discovery.completed"  // 扫描完成。消费者 monitoralarm
	SubjectOpstoolsDiscoveryNewHost    = "opstools.discovery.newhost"    // 新主机。消费者 monitoralarm
	SubjectOpstoolsSessionOpened       = "opstools.session.opened"       // 远程会话建立。消费者 framework-web（在线远程徽标）
	SubjectOpstoolsSessionClosed       = "opstools.session.closed"       // 远程会话断开/失败。消费者 framework-web（在线远程徽标）
	// SubjectOpstoolsDbMonitorMetric 数据库连接周期采集级别迁移事件（16-03 M3）。
	// 生产者 opstools DbMonitorScheduler（仅在级别迁移/建连失败/恢复时发，迁移矩阵见详设 §5.3）；
	// 消费者 monitoralarm（转告警，metric_key=db_conn{id}_{key} 防跨连接误恢复）。
	SubjectOpstoolsDbMonitorMetric = "opstools.dbmonitor.metric"
)

// ---- 备份运维域（ops 发布，core NATS 发布，stream: OPS 捕获）----
const (
	SubjectOpsBackupCompleted = "ops.backup.completed" // 备份/恢复完成。消费者 moniternotice
	SubjectOpsStorageAlert    = "ops.storage.alert"    // 备份存储告警。消费者 monitoralarm
)

// ---- AI 运维域（aiops 发布，stream: AI）----
const (
	SubjectAiApprovalPending       = "ai.approval.pending"        // 审批待处理。消费者 通知侧
	SubjectAiApprovalTimeoutRemind = "ai.approval.timeout_remind" // 审批超时提醒
	SubjectAiApprovalDecided       = "ai.approval.decided"        // 审批结论
	SubjectAiAnalysisCompleted     = "ai.analysis.completed"      // AI 分析完成。消费者 monitoralarm（回写）、remediation
	SubjectAiTaskEvents            = "ai.task.events"             // 任务状态推进。消费者 remediation
	SubjectAiAraneaHealth          = "ai.aranea.health"           // Aranea 健康变更
)

// ---- 自动修复域（remediation 发布，stream: REMEDIATION）----
const (
	SubjectRemediationEvents           = "remediation.events"            // 执行状态事件
	SubjectRemediationApprovalRequired = "remediation.approval.required" // 审批请求
)

// ---- 视频域（video 发布，stream: VIDEO）----
const (
	// SubjectVideoEvents AI 布控命中事件（信封 event_type=video.analytic.hit）。消费者 monitoralarm（转告警）、video 联动执行器。
	SubjectVideoEvents = "video.events"
	// EventTypeVideoAnalyticHit video.events 信封 event_type。
	EventTypeVideoAnalyticHit = "video.analytic.hit"
)

// ---- 资产域（admin 发布，stream: ASSET / INVENTORY / WAREHOUSE）----
const (
	SubjectAssetCreated       = "asset.created"         // 消费者 admin 库存侧（兜底清理）
	SubjectAssetScrapped      = "asset.scrapped"        // 消费者 admin 库存侧
	SubjectAssetMaintenance   = "asset.maintenance"     // 消费者 admin 库存侧
	SubjectAssetStatusChanged = "asset.status.changed"  // 消费者 admin 库存侧
	SubjectAssetDeleted       = "asset.deleted"         // 消费者 admin 库存侧
	SubjectAssetGroupChanged  = "asset.group.changed"   // 消费者 admin 库存侧
	SubjectAssetTagsChanged   = "asset.tags.changed"    // 消费者 admin 库存侧
	SubjectAssetProtocolChanged = "asset.protocol.changed" // 消费者：collector-snmp-ssh（协议热加载）；monitorquery（declared_metrics→字典 stub，v1.1）
	// SubjectCommnavConfigChanged 通信导航业务链配置变更（26-通信导航监视 M2.3）。
	// 生产者：admin（链 CRUD 成功后发布）；消费者：monitorquery（链配置 TTL 缓存失效）。
	SubjectCommnavConfigChanged = "commnav.config.changed"
	SubjectAssetDCChanged     = "asset.dc.changed"      // 机房变更

	// SubjectAssetRunwayEndBindingChanged 跑道端设备绑定变更（2026-09 审计修复 T5）。
	// 生产者：admin（runway_end_device_bindings Create/Update/Delete 成功后发布，失败写重试队列）。
	// 载荷：{action: created|updated|deleted, binding_id, runway_end_id, device_id, role, status,
	//       position_m, lateral_m}；lateral_m 符号约定与可视化域一致（负值左侧/正值右侧）。
	// 消费者：visualization（同步镜像到 amos_device_rwy_end_relations：device_id 定位/创建
	// amos_device_configs，按 (amos_device_id, runway_end_id) upsert 位置字段；deleted 或
	// status=OFF 时软删关联）。ASSET stream（asset.>）已捕获，无需 stream 变更。
	SubjectAssetRunwayEndBindingChanged = "asset.runway_end_binding.changed"

	// SubjectInventoryAlertPrefix 库存预警（按级别拼接：inventory.alert.{level}）。消费者 monitoralarm（inventory.alert.*）。
	SubjectInventoryAlertPrefix = "inventory.alert."
	// SubjectInventoryAlertWildcard monitoralarm 订阅词。
	SubjectInventoryAlertWildcard = "inventory.alert.*"

	SubjectWarehouseEnvAlarm = "warehouse.env.alarm" // 库房环境告警。消费者 monitoralarm
)

// ---- 安全域（admin 发布，stream: SECURITY）----
const (
	// SubjectSecurityLoginAbuse 撞库/暴力破解告警（登录失败计数触发锁定时发布）。
	// 生产者 admin（认证链路）；消费者 monitoralarm（转告警）。
	SubjectSecurityLoginAbuse = "security.login.abuse"
)

// ---- 许可证域（admin 发布，core NATS）----
const (
	SubjectLicenseActivated = "license.activated" // 消费者 gateway
	SubjectLicenseRevoked   = "license.revoked"   // 消费者 gateway
	SubjectLicenseExpiring  = "license.expiring"
)

// ---- 虚拟化域（virtman 发布，core NATS）----
const (
	SubjectVirtmanStatus = "virtman.status"
)

// ---- 可视化域（visualization 发布，core NATS）----
const (
	SubjectSceneChanged = "scene.changed"
)

// ---- 值班管理域（admin 发布，core NATS；排班变更管理设计 §6）----
const (
	SubjectScheduleChanged = "schedule.changed" // 排班变更。消费者 admin ScheduleNotifySubscriber（站内信）
)

// ---- AWOS 自动观测域（awos-collector 发布，stream: AWOS；见 docs/开发文档/25-民用航空自动观测系统）----
//
// 消费者现状（2026-10 数据中心化改造）：awos.met.data 生产消费者 = awos-pool（compute/obsbook，
// D12 退役）+ awos-datacenter（ingest，2b D3 起）+ collector-snmp-ssh awos_bridge（D12 下线）；
// awos.device.status 消费者 = awos-pool obsbook + awos-datacenter（devstate 状态项）；
// awos.raw.data 暂无消费者（仅归档规划）。
const (
	// SubjectAwosRawData 原始帧（含线路标签与生效标记，base64 载荷）。消费者：暂无（规划 awos-pool、归档）。
	SubjectAwosRawData = "awos.raw.data"
	// SubjectAwosMetData 标准要素帧（内部数据交互格式，仅生效线路产出）。
	// 消费者：collector-snmp-ssh awos_bridge（迁移期）、awos-pool compute（派生入窗）/obsbook（记录簿）。
	SubjectAwosMetData = "awos.met.data"
	// SubjectAwosDeviceStatus 线路活跃状态变更。
	// 消费者：collector-snmp-ssh awos_bridge（聚合组在线状态→设备状态链）、awos-pool obsbook（记录簿）；规划 前端展示。
	SubjectAwosDeviceStatus = "awos.device.status"
	// SubjectAwosDeviceAlarm 自观设备告警状态迁移（raise/clear；2026-09 审计修复 T2）。
	// 生产者：collector-snmp-ssh awos_bridge（从 met 帧 alarms 差分得到——met 帧每帧重发全部
	// 命中告警，桥内按 device+alarm_id+code[:bit] 维护活跃集，新键 raise、消失键 clear）。
	// 消费者：monitoralarm（raise→alarm.events 创建、clear→恢复；保持 alarm.events 唯一生产者）、
	// awos-pool obsbook（记录簿留痕）。
	SubjectAwosDeviceAlarm = "awos.device.alarm"
	// SubjectAwosLinkActive 当前生效链路广播。消费者：暂无（规划 前端、日志）。
	SubjectAwosLinkActive = "awos.link.active"
	// SubjectAwosLinkSwitch 链路切换留痕事件（业务事件，非告警）。消费者：暂无（规划 日志/报文查询）。
	SubjectAwosLinkSwitch = "awos.link.switch"
	// SubjectAwosLineSwitchCmd 人工线路切换命令（core NATS，不持久化；queue group awos-collector）。
	SubjectAwosLineSwitchCmd = "awos.lineswitch.cmd"
	// SubjectAwosConfigChanged 配置变更广播（awos-admin 发布；core NATS，不持久化）。
	// 载荷 {domain, id, version, at}；消费者 awos-collector（domain=checksum_scripts→scriptsync、
	// message_formats/parse_rules/alarm_mappings→formatsync、line_groups→groupsync）。
	SubjectAwosConfigChanged = "awos.config.changed"

	// ---- AWOS 阶段 2 计算池（awos-pool 发布/消费；见 docs/开发文档/AWOS阶段2计算池服务设计-2026-09）----
	// SubjectAwosMetDerived 派生要素帧（10min 均值/阵风/RVR 趋势/风向矢量均值）。
	// Deprecated（D12，2-计算数据中心池概要设计 §8）：awos-pool compute 退役后无生产者；
	// report 消费源已切 awos.dc.data.>（source=derived）。常量保留仅作历史登记，新代码勿用。
	SubjectAwosMetDerived = "awos.met.derived"
	// SubjectAwosReportMetar 航空气象报文（METAR/SPECI，ICAO Annex 3）。
	// 生产者 awos-pool report；消费者 航空气象发报系统/归档（规划）。
	SubjectAwosReportMetar = "awos.report.metar"
	// SubjectAwosReportIssued 报文签发留痕事件（3-观测发报详细设计 §1.6）。
	// 生产者 awos-pool report（每份签发报文归档 awos_reports 后发布）；
	// 载荷 {seq, icao_code, type, body, trigger, mode, operator, correction_of, issued_at, observed_at}；
	// 消费者 9-对外数据转发 / 7-报文查询（规划）。落入 AWOS stream（awos.report.> 已覆盖）。
	SubjectAwosReportIssued = "awos.report.issued"

	// ---- AWOS 阶段 2b 数据中心（awos-datacenter / awos-compute；stream: AWOS_DC 短保留 30min）----
	// 词表唯一源本文件；键规范/subject 映射语义见 pkg/awos（SubjectDCPrefix 为下方常量的别名）。
	// SubjectAwosDCDataPrefix 数据项实时推送前缀：subject=awos.dc.data.{key}（详细设计 §2.4）。
	// 生产者 awos-datacenter dispatch；消费者 awos-compute subscribe（键模式订阅）、5-/3-模块（D12）。
	SubjectAwosDCDataPrefix = "awos.dc.data."
	// SubjectAwosDCDataAll 数据项推送全量通配（订阅用；JetStream durable 消费受 30min 保留约束）。
	SubjectAwosDCDataAll = "awos.dc.data.>"
	// SubjectAwosDCWrite 计算回写（awos-compute → awos-datacenter ingest.HandleWriteback）。
	// JetStream queue group awos-datacenter（多实例互斥消费）；幂等键=key+observed_at（60s 窗口）。
	SubjectAwosDCWrite = "awos.dc.write"
	// SubjectAwosQualityEvents 质量事件（状态迁移/丢帧/seq reset/迟到，详细设计 §5）。
	// 生产者 awos-datacenter quality（D5）；载荷 {key, from, to, reason, at}；
	// 落入 AWOS_DC stream（30min 短保留，断线补投用）。
	SubjectAwosQualityEvents = "awos.quality.events"

	// ---- AWOS 对外转发（awos-forward 发布；stream: AWOS；9-对外数据转发服务概要设计 §2）----
	// SubjectAwosForwardStatus 转发状态留痕事件。
	// 生产者 awos-forward；载荷 {plan_id, rule_id, channel_id, kind(sent/failed/timeout/recovered/dead),
	// attempts, error, at}；消费者：报文查询关联回执（规划）、运维页。
	// 注意：本主题为状态留痕，非告警——告警生命周期仍唯一归 monitoralarm（自观零告警红线）。
	SubjectAwosForwardStatus = "awos.forward.status"
)
