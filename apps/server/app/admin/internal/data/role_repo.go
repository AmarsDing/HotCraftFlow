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
	"framework/app/admin/internal/data/ent/role"
	"framework/app/admin/internal/data/ent/roleapi"
	"framework/app/admin/internal/data/ent/rolemenu"
	"framework/app/admin/internal/data/ent/userrole"

	userV1 "framework/api/gen/go/user/v1"
)

type RoleRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[userV1.Role, ent.Role]
}

func NewRoleRepo(data *Data, logger log.Logger) *RoleRepo {
	repo := &RoleRepo{
		log:    log.NewHelper(log.With(logger, "module", "role/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[userV1.Role, ent.Role](),
	}

	repo.init()

	return repo
}

func (r *RoleRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

func (r *RoleRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().Role.Query()
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

func (r *RoleRepo) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListRoleResponse, error) {
	if req == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Role.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), role.FieldCreateTime,
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

	dtos := make([]*userV1.Role, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &userV1.ListRoleResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

func (r *RoleRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().Role.Query().
		Where(role.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, userV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

// fillRoleMenus 查询 ap_org_role_menu 表填充 role.Menus（菜单 ID 列表）。
func (r *RoleRepo) fillRoleMenus(ctx context.Context, roleID uint32, dto *userV1.Role) {
	if roleID == 0 || dto == nil {
		return
	}
	entities, err := r.data.db.Client().RoleMenu.Query().
		Where(rolemenu.RoleIDEQ(roleID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query role menus failed [%d]: %s", roleID, err.Error())
		return
	}
	menus := make([]uint32, 0, len(entities))
	for _, e := range entities {
		menus = append(menus, e.MenuID)
	}
	dto.Menus = menus
}

// saveRoleMenus 全量替换角色的菜单关联（先删后插），用于 Create/Update。
// 使用事务保证删除/插入原子性，避免中途失败导致角色丢失所有菜单。
func (r *RoleRepo) saveRoleMenus(ctx context.Context, roleID uint32, menuIDs []uint32) error {
	if roleID == 0 {
		return nil
	}
	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed [%d]: %s", roleID, err.Error())
		return userV1.ErrorInternalServerError("save role menus failed")
	}
	// 先删除旧关联
	if _, err := tx.RoleMenu.Delete().
		Where(rolemenu.RoleIDEQ(roleID)).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete role menus failed [%d]: %s", roleID, err.Error())
		return userV1.ErrorInternalServerError("save role menus failed")
	}
	// 批量插入新关联
	builders := make([]*ent.RoleMenuCreate, 0, len(menuIDs))
	for _, mid := range menuIDs {
		if mid == 0 {
			continue
		}
		builders = append(builders,
			tx.RoleMenu.Create().
				SetRoleID(roleID).
				SetMenuID(mid),
		)
	}
	if len(builders) > 0 {
		if err := tx.RoleMenu.CreateBulk(builders...).Exec(ctx); err != nil {
			_ = tx.Rollback()
			r.log.Errorf("bulk insert role menus failed [%d]: %s", roleID, err.Error())
			return userV1.ErrorInternalServerError("save role menus failed")
		}
	}
	if err := tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed [%d]: %s", roleID, err.Error())
		return userV1.ErrorInternalServerError("save role menus failed")
	}
	return nil
}

// EnsureMenus 全量同步角色的菜单关联，供 seed 对已存在角色补建菜单。
func (r *RoleRepo) EnsureMenus(ctx context.Context, roleID uint32, menuIDs []uint32) error {
	return r.saveRoleMenus(ctx, roleID, menuIDs)
}

// fillRoleApis 查询 ap_org_role_api 表填充 role.Apis（API 资源 ID 列表）。
func (r *RoleRepo) fillRoleApis(ctx context.Context, roleID uint32, dto *userV1.Role) {
	if roleID == 0 || dto == nil {
		return
	}
	entities, err := r.data.db.Client().RoleApi.Query().
		Where(roleapi.RoleIDEQ(roleID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query role apis failed [%d]: %s", roleID, err.Error())
		return
	}
	apis := make([]uint32, 0, len(entities))
	for _, e := range entities {
		apis = append(apis, e.APIID)
	}
	dto.Apis = apis
}

// saveRoleApis 全量替换角色的 API 资源关联（先删后插），用于 Create/Update。
// 使用事务保证删除/插入原子性，避免中途失败导致角色丢失所有 API 授权。
func (r *RoleRepo) saveRoleApis(ctx context.Context, roleID uint32, apiIDs []uint32) error {
	if roleID == 0 {
		return nil
	}
	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed [%d]: %s", roleID, err.Error())
		return userV1.ErrorInternalServerError("save role apis failed")
	}
	// 先删除旧关联
	if _, err := tx.RoleApi.Delete().
		Where(roleapi.RoleIDEQ(roleID)).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete role apis failed [%d]: %s", roleID, err.Error())
		return userV1.ErrorInternalServerError("save role apis failed")
	}
	// 批量插入新关联
	builders := make([]*ent.RoleApiCreate, 0, len(apiIDs))
	for _, aid := range apiIDs {
		if aid == 0 {
			continue
		}
		builders = append(builders,
			tx.RoleApi.Create().
				SetRoleID(roleID).
				SetAPIID(aid),
		)
	}
	if len(builders) > 0 {
		if err := tx.RoleApi.CreateBulk(builders...).Exec(ctx); err != nil {
			_ = tx.Rollback()
			r.log.Errorf("bulk insert role apis failed [%d]: %s", roleID, err.Error())
			return userV1.ErrorInternalServerError("save role apis failed")
		}
	}
	if err := tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed [%d]: %s", roleID, err.Error())
		return userV1.ErrorInternalServerError("save role apis failed")
	}
	return nil
}

// EnsureApis 全量同步角色的 API 资源关联，供 seed 对已存在角色补建 API 授权。
func (r *RoleRepo) EnsureApis(ctx context.Context, roleID uint32, apiIDs []uint32) error {
	return r.saveRoleApis(ctx, roleID, apiIDs)
}

// ListAllRoleApiBindings 返回全量 角色ID→API资源ID列表 映射，供 Authorizer 全量生成策略。
func (r *RoleRepo) ListAllRoleApiBindings(ctx context.Context) (map[uint32][]uint32, error) {
	entities, err := r.data.db.Client().RoleApi.Query().All(ctx)
	if err != nil {
		r.log.Errorf("query all role api bindings failed: %s", err.Error())
		return nil, userV1.ErrorInternalServerError("query role api bindings failed")
	}
	bindings := make(map[uint32][]uint32, len(entities))
	for _, e := range entities {
		bindings[e.RoleID] = append(bindings[e.RoleID], e.APIID)
	}
	return bindings, nil
}

// EnsureUserRole 创建用户-角色关联（如不存在），供 seed 补建 admin 及 seed 用户的角色绑定。
func (r *RoleRepo) EnsureUserRole(ctx context.Context, userID, roleID uint32) error {
	if userID == 0 || roleID == 0 {
		return nil
	}
	// 幂等：已存在则跳过
	exist, err := r.data.db.Client().UserRole.Query().
		Where(userrole.UserIDEQ(userID), userrole.RoleIDEQ(roleID)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("query user role failed [%d -> %d]: %s", userID, roleID, err.Error())
		return userV1.ErrorInternalServerError("ensure user role failed")
	}
	if exist != nil {
		return nil // 已存在
	}
	return r.data.db.Client().UserRole.Create().
		SetUserID(userID).
		SetRoleID(roleID).
		Exec(ctx)
}

func (r *RoleRepo) Get(ctx context.Context, id uint32) (*userV1.Role, error) {
	if id == 0 {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Role.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorRoleNotFound("role not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	dto := r.mapper.ToDTO(entity)
	r.fillRoleMenus(ctx, id, dto)
	r.fillRoleApis(ctx, id, dto)
	return dto, nil
}

func (r *RoleRepo) GetRoleByCode(ctx context.Context, code string) (*userV1.Role, error) {
	if code == "" {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Role.Query().
		Where(role.CodeEQ(code)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorRoleNotFound("role not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	dto := r.mapper.ToDTO(entity)
	r.fillRoleMenus(ctx, entity.ID, dto)
	r.fillRoleApis(ctx, entity.ID, dto)
	return dto, nil
}

func (r *RoleRepo) Create(ctx context.Context, req *userV1.CreateRoleRequest) error {
	if req == nil || req.Data == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Role.Create().
		SetNillableName(req.Data.Name).
		SetNillableParentID(req.Data.ParentId).
		SetNillableSortID(req.Data.SortId).
		SetNillableCode(req.Data.Code).
		SetNillableStatus((*role.Status)(req.Data.Status)).
		SetNillableRemark(req.Data.Remark).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	// Save() 返回创建的 entity 以获取自增 ID（用于关联表写入）
	entity, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return userV1.ErrorInternalServerError("insert data failed")
	}

	// 保存角色-菜单关联
	if len(req.Data.Menus) > 0 {
		if err := r.saveRoleMenus(ctx, entity.ID, req.Data.Menus); err != nil {
			return err
		}
	}

	// 保存角色-API 资源关联
	if len(req.Data.Apis) > 0 {
		if err := r.saveRoleApis(ctx, entity.ID, req.Data.Apis); err != nil {
			return err
		}
	}

	return nil
}

func (r *RoleRepo) Update(ctx context.Context, req *userV1.UpdateRoleRequest) error {
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
			createReq := &userV1.CreateRoleRequest{Data: req.Data}
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

	builder := r.data.db.Client().Role.UpdateOneID(id).
		SetNillableName(req.Data.Name).
		SetNillableParentID(req.Data.ParentId).
		SetNillableSortID(req.Data.SortId).
		SetNillableCode(req.Data.Code).
		SetNillableRemark(req.Data.Remark).
		SetNillableStatus((*role.Status)(req.Data.Status)).
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
		return userV1.ErrorInternalServerError("update data failed")
	}

	// 更新角色-菜单关联（menus 不在 fieldmask 管控范围，只要请求携带就全量替换）
	if req.Data.Menus != nil {
		if err := r.saveRoleMenus(ctx, id, req.Data.Menus); err != nil {
			return err
		}
	}

	// 更新角色-API 资源关联（apis 同理：携带即全量替换，空数组表示清空）
	if req.Data.Apis != nil {
		if err := r.saveRoleApis(ctx, id, req.Data.Apis); err != nil {
			return err
		}
	}

	return nil
}

func (r *RoleRepo) Delete(ctx context.Context, req *userV1.DeleteRoleRequest) error {
	if req == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().Role.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return userV1.ErrorNotFound("role not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return userV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
