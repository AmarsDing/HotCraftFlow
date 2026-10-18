// Package miaworker MIA 市场影响分析的 NATS 运行时与定时调度（03-MIA 概要设计 §5 部署视图：
// intel 接入消费与双调度器均为 kratos transport.Server，失败相互隔离）。
package miaworker

import (
	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/pkg/events"
	"framework/pkg/natsx"
)

// durable 消费名（03-MIA：消费进度独立于 TT，互不影响重投）。
const durableIngestIntel = "mia-ingest-intel"

// ensureStreamsWith js 上下文上幂等确保指定 stream（词表 events.StreamDefs 唯一真相）。
// 失败仅告警不阻塞启动（与 core.CoreServer.ensureStreams / ttworker 口径一致）。
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
