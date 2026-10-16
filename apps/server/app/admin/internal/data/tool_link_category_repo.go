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
	"framework/app/admin/internal/data/ent/toollinkcategory"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	adminV1 "framework/api/gen/go/admin/v1"
)

type ToolLinkCategoryRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[adminV1.ToolLinkCategory, ent.ToolLinkCategory]
}

func NewToolLinkCategoryRepo(data *Data, logger log.Logger) biz.ToolLinkCategoryRepo {
	repo := &ToolLinkCategoryRepo{
		log:    log.NewHelper(log.With(logger, "module", "tool-link-category/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[adminV1.ToolLinkCategory, ent.ToolLinkCategory](),
	}

	repo.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	repo.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())

	return repo
}

func (r *ToolLinkCategoryRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().ToolLinkCategory.Query().
		Where(toollinkcategory.DeleteTimeIsNil())
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

// CountAll 统计分类总数（启动播种判空用）。
func (r *ToolLinkCategoryRepo) CountAll(ctx context.Context) (int, error) {
	return r.Count(ctx, nil)
}

func (r *ToolLinkCategoryRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkCategoryResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().ToolLinkCategory.Query().
		Where(toollinkcategory.DeleteTimeIsNil())

	orderBys := req.GetOrderBy()
	if len(orderBys) == 0 {
		orderBys = []string{toollinkcategory.FieldSort, toollinkcategory.FieldID}
	}

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		orderBys, toollinkcategory.FieldSort,
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

	dtos := make([]*adminV1.ToolLinkCategory, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListToolLinkCategoryResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

func (r *ToolLinkCategoryRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().ToolLinkCategory.Query().
		Where(toollinkcategory.IDEQ(id), toollinkcategory.DeleteTimeIsNil()).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *ToolLinkCategoryRepo) Get(ctx context.Context, req *adminV1.GetToolLinkCategoryRequest) (*adminV1.ToolLinkCategory, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().ToolLinkCategory.Query().
		Where(toollinkcategory.IDEQ(req.GetId()), toollinkcategory.DeleteTimeIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("tool link category not found")
		}
		r.log.Errorf("query one data failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *ToolLinkCategoryRepo) Create(ctx context.Context, req *adminV1.CreateToolLinkCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	builder := r.data.db.Client().ToolLinkCategory.Create().
		SetNillableName(req.Data.Name).
		SetNillableIcon(req.Data.Icon).
		SetNillableDescription(req.Data.Description).
		SetNillableSort(req.Data.Sort).
		SetNillableStatus((*toollinkcategory.Status)(req.Data.Status)).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if err := builder.Exec(ctx); err != nil {
		// 唯一约束兜底（并发竞态/预检漏网）：locale 无关的 SQLSTATE 23505 识别
		if isPgUniqueViolation(err) {
			return adminV1.ErrorConflict("category name %q already exists", req.Data.GetName())
		}
		r.log.Errorf("insert one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *ToolLinkCategoryRepo) Update(ctx context.Context, req *adminV1.UpdateToolLinkCategoryRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	id := req.GetId()
	if id == 0 {
		id = req.Data.GetId()
	}

	if req.GetAllowMissing() {
		exist, err := r.IsExist(ctx, id)
		if err != nil {
			return err
		}
		if !exist {
			createReq := &adminV1.CreateToolLinkCategoryRequest{Data: req.Data}
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

	builder := r.data.db.Client().ToolLinkCategory.UpdateOneID(id).
		SetNillableName(req.Data.Name).
		SetNillableIcon(req.Data.Icon).
		SetNillableDescription(req.Data.Description).
		SetNillableSort(req.Data.Sort).
		SetNillableStatus((*toollinkcategory.Status)(req.Data.Status)).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

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

// Delete 硬删除（name 为唯一约束，软删会占用名称导致同名重建失败）。
func (r *ToolLinkCategoryRepo) Delete(ctx context.Context, req *adminV1.DeleteToolLinkCategoryRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	// biz 层删除前已将关联工具链接 category_id 置空，无悬挂引用。
	affected, err := r.data.db.Client().ToolLinkCategory.Delete().
		Where(toollinkcategory.IDEQ(req.GetId()), toollinkcategory.DeleteTimeIsNil()).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	if affected == 0 {
		return adminV1.ErrorNotFound("tool link category not found")
	}

	return nil
}
