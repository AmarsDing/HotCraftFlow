package worker

import (
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// JetStream 订阅投递/确认策略（毒消息防无限重投加固，口径与 monitoralarm jetstream_policy.go 一致）：
//   - MaxDeliver=10：业务失败 NAK 重投上限（超限后 JetStream 停止投递）
//   - AckWait=60s：略大于 handler 处理超时（30s），确保"先超时返回再重投"
//
// 格式错误（反序列化失败）的毒消息统一 Warn 日志后 ACK 丢弃（重投无意义）。
// 订阅点的 durable 与 DeliverAll/DeliverNew 由调用方自带。
const (
	jsMaxDeliver = 10
	jsAckWait    = 60 * time.Second
)

// jsSubscribeOpts 订阅选项公共件（ManualAck + MaxDeliver + AckWait）。
func jsSubscribeOpts() []natsgo.SubOpt {
	return []natsgo.SubOpt{
		natsgo.ManualAck(),
		natsgo.MaxDeliver(jsMaxDeliver),
		natsgo.AckWait(jsAckWait),
	}
}
