package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"

	"framework/app/media/internal/authnutil"
	"framework/app/media/internal/conf"
	"framework/app/media/internal/engines"
	"framework/app/media/internal/hostcaps"
	"framework/app/media/internal/jobrun"
	"framework/app/media/internal/jobstore"
	"framework/app/media/internal/server"
	"framework/app/media/internal/service"
	"framework/app/media/internal/sidecars"
	"framework/app/media/internal/sseapi"
	"framework/app/media/internal/wsapi"
)

// buildApp 手工依赖装配（v1 不引 wire 生成链；依赖图小，显式接线更可读）。
func buildApp(bc *conf.Bootstrap, logger log.Logger, tp *tracesdk.TracerProvider) (*kratos.App, func(), error) {
	ctx := context.Background()
	media := bc.GetMedia()

	// 底层结构化日志（引擎/sidecar 层用 slog，kratos 用 log；两者各归其面）。
	sl := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// ---- 存储 ----
	store, err := jobstore.Open(bc.GetData().GetDatabase().GetSource(),
		int(media.GetJobs().GetEventCapPerJob()))
	if err != nil {
		return nil, nil, fmt.Errorf("open jobstore: %w", err)
	}
	if err := store.EnsureSchema(ctx); err != nil {
		return nil, nil, err
	}

	// ---- 宿主探测 + 引擎注册表 ----
	host := hostcaps.Detect()
	var reg *engines.Registry
	reg = engines.NewRegistry(host, func(id string) {
		// 单引擎驻留纪律：换引擎时收割旧 sidecar。
		if e, err := reg.EngineFor(id); err == nil {
			if se, ok := e.(*engines.SidecarEngine); ok {
				se.Driver().ReapIdle()
			}
		}
	})
	reg.Register(engines.NewEchoEngine())
	reg.RegisterLazy(sidecarEchoID, func() engines.Engine {
		// 惰性装配：二进制缺失时 Available=false 优雅降级为"不可用"清单项。
		return engines.NewSidecarEngine(engines.SidecarSpec{
			ID:          sidecarEchoID,
			DisplayName: "Echo Sidecar（子进程占位）",
			Kind:        engines.KindTTS,
			Caps: engines.Caps{
				GPUCompat: []string{"cpu"},
			},
			Bin: sidecarEchoBin(),
			DriverCfg: sidecars.Config{
				ReadyTimeout:  media.GetSidecar().GetReadyTimeout().AsDuration(),
				GenWatchdog:   media.GetSidecar().GetGenWatchdog().AsDuration(),
				IdleReap:      media.GetSidecar().GetIdleReap().AsDuration(),
				MaxFrameBytes: media.GetSidecar().GetMaxFrameBytes(),
			},
			Log: sl,
		})
	})

	// ---- 执行器 + v1 处理器 ----
	executor := jobrun.New(store, jobrun.Config{
		Lanes:          media.GetJobs().GetLanes(),
		EventCapPerJob: int(media.GetJobs().GetEventCapPerJob()),
	}, "media-1", sl)
	executor.RegisterHandler("echo.synthesize", jobrun.EchoSynthesizeHandler(jobrun.EchoDeps{
		Registry:      reg,
		DefaultEngine: media.GetEngines().GetDefault(),
		ArtifactDir:   media.GetStorage().GetArtifactDir(),
	}))
	executor.RegisterHandler("exec.probe", jobrun.ExecProbeHandler())
	executor.RegisterHandler("mme.render", jobrun.MMERenderHandler(jobrun.MMERenderDeps{
		FFmpegPath: envOr("MEDIA_FFMPEG_PATH", "ffmpeg"),
		Log:        sl,
	}))

	// ---- 鉴权（与 admin 同钥互通）----
	jwtKey := bc.GetAuth().GetAuthn().GetJwt().GetKey()
	verifier := authnutil.NewVerifier(jwtKey)
	authenticator := server.NewAuthenticator(bc)

	// ---- WS 集线器（transport.Server：Start/Stop 交给 kratos 生命周期）----
	hub := wsapi.NewHub(wsapi.Deps{Verifier: verifier, Executor: executor, Log: sl})

	// ---- 服务层 + REST 装配 ----
	jobsSvc := service.NewJobsService(executor)
	enginesSvc := service.NewEnginesService(reg, host)
	doctorSvc := service.NewDoctorService(
		media.GetDoctor().GetProbes(),
		media.GetDoctor().GetProbeTimeout().AsDuration(),
		host,
	)

	httpSrv := server.NewRESTServer(bc, logger, tp, authenticator,
		jobsSvc, enginesSvc, doctorSvc,
		server.RawDeps{
			Verifier: verifier,
			SSEHandler: sseapi.Deps{
				Verifier: verifier,
				Executor: executor,
				Log:      sl,
			},
			WSHub:        hub,
			ArtifactBase: media.GetStorage().GetArtifactDir(),
		},
		store.Ping,
	)

	servers := []transport.Server{httpSrv, executor, hub}
	app := kratos.New(
		kratos.ID(os.Getenv("HOSTNAME")),
		kratos.Name(Name),
		kratos.Version("1.0.0"),
		kratos.Logger(logger),
		kratos.Server(servers...),
	)

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownCtx
		_ = store.Close()
	}
	return app, cleanup, nil
}

// envOr 环境变量覆盖工具（空则取缺省）。
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// sidecarEchoID 示范 sidecar 引擎的注册 id。
const sidecarEchoID = "echo-sidecar"

// sidecarEchoBin 示范 sidecar 二进制定位：env 覆盖 > 同目录 > PATH。
func sidecarEchoBin() string {
	if v := os.Getenv("MEDIA_SIDECAR_ECHO_BIN"); v != "" {
		return v
	}
	return "media_sidecar_echo"
}
