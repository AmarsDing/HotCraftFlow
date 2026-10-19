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

	// stopOnce 幂等关停守卫（B-25 评审：双调用方重复 Stop 时 close 已关通道
	// 会 panic 阻断优雅停机链路；sync.Once 收敛为单次语义）。
	stopOnce sync.Once
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
// stopOnce 保证重复调用幂等（close 已关通道 panic 被收敛）。
func (w *LifecycleWorker) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
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

	// M1 修复：一轮 sweep 的近 24h 热度分位只算一次（全体活跃话题共享同一
	// (from,to) 窗口），注入各 EvaluateOne，不再每话题全量扫 24h 聚合。
	now := time.Now().UTC()
	quant, err := w.eval.ComputeQuantiles(ctx, now)
	if err != nil {
		w.log.Warnf("compute quantiles failed: %v", err)
		// 分位不可得：传 nil 由各 EvaluateOne 现场计算/降级，不致整轮停摆。
	}

	conc := w.conf.Lifecycle.Concurrency
	if conc <= 0 {
		conc = 8
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
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
			changed, err := w.eval.EvaluateOne(ctx, row, now, quant)
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
