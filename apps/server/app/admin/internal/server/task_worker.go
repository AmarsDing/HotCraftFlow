package server

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	adminV1 "framework/api/gen/go/admin/v1"
	"framework/app/admin/internal/biz"
)

var _ transport.Server = (*TaskWorker)(nil)

// tokenCleanupPayload user_token_cleanup 任务载荷（当前无参数，占位结构保证 codec 可序列化）。
type tokenCleanupPayload struct{}


// TaskWorker 调度任务引导 worker：
//   - 构造期向 asynq 注册全部内置任务处理器（asynq handleFunc 禁止 Start 后注册，必须在构造期完成）；
//   - Start 时播种内置任务（EnsureSeedTasks，幂等）并恢复所有已启用周期任务的调度（StartEnabledTasks）。
//
// asynq 未配置时 Start 直接跳过（任务 CRUD 仍可用，调度为 no-op）。
type TaskWorker struct {
	taskUC  *biz.TaskUsecase
	log     *log.Helper
	asynqOn bool
}

func NewTaskWorker(asynqSrv *asynqServer.Server, taskUC *biz.TaskUsecase, sessionUC *biz.SessionUsecase, logger log.Logger) *TaskWorker {
	l := log.NewHelper(log.With(logger, "module", "task-worker"))
	w := &TaskWorker{
		taskUC:  taskUC,
		log:     l,
		asynqOn: asynqSrv != nil,
	}
	if asynqSrv != nil {
		handler := wrapWithExecutionRecord(taskUC, biz.TaskTypeNameUserTokenCleanup,
			func(ctx context.Context) error {
				_, err := sessionUC.CleanupExpiredTokens(ctx)
				return err
			})
		if err := asynqServer.RegisterSubscriberWithCtx(asynqSrv, biz.TaskTypeNameUserTokenCleanup,
			func(ctx context.Context, _ string, _ *tokenCleanupPayload) error {
				return handler(ctx)
			}); err != nil {
			panic("register user_token_cleanup subscriber failed: " + err.Error())
		}
	}
	return w
}

// wrapWithExecutionRecord 包装任务处理器：记录每次执行的起止时间、成败与错误信息。
// 记录动作本身失败不阻断任务（RecordExecution 内部降级为日志）。
func wrapWithExecutionRecord(taskUC *biz.TaskUsecase, typeName string, handler func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		startedAt := time.Now()
		err := handler(ctx)
		finishedAt := time.Now()
		status := adminV1.TaskExecutionStatus_ExecutionStatus_Success
		if err != nil {
			status = adminV1.TaskExecutionStatus_ExecutionStatus_Failed
		}
		taskUC.RecordExecution(ctx, typeName, status, err, startedAt, finishedAt)
		return err
	}
}

func (w *TaskWorker) Start(ctx context.Context) error {
	if !w.asynqOn {
		w.log.Warn("asynq not configured, task scheduler disabled (task CRUD still available)")
		return nil
	}
	// 先播种（仅落库）再统一恢复调度，避免单任务重复注册。
	if err := w.taskUC.EnsureSeedTasks(ctx); err != nil {
		w.log.Errorf("seed builtin tasks failed: %s", err.Error())
		return err
	}
	count, err := w.taskUC.StartEnabledTasks(ctx)
	if err != nil {
		w.log.Errorf("start enabled tasks failed: %s", err.Error())
		return err
	}
	w.log.Infof("task worker started: %d periodic task(s) scheduled", count)
	return nil
}

func (w *TaskWorker) Stop(context.Context) error {
	// 调度器生命周期由 asynqSrv 自身 Stop 管理（优雅关闭），此处无需操作。
	return nil
}
