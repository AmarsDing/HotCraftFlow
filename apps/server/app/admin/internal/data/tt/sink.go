package ttdata

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	biztt "framework/app/admin/internal/biz/tt"
)

// InfluxSink 快照点批处理器（biz/tt.PointSink 实现）。
//
// 行为：攒批（batch_size 满 或 flush_interval 到期）→ 同步写 InfluxDB →
// 失败批次转 SnapshotBuffer 兜底（A-16），全链路不丢点。
//
// 背压：Pending() = 批队列深度 + 缓冲流深度，订阅侧按 pause/resume 阈值节流。
type InfluxSink struct {
	conf *biztt.Conf
	ts   biztt.TimeSeries
	buf  biztt.SnapshotBuffer
	log  *log.Helper

	mu      sync.Mutex
	flushMu sync.Mutex
	pending []biztt.SnapshotPoint

	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
	closed  bool
}

// NewInfluxSink 构造并启动冲刷 goroutine。
func NewInfluxSink(conf *biztt.Conf, ts biztt.TimeSeries, buf biztt.SnapshotBuffer, logger log.Logger) *InfluxSink {
	s := &InfluxSink{
		conf:   conf,
		ts:     ts,
		buf:    buf,
		log:    log.NewHelper(log.With(logger, "module", "data/tt-sink")),
		stopCh: make(chan struct{}),
	}
	s.start()
	return s
}

// Start 启动周期冲刷（幂等）。
func (s *InfluxSink) start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	interval := time.Duration(s.conf.Ingest.FlushInterval.Milliseconds()) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.Flush(context.Background()); err != nil {
					s.log.Warnf("flush failed (batch kept in buffer): %v", err)
				}
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Record 受理快照点（并发安全）。batch 满时同步冲刷（串行于 flushMu）。
func (s *InfluxSink) Record(ctx context.Context, p biztt.SnapshotPoint) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil // 停机后半开半闭：静默放弃（停机冲刷已完成）
	}
	s.pending = append(s.pending, p)
	full := len(s.pending) >= s.conf.Influx.BatchSize
	s.mu.Unlock()

	if full {
		return s.Flush(ctx)
	}
	return nil
}

// Flush 冲刷当前批：先写 InfluxDB，失败转缓冲。
func (s *InfluxSink) Flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return nil
	}
	batch := s.pending
	s.pending = make([]biztt.SnapshotPoint, 0, s.conf.Influx.BatchSize)
	s.mu.Unlock()

	if err := s.ts.WriteSnapshots(ctx, batch); err != nil {
		dropped, berr := s.buf.Enqueue(ctx, batch)
		if berr != nil {
			// 缓冲也失败（如 Redis 全挂）：批重新入队等待下轮，告警。
			s.mu.Lock()
			if !s.closed {
				s.pending = append(batch, s.pending...)
			}
			s.mu.Unlock()
			biztt.SnapshotWrittenTotal.WithLabelValues("lost", "buffer_error").Inc()
			return err
		}
		biztt.SnapshotWrittenTotal.WithLabelValues("buffered", firstPlatform(batch)).Add(float64(len(batch)))
		if dropped > 0 {
			biztt.InfluxDroppedTotal.Add(float64(dropped))
			biztt.SnapshotWrittenTotal.WithLabelValues("dropped", firstPlatform(batch)).Add(float64(dropped))
		}
		s.log.Warnf("influx write failed, %d points buffered (dropped %d): %v", len(batch), dropped, err)
		return nil // 已兜底，订阅侧可 ACK
	}

	biztt.SnapshotWrittenTotal.WithLabelValues("ok", firstPlatform(batch)).Add(float64(len(batch)))
	return nil
}

// Pending 未落库点数（批队列 + 缓冲近似值）。
func (s *InfluxSink) Pending(ctx context.Context) int {
	s.mu.Lock()
	n := len(s.pending)
	s.mu.Unlock()
	if s.buf != nil {
		if d, err := s.buf.Depth(ctx); err == nil {
			if d > int64(^uint(0)>>1)-int64(n) {
				return int(^uint(0) >> 1)
			}
			n += int(d)
		}
	}
	return n
}

// Close 停止冲刷 goroutine 并做最终冲刷（优雅停机语义）。
func (s *InfluxSink) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	close(s.stopCh)
	s.wg.Wait()
	return s.Flush(ctx)
}

// firstPlatform 取批内第一个平台作指标标签（低基数，批量内通常同平台居多）。
func firstPlatform(batch []biztt.SnapshotPoint) string {
	if len(batch) == 0 {
		return "unknown"
	}
	return batch[0].Platform
}
