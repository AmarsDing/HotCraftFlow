package biz

import (
	"context"
	"sort"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
)

// MenuRepo 菜单仓储端口，由 data 层 MenuRepo 实现并经 wire.Bind 绑定。
type MenuRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListMenuResponse, error)
	Get(ctx context.Context, req *adminV1.GetMenuRequest) (*adminV1.Menu, error)
	Create(ctx context.Context, req *adminV1.CreateMenuRequest) error
	Update(ctx context.Context, req *adminV1.UpdateMenuRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteMenuRequest) error
	IsExist(ctx context.Context, id int32) (bool, error)
	// CountAll 返回菜单总行数（seed 首初始化判空用）。
	CountAll(ctx context.Context) (int, error)
	// ResetSequence 把菜单 id 自增序列对齐到当前最大 id（seed 显式 id 插入不推进序列）。
	ResetSequence(ctx context.Context) error
	// ListByIDs 按菜单 ID 列表查询扁平菜单（仅 status=ON，按 meta.order 升序）。
	// onlyButton=true 仅查按钮（权限码用）；false 排除按钮（路由树用）。
	ListByIDs(ctx context.Context, ids []int32, onlyButton bool) ([]*adminV1.Menu, error)
	// ListAll 全量扁平查询（含 disabled，seed 菜单差量补齐的 path/id 占用比对用）。
	ListAll(ctx context.Context) ([]*adminV1.Menu, error)
	// BuildTree 把扁平菜单列表按 parent_id 组装为树形结构（纯函数，不触库）。
	BuildTree(menus []*adminV1.Menu) []*adminV1.Menu
	// CountChildren 统计直接子节点数量（删除保护/BUTTON 约束校验用）。
	CountChildren(ctx context.Context, id int32) (int, error)
	// IsNameExists 判断路由命名是否已被其他菜单占用（excludeId 为编辑态自身，创建态传 0）。
	IsNameExists(ctx context.Context, name string, excludeId int32) (bool, error)
	// IsSiblingPathExists 判断同父级下路由路径是否已被占用（根级 parentId 传 0）。
	IsSiblingPathExists(ctx context.Context, parentId int32, path string, excludeId int32) (bool, error)
	// GetImpact 统计菜单影响面：引用该菜单的角色数，以及经这些角色波及的去重用户数。
	GetImpact(ctx context.Context, id int32) (roleCount int, userCount int, err error)
	// ListRoleRefs 一次性查询全量角色-菜单引用关系（列表徽标 + 角色清单抽屉）。
	ListRoleRefs(ctx context.Context) ([]*adminV1.MenuRoleRef, error)
}

// MenuUsecase 菜单业务用例（树组装逻辑自 service 层下移，行为不变）。
type MenuUsecase struct {
	repo MenuRepo
	log  *log.Helper
}

func NewMenuUsecase(repo MenuRepo, logger log.Logger) *MenuUsecase {
	return &MenuUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "menu/usecase/admin-service")),
	}
}

// List 菜单管理列表：先按 meta.order 升序排序，再按 parent_id 组装树。
func (uc *MenuUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListMenuResponse, error) {
	ret, err := uc.repo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(ret.Items, func(i, j int) bool {
		return ret.Items[i].GetMeta().GetOrder() < ret.Items[j].GetMeta().GetOrder()
	})
	ret.Items = uc.repo.BuildTree(ret.Items)

	return ret, nil
}

func (uc *MenuUsecase) Get(ctx context.Context, req *adminV1.GetMenuRequest) (*adminV1.Menu, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *MenuUsecase) Create(ctx context.Context, req *adminV1.CreateMenuRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if err := uc.validate(ctx, 0, req.Data, nil); err != nil {
		return err
	}
	return uc.repo.Create(ctx, req)
}

func (uc *MenuUsecase) Update(ctx context.Context, req *adminV1.UpdateMenuRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	// 与 data 层同口径：优先取顶层 id（URL 路径参数），回退 data.id
	id := int32(req.GetId())
	if id == 0 {
		id = req.Data.GetId()
	}

	// AllowMissing 且不存在时走创建语义校验（data 层将执行插入）
	existing, err := uc.repo.Get(ctx, &adminV1.GetMenuRequest{Id: id})
	if err != nil {
		if req.GetAllowMissing() {
			if verr := uc.validate(ctx, 0, req.Data, nil); verr != nil {
				return verr
			}
			return uc.repo.Update(ctx, req)
		}
		return err
	}

	// 前端按 updateMask 局部提交（如状态开关仅携带 status），
	// 校验必须把补丁合并到存量实体上，避免对未提交字段误判为空。
	merged := mergeMenu(existing, req.Data)
	if verr := uc.validate(ctx, id, merged, existing); verr != nil {
		return verr
	}
	return uc.repo.Update(ctx, req)
}

// mergeMenu 把补丁 data 的非 nil 字段叠加到 existing 上，产出校验用的完整视图。
// 仅覆盖与结构校验相关的字段（meta/status 不影响结构规则，不叠加）。
func mergeMenu(existing, data *adminV1.Menu) *adminV1.Menu {
	out := &adminV1.Menu{
		Id:        existing.Id,
		ParentId:  existing.ParentId,
		Type:      existing.Type,
		Name:      existing.Name,
		Path:      existing.Path,
		Component: existing.Component,
		Redirect:  existing.Redirect,
	}
	if data.ParentId != nil {
		out.ParentId = data.ParentId
	}
	if data.Type != nil {
		out.Type = data.Type
	}
	if data.Name != nil {
		out.Name = data.Name
	}
	if data.Path != nil {
		out.Path = data.Path
	}
	if data.Component != nil {
		out.Component = data.Component
	}
	if data.Redirect != nil {
		out.Redirect = data.Redirect
	}
	return out
}

// validate 菜单结构校验（创建/更新统一入口）：
// 唯一性（路由命名、同级路径）、类型约束（FOLDER/MENU/BUTTON）、父级合法性与循环检测。
// id 为编辑态自身 id（创建态传 0）；existing 为更新前实体（创建态传 nil）。
func (uc *MenuUsecase) validate(ctx context.Context, id int32, data *adminV1.Menu, existing *adminV1.Menu) error {
	isCreate := existing == nil

	// 1. 路由命名必填 + 全局唯一（前端 addRoute 按 name 注册，重名会静默覆盖）
	name := data.GetName()
	if name == "" {
		return adminV1.ErrorBadRequest("路由命名不能为空")
	}
	if isCreate || name != existing.GetName() {
		exist, err := uc.repo.IsNameExists(ctx, name, id)
		if err != nil {
			return err
		}
		if exist {
			return adminV1.ErrorBadRequest("路由命名已存在：%s", name)
		}
	}

	// 2. 类型约束
	menuType := data.GetType()
	path := data.GetPath()
	switch menuType {
	case adminV1.MenuType_FOLDER:
		// 经典目录无组件，必须 redirect，否则动态路由只挂空 folder，点开会 404。
		// 容器目录（气象数据 341）有组件、无子页，侧栏靠前端注入分组，redirect 可空。
		if path == "" {
			return adminV1.ErrorBadRequest("目录必须填写路由路径")
		}
		if data.GetComponent() == "" && data.GetRedirect() == "" {
			return adminV1.ErrorBadRequest("目录必须填写重定向地址或前端组件")
		}
	case adminV1.MenuType_MENU:
		if path == "" {
			return adminV1.ErrorBadRequest("菜单必须填写路由路径")
		}
		// 未登记组件的菜单动态注册后回退 NotFoundPage，用户点击白屏
		if data.GetComponent() == "" {
			return adminV1.ErrorBadRequest("菜单必须填写前端组件")
		}
	case adminV1.MenuType_BUTTON:
		// 按钮不得拥有子节点（权限树叶子）；编辑态需检查既有子节点
		if !isCreate {
			children, err := uc.repo.CountChildren(ctx, id)
			if err != nil {
				return err
			}
			if children > 0 {
				return adminV1.ErrorBadRequest("按钮不能拥有子菜单，请先移除全部子节点")
			}
		}
	}

	// 3. 同级路径唯一（按钮无路径不参与；路径为空跳过）
	if menuType != adminV1.MenuType_BUTTON && path != "" {
		parentChanged := isCreate || data.GetParentId() != existing.GetParentId() || path != existing.GetPath()
		if parentChanged {
			exist, err := uc.repo.IsSiblingPathExists(ctx, data.GetParentId(), path, id)
			if err != nil {
				return err
			}
			if exist {
				return adminV1.ErrorBadRequest("同级下路由路径已存在：%s", path)
			}
		}
	}

	// 4. 父级合法性 + 循环检测（父级未变更时跳过，存量数据不受追溯影响）
	parentID := data.GetParentId()
	if parentID != 0 {
		parentDirty := isCreate || parentID != existing.GetParentId() || menuType != existing.GetType()
		if parentDirty {
			if err := uc.validateParent(ctx, id, parentID); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateParent 校验父菜单存在、非按钮，且沿父链上溯不成环。
// BuildTree 遇环会把整簇节点静默吞掉（菜单凭空消失无报错），必须在写入前拦截。
func (uc *MenuUsecase) validateParent(ctx context.Context, id int32, parentID int32) error {
	visited := map[int32]bool{}
	current := parentID
	for current != 0 {
		if current == id {
			return adminV1.ErrorBadRequest("父级选择不能形成循环")
		}
		if visited[current] {
			// 存量数据已存在不涉自身的环：放行本次写入，避免误伤
			return nil
		}
		visited[current] = true

		parent, err := uc.repo.Get(ctx, &adminV1.GetMenuRequest{Id: current})
		if err != nil {
			return adminV1.ErrorBadRequest("父菜单不存在")
		}
		if current == parentID && parent.GetType() == adminV1.MenuType_BUTTON {
			return adminV1.ErrorBadRequest("按钮不能作为父级菜单")
		}
		current = parent.GetParentId()
	}
	return nil
}

func (uc *MenuUsecase) Delete(ctx context.Context, req *adminV1.DeleteMenuRequest) error {
	return uc.repo.Delete(ctx, req)
}

// GetImpact 查询菜单影响面（删除/禁用前的前端二次确认数据）。
func (uc *MenuUsecase) GetImpact(ctx context.Context, req *adminV1.GetMenuRequest) (*adminV1.MenuImpactResponse, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	exist, err := uc.repo.IsExist(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, adminV1.ErrorNotFound("menu not found")
	}
	roleCount, userCount, err := uc.repo.GetImpact(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &adminV1.MenuImpactResponse{
		RoleCount: uint32(roleCount),
		UserCount: uint32(userCount),
	}, nil
}

func (uc *MenuUsecase) IsExist(ctx context.Context, id int32) (bool, error) {
	return uc.repo.IsExist(ctx, id)
}

// ListRoleRefs 查询全部菜单的角色引用关系（菜单管理页徽标 + 抽屉）。
func (uc *MenuUsecase) ListRoleRefs(ctx context.Context) (*adminV1.MenuRoleRefListResponse, error) {
	refs, err := uc.repo.ListRoleRefs(ctx)
	if err != nil {
		return nil, err
	}
	return &adminV1.MenuRoleRefListResponse{Items: refs}, nil
}

func (uc *MenuUsecase) CountAll(ctx context.Context) (int, error) {
	return uc.repo.CountAll(ctx)
}

func (uc *MenuUsecase) ResetSequence(ctx context.Context) error {
	return uc.repo.ResetSequence(ctx)
}

func (uc *MenuUsecase) ListByIDs(ctx context.Context, ids []int32, onlyButton bool) ([]*adminV1.Menu, error) {
	return uc.repo.ListByIDs(ctx, ids, onlyButton)
}

// ListAll 全量扁平查询（含 disabled；seed 差量补齐用，不组装树）。
func (uc *MenuUsecase) ListAll(ctx context.Context) ([]*adminV1.Menu, error) {
	return uc.repo.ListAll(ctx)
}

func (uc *MenuUsecase) BuildTree(menus []*adminV1.Menu) []*adminV1.Menu {
	return uc.repo.BuildTree(menus)
}
