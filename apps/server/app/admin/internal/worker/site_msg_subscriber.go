package worker

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	natsgo "github.com/nats-io/nats.go"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/core"
	"framework/pkg/events"
)

var _ transport.Server = (*SiteMsgSubscriber)(nil)

// 10 通知服务站内消息主题（moniternotice SiteMsgAdapter 发布）。
const subjectNoticeSiteMsg = events.SubjectNoticeSiteMsg

// siteMsgDurable JetStream 持久订阅名（消费进度独立于其他订阅者）。
const siteMsgDurable = "admin-notice-site-msg"

// siteMsgDedupTTL event_id 幂等去重标记 TTL。
const siteMsgDedupTTL = 24 * time.Hour

// SiteMsgSubscriber 订阅 notice.site_msg 站内消息事件并落库到站内消息表。
//
// 背景：moniternotice 的站内信渠道适配器只负责把消息发布到 NATS，
// 本订阅器是站内信链路的最终消费者——写入 notification_messages（PUBLISHED）
// 与 notification_message_recipients（RECEIVED），前端轮询即可触达用户。
//
// 幂等：event_id Redis SET NX EX 24h 去重；处理失败删除标记并 NAK 让 JetStream 重投。
type SiteMsgSubscriber struct {
	core     *core.CoreServer
	notifier biz.InternalMessageNotifier
	redis    biz.RedisStore
	log      *log.Helper

	mu      sync.Mutex // 保护 stopped（停止标记与 handleMessage 的 wg.Add 配对，防止 Add 与 Wait 竞态）
	stopped bool
	wg      sync.WaitGroup
}

func NewSiteMsgSubscriber(coreSrv *core.CoreServer, notifier biz.InternalMessageNotifier, redis biz.RedisStore, logger log.Logger) *SiteMsgSubscriber {
	return &SiteMsgSubscriber{
		core:     coreSrv,
		notifier: notifier,
		redis:    redis,
		log:      log.NewHelper(log.With(logger, "module", "site-msg-subscriber")),
	}
}

// Start 启动 JetStream 持久订阅；NATS/JetStream 不可用时降级为仅告警，不阻塞服务启动。
func (s *SiteMsgSubscriber) Start(ctx context.Context) error {
	nc := s.core.Nats().RawConn()
	if nc == nil {
		s.log.Warn("nats connection nil, site msg subscriber not started")
		return nil
	}

	js, err := nc.JetStream()
	if err != nil {
		s.log.Warnf("get jetstream context failed, site msg subscriber not started: %v", err)
		return nil
	}

	// MaxDeliver/AckWait 见 jetstream_policy.go
	opts := append([]natsgo.SubOpt{natsgo.Durable(siteMsgDurable), natsgo.DeliverAll()}, jsSubscribeOpts()...)
	_, err = js.Subscribe(
		subjectNoticeSiteMsg,
		s.handleMessage,
		opts...,
	)
	if err != nil {
		s.log.Warnf("subscribe %s failed, site msg subscriber not started: %v", subjectNoticeSiteMsg, err)
		return nil
	}

	s.log.Infof("site msg subscriber started: subject=%s durable=%s", subjectNoticeSiteMsg, siteMsgDurable)
	return nil
}

// Stop 等待 in-flight 处理完成。
// 刻意不 Unsubscribe/Drain：nats.go 对库创建的 durable consumer 在 Unsubscribe/Drain 时会
// 服务端删除（v1.52 文档明确），停机即丢消费进度、重启全量重放。保留 durable，由连接关闭
// 断开；未 ACK 消息由 JetStream 重投（消费幂等兜底）。
func (s *SiteMsgSubscriber) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.wg.Wait()
	s.log.Info("site msg subscriber stopped")
	return nil
}

// handleMessage 处理单条 NATS 消息；处理失败 NAK 让 JetStream 重投。
func (s *SiteMsgSubscriber) handleMessage(msg *natsgo.Msg) {
	// wg.Add 与 Stop 的 wg.Wait 配对必须在 mu 下完成：
	// 停止后到达的消息不再处理（不 ACK，交由 JetStream 重投），防止 Add 晚于 Wait 返回。
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

	if err := s.processMessage(ctx, msg); err != nil {
		s.log.Warnf("process site msg failed: %v", err)
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

// processMessage 解析事件信封，event_id 去重后写入站内消息表。
func (s *SiteMsgSubscriber) processMessage(ctx context.Context, msg *natsgo.Msg) error {
	if len(msg.Data) == 0 {
		return nil
	}

	// 消息体无法解析（gzip 魔数嗅探解压失败/非 JSON 信封）：重投无意义，吞掉（ACK）避免毒消息阻塞队列
	event, err := events.Decode(msg.Data)
	if err != nil {
		s.log.Warnf("decode site msg event failed (dropped): %v", err)
		return nil
	}

	// event_id 幂等去重（Redis SET NX EX 24h）
	dedupKey := ""
	if event.EventID != "" && s.redis != nil && s.redis.IsConfigured() {
		dedupKey = "notice:site_msg:dedup:" + event.EventID
		_, acquired, err := s.redis.TryLock(ctx, dedupKey, siteMsgDedupTTL)
		if err != nil {
			s.log.Warnf("check site msg dedup failed, proceed without dedup: %s", err.Error())
			dedupKey = ""
		} else if !acquired {
			s.log.Infof("site msg already processed, skipped: event_id=%s", event.EventID)
			return nil
		}
	}

	if err := s.deliver(ctx, event); err != nil {
		// 处理失败：删除去重标记让重投可再处理
		if dedupKey != "" {
			_ = s.redis.Del(ctx, dedupKey)
		}
		return err
	}
	return nil
}

// deliver 提取 payload 并创建站内消息。
// payload 结构（moniternotice Publisher.PublishSiteMsg）：
//
//	{"user_id": 1, "title": "...", "content": "...", "alarm_id": "...", "timestamp": 1700000000}
func (s *SiteMsgSubscriber) deliver(ctx context.Context, event *biz.AssetEvent) error {
	title, _ := event.Payload["title"].(string)
	content, _ := event.Payload["content"].(string)
	alarmID, _ := event.Payload["alarm_id"].(string)
	uidFloat, ok := payloadFloat(event.Payload["user_id"])

	if title == "" || !ok || uidFloat <= 0 {
		// 缺标题或接收人：毒消息，吞掉
		s.log.Warnf("site msg missing title/user_id (dropped): event_id=%s payload=%v", event.EventID, event.Payload)
		return nil
	}
	userID := uint32(uidFloat)

	if s.notifier == nil {
		s.log.Warn("internal message notifier nil, site msg dropped")
		return nil
	}

	if err := s.notifier.CreateSystemMessage(ctx, title, content, []uint32{userID}); err != nil {
		return err
	}
	s.log.Infof("site msg delivered: event_id=%s user_id=%d alarm_id=%s title=%s", event.EventID, userID, alarmID, title)
	return nil
}
