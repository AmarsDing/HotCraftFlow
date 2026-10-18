package icworker

import (
	"github.com/google/wire"

	"framework/app/admin/internal/biz"
)

// ProviderSet worker/ic 注入清单：发布器（端口绑定）+ 归一化消费者 + 调度器，
// 均满足 transport.Server 生命周期（kratos 统一 Start/Stop）。
var ProviderSet = wire.NewSet(
	NewNatsIntelPublisher,
	NewNormalizerConsumer,
	NewScheduler,

	wire.Bind(new(biz.IntelPublisherPort), new(*NatsIntelPublisher)),
)
