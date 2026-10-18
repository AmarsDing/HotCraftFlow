package pubdata

import (
	"github.com/google/wire"

	bizpub "framework/app/admin/internal/biz/publish"
)

// ProviderSet data/publish 注入清单（⑤ MAP 持久化与横切端口；biz 端口经 wire.Bind 绑定）。
// 独立子包，不触碰 data.ProviderSet——并行开发期间保持纯增量。
var ProviderSet = wire.NewSet(
	NewEntChannelRepo,
	NewEntAccountRepo,
	NewEntDispatchRepo,
	NewEntApprovalRepo,
	NewEntReceiptRepo,
	NewEntPerformanceRepo,
	NewMapCredentialVault,
	NewRedisPublishLocker,

	wire.Bind(new(bizpub.ChannelRepo), new(*EntChannelRepo)),
	wire.Bind(new(bizpub.AccountRepo), new(*EntAccountRepo)),
	wire.Bind(new(bizpub.DispatchRepo), new(*EntDispatchRepo)),
	wire.Bind(new(bizpub.ApprovalRepo), new(*EntApprovalRepo)),
	wire.Bind(new(bizpub.ReceiptRepo), new(*EntReceiptRepo)),
	wire.Bind(new(bizpub.PerformanceRepo), new(*EntPerformanceRepo)),
	wire.Bind(new(bizpub.CredentialVault), new(*MapCredentialVault)),
	wire.Bind(new(bizpub.PublishLocker), new(*RedisPublishLocker)),
)
