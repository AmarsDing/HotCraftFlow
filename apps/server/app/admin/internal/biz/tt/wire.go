package tt

import "github.com/google/wire"

// ProviderSet biz/tt providers。
var ProviderSet = wire.NewSet(
	NewIngestUsecase,
	NewWriterUsecase,
	NewHeatEnricher,
	NewWeightUsecase,
	NewClusterUsecase,
	NewTopicAdminUsecase,
	NewLifecycleEvaluator,
)
