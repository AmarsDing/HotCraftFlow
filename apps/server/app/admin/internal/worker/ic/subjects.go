// Package icworker ① IC 情报收集的 NATS/asynq 运行时（01-IC 概要设计部署视图：
//   - Scheduler：asynq 周期任务（信源采集 / B-25 重发 / S-2 闭簇 / 保留期清扫）
//   - NormalizerConsumer：mm.intel.raw 持久消费，驱动归一化↔去重↔闸门↔发布链路
//   - Publisher：三条契约 subject 的 JetStream 发布（msg-id = intel_id，B-25）
//
// 全部为 kratos transport.Server 形态，启动失败降级告警不阻塞服务（与 tt/mia 同纪律）。
package icworker

import (
	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/pkg/events"
	"framework/pkg/natsx"
)

// durable 消费名：消费进度独立于其他子系统（01-IC M1 约定）。
const durableNormalizer = "ic-normalizer"

// asynq 任务类型名（即 task_repo.go 白名单项；任务中心可见便于人工重跑/观测）。
const (
	taskCollect          = "ic.collect"
	taskRepublishPending = "ic.republish"
	taskClusterIdleClose = "ic.cluster_idle_close"
	taskRetentionSweep   = "ic.retention_sweep"
)

// 全局周期任务的固定 taskID（信源采集任务按 sourceID 派生，见 schedule.go）。
const (
	jobRepublishPending = "ic:job:republish"
	jobClusterIdleClose = "ic:job:cluster_idle_close"
	jobRetentionSweep   = "ic:job:retention_sweep"
)

// ensureStreamsWith 幂等确保 stream（词表 events.StreamDefs 唯一真相）。
// 失败仅告警不阻塞启动（与 core.CoreServer.ensureStreams 同语义；worker/tt 同款，
// 不共享的原因：worker 子包刻意不互 import——各子系统可独立裁剪部署）。
func ensureStreamsWith(js natsgo.JetStreamContext, l *log.Helper, names ...string) {
	for _, def := range events.StreamDefs {
		for _, want := range names {
			if def.Name != want {
				continue
			}
			if err := natsx.EnsureStream(js, def, l); err != nil {
				l.Warnf("ensure stream %s failed: %v", def.Name, err)
			}
		}
	}
}
