package ttworker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	natsgo "github.com/nats-io/nats.go"

	biztt "framework/app/admin/internal/biz/tt"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/worker"
	"framework/pkg/mediacontract"
)

var _ transport.Server = (*SnapshotWriter)(nil)

// backpressurePoll 背压等待的轮询间隔。
const backpressurePoll = 200 * time.Millisecond

// backpressureMaxWait 单次背压最长等待（超过 NAK 让 JetStream 重投，
// 避免 handler 超过 AckWait=60s 触发 in-flight 重投风暴）。
const backpressureMaxWait = 30 * time.Second

// SnapshotWriter 订阅 mm.trend.snapshot.v1，经 WriterUsecase 入批写 InfluxDB。
//
// C-9：InfluxDB 写入只发生在本消费端。
// 背压（A-16 配套）：PointSink.Pending ≥ pause 阈值 → 节流等待；A-16 的
// resume 阈值界面留给订阅侧（详情详见 03-详细设计 §2.4 拥塞矩阵）。
type SnapshotWriter struct {
	conf *biztt.Conf
	core *core.CoreServer
	uc   *biztt.WriterUsecase
	log  *log.Helper

	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
	sub     *natsgo.Subscription
}

// NewSnapshotWriter 构造。
func NewSnapshotWriter(conf *biztt.Conf, coreSrv *core.CoreServer, uc *biztt.WriterUsecase, logger log.Logger) *SnapshotWriter {
	return &SnapshotWriter{
		conf: conf,
		core: coreSrv,
		uc:   uc,
		log:  log.NewHelper(log.With(logger, "module", "worker/tt-ts-writer")),
	}
}

// Start 启动持久订阅。
func (s *SnapshotWriter) Start(ctx context.Context) error {
	if s.conf == nil || !s.conf.Enabled {
		s.log.Info("tt disabled, snapshot writer not started")
		return nil
	}
	nc := s.core.Nats().RawConn()
	if nc == nil {
		s.log.Warn("nats connection nil, tt snapshot writer not started")
		return nil
	}
	js, err := nc.JetStream()
	if err != nil {
		s.log.Warnf("get jetstream context failed, tt snapshot writer not started: %v", err)
		return nil
	}
	ensureStreamsWith(js, s.log, mediacontract.StreamTrend)

	opts := append([]natsgo.SubOpt{natsgo.Durable(durableTSWriter), natsgo.DeliverAll()}, worker.JSSubscribeOpts()...)
	sub, err := js.Subscribe(mediacontract.SubjectTrendSnapshot, s.handleMessage, opts...)
	if err != nil {
		s.log.Warnf("subscribe %s failed, tt snapshot writer not started: %v", mediacontract.SubjectTrendSnapshot, err)
		return nil
	}
	s.sub = sub
	s.log.Infof("tt snapshot writer started: subject=%s durable=%s", mediacontract.SubjectTrendSnapshot, durableTSWriter)
	return nil
}

// Stop 等待 in-flight 并冲刷批处理余量（优雅停机：不丢已受理点）。
func (s *SnapshotWriter) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
	if s.uc.Sink() != nil {
		if err := s.uc.Sink().Close(ctx); err != nil {
			s.log.Warnf("sink close flush failed: %v", err)
		}
	}
	s.log.Info("tt snapshot writer stopped")
	return nil
}

// handleMessage 背压判定后受理快照点。
func (s *SnapshotWriter) handleMessage(msg *natsgo.Msg) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if !s.awaitBackpressure(ctx) {
		// 等待超时仍未解除拥塞：NAK 重投，把压力还给 JetStream 而非阻塞消费线程。
		_ = msg.Nak()
		return
	}

	env, err := mediacontract.DecodeEnvelope(msg.Data)
	if err != nil {
		s.log.Warnf("decode snapshot envelope failed (poison, ack & drop): %v", err)
		_ = msg.Ack()
		return
	}
	if err := s.uc.HandleSnapshotData(ctx, env.Payload); err != nil {
		s.log.Warnf("record snapshot failed (nak): %v", err)
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

// awaitBackpressure Pending ≥ PauseDepth 时轮询等待至 ≤ ResumeDepth；ctx 超时返回 false。
func (s *SnapshotWriter) awaitBackpressure(ctx context.Context) bool {
	sink := s.uc.Sink()
	if sink == nil {
		return true
	}
	pause := s.conf.Buffer.PauseDepth
	resume := s.conf.Buffer.ResumeDepth

	deadline := time.NewTimer(backpressureMaxWait)
	defer deadline.Stop()
	ticker := time.NewTicker(backpressurePoll)
	defer ticker.Stop()
	paused := false

	for {
		pending := int64(sink.Pending(ctx))
		threshold := resume
		if !paused {
			threshold = pause
		}
		if pending < threshold {
			if paused {
				s.log.Infof("backpressure released: pending=%d resume=%d", pending, resume)
			}
			return true
		}
		if !paused {
			paused = true
			s.log.Warnf("backpressure engaged: pending=%d pause=%d", pending, pause)
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}
