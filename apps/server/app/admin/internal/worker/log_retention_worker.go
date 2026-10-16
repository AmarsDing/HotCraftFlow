package worker

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	"framework/app/admin/internal/biz"
	"framework/pkg/retention"
)

// 审计日志保留策略：操作日志保留 30 天、登录日志保留 90 天，每 6 小时清理一轮。
const (
	operationLogRetentionDays = 30
	loginLogRetentionDays     = 90
	// logRetentionCron 每 6 小时第 23 分执行（原 ticker 6h 口径的 cron 化）。
	logRetentionCron    = "23 */6 * * *"
	logRetentionTimeout = 10 * time.Minute
)

var _ transport.Server = (*LogRetentionWorker)(nil)

// LogRetentionWorker 审计日志保留期清理 worker：
// 定期物理删除 create_time 早于 30 天前的 admin_operation_logs、早于 90 天前的 admin_login_logs。
// 执行骨架已收敛至 pkg/retention.Worker，本文件仅保留 admin 侧适配。
type LogRetentionWorker struct {
	operationLogRepo biz.AdminOperationLogRepo
	loginLogRepo     biz.AdminLoginLogRepo
	inner            *retention.Worker
	log              *log.Helper
}

func NewLogRetentionWorker(operationLogRepo biz.AdminOperationLogRepo, loginLogRepo biz.AdminLoginLogRepo, logger log.Logger) *LogRetentionWorker {
	w := &LogRetentionWorker{
		operationLogRepo: operationLogRepo,
		loginLogRepo:     loginLogRepo,
		log:              log.NewHelper(log.With(logger, "module", "log-retention-worker")),
	}
	w.inner = retention.New(retention.Config{
		Enabled:    true,
		Module:     "log-retention-worker",
		Cron:       logRetentionCron,
		Timeout:    logRetentionTimeout,
		RunOnStart: true, // 启动即清理一轮（存量超期数据不等首个周期）
		Job:        w.cleanup,
	}, logger)
	return w
}

func (w *LogRetentionWorker) Start(ctx context.Context) error { return w.inner.Start(ctx) }

func (w *LogRetentionWorker) Stop(ctx context.Context) error { return w.inner.Stop(ctx) }

// cleanup 单轮清理主体（pkg/retention.Worker 回调；两类日志独立 cutoff，单类失败不阻塞另一类）。
func (w *LogRetentionWorker) cleanup(ctx context.Context) {
	operationCutoff := retention.Cutoff(operationLogRetentionDays)
	deleted, err := w.operationLogRepo.DeleteOlderThan(ctx, operationCutoff)
	if err != nil {
		w.log.Errorf("delete expired operation logs failed: %s", err.Error())
	} else if deleted > 0 {
		w.log.Infof("operation log retention done: deleted=%d cutoff=%s", deleted, operationCutoff.Format(time.RFC3339))
	}

	loginCutoff := retention.Cutoff(loginLogRetentionDays)
	deleted, err = w.loginLogRepo.DeleteOlderThan(ctx, loginCutoff)
	if err != nil {
		w.log.Errorf("delete expired login logs failed: %s", err.Error())
	} else if deleted > 0 {
		w.log.Infof("login log retention done: deleted=%d cutoff=%s", deleted, loginCutoff.Format(time.RFC3339))
	}
}
