package biz

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
	"framework/pkg/storage"
)

// ToolLinkRepo 工具链接仓储接口（详细设计 §3.15，由 data 层 ToolLinkRepo 实现）。
type ToolLinkRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkResponse, error)
	Get(ctx context.Context, req *adminV1.GetToolLinkRequest) (*adminV1.ToolLink, error)
	Create(ctx context.Context, req *adminV1.CreateToolLinkRequest) error
	Update(ctx context.Context, req *adminV1.UpdateToolLinkRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteToolLinkRequest) error
	Move(ctx context.Context, req *adminV1.MoveToolLinkRequest) error

	// Reorder 批量重排：按 ids 下标重写 sort。
	Reorder(ctx context.Context, ids []uint32) error
	// IncrementClick 累计点击数 +1。
	IncrementClick(ctx context.Context, id uint32) error
	// UpdateHealth 写回健康检测结果。
	UpdateHealth(ctx context.Context, id uint32, status string, checkedAt time.Time) error
	// UpdateCategoryName 级联更新引用指定分类的冗余分类名。
	UpdateCategoryName(ctx context.Context, categoryID uint32, name string) error
	// ClearCategoryID 解除引用指定分类的关联。
	ClearCategoryID(ctx context.Context, categoryID uint32) error
	// ListAll 查询全部未删除记录（导出用）。
	ListAll(ctx context.Context) ([]*adminV1.ToolLink, error)
	// FilterExistingURLs 返回给定 URL 中已存在的子集（导入去重用）。
	FilterExistingURLs(ctx context.Context, urls []string) (map[string]bool, error)
}

// ToolLinkUsecase 工具链接门户业务逻辑。
// 自定义图标（IMAGE 类型）的孤儿文件清理由本层编排：更新/删除成功后移除不再引用的文件。
type ToolLinkUsecase struct {
	repo          ToolLinkRepo
	markRepo      ToolLinkUserMarkRepo
	ssoTicketRepo ToolLinkSsoTicketRepo
	store         *storage.LocalStorage // 可为 nil（storage.root 未配置时）
	log           *log.Helper
}

func NewToolLinkUsecase(repo ToolLinkRepo, markRepo ToolLinkUserMarkRepo, ssoTicketRepo ToolLinkSsoTicketRepo, store *storage.LocalStorage, logger log.Logger) *ToolLinkUsecase {
	return &ToolLinkUsecase{
		repo:          repo,
		markRepo:      markRepo,
		ssoTicketRepo: ssoTicketRepo,
		store:         store,
		log:           log.NewHelper(log.With(logger, "module", "tool-link/usecase/admin-service")),
	}
}

// List 查询列表；按 visible_roles 过滤（管理员放行全部），并注入当前用户的收藏/点击标记。
func (uc *ToolLinkUsecase) List(ctx context.Context, req *pagination.PagingRequest, userID uint32, roles []string, isAdmin bool) (*adminV1.ListToolLinkResponse, error) {
	resp, err := uc.repo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	// 角色可见性过滤：visible_roles 为空=全员可见；非空时需与当前用户角色有交集。
	if !isAdmin {
		filtered := make([]*adminV1.ToolLink, 0, len(resp.Items))
		for _, item := range resp.Items {
			if toolLinkVisibleTo(item.GetVisibleRoles(), roles) {
				filtered = append(filtered, item)
			}
		}
		resp.Total -= uint32(len(resp.Items) - len(filtered))
		resp.Items = filtered
	}

	// 注入当前用户标记（收藏/个人点击数）；失败仅记日志不阻断列表。
	uc.injectUserMarks(ctx, resp.Items, userID)

	return resp, nil
}

// toolLinkVisibleTo visibleRoles 为空=全员可见；否则需与 roles 有交集。
func toolLinkVisibleTo(visibleRoles, roles []string) bool {
	if len(visibleRoles) == 0 {
		return true
	}
	for _, vr := range visibleRoles {
		for _, r := range roles {
			if vr == r {
				return true
			}
		}
	}
	return false
}

// injectUserMarks 把当前用户的收藏/点击标记填充到列表项（计算字段，不落库）。
func (uc *ToolLinkUsecase) injectUserMarks(ctx context.Context, items []*adminV1.ToolLink, userID uint32) {
	if len(items) == 0 || userID == 0 || uc.markRepo == nil {
		return
	}
	marks, err := uc.markRepo.ListByUser(ctx, userID)
	if err != nil {
		uc.log.Warnf("list user marks failed (user_id=%d): %v", userID, err)
		return
	}
	markMap := make(map[uint32]*ToolLinkUserMark, len(marks))
	for _, m := range marks {
		markMap[m.ToolLinkID] = m
	}
	for _, item := range items {
		if m, ok := markMap[item.GetId()]; ok {
			item.Favorite = &m.Favorite
			item.MyClickCount = &m.ClickCount
		}
	}
}

// Get 查询详情（无用户上下文，不注入标记）。
func (uc *ToolLinkUsecase) Get(ctx context.Context, req *adminV1.GetToolLinkRequest) (*adminV1.ToolLink, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *ToolLinkUsecase) Create(ctx context.Context, req *adminV1.CreateToolLinkRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}
	if err := validateIconPair(req.Data.GetIconType(), req.Data.IconValue); err != nil {
		return err
	}
	return uc.repo.Create(ctx, req)
}

// validateIconPair 校验图标类型与取值的配对口径：
// IMAGE=本地存储相对路径（形如 bucket/object.ext，必含 '/'）；LUCIDE=内置图标名（kebab-case，不含 '/'）。
// 历史前端缺陷曾把 lucide 图标名以 IMAGE 落库（渲染链路必 404），本校验阻断新增污染；
// iconValue 为 nil（未触及）时跳过，兼容仅改其它字段的存量脏数据编辑。
func validateIconPair(iconType adminV1.ToolLinkIconType, iconValue *string) error {
	if iconValue == nil || *iconValue == "" {
		return nil
	}
	isPath := strings.Contains(*iconValue, "/")
	switch iconType {
	case adminV1.ToolLinkIconType_IMAGE:
		if !isPath {
			return adminV1.ErrorBadRequest("icon_value must be a storage path when icon_type=IMAGE")
		}
	case adminV1.ToolLinkIconType_LUCIDE:
		if isPath {
			return adminV1.ErrorBadRequest("icon_value must be a lucide icon name when icon_type=LUCIDE")
		}
	}
	return nil
}

// Update 更新记录；更新成功后清理被替换掉的自定义图标文件。
func (uc *ToolLinkUsecase) Update(ctx context.Context, req *adminV1.UpdateToolLinkRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	// 先取旧记录，用于更新成功后清理被替换掉的自定义图标文件
	var oldIcon *adminV1.ToolLink
	if id := req.GetId(); id != 0 {
		oldIcon, _ = uc.repo.Get(ctx, &adminV1.GetToolLinkRequest{Id: id})
	}

	// 图标配对校验：icon_type 未触及（nil）时以旧记录类型为准；icon_value 未触及则跳过校验
	if req.Data.IconValue != nil {
		effectiveType := req.Data.GetIconType()
		if req.Data.IconType == nil && oldIcon != nil {
			effectiveType = oldIcon.GetIconType()
		}
		if err := validateIconPair(effectiveType, req.Data.IconValue); err != nil {
			return err
		}
	}

	if err := uc.repo.Update(ctx, req); err != nil {
		return err
	}

	// 仅当本次更新真正触及 icon_value 时才允许清理旧图标：
	// 替换（非 nil）或经 updateMask 显式置空（NilValuePaths 链路 SetNull）。
	// 未触及时 DB 保留原引用（SetNillableIconValue 跳过 nil），误删会使图标 404。
	if oldIcon != nil {
		iconTouched := req.Data.IconValue != nil
		if !iconTouched && req.UpdateMask != nil {
			for _, p := range req.UpdateMask.GetPaths() {
				if p == "icon_value" {
					iconTouched = true
					break
				}
			}
		}
		if iconTouched {
			uc.cleanupOrphanIcon(ctx, oldIcon.GetIconType(), oldIcon.GetIconValue(), req.Data.GetIconValue())
		}
	}
	return nil
}

// Delete 软删除记录；删除成功后清理自定义图标文件。
func (uc *ToolLinkUsecase) Delete(ctx context.Context, req *adminV1.DeleteToolLinkRequest) error {
	oldIcon, _ := uc.repo.Get(ctx, &adminV1.GetToolLinkRequest{Id: req.GetId()})

	if err := uc.repo.Delete(ctx, req); err != nil {
		return err
	}

	if oldIcon != nil {
		uc.cleanupOrphanIcon(ctx, oldIcon.GetIconType(), oldIcon.GetIconValue(), "")
	}
	return nil
}

func (uc *ToolLinkUsecase) Move(ctx context.Context, req *adminV1.MoveToolLinkRequest) error {
	return uc.repo.Move(ctx, req)
}

// cleanupOrphanIcon 删除不再被引用的自定义图标文件（仅 IMAGE 类型且路径发生变化时）。
// 文件删除失败仅记录日志，不影响主流程（残留文件可由人工清理）。
func (uc *ToolLinkUsecase) cleanupOrphanIcon(ctx context.Context, iconType adminV1.ToolLinkIconType, oldPath, newPath string) {
	if uc.store == nil {
		return
	}
	if iconType != adminV1.ToolLinkIconType_IMAGE {
		return
	}
	if oldPath == "" || oldPath == newPath {
		return
	}
	if err := uc.store.Delete(ctx, oldPath); err != nil {
		uc.log.Warnf("delete orphan tool-link icon file [%s] failed: %v", oldPath, err)
	}
}
