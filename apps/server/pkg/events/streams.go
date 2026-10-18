package events

import (
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// StreamDef JetStream stream 定义（词表唯一直相）。
// cmd/nats_setup 据此预建；publisher 启动期幂等 ensure 也以此为准，二者不再各自维护。
type StreamDef struct {
	Name     string
	Subjects []string
	MaxAge   time.Duration
}

// StreamDefs 全站 stream 词表（与 cmd/nats_setup 历史声明逐一对应，MaxAge 保持原值）。
//
// 注意：ops.backup.completed 等 core NATS 发布也会被同名 stream 按 subject 捕获，
// 与发布走 core 还是 JetStream API 无关。
var StreamDefs = []StreamDef{
	// collector.snmp.trap 由 collector publisher 启动时幂等并入本流，须保持声明一致。
	{Name: "MONITOR", Subjects: []string{"monitor.data.>", SubjectDeviceStatus, SubjectCollectorSnmpTrap}, MaxAge: 72 * time.Hour},
	{Name: "ALARM_EVENTS", Subjects: []string{"alarm.>"}, MaxAge: 168 * time.Hour},
	{Name: "NOTICE", Subjects: []string{"notice.>"}, MaxAge: 168 * time.Hour},
	{Name: "LINE", Subjects: []string{"line.>"}, MaxAge: 168 * time.Hour},
	{Name: "INVENTORY", Subjects: []string{"inventory.>"}, MaxAge: 168 * time.Hour},
	{Name: "WAREHOUSE", Subjects: []string{"warehouse.>"}, MaxAge: 168 * time.Hour},
	{Name: "ASSET", Subjects: []string{"asset.>"}, MaxAge: 168 * time.Hour},
	// 通信导航业务链配置变更（26 M2.3）；subject 不在 asset.> 下，独立流以免改 ASSET 通配。
	{Name: "COMMNAV", Subjects: []string{"commnav.>"}, MaxAge: 168 * time.Hour},
	// radar.echo.alert 不在 weather.> 下，须单列。
	{Name: "WEATHER", Subjects: []string{"weather.>", SubjectRadarEchoAlert}, MaxAge: 168 * time.Hour},
	{Name: "OPSTOOLS", Subjects: []string{"opstools.>"}, MaxAge: 168 * time.Hour},
	{Name: "REMEDIATION", Subjects: []string{"remediation.>"}, MaxAge: 168 * time.Hour},
	{Name: "FLIGHT", Subjects: []string{"flight.>"}, MaxAge: 168 * time.Hour},
	{Name: "AI", Subjects: []string{"ai.>"}, MaxAge: 168 * time.Hour},
	{Name: "OPS", Subjects: []string{"ops.>"}, MaxAge: 168 * time.Hour},
	{Name: "VIDEO", Subjects: []string{"video.>"}, MaxAge: 168 * time.Hour},
	{Name: "SECURITY", Subjects: []string{"security.>"}, MaxAge: 168 * time.Hour},
	{Name: "SCHEDULE", Subjects: []string{"schedule.>"}, MaxAge: 168 * time.Hour},
	// AWOS 自动观测域。注意逐前缀列举：awos.lineswitch.cmd 为人工切换命令（core NATS 语义），不入流持久化。
	// awos.met.derived（阶段 2 计算池派生要素）落入 awos.met.>；awos.report.> 为报文生成产物；
	// awos.forward.> 为对外转发留痕事件（awos-forward 发布，9-对外数据转发服务概要设计 §2）。
	{Name: "AWOS", Subjects: []string{"awos.raw.>", "awos.met.>", "awos.device.>", "awos.link.>", "awos.report.>", "awos.forward.>"}, MaxAge: 168 * time.Hour},
	// AWOS 阶段 2b 数据中心（2-计算数据中心池详细设计 §2.4）：短保留 30min——定位订阅者短暂
	// 断线补投 + 冷启动补窗，非历史存储（历史落 awos-pb awos_items）。awos.config.changed 为
	// core NATS 瞬时广播，不入流。
	{Name: "AWOS_DC", Subjects: []string{"awos.dc.>", "awos.quality.>"}, MaxAge: 30 * time.Minute},

	// ---- 媒体矩阵域（HotCraftFlow 五子系统，契约 00-shared/01-data-contract.md §8.2）----
	// MM_INTEL：① IC 原始片段与归一化产出（mm.intel.>），保留 90d。
	// 生产者 IC；消费者 TT（全量 normalized）、MIA（C_market）、MME（B_material）、人工队列（blocked）。
	{Name: "MM_INTEL", Subjects: []string{"mm.intel.>"}, MaxAge: 90 * 24 * time.Hour},
	// MM_TREND：② TT 快照/突发/选题（mm.trend.>），保留 30d。
	// 生产者 TT；消费者 TT 时序写入、推送服务、MME/MIA。
	{Name: "MM_TREND", Subjects: []string{"mm.trend.>"}, MaxAge: 30 * 24 * time.Hour},
}

// ToNatsConfig 转为 nats.go StreamConfig（FileStorage + LimitsPolicy 全站统一）。
func (d StreamDef) ToNatsConfig() *natsgo.StreamConfig {
	return &natsgo.StreamConfig{
		Name:      d.Name,
		Subjects:  append([]string{}, d.Subjects...),
		Storage:   natsgo.FileStorage,
		Retention: natsgo.LimitsPolicy,
		MaxAge:    d.MaxAge,
	}
}
