package biz

import (
	"context"
	"net/netip"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	entSQL "entgo.io/ent/dialect/sql"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"

	adminV1 "framework/api/gen/go/admin/v1"
	userV1 "framework/api/gen/go/user/v1"
)

// AdminLoginRestrictionRepo 登录限制仓储端口，由 data 层 AdminLoginRestrictionRepo 实现并经 wire.Bind 绑定。
type AdminLoginRestrictionRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginRestrictionResponse, error)
	ListByTargetType(ctx context.Context, targetType adminV1.AdminLoginRestrictionTargetType) ([]*adminV1.AdminLoginRestriction, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *adminV1.GetAdminLoginRestrictionRequest) (*adminV1.AdminLoginRestriction, error)
	Create(ctx context.Context, req *adminV1.CreateAdminLoginRestrictionRequest) error
	Update(ctx context.Context, req *adminV1.UpdateAdminLoginRestrictionRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteAdminLoginRestrictionRequest) error
	// IncrementHit 命中统计：hit_count+1 并回写 last_hit_time（登录判定命中时异步调用，失败仅记日志）。
	IncrementHit(ctx context.Context, id uint32) error
}

// LoginRestrictionOperator 操作者上下文（service 边缘从 JWT/请求提取后传入），供自锁护栏与预演判定。
type LoginRestrictionOperator struct {
	UserID   uint32
	Username string
	ClientIP string
}

// AdminLoginRestrictionUsecase 登录限制业务用例。
// 写操作统一带自锁护栏：套用变更后的规则集对操作者本人（IP+USER 两路）重判定，将导致操作者被拒的变更
// 以 409 LOGIN_RESTRICTION_SELF_LOCK 拦截（请求带 force 时跳过）。
type AdminLoginRestrictionUsecase struct {
	repo      AdminLoginRestrictionRepo
	userRepo  UserRepo
	userToken UserTokenRepo
	log       *log.Helper
}

func NewAdminLoginRestrictionUsecase(repo AdminLoginRestrictionRepo, userRepo UserRepo, userToken UserTokenRepo, logger log.Logger) *AdminLoginRestrictionUsecase {
	return &AdminLoginRestrictionUsecase{
		repo:      repo,
		userRepo:  userRepo,
		userToken: userToken,
		log:       log.NewHelper(log.With(logger, "module", "admin-login-restriction/usecase/admin-service")),
	}
}

func (uc *AdminLoginRestrictionUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginRestrictionResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *AdminLoginRestrictionUsecase) ListByTargetType(ctx context.Context, targetType adminV1.AdminLoginRestrictionTargetType) ([]*adminV1.AdminLoginRestriction, error) {
	return uc.repo.ListByTargetType(ctx, targetType)
}

func (uc *AdminLoginRestrictionUsecase) Get(ctx context.Context, req *adminV1.GetAdminLoginRestrictionRequest) (*adminV1.AdminLoginRestriction, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *AdminLoginRestrictionUsecase) Create(ctx context.Context, req *adminV1.CreateAdminLoginRestrictionRequest, op *LoginRestrictionOperator) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if err := validateLoginRestriction(req.Data); err != nil {
		return err
	}
	if err := uc.checkUserTargetExists(ctx, req.Data); err != nil {
		return err
	}
	if err := uc.checkDuplicateConflict(ctx, req.Data); err != nil {
		return err
	}
	if !req.GetForce() {
		if err := uc.checkSelfLock(ctx, req.Data, adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_CREATE, op, false); err != nil {
			return err
		}
	}
	if err := uc.repo.Create(ctx, req); err != nil {
		return err
	}
	uc.kickAffectedSessions(ctx, req.Data, adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_CREATE, false)
	return nil
}

func (uc *AdminLoginRestrictionUsecase) Update(ctx context.Context, req *adminV1.UpdateAdminLoginRestrictionRequest, op *LoginRestrictionOperator) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	// 兼容 updateMask 部分更新：未提供的字段取库内现值，校验合并后的最终态。
	// allowMissing 且记录不存在时按新建处理，直接校验请求体。
	id := req.GetId()
	if id == 0 {
		id = req.Data.GetId()
	}
	toValidate := req.Data
	// 清空过期时间语义：updateMask 含 expire_time 且值为 null → 最终态 ExpireTime=nil（永久有效），
	// 护栏/预演/踢会话必须按此判定，不得回退取库内旧值（2026-08-26 深查 P3-1）。
	expireCleared := req.Data.GetExpireTime() == nil && updateMaskHasPath(req.GetUpdateMask(), "expire_time", "expireTime")
	existing, err := uc.repo.Get(ctx, &adminV1.GetAdminLoginRestrictionRequest{Id: id})
	switch {
	case err == nil:
		toValidate = mergeLoginRestrictionForValidate(req.Data, existing, expireCleared)
		toValidate.Id = existing.Id
	case req.GetAllowMissing() && adminV1.IsNotFound(err):
		// upsert 新建分支：按请求体校验
	default:
		return err
	}
	if err := validateLoginRestriction(toValidate); err != nil {
		return err
	}
	if err := uc.checkUserTargetExists(ctx, toValidate); err != nil {
		return err
	}
	if err := uc.checkDuplicateConflict(ctx, toValidate); err != nil {
		return err
	}
	if !req.GetForce() {
		if err := uc.checkSelfLock(ctx, toValidate, adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_UPDATE, op, expireCleared); err != nil {
			return err
		}
	}
	if err := uc.repo.Update(ctx, req); err != nil {
		return err
	}
	uc.kickAffectedSessions(ctx, toValidate, adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_UPDATE, expireCleared)
	return nil
}

func (uc *AdminLoginRestrictionUsecase) Delete(ctx context.Context, req *adminV1.DeleteAdminLoginRestrictionRequest, op *LoginRestrictionOperator) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if !req.GetForce() {
		// 删除 ALLOW 规则可能使操作者落入白名单未命中，同样走自锁护栏
		change := &adminV1.AdminLoginRestriction{Id: &req.Id}
		if err := uc.checkSelfLock(ctx, change, adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_DELETE, op, false); err != nil {
			return err
		}
	}
	if err := uc.repo.Delete(ctx, req); err != nil {
		return err
	}
	uc.kickAffectedSessions(ctx, &adminV1.AdminLoginRestriction{Id: &req.Id}, adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_DELETE, false)
	return nil
}

// kickAffectedSessions 踢会话联动：规则变更落库后，对现存在线会话套用变更后规则集重判定，
// 被拒用户的全部令牌立即撤销（强制下线）。同步执行但全部错误降级为日志，不影响写操作响应。
// 与 Simulate 的受影响统计同口径（IP/USER 两路均放行才保留会话）。
func (uc *AdminLoginRestrictionUsecase) kickAffectedSessions(ctx context.Context, change *adminV1.AdminLoginRestriction, changeType adminV1.AdminLoginRestrictionChangeType, expireCleared bool) {
	postIP, postUser, err := uc.buildPostChangeRuleSets(ctx, change, changeType, expireCleared)
	if err != nil {
		uc.log.Warnf("kick affected sessions: build post-change rule sets failed (skip): %v", err)
		return
	}
	onlineUsers, err := uc.userToken.ListOnlineUsers(ctx)
	if err != nil {
		uc.log.Warnf("kick affected sessions: list online users failed (skip): %v", err)
		return
	}
	now := time.Now()
	kicked := make(map[uint32]struct{})
	for _, ou := range onlineUsers {
		if _, done := kicked[ou.UserID]; done {
			continue
		}
		ipDecision := EvaluateLoginRestriction(postIP, adminV1.AdminLoginRestrictionTargetType_IP, ou.LoginIP, now)
		userDecision := EvaluateLoginRestriction(postUser, adminV1.AdminLoginRestrictionTargetType_USER, ou.Username, now)
		if ipDecision.Allowed && userDecision.Allowed {
			continue
		}
		if err := uc.userToken.RemoveToken(ctx, ou.UserID); err != nil {
			uc.log.Warnf("kick affected sessions: revoke tokens for user %d failed: %v", ou.UserID, err)
			continue
		}
		kicked[ou.UserID] = struct{}{}
		uc.log.Warnf("login restriction changed, user kicked: userId=%d username=%s ip=%s ipVerdict=%s userVerdict=%s",
			ou.UserID, ou.Username, ou.LoginIP, ipDecision.Verdict.String(), userDecision.Verdict.String())
	}
	if len(kicked) > 0 {
		uc.log.Infof("login restriction change kicked %d online user(s)", len(kicked))
	}
}

// Simulate 预演登录限制变更：不落库，返回变更后的影响报告（操作者自锁判定/受影响在线会话/规则冲突）。
func (uc *AdminLoginRestrictionUsecase) Simulate(ctx context.Context, req *adminV1.SimulateAdminLoginRestrictionRequest, op *LoginRestrictionOperator) (*adminV1.SimulateAdminLoginRestrictionResponse, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}
	changeType := req.GetChangeType()
	if changeType == adminV1.AdminLoginRestrictionChangeType_ADMIN_LOGIN_RESTRICTION_CHANGE_TYPE_UNSPECIFIED {
		return nil, adminV1.ErrorBadRequest("变更类型 changeType 必填（CREATE/UPDATE/DELETE）")
	}

	change := req.Data
	// Simulate 载荷为全量表单语义：UPDATE 且 ExpireTime 为 nil 即"清空过期时间"（永久有效）
	expireCleared := changeType == adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_UPDATE && change.GetExpireTime() == nil
	var candidate *adminV1.AdminLoginRestriction
	if changeType != adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_DELETE {
		// CREATE/UPDATE：校验拟变更最终态并产出冲突检测候选
		candidate = change
		if changeType == adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_UPDATE && change.GetId() != 0 {
			if existing, err := uc.repo.Get(ctx, &adminV1.GetAdminLoginRestrictionRequest{Id: change.GetId()}); err == nil {
				candidate = mergeLoginRestrictionForValidate(change, existing, expireCleared)
				candidate.Id = existing.Id
			}
		}
		if err := validateLoginRestriction(candidate); err != nil {
			return nil, err
		}
	} else if change.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("DELETE 预演时 data.id 必填")
	}

	postIP, postUser, err := uc.buildPostChangeRuleSets(ctx, change, changeType, expireCleared)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	resp := &adminV1.SimulateAdminLoginRestrictionResponse{}

	// 操作者自锁判定：IP/USER 两路均放行才算可登录
	if op != nil {
		ipDecision := EvaluateLoginRestriction(postIP, adminV1.AdminLoginRestrictionTargetType_IP, op.ClientIP, now)
		userDecision := EvaluateLoginRestriction(postUser, adminV1.AdminLoginRestrictionTargetType_USER, op.Username, now)
		decisive := ipDecision
		if ipDecision.Allowed {
			decisive = userDecision
		}
		allowed := ipDecision.Allowed && userDecision.Allowed
		resp.OperatorAllowed = &allowed
		resp.OperatorVerdict = &decisive.Verdict
		if decisive.MatchedRule != nil {
			resp.MatchedRuleId = decisive.MatchedRule.Id
		}
		resp.OperatorIp = &op.ClientIP
		resp.OperatorUsername = &op.Username
	}

	// 受影响在线会话：逐会话重判定（IP 或 USER 任一路被拒即受影响）
	if onlineUsers, err := uc.userToken.ListOnlineUsers(ctx); err != nil {
		uc.log.Warnf("simulate: list online users failed (skip affected stat): %v", err)
	} else {
		seen := make(map[uint32]struct{})
		var affectedUserIDs []uint32
		var sessions uint32
		for _, ou := range onlineUsers {
			ipDecision := EvaluateLoginRestriction(postIP, adminV1.AdminLoginRestrictionTargetType_IP, ou.LoginIP, now)
			userDecision := EvaluateLoginRestriction(postUser, adminV1.AdminLoginRestrictionTargetType_USER, ou.Username, now)
			if ipDecision.Allowed && userDecision.Allowed {
				continue
			}
			sessions++
			if _, ok := seen[ou.UserID]; !ok {
				seen[ou.UserID] = struct{}{}
				affectedUserIDs = append(affectedUserIDs, ou.UserID)
			}
		}
		resp.AffectedUserIds = affectedUserIDs
		resp.AffectedOnlineSessions = &sessions
	}

	// 规则冲突（CREATE/UPDATE 候选与现存规则比对）
	if candidate != nil {
		curIP, err := uc.repo.ListByTargetType(ctx, adminV1.AdminLoginRestrictionTargetType_IP)
		if err != nil {
			return nil, err
		}
		curUser, err := uc.repo.ListByTargetType(ctx, adminV1.AdminLoginRestrictionTargetType_USER)
		if err != nil {
			return nil, err
		}
		resp.Conflicts = DetectLoginRestrictionConflicts(append(curIP, curUser...), candidate, now)
	}

	return resp, nil
}

// checkUserTargetExists USER 类型规则的目标用户名必须真实存在（选择代替手填的服务端兜底）。
func (uc *AdminLoginRestrictionUsecase) checkUserTargetExists(ctx context.Context, r *adminV1.AdminLoginRestriction) error {
	if r.GetTargetType() != adminV1.AdminLoginRestrictionTargetType_USER {
		return nil
	}
	_, err := uc.userRepo.GetUserByUserName(ctx, r.GetTargetValue())
	switch {
	case err == nil:
		return nil
	case userV1.IsUserNotFound(err):
		return adminV1.ErrorBadRequest("目标用户 %q 不存在，请从用户列表选择", r.GetTargetValue())
	default:
		return err
	}
}

// checkDuplicateConflict 唯一索引口径（target_type+target_value+action）查重预检，命中返回 409 友好错误。
// 与 DB 唯一索引同口径：不考虑状态与过期（停用/过期记录同样占位）。
func (uc *AdminLoginRestrictionUsecase) checkDuplicateConflict(ctx context.Context, candidate *adminV1.AdminLoginRestriction) error {
	ipRules, err := uc.repo.ListByTargetType(ctx, adminV1.AdminLoginRestrictionTargetType_IP)
	if err != nil {
		return err
	}
	userRules, err := uc.repo.ListByTargetType(ctx, adminV1.AdminLoginRestrictionTargetType_USER)
	if err != nil {
		return err
	}
	if dup := FindLoginRestrictionDuplicate(append(ipRules, userRules...), candidate); dup != nil {
		return adminV1.ErrorLoginRestrictionConflict(
			"与现存规则#%d 重复（目标类型 %s、目标值 %q、动作 %s 已存在）",
			dup.GetId(), dup.GetTargetType().String(), dup.GetTargetValue(), dup.GetAction().String())
	}
	return nil
}

// checkSelfLock 自锁护栏：套用拟变更后的规则集，对操作者本人（IP+USER 两路）重判定，任一路被拒即拦截。
func (uc *AdminLoginRestrictionUsecase) checkSelfLock(ctx context.Context, change *adminV1.AdminLoginRestriction, changeType adminV1.AdminLoginRestrictionChangeType, op *LoginRestrictionOperator, expireCleared bool) error {
	if op == nil {
		return nil
	}
	postIP, postUser, err := uc.buildPostChangeRuleSets(ctx, change, changeType, expireCleared)
	if err != nil {
		return err
	}
	now := time.Now()
	if d := EvaluateLoginRestriction(postIP, adminV1.AdminLoginRestrictionTargetType_IP, op.ClientIP, now); !d.Allowed {
		uc.log.Warnf("self-lock guard hit: operator=%s ip=%s verdict=%s", op.Username, op.ClientIP, d.Verdict.String())
		return adminV1.ErrorLoginRestrictionSelfLock(
			"该变更生效后，当前操作者来源 IP %s 将被拒绝登录（%s）。如确认无误，请使用 force 强制保存", op.ClientIP, d.Reason)
	}
	if d := EvaluateLoginRestriction(postUser, adminV1.AdminLoginRestrictionTargetType_USER, op.Username, now); !d.Allowed {
		uc.log.Warnf("self-lock guard hit: operator=%s verdict=%s", op.Username, d.Verdict.String())
		return adminV1.ErrorLoginRestrictionSelfLock(
			"该变更生效后，当前操作者账号 %s 将被拒绝登录（%s）。如确认无误，请使用 force 强制保存", op.Username, d.Reason)
	}
	return nil
}

// buildPostChangeRuleSets 加载当前两路规则并套用拟变更，返回变更后的 IP/USER 规则集。
// UPDATE 先按 id 从两集合移除旧规则再按变更后类型插入合并结果，兼容 updateMask 部分更新与目标类型变更。
// expireCleared 语义同 mergeLoginRestrictionForValidate。
func (uc *AdminLoginRestrictionUsecase) buildPostChangeRuleSets(ctx context.Context, change *adminV1.AdminLoginRestriction, changeType adminV1.AdminLoginRestrictionChangeType, expireCleared bool) (ipRules, userRules []*adminV1.AdminLoginRestriction, err error) {
	ipRules, err = uc.repo.ListByTargetType(ctx, adminV1.AdminLoginRestrictionTargetType_IP)
	if err != nil {
		return nil, nil, err
	}
	userRules, err = uc.repo.ListByTargetType(ctx, adminV1.AdminLoginRestrictionTargetType_USER)
	if err != nil {
		return nil, nil, err
	}

	switch changeType {
	case adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_CREATE:
		switch change.GetTargetType() {
		case adminV1.AdminLoginRestrictionTargetType_IP:
			ipRules = append(ipRules, change)
		case adminV1.AdminLoginRestrictionTargetType_USER:
			userRules = append(userRules, change)
		}
		return ipRules, userRules, nil

	case adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_DELETE:
		return removeLoginRestrictionByID(ipRules, change.GetId()), removeLoginRestrictionByID(userRules, change.GetId()), nil

	case adminV1.AdminLoginRestrictionChangeType_CHANGE_TYPE_UPDATE:
		merged := change
		if existing := findLoginRestrictionByID(ipRules, change.GetId()); existing != nil {
			merged = mergeLoginRestrictionForValidate(change, existing, expireCleared)
			merged.Id = existing.Id
		} else if existing := findLoginRestrictionByID(userRules, change.GetId()); existing != nil {
			merged = mergeLoginRestrictionForValidate(change, existing, expireCleared)
			merged.Id = existing.Id
		}
		ipRules = removeLoginRestrictionByID(ipRules, change.GetId())
		userRules = removeLoginRestrictionByID(userRules, change.GetId())
		switch merged.GetTargetType() {
		case adminV1.AdminLoginRestrictionTargetType_IP:
			ipRules = append(ipRules, merged)
		case adminV1.AdminLoginRestrictionTargetType_USER:
			userRules = append(userRules, merged)
		}
		return ipRules, userRules, nil
	}
	return ipRules, userRules, nil
}

func findLoginRestrictionByID(rules []*adminV1.AdminLoginRestriction, id uint32) *adminV1.AdminLoginRestriction {
	if id == 0 {
		return nil
	}
	for _, r := range rules {
		if r.GetId() == id {
			return r
		}
	}
	return nil
}

func removeLoginRestrictionByID(rules []*adminV1.AdminLoginRestriction, id uint32) []*adminV1.AdminLoginRestriction {
	if id == 0 {
		return rules
	}
	out := make([]*adminV1.AdminLoginRestriction, 0, len(rules))
	for _, r := range rules {
		if r.GetId() != id {
			out = append(out, r)
		}
	}
	return out
}

// validateLoginRestriction 校验登录限制规则最终态：
//   - target_type 必须为 IP/USER，action 必须为 BLOCK/ALLOW；
//   - target_value 必填；target_type=IP 时必须是合法 IP 或 CIDR（如 10.0.0.0/8）；
//   - status 非空时必须是 ON/OFF。
func validateLoginRestriction(r *adminV1.AdminLoginRestriction) error {
	targetType := r.GetTargetType()
	if targetType != adminV1.AdminLoginRestrictionTargetType_IP &&
		targetType != adminV1.AdminLoginRestrictionTargetType_USER {
		return adminV1.ErrorBadRequest("限制目标类型 targetType 必须为 IP 或 USER")
	}
	value := r.GetTargetValue()
	if value == "" {
		return adminV1.ErrorBadRequest("限制目标值 targetValue 不能为空")
	}
	if targetType == adminV1.AdminLoginRestrictionTargetType_IP && !isValidIPOrCIDR(value) {
		return adminV1.ErrorBadRequest("限制目标值 %q 不是合法的 IP 地址或 CIDR 网段（如 192.168.1.1 或 192.168.1.0/24）", value)
	}
	action := r.GetAction()
	if action != adminV1.AdminLoginRestrictionAction_BLOCK &&
		action != adminV1.AdminLoginRestrictionAction_ALLOW {
		return adminV1.ErrorBadRequest("动作 action 必须为 BLOCK 或 ALLOW")
	}
	if s := r.GetStatus(); s != "" && s != LoginRestrictionStatusON && s != LoginRestrictionStatusOFF {
		return adminV1.ErrorBadRequest("状态 status 必须为 ON 或 OFF")
	}
	return nil
}

// isValidIPOrCIDR 校验字符串是合法 IP 地址或 CIDR 网段。
func isValidIPOrCIDR(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	if _, err := netip.ParsePrefix(s); err == nil {
		return true
	}
	return false
}

// MatchRestrictionTarget 判断登录限制规则是否命中目标值。
// 先精确匹配；targetType=IP 时额外支持 CIDR 网段匹配（规则值为网段、目标为具体 IP）。
func MatchRestrictionTarget(targetType adminV1.AdminLoginRestrictionTargetType, ruleValue, targetValue string) bool {
	if ruleValue == targetValue {
		return true
	}
	if targetType != adminV1.AdminLoginRestrictionTargetType_IP {
		return false
	}
	prefix, err := netip.ParsePrefix(ruleValue)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(targetValue)
	if err != nil {
		return false
	}
	return prefix.Contains(addr)
}

// mergeLoginRestrictionForValidate 合并更新请求与库内现值，得到更新后的最终态用于校验/护栏判定。
// expireCleared=true 表示调用方已明确"清空过期时间"（updateMask 含 expire_time 且值为 null，
// 或 Simulate 全量载荷语义下 ExpireTime 为 nil），不再回退取库内旧值。
func mergeLoginRestrictionForValidate(in, base *adminV1.AdminLoginRestriction, expireCleared bool) *adminV1.AdminLoginRestriction {
	out := &adminV1.AdminLoginRestriction{
		TargetType:  in.TargetType,
		TargetValue: in.TargetValue,
		Action:      in.Action,
		Reason:      in.Reason,
		Status:      in.Status,
		ExpireTime:  in.ExpireTime,
	}
	if out.TargetType == nil {
		out.TargetType = base.TargetType
	}
	if out.TargetValue == nil {
		out.TargetValue = base.TargetValue
	}
	if out.Action == nil {
		out.Action = base.Action
	}
	if out.Reason == nil {
		out.Reason = base.Reason
	}
	if out.Status == nil {
		out.Status = base.Status
	}
	if out.ExpireTime == nil && !expireCleared {
		out.ExpireTime = base.ExpireTime
	}
	return out
}

// updateMaskHasPath 判断 updateMask 是否包含任一指定字段路径
//（proto JSON 解析后规范为 snake_case；此处同时兼容 camelCase 直传）。mask 为 nil 时安全返回 false。
func updateMaskHasPath(mask *fieldmaskpb.FieldMask, paths ...string) bool {
	for _, p := range mask.GetPaths() {
		for _, want := range paths {
			if p == want {
				return true
			}
		}
	}
	return false
}
