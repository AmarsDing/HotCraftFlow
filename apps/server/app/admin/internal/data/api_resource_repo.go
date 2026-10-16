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

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/apiresource"
	"framework/app/admin/internal/data/ent/roleapi"

	adminV1 "framework/api/gen/go/admin/v1"
	"framework/app/admin/internal/biz"
)

type ApiResourceRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[adminV1.ApiResource, ent.ApiResource]
}

func NewApiResourceRepo(data *Data, logger log.Logger) *ApiResourceRepo {
	repo := &ApiResourceRepo{
		log:    log.NewHelper(log.With(logger, "module", "api-resource/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[adminV1.ApiResource, ent.ApiResource](),
	}

	repo.init()

	return repo
}

func (r *ApiResourceRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

// isConflictErr 唯一约束冲突（path+method 重复）转 409，替代裸 500。
func isConflictErr(err error) bool {
	return ent.IsConstraintError(err)
}

func (r *ApiResourceRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().ApiResource.Query()
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

func (r *ApiResourceRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListApiResourceResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().ApiResource.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), apiresource.FieldCreateTime,
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

	dtos := make([]*adminV1.ApiResource, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListApiResourceResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

func (r *ApiResourceRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().ApiResource.Query().
		Where(apiresource.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *ApiResourceRepo) Get(ctx context.Context, req *adminV1.GetApiResourceRequest) (*adminV1.ApiResource, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().ApiResource.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("api resource not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *ApiResourceRepo) GetApiResourceByEndpoint(ctx context.Context, path, method string) (*adminV1.ApiResource, error) {
	if path == "" || method == "" {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().ApiResource.Query().
		Where(
			apiresource.PathEQ(path),
			apiresource.MethodEQ(method),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("api resource not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *ApiResourceRepo) Create(ctx context.Context, req *adminV1.CreateApiResourceRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().ApiResource.Create().
		SetNillableDescription(req.Data.Description).
		SetNillableModule(req.Data.Module).
		SetNillableModuleDescription(req.Data.ModuleDescription).
		SetNillableOperation(req.Data.Operation).
		SetNillablePath(req.Data.Path).
		SetNillableMethod(req.Data.Method).
		SetNillableSource(req.Data.Source).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		if isConflictErr(err) {
			return adminV1.ErrorConflict("相同路径与方法的 API 资源已存在")
		}
		r.log.Errorf("insert one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *ApiResourceRepo) Update(ctx context.Context, req *adminV1.UpdateApiResourceRequest) error {
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
			createReq := &adminV1.CreateApiResourceRequest{Data: req.Data}
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

	builder := r.data.db.Client().ApiResource.
		UpdateOneID(id).
		SetNillableDescription(req.Data.Description).
		SetNillableModule(req.Data.Module).
		SetNillableModuleDescription(req.Data.ModuleDescription).
		SetNillableOperation(req.Data.Operation).
		SetNillablePath(req.Data.Path).
		SetNillableMethod(req.Data.Method).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	// 启停开关（人工语义）：仅在 updateMask 显式携带 status 时写入；非法取值拒绝
	if req.Data.Status != nil {
		st := apiresource.Status(req.Data.GetStatus())
		if st != apiresource.StatusOn && st != apiresource.StatusOff {
			return adminV1.ErrorBadRequest("invalid status, expect ON/OFF")
		}
		builder.SetStatus(st)
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
		if isConflictErr(err) {
			return adminV1.ErrorConflict("相同路径与方法的 API 资源已存在")
		}
		r.log.Errorf("update one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

// Delete 事务级联：先清 ap_org_role_api 中该资源的全部角色绑定，再删资源行。
// 授权影响面对调用方透明（前端删除确认已按 role_count 提示）。
func (r *ApiResourceRepo) Delete(ctx context.Context, req *adminV1.DeleteApiResourceRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed [%d]: %s", req.GetId(), err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}

	if _, err = tx.RoleApi.Delete().
		Where(roleapi.APIIDEQ(req.GetId())).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete role api bindings failed [%d]: %s", req.GetId(), err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}

	if err = tx.ApiResource.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("api resource not found")
		}
		r.log.Errorf("delete one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}

	if err = tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed [%d]: %s", req.GetId(), err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}

	return nil
}

// ListAllForSync 返回全部 API 资源的轻量行（同步 upsert diff 用，一次往返）。
// 行类型由 biz 层拥有（biz.ApiResourceSyncRow），避免 data 层类型反向泄漏。
func (r *ApiResourceRepo) ListAllForSync(ctx context.Context) ([]biz.ApiResourceSyncRow, error) {
	entities, err := r.data.db.Client().ApiResource.Query().All(ctx)
	if err != nil {
		r.log.Errorf("list all api resources failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	rows := make([]biz.ApiResourceSyncRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, biz.ApiResourceSyncRow{
			ID:                e.ID,
			Path:              e.Path,
			Method:            e.Method,
			Source:            e.Source,
			Description:       derefStr(e.Description),
			Module:            derefStr(e.Module),
			ModuleDescription: derefStr(e.ModuleDescription),
			Operation:         derefStr(e.Operation),
			Delisted:          e.Delisted,
		})
	}
	return rows, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// UpdateSyncFields 同步 upsert 专用：仅更新文档派生字段与 delisted 标记，
// 绝不触碰 id/status（人工启停）/create_by——保证授权绑定不因同步漂移。
func (r *ApiResourceRepo) UpdateSyncFields(ctx context.Context, id uint32, description, module, moduleDescription, operation string, delisted bool) error {
	err := r.data.db.Client().ApiResource.UpdateOneID(id).
		SetDescription(description).
		SetModule(module).
		SetModuleDescription(moduleDescription).
		SetOperation(operation).
		SetDelisted(delisted).
		SetUpdateTime(time.Now()).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("update sync fields failed [%d]: %s", id, err.Error())
		return adminV1.ErrorInternalServerError("update sync fields failed")
	}
	return nil
}

// SetDelisted 标记/取消疑似下线（同步独占维护）。
func (r *ApiResourceRepo) SetDelisted(ctx context.Context, id uint32, delisted bool) error {
	err := r.data.db.Client().ApiResource.UpdateOneID(id).
		SetDelisted(delisted).
		SetUpdateTime(time.Now()).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("set delisted failed [%d]: %s", id, err.Error())
		return adminV1.ErrorInternalServerError("set delisted failed")
	}
	return nil
}

// CountRoleBindingsByApiIDs 统计各 API 资源的角色直绑数（删除影响面/列表 role_count 列）。
func (r *ApiResourceRepo) CountRoleBindingsByApiIDs(ctx context.Context, ids []uint32) (map[uint32]int, error) {
	result := make(map[uint32]int, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var rows []struct {
		ApiID uint32 `json:"api_id"`
		Count int    `json:"count"`
	}
	err := r.data.db.Client().RoleApi.Query().
		Where(roleapi.APIIDIn(ids...)).
		GroupBy(roleapi.FieldAPIID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		r.log.Errorf("count role api bindings failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query role bindings failed")
	}

	for _, row := range rows {
		result[row.ApiID] = row.Count
	}
	return result, nil
}

// ApiResourceBrief 是供 authorizer 内部使用的轻量结构，避免依赖 proto DTO。
type ApiResourceBrief struct {
	ID     uint32
	Path   string
	Method string
}

// ListAllBriefs 返回参与鉴权展开的 API 资源轻量结构：
// 仅 status 启用（NULL 视为启用）且未标记下线的端点进入 casbin 策略。
func (r *ApiResourceRepo) ListAllBriefs(ctx context.Context) ([]ApiResourceBrief, error) {
	entities, err := r.data.db.Client().ApiResource.Query().
		Where(
			apiresource.DelistedEQ(false),
			func(s *sql.Selector) {
				s.Where(sql.Or(
					sql.IsNull(apiresource.FieldStatus),
					sql.EQ(apiresource.FieldStatus, string(apiresource.StatusOn)),
				))
			},
		).
		All(ctx)
	if err != nil {
		r.log.Errorf("list api resource briefs failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	briefs := make([]ApiResourceBrief, 0, len(entities))
	for _, e := range entities {
		briefs = append(briefs, ApiResourceBrief{
			ID:     e.ID,
			Path:   e.Path,
			Method: e.Method,
		})
	}
	return briefs, nil
}
