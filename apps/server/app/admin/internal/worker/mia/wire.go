package miaworker

import (
	"github.com/google/wire"

	bizmia "framework/app/admin/internal/biz/mia"
)

// ProviderSet worker/mia 注入清单：影响卡发布器、intel 消费者与双调度器。
// ImpactPublisher 端口经 wire.Bind 绑定（NATS 是唯一发布通道；
// data 层无此实现，避免与行情/情感适配器混装）。
var ProviderSet = wire.NewSet(
	NewNatsImpactPublisher,
	NewIntelSubscriber,
	NewMiaEquityScheduler,
	NewMiaCryptoScheduler,

	wire.Bind(new(bizmia.ImpactPublisher), new(*NatsImpactPublisher)),
)
