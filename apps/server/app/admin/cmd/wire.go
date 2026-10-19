//go:build wireinject
// +build wireinject

package main

import (
	"framework/app/admin/internal/biz"
	bizmia "framework/app/admin/internal/biz/mia"
	iccollector "framework/app/admin/internal/collector/ic"
	bizmme "framework/app/admin/internal/biz/mme"
	bizpublish "framework/app/admin/internal/biz/publish"
	biztt "framework/app/admin/internal/biz/tt"
	"framework/app/admin/internal/conf"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/data"
	pubdata "framework/app/admin/internal/data/publish"
	ttdata "framework/app/admin/internal/data/tt"
	"framework/app/admin/internal/publishadapters"
	"framework/app/admin/internal/server"
	"framework/app/admin/internal/service"
	"framework/app/admin/internal/worker"
	icworker "framework/app/admin/internal/worker/ic"
	mapworker "framework/app/admin/internal/worker/map"
	miaworker "framework/app/admin/internal/worker/mia"
	mmeworker "framework/app/admin/internal/worker/mme"
	ttworker "framework/app/admin/internal/worker/tt"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// wireApp init kratos application.
// 公共配置类型（Data/Nats/Registry/Trace/Log）已在根 conf/pb 包定义，
// core.NewCoreServer 直接接收 *conf.Bootstrap，内部取 bc.GetNats()，无需额外 provider。
func wireApp(bc *conf.Bootstrap, ttb *biztt.Conf, mc *bizpublish.Conf, mpconfs publishadapters.PlatformConfMap, logger log.Logger, tp *tracesdk.TracerProvider) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		service.ProviderSet,
		biz.ProviderSet,
		data.ProviderSet,
		core.ProviderSet,
		worker.ProviderSet,
		// ② TT 热点追踪（M1：接入消费 / 时序写入 / 缓冲重放）
		biztt.ProviderSet,
		ttdata.ProviderSet,
		ttworker.ProviderSet,
		// ③ MIA 市场影响（查询服务链 + NATS 消费/双调度）
		bizmia.ProviderSet,
		miaworker.ProviderSet,
		// ⑤ MAP 媒体发布矩阵（审批门/账号池/适配器注册表/排期到点发；mc/mpconfs 由 main 从 map.yaml/platforms.yaml scan）
		bizpublish.ProviderSet,
		pubdata.ProviderSet,
		publishadapters.ProviderSet,
		mapworker.ProviderSet,
		// ④ MME 短视频矩阵混剪（任务/素材/展开/渲染规划/QC/差异化/分发）
		bizmme.ProviderSet,
		//   worker 侧：mme_render 专用 asynq 队列 + 契约事件发布（TaskEnqueuer/EventPublisher 真实 adapter）
		mmeworker.ProviderSet,
		// ① IC 情报收集（五路采集器 + 端口适配器 / 调度器 + mm.intel.raw 消费 + JetStream 发布）
		iccollector.ProviderSet,
		icworker.ProviderSet,
		newApp,
	))
}
