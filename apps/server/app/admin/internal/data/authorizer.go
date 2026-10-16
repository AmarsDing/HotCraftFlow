package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"

	authzEngine "github.com/tx7do/kratos-authz/engine"
	"github.com/tx7do/kratos-authz/engine/casbin"
	"github.com/tx7do/kratos-authz/engine/noop"

	conf "framework/app/admin/internal/conf"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	userV1 "framework/api/gen/go/user/v1"
)

// Authorizer 封装 Casbin 授权引擎。
// 策略来源链路：角色 → ap_org_role_api → api_resource（仅角色直绑；资源组间接链路已下线）
// 展开后生成 (roleCode, apiPath, method) 策略，加载到内存 enforcer 并持久化到 casbin_rule 表。
// 仅 status 启用且未标记 delisted 的资源进入策略（见 ListAllBriefs）。
type Authorizer struct {
	log *log.Helper

	roleRepo        *RoleRepo
	apiResourceRepo *ApiResourceRepo
	casbinRuleRepo  *CasbinRuleRepo

	engine authzEngine.Engine
}

func NewAuthorizer(
	logger log.Logger,
	cfg *conf.Bootstrap,
	roleRepo *RoleRepo,
	apiResourceRepo *ApiResourceRepo,
	casbinRuleRepo *CasbinRuleRepo,
) *Authorizer {
	a := &Authorizer{
		log:             log.NewHelper(log.With(logger, "module", "authorizer/repo/admin-service")),
		roleRepo:        roleRepo,
		apiResourceRepo: apiResourceRepo,
		casbinRuleRepo:  casbinRuleRepo,
	}

	a.init(cfg)

	return a
}

func (a *Authorizer) init(cfg *conf.Bootstrap) {
	a.engine = a.newEngine(cfg)

	if err := a.ResetPolicies(context.Background()); err != nil {
		a.log.Errorf("reset policies error: %v", err)
	}
}

func (a *Authorizer) newEngine(cfg *conf.Bootstrap) authzEngine.Engine {
	if cfg.GetAuth() == nil || cfg.GetAuth().GetAuthz() == nil {
		return nil
	}

	ctx := context.Background()

	switch cfg.GetAuth().GetAuthz().GetType() {
	default:
		fallthrough
	case "noop":
		state, err := noop.NewEngine(ctx)
		if err != nil {
			a.log.Errorf("new noop engine error: %v", err)
			return nil
		}
		return state

	case "casbin":
		state, err := casbin.NewEngine(ctx,
			casbin.WithDefaultModel("restfull_with_role"),
		)
		if err != nil {
			a.log.Errorf("init casbin engine error: %v", err)
			return nil
		}
		return state
	}
}

func (a *Authorizer) Engine() authzEngine.Engine {
	return a.engine
}

// ResetPolicies 全量重建策略：从关系表生成 → 加载内存 enforcer → 持久化 casbin_rule 表。
// 启动时调用；资源组/API 资源变更时调用。
func (a *Authorizer) ResetPolicies(ctx context.Context) error {
	rules, err := a.generateAllPolicies(ctx)
	if err != nil {
		return err
	}

	if err := a.loadIntoEngine(ctx, rules); err != nil {
		return err
	}

	if err := a.persistToDB(ctx, rules); err != nil {
		// 持久化失败不阻断鉴权（内存已加载），仅记录日志
		a.log.Errorf("persist casbin rules to db failed (non-fatal): %v", err)
	}

	a.log.Infof("reset policies done: %d rules loaded", len(rules))
	return nil
}

// SyncPolicies 增量同步单个角色的策略：更新 casbin_rule 表中该角色的行，然后全量重载内存 enforcer。
// 角色绑定/解绑 API 资源时调用。
func (a *Authorizer) SyncPolicies(ctx context.Context, roleID uint32) error {
	role, err := a.roleRepo.Get(ctx, roleID)
	if err != nil {
		return fmt.Errorf("get role %d failed: %w", roleID, err)
	}
	if role == nil || role.GetCode() == "" {
		return nil
	}

	rules, err := a.generateRolePolicies(ctx, role)
	if err != nil {
		return err
	}

	// 更新 DB：先删后插
	if _, err := a.casbinRuleRepo.DeleteBySubject(ctx, role.GetCode()); err != nil {
		return fmt.Errorf("delete old policies for role %s failed: %w", role.GetCode(), err)
	}
	if err := a.casbinRuleRepo.BulkInsert(ctx, rules); err != nil {
		return fmt.Errorf("insert new policies for role %s failed: %w", role.GetCode(), err)
	}

	// 全量重载内存 enforcer（简单可靠，策略量不大时性能可接受）
	allRules, err := a.generateAllPolicies(ctx)
	if err != nil {
		return err
	}
	if err := a.loadIntoEngine(ctx, allRules); err != nil {
		return err
	}

	a.log.Infof("sync policies for role %s done: %d rules", role.GetCode(), len(rules))
	return nil
}

// DeletePolicies 删除角色的全部策略（角色删除时调用）。
func (a *Authorizer) DeletePolicies(ctx context.Context, roleCode string) error {
	if _, err := a.casbinRuleRepo.DeleteBySubject(ctx, roleCode); err != nil {
		return err
	}
	// 全量重载内存 enforcer
	allRules, err := a.generateAllPolicies(ctx)
	if err != nil {
		return err
	}
	return a.loadIntoEngine(ctx, allRules)
}

// generateAllPolicies 从关系表全量生成策略。
func (a *Authorizer) generateAllPolicies(ctx context.Context) ([]CasbinRuleRow, error) {
	roles, err := a.roleRepo.List(ctx, &pagination.PagingRequest{NoPaging: trans.Ptr(true)})
	if err != nil {
		return nil, fmt.Errorf("list roles failed: %w", err)
	}
	if roles == nil || len(roles.Items) == 0 {
		a.log.Warnf("no roles found to set policies")
		return nil, nil
	}

	briefs, err := a.apiResourceRepo.ListAllBriefs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api resource briefs failed: %w", err)
	}
	if len(briefs) == 0 {
		a.log.Warnf("no api resources found to set policies")
		return nil, nil
	}

	// 角色-API 直接绑定（角色编辑 API 权限 tab 配置），接口授权的唯一来源
	apiBindings, err := a.roleRepo.ListAllRoleApiBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list role api bindings failed: %w", err)
	}

	apiByID := briefsByID(briefs)

	var rules []CasbinRuleRow
	for _, role := range roles.Items {
		if role.GetId() == 0 || role.GetCode() == "" {
			continue
		}
		directAPIIDs := apiBindings[role.GetId()]
		rules = append(rules, expandRolePolicies(role, directAPIIDs, apiByID)...)
	}
	return rules, nil
}

// generateRolePolicies 生成单个角色的策略。
func (a *Authorizer) generateRolePolicies(ctx context.Context, role *userV1.Role) ([]CasbinRuleRow, error) {
	briefs, err := a.apiResourceRepo.ListAllBriefs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list api resource briefs failed: %w", err)
	}
	if len(briefs) == 0 {
		return nil, nil
	}

	apiByID := briefsByID(briefs)
	// role 经 roleRepo.Get 查询，Apis 已由 fillRoleApis 填充
	return expandRolePolicies(role, role.GetApis(), apiByID), nil
}

// loadIntoEngine 将策略加载到内存 enforcer。
func (a *Authorizer) loadIntoEngine(ctx context.Context, rules []CasbinRuleRow) error {
	if a.engine == nil {
		return errors.New("authz engine not initialized")
	}
	if a.engine.Name() != "casbin" {
		a.log.Warnf("engine %s is not casbin, skip loading policies", a.engine.Name())
		return nil
	}

	policyRules := make([]casbin.PolicyRule, 0, len(rules))
	for _, r := range rules {
		policyRules = append(policyRules, casbin.PolicyRule{
			PType: r.PType,
			V0:    r.V0,
			V1:    r.V1,
			V2:    r.V2,
			V3:    r.V3,
			V4:    r.V4,
			V5:    r.V5,
		})
	}

	policies := authzEngine.PolicyMap{
		"policies": policyRules,
		"projects": authzEngine.MakeProjects(),
	}

	if err := a.engine.SetPolicies(ctx, policies, nil); err != nil {
		return fmt.Errorf("set policies to engine failed: %w", err)
	}
	return nil
}

// persistToDB 全量写入 casbin_rule 表（先清空再批量插入）。
func (a *Authorizer) persistToDB(ctx context.Context, rules []CasbinRuleRow) error {
	if err := a.casbinRuleRepo.ClearAll(ctx); err != nil {
		return err
	}
	return a.casbinRuleRepo.BulkInsert(ctx, rules)
}

// briefsByID 将 API 资源按 id 索引，供角色-API 直接绑定展开。
func briefsByID(briefs []ApiResourceBrief) map[uint32]ApiResourceBrief {
	indexed := make(map[uint32]ApiResourceBrief, len(briefs))
	for _, b := range briefs {
		if b.ID == 0 {
			continue
		}
		indexed[b.ID] = b
	}
	return indexed
}

// expandRolePolicies 展开角色的直接 API 绑定为具体 API 策略（按 path|method 去重）。
// 绑定指向的资源已禁用/下线/删除时静默跳过（apiByID 仅含启用未下线资源）。
func expandRolePolicies(role *userV1.Role, directAPIIDs []uint32, apiByID map[uint32]ApiResourceBrief) []CasbinRuleRow {
	seen := make(map[string]struct{})
	var rules []CasbinRuleRow

	appendRule := func(api ApiResourceBrief) {
		if api.Path == "" || api.Method == "" {
			return
		}
		key := api.Path + "|" + api.Method
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}

		rules = append(rules, CasbinRuleRow{
			PType: "p",
			V0:    role.GetCode(),
			V1:    api.Path,
			V2:    api.Method,
			V3:    "*",
		})
	}

	for _, apiID := range directAPIIDs {
		if api, ok := apiByID[apiID]; ok {
			appendRule(api)
		}
	}
	return rules
}
