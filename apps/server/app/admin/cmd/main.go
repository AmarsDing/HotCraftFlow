package main

import (
	"context"
	"flag"
	"os"

	"framework/app/admin/internal/conf"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/server"
	"framework/app/admin/internal/worker"
	pkglog "framework/pkg/log"
	"framework/pkg/observability"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/registry"
	"github.com/go-kratos/kratos/v2/selector"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/http"

	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	wb "framework/pkg/weight"
)

var flagconfCommon string
var flagconfPrivate string

// Name 是当前服务名，用于 kratos.App 注册与 trace ServiceName（对齐 v1 包级 Name 变量）。
var Name = "admin"

// weight 是注册到 discovery 的实例权重元数据（main 内从 bc.Discovery 读取后赋值）。
var weight string

func init() {
	flag.StringVar(&flagconfCommon, "conf-common", "../../../conf", "公共配置目录（多服务共享）")
	flag.StringVar(&flagconfPrivate, "conf", "../config", "私有配置目录（admin 专属）")
	// 服务间 gRPC 调用全局权重选择器（pkg/weight，规范 §6）
	selector.SetGlobalSelector(wb.NewBuilder())
}

func newApp(
	logger log.Logger,
	reg registry.Registrar,
	hs *http.Server,
	coreSrv *core.CoreServer,
	asynqSrv *asynqServer.Server,
	taskWorker *server.TaskWorker,
	siteMsgSub *worker.SiteMsgSubscriber,
	logRetentionWorker *worker.LogRetentionWorker,
	radarEventRetentionWorker *worker.RadarEventRetentionWorker,
) *kratos.App {
	// asynq 未启用时 NewAsynqServer 返回 nil，勿注册以免 nil 接口调用 Start panic。
	servers := []transport.Server{hs, coreSrv, taskWorker, siteMsgSub, logRetentionWorker, radarEventRetentionWorker}
	if asynqSrv != nil {
		servers = append(servers, asynqSrv)
	}
	return kratos.New(
		kratos.ID(os.Getenv("HOSTNAME")),
		kratos.Name(Name),
		kratos.Version("1.0.0"),
		kratos.Logger(logger),
		kratos.Server(servers...),
		kratos.Metadata(map[string]string{"weight": weight}),
		kratos.Registrar(reg),
	)
}


func main() {
	flag.Parse()

	// 加载配置：公共配置 + 私有配置（私有覆盖公共同名 key）
	// conf/ 只放 yaml，proto 生成的 Go 代码在 conf/pb/，目录职责分离
	c := config.New(
		config.WithSource(
			file.NewSource(flagconfCommon),  // 公共配置（先加载）
			file.NewSource(flagconfPrivate), // 私有配置（后加载，覆盖同名 key）
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	// discovery 实例权重（写入注册元数据，供权重选择器；未配置时为空串）
	weight = bc.GetDiscovery().GetWeight()

	// bc.GetLog() 返回 *pb.Log（公共包 framework/conf/pb 的类型），pkglog 直接接收
	logger := pkglog.NewLogger(bc.GetLog(),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", os.Getenv("HOSTNAME"),
		"service.name", Name,
		"service.version", "1.0.0",
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)

	// OTLP tracer（Phase 5-5.3：统一收敛到 pkg/observability.InitTracerProvider，OTLP HTTP → Jaeger :4318；
	// tp 仍通过 wire 显式注入，不设置全局 otel provider）
	tp, err := observability.InitTracerProvider(Name, bc.Trace.GetEndpoint())
	if err != nil {
		panic(err)
	}
	if tp != nil {
		defer tp.Shutdown(context.Background())
	}

	app, cleanup, err := wireApp(&bc, logger, tp)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
