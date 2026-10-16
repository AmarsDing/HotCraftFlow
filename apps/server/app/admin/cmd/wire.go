//go:build wireinject
// +build wireinject

package main

import (
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/data"
	"framework/app/admin/internal/server"
	"framework/app/admin/internal/service"
	"framework/app/admin/internal/worker"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// wireApp init kratos application.
// 公共配置类型（Data/Nats/Registry/Trace/Log）已在根 conf/pb 包定义，
// core.NewCoreServer 直接接收 *conf.Bootstrap，内部取 bc.GetNats()，无需额外 provider。
func wireApp(bc *conf.Bootstrap, logger log.Logger, tp *tracesdk.TracerProvider) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		service.ProviderSet,
		biz.ProviderSet,
		data.ProviderSet,
		core.ProviderSet,
		worker.ProviderSet,
		newApp,
	))
}
