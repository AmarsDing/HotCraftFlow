package ttworker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	biztt "framework/app/admin/internal/biz/tt"
)

var _ transport.Server = (*LifecycleWorker)(nil)

// LifecycleWorker 生命周期周期评估 worker（A-8：streak/分位由时序查询推导，
// 零累加器，重启幂等——任何时刻重启都只是「少评一轮」，下轮自愈）。
//
// 节奏 conf.lifecycle.interval（默认 15min），每轮：ListActiveTopics →
// 信号量限并发 EvaluateOne → 计数 changed/unchanged/error。
// 单话题评估由 data 层乐观条件写（stage != 目标）兜底并发抢先；
// 跨进程同跑两轮亦安全（条件写幂等收敛）。
type LifecycleWorker struct {
	conf *biztt.Conf
	eval *biztt.LifecycleEvaluator
	log  *log.Helper

	mu     sync.Mutex
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewLifecycleWorker 构造。evaluator 构造失败（配置红线）向上抛，wire 期即败，
// 不放行零值配置静默灾变。
func NewLifecycleWorker(conf *biztt.Conf, eval *biztt.LifecycleEvaluator, logger log.Logger) *LifecycleWorker {
	return &LifecycleWorker{
		conf:   conf,
		eval:   eval,
		log:    log.NewHelper(log.With(logger, "module", "worker/tt-lifecycle")),
		stopCh: make(chan struct{}),
	}
}

// Start 启动评估循环。
func (w *LifecycleWorker) Start(ctx context.Context) error {
	if w.conf == nil || !w.conf.Enabled {
		w.log.Info("tt disabled, lifecycle worker not started")
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Add(1)
	go w.loop()
	w.log.Infof("tt lifecycle worker started: interval=%s scan_limit=%d concurrency=%d",
		w.conf.Lifecycle.Interval, w.conf.Lifecycle.ScanLimit, w.conf.Lifecycle.Concurrency)
	return nil
}

// Stop 停止循环（等当前轮结束；单轮有超时，不会悬挂）。
func (w *LifecycleWorker) Stop(ctx context.Context) error {
	close(w.stopCh)
	w.wg.Wait()
	w.log.Info("tt lifecycle worker stopped")
	return nil
}

// loop 周期评估。
func (w *LifecycleWorker) loop() {
	defer w.wg.Done()
	interval := w.conf.Lifecycle.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.scanOnce()
		}
	}
}

// scanOnce 单轮：扫一批活跃话题，并发评估。
// 超时 = interval（避免单轮拖过下一轮；评估幂等，截断无害）。
func (w *LifecycleWorker) scanOnce() {
	interval := w.conf.Lifecycle.Interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), interval)
	defer cancel()

	rows, err := w.eval.ListActiveTopics(ctx, w.conf.Lifecycle.ScanLimit)
	if err != nil {
		w.log.Warnf("list active topics failed: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	conc := w.conf.Lifecycle.Concurrency
	if conc <= 0 {
		conc = 8
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	now := time.Now().UTC()
	for _, row := range rows {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(row biztt.LifecycleTopicRow) {
			defer wg.Done()
			defer func() { <-sem }()
			changed, err := w.eval.EvaluateOne(ctx, row, now)
			switch {
			case err != nil:
				biztt.LifecycleEvalTotal.WithLabelValues("error").Inc()
				w.log.Warnf("evaluate topic=%s failed: %v", row.TopicID, err)
			case changed:
				biztt.LifecycleEvalTotal.WithLabelValues("changed").Inc()
			default:
				biztt.LifecycleEvalTotal.WithLabelValues("unchanged").Inc()
			}
		}(row)
	}
	wg.Wait()
}
