package biz

import "context"

// InternalMessageNotifier 站内信投递端口（骨架保留，供 site_msg worker 使用）。
type InternalMessageNotifier interface {
	CreateSystemMessage(ctx context.Context, subject, content string, recipientIDs []uint32) error
}
