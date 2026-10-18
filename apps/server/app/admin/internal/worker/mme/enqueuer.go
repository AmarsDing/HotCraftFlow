package mmeworker

import (
	"context"

	"github.com/hibiken/asynq"

	bizmme "framework/app/admin/internal/biz/mme"
)

// MmeTaskEnqueuer bizmme.TaskEnqueuer 的真实 adapter：
// 全部入队强制走 mme_render 队列（asynq.Queue(queueName)），与消费端对齐。
// 🔴 队列未配置时返回 MME_QUEUE_UNAVAILABLE——绝不假排队（调用方据此把任务落 failed
// 并保留可读错误，而不是"任务已提交"的幻觉）。
type MmeTaskEnqueuer struct {
	queue *RenderQueue
}

// NewMmeTaskEnqueuer 构造。
func NewMmeTaskEnqueuer(queue *RenderQueue) *MmeTaskEnqueuer {
	return &MmeTaskEnqueuer{queue: queue}
}

func (e *MmeTaskEnqueuer) enqueue(typeName string, payload any) error {
	if e == nil || e.queue == nil || e.queue.Srv() == nil {
		return bizmme.ErrQueueUnavailable
	}
	return e.queue.Srv().NewTask(typeName, payload, asynq.Queue(queueName))
}

// EnqueueDownload 下载任务入队（端点：Create 后立即入库）。
func (e *MmeTaskEnqueuer) EnqueueDownload(_ context.Context, taskID string) error {
	return e.enqueue(TaskTypeTaskDownload, &taskPayload{TaskID: taskID})
}

// EnqueueAnalyze 分析任务入队（ASR→高光）。
func (e *MmeTaskEnqueuer) EnqueueAnalyze(_ context.Context, taskID string) error {
	return e.enqueue(TaskTypeTaskAnalyze, &taskPayload{TaskID: taskID})
}

// EnqueueRender 渲染任务入队（FR-MIX-03 规划产出）。
func (e *MmeTaskEnqueuer) EnqueueRender(_ context.Context, jobID string) error {
	return e.enqueue(TaskTypeRenderVariant, &jobPayload{JobID: jobID})
}

var _ bizmme.TaskEnqueuer = (*MmeTaskEnqueuer)(nil)
