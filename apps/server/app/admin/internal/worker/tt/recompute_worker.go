package ttworker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	biztt "framework/app/admin/internal/biz/tt"
)

var _ transport.Server = (*RecomputeWorker)(nil)

// RecomputeWorker 迟到回填重算 worker（S-23/A-14）。
//
// 节奏 conf.recompute.interval（默认 5min），每轮 RecomputeScan：
// SCAN tt:recompute:* → 逐话题加锁 → 分平台回放 EWMA 链覆盖写 → 清标记。
// 幂等：覆盖写按 (series, t_bucket)，重跑收敛；标记 TTL 7d 防泄漏；
// 锁陈旧阈值 2×interval 崩溃自愈。
type RecomputeWorker struct {
	conf *biztt.Conf
	uc   *biztt.RecomputeUsecase
	log  *log.Helper

	mu     sync.Mutex
	stopCh chan struct{}
	wg     sync.WaitGroup

	// stopOnce 幂等关停守卫（B-25：重复 Stop 时 close 已关通道会 panic；Once 收敛）。
	stopOnce sync.Once
}

// NewRecomputeWorker 构造。
func NewRecomputeWorker(conf *biztt.Conf, uc *biztt.RecomputeUsecase, logger log.Logger) *RecomputeWorker {
	return &RecomputeWorker{
		conf:   conf,
		uc:     uc,
		log:    log.NewHelper(log.With(logger, "module", "worker/tt-recompute")),
		stopCh: make(chan struct{}),
	}
}

// Start 启动扫描循环。
func (w *RecomputeWorker) Start(ctx context.Context) error {
	if w.conf == nil || !w.conf.Enabled {
		w.log.Info("tt disabled, recompute worker not started")
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Add(1)
	go w.loop()
	w.log.Infof("tt recompute worker started: interval=%s scan_limit=%d",
		w.conf.Recompute.Interval, w.conf.Recompute.ScanLimit)
	return nil
}

// Stop 停止循环（等当前轮结束）。
func (w *RecomputeWorker) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.wg.Wait()
	w.log.Info("tt recompute worker stopped")
	return nil
}

func (w *RecomputeWorker) loop() {
	defer w.wg.Done()
	interval := w.conf.Recompute.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
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

func (w *RecomputeWorker) scanOnce() {
	interval := w.conf.Recompute.Interval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), interval)
	defer cancel()

	limit := w.conf.Recompute.ScanLimit
	if limit <= 0 {
		limit = 200
	}
	n, err := w.uc.RecomputeScan(ctx, limit)
	if err != nil {
		w.log.Warnf("recompute scan failed: %v", err)
		return
	}
	if n > 0 {
		w.log.Infof("recompute scan done: %d topics", n)
	}
}
