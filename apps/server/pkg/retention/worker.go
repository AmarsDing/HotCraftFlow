// worker.go 通用保留清理 worker（robfig/cron v3，5 字段标准表达式）。
// 收敛 flight/metar/satellite/admin 各服务保留清理 worker 的同构实现：
// enabled 开关、cron 注册/启停、单次超时控制、启动日志口径统一；
// 清理主体（删哪些表、按什么口径 cutoff、是否连带删文件/分区）由各服务经 Config.Job 注入。
//
// 与 monitoralarm.AlarmRetentionScheduler 的分工：后者走 alarm_retention_policies 表
// （策略动态 CRUD，任务级调度）；本包面向"代码静态口径"的时序表物理清理（cron+天数固定或配置覆盖）。
package retention

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/robfig/cron/v3"
)

// Config 保留清理配置。
type Config struct {
	// Enabled 清理开关（false 时 Start 直接跳过）。
	Enabled bool
	// Module 日志 module 标识（如 "metar/biz/retention"）。
	Module string
	// Cron 清理窗口 cron 表达式（5 字段，如 "20 3 * * *" 每日低峰）。
	Cron string
	// Timeout 单次清理超时（<=0 默认 30 分钟）。
	Timeout time.Duration
	// RunOnStart 启动即执行一轮（存量超期数据不等首个 cron 窗口）。
	RunOnStart bool
	// Job 清理主体（优先；内部自行计算 cutoff、分批、连带清理，错误自行记日志）。
	Job func(ctx context.Context)
	// Domain 阶段编排模式的域配置（2026-08-21 设计 §6.5.1；Job 为空且 Domain/Executor 均非空时生效，
	// Job 自动包装为 Executor.Run(Domain)）。旧配置（仅 days）用 LegacyStages 转单阶段即可零改动接入。
	Domain *DomainConfig
	// Executor 阶段编排器（与 Domain 配套）。
	Executor *Executor
	// DisabledHint Enabled=false 时的启动日志提示（通常为配置路径）。
	DisabledHint string
}

// Worker 通用保留清理 worker（实现 transport.Server，生命周期纳入 kratos.App）。
type Worker struct {
	cfg Config
	log *log.Helper

	cron *cron.Cron
}

// New 构造保留清理 worker。Cron 非法时 Start 记错误日志并跳过（不 panic）。
// Job 为空时尝试 Domain+Executor 阶段编排；两者皆缺则 Start 记错误并跳过（不 panic）。
func New(cfg Config, logger log.Logger) *Worker {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}
	if cfg.Job == nil && cfg.Domain != nil && cfg.Executor != nil {
		domain := *cfg.Domain
		executor := cfg.Executor
		cfg.Job = func(ctx context.Context) {
			if err := executor.Run(ctx, domain); err != nil {
				log.NewHelper(log.With(logger, "module", cfg.Module)).Errorf("staged retention finished with errors: %v", err)
			}
		}
	}
	return &Worker{
		cfg:  cfg,
		log:  log.NewHelper(log.With(logger, "module", cfg.Module)),
		cron: cron.New(), // 标准 5 字段
	}
}

// Start 服务启动钩子：注册清理 cron 并启动（未启用时跳过；RunOnStart 先执行一轮）。
func (w *Worker) Start(_ context.Context) error {
	if !w.cfg.Enabled {
		w.log.Infof("retention worker disabled (%s)", w.cfg.DisabledHint)
		return nil
	}
	if w.cfg.Job == nil {
		w.log.Error("retention worker has no job (neither Job nor Domain+Executor configured), skipped")
		return nil
	}
	if w.cfg.RunOnStart {
		go w.RunOnce()
	}
	if _, err := w.cron.AddFunc(w.cfg.Cron, w.RunOnce); err != nil {
		w.log.Errorf("register retention cron failed: expr=%q err=%v", w.cfg.Cron, err)
		return nil // 清理注册失败不阻断服务启动
	}
	w.cron.Start()
	w.log.Infof("retention worker started: cron=%q timeout=%s runOnStart=%v", w.cfg.Cron, w.cfg.Timeout, w.cfg.RunOnStart)
	return nil
}

// Stop 服务停止钩子（等待运行中清理收尾由 Job 自身超时控制）。
func (w *Worker) Stop(_ context.Context) error {
	if !w.cfg.Enabled {
		return nil
	}
	stopCtx := w.cron.Stop()
	<-stopCtx.Done()
	w.log.Info("retention worker stopped")
	return nil
}

// RunOnce 执行一轮清理（cron 回调、启动兜底与手动触发共用；统一超时控制）。
func (w *Worker) RunOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), w.cfg.Timeout)
	defer cancel()
	w.cfg.Job(ctx)
}

// Cutoff 保留天数 → 截止时间（UTC 口径，时序表清理通用）。
func Cutoff(days int) time.Time {
	return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
}

// CutoffWall 保留天数 → 墙钟截止时间（对齐 PostgreSQL TIMESTAMP WITHOUT TIME ZONE + NOW()）。
// 日志表 collection_failures.created_at / trap_events.received_at 均为无时区 TIMESTAMP、写入用 NOW()；
// 若用 UTC Cutoff，在 Asia/Shanghai 等时区下「清空全部」(days≤0→UTC now) 会对不上本地墙钟，deleted 恒为 0。
// days≤0 返回零值，表示调用方不按时间过滤。
func CutoffWall(days int) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	return time.Now().Add(-time.Duration(days) * 24 * time.Hour)
}

// 编译期接口断言。
var _ transport.Server = (*Worker)(nil)
