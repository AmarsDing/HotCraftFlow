package worker

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewLogRetentionWorker,
	NewSiteMsgSubscriber,
)
