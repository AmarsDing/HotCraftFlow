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
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/systemconfig"

	adminV1 "framework/api/gen/go/admin/v1"
)

// systemConfigRepo 未导出，实现 biz.SystemConfigRepo 接口（端口-适配器模式）。
type systemConfigRepo struct {
	data   *Data
	log    *log.Helper
	mapper *mapper.CopierMapper[adminV1.SystemConfig, ent.SystemConfig]
}

// NewSystemConfigRepo 返回 biz.SystemConfigRepo 接口，wire 据返回类型自动绑定到 Usecase 入参。
func NewSystemConfigRepo(data *Data, logger log.Logger) biz.SystemConfigRepo {
	repo := &systemConfigRepo{
		log:    log.NewHelper(log.With(logger, "module", "system-config/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[adminV1.SystemConfig, ent.SystemConfig](),
	}
	repo.init()
	return repo
}

func (r *systemConfigRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

func (r *systemConfigRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().SystemConfig.Query()
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

func (r *systemConfigRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListSystemConfigResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().SystemConfig.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), systemconfig.FieldCreateTime,
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

	dtos := make([]*adminV1.SystemConfig, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListSystemConfigResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

func (r *systemConfigRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().SystemConfig.Query().
		Where(systemconfig.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *systemConfigRepo) Get(ctx context.Context, req *adminV1.GetSystemConfigRequest) (*adminV1.SystemConfig, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	entity, err := r.data.db.Client().SystemConfig.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("system config not found")
		}
		r.log.Errorf("query one data failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query data failed")
	}
	return r.mapper.ToDTO(entity), nil
}

// GetByKey 按 config_key 查询单条配置（seed 与缓存加载用）。
func (r *systemConfigRepo) GetByKey(ctx context.Context, key string) (*adminV1.SystemConfig, error) {
	if key == "" {
		return nil, nil
	}
	entity, err := r.data.db.Client().SystemConfig.Query().
		Where(systemconfig.ConfigKeyEQ(key)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query by key failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query data failed")
	}
	return r.mapper.ToDTO(entity), nil
}

// GetAll 全量加载配置（缓存刷新用，无分页）。
func (r *systemConfigRepo) GetAll(ctx context.Context) ([]*adminV1.SystemConfig, error) {
	entities, err := r.data.db.Client().SystemConfig.Query().All(ctx)
	if err != nil {
		r.log.Errorf("query all failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query all failed")
	}
	dtos := make([]*adminV1.SystemConfig, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}
	return dtos, nil
}

func (r *systemConfigRepo) Create(ctx context.Context, req *adminV1.CreateSystemConfigRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().SystemConfig.Create().
		SetConfigKey(req.Data.GetConfigKey()).
		SetConfigValue(req.Data.GetConfigValue()).
		SetConfigType(req.Data.GetConfigType()).
		SetNillableDescription(req.Data.Description).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.ConfigType == nil {
		builder.SetConfigType("string")
	}
	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}
	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert data failed")
	}
	return nil
}

func (r *systemConfigRepo) Update(ctx context.Context, req *adminV1.UpdateSystemConfigRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
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
			createReq := &adminV1.CreateSystemConfigRequest{Data: req.Data}
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

	builder := r.data.db.Client().SystemConfig.
		UpdateOneID(id).
		SetNillableDescription(req.Data.Description).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	// 必填列按"字段是否被提交"守卫写入：部分更新时 fieldmaskutil.Filter
	// 已把未触碰字段置 nil，直接 Get 会得到空串并把列擦空。
	if req.Data.ConfigKey != nil {
		builder.SetConfigKey(req.Data.GetConfigKey())
	}
	if req.Data.ConfigValue != nil {
		builder.SetConfigValue(req.Data.GetConfigValue())
	}
	if req.Data.ConfigType != nil {
		builder.SetConfigType(req.Data.GetConfigType())
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

func (r *systemConfigRepo) Delete(ctx context.Context, req *adminV1.DeleteSystemConfigRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if err := r.data.db.Client().SystemConfig.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("system config not found")
		}
		r.log.Errorf("delete one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	return nil
}
