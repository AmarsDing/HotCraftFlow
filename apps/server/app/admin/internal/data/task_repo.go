package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/entgo/query"
	entgoUpdate "github.com/tx7do/go-utils/entgo/update"
	"github.com/tx7do/go-utils/fieldmaskutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/task"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 编译期接口检查：确保 taskRepo 完整实现 biz.TaskRepo 端口。
var _ biz.TaskRepo = (*taskRepo)(nil)

// taskRepo 未导出，实现 biz.TaskRepo 接口（端口-适配器模式）。
// service/biz 层只依赖 biz.TaskRepo interface，不感知此 struct。
type taskRepo struct {
	data *Data
	log  *log.Helper

	mapper        *mapper.CopierMapper[adminV1.Task, ent.Task]
	typeConverter *mapper.EnumTypeConverter[adminV1.TaskType, task.Type]
}

// NewTaskRepo 返回 biz.TaskRepo 接口，wire 据返回类型自动绑定到 Usecase 入参。
func NewTaskRepo(data *Data, logger log.Logger) biz.TaskRepo {
	repo := &taskRepo{
		log:           log.NewHelper(log.With(logger, "module", "task/repo/admin-service")),
		data:          data,
		mapper:        mapper.NewCopierMapper[adminV1.Task, ent.Task](),
		typeConverter: mapper.NewEnumTypeConverter[adminV1.TaskType, task.Type](adminV1.TaskType_name, adminV1.TaskType_value),
	}

	repo.init()

	return repo
}

func (r *taskRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.typeConverter.NewConverterPair())
}

func (r *taskRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().Task.Query()
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

func (r *taskRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListTaskResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Task.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), task.FieldCreateTime,
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

	dtos := make([]*adminV1.Task, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListTaskResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

func (r *taskRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().Task.Query().
		Where(task.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *taskRepo) Get(ctx context.Context, id uint32) (*adminV1.Task, error) {
	if id == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Task.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("task not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *taskRepo) GetByTypeName(ctx context.Context, typeName string) (*adminV1.Task, error) {
	if typeName == "" {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Task.Query().
		Where(task.TypeNameEQ(typeName)).
		First(ctx)
	if err != nil {
		r.log.Errorf("query one data failed: %s", err.Error())

		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("task not found")
		}

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *taskRepo) Create(ctx context.Context, req *adminV1.CreateTaskRequest) (*adminV1.Task, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Task.Create().
		SetNillableType(r.typeConverter.ToEntity(req.Data.Type)).
		SetNillableTypeName(req.Data.TypeName).
		SetNillableName(req.Data.Name).
		SetNillableTaskPayload(req.Data.TaskPayload).
		SetNillableCronSpec(req.Data.CronSpec).
		SetNillableEnable(req.Data.Enable).
		SetNillableRemark(req.Data.Remark).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.TaskOptions != nil {
		builder.SetTaskOptions(req.Data.TaskOptions)
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	t, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("insert data failed")
	}

	return r.mapper.ToDTO(t), nil
}

func (r *taskRepo) Update(ctx context.Context, req *adminV1.UpdateTaskRequest) (*adminV1.Task, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
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
			return nil, err
		}
		if !exist {
			createReq := &adminV1.CreateTaskRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			return r.Create(ctx, createReq)
		}
	}

	if req.UpdateMask != nil {
		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return nil, adminV1.ErrorBadRequest("invalid field mask")
		}
		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().
		//Debug().
		Task.UpdateOneID(id).
		SetNillableType(r.typeConverter.ToEntity(req.Data.Type)).
		SetNillableTypeName(req.Data.TypeName).
		SetNillableName(req.Data.Name).
		SetNillableTaskPayload(req.Data.TaskPayload).
		SetNillableCronSpec(req.Data.CronSpec).
		SetNillableEnable(req.Data.Enable).
		SetNillableRemark(req.Data.Remark).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	if req.Data.TaskOptions != nil {
		builder.SetTaskOptions(req.Data.TaskOptions)
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

	t, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("update one data failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("update data failed")
	}

	return r.mapper.ToDTO(t), nil
}

func (r *taskRepo) Delete(ctx context.Context, req *adminV1.DeleteTaskRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().Task.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("task not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return adminV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
