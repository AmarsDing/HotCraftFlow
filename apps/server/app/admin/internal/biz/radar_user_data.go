package biz

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
)

// ─────────────────────────────────────────────────────────────
// Radar 用户域数据（盯盘 / 盯盘事件 / 用户配置）。
// 行级隔离口径：所有查询/变更都必须带 user_id（来自平台 JWT，见 auth.FromContext）。
// 配置类数据（GitHub Token / LLM / 克隆）为平台级单行（userID=0），仅管理员可写。
// ─────────────────────────────────────────────────────────────

// RadarSystemUserID 平台级配置行的 sentinel user_id（token/LLM/克隆设置为全局单行）。
const RadarSystemUserID uint32 = 0

// RadarMaxWatchEventsPerUser 单用户盯盘事件保留上限（超出按最旧优先删除）。
const RadarMaxWatchEventsPerUser = 200

// radarFullNamePattern owner/repo：GitHub 用户/组织名与仓库名字符合式。
// 注意：Go regexp 不支持前瞻 (?=)，原写法会在 init 期 panic；此处用等价非前瞻写法，
// 并由 normalizeFullName 额外校验 owner ≤39 / repo ≤100 的长度上限。
var radarFullNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?/[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// normalizeFullName 校验并规范化 full_name（owner/repo）。
func normalizeFullName(fullName string) (string, error) {
	name := strings.TrimSpace(fullName)
	if name == "" {
		return "", adminV1.ErrorBadRequest("invalid parameter: full_name is required")
	}
	owner, repo, ok := strings.Cut(name, "/")
	if !ok || len(name) > 200 || len(owner) > 39 || len(repo) > 100 || !radarFullNamePattern.MatchString(name) {
		return "", adminV1.ErrorBadRequest("invalid parameter: full_name must match owner/repo")
	}
	return name, nil
}

func formatStarEventMessage(fullName string, grown int64) string {
	return fmt.Sprintf("%s 星标 +%d", fullName, grown)
}

// RadarWatch 用户对某仓库的盯盘记录。
type RadarWatch struct {
	ID            uint32
	FullName      string
	BaselineStars int64
	Stars         int64
	StarsDelta    int64
	UnreadCount   uint32
	HTMLURL       string
	Sparkline     []int64
	CreateTime    time.Time
	UpdateTime    time.Time
}

// RadarWatchEvent 盯盘仓库的变更事件。
type RadarWatchEvent struct {
	ID         uint32
	FullName   string
	Kind       string // stars | release | description
	Message    string
	Delta      int64
	Read       bool
	CreateTime time.Time
}

// RadarUserConfigData 用户/平台雷达配置（密钥字段由 repo 层加密存储）。
type RadarUserConfigData struct {
	GitHubToken  string
	LLMEnabled   bool
	LLMBaseURL   string
	LLMAPIKey    string
	LLMModel     string
	CloneEnabled bool
	CloneDir     string
}

// RadarWatchRepo 盯盘仓储（行级强制 user_id）。
type RadarWatchRepo interface {
	ListByUser(ctx context.Context, userID uint32) ([]*RadarWatch, error)
	GetByFullName(ctx context.Context, userID uint32, fullName string) (*RadarWatch, error)
	// Upsert 不存在则创建（记录基线星标），存在则原样返回。
	Upsert(ctx context.Context, userID uint32, fullName string, baselineStars int64) (*RadarWatch, error)
	// Delete 按 (user_id, id) 删除；返回是否删除成功。
	Delete(ctx context.Context, userID uint32, id uint32) (bool, error)
	// UpdateStars 刷新星标快照与 sparkline。
	UpdateStars(ctx context.Context, userID uint32, fullName string, stars, delta int64, sparkline []int64) error
	// SetUnreadCount 设置未读数。
	SetUnreadCount(ctx context.Context, userID uint32, fullName string, n uint32) error
}

// RadarWatchEventRepo 盯盘事件仓储（行级强制 user_id）。
type RadarWatchEventRepo interface {
	ListByUser(ctx context.Context, userID uint32, unreadOnly bool) ([]*RadarWatchEvent, error)
	// CreateBatch 批量写入事件（自动按用户裁剪到 RadarMaxWatchEventsPerUser）。
	CreateBatch(ctx context.Context, userID uint32, events []*RadarWatchEvent) error
	// MarkRead 按 id 批量标记已读（ids 为空返回 0，不做全量标记）。
	MarkRead(ctx context.Context, userID uint32, ids []uint32) (int, error)
	// CountUnreadByRepo 某仓库未读事件数。
	CountUnreadByRepo(ctx context.Context, userID uint32, fullName string) (uint32, error)
	// DeleteOlderThan 保留期清理（返回删除行数）。
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}

// RadarUserConfigRepo 雷达配置仓储（平台级单行，密钥加密存储）。
type RadarUserConfigRepo interface {
	// Get 读取配置；未配置返回 (nil, nil)。
	Get(ctx context.Context, userID uint32) (*RadarUserConfigData, error)
	// Save 写入配置（不存在则创建，存在则更新）。
	Save(ctx context.Context, userID uint32, cfg *RadarUserConfigData) error
}

// ─────────────────────────────────────────────────────────────
// RadarWatchUsecase 盯盘用例。
// ─────────────────────────────────────────────────────────────

type RadarWatchUsecase struct {
	watchRepo RadarWatchRepo
	eventRepo RadarWatchEventRepo
	log       *log.Helper
}

func NewRadarWatchUsecase(watchRepo RadarWatchRepo, eventRepo RadarWatchEventRepo, logger log.Logger) *RadarWatchUsecase {
	return &RadarWatchUsecase{
		watchRepo: watchRepo,
		eventRepo: eventRepo,
		log:       log.NewHelper(log.With(logger, "module", "radar/watch/usecase")),
	}
}

// List 当前用户盯盘列表（按加入时间倒序）。
func (uc *RadarWatchUsecase) List(ctx context.Context, userID uint32) ([]*RadarWatch, error) {
	items, err := uc.watchRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	// repo 层按 create_time 正序返回，这里翻转为倒序（新加入在前）
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

// Add 添加盯盘（full_name 格式校验 + 幂等）。
func (uc *RadarWatchUsecase) Add(ctx context.Context, userID uint32, fullName string) (*RadarWatch, error) {
	name, err := normalizeFullName(fullName)
	if err != nil {
		return nil, err
	}
	existing, err := uc.watchRepo.GetByFullName(ctx, userID, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return uc.watchRepo.Upsert(ctx, userID, name, 0)
}

// Remove 移除盯盘（按 user_id + id，删除不存在返回 false）。
func (uc *RadarWatchUsecase) Remove(ctx context.Context, userID uint32, id uint32) (bool, error) {
	return uc.watchRepo.Delete(ctx, userID, id)
}

// ListEvents 盯盘事件（unreadOnly 时仅未读）。
func (uc *RadarWatchUsecase) ListEvents(ctx context.Context, userID uint32, unreadOnly bool) ([]*RadarWatchEvent, error) {
	return uc.eventRepo.ListByUser(ctx, userID, unreadOnly)
}

// MarkEventsRead 按 id 批量已读；ids 为空返回错误（防止误清空未读）。
func (uc *RadarWatchUsecase) MarkEventsRead(ctx context.Context, userID uint32, ids []uint32) (int, error) {
	if len(ids) == 0 {
		return 0, adminV1.ErrorBadRequest("invalid parameter: ids must not be empty")
	}
	n, err := uc.eventRepo.MarkRead(ctx, userID, ids)
	if err != nil {
		return 0, err
	}
	// 重算各仓库未读数
	if err := uc.recountUnread(ctx, userID); err != nil {
		uc.log.Warnf("recount unread after mark-read failed (user_id=%d): %v", userID, err)
	}
	return n, nil
}

// recountUnread 按事件表重算每个盯盘仓库的未读数。
func (uc *RadarWatchUsecase) recountUnread(ctx context.Context, userID uint32) error {
	watches, err := uc.watchRepo.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, w := range watches {
		n, err := uc.eventRepo.CountUnreadByRepo(ctx, userID, w.FullName)
		if err != nil {
			return err
		}
		if err := uc.watchRepo.SetUnreadCount(ctx, userID, w.FullName, n); err != nil {
			return err
		}
	}
	return nil
}

// SyncStars 用最新星标快照刷新盯盘并在跨越阈值时生成事件。
// threshold 为相对上一次已记录事件的最小新增量（<=0 关闭事件生成）。
func (uc *RadarWatchUsecase) SyncStars(ctx context.Context, userID uint32, fullName string, stars int64, sparkline []int64, threshold int64) error {
	w, err := uc.watchRepo.GetByFullName(ctx, userID, fullName)
	if err != nil {
		return err
	}
	if w == nil {
		return nil // 未盯盘该仓库
	}
	delta := stars - w.BaselineStars
	if err := uc.watchRepo.UpdateStars(ctx, userID, fullName, stars, delta, sparkline); err != nil {
		return err
	}
	grown := delta - w.StarsDelta
	if threshold > 0 && grown >= threshold {
		ev := &RadarWatchEvent{
			FullName: fullName,
			Kind:     "stars",
			Message:  formatStarEventMessage(fullName, grown),
			Delta:    grown,
		}
		if err := uc.eventRepo.CreateBatch(ctx, userID, []*RadarWatchEvent{ev}); err != nil {
			return err
		}
		n, err := uc.eventRepo.CountUnreadByRepo(ctx, userID, fullName)
		if err != nil {
			return err
		}
		return uc.watchRepo.SetUnreadCount(ctx, userID, fullName, n)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// RadarNotifyUsecase 通知用例：盯盘事件即通知（消除与平台通知体系的重复存储）。
// ─────────────────────────────────────────────────────────────

type RadarNotifyUsecase struct {
	eventRepo RadarWatchEventRepo
}

func NewRadarNotifyUsecase(eventRepo RadarWatchEventRepo) *RadarNotifyUsecase {
	return &RadarNotifyUsecase{eventRepo: eventRepo}
}

// ListNotifications 通知=最近盯盘事件（unreadOnly 时仅未读）。
func (uc *RadarNotifyUsecase) ListNotifications(ctx context.Context, userID uint32, unreadOnly bool) ([]*RadarWatchEvent, error) {
	return uc.eventRepo.ListByUser(ctx, userID, unreadOnly)
}

// MarkNotificationsRead 通知已读=事件已读（ids 为空返回错误）。
func (uc *RadarNotifyUsecase) MarkNotificationsRead(ctx context.Context, userID uint32, ids []uint32) (int, error) {
	if len(ids) == 0 {
		return 0, adminV1.ErrorBadRequest("invalid parameter: ids must not be empty")
	}
	return uc.eventRepo.MarkRead(ctx, userID, ids)
}

// UnreadCount 未读通知数。
func (uc *RadarNotifyUsecase) UnreadCount(ctx context.Context, userID uint32) (int, error) {
	items, err := uc.eventRepo.ListByUser(ctx, userID, true)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}
