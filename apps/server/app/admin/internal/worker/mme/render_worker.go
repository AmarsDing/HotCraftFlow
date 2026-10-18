// Package mmeworker MME 短视频矩阵混剪的异步运行时：
//   - RenderQueue：mme_render 专用 asynq 队列（独立并发，掐死在 conf.render.concurrency，
//     不与 "admin" 队列共享 worker，避免长渲染饿平台任务——概要设计 §5 部署视图）；
//   - MmeRenderWorker：处理下载/分析/渲染三类任务（handler 必须在构造期注册，
//     asynq 禁止 Start 后注册）；
//   - MmeTaskEnqueuer：bizmme.TaskEnqueuer 的真实 adapter（noop 只留给降级场景）。
package mmeworker

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	"framework/app/admin/internal/conf"
	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/core"
	"framework/pkg/mediacontract"
)

// 任务类型词表（与入队侧 enqueuer.go 同源；msgType 唯一真相在此）。
const (
	// TaskTypeTaskDownload 任务下载（yt-dlp / 本地入库 + 指纹去重）。
	TaskTypeTaskDownload = "mme.task.download"
	// TaskTypeTaskAnalyze 任务分析（ASR → 高光建议）。
	TaskTypeTaskAnalyze = "mme.task.analyze"
	// TaskTypeRenderVariant 单变体渲染（ffmpeg 渲染 + QC + manifest）。
	TaskTypeRenderVariant = "mme.render.variant"

	// queueName 本队列名（与 "admin" 队列硬隔离）。
	queueName = "mme_render"
)

// taskDownloadPayload / taskAnalyzePayload / renderPayload 队列载荷（json codec 序列化）。
type taskPayload struct {
	TaskID string `json:"task_id"`
}

type jobPayload struct {
	JobID string `json:"job_id"`
}

// RenderQueue mme_render 专用 asynq transport server。
// asynq 未配置（cfg.Server.Asynq 为空或 uri 为空）时 srv=nil，Start no-op（⚠️ 降级可体，
// 入队在 enqueuer 以 MME_QUEUE_UNAVAILABLE 如实报错）。
type RenderQueue struct {
	srv    *asynqServer.Server
	log    *log.Helper
	workerID string
}

var _ transport.Server = (*RenderQueue)(nil)

// NewRenderQueue 构造队列（并发 = conf.render.concurrency，默认 2——既定上限防单机过载）。
func NewRenderQueue(cfg *conf.Bootstrap, mmeCfg bizmme.Config, logger log.Logger) *RenderQueue {
	l := log.NewHelper(log.With(logger, "module", "worker/mme-render-queue"))
	q := &RenderQueue{log: l}

	asynqCfg := cfg.GetServer().GetAsynq()
	if asynqCfg == nil || asynqCfg.GetUri() == "" {
		l.Warn("asynq not configured: mme_render queue disabled (enqueue will report MME_QUEUE_UNAVAILABLE)")
		return q
	}

	concurrency := mmeCfg.RenderConcurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	opts := []asynqServer.ServerOption{
		asynqServer.WithRedisURI(asynqCfg.GetUri()),
		asynqServer.WithGracefullyShutdown(asynqCfg.GetEnableGracefullyShutdown()),
		asynqServer.WithQueues(map[string]int{queueName: 1}),
		asynqServer.WithConcurrency(concurrency),
		asynqServer.WithLogger(log.NewHelper(log.With(logger, "module", "asynq/server/mme-render"))),
	}
	if asynqCfg.GetShutdownTimeout() != nil {
		opts = append(opts, asynqServer.WithShutdownTimeout(asynqCfg.GetShutdownTimeout().AsDuration()))
	}
	q.srv = asynqServer.NewServer(opts...)
	return q
}

// Srv 暴露底层 asynq server（注册 handler / 入队用；可能为 nil）。
func (q *RenderQueue) Srv() *asynqServer.Server { return q.srv }

func (q *RenderQueue) Start(ctx context.Context) error {
	if q.srv == nil {
		return nil
	}
	return q.srv.Start(ctx)
}

func (q *RenderQueue) Stop(ctx context.Context) error {
	if q.srv == nil {
		return nil
	}
	return q.srv.Stop(ctx)
}

// MmeRenderWorker 三类 handler 的引导者（构造期注册！）+ 启动期确保 MM_MEDIA stream。
type MmeRenderWorker struct {
	queue *RenderQueue
	core  *core.CoreServer
	log   *log.Helper
}

var _ transport.Server = (*MmeRenderWorker)(nil)

// NewMmeRenderWorker 构造（asynq 未配置时跳过注册，Start 仅告警）。
func NewMmeRenderWorker(
	queue *RenderQueue,
	coreSrv *core.CoreServer,
	taskUC *bizmme.TaskUsecase,
	toolUC *bizmme.ToolUsecase,
	logger log.Logger,
) *MmeRenderWorker {
	l := log.NewHelper(log.With(logger, "module", "worker/mme-render"))
	w := &MmeRenderWorker{queue: queue, core: coreSrv, log: l}

	if queue == nil || queue.Srv() == nil {
		return w
	}
	srv := queue.Srv()
	must := func(name string, err error) {
		if err != nil {
			panic("register " + name + " subscriber failed: " + err.Error())
		}
	}
	must(TaskTypeTaskDownload, asynqServer.RegisterSubscriberWithCtx(srv, TaskTypeTaskDownload,
		func(ctx context.Context, _ string, p *taskPayload) error {
			return taskUC.RunDownload(ctx, p.TaskID)
		}))
	must(TaskTypeTaskAnalyze, asynqServer.RegisterSubscriberWithCtx(srv, TaskTypeTaskAnalyze,
		func(ctx context.Context, _ string, p *taskPayload) error {
			return taskUC.RunAnalyze(ctx, p.TaskID)
		}))
	// 🔴 渲染失败已由 ToolUsecase.failRender 落库且返回 nil——asynq 不重投，
	// 人工 RetryJob 才走第二次（attempt++ 留痕）。
	must(TaskTypeRenderVariant, asynqServer.RegisterSubscriberWithCtx(srv, TaskTypeRenderVariant,
		func(ctx context.Context, _ string, p *jobPayload) error {
			return toolUC.RunRenderJob(ctx, p.JobID)
		}))
	return w
}

// Start 启动期确保 MM_MEDIA stream 存在（Dispatch 发布侧先备；失败仅告警不阻塞）。
func (w *MmeRenderWorker) Start(context.Context) error {
	if w.core != nil && w.core.Nats() != nil && w.core.Nats().RawConn() != nil {
		if js, err := w.core.Nats().RawConn().JetStream(); err == nil {
			ensureStreamsWith(js, w.log, mediacontract.StreamMedia)
		}
	}
	w.log.Info("mme render worker handlers registered (mme.task.download / mme.task.analyze / mme.render.variant)")
	return nil
}

// Stop 队列生命周期由 RenderQueue.Stop 管理（同进程）。
func (w *MmeRenderWorker) Stop(context.Context) error { return nil }
