package ttworker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	biztt "framework/app/admin/internal/biz/tt"
)

var _ transport.Server = (*BufferReplay)(nil)

// BufferReplay InfluxDB 写失败缓冲的周期重放 worker（A-16）。
//
// 节奏 conf.buffer.replay_interval（默认 15s），每轮：ReadBatch →
// TimeSeries.WriteSnapshots → Ack（成功）/ Nack（失败，超限转 poison）。
// InfluxDB 降级窗口内积压的点全部经此补齐，不落对账人工活。
type BufferReplay struct {
	conf *biztt.Conf
	buf  biztt.SnapshotBuffer
	ts   biztt.TimeSeries
	log  *log.Helper

	mu     sync.Mutex
	stopCh chan struct{}
	wg     sync.WaitGroup

	// stopOnce 幂等关停守卫（B-25：重复 Stop 时 close 已关通道会 panic；Once 收敛）。
	stopOnce sync.Once
}

// NewBufferReplay 构造。
func NewBufferReplay(conf *biztt.Conf, buf biztt.SnapshotBuffer, ts biztt.TimeSeries, logger log.Logger) *BufferReplay {
	return &BufferReplay{
		conf:   conf,
		buf:    buf,
		ts:     ts,
		log:    log.NewHelper(log.With(logger, "module", "worker/tt-buffer-replay")),
		stopCh: make(chan struct{}),
	}
}

// Start 启动重放循环。
func (w *BufferReplay) Start(ctx context.Context) error {
	if w.conf == nil || !w.conf.Enabled {
		w.log.Info("tt disabled, buffer replay not started")
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wg.Add(1)
	go w.loop()
	w.log.Infof("tt buffer replay started: interval=%s batch=%d", w.conf.Buffer.ReplayInterval, w.conf.Buffer.ReplayBatch)
	return nil
}

// Stop 停止循环（等当前轮结束）。
func (w *BufferReplay) Stop(ctx context.Context) error {
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
	w.wg.Wait()
	w.log.Info("tt buffer replay stopped")
	return nil
}

// loop 周期重放。
func (w *BufferReplay) loop() {
	defer w.wg.Done()
	interval := w.conf.Buffer.ReplayInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.replayOnce()
		}
	}
}

// replayOnce 单轮重放：一口气跑到流空或出错（出错等下轮）。
func (w *BufferReplay) replayOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for {
		points, batchID, err := w.buf.ReadBatch(ctx, w.conf.Buffer.ReplayBatch)
		if err != nil {
			w.log.Warnf("read buffer batch failed: %v", err)
			biztt.BufferReplayTotal.WithLabelValues("read_error").Inc()
			return
		}
		if len(points) == 0 || batchID == "" {
			return
		}

		if err := w.ts.WriteSnapshots(ctx, points); err != nil {
			poisoned, nerr := w.buf.Nack(ctx, batchID, points, w.conf.Buffer.MaxAttempts)
			if nerr != nil {
				w.log.Warnf("nack batch failed: %v", nerr)
			}
			w.log.Warnf("replay write failed (%d points, poisoned entries moved=%v): %v", len(points), poisoned, err)
			if poisoned {
				biztt.BufferReplayTotal.WithLabelValues("poison").Inc()
			}
			biztt.BufferReplayTotal.WithLabelValues("failed").Inc()
			return // InfluxDB 仍异常：退出等下轮，不空转
		}

		if err := w.buf.Ack(ctx, batchID); err != nil {
			// Ack 失败仅告警：已落库的点由 (series,ts) 覆盖幂等兜底一次重复写。
			w.log.Warnf("ack batch failed (duplicate write possible, idempotent at series level): %v", err)
		}
		biztt.BufferReplayTotal.WithLabelValues("ok").Add(float64(len(points)))
	}
}
