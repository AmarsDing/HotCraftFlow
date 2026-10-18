package mme

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"framework/app/admin/internal/conf"
	"framework/pkg/mediacontract"
)

// NewConfig wire 提供者：Bootstrap → Config。
func NewConfig(bc *conf.Bootstrap) Config { return ResolveConfig(bc) }

// ProviderSet MME biz 层 wire 集合。
// 依赖端口（TaskRepo/...、Downloader/...、TaskEnqueuer/EventPublisher/MediaStorage）
// 由 data / server 层适配器提供。
var ProviderSet = wire.NewSet(
	NewConfig,
	NewTaskUsecase,
	NewAssetDeduper,
	NewAnalysisCache,
	NewCopyGenerator,
	NewRenderPlanner,
	NewDiffChecker,
	NewQCChecker,
	NewTemplateUsecase,
	NewToolUsecase,
	NewVariantSetUsecase,
	// 注意：TaskEnqueuer / EventPublisher 端口由 worker/mme 层（mme_render 队列、NATS 契约发布器）
	// 经 wire.Bind 提供；NewNoopEnqueuer / NewNoopPublisher 仅作单测/降级启动备用，不进本集合。
)

// noopEnqueuer 无 asynq 环境（单测/降级启动）的入队器：返回 ErrQueueUnavailable，
// 🔴 绝不假排队——调用方必须能看到任务没有真正进入队列。
type noopEnqueuer struct{ log *log.Helper }

// NewNoopEnqueuer 构造。
func NewNoopEnqueuer(logger log.Logger) TaskEnqueuer {
	return &noopEnqueuer{log: log.NewHelper(log.With(logger, "module", "biz/mme/noop-enq"))}
}

// EnqueueDownload 未配置队列——返回明确错误。
func (n *noopEnqueuer) EnqueueDownload(context.Context, string) error {
	n.log.Warn("TaskEnqueuer not wired: download not queued (MME_QUEUE_UNAVAILABLE)")
	return ErrQueueUnavailable
}

// EnqueueAnalyze 未配置队列——返回明确错误。
func (n *noopEnqueuer) EnqueueAnalyze(context.Context, string) error {
	n.log.Warn("TaskEnqueuer not wired: analyze not queued (MME_QUEUE_UNAVAILABLE)")
	return ErrQueueUnavailable
}

// EnqueueRender 未配置队列——返回明确错误。
func (n *noopEnqueuer) EnqueueRender(context.Context, string) error {
	n.log.Warn("TaskEnqueuer not wired: render not queued (MME_QUEUE_UNAVAILABLE)")
	return ErrQueueUnavailable
}

// noopPublisher 无 NATS 环境（单测/降级启动）的事件发布器：返回 ErrQueueUnavailable，
// 🔴 Dispatch 绝不会"假装已送达 MAP"。
type noopPublisher struct{ log *log.Helper }

// NewNoopPublisher 构造。
func NewNoopPublisher(logger log.Logger) EventPublisher {
	return &noopPublisher{log: log.NewHelper(log.With(logger, "module", "biz/mme/noop-pub"))}
}

// PublishVariantReady 未配置 NATS——返回明确错误。
func (n *noopPublisher) PublishVariantReady(context.Context, *mediacontract.VariantSet, string) error {
	n.log.Warn("EventPublisher not wired: variant.ready not published (MME_QUEUE_UNAVAILABLE)")
	return ErrQueueUnavailable
}
