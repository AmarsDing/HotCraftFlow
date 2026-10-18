package ttworker

import (
	"github.com/google/wire"

	biztt "framework/app/admin/internal/biz/tt"
)

// ProviderSet worker/tt 注入清单：快照发布器与三个长驻 transport.Server。
// SnapshotPublisher 端口经 wire.Bind 绑定（NATS 是唯一发布通道）。
var ProviderSet = wire.NewSet(
	NewNatsSnapshotPublisher,
	NewIngestSubscriber,
	NewSnapshotWriter,
	NewBufferReplay,

	wire.Bind(new(biztt.SnapshotPublisher), new(*NatsSnapshotPublisher)),
)
