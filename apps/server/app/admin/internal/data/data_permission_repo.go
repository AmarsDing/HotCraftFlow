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
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/datapermission"
	"framework/app/admin/internal/data/ent/user"
	"framework/app/admin/internal/data/ent/userrole"

	adminV1 "framework/api/gen/go/admin/v1"
)

// dataPermissionRepo 未导出，实现 biz.DataPermissionRepo 接口（端口-适配器模式）。
type dataPermissionRepo struct {
	data   *Data
	log    *log.Helper
	mapper *mapper.CopierMapper[adminV1.DataPermission, ent.DataPermission]
}

// NewDataPermissionRepo 返回 biz.DataPermissionRepo 接口，wire 据返回类型自动绑定到 Usecase 入参。
func NewDataPermissionRepo(data *Data, logger log.Logger) biz.DataPermissionRepo {
	repo := &dataPermissionRepo{
		log:    log.NewHelper(log.With(logger, "module", "data-permission/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[adminV1.DataPermission, ent.DataPermission](),
	}
	repo.init()
	return repo
}

func (r *dataPermissionRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

func (r *dataPermissionRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().DataPermission.Query()
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

func (r *dataPermissionRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDataPermissionResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().DataPermission.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), datapermission.FieldCreateTime,
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

	dtos := make([]*adminV1.DataPermission, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListDataPermissionResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

func (r *dataPermissionRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().DataPermission.Query().
		Where(datapermission.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *dataPermissionRepo) Get(ctx context.Context, req *adminV1.GetDataPermissionRequest) (*adminV1.DataPermission, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	entity, err := r.data.db.Client().DataPermission.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("data permission not found")
		}
		r.log.Errorf("query one data failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query data failed")
	}
	return r.mapper.ToDTO(entity), nil
}

// GetByRoleIDs 查询多个角色的数据权限规则（仅返回启用的）。
func (r *dataPermissionRepo) GetByRoleIDs(ctx context.Context, roleIDs []uint32) ([]*adminV1.DataPermission, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	entities, err := r.data.db.Client().DataPermission.Query().
		Where(
			datapermission.RoleIDIn(roleIDs...),
			datapermission.StatusEQ(datapermission.StatusOn),
		).
		All(ctx)
	if err != nil {
		r.log.Errorf("query by role ids failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query data failed")
	}
	dtos := make([]*adminV1.DataPermission, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}
	return dtos, nil
}

// GetRoleIDsByUserID 查询用户绑定的角色 ID 列表（通过 user_role 表）。
func (r *dataPermissionRepo) GetRoleIDsByUserID(ctx context.Context, userID uint32) ([]uint32, error) {
	if userID == 0 {
		return nil, nil
	}
	entities, err := r.data.db.Client().UserRole.Query().
		Where(userrole.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query user roles failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query user roles failed")
	}
	roleIDs := make([]uint32, 0, len(entities))
	for _, e := range entities {
		roleIDs = append(roleIDs, e.RoleID)
	}
	return roleIDs, nil
}

// GetUserDeptID 查询用户所属部门 ID（通过 user 表 dept_id 字段）。
func (r *dataPermissionRepo) GetUserDeptID(ctx context.Context, userID uint32) (uint32, error) {
	if userID == 0 {
		return 0, nil
	}
	u, err := r.data.db.Client().User.Query().
		Where(user.IDEQ(userID)).
		Select(user.FieldDeptID).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, nil
		}
		r.log.Errorf("query user dept id failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("query user dept failed")
	}
	if u.DeptID == nil {
		return 0, nil
	}
	return *u.DeptID, nil
}

// GetUserAuthority 查询用户授权等级（SYS_ADMIN/TENANT_ADMIN/CUSTOMER_USER/GUEST）。
// 用户不存在或字段为空时返回空字符串，由 biz 层按普通用户处理。
func (r *dataPermissionRepo) GetUserAuthority(ctx context.Context, userID uint32) (string, error) {
	if userID == 0 {
		return "", nil
	}
	u, err := r.data.db.Client().User.Query().
		Where(user.IDEQ(userID)).
		Select(user.FieldAuthority).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil
		}
		r.log.Errorf("query user authority failed: %s", err.Error())
		return "", adminV1.ErrorInternalServerError("query user authority failed")
	}
	if u.Authority == nil {
		return "", nil
	}
	return string(*u.Authority), nil
}

// normalizeScope 归一化范围值：proto 文档与前端使用小写（all/dept/custom/self），
// ent 枚举与 DB 存储为大写（ALL/DEPT/CUSTOM/SELF），写入前统一转大写，对调用方大小写宽容。
func normalizeScope(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func (r *dataPermissionRepo) Create(ctx context.Context, req *adminV1.CreateDataPermissionRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().DataPermission.Create().
		SetRoleID(req.Data.GetRoleId()).
		SetName(req.Data.GetName()).
		SetNillableField(req.Data.Field).
		SetNillableRuleJSON(req.Data.RuleJson).
		SetNillableTenantID(req.Data.TenantId)

	// scope 为空时跳过，交由 ent 默认值 SELF 生效
	if scope := normalizeScope(req.Data.GetScope()); scope != "" {
		builder.SetScope(datapermission.Scope(scope))
	}

	// Status 默认 ON
	if req.Data.Status != nil && *req.Data.Status != "" {
		s := datapermission.Status(*req.Data.Status)
		builder.SetStatus(s)
	}
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
		return adminV1.ErrorInternalServerError("insert data failed")
	}
	return nil
}

func (r *dataPermissionRepo) Update(ctx context.Context, req *adminV1.UpdateDataPermissionRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
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
			createReq := &adminV1.CreateDataPermissionRequest{Data: req.Data}
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

	// SetNillable 模式：updateMask 过滤后未命中的字段为 nil，SetNillable* 自动跳过，
	// 避免部分更新（如仅改 scope/status）把 role_id/name 冲成零值。
	builder := r.data.db.Client().DataPermission.
		UpdateOneID(id).
		SetNillableRoleID(req.Data.RoleId).
		SetNillableName(req.Data.Name).
		SetNillableField(req.Data.Field).
		SetNillableRuleJSON(req.Data.RuleJson)

	// scope 仅在请求显式携带时更新，且归一化为大写枚举
	if req.Data.Scope != nil {
		builder.SetScope(datapermission.Scope(normalizeScope(req.Data.GetScope())))
	}

	if req.Data.Status != nil && *req.Data.Status != "" {
		s := datapermission.Status(*req.Data.Status)
		builder.SetStatus(s)
	}
	builder.SetUpdateTime(time.Now())

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

func (r *dataPermissionRepo) Delete(ctx context.Context, req *adminV1.DeleteDataPermissionRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if err := r.data.db.Client().DataPermission.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("data permission not found")
		}
		r.log.Errorf("delete one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	return nil
}

// 确保编译期接口实现：dataPermissionRepo 实现 biz.DataPermissionRepo。
var _ biz.DataPermissionRepo = (*dataPermissionRepo)(nil)
