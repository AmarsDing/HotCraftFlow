package publishadapters

import (
	"github.com/google/wire"

	bizpub "framework/app/admin/internal/biz/publish"
)

// ProviderSet publishadapters 注入清单（⑤ MAP 适配器注册表 + 能力目录）。
var ProviderSet = wire.NewSet(
	ProvideRegistry,
	NewPlatformCatalog,

	wire.Bind(new(bizpub.AdapterLookup), new(*Registry)),
	wire.Bind(new(bizpub.CapabilityProvider), new(*PlatformCatalog)),
)
