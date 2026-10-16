package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
)

// ToolLinkCategoryRepo 工具链接分类仓储接口（由 data 层 toolLinkCategoryRepo 实现）。
type ToolLinkCategoryRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkCategoryResponse, error)
	Get(ctx context.Context, req *adminV1.GetToolLinkCategoryRequest) (*adminV1.ToolLinkCategory, error)
	Create(ctx context.Context, req *adminV1.CreateToolLinkCategoryRequest) error
	Update(ctx context.Context, req *adminV1.UpdateToolLinkCategoryRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteToolLinkCategoryRequest) error
	// CountAll 统计分类总数（启动播种判空用）。
	CountAll(ctx context.Context) (int, error)
}

// 默认分类种子（表为空时构造期播种，与 NewUserUsecase 播种超管同模式）。
// ptrString/ptrInt32 复用 biz 包内已有声明（document.go/wiring.go）。
var defaultToolLinkCategories = []*adminV1.ToolLinkCategory{
	{Name: ptrString("监控"), Icon: ptrString("monitor"), Sort: ptrInt32(1)},
	{Name: ptrString("日志"), Icon: ptrString("file-text"), Sort: ptrInt32(2)},
	{Name: ptrString("文档"), Icon: ptrString("book-open"), Sort: ptrInt32(3)},
}

// ToolLinkCategoryUsecase 工具链接分类业务逻辑。
// 分类改名时级联同步工具链接上的冗余分类名（category 自由文本列）。
type ToolLinkCategoryUsecase struct {
	repo     ToolLinkCategoryRepo
	linkRepo ToolLinkRepo
	log      *log.Helper
}

func NewToolLinkCategoryUsecase(repo ToolLinkCategoryRepo, linkRepo ToolLinkRepo, logger log.Logger) *ToolLinkCategoryUsecase {
	uc := &ToolLinkCategoryUsecase{
		repo:     repo,
		linkRepo: linkRepo,
		log:      log.NewHelper(log.With(logger, "module", "tool-link-category/usecase/admin-service")),
	}
	uc.seedDefaults()
	return uc
}

// seedDefaults 表为空时写入默认分类；失败仅记日志不影响启动。
func (uc *ToolLinkCategoryUsecase) seedDefaults() {
	ctx := context.Background()
	count, err := uc.repo.CountAll(ctx)
	if err != nil {
		uc.log.Warnf("count tool link categories failed, skip seed: %v", err)
		return
	}
	if count > 0 {
		return
	}
	for _, c := range defaultToolLinkCategories {
		c.Status = ptrString("ON")
		if err = uc.repo.Create(ctx, &adminV1.CreateToolLinkCategoryRequest{Data: c}); err != nil {
			uc.log.Warnf("seed default tool link category [%s] failed: %v", c.GetName(), err)
		}
	}
	uc.log.Info("default tool link categories seeded")
}

func (uc *ToolLinkCategoryUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkCategoryResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *ToolLinkCategoryUsecase) Get(ctx context.Context, req *adminV1.GetToolLinkCategoryRequest) (*adminV1.ToolLinkCategory, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *ToolLinkCategoryUsecase) Create(ctx context.Context, req *adminV1.CreateToolLinkCategoryRequest) error {
	if req == nil || req.Data == nil || req.Data.GetName() == "" {
		return adminV1.ErrorBadRequest("category name required")
	}
	return uc.repo.Create(ctx, req)
}

// Update 更新分类；名称变化时级联同步引用该分类的工具链接冗余名称。
func (uc *ToolLinkCategoryUsecase) Update(ctx context.Context, req *adminV1.UpdateToolLinkCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	var oldName string
	if id := req.GetId(); id != 0 {
		if old, err := uc.repo.Get(ctx, &adminV1.GetToolLinkCategoryRequest{Id: id}); err == nil {
			oldName = old.GetName()
		}
	}

	if err := uc.repo.Update(ctx, req); err != nil {
		return err
	}

	newName := req.Data.GetName()
	if newName != "" && newName != oldName && req.GetId() != 0 {
		if err := uc.linkRepo.UpdateCategoryName(ctx, req.GetId(), newName); err != nil {
			uc.log.Warnf("cascade update tool link category name failed (category_id=%d): %v", req.GetId(), err)
		}
	}
	return nil
}

// Delete 删除分类；引用该分类的工具链接解除 category_id 关联（保留冗余名称作自由文本）。
func (uc *ToolLinkCategoryUsecase) Delete(ctx context.Context, req *adminV1.DeleteToolLinkCategoryRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}
	if err := uc.linkRepo.ClearCategoryID(ctx, req.GetId()); err != nil {
		uc.log.Warnf("clear tool link category_id failed (category_id=%d): %v", req.GetId(), err)
	}
	return uc.repo.Delete(ctx, req)
}
