package mapworker

import (
	"github.com/google/wire"

	bizpub "framework/app/admin/internal/biz/publish"
)

// ProviderSet worker/map 注入清单：排期 worker 满足 transport.Server 生命周期，
// 复用共享 asynq server；发布执行器绑定到 biz 用例；表现回流 worker 同型注册。
var ProviderSet = wire.NewSet(
	NewScheduleWorker,
	NewAnalyticsWorker,

	wire.Bind(new(PubRunner), new(*bizpub.PublishUsecase)),
)
