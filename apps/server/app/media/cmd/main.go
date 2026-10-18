// media：HotCraftFlow 的媒体推理独立服务（热点矩阵 M 线的引擎侧）。
//
// 职责边界（v1）：长任务状态机 + 事件镜像（PG）、lane 分槽执行器、
// SSE after_seq 回放、WS 失效信号总线、引擎声明式能力注册表
// （echo 占位 + sidecar 帧协议驱动）、doctor 体检、artifacts 下载。
// 真实 TTS/ASR 引擎以下期 sidecar 接入；业务 handler 由 MME 开工时注册。
package main

import (
	"context"
	"flag"
	"os"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/tracing"

	"framework/app/media/internal/conf"

	pkglog "framework/pkg/log"
	"framework/pkg/observability"
)

// Name 服务名（kratos.App 注册与 trace ServiceName）。
var Name = "media"

var (
	flagconfCommon  string
	flagconfPrivate string
)

func init() {
	flag.StringVar(&flagconfCommon, "conf-common", "../../../conf", "公共配置目录（多服务共享）")
	flag.StringVar(&flagconfPrivate, "conf", "../config", "私有配置目录（media 专属）")
}

func main() {
	flag.Parse()

	c := config.New(
		config.WithSource(
			file.NewSource(flagconfCommon),
			file.NewSource(flagconfPrivate),
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

	// media 域配置：补默认 → 判红线，无效配置启动期炸停（fail-fast）。
	media := bc.GetMedia()
	media.Normalize()
	if err := media.Validate(); err != nil {
		panic(err)
	}
	if !media.GetEnabled() {
		panic("media.enabled=false：本进程没有意义，请直接不要启动 media 服务")
	}

	logger := pkglog.NewLogger(bc.GetLog(),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", os.Getenv("HOSTNAME"),
		"service.name", Name,
		"service.version", "1.0.0",
		"trace.id", tracing.TraceID(),
		"span.id", tracing.SpanID(),
	)

	tp, err := observability.InitTracerProvider(Name, bc.GetTrace().GetEndpoint())
	if err != nil {
		panic(err)
	}
	if tp != nil {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	app, cleanup, err := buildApp(&bc, logger, tp)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
