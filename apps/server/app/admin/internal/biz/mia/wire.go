package mia

import (
	"github.com/google/wire"
)

// ProviderSet biz/mia 注入清单。
//
// ResolveConfig 直接作 provider（*conf.Bootstrap → Config，mia.yaml 解析）；
// IngestUsecase 随采集/归一 worker 落位（⑤ M2 之后）再进集合。
var ProviderSet = wire.NewSet(
	ResolveConfig,
	NewEventUsecase,
	NewMappingUsecase,
	NewImpactUsecase,
	NewRuleUsecase,
	NewNotifier,
	NewIngestUsecase,
)
