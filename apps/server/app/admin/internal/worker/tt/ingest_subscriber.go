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

var _ transport.Server = (*IngestSubscriber)(nil)

// IngestSubscriber 订阅 mm.intel.normalized.v1（① IC 归一化产出），驱动
// IngestUsecase 完成话题归并 + 构点 + 快照事件发布。
//
// 投递纪律复用 worker.JSSubscribeOpts（MaxDeliver=10 / AckWait=60s）；
// 处理结果映射：ResultError → NAK 重投，其余（含毒消息）→ ACK。
type IngestSubscriber struct {
	conf *biztt.Conf
	core *core.CoreServer
	uc   *biztt.IngestUsecase
	log  *log.Helper

	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
	sub     *natsgo.Subscription
}

// NewIngestSubscriber 构造。
func NewIngestSubscriber(conf *biztt.Conf, coreSrv *core.CoreServer, uc *biztt.IngestUsecase, logger log.Logger) *IngestSubscriber {
	return &IngestSubscriber{
		conf: conf,
		core: coreSrv,
		uc:   uc,
		log:  log.NewHelper(log.With(logger, "module", "worker/tt-ingest")),
	}
}

// Start 启动 JetStream 持久订阅；NATS 不可用或 TT 关闭时降级不阻塞服务启动。
func (s *IngestSubscriber) Start(ctx context.Context) error {
	if s.conf == nil || !s.conf.Enabled {
		s.log.Info("tt disabled, ingest subscriber not started")
		return nil
	}
	nc := s.core.Nats().RawConn()
	if nc == nil {
		s.log.Warn("nats connection nil, tt ingest subscriber not started")
		return nil
	}
	js, err := nc.JetStream()
	if err != nil {
		s.log.Warnf("get jetstream context failed, tt ingest subscriber not started: %v", err)
		return nil
	}
	ensureStreamsWith(js, s.log, mediacontract.StreamIntel)

	durable := s.conf.Ingest.Durable
	if durable == "" {
		durable = durableIngestIntel
	}
	opts := append([]natsgo.SubOpt{natsgo.Durable(durable), natsgo.DeliverAll()}, worker.JSSubscribeOpts()...)
	sub, err := js.Subscribe(mediacontract.SubjectIntelNormalized, s.handleMessage, opts...)
	if err != nil {
		s.log.Warnf("subscribe %s failed, tt ingest subscriber not started: %v", mediacontract.SubjectIntelNormalized, err)
		return nil
	}
	s.sub = sub
	s.log.Infof("tt ingest subscriber started: subject=%s durable=%s", mediacontract.SubjectIntelNormalized, durable)
	return nil
}

// Stop 等待 in-flight 消息；保留 durable（不 Unsubscribe，重投由 JetStream 兜底）。
func (s *IngestSubscriber) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
	s.log.Info("tt ingest subscriber stopped")
	return nil
}

// handleMessage 处理单条归一化情报。
func (s *IngestSubscriber) handleMessage(msg *natsgo.Msg) {
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

	result, err := s.uc.HandleIntelData(ctx, msg.Data)
	if err != nil {
		// ResultError：NAK 重投（JetStream 至少一次 + 幂等三层防线兜底）。
		s.log.Warnf("handle intel failed (nak): result=%s err=%v", result, err)
		_ = msg.Nak()
		return
	}
	if result != biztt.ResultOK {
		s.log.Debugf("intel handled: result=%s", result)
	}
	_ = msg.Ack()
}
