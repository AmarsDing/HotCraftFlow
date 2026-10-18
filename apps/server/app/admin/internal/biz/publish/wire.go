package publish

import "github.com/google/wire"

// ProviderSet biz/publish providers（⑤ MAP 媒体自动化发布矩阵）。
var ProviderSet = wire.NewSet(
	NewApprovalGateUsecase,
	NewIntakeUsecase,
	NewPublishUsecase,
	NewDispatchUsecase,
	NewChannelUsecase,
	NewAccountUsecase,
	NewBreakerUsecase,
	NewQuotaUsecase,
)
