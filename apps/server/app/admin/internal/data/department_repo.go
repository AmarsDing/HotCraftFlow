package data

import (
	"context"
	"sort"
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
	"framework/app/admin/internal/data/ent/department"
	"framework/app/admin/internal/data/ent/organization"
	"framework/app/admin/internal/data/ent/user"

	userV1 "framework/api/gen/go/user/v1"
)

type DepartmentRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[userV1.Department, ent.Department]
}

func NewDepartmentRepo(data *Data, logger log.Logger) *DepartmentRepo {
	repo := &DepartmentRepo{
		log:    log.NewHelper(log.With(logger, "module", "department/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[userV1.Department, ent.Department](),
	}

	repo.init()

	return repo
}

func (r *DepartmentRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

// toDTO 转换实体为 DTO，并显式回填 organizationId（copier 无法自动处理 int32↔uint32 nillable 转换）
// 与 leaderId（ent 字段 LeaderID 与 DTO 字段 LeaderId 名不一致，copier 按名映射会漏掉）。
func (r *DepartmentRepo) toDTO(entity *ent.Department) *userV1.Department {
	dto := r.mapper.ToDTO(entity)
	if entity.OrganizationID != nil {
		v := int32(*entity.OrganizationID)
		dto.OrganizationId = &v
	}
	if entity.LeaderID != nil {
		dto.LeaderId = entity.LeaderID
	}
	return dto
}

// fillOrganizationNames 批量填充部门 DTO 的组织名称（一次 IN 查询，避免 N+1）。
func (r *DepartmentRepo) fillOrganizationNames(ctx context.Context, dtos []*userV1.Department) {
	idSet := make(map[uint32]struct{})
	var walk func(items []*userV1.Department)
	walk = func(items []*userV1.Department) {
		for _, d := range items {
			if d.GetOrganizationId() > 0 {
				idSet[uint32(d.GetOrganizationId())] = struct{}{}
			}
			if len(d.Children) > 0 {
				walk(d.Children)
			}
		}
	}
	walk(dtos)
	if len(idSet) == 0 {
		return
	}
	ids := make([]uint32, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	orgs, err := r.data.db.Client().Organization.Query().
		Where(organization.IDIn(ids...)).
		Select(organization.FieldID, organization.FieldName).
		All(ctx)
	if err != nil {
		r.log.Errorf("query organizations for departments failed: %s", err.Error())
		return
	}
	names := make(map[uint32]string, len(orgs))
	for _, o := range orgs {
		if o.Name != nil {
			names[o.ID] = *o.Name
		}
	}
	var fill func(items []*userV1.Department)
	fill = func(items []*userV1.Department) {
		for _, d := range items {
			if name, ok := names[uint32(d.GetOrganizationId())]; ok {
				d.OrganizationName = &name
			}
			if len(d.Children) > 0 {
				fill(d.Children)
			}
		}
	}
	fill(dtos)
}

// fillLeaderNames 批量填充部门 DTO 的领导姓名（一次 IN 查询 user 表，避免 N+1；仿 fillOrganizationNames）。
// 姓名口径与用户列表一致：realname 优先，回退 nickname。
func (r *DepartmentRepo) fillLeaderNames(ctx context.Context, dtos []*userV1.Department) {
	idSet := make(map[uint32]struct{})
	var walk func(items []*userV1.Department)
	walk = func(items []*userV1.Department) {
		for _, d := range items {
			if d.GetLeaderId() > 0 {
				idSet[d.GetLeaderId()] = struct{}{}
			}
			if len(d.Children) > 0 {
				walk(d.Children)
			}
		}
	}
	walk(dtos)
	if len(idSet) == 0 {
		return
	}
	ids := make([]uint32, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	users, err := r.data.db.Client().User.Query().
		Where(user.IDIn(ids...)).
		Select(user.FieldID, user.FieldRealname, user.FieldNickname).
		All(ctx)
	if err != nil {
		r.log.Errorf("query leaders for departments failed: %s", err.Error())
		return
	}
	names := make(map[uint32]string, len(users))
	for _, u := range users {
		name := ""
		if u.Realname != nil {
			name = *u.Realname
		}
		if name == "" && u.Nickname != nil {
			name = *u.Nickname
		}
		if name != "" {
			names[u.ID] = name
		}
	}
	var fill func(items []*userV1.Department)
	fill = func(items []*userV1.Department) {
		for _, d := range items {
			if name, ok := names[d.GetLeaderId()]; ok {
				d.LeaderName = &name
			}
			if len(d.Children) > 0 {
				fill(d.Children)
			}
		}
	}
	fill(dtos)
}

// fillAssetCounts 批量填充各部门直接管理资产数量（按 monitor_assets.manage_dept_id 一次 GROUP BY，避免 N+1）。
// 口径：仅统计本部门直接管理的资产，不向父部门聚合子部门数量（与设备分类树 fillAssetCounts 同模式）。
func (r *DepartmentRepo) fillAssetCounts(ctx context.Context, dtos []*userV1.Department) {
	if len(dtos) == 0 {
		return
	}
	// v2.6 D1：徽标口径由「直属」改为「含下级部门」（管理部门口径），
	// 与列表点部门节点（含子孙筛选）保持一致，避免徽标数与筛选结果对不上。
	var tree []treePair
	if err := r.data.db.Client().Department.Query().
		Where(department.DeleteTimeIsNil()).
		Select(department.FieldID, department.FieldParentID).
		Scan(ctx, &tree); err != nil {
		r.log.Warnf("query department tree failed: %s", err.Error())
		return
	}
	children := buildChildrenMap(tree)
	direct := deptDirectAssetCounts(ctx, r.log, r.data, false)

	var fill func(items []*userV1.Department)
	fill = func(items []*userV1.Department) {
		for _, d := range items {
			sum := 0
			for _, id := range subtreeIDs(children, d.GetId()) {
				sum += direct[id]
			}
			if sum > 0 {
				c := int32(sum)
				d.AssetCount = &c
			}
			if len(d.Children) > 0 {
				fill(d.Children)
			}
		}
	}
	fill(dtos)
}

func (r *DepartmentRepo) travelChild(nodes []*userV1.Department, node *userV1.Department) bool {
	if nodes == nil {
		return false
	}

	if node.ParentId == nil {
		nodes = append(nodes, node)
		return true
	}

	for _, n := range nodes {
		if node.ParentId == nil {
			continue
		}

		if n.GetId() == node.GetParentId() {
			n.Children = append(n.Children, node)
			return true
		} else {
			if r.travelChild(n.Children, node) {
				return true
			}
		}
	}
	return false
}

func (r *DepartmentRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().Department.Query()
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

func (r *DepartmentRepo) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListDepartmentResponse, error) {
	if req == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Department.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), department.FieldCreateTime,
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

	sort.SliceStable(entities, func(i, j int) bool {
		if entities[j].ParentID == nil {
			return true
		}
		if entities[i].ParentID == nil {
			return true
		}
		return *entities[i].ParentID < *entities[j].ParentID
	})

	dtos := make([]*userV1.Department, 0, len(entities))
	for _, entity := range entities {
		if entity.ParentID == nil {
			dto := r.toDTO(entity)
			dtos = append(dtos, dto)
		}
	}
	for _, entity := range entities {
		if entity.ParentID != nil {
			dto := r.toDTO(entity)

			if r.travelChild(dtos, dto) {
				continue
			}

			dtos = append(dtos, dto)
		}
	}

	r.fillOrganizationNames(ctx, dtos)
	r.fillLeaderNames(ctx, dtos)
	r.fillAssetCounts(ctx, dtos)

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	ret := userV1.ListDepartmentResponse{
		Total: uint32(count),
		Items: dtos,
	}

	return &ret, err
}

func (r *DepartmentRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().Department.Query().
		Where(department.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, userV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *DepartmentRepo) Get(ctx context.Context, req *userV1.GetDepartmentRequest) (*userV1.Department, error) {
	if req == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Department.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorDepartmentNotFound("department not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	dto := r.toDTO(entity)
	r.fillOrganizationNames(ctx, []*userV1.Department{dto})
	r.fillLeaderNames(ctx, []*userV1.Department{dto})
	return dto, nil
}

func (r *DepartmentRepo) Create(ctx context.Context, req *userV1.CreateDepartmentRequest) error {
	if req == nil || req.Data == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Department.Create().
		SetNillableName(req.Data.Name).
		SetNillableParentID(req.Data.ParentId).
		SetNillableSortID(req.Data.SortId).
		SetNillableRemark(req.Data.Remark).
		SetNillableStatus((*department.Status)(req.Data.Status)).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableLeaderID(req.Data.LeaderId).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.OrganizationId != nil {
		builder.SetOrganizationID(uint32(req.Data.GetOrganizationId()))
	}

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return userV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *DepartmentRepo) Update(ctx context.Context, req *userV1.UpdateDepartmentRequest) error {
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
			createReq := &userV1.CreateDepartmentRequest{Data: req.Data}
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

	builder := r.data.db.Client().Department.
		UpdateOneID(id).
		SetNillableName(req.Data.Name).
		SetNillableParentID(req.Data.ParentId).
		SetNillableSortID(req.Data.SortId).
		SetNillableRemark(req.Data.Remark).
		SetNillableStatus((*department.Status)(req.Data.Status)).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableLeaderID(req.Data.LeaderId).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	if req.Data.OrganizationId != nil {
		builder.SetOrganizationID(uint32(req.Data.GetOrganizationId()))
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
		return userV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

func (r *DepartmentRepo) Delete(ctx context.Context, req *userV1.DeleteDepartmentRequest) error {
	if req == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().Department.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return userV1.ErrorNotFound("department not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return userV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
