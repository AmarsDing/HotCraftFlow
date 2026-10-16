package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"

	"framework/pkg/event"
)

// AssetEvent 类型别名，指向共享包 framework/pkg/event。
// 消除各模块重复定义，保持现有代码 *AssetEvent 引用不变。
type AssetEvent = event.AssetEvent

// EventPublisher 事件发布端口，由 core 层 NATS 实现。
type EventPublisher interface {
	PublishAssetEvent(ctx context.Context, event *AssetEvent) error
}

// PublishAssetEventWithRetry 组装 AssetEvent 并发布；失败时写重试队列（§4.4），
// 供未内嵌发布能力的 Usecase（资产分组/标签等）复用与 AssetUsecase 一致的可靠投递模式。
func PublishAssetEventWithRetry(ctx context.Context, publisher EventPublisher, retryRepo AssetNatsRetryRepo, logger *log.Helper, topic string, assetID uint32, operatorID *uint32, payload map[string]any) {
	evt := &AssetEvent{
		EventID:      uuid.New().String(),
		EventType:    topic,
		EventVersion: "1.0",
		OccurredAt:   time.Now().Format(time.RFC3339),
		AssetID:      assetID,
		OperatorID:   operatorID,
		Payload:      payload,
	}

	if publisher == nil {
		return
	}
	if err := publisher.PublishAssetEvent(ctx, evt); err != nil {
		logger.Errorf("publish nats event %s failed: %s, enqueueing retry", topic, err.Error())
		eventMap := map[string]any{
			"event_id":      evt.EventID,
			"event_type":    evt.EventType,
			"event_version": evt.EventVersion,
			"occurred_at":   evt.OccurredAt,
			"asset_id":      evt.AssetID,
			"payload":       payload,
		}
		if operatorID != nil {
			eventMap["operator_id"] = *operatorID
		}
		if retryRepo != nil {
			if retryErr := retryRepo.EnqueueRetry(ctx, topic, eventMap, err.Error()); retryErr != nil {
				logger.Errorf("enqueue nats retry for %s also failed: %s", topic, retryErr.Error())
			}
		}
	}
}

// RetryItem 重试队列中的一条待重试记录（§4.4）。
type RetryItem struct {
	ID          uint32
	Topic       string
	Payload     map[string]any
	RetryCount  int
	MaxRetry    int
}

// ToEvent 将存储的 payload map 重建为 AssetEvent，供重新发布。
func (r *RetryItem) ToEvent() *AssetEvent {
	event := &AssetEvent{
		EventType:    r.Topic,
		EventVersion: "1.0",
		Payload:      map[string]any{},
	}
	if v, ok := r.Payload["event_id"].(string); ok {
		event.EventID = v
	}
	if v, ok := r.Payload["event_type"].(string); ok {
		event.EventType = v
	}
	if v, ok := r.Payload["event_version"].(string); ok {
		event.EventVersion = v
	}
	if v, ok := r.Payload["occurred_at"].(string); ok {
		event.OccurredAt = v
	}
	if v, ok := r.Payload["trace_id"].(string); ok {
		event.TraceID = v
	}
	if v, ok := r.Payload["asset_id"]; ok {
		switch n := v.(type) {
		case float64:
			event.AssetID = uint32(n)
		case uint32:
			event.AssetID = n
		case int:
			event.AssetID = uint32(n)
		}
	}
	if v, ok := r.Payload["operator_id"]; ok {
		switch n := v.(type) {
		case float64:
			u := uint32(n)
			event.OperatorID = &u
		case uint32:
			event.OperatorID = &n
		case int:
			u := uint32(n)
			event.OperatorID = &u
		}
	}
	if p, ok := r.Payload["payload"].(map[string]any); ok {
		event.Payload = p
	}
	return event
}

// AssetNatsRetryRepo NATS 事件重试队列仓储端口，发布失败时持久化待重试。
type AssetNatsRetryRepo interface {
	// EnqueueRetry 写入一条待重试记录（retry_count=0, next_retry_at=now+10s）。
	EnqueueRetry(ctx context.Context, topic string, payload map[string]any, lastError string) error
	// ListPending 查询 next_retry_at <= now 且 retry_count < max_retry 且 status=pending 的记录。
	ListPending(ctx context.Context, limit int, now time.Time) ([]*RetryItem, error)
	// MarkRetrySuccess 标记重试成功，删除记录（事件已投递，无需保留）。
	MarkRetrySuccess(ctx context.Context, id uint32) error
	// MarkRetryFailed 递增 retry_count，按退避更新 next_retry_at，记录最近错误。
	MarkRetryFailed(ctx context.Context, id uint32, retryCount int, nextRetryAt time.Time, lastError string) error
	// MarkDead 标记为死信（retry_count >= max_retry），触发告警。
	MarkDead(ctx context.Context, id uint32, lastError string) error
}
