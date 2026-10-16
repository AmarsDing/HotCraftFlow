package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/copierutil"
	entgo "github.com/tx7do/go-utils/entgo/query"
	entgoUpdate "github.com/tx7do/go-utils/entgo/update"
	"github.com/tx7do/go-utils/fieldmaskutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/toollink"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	adminV1 "framework/api/gen/go/admin/v1"
)

type ToolLinkRepo struct {
	data *Data
	log  *log.Helper

	mapper           *mapper.CopierMapper[adminV1.ToolLink, ent.ToolLink]
	iconTypeConverter *mapper.EnumTypeConverter[adminV1.ToolLinkIconType, toollink.IconType]
}

func NewToolLinkRepo(data *Data, logger log.Logger) biz.ToolLinkRepo {
	repo := &ToolLinkRepo{
		log:               log.NewHelper(log.With(logger, "module", "tool-link/repo/admin-service")),
		data:              data,
		mapper:            mapper.NewCopierMapper[adminV1.ToolLink, ent.ToolLink](),
		iconTypeConverter: mapper.NewEnumTypeConverter[adminV1.ToolLinkIconType, toollink.IconType](adminV1.ToolLinkIconType_name, adminV1.ToolLinkIconType_value),
	}

	repo.init()

	return repo
}

func (r *ToolLinkRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.iconTypeConverter.NewConverterPair())
}

// dtoOf entity→DTO 转换；copier 按字段名精确匹配，i18n 字段（ent: NameI18n / DTO: NameI18N）需手动补齐。
func (r *ToolLinkRepo) dtoOf(entity *ent.ToolLink) *adminV1.ToolLink {
	dto := r.mapper.ToDTO(entity)
	if dto == nil || entity == nil {
		return dto
	}
	dto.NameI18N = entity.NameI18n
	dto.DescriptionI18N = entity.DescriptionI18n
	return dto
}

func (r *ToolLinkRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().ToolLink.Query().
		Where(toollink.DeleteTimeIsNil())
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *ToolLinkRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().ToolLink.Query().
		Where(toollink.DeleteTimeIsNil())

	// 默认按 sort ASC, id ASC 输出（文档 §3.15）；调用方显式传 orderBy 时从其序
	orderBys := req.GetOrderBy()
	if len(orderBys) == 0 {
		orderBys = []string{toollink.FieldSort, toollink.FieldID}
	}

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		orderBys, toollink.FieldSort,
		req.GetFieldMask().GetPaths(),
	)
	if err != nil {
		r.log.Errorf("parse list param error [%s]", err.Error())
		return nil, adminV1.ErrorBadRequest("invalid query parameter")
	}

	if querySelectors != nil {
		builder.Modify(querySelectors...)
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*adminV1.ToolLink, 0, len(entities))
	for _, entity := range entities {
		dto := r.dtoOf(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListToolLinkResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

func (r *ToolLinkRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().ToolLink.Query().
		Where(toollink.IDEQ(id), toollink.DeleteTimeIsNil()).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *ToolLinkRepo) Get(ctx context.Context, req *adminV1.GetToolLinkRequest) (*adminV1.ToolLink, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().ToolLink.Query().
		Where(toollink.IDEQ(req.GetId()), toollink.DeleteTimeIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("tool link not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.dtoOf(entity), nil
}

// sortValue 读取 Nillable sort 字段，nil 视为 0。
func sortValue(entity *ent.ToolLink) int32 {
	if entity == nil || entity.Sort == nil {
		return 0
	}
	return *entity.Sort
}

// nextSort 取当前最大 sort + 1，新建记录默认排到末尾。
func (r *ToolLinkRepo) nextSort(ctx context.Context) int32 {
	entity, err := r.data.db.Client().ToolLink.Query().
		Where(toollink.DeleteTimeIsNil()).
		Order(ent.Desc(toollink.FieldSort), ent.Desc(toollink.FieldID)).
		First(ctx)
	if err != nil {
		return 0
	}
	return sortValue(entity) + 1
}

func (r *ToolLinkRepo) Create(ctx context.Context, req *adminV1.CreateToolLinkRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	builder := r.data.db.Client().ToolLink.Create().
		SetNillableName(req.Data.Name).
		SetNillableURL(req.Data.Url).
		SetNillableIconType(r.iconTypeConverter.ToEntity(req.Data.IconType)).
		SetNillableIconValue(req.Data.IconValue).
		SetNillableCategory(req.Data.Category).
		SetNillableDescription(req.Data.Description).
		SetNillableExternal(req.Data.External).
		SetNillableStatus((*toollink.Status)(req.Data.Status)).
		SetNillableCategoryID(req.Data.CategoryId).
		SetNillableHealthStatus((*toollink.HealthStatus)(req.Data.HealthStatus)).
		SetNillableLinkType((*toollink.LinkType)(req.Data.LinkType)).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.VisibleRoles != nil {
		builder.SetVisibleRoles(req.Data.VisibleRoles)
	}
	if req.Data.NameI18N != nil {
		builder.SetNameI18n(req.Data.NameI18N)
	}
	if req.Data.DescriptionI18N != nil {
		builder.SetDescriptionI18n(req.Data.DescriptionI18N)
	}

	if req.Data.Sort != nil {
		builder.SetNillableSort(req.Data.Sort)
	} else {
		builder.SetSort(r.nextSort(ctx))
	}

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *ToolLinkRepo) Update(ctx context.Context, req *adminV1.UpdateToolLinkRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	// 优先取顶层 id（来自 URL 路径），回退 data.id。
	// 必须在 fieldmaskutil.Filter 之前捕获：Filter 会清空不在 updateMask 中的字段（含 data.id）。
	id := req.GetId()
	if id == 0 {
		id = req.Data.GetId()
	}

	// 如果不存在则创建
	if req.GetAllowMissing() {
		exist, err := r.IsExist(ctx, id)
		if err != nil {
			return err
		}
		if !exist {
			createReq := &adminV1.CreateToolLinkRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			return r.Create(ctx, createReq)
		}
	}

	if req.UpdateMask != nil {
		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return adminV1.ErrorBadRequest("invalid field mask")
		}
		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().ToolLink.UpdateOneID(id).
		SetNillableName(req.Data.Name).
		SetNillableURL(req.Data.Url).
		SetNillableIconType(r.iconTypeConverter.ToEntity(req.Data.IconType)).
		SetNillableIconValue(req.Data.IconValue).
		SetNillableCategory(req.Data.Category).
		SetNillableDescription(req.Data.Description).
		SetNillableSort(req.Data.Sort).
		SetNillableExternal(req.Data.External).
		SetNillableStatus((*toollink.Status)(req.Data.Status)).
		SetNillableCategoryID(req.Data.CategoryId).
		SetNillableLinkType((*toollink.LinkType)(req.Data.LinkType)).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	// JSON 字段仅在显式传入（fieldmask 覆盖）时写入；nil 表示不更新。
	if req.Data.VisibleRoles != nil {
		builder.SetVisibleRoles(req.Data.VisibleRoles)
	}
	if req.Data.NameI18N != nil {
		builder.SetNameI18n(req.Data.NameI18N)
	}
	if req.Data.DescriptionI18N != nil {
		builder.SetDescriptionI18n(req.Data.DescriptionI18N)
	}

	if req.Data.UpdateTime == nil {
		builder.SetUpdateTime(time.Now())
	}

	if req.UpdateMask != nil {
		nilPaths := fieldmaskutil.NilValuePaths(req.Data, req.GetUpdateMask().GetPaths())
		nilUpdater := entgoUpdate.BuildSetNullUpdater(nilPaths)
		if nilUpdater != nil {
			builder.Modify(nilUpdater)
		}
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("update one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

// Delete 软删除（置 delete_time），图标文件清理由 service 层在删除成功后处理。
func (r *ToolLinkRepo) Delete(ctx context.Context, req *adminV1.DeleteToolLinkRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	affected, err := r.data.db.Client().ToolLink.Update().
		Where(toollink.IDEQ(req.GetId()), toollink.DeleteTimeIsNil()).
		SetDeleteTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("delete one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("tool link not found")
	}

	return nil
}

// Move 调整排序：与相邻记录交换 sort 值；sort 相等时用列表位置值区分，保证排序稳定推进。
func (r *ToolLinkRepo) Move(ctx context.Context, req *adminV1.MoveToolLinkRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	direction := req.GetDirection()
	if direction != adminV1.ToolLinkMoveDirection_UP && direction != adminV1.ToolLinkMoveDirection_DOWN {
		return adminV1.ErrorBadRequest("invalid direction")
	}

	entities, err := r.data.db.Client().ToolLink.Query().
		Where(toollink.DeleteTimeIsNil()).
		Order(ent.Asc(toollink.FieldSort), ent.Asc(toollink.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query list for move failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("query list failed")
	}

	idx := -1
	for i, entity := range entities {
		if entity.ID == req.GetId() {
			idx = i
			break
		}
	}
	if idx < 0 {
		return adminV1.ErrorNotFound("tool link not found")
	}

	neighborIdx := idx - 1
	if direction == adminV1.ToolLinkMoveDirection_DOWN {
		neighborIdx = idx + 1
	}
	if neighborIdx < 0 || neighborIdx >= len(entities) {
		// 已在边界，幂等成功
		return nil
	}

	target := entities[idx]
	neighbor := entities[neighborIdx]

	targetSort := sortValue(target)
	neighborSort := sortValue(neighbor)

	newTargetSort, newNeighborSort := neighborSort, targetSort
	if targetSort == neighborSort {
		// sort 相等时交换不会改变相对顺序，改用列表位置值强制区分
		newTargetSort, newNeighborSort = int32(neighborIdx), int32(idx)
	}

	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("move failed")
	}

	if err := tx.ToolLink.UpdateOneID(target.ID).SetSort(newTargetSort).Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("update target sort failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("move failed")
	}
	if err := tx.ToolLink.UpdateOneID(neighbor.ID).SetSort(newNeighborSort).Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("update neighbor sort failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("move failed")
	}

	if err := tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("move failed")
	}

	return nil
}

// Reorder 批量重排：按 ids 下标重写 sort（0..n-1），未出现在 ids 中的记录保持原 sort。
func (r *ToolLinkRepo) Reorder(ctx context.Context, ids []uint32) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("reorder failed")
	}

	for i, id := range ids {
		if err := tx.ToolLink.Update().
			Where(toollink.IDEQ(id), toollink.DeleteTimeIsNil()).
			SetSort(int32(i)).
			Exec(ctx); err != nil {
			_ = tx.Rollback()
			r.log.Errorf("update sort failed (id=%d): %s", id, err.Error())
			return adminV1.ErrorInternalServerError("reorder failed")
		}
	}

	if err := tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("reorder failed")
	}

	return nil
}

// IncrementClick 累计点击数 +1。
func (r *ToolLinkRepo) IncrementClick(ctx context.Context, id uint32) error {
	affected, err := r.data.db.Client().ToolLink.Update().
		Where(toollink.IDEQ(id), toollink.DeleteTimeIsNil()).
		AddClickCount(1).
		Save(ctx)
	if err != nil {
		r.log.Errorf("increment click count failed (id=%d): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("record click failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("tool link not found")
	}
	return nil
}

// UpdateHealth 写回健康检测结果。
func (r *ToolLinkRepo) UpdateHealth(ctx context.Context, id uint32, status string, checkedAt time.Time) error {
	affected, err := r.data.db.Client().ToolLink.Update().
		Where(toollink.IDEQ(id), toollink.DeleteTimeIsNil()).
		SetHealthStatus(toollink.HealthStatus(status)).
		SetLastCheckAt(checkedAt).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update health status failed (id=%d): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("update health status failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("tool link not found")
	}
	return nil
}

// UpdateCategoryName 级联更新引用指定分类的工具链接冗余分类名。
func (r *ToolLinkRepo) UpdateCategoryName(ctx context.Context, categoryID uint32, name string) error {
	_, err := r.data.db.Client().ToolLink.Update().
		Where(toollink.CategoryIDEQ(categoryID), toollink.DeleteTimeIsNil()).
		SetCategory(name).
		Save(ctx)
	if err != nil {
		r.log.Errorf("cascade update category name failed (category_id=%d): %s", categoryID, err.Error())
		return adminV1.ErrorInternalServerError("cascade update category name failed")
	}
	return nil
}

// ClearCategoryID 解除引用指定分类的工具链接关联（category_id 与冗余分类名一并清空，归为未分类）。
func (r *ToolLinkRepo) ClearCategoryID(ctx context.Context, categoryID uint32) error {
	// category_id 与冗余 category 名一并清空，删除分类后链接归为未分类
	_, err := r.data.db.Client().ToolLink.Update().
		Where(toollink.CategoryIDEQ(categoryID), toollink.DeleteTimeIsNil()).
		ClearCategoryID().
		ClearCategory().
		Save(ctx)
	if err != nil {
		r.log.Errorf("clear category_id failed (category_id=%d): %s", categoryID, err.Error())
		return adminV1.ErrorInternalServerError("clear category_id failed")
	}
	return nil
}

// ListAll 查询全部未删除记录（导出用），按 sort ASC, id ASC 输出。
func (r *ToolLinkRepo) ListAll(ctx context.Context) ([]*adminV1.ToolLink, error) {
	entities, err := r.data.db.Client().ToolLink.Query().
		Where(toollink.DeleteTimeIsNil()).
		Order(ent.Asc(toollink.FieldSort), ent.Asc(toollink.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query all failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*adminV1.ToolLink, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.dtoOf(entity))
	}
	return dtos, nil
}

// FilterExistingURLs 返回给定 URL 集合中已存在（未删除）的 URL 子集（导入去重用）。
func (r *ToolLinkRepo) FilterExistingURLs(ctx context.Context, urls []string) (map[string]bool, error) {
	existing := make(map[string]bool, len(urls))
	if len(urls) == 0 {
		return existing, nil
	}

	entities, err := r.data.db.Client().ToolLink.Query().
		Select(toollink.FieldURL).
		Where(toollink.URLIn(urls...), toollink.DeleteTimeIsNil()).
		All(ctx)
	if err != nil {
		r.log.Errorf("query existing urls failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query existing urls failed")
	}

	for _, entity := range entities {
		if entity.URL != nil {
			existing[*entity.URL] = true
		}
	}
	return existing, nil
}
