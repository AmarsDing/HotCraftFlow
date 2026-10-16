package biz

import (
	"context"
	"strings"

	"github.com/tx7do/go-utils/trans"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 本文件承载 DictUsecase 的字典分类（dict_category）业务方法（P2 分类实体化）。
// 分类元数据权威 = dict_category 表（名称/排序/备注）+ dictCategoryRegistry 注册表
// （用途/内置/消费方/值校验，代码耦合事实）；护栏规则集中在 Usecase，data 层只做存取。

// dictCategoryAssetSubsystem 资产子系统分类编码（条目删除护栏：被 monitor_assets.subsystem_id 按 ID 引用时禁删）。
const dictCategoryAssetSubsystem = "asset_subsystem"

// ListCategories 查询分类列表并合并注册表元数据与动态引用检测。
func (uc *DictUsecase) ListCategories(ctx context.Context) (*adminV1.ListDictCategoryResponse, error) {
	items, err := uc.repo.ListCategories(ctx)
	if err != nil {
		return nil, err
	}

	// 动态消费方：排班规则 holiday_config 引用的字典编码（DB 事实，注册表无法静态声明）。
	// 检测失败降级为无动态消费方，不阻断分类列表主流程。
	holidaySet := map[string]struct{}{}
	if codes, derr := uc.repo.ListHolidayDictCodes(ctx); derr == nil {
		for _, code := range codes {
			holidaySet[code] = struct{}{}
		}
	} else {
		uc.log.Warnf("detect holiday dict codes degraded: %s", derr.Error())
	}

	for _, item := range items {
		code := item.GetCode()
		consumers := make([]string, 0, 2)
		if meta, ok := lookupDictCategoryMeta(code); ok {
			// 注册表语义权威：kind/builtin 以代码声明为准（库内值为种子快照）
			item.Kind = trans.Ptr(meta.Kind)
			item.Builtin = trans.Ptr(meta.Builtin)
			consumers = append(consumers, meta.Consumers...)
		}
		if _, ok := holidaySet[code]; ok {
			consumers = append(consumers, "排班规则 · 节假日配置（holiday_config 动态引用）")
		}
		item.Consumers = consumers
	}

	return &adminV1.ListDictCategoryResponse{
		Items: items,
		Total: uint32(len(items)),
	}, nil
}

// CreateCategory 手工创建分类（仅非内置编码；内置分类由种子自动注册，拒绝手工撞码）。
func (uc *DictUsecase) CreateCategory(ctx context.Context, req *adminV1.CreateDictCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	code := strings.TrimSpace(req.Data.GetCode())
	if code == "" {
		return adminV1.ErrorBadRequest("分类编码不能为空")
	}
	kind := req.Data.GetKind()
	if kind == "" {
		kind = DictCategoryKindEnum
	}
	if kind != DictCategoryKindEnum && kind != DictCategoryKindParam {
		return adminV1.ErrorBadRequest("分类用途仅允许 enum（枚举选项）/ param（系统参数）")
	}
	if _, ok := lookupDictCategoryMeta(code); ok {
		return adminV1.ErrorConflict("分类 %q 为系统内置分类，由服务端自动注册，无需手工创建", code)
	}
	req.Data.Code = trans.Ptr(code)
	req.Data.Kind = trans.Ptr(kind)
	return uc.repo.CreateCategory(ctx, req)
}

// UpdateCategory 更新分类可编辑字段（name/sort/remark；非内置还可改 kind）。
// 编码为身份不可改；内置分类用途锁定（防"参数被改当枚举"事故）；改名联动同步条目 type_name。
func (uc *DictUsecase) UpdateCategory(ctx context.Context, req *adminV1.UpdateDictCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	code := req.GetCode()
	existing, err := uc.repo.GetCategory(ctx, code)
	if err != nil {
		return err
	}

	if existing.GetBuiltin() {
		// 内置分类：kind 为代码耦合事实，锁定为库内值（提交值一律忽略）
		req.Data.Kind = trans.Ptr(existing.GetKind())
	} else {
		// 部分更新兼容：未提交 kind 时继承库内值（kind 非指针，缺省 "" 不可直接入库校验）
		if req.Data.GetKind() == "" {
			req.Data.Kind = trans.Ptr(existing.GetKind())
		}
		if kind := req.Data.GetKind(); kind != DictCategoryKindEnum && kind != DictCategoryKindParam {
			return adminV1.ErrorBadRequest("分类用途仅允许 enum（枚举选项）/ param（系统参数）")
		}
	}

	// 名称为空提交时保留库内名称（防部分更新/误清空把分类名抹掉；改名同步以有效名称为准）
	if strings.TrimSpace(req.Data.GetName()) == "" {
		req.Data.Name = trans.Ptr(existing.GetName())
	}

	if err = uc.repo.UpdateCategory(ctx, req); err != nil {
		return err
	}

	// 分类改名 → 单点同步该分类全部条目的 type_name 冗余列（替代前端 N 次循环更新）
	if newName := strings.TrimSpace(req.Data.GetName()); newName != "" && newName != existing.GetName() {
		if err = uc.repo.SyncCategoryName(ctx, code, newName); err != nil {
			return err
		}
	}
	return nil
}

// DeleteCategory 删除分类：仅允许"非内置 + 未被代码引用 + 无条目"的分类。
func (uc *DictUsecase) DeleteCategory(ctx context.Context, req *adminV1.DeleteDictCategoryRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	code := req.GetCode()

	existing, err := uc.repo.GetCategory(ctx, code)
	if err != nil {
		return err
	}
	if existing.GetBuiltin() {
		return adminV1.ErrorConflict("系统内置分类 %q 不可删除", code)
	}
	if _, ok := lookupDictCategoryMeta(code); ok {
		return adminV1.ErrorConflict("分类 %q 被业务代码引用，不可删除", code)
	}
	count, err := uc.repo.CountCategoryEntries(ctx, code)
	if err != nil {
		return err
	}
	if count > 0 {
		return adminV1.ErrorConflict("分类 %q 下仍有 %d 个字典条目，请先删除全部条目", code, count)
	}
	return uc.repo.DeleteCategory(ctx, req)
}

// SeedDictCategories 启动种子：注册表内置分类 upsert（name/kind/builtin 对齐）+ 存量 dict.type 发现补登。
// 幂等，每次启动执行；保证分类面板 = 注册表 ∪ 实际使用，无需手工迁移脚本。
func (uc *DictUsecase) SeedDictCategories(ctx context.Context) error {
	for _, meta := range dictCategoryRegistry {
		if err := uc.repo.UpsertCategoryMeta(ctx, meta.Code, meta.Name, meta.Kind, meta.Builtin); err != nil {
			return err
		}
	}
	return uc.repo.DiscoverCategories(ctx)
}

// validateCategoryValue 按注册表对内置参数类分类做值校验；未登记或无校验器时放行。
func validateCategoryValue(category, itemKey, value string) error {
	meta, ok := lookupDictCategoryMeta(category)
	if !ok || meta.Validate == nil {
		return nil
	}
	return meta.Validate(itemKey, value)
}
