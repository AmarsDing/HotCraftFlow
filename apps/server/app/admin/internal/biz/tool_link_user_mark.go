package biz

import (
	"context"
	"time"
)

// ToolLinkUserMark 用户对工具链接的标记（收藏 + 个人点击统计），biz 层内部契约。
type ToolLinkUserMark struct {
	ToolLinkID  uint32
	Favorite    bool
	ClickCount  uint32
	LastClickAt time.Time
}

// ToolLinkUserMarkRepo 用户标记仓储接口（由 data 层 toolLinkUserMarkRepo 实现）。
type ToolLinkUserMarkRepo interface {
	// ListByUser 查询某用户全部标记。
	ListByUser(ctx context.Context, userID uint32) ([]*ToolLinkUserMark, error)
	// SetFavorite 设置/取消收藏（不存在则插入）。
	SetFavorite(ctx context.Context, userID, toolLinkID uint32, favorite bool) error
	// RecordClick 记录一次点击（click_count+1 并刷新 last_click_at，不存在则插入）。
	RecordClick(ctx context.Context, userID, toolLinkID uint32) error
}
