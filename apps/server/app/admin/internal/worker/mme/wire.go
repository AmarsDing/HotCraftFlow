package mmeworker

import (
	"github.com/google/wire"

	bizmme "framework/app/admin/internal/biz/mme"
)

// ProviderSet worker/mme 注入清单：mme_render 队列、渲染 worker、
// TaskEnqueuer 真实 adapter、NATS 契约发布器。
// 端口经 wire.Bind 绑定（NATS 是唯一发布通道；data 层无此实现——同 worker/mia 口径）。
var ProviderSet = wire.NewSet(
	NewRenderQueue,
	NewMmeRenderWorker,
	NewMmeTaskEnqueuer,
	NewNatsVariantPublisher,

	wire.Bind(new(bizmme.TaskEnqueuer), new(*MmeTaskEnqueuer)),
	wire.Bind(new(bizmme.EventPublisher), new(*NatsVariantPublisher)),
)
