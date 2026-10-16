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

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/adminloginrestriction"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	adminV1 "framework/api/gen/go/admin/v1"
)

type AdminLoginRestrictionRepo struct {
	data *Data
	log  *log.Helper

	mapper              *mapper.CopierMapper[adminV1.AdminLoginRestriction, ent.AdminLoginRestriction]
	targetTypeConverter *mapper.EnumTypeConverter[adminV1.AdminLoginRestrictionTargetType, adminloginrestriction.TargetType]
	actionConverter     *mapper.EnumTypeConverter[adminV1.AdminLoginRestrictionAction, adminloginrestriction.Action]
}

func NewAdminLoginRestrictionRepo(data *Data, logger log.Logger) *AdminLoginRestrictionRepo {
	repo := &AdminLoginRestrictionRepo{
		log:                 log.NewHelper(log.With(logger, "module", "admin-login-restriction/repo/admin-service")),
		data:                data,
		mapper:              mapper.NewCopierMapper[adminV1.AdminLoginRestriction, ent.AdminLoginRestriction](),
		targetTypeConverter: mapper.NewEnumTypeConverter[adminV1.AdminLoginRestrictionTargetType, adminloginrestriction.TargetType](adminV1.AdminLoginRestrictionTargetType_name, adminV1.AdminLoginRestrictionTargetType_value),
		actionConverter:     mapper.NewEnumTypeConverter[adminV1.AdminLoginRestrictionAction, adminloginrestriction.Action](adminV1.AdminLoginRestrictionAction_name, adminV1.AdminLoginRestrictionAction_value),
	}

	repo.init()

	return repo
}

func (r *AdminLoginRestrictionRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.targetTypeConverter.NewConverterPair())
	r.mapper.AppendConverters(r.actionConverter.NewConverterPair())
	// Status：ent 具名字符串枚举指针 ↔ DTO *string（copier 不自动处理具名指针类型）
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		(*adminloginrestriction.Status)(nil), (*string)(nil),
		func(src *adminloginrestriction.Status) *string {
			if src == nil {
				return nil
			}
			s := string(*src)
			return &s
		},
		func(src *string) *adminloginrestriction.Status {
			if src == nil {
				return nil
			}
			s := adminloginrestriction.Status(*src)
			return &s
		},
	))
	// HitCount：ent uint64（非指针）↔ DTO *uint64（proto optional）
	r.mapper.AppendConverters(copierutil.NewGenericTypeConverterPair(
		uint64(0), (*uint64)(nil),
		func(src uint64) *uint64 { v := src; return &v },
		func(src *uint64) uint64 {
			if src == nil {
				return 0
			}
			return *src
		},
	))
}

func (r *AdminLoginRestrictionRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().AdminLoginRestriction.Query()
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

func (r *AdminLoginRestrictionRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginRestrictionResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().AdminLoginRestriction.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), adminloginrestriction.FieldCreateTime,
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

	dtos := make([]*adminV1.AdminLoginRestriction, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListAdminLoginRestrictionResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

// ListByTargetType 按目标类型查询所有限制规则，供登录流程校验使用。
// 返回该目标类型下的全部规则，由调用方在内存中按 target_value/action 比对，
// 规则数量通常极少，无需分页。
func (r *AdminLoginRestrictionRepo) ListByTargetType(ctx context.Context, targetType adminV1.AdminLoginRestrictionTargetType) ([]*adminV1.AdminLoginRestriction, error) {
	entityType := r.targetTypeConverter.ToEntity(&targetType)
	if entityType == nil {
		return nil, adminV1.ErrorBadRequest("invalid target type")
	}

	entities, err := r.data.db.Client().AdminLoginRestriction.Query().
		Where(adminloginrestriction.TargetTypeEQ(*entityType)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query by target type failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query login restriction failed")
	}

	dtos := make([]*adminV1.AdminLoginRestriction, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}
	return dtos, nil
}

func (r *AdminLoginRestrictionRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().AdminLoginRestriction.Query().
		Where(adminloginrestriction.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *AdminLoginRestrictionRepo) Get(ctx context.Context, req *adminV1.GetAdminLoginRestrictionRequest) (*adminV1.AdminLoginRestriction, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().AdminLoginRestriction.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("admin login restriction not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *AdminLoginRestrictionRepo) Create(ctx context.Context, req *adminV1.CreateAdminLoginRestrictionRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}

	builder := r.data.db.Client().AdminLoginRestriction.Create().
		SetNillableTargetType(r.targetTypeConverter.ToEntity(req.Data.TargetType)).
		SetNillableTargetValue(req.Data.TargetValue).
		SetNillableAction(r.actionConverter.ToEntity(req.Data.Action)).
		SetNillableReason(req.Data.Reason).
		SetNillableStatus((*adminloginrestriction.Status)(req.Data.Status)).
		SetNillableExpireTime(timeutil.TimestamppbToTime(req.Data.ExpireTime)).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if err := builder.Exec(ctx); err != nil {
		// 唯一索引（target_type+target_value+action）冲突兜底映射为 409 友好错误（biz 预检未覆盖的并发场景）
		if ent.IsConstraintError(err) {
			return adminV1.ErrorLoginRestrictionConflict("与现存规则重复（目标类型+目标值+动作已存在）")
		}
		r.log.Errorf("insert one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *AdminLoginRestrictionRepo) Update(ctx context.Context, req *adminV1.UpdateAdminLoginRestrictionRequest) error {
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
			createReq := &adminV1.CreateAdminLoginRestrictionRequest{Data: req.Data}
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

	builder := r.data.db.Client().AdminLoginRestriction.UpdateOneID(id).
		SetNillableTargetType(r.targetTypeConverter.ToEntity(req.Data.TargetType)).
		SetNillableTargetValue(req.Data.TargetValue).
		SetNillableAction(r.actionConverter.ToEntity(req.Data.Action)).
		SetNillableReason(req.Data.Reason).
		SetNillableStatus((*adminloginrestriction.Status)(req.Data.Status)).
		SetNillableExpireTime(timeutil.TimestamppbToTime(req.Data.ExpireTime)).
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
		// 唯一索引冲突兜底映射（同 Create）
		if ent.IsConstraintError(err) {
			return adminV1.ErrorLoginRestrictionConflict("与现存规则重复（目标类型+目标值+动作已存在）")
		}
		r.log.Errorf("update one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

// IncrementHit 命中统计：hit_count+1 并回写 last_hit_time。
// 登录判定命中决定性规则（ALLOW_MATCH/BLOCK_MATCH）时异步调用；失败由调用方降级为日志，不影响登录链路。
func (r *AdminLoginRestrictionRepo) IncrementHit(ctx context.Context, id uint32) error {
	if id == 0 {
		return nil
	}
	if err := r.data.db.Client().AdminLoginRestriction.UpdateOneID(id).
		AddHitCount(1).
		SetLastHitTime(time.Now()).
		Exec(ctx); err != nil {
		r.log.Warnf("increment hit count for rule %d failed: %s", id, err.Error())
		return err
	}
	return nil
}

func (r *AdminLoginRestrictionRepo) Delete(ctx context.Context, req *adminV1.DeleteAdminLoginRestrictionRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().AdminLoginRestriction.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("admin login restriction not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return adminV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
