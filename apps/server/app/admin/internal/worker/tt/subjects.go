// Package ttworker TT 热点追踪的 NATS 运行时（02-TT 概要设计 §4 部署视图：
// 接入消费 / 时序写入 / 缓冲重放 三个长驻 worker，均为 kratos transport.Server）。
package ttworker

import (
	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/pkg/events"
	"framework/pkg/natsx"
)

// durable 消费名（02-TT 开发进度 M1：消费进度独立于其他子系统）。
const (
	durableIngestIntel = "tt-ingest-intel"
	durableTSWriter    = "tt-ts-writer"
)

// ensureStreamsWith js 上下文上幂等确保指定 stream（词表 events.StreamDefs 唯一真相）。
// 失败仅告警不阻塞启动（core 发布回执缺失语义与 core.CoreServer.ensureStreams 一致）。
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
