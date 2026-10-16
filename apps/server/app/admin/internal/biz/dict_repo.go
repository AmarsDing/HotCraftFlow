package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
)

// DictRepo 是字典仓储端口，由 data 层未导出的 dictRepo 实现。
// 接口只暴露 Usecase 实际需要的方法；Count 等 data 内部辅助方法不进 interface。
// 分类（dict_category）方法实现见 data/dict_category_repo.go，与条目方法同属字典域。
type DictRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDictResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *adminV1.GetDictRequest) (*adminV1.Dict, error)
	Create(ctx context.Context, req *adminV1.CreateDictRequest) error
	Update(ctx context.Context, req *adminV1.UpdateDictRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteDictRequest) error

	// 字典分类（P2 分类实体化）
	ListCategories(ctx context.Context) ([]*adminV1.DictCategory, error)
	GetCategory(ctx context.Context, code string) (*adminV1.DictCategory, error)
	CreateCategory(ctx context.Context, req *adminV1.CreateDictCategoryRequest) error
	UpdateCategory(ctx context.Context, req *adminV1.UpdateDictCategoryRequest) error
	DeleteCategory(ctx context.Context, req *adminV1.DeleteDictCategoryRequest) error
	// EnsureCategory 建条目链路兜底：分类不存在则以 enum 用途补建（幂等）
	EnsureCategory(ctx context.Context, code, name string) error
	// SyncCategoryName 分类改名后同步该分类全部条目的 type_name 冗余列
	SyncCategoryName(ctx context.Context, code, name string) error
	CountCategoryEntries(ctx context.Context, code string) (int, error)
	// CountAssetSubsystemRefs 统计引用指定字典条目 ID 的在册资产数（asset_subsystem 删除护栏）
	CountAssetSubsystemRefs(ctx context.Context, entryID uint32) (int, error)
	// ListHolidayDictCodes 排班规则 holiday_config 动态引用的字典编码集合
	ListHolidayDictCodes(ctx context.Context) ([]string, error)
	// UpsertCategoryMeta 种子链路：注册表元数据 upsert（name/kind/builtin 以注册表为准）
	UpsertCategoryMeta(ctx context.Context, code, name, kind string, builtin bool) error
	// DiscoverCategories 启动发现：dict 实际使用但未注册的 type 补建分类行
	DiscoverCategories(ctx context.Context) error
}

// dictTypeWarehouseEnvThresholds 库房环境阈值全局字典类型（与 data 层读取约定一致）。
// 该类型字典变更后需失效全部库房的环境阈值合并缓存（wh:env:thresholds:*）。
const dictTypeWarehouseEnvThresholds = "warehouse.env_thresholds"

// DictUsecase 字典业务用例。
// 业务规则（如 key 唯一性校验、跨 repo 编排）在此承载；当前 dict 为简单 CRUD，
// 仅做参数校验后透传 Repo。service 层负责 HTTP 入口鉴权与 operator 注入。
type DictUsecase struct {
	repo DictRepo
	log   *log.Helper
}

// NewDictUsecase 创建字典 Usecase。
// repo 由 wire 注入：data.NewDictRepo 返回 biz.DictRepo interface，wire 据返回类型自动绑定。
// envUC 为 biz 包内具体类型依赖，wire 直接注入；二者无依赖环（envUC 不依赖 DictUsecase）。
func NewDictUsecase(repo DictRepo, logger log.Logger) *DictUsecase {
	return &DictUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "dict/usecase/admin-service")),
	}
}

// List 分页查询字典列表
func (uc *DictUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDictResponse, error) {
	return uc.repo.List(ctx, req)
}

// Get 查询单个字典
func (uc *DictUsecase) Get(ctx context.Context, req *adminV1.GetDictRequest) (*adminV1.Dict, error) {
	return uc.repo.Get(ctx, req)
}

// Create 创建字典
func (uc *DictUsecase) Create(ctx context.Context, req *adminV1.CreateDictRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	category := strings.TrimSpace(req.Data.GetCategory())
	if category == "" {
		return adminV1.ErrorBadRequest("字典分类不能为空")
	}
	// P1 值校验：内置参数类分类的值必须满足代码读取约定
	if err := validateCategoryValue(category, req.Data.GetKey(), req.Data.GetValue()); err != nil {
		return err
	}
	// P2 分类注册表兜底：先保证分类行存在（手工新分类登记为 enum），再落条目，
	// 保持"条目必有所属分类"闭环；顺序倒置会让 EnsureCategory 失败时条目已落库而报错误导重试撞唯一键
	if err := uc.repo.EnsureCategory(ctx, category, req.Data.GetCategoryDesc()); err != nil {
		return err
	}
	if err := uc.repo.Create(ctx, req); err != nil {
		return err
	}
	uc.invalidateEnvThresholdCaches(ctx, category)
	return nil
}

// Update 更新字典
func (uc *DictUsecase) Update(ctx context.Context, req *adminV1.UpdateDictRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	id := dictIDOf(req.GetId(), req.Data.GetId())

	// 读现有记录：内置参数身份锁定（分类/键/状态禁改）与有效分类/键推导。
	// 仅 NotFound 容忍（主流程由 repo.Update 统一报），其余读取错误直接返回，
	// 防止 DB 抖动时身份锁定被绕过（内置参数分类/键被改写）。
	var existing *adminV1.Dict
	if id != 0 {
		var gerr error
		existing, gerr = uc.repo.Get(ctx, &adminV1.GetDictRequest{Id: id})
		if gerr != nil {
			if !adminV1.IsNotFound(gerr) {
				return gerr
			}
			existing = nil
		}
	}

	// 更新掩码可能不含 category：取现有记录分类判定类型（字典低频操作，多一次查询可接受）
	category := req.Data.GetCategory()
	if category == "" && existing != nil {
		category = existing.GetCategory()
	}

	// P1 身份锁定：内置参数类条目的分类/键被代码按字面值读取，变更即运行期故障
	if existing != nil {
		if meta, ok := lookupDictCategoryMeta(existing.GetCategory()); ok && meta.Builtin && meta.Kind == DictCategoryKindParam {
			if req.Data.Category != nil && req.Data.GetCategory() != existing.GetCategory() {
				return adminV1.ErrorConflict("系统内置参数 %q 的分类不可变更", existing.GetKey())
			}
			if req.Data.Key != nil && req.Data.GetKey() != existing.GetKey() {
				return adminV1.ErrorConflict("系统内置参数 %q 的键不可变更", existing.GetKey())
			}
			// 状态锁定：内置参数的生效通道是"值"（如 enabled=true/false），读取方不过滤 status 列，
			// UI 停用会造成"已停用"假象而运行期仍在生效——只允许改值，不允许启停
			if req.Data.Status != nil && req.Data.GetStatus() != existing.GetStatus() {
				return adminV1.ErrorConflict("系统内置参数 %q 不支持启停操作，请通过修改值控制生效（如 enabled=true/false）", existing.GetKey())
			}
		}
	}

	// P1 值校验：值被提交时按有效分类/键校验
	if req.Data.Value != nil {
		key := req.Data.GetKey()
		if key == "" && existing != nil {
			key = existing.GetKey()
		}
		if err := validateCategoryValue(category, key, req.Data.GetValue()); err != nil {
			return err
		}
	}

	if err := uc.repo.Update(ctx, req); err != nil {
		return err
	}
	uc.invalidateEnvThresholdCaches(ctx, category)
	return nil
}

// Delete 删除字典
func (uc *DictUsecase) Delete(ctx context.Context, req *adminV1.DeleteDictRequest) error {
	existing, err := uc.repo.Get(ctx, &adminV1.GetDictRequest{Id: req.GetId()})
	if err != nil {
		return err
	}
	category := existing.GetCategory()

	// P1 删除护栏：内置参数条目被代码读取，删除即运行期故障（允许改值）
	if meta, ok := lookupDictCategoryMeta(category); ok && meta.Builtin && meta.Kind == DictCategoryKindParam {
		return adminV1.ErrorConflict("系统内置参数 %q 被业务代码读取，不可删除（允许修改值）", existing.GetKey())
	}
	// P1 删除护栏：asset_subsystem 条目被在册资产按 ID 引用
	if category == dictCategoryAssetSubsystem {
		refs, err := uc.repo.CountAssetSubsystemRefs(ctx, req.GetId())
		if err != nil {
			return err
		}
		if refs > 0 {
			return adminV1.ErrorConflict("该子系统仍被 %d 个在册资产引用，不可删除", refs)
		}
	}

	if err := uc.repo.Delete(ctx, req); err != nil {
		return err
	}
	uc.invalidateEnvThresholdCaches(ctx, category)
	return nil
}

// dictIDOf 取首个非零 id（路径参数优先，其次 body）。
func dictIDOf(ids ...uint32) uint32 {
	for _, id := range ids {
		if id != 0 {
			return id
		}
	}
	return 0
}

// invalidateEnvThresholdCaches 命中间值字典类型时失效全部库房环境阈值缓存。
func (uc *DictUsecase) invalidateEnvThresholdCaches(ctx context.Context, category string) {
	// skeleton: warehouse env threshold cache removed
	_ = ctx
	_ = category
}
