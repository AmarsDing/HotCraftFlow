package mmeworker

import (
	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/pkg/events"
	"framework/pkg/natsx"
)

// ensureStreamsWith js 上下文上幂等确保指定 stream（词表 events.StreamDefs 唯一真相，
// 与 worker/mia、worker/tt 完全同型——失败仅告警不阻塞启动）。
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
