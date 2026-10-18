package icworker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/worker"
	contract "framework/pkg/mediacontract"
)

// NormalizerConsumer 消费 mm.intel.raw（B-8 第 4 步），驱动 归一化→去重→理解层→闸门→发布
// 主链路（B-8 第 5-8 步全部内聚于 biz.IcNormalizeUsecase.Process，worker 只做传输适配）。
//
// 投递纪律（worker.JSSubscribeOpts）：至少一次 + 手动 ACK。
//   - 处理失败 → NAK 重投（归一化本身幂等：含短链/去重键冲突的碰撞合并均为收敛操作）；
//   - 消息解码失败/契约缺失 intel_id → Term（毒消息，重投无意义）；
//   - Process 返回 (nil, nil)（条目已被清扫/半事务失败）→ ACK 丢弃。
type NormalizerConsumer struct {
	conf *biz.IcConfig
	core *core.CoreServer
	uc   *biz.IcNormalizeUsecase
	log  *log.Helper

	sub     *natsgo.Subscription
	mu      sync.Mutex
	wg      sync.WaitGroup
	stopped bool
}

func NewNormalizerConsumer(coreSrv *core.CoreServer, conf *biz.IcConfig, uc *biz.IcNormalizeUsecase, logger log.Logger) *NormalizerConsumer {
	return &NormalizerConsumer{
		conf: conf,
		core: coreSrv,
		uc:   uc,
		log:  log.NewHelper(log.With(logger, "module", "worker/ic-normalizer")),
	}
}

// Start 启动 JetStream 持久订阅；IC 关闭或 NATS 不可用时降级不阻塞服务启动。
func (s *NormalizerConsumer) Start(ctx context.Context) error {
	if s.conf == nil || !s.conf.Enabled {
		s.log.Info("ic disabled, normalizer consumer not started")
		return nil
	}
	nc := s.core.Nats().RawConn()
	if nc == nil {
		s.log.Warn("nats connection nil, ic normalizer consumer not started")
		return nil
	}
	js, err := nc.JetStream()
	if err != nil {
		s.log.Warnf("get jetstream context failed, ic normalizer consumer not started: %v", err)
		return nil
	}
	ensureStreamsWith(js, s.log, contract.StreamIntel)

	opts := append([]natsgo.SubOpt{natsgo.Durable(durableNormalizer), natsgo.DeliverAll()}, worker.JSSubscribeOpts()...)
	sub, err := js.Subscribe(contract.SubjectIntelRaw, s.handleMessage, opts...)
	if err != nil {
		s.log.Warnf("subscribe %s failed, ic normalizer consumer not started: %v", contract.SubjectIntelRaw, err)
		return nil
	}
	s.sub = sub
	s.log.Infof("ic normalizer consumer started: subject=%s durable=%s", contract.SubjectIntelRaw, durableNormalizer)
	return nil
}

// Stop 等待 in-flight 消息；保留 durable（不 Unsubscribe，重投由 JetStream 兜底）。
func (s *NormalizerConsumer) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
	s.log.Info("ic normalizer consumer stopped")
	return nil
}

// handleMessage 解码包身 → 归一化主链路。
func (s *NormalizerConsumer) handleMessage(msg *natsgo.Msg) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	// 归一化含短链展开 + 理解层 + 闸门 + 发布，取 60s 上限（短链 5 跳最坏 ~25s）。
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	env, err := contract.DecodeEnvelope(msg.Data)
	if err != nil {
		s.log.Warnf("decode envelope failed (term): %v", err)
		_ = msg.Term()
		return
	}
	var notice contract.RawIntelNotice
	if err := env.DecodePayload(&notice); err != nil {
		s.log.Warnf("decode raw notice failed (term): %v", err)
		_ = msg.Term()
		return
	}
	if notice.IntelID == "" {
		// 契约违例（B-8：memo 事件必须带 intel_id）。
		s.log.Warn("raw notice missing intel_id (term)")
		_ = msg.Term()
		return
	}

	if _, err := s.uc.Process(ctx, &notice, env.TraceID); err != nil {
		// 失败 → NAK 重投（JetStream 至少一次；归一化对重放幂等）。
		s.log.Warnf("normalize failed (nak): intel_id=%s err=%v", notice.IntelID, err)
		_ = msg.Nak()
		return
	}
	// 成功与「条目不存在(返回 nil,nil)」均 ACK：后者重投无意义。
	_ = msg.Ack()
}

var _ interface {
	Start(context.Context) error
	Stop(context.Context) error
} = (*NormalizerConsumer)(nil)
