package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/tx7do/go-utils/copierutil"
	"github.com/tx7do/go-utils/entgo/query"
	entgoUpdate "github.com/tx7do/go-utils/entgo/update"
	"github.com/tx7do/go-utils/fieldmaskutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/menu"
	"framework/app/admin/internal/data/ent/rolemenu"
	"framework/app/admin/internal/data/ent/userrole"

	adminV1 "framework/api/gen/go/admin/v1"
)

type MenuRepo struct {
	data *Data
	log  *log.Helper

	mapper          *mapper.CopierMapper[adminV1.Menu, ent.Menu]
	statusConverter *mapper.EnumTypeConverter[adminV1.MenuStatus, menu.Status]
	typeConverter   *mapper.EnumTypeConverter[adminV1.MenuType, menu.Type]
}

func NewMenuRepo(data *Data, logger log.Logger) *MenuRepo {
	repo := &MenuRepo{
		log:             log.NewHelper(log.With(logger, "module", "menu/repo/admin-service")),
		data:            data,
		mapper:          mapper.NewCopierMapper[adminV1.Menu, ent.Menu](),
		statusConverter: mapper.NewEnumTypeConverter[adminV1.MenuStatus, menu.Status](adminV1.MenuStatus_name, adminV1.MenuStatus_value),
		typeConverter:   mapper.NewEnumTypeConverter[adminV1.MenuType, menu.Type](adminV1.MenuType_name, adminV1.MenuType_value),
	}

	repo.init()

	return repo
}

func (r *MenuRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
	r.mapper.AppendConverters(r.typeConverter.NewConverterPair())
}

func (r *MenuRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().Menu.Query()
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

// CountAll 返回菜单总行数（seed 首初始化判空用：空表才播种基线菜单）。
func (r *MenuRepo) CountAll(ctx context.Context) (int, error) {
	count, err := r.data.db.Client().Menu.Query().Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

// ResetSequence 把自增序列对齐到当前最大 id。
// seed 按显式 id 插入不推进序列，滞后会导致菜单管理页新建菜单撞主键（同 api_resource 先例）；
// 孤儿 ID 段被序列复用后还会被 seed 清理误删。幂等无损，每次启动执行。
func (r *MenuRepo) ResetSequence(ctx context.Context) error {
	if _, err := r.data.db.DB().ExecContext(ctx,
		"SELECT setval('menu_permissions_id_seq', COALESCE((SELECT MAX(id) FROM menu_permissions), 1))"); err != nil {
		r.log.Errorf("reset menu sequence failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("reset sequence failed")
	}
	return nil
}

func (r *MenuRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListMenuResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Menu.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), menu.FieldCreateTime,
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

	dtos := make([]*adminV1.Menu, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListMenuResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

// ListByIDs 按菜单 ID 列表查询扁平菜单，仅返回 status=ON 的菜单，结果按 meta.order 升序排序。
// onlyButton=true 时仅查 type=BUTTON 的菜单（用于权限码）；false 时排除 BUTTON（用于路由树）。
func (r *MenuRepo) ListByIDs(ctx context.Context, ids []int32, onlyButton bool) ([]*adminV1.Menu, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	builder := r.data.db.Client().Menu.Query().
		Where(menu.IDIn(ids...)).
		Where(menu.StatusEQ(menu.StatusOn))

	if onlyButton {
		builder = builder.Where(menu.TypeEQ(menu.TypeButton))
	} else {
		builder = builder.Where(menu.TypeNEQ(menu.TypeButton))
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list by ids failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*adminV1.Menu, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}

	// 按 meta.order 升序排序，order 缺省视为 0，保证菜单顺序由业务定义而非插入时间决定
	sort.SliceStable(dtos, func(i, j int) bool {
		return dtos[i].GetMeta().GetOrder() < dtos[j].GetMeta().GetOrder()
	})

	return dtos, nil
}

// ListAll 全量扁平查询（含 disabled，供 seed 菜单差量补齐做 path/id 占用比对；
// 不做分页——菜单行数量级百级，一次全取）。
func (r *MenuRepo) ListAll(ctx context.Context) ([]*adminV1.Menu, error) {
	entities, err := r.data.db.Client().Menu.Query().All(ctx)
	if err != nil {
		r.log.Errorf("query all menus failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query all menus failed")
	}
	dtos := make([]*adminV1.Menu, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.mapper.ToDTO(entity))
	}
	return dtos, nil
}

// BuildTree 把扁平菜单列表按 parent_id 组装为树形结构。
// 入参需已按展示顺序排好（如 ListByIDs 已按 meta.order 排序），
// 同一父节点下子菜单按切片顺序追加，根节点（parent_id=0）作为顶层返回。
func (r *MenuRepo) BuildTree(menus []*adminV1.Menu) []*adminV1.Menu {
	if len(menus) == 0 {
		return nil
	}

	byID := make(map[int32]*adminV1.Menu, len(menus))
	for _, m := range menus {
		byID[m.GetId()] = m
	}

	roots := make([]*adminV1.Menu, 0)
	for _, m := range menus {
		parentID := m.GetParentId()
		if parentID == 0 {
			roots = append(roots, m)
			continue
		}
		if parent, ok := byID[parentID]; ok {
			parent.Children = append(parent.Children, m)
		} else {
			// 父节点不在本次结果集（可能被权限过滤），作为根返回避免丢失
			roots = append(roots, m)
		}
	}
	return roots
}

func (r *MenuRepo) IsExist(ctx context.Context, id int32) (bool, error) {
	exist, err := r.data.db.Client().Menu.Query().
		Where(menu.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *MenuRepo) Get(ctx context.Context, req *adminV1.GetMenuRequest) (*adminV1.Menu, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Menu.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("menu not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *MenuRepo) Create(ctx context.Context, req *adminV1.CreateMenuRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().Menu.Create().
		SetNillableParentID(req.Data.ParentId).
		SetNillableType(r.typeConverter.ToEntity(req.Data.Type)).
		SetNillablePath(req.Data.Path).
		SetNillableRedirect(req.Data.Redirect).
		SetNillableAlias(req.Data.Alias).
		SetNillableName(req.Data.Name).
		SetNillableComponent(req.Data.Component).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Meta != nil {
		builder.SetMeta(req.Data.Meta)
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

func (r *MenuRepo) Update(ctx context.Context, req *adminV1.UpdateMenuRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	// 优先取顶层 id（来自 URL 路径），回退 data.id。
	// 必须在 fieldmaskutil.Filter 之前捕获：Filter 会清空不在 updateMask 中的字段（含 data.id）。
	// 注意：Menu.id 为 int32，而 req.id 为 uint32，需要类型转换。
	id := int32(req.GetId())
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
			createReq := &adminV1.CreateMenuRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			return r.Create(ctx, createReq)
		}
	}

	wholeMeta, metaPaths := classifyMetaMask(req.UpdateMask)
	if req.UpdateMask != nil {
		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return adminV1.ErrorBadRequest("invalid field mask")
		}
		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().
		//Debug().
		Menu.UpdateOneID(id).
		SetNillableParentID(req.Data.ParentId).
		SetNillableType(r.typeConverter.ToEntity(req.Data.Type)).
		SetNillablePath(req.Data.Path).
		SetNillableRedirect(req.Data.Redirect).
		SetNillableAlias(req.Data.Alias).
		SetNillableName(req.Data.Name).
		SetNillableComponent(req.Data.Component).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	if req.Data.UpdateTime == nil {
		builder.SetUpdateTime(time.Now())
	}

	if req.Data.Meta != nil {
		if wholeMeta {
			// 表单始终提交完整 meta 对象，整体替换即"所见即所存"
			builder.SetMeta(req.Data.Meta)
		} else {
			r.updateMetaField(builder, req.Data.Meta, metaPaths)
		}
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

// classifyMetaMask 把 updateMask 路径分类为 meta 更新口径：
// wholeMeta=true —— 整体替换 meta（标准 field mask 语义：消息字段路径即整体替换；
// 无 mask 视为全量更新，同属整体替换）；
// 否则 metaPaths 为细粒度 meta.* 子字段名（进程内调用方如 seed upsert 使用）。
// 注意：HTTP 调用方只能携带整体 "meta" —— RouteMeta 字段为 camelCase 声明，
// protojson 解码 FieldMask 会把 meta.hideInMenu 转成 meta.hide_in_menu 被 IsValid 拒绝，
// 前端 buildUpdateMask 也只产出顶层键；若只认 meta.*，表单对 title/icon/order/
// authority/hideInMenu 的修改会被静默丢弃（菜单改名不生效的根因）。
func classifyMetaMask(fm *fieldmaskpb.FieldMask) (wholeMeta bool, metaPaths []string) {
	if fm == nil {
		return true, nil
	}
	for _, v := range fm.GetPaths() {
		if v == "meta" {
			wholeMeta = true
			continue
		}
		if strings.HasPrefix(v, "meta.") {
			metaPaths = append(metaPaths, strings.SplitAfter(v, "meta.")[1])
		}
	}
	return wholeMeta, metaPaths
}

func (r *MenuRepo) updateMetaField(builder *ent.MenuUpdateOne, meta *adminV1.RouteMeta, metaPaths []string) {
	// tx7do ExtractJsonFieldKeyValues 不支持 repeated 字段（只追加 key 不追加 value，
	// jsonb_build_object 参数个数为奇数 → pq 22023），authority 必须剥离单独处理。
	// 同函数对 bool 用 %d 格式化产出 %!d(bool=...) 非法 SQL（pq 42703），bool 路径同样剥离单独处理。
	scalarPaths := make([]string, 0, len(metaPaths))
	boolPaths := make([]string, 0, len(metaPaths))
	hasAuthority := false
	for _, p := range metaPaths {
		if p == "authority" {
			hasAuthority = true
			continue
		}
		if fd := meta.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(p)); fd != nil && fd.Kind() == protoreflect.BoolKind {
			boolPaths = append(boolPaths, p)
			continue
		}
		scalarPaths = append(scalarPaths, p)
	}

	// ent UpdateBuilder.Set 同列后写覆盖前写：nil 键删除 / 标量 merge / authority 增删
	// 若拆成多次 Set(meta,...) 会相互覆盖（原实现 authority 删除器把 title/icon/order 的
	// merge 整体吞掉，存量菜单 meta 标量更新静默失效），必须合成单个表达式一次 Set。
	expr := `"` + menu.FieldMeta + `"`
	changed := false

	// 1. 空值标量键删除（proto 字段未赋值 → 剔除该键；bool 路径一并参与空值判定）
	if nilPaths := fieldmaskutil.NilValuePaths(meta, append(scalarPaths, boolPaths...)); len(nilPaths) > 0 {
		expr += fmt.Sprintf(" - '{%s}'::text[]", strings.Join(nilPaths, ","))
		changed = true
	}
	// 2. 非空标量键值 merge（ExtractJsonFieldKeyValues 跳过未赋值字段，参数个数恒为偶数）
	if keyValues := entgoUpdate.ExtractJsonFieldKeyValues(meta, scalarPaths, false); len(keyValues) > 0 {
		expr += fmt.Sprintf(" || jsonb_build_object(%s)", strings.Join(keyValues, ","))
		changed = true
	}
	// 3. authority：非空数组合并为 jsonb，空数组删除该键（元素 json.Marshal 序列化并 doubling 单引号防注入）
	if hasAuthority {
		if authority := meta.GetAuthority(); len(authority) > 0 {
			if raw, err := json.Marshal(authority); err == nil {
				expr += fmt.Sprintf(" || jsonb_build_object('authority', '%s'::jsonb)", strings.ReplaceAll(string(raw), "'", "''"))
				changed = true
			}
		} else {
			expr += ` - 'authority'`
			changed = true
		}
	}
	// 4. bool 标量（hideInMenu 等）：非空指针序列化为 jsonb bool；nil 已由步骤 1 剔除键
	rfl := meta.ProtoReflect()
	for _, p := range boolPaths {
		fd := rfl.Descriptor().Fields().ByName(protoreflect.Name(p))
		if fd == nil || !rfl.Has(fd) {
			continue
		}
		expr += fmt.Sprintf(" || jsonb_build_object('%s', '%t'::jsonb)", p, rfl.Get(fd).Bool())
		changed = true
	}

	if !changed {
		return
	}
	builder.Modify(func(u *sql.UpdateBuilder) {
		u.Set(menu.FieldMeta, sql.Expr(expr))
	})
}

// CountChildren 统计直接子节点数量（删除保护/BUTTON 约束校验共用）。
func (r *MenuRepo) CountChildren(ctx context.Context, id int32) (int, error) {
	count, err := r.data.db.Client().Menu.Query().
		Where(menu.ParentIDEQ(id)).
		Count(ctx)
	if err != nil {
		r.log.Errorf("query child menu count failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("query child menu count failed")
	}
	return count, nil
}

// IsNameExists 判断路由命名是否已被其他菜单占用（excludeId 为编辑态自身，创建态传 0）。
func (r *MenuRepo) IsNameExists(ctx context.Context, name string, excludeId int32) (bool, error) {
	exist, err := r.data.db.Client().Menu.Query().
		Where(menu.NameEQ(name)).
		Where(menu.IDNEQ(excludeId)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query menu name failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query menu name failed")
	}
	return exist, nil
}

// IsSiblingPathExists 判断同父级下路由路径是否已被占用（根级 parentId 传 0）。
// 根节点的 parent_id 存 NULL（SetNillableParentID 不携带即空），历史数据可能落 0，两种都视为根。
func (r *MenuRepo) IsSiblingPathExists(ctx context.Context, parentId int32, path string, excludeId int32) (bool, error) {
	builder := r.data.db.Client().Menu.Query().
		Where(menu.PathEQ(path)).
		Where(menu.IDNEQ(excludeId))
	if parentId == 0 {
		builder = builder.Where(menu.Or(menu.ParentIDIsNil(), menu.ParentIDEQ(0)))
	} else {
		builder = builder.Where(menu.ParentIDEQ(parentId))
	}
	exist, err := builder.Exist(ctx)
	if err != nil {
		r.log.Errorf("query sibling path failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query sibling path failed")
	}
	return exist, nil
}

// ListRoleRefs 一次性查询全量角色-菜单关联，按 menu_id 分组返回角色ID列表。
// 用于前端菜单列表行徽标（引用角色数）和抽屉（角色清单）。
func (r *MenuRepo) ListRoleRefs(ctx context.Context) ([]*adminV1.MenuRoleRef, error) {
	roleMenus, err := r.data.db.Client().RoleMenu.Query().All(ctx)
	if err != nil {
		r.log.Errorf("query role menu refs failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query role menu refs failed")
	}

	byMenu := make(map[uint32][]uint32, len(roleMenus))
	seen := make(map[string]bool, len(roleMenus))
	for _, rm := range roleMenus {
		key := fmt.Sprintf("%d-%d", rm.MenuID, rm.RoleID)
		if seen[key] {
			continue
		}
		seen[key] = true
		byMenu[rm.MenuID] = append(byMenu[rm.MenuID], rm.RoleID)
	}

	refs := make([]*adminV1.MenuRoleRef, 0, len(byMenu))
	for menuID, roleIDs := range byMenu {
		refs = append(refs, &adminV1.MenuRoleRef{
			MenuId:  menuID,
			RoleIds: roleIDs,
		})
	}
	return refs, nil
}

// GetImpact 统计菜单影响面：引用该菜单的角色数（ap_org_role_menu），
// 以及经这些角色波及的去重用户数（ap_org_user_role）。角色/用户量级小，内存去重即可。
func (r *MenuRepo) GetImpact(ctx context.Context, id int32) (roleCount int, userCount int, err error) {
	roleMenus, err := r.data.db.Client().RoleMenu.Query().
		Where(rolemenu.MenuIDEQ(uint32(id))).
		All(ctx)
	if err != nil {
		r.log.Errorf("query role menu refs failed: %s", err.Error())
		return 0, 0, adminV1.ErrorInternalServerError("query role menu refs failed")
	}

	roleIDs := make([]uint32, 0, len(roleMenus))
	seenRole := make(map[uint32]bool, len(roleMenus))
	for _, rm := range roleMenus {
		if seenRole[rm.RoleID] {
			continue
		}
		seenRole[rm.RoleID] = true
		roleIDs = append(roleIDs, rm.RoleID)
	}
	if len(roleIDs) == 0 {
		return 0, 0, nil
	}

	userRoles, err := r.data.db.Client().UserRole.Query().
		Where(userrole.RoleIDIn(roleIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query user role refs failed: %s", err.Error())
		return 0, 0, adminV1.ErrorInternalServerError("query user role refs failed")
	}
	seenUser := make(map[uint32]bool, len(userRoles))
	for _, ur := range userRoles {
		seenUser[ur.UserID] = true
	}

	return len(roleIDs), len(seenUser), nil
}

func (r *MenuRepo) Delete(ctx context.Context, req *adminV1.DeleteMenuRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	// 检查是否存在子菜单，有则拒绝删除以避免产生孤儿节点（parent_id 指向已删除节点）
	childCount, err := r.CountChildren(ctx, req.GetId())
	if err != nil {
		return err
	}
	if childCount > 0 {
		return adminV1.ErrorBadRequest("请先删除子菜单后再删除当前菜单")
	}

	// 事务内同步清理角色-菜单关联（ap_org_role_menu），
	// 否则角色残留失效菜单 id 成为脏数据（角色页勾选计数偏移、影响面统计失真）。
	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	if _, err = tx.RoleMenu.Delete().
		Where(rolemenu.MenuIDEQ(uint32(req.GetId()))).
		Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete role menu refs failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	if err = tx.Menu.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("menu not found")
		}
		r.log.Errorf("delete one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}
	if err = tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
