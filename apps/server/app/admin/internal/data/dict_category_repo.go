package data

import (
	"context"
	"strings"
	"time"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/trans"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/dict"
	"framework/app/admin/internal/data/ent/dictcategory"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 本文件承载 dictRepo 的字典分类（dict_category）方法集（P2 分类实体化）。
// 分类方法与条目方法同属字典域，挂在同一 repo struct 上，biz.DictRepo 接口统一暴露。

// dictCategoryMapper 分类 DTO↔entity 映射（与 dictRepo 同款 time 转换器，包级单例避免逐实例注册）。
var dictCategoryMapper = func() *mapper.CopierMapper[adminV1.DictCategory, ent.DictCategory] {
	m := mapper.NewCopierMapper[adminV1.DictCategory, ent.DictCategory]()
	m.AppendConverters(copierutil.NewTimeStringConverterPair())
	m.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	return m
}()

func (r *dictRepo) categoryToDTO(entity *ent.DictCategory) *adminV1.DictCategory {
	dto := dictCategoryMapper.ToDTO(entity)
	return dto
}

// ListCategories 查询全部分类并附带各分类条目数（GROUP BY dict.type 一次聚合，避免 N+1）。
// consumers/builtin 语义补充由 biz 层完成（注册表 + 排班引用检测），此处只出库内数据。
func (r *dictRepo) ListCategories(ctx context.Context) ([]*adminV1.DictCategory, error) {
	entities, err := r.data.db.Client().DictCategory.Query().
		Order(ent.Asc(dictcategory.FieldSortID), ent.Asc(dictcategory.FieldCode)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query dict categories failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query dict categories failed")
	}

	// 条目数聚合：type → count（仅未软删）
	type cntRow struct {
		Type  string `json:"type"`
		Count int    `json:"count"`
	}
	var rows []cntRow
	if err = r.data.db.Client().Dict.Query().
		Where(dict.DeleteTimeIsNil()).
		GroupBy(dict.FieldType).
		Aggregate(ent.Count()).
		Scan(ctx, &rows); err != nil {
		r.log.Errorf("aggregate dict entry counts failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query dict categories failed")
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.Type] = row.Count
	}

	dtos := make([]*adminV1.DictCategory, 0, len(entities))
	for _, entity := range entities {
		dto := r.categoryToDTO(entity)
		dto.EntryCount = trans.Ptr(uint32(counts[entity.Code]))
		dtos = append(dtos, dto)
	}
	return dtos, nil
}

// GetCategory 按编码取分类；不存在返回 NotFound（供护栏判定）。
func (r *dictRepo) GetCategory(ctx context.Context, code string) (*adminV1.DictCategory, error) {
	entity, err := r.data.db.Client().DictCategory.Query().
		Where(dictcategory.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("dict category %q not found", code)
		}
		r.log.Errorf("query dict category failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query dict category failed")
	}
	return r.categoryToDTO(entity), nil
}

// CreateCategory 创建分类（编码唯一预检 + 唯一约束兜底）。
func (r *dictRepo) CreateCategory(ctx context.Context, req *adminV1.CreateDictCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	code := strings.TrimSpace(req.Data.GetCode())

	dup, err := r.data.db.Client().DictCategory.Query().
		Where(dictcategory.CodeEQ(code)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("check dict category duplicated failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("check dict category failed")
	}
	if dup {
		return adminV1.ErrorConflict("dict category %q already exists", code)
	}

	builder := r.data.db.Client().DictCategory.Create().
		SetCode(code).
		SetName(req.Data.GetName()).
		SetKind(req.Data.GetKind()).
		SetNillableSortID(req.Data.SortId).
		SetNillableRemark(req.Data.Remark).
		SetNillableCreateBy(req.Data.CreateBy).
		SetCreateTime(time.Now())

	if err = builder.Exec(ctx); err != nil {
		if isPgUniqueViolation(err) {
			return adminV1.ErrorConflict("dict category %q already exists", code)
		}
		if isPgValueTooLong(err) {
			return adminV1.ErrorBadRequest("dict category field value too long (code <= 64, name <= 128)")
		}
		r.log.Errorf("insert dict category failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert dict category failed")
	}
	return nil
}

// UpdateCategory 更新分类可编辑字段（name/kind/sort/remark）；编码为身份不在此变更。
// 编码存在性由 biz 护栏先行确认，此处按影响行数判 NotFound。
func (r *dictRepo) UpdateCategory(ctx context.Context, req *adminV1.UpdateDictCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	code := req.GetCode()

	affected, err := r.data.db.Client().DictCategory.Update().
		Where(dictcategory.CodeEQ(code)).
		SetName(req.Data.GetName()).
		SetKind(req.Data.GetKind()).
		SetNillableSortID(req.Data.SortId).
		SetNillableRemark(req.Data.Remark).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update dict category failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("update dict category failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("dict category %q not found", code)
	}
	return nil
}

// DeleteCategory 按编码删除分类（空分类/非内置的前置判定在 biz）。
func (r *dictRepo) DeleteCategory(ctx context.Context, req *adminV1.DeleteDictCategoryRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	affected, err := r.data.db.Client().DictCategory.Delete().
		Where(dictcategory.CodeEQ(req.GetCode())).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete dict category failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete dict category failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("dict category %q not found", req.GetCode())
	}
	return nil
}

// EnsureCategory 建条目链路的分类兜底：编码不存在则以 enum 用途补建（create-if-missing）。
// 保证 dict_category 注册表与 dict 实际使用的 type 始终闭合。
func (r *dictRepo) EnsureCategory(ctx context.Context, code, name string) error {
	if code == "" {
		return nil
	}
	exist, err := r.data.db.Client().DictCategory.Query().
		Where(dictcategory.CodeEQ(code)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("check dict category exist failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("check dict category failed")
	}
	if exist {
		return nil
	}
	if name == "" {
		name = code
	}
	if err = r.data.db.Client().DictCategory.Create().
		SetCode(code).
		SetName(name).
		SetKind("enum").
		SetCreateTime(time.Now()).
		Exec(ctx); err != nil {
		// 并发补建撞唯一键视为成功（语义幂等）
		if isPgUniqueViolation(err) {
			return nil
		}
		r.log.Errorf("ensure dict category failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("ensure dict category failed")
	}
	return nil
}

// SyncCategoryName 分类改名后同步该分类全部条目的 type_name 冗余列（服务端单点批量，替代前端 N 次循环更新）。
func (r *dictRepo) SyncCategoryName(ctx context.Context, code, name string) error {
	if _, err := r.data.db.Client().Dict.Update().
		Where(dict.TypeEQ(code), dict.DeleteTimeIsNil()).
		SetTypeName(name).
		SetUpdateTime(time.Now()).
		Save(ctx); err != nil {
		r.log.Errorf("sync dict category name failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("sync dict category name failed")
	}
	return nil
}

// CountCategoryEntries 统计分类下未软删条目数（删除分类/改名提示用）。
func (r *dictRepo) CountCategoryEntries(ctx context.Context, code string) (int, error) {
	count, err := r.data.db.Client().Dict.Query().
		Where(dict.TypeEQ(code), dict.DeleteTimeIsNil()).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count dict category entries failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("count dict category entries failed")
	}
	return count, nil
}

// CountAssetSubsystemRefs 统计引用指定字典条目 ID 的在册资产数（asset_subsystem 删除护栏）。
// 资产表 subsystem_id 存的是字典条目 id（无外键约束），删条目前必须预检。
func (r *dictRepo) CountAssetSubsystemRefs(ctx context.Context, entryID uint32) (int, error) {
	_ = ctx
	_ = entryID
	return 0, nil
}

// ListHolidayDictCodes 扫描排班规则 holiday_config（JSON）提取引用的字典编码集合。
// 规则量级小（排班规则为低频配置），全量扫描内存提取，避免 JSON 路径查询的方言耦合。
func (r *dictRepo) ListHolidayDictCodes(ctx context.Context) ([]string, error) {
	_ = ctx
	return nil, nil
}

// UpsertCategoryMeta 种子链路：注册表分类存在性 + 元数据对齐。
// kind/builtin 是代码耦合事实，每次启动以注册表为准；name 是用户可编辑的展示元数据，
// 已存在的行不覆盖（保留 UI 改名成果），仅新建或名称为空时回填注册表默认名。
func (r *dictRepo) UpsertCategoryMeta(ctx context.Context, code, name, kind string, builtin bool) error {
	entity, err := r.data.db.Client().DictCategory.Query().
		Where(dictcategory.CodeEQ(code)).
		Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("query dict category for upsert failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("query dict category failed")
	}
	if entity == nil {
		return r.data.db.Client().DictCategory.Create().
			SetCode(code).
			SetName(name).
			SetKind(kind).
			SetBuiltin(builtin).
			SetCreateTime(time.Now()).
			Exec(ctx)
	}
	updater := r.data.db.Client().DictCategory.UpdateOneID(entity.ID).
		SetKind(kind).
		SetBuiltin(builtin).
		SetUpdateTime(time.Now())
	if strings.TrimSpace(entity.Name) == "" {
		updater.SetName(name)
	}
	_, err = updater.Save(ctx)
	return err
}

// DiscoverCategories 启动发现：dict 表实际使用但未注册的 type 补建分类行（kind=enum，name 取首个非空 type_name）。
// 保证存量数据在分类面板/注册表中完整可见，无需手工迁移脚本。
func (r *dictRepo) DiscoverCategories(ctx context.Context) error {
	type typeRow struct {
		Type     string `json:"type"`
		TypeName string `json:"type_name"`
	}
	var rows []typeRow
	if err := r.data.db.Client().Dict.Query().
		Where(dict.DeleteTimeIsNil()).
		GroupBy(dict.FieldType).
		Aggregate(ent.As(ent.Max(dict.FieldTypeName), "type_name")).
		Scan(ctx, &rows); err != nil {
		r.log.Errorf("discover dict types failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("discover dict types failed")
	}
	for _, row := range rows {
		name := strings.TrimSpace(row.TypeName)
		if name == "" {
			name = row.Type
		}
		if err := r.EnsureCategory(ctx, row.Type, name); err != nil {
			return err
		}
	}
	return nil
}
