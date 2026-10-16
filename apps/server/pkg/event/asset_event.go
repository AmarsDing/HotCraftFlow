// Package event 提供跨模块共享的 NATS 事件信封类型。
//
// 历史包：信封与各域 subject 常量已收敛至 framework/pkg/events（契约唯一源），
// 本包保留 AssetEvent 别名以兼容存量引用，新代码请直接使用 pkg/events。
package event

import "framework/pkg/events"

// AssetEvent 是 events.Envelope 的别名（JSON 线格式不变）。
type AssetEvent = events.Envelope
