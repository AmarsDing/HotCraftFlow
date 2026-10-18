package mediacontract

import (
	"encoding/json"
	"time"
)

// EventEnvelope 是所有 NATS 事件消息的强制信封（契约 §8.1）。
// 所有经 NATS 传输的契约消息必须包裹它，不得裸传业务载荷。
// 它是幂等（event_id）与链路追踪（trace_id）的唯一载体。
type EventEnvelope struct {
	EventID    string          `json:"event_id"`    // evt_ 前缀，全局唯一幂等键
	TraceID    string          `json:"trace_id"`    // 贯穿全链路，透传不重建
	Producer   string          `json:"producer"`    // ic | tt | mia | mme | map
	OccurredAt time.Time       `json:"occurred_at"` // 事件发生时间 UTC（非投递时间）
	Subject    string          `json:"subject"`     // mm.<子系统>.<事件>.v<版本>
	Payload    json.RawMessage `json:"payload"`     // 七份契约之一（含 api_version）
}

// NewEnvelope 构造一个信封。occurredAt 取事件发生时间。
func NewEnvelope(producer, subject, traceID string, payload any) (*EventEnvelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if traceID == "" {
		traceID = ULID() // 链路起点新建 trace_id
	}
	return &EventEnvelope{
		EventID:    NewEventID(),
		TraceID:    traceID,
		Producer:   producer,
		OccurredAt: UTCNow(),
		Subject:    subject,
		Payload:    b,
	}, nil
}

// Encode 序列化信封（发布到 NATS 用）。
func (e *EventEnvelope) Encode() ([]byte, error) {
	return json.Marshal(e)
}

// DecodeEnvelope 反序列化信封（消费侧用）。
func DecodeEnvelope(b []byte) (*EventEnvelope, error) {
	var e EventEnvelope
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// DecodePayload 把信封内的载荷解码到目标结构。
func (e *EventEnvelope) DecodePayload(target any) error {
	return json.Unmarshal(e.Payload, target)
}

// NATS subject 常量（契约 §8.2）。版本在 subject 里，便于灰度切换与并存消费。
const (
	SubjectIntelRaw         = "mm.intel.raw"
	SubjectIntelNormalized  = "mm.intel.normalized.v1"
	SubjectIntelBlocked     = "mm.intel.blocked.v1"
	SubjectTrendSnapshot    = "mm.trend.snapshot.v1"
	SubjectTrendBurst       = "mm.trend.burst.v1"
	SubjectTrendCandidate   = "mm.trend.candidate.v1"
	SubjectImpactCard       = "mm.impact.card.v1"
	SubjectMMEVariantReady  = "mm.mme.variant.ready.v1"
	SubjectMapDispatch      = "mm.map.dispatch.v1"
	SubjectMapFeedback      = "mm.map.feedback.v1"
)

// JetStream Stream 名（契约 §8.2 配置基线）。
const (
	StreamIntel  = "MM_INTEL"
	StreamTrend  = "MM_TREND"
	StreamImpact = "MM_IMPACT"
	StreamMedia  = "MM_MEDIA"
	StreamPub    = "MM_PUB"
)
