package server

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/hibiken/asynq"
	"github.com/tx7do/kratos-transport/broker"
	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
)

// asynqQueueName 本服务专用 asynq 队列名（多服务共用同一 Redis 实例，必须按服务隔离队列：
// 分析服务消费 "analysis"，本服务消费 "admin"，互不抢对方的任务）。
const asynqQueueName = "admin"

// NewAsynqServer 构造 asynq 传输服务器（既是 transport.Server 又是调度客户端）。
// 未配置（cfg.Server.Asynq 为空或 uri 为空）时返回 nil，调用方按 nil 跳过注册。
// 修复 B4：此前 *asynqServer.Server 从未由 wire 构造，TaskService.Server 永远 nil。
func NewAsynqServer(cfg *conf.Bootstrap, logger log.Logger) *asynqServer.Server {
	asynqCfg := cfg.GetServer().GetAsynq()
	if asynqCfg == nil || asynqCfg.GetUri() == "" {
		return nil
	}

	opts := []asynqServer.ServerOption{
		asynqServer.WithRedisURI(asynqCfg.GetUri()),
		asynqServer.WithGracefullyShutdown(asynqCfg.GetEnableGracefullyShutdown()),
		asynqServer.WithQueues(map[string]int{asynqQueueName: 1}),
		asynqServer.WithLogger(log.NewHelper(log.With(logger, "module", "asynq/server/admin-service"))),
	}
	if asynqCfg.GetShutdownTimeout() != nil {
		opts = append(opts, asynqServer.WithShutdownTimeout(asynqCfg.GetShutdownTimeout().AsDuration()))
	}
	if asynqCfg.GetCodec() != "" {
		opts = append(opts, asynqServer.WithCodec(asynqCfg.GetCodec()))
	}

	return asynqServer.NewServer(opts...)
}

// NewTaskScheduler 将 *asynqServer.Server 适配为 biz.TaskScheduler 端口。
// srv 为 nil（asynq 未配置）时返回 noopTaskScheduler，保证任务 CRUD 不受影响，
// 调度类方法均为 no-op。真实实现经 queueTaskScheduler 包装：入队统一注入
// 本服务专用队列（asynq.Queue(asynqQueueName)），与服务端消费队列对齐。
func NewTaskScheduler(srv *asynqServer.Server) biz.TaskScheduler {
	if srv == nil {
		return noopTaskScheduler{}
	}
	return queueTaskScheduler{srv: srv}
}

// queueTaskScheduler 包装 *asynqServer.Server：所有入队操作追加 asynq.Queue 选项，
// 保证任务进入本服务专用队列（否则落默认 "default" 队列，本服务消费不到）。
type queueTaskScheduler struct {
	srv *asynqServer.Server
}

func (q queueTaskScheduler) NewPeriodicTask(cronSpec, taskID, typeName string, payload broker.Any, opts ...asynq.Option) (string, error) {
	opts = append(opts, asynq.Queue(asynqQueueName))
	return q.srv.NewPeriodicTask(cronSpec, taskID, typeName, payload, opts...)
}

func (q queueTaskScheduler) NewTask(typeName string, payload broker.Any, opts ...asynq.Option) error {
	opts = append(opts, asynq.Queue(asynqQueueName))
	return q.srv.NewTask(typeName, payload, opts...)
}

func (q queueTaskScheduler) NewWaitResultTask(typeName string, payload broker.Any, opts ...asynq.Option) error {
	opts = append(opts, asynq.Queue(asynqQueueName))
	return q.srv.NewWaitResultTask(typeName, payload, opts...)
}

func (q queueTaskScheduler) RemovePeriodicTask(taskID string) error {
	return q.srv.RemovePeriodicTask(taskID)
}

func (q queueTaskScheduler) RemoveAllPeriodicTask() {
	q.srv.RemoveAllPeriodicTask()
}

// noopTaskScheduler 是 asynq 未配置时的空实现，避免 Usecase 处处判空。
type noopTaskScheduler struct{}

func (noopTaskScheduler) NewPeriodicTask(string, string, string, broker.Any, ...asynq.Option) (string, error) {
	return "", nil
}
func (noopTaskScheduler) NewTask(string, broker.Any, ...asynq.Option) error            { return nil }
func (noopTaskScheduler) NewWaitResultTask(string, broker.Any, ...asynq.Option) error  { return nil }
func (noopTaskScheduler) RemovePeriodicTask(string) error                              { return nil }
func (noopTaskScheduler) RemoveAllPeriodicTask()                                       {}
