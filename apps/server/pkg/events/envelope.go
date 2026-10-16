// Package events 是全站 NATS 事件契约的唯一定义源（single source of truth）。
//
// 背景：此前 alarm.events、weather.* 等 subject 字符串常量在 6+ 个服务重复声明，
// 信封结构体在各服务 internal/biz 下复制粘贴，曾出现压缩配置与消费端不兼容等隐患。
// 本包集中定义：
//   - Envelope：统一事件信封（JSON 线格式与历史 pkg/event.AssetEvent 完全一致）
//   - Subject 常量：按域分组，发布方与订阅方必须引用本包常量，禁止本地重声明
//   - Stream 定义：JetStream stream 词表，cmd/nats_setup 与各 publisher ensure 共用
//   - Codec：信封编解码（含 gzip 透明解压，修复"发布端压缩、消费端不解压"陷阱）
//
// 迁移纪律：新增事件主题时，先在本包登记 subject 常量与（如需）stream 词表，再写发布/订阅代码。
package events

import (
	"time"

	"github.com/google/uuid"
)

// 信封版本号。历史线上格式：绝大多数事件为 "1.0"，alarm.events 为 "2.0"。
const (
	Version1 = "1.0"
	Version2 = "2.0"
)

// Envelope NATS 事件统一信封。
//
// JSON tag 与历史 pkg/event.AssetEvent 逐字段一致，线格式零变化；
// pkg/event.AssetEvent 已改为本类型的别名，存量消费端无需改动。
type Envelope struct {
	EventID      string         `json:"event_id"`
	EventType    string         `json:"event_type"`
	EventVersion string         `json:"event_version"`
	OccurredAt   string         `json:"occurred_at"`
	AssetID      uint32         `json:"asset_id"`
	OperatorID   *uint32        `json:"operator_id,omitempty"`
	TraceID      string         `json:"trace_id,omitempty"`
	Payload      map[string]any `json:"payload"`
}

// New 以 v1.0 构造信封：EventID 随机 UUID，OccurredAt 取当前 UTC。
func New(subject string, assetID uint32, payload map[string]any) *Envelope {
	return NewWithVersion(subject, Version1, assetID, payload)
}

// NewWithVersion 按指定版本构造信封（alarm.events 传 Version2）。
func NewWithVersion(subject string, version string, assetID uint32, payload map[string]any) *Envelope {
	return &Envelope{
		EventID:      uuid.NewString(),
		EventType:    subject,
		EventVersion: version,
		OccurredAt:   time.Now().UTC().Format(time.RFC3339),
		AssetID:      assetID,
		Payload:      payload,
	}
}
