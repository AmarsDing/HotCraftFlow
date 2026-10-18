package miaworker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	natsgo "github.com/nats-io/nats.go"

	"framework/app/admin/internal/biz"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/core"
	"framework/app/admin/internal/worker"
	"framework/pkg/mediacontract"
)

var _ transport.Server = (*IntelSubscriber)(nil)

// IntelSubscriber 订阅 mm.intel.normalized.v1（① IC 归一化产出），
// 过滤 C_market 类目后驱动 IngestUsecase 主分析链路（FR-MIA-01~06/12）。
//
// 消费纪律（对齐 worker/jetstream_policy.go）：
//   - JetStream 持久订阅（durable=mia-ingest-intel，进度独立于 TT）；
//   - event_id Redis SET NX 幂等（至少 once 重投去重）；
//   - 处理错误 → NAK 重投（MaxDeliver=10 上限后停投）；
//   - 毒消息（解码/契约校验失败）→ Warn + ACK 丢弃（重投无意义）。
type IntelSubscriber struct {
	cfg   bizmia.Config
	core  *core.CoreServer
	uc    *bizmia.IngestUsecase
	redis biz.RedisStore
	log   *log.Helper

	mu      sync.Mutex
	stopped bool
	wg      sync.WaitGroup
	sub     *natsgo.Subscription
}

// NewIntelSubscriber 构造。cfg 由 bizmia.ResolveConfig 注入（已含默认值归一）。
func NewIntelSubscriber(
	cfg bizmia.Config,
	coreSrv *core.CoreServer,
	uc *bizmia.IngestUsecase,
	redis biz.RedisStore,
	logger log.Logger,
) *IntelSubscriber {
	return &IntelSubscriber{
		cfg:   cfg,
		core:  coreSrv,
		uc:    uc,
		redis: redis,
		log:   log.NewHelper(log.With(logger, "module", "worker/mia-ingest")),
	}
}

// Start 启动 JetStream 持久订阅；MIA 关闭或 NATS 不可用时降级，不阻塞服务启动。
func (s *IntelSubscriber) Start(ctx context.Context) error {
	if s.uc == nil || !s.cfg.Enabled {
		s.log.Info("mia disabled, intel subscriber not started")
		return nil
	}
	nc := s.core.Nats().RawConn()
	if nc == nil {
		s.log.Warn("nats connection nil, mia intel subscriber not started")
		return nil
	}
	js, err := nc.JetStream()
	if err != nil {
		s.log.Warnf("get jetstream context failed, mia intel subscriber not started: %v", err)
		return nil
	}
	ensureStreamsWith(js, s.log, mediacontract.StreamIntel, mediacontract.StreamImpact)

	opts := append([]natsgo.SubOpt{natsgo.Durable(durableIngestIntel), natsgo.DeliverAll()}, worker.JSSubscribeOpts()...)
	sub, err := js.Subscribe(mediacontract.SubjectIntelNormalized, s.handleMessage, opts...)
	if err != nil {
		s.log.Warnf("subscribe %s failed, mia intel subscriber not started: %v", mediacontract.SubjectIntelNormalized, err)
		return nil
	}
	s.sub = sub
	s.log.Infof("mia intel subscriber started: subject=%s durable=%s", mediacontract.SubjectIntelNormalized, durableIngestIntel)
	return nil
}

// Stop 等待 in-flight 消息；保留 durable（不 Unsubscribe，重投由 JetStream 兜底）。
func (s *IntelSubscriber) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
	s.log.Info("mia intel subscriber stopped")
	return nil
}

// miaDedupTTL event_id 去重窗（至少 once 重投去重；覆盖周末积压回放）。
const miaDedupTTL = 72 * time.Hour

// handleMessage 处理单条归一化情报。
func (s *IntelSubscriber) handleMessage(msg *natsgo.Msg) {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1) 信封解码（契约 §8.1：不得裸传业务载荷；格式毒消息 ACK 丢弃）。
	env, err := mediacontract.DecodeEnvelope(msg.Data)
	if err != nil {
		s.log.Warnf("decode envelope failed (poison, ack & drop): %v", err)
		_ = msg.Ack()
		return
	}

	// 2) event_id 幂等去重（JetStream 至少 once 重投）。Redis 未配置时 fail-open。
	if s.redis != nil && s.redis.IsConfigured() && env.EventID != "" {
		_, got, err := s.redis.TryLock(ctx, miaDedupKey(env.EventID), miaDedupTTL)
		if err != nil {
			s.log.Warnf("mia dedup check failed (fail-open): %v", err)
		} else if !got {
			_ = msg.Ack()
			return
		}
	}

	// 3) 载荷解码 + 契约校验（毒消息 ACK 丢弃）。
	var item mediacontract.IntelItem
	if err := env.DecodePayload(&item); err != nil {
		s.log.Warnf("decode IntelItem failed (poison, ack & drop): %v", err)
		_ = msg.Ack()
		return
	}
	if err := item.Validate(); err != nil {
		s.log.Warnf("IntelItem validate failed (poison, ack & drop): %v", err)
		_ = msg.Ack()
		return
	}

	// 4) 类目过滤：MIA 只消费 C_market（契约 §1 消费约定）；其余 ACL 静默（非本消费者职责）。
	if item.Category != mediacontract.CategoryCMarket {
		_ = msg.Ack()
		return
	}
	// defense-in-depth：blocked 条目不进自动流程（normalized 流本不应含）。
	if item.BlocksAutoFlow() {
		s.log.Warnf("blocked intel %s reached mia pipeline, dropped", item.IntelID)
		_ = msg.Ack()
		return
	}

	// 5) 主分析链路。处理错误 → NAK 重投（JetStream 兜底 + 幂等防线）。
	if _, err := s.uc.Ingest(ctx, toIngestItem(env, &item)); err != nil {
		s.log.Warnf("mia ingest failed (nak): intel=%s err=%v", item.IntelID, err)
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

// toIngestItem IntelItem 契约 → 管线输入。
// 发生时间：优先 published_at（事件发生时间），缺失回退 collected_at（契约允许兜底但记日志由上游保证）。
func toIngestItem(env *mediacontract.EventEnvelope, item *mediacontract.IntelItem) bizmia.IngestItem {
	occurredAt := item.Source.CollectedAt.UTC()
	if item.Source.PublishedAt != nil && !item.Source.PublishedAt.IsZero() {
		occurredAt = item.Source.PublishedAt.UTC()
	}
	return bizmia.IngestItem{
		IntelID:    item.IntelID,
		TraceID:    env.TraceID,
		Title:      item.Payload.Title,
		Text:       item.Payload.Text,
		Lang:       item.Payload.Lang,
		SourceURL:  item.Source.URL,
		OccurredAt: occurredAt,
		Entities:   item.Entities,
		// IsMacro 不在消费侧猜测——由事件分类器在管线内判定（policy/macro 实体与关键词）。
	}
}

func miaDedupKey(eventID string) string { return "mia:ingest:dedup:" + eventID }
