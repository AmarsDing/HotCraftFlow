package biz

import (
	"context"
	"time"
)

// ToolLinkSsoTicket SSO 免密跳转一次性票据，biz 层内部契约。
type ToolLinkSsoTicket struct {
	Ticket     string
	UserID     uint32
	ToolLinkID uint32
	ExpireAt   time.Time
	UsedAt     *time.Time
}

// ToolLinkSsoTicketRepo SSO 票据仓储接口（由 data 层 toolLinkSsoTicketRepo 实现）。
type ToolLinkSsoTicketRepo interface {
	// Create 插入新票据。
	Create(ctx context.Context, ticket *ToolLinkSsoTicket) error
	// GetByTicket 按票据串精确查询。
	GetByTicket(ctx context.Context, ticket string) (*ToolLinkSsoTicket, error)
	// MarkUsed 写入使用时间（一次性语义）。
	MarkUsed(ctx context.Context, ticket string, usedAt time.Time) error
}
