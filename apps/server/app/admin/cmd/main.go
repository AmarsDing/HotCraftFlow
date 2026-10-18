package main

import (
	"context"
	"flag"
	"os"

	bizpublish "framework/app/admin/internal/biz/publish"
	biztt "framework/app/admin/internal/biz/tt"
	"framework/app/admin/internal/conf"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/publishadapters"
	"framework/app/admin/internal/server"
	"framework/app/admin/internal/worker"
	icworker "framework/app/admin/internal/worker/ic"
	miaworker "framework/app/admin/internal/worker/mia"
	mmeworker "framework/app/admin/internal/worker/mme"
	ttworker "framework/app/admin/internal/worker/tt"
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
	ttIngestSub *ttworker.IngestSubscriber,
	ttSnapshotWriter *ttworker.SnapshotWriter,
	ttBufferReplay *ttworker.BufferReplay,
	miaIntelSub *miaworker.IntelSubscriber,
	miaEquitySched *miaworker.MiaEquityScheduler,
	miaCryptoSched *miaworker.MiaCryptoScheduler,
	mmeQueue *mmeworker.RenderQueue,
	mmeWorker *mmeworker.MmeRenderWorker,
	icScheduler *icworker.Scheduler,
	icNormalizer *icworker.NormalizerConsumer,
) *kratos.App {
	// asynq 未启用时 NewAsynqServer 返回 nil，勿注册以免 nil 接口调用 Start panic。
	// TT/MIA 各件套内部均以 conf.Enabled 自己把关，此处无条件注册。
	// MME 队列/worker 未配置 asynq 时 Start no-op（入队以 MME_QUEUE_UNAVAILABLE 如实报错），可无条件注册。
	// IC 调度器/归一化消费者同样内部降级（ic.enabled=false 或 asynq/nats 缺失时 Start no-op）。
	servers := []transport.Server{hs, coreSrv, taskWorker, siteMsgSub, logRetentionWorker, radarEventRetentionWorker,
		ttIngestSub, ttSnapshotWriter, ttBufferReplay, miaIntelSub, miaEquitySched, miaCryptoSched,
		mmeQueue, mmeWorker,
		icScheduler, icNormalizer}
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

	// TT 子系统配置（独立 tt.yaml，避免 conf.pb.go 再生成重链）。
	// 扫描 → 补默认 → 判红线；无效配置直接终止（fail-fast 胜过错配静默运行）。
	var ttb biztt.Conf
	if err := c.Value("tt").Scan(&ttb); err != nil {
		panic(err)
	}
	ttb.Normalize()
	if err := ttb.Validate(); err != nil {
		panic(err)
	}

	// ⑤ MAP 子系统配置（独立 map.yaml；🔴 安全红线判据——禁自批/禁自动审批发布/默认私密）。
	// 以 DefaultConf 打底（缺 map.yaml 时仍保持安全默认：私密 + 熔断全开，绝不松）。
	mc := bizpublish.DefaultConf()
	_ = c.Value("map").Scan(&mc) // 键缺失时保留默认；解析错误由 Validate 判出或容忍默认兜底
	mc.Normalize()
	if err := mc.Validate(); err != nil {
		panic(err) // 🔴 红线违例（如 allow_public 而禁自批被关）直接终止
	}

	// MAP 平台能力配置（platforms.yaml；键缺失 → 空目录，发布时按 ADAPTER_NOT_FOUND 明确报错）
	var mpconfs publishadapters.PlatformConfMap
	_ = c.Value("map_platforms").Scan(&mpconfs)

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

	app, cleanup, err := wireApp(&bc, &ttb, &mc, mpconfs, logger, tp)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
