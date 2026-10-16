package data

import (
	"context"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/copierutil"
	entgo "github.com/tx7do/go-utils/entgo/query"
	entgoUpdate "github.com/tx7do/go-utils/entgo/update"
	"github.com/tx7do/go-utils/fieldmaskutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/tenant"
	"framework/app/admin/internal/data/ent/user"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	userV1 "framework/api/gen/go/user/v1"
)

// tenantOrderFields 排序字段白名单（orderBy 前端 camelCase / 底层 snake_case → ent 列名）。
// BuildQuerySelector 将 orderBy 原样拼入 SQL 列名，前端列名（createTime/memberCount）会导致 500，
// 参照 monitor_asset_repo.go 的 monitorAssetOrderFields 先例做显式映射，同时防止任意列名注入。
var tenantOrderFields = map[string]string{
	"id":           tenant.FieldID,
	"name":         tenant.FieldName,
	"code":         tenant.FieldCode,
	"memberCount":  tenant.FieldMemberCount,
	"member_count": tenant.FieldMemberCount,
	"status":       tenant.FieldStatus,
	"createTime":   tenant.FieldCreateTime,
	"create_time":  tenant.FieldCreateTime,
	"updateTime":   tenant.FieldUpdateTime,
	"update_time":  tenant.FieldUpdateTime,
}

// mapTenantOrderBy 映射排序字段；全部非法时返回 nil（由 BuildQuerySelector 回退默认排序）。
func mapTenantOrderBy(orderBys []string) []string {
	if len(orderBys) == 0 {
		return nil
	}
	out := make([]string, 0, len(orderBys))
	for _, o := range orderBys {
		desc := strings.HasPrefix(o, "-")
		key := strings.TrimPrefix(o, "-")
		col, ok := tenantOrderFields[key]
		if !ok {
			continue
		}
		if desc {
			out = append(out, "-"+col)
		} else {
			out = append(out, col)
		}
	}
	return out
}

type TenantRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[userV1.Tenant, ent.Tenant]
}

func NewTenantRepo(data *Data, logger log.Logger) *TenantRepo {
	repo := &TenantRepo{
		log:    log.NewHelper(log.With(logger, "module", "tenant/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[userV1.Tenant, ent.Tenant](),
	}

	repo.init()

	return repo
}

func (r *TenantRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

func (r *TenantRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().Tenant.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, userV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *TenantRepo) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListTenantResponse, error) {
	if req == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Tenant.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		mapTenantOrderBy(req.GetOrderBy()), tenant.FieldCreateTime,
		req.GetFieldMask().GetPaths(),
	)
	if err != nil {
		r.log.Errorf("parse list param error [%s]", err.Error())
		return nil, userV1.ErrorBadRequest("invalid query parameter")
	}

	if querySelectors != nil {
		builder.Modify(querySelectors...)
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list failed: %s", err.Error())
		return nil, userV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*userV1.Tenant, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	// 成员数为列表展示实时聚合（member_count 存储字段无维护链路，直读会失真）
	r.fillMemberCounts(ctx, entities, dtos)

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &userV1.ListTenantResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

// fillMemberCounts 批量填充各租户真实成员数（一次 GROUP BY，避免 N+1）；
// 聚合失败仅降级为不覆写（保留库值），不阻断列表。
func (r *TenantRepo) fillMemberCounts(ctx context.Context, entities []*ent.Tenant, dtos []*userV1.Tenant) {
	if len(entities) == 0 {
		return
	}
	ids := make([]uint32, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ID)
	}
	var rows []struct {
		TenantID uint32 `json:"tenant_id"`
		Count    int    `json:"count"`
	}
	if err := r.data.db.Client().User.Query().
		Where(user.TenantIDIn(ids...), user.DeleteTimeIsNil()).
		GroupBy(user.FieldTenantID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows); err != nil {
		r.log.Warnf("aggregate tenant member counts failed: %s", err.Error())
		return
	}
	counts := make(map[uint32]int32, len(rows))
	for _, row := range rows {
		counts[row.TenantID] = int32(row.Count)
	}
	for i, e := range entities {
		dtos[i].MemberCount = trans.Ptr(counts[e.ID])
	}
}

func (r *TenantRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().Tenant.Query().
		Where(tenant.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, userV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *TenantRepo) Get(ctx context.Context, req *userV1.GetTenantRequest) (*userV1.Tenant, error) {
	if req == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Tenant.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorTenantNotFound("tenant not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *TenantRepo) Create(ctx context.Context, req *userV1.CreateTenantRequest) error {
	if req == nil || req.Data == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Tenant.Create().
		SetNillableName(req.Data.Name).
		SetNillableCode(req.Data.Code).
		SetNillableMemberCount(req.Data.MemberCount).
		SetNillableRemark(req.Data.Remark).
		SetNillableStatus((*tenant.Status)(req.Data.Status)).
		SetNillableSubscriptionAt(timeutil.TimestamppbToTime(req.Data.SubscriptionAt)).
		SetNillableUnsubscribeAt(timeutil.TimestamppbToTime(req.Data.UnsubscribeAt))

	builder.SetNillableCreateBy(req.Data.CreateBy)

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	} else {
		builder.SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		if isPgUniqueViolation(err) || ent.IsConstraintError(err) {
			return userV1.ErrorBadRequest("租户编码已存在")
		}
		if ent.IsValidationError(err) {
			return userV1.ErrorBadRequest("租户数据校验失败：%s", err.Error())
		}
		return userV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *TenantRepo) Update(ctx context.Context, req *userV1.UpdateTenantRequest) error {
	if req == nil || req.Data == nil {
		return userV1.ErrorBadRequest("invalid parameter")
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
			createReq := &userV1.CreateTenantRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			return r.Create(ctx, createReq)
		}
	}

	if req.UpdateMask != nil {
		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return userV1.ErrorBadRequest("invalid field mask")
		}
		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().Tenant.UpdateOneID(id).
		SetNillableName(req.Data.Name).
		SetNillableCode(req.Data.Code).
		SetNillableMemberCount(req.Data.MemberCount).
		SetNillableRemark(req.Data.Remark).
		SetNillableStatus((*tenant.Status)(req.Data.Status)).
		SetNillableSubscriptionAt(timeutil.TimestamppbToTime(req.Data.SubscriptionAt)).
		SetNillableUnsubscribeAt(timeutil.TimestamppbToTime(req.Data.UnsubscribeAt))

	builder.SetNillableUpdateBy(req.Data.UpdateBy)

	if req.Data.UpdateTime == nil {
		builder.SetUpdateTime(time.Now())
	} else {
		builder.SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))
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
		if isPgUniqueViolation(err) || ent.IsConstraintError(err) {
			return userV1.ErrorBadRequest("租户编码已存在")
		}
		if ent.IsValidationError(err) {
			return userV1.ErrorBadRequest("租户数据校验失败：%s", err.Error())
		}
		return userV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

func (r *TenantRepo) Delete(ctx context.Context, req *userV1.DeleteTenantRequest) error {
	if req == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	// 删除前校验：仍存在关联用户（未软删）时拒绝删除（需求 4.9）
	userCount, err := r.data.db.Client().User.Query().
		Where(user.TenantIDEQ(req.GetId()), user.DeleteTimeIsNil()).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count tenant users failed: %s", err.Error())
		return userV1.ErrorInternalServerError("delete failed")
	}
	if userCount > 0 {
		return userV1.ErrorBadRequest("tenant %d still has %d associated user(s), remove them first", req.GetId(), userCount)
	}

	if err := r.data.db.Client().Tenant.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return userV1.ErrorNotFound("tenant not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return userV1.ErrorInternalServerError("delete failed")
	}

	return nil
}

func (r *TenantRepo) GetTenantByTenantCode(ctx context.Context, code string) (*userV1.Tenant, error) {
	entity, err := r.data.db.Client().Tenant.Query().
		Where(tenant.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorNotFound("tenant not found")
		}

		r.log.Errorf("query tenant data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}
