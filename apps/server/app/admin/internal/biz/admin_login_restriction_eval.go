package biz

import (
	"fmt"
	"net/netip"
	"time"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 登录限制规则状态值（与 mixin.SwitchStatus 枚举口径一致）。
const (
	LoginRestrictionStatusON  = "ON"
	LoginRestrictionStatusOFF = "OFF"
)

// 登录限制规则冲突类型（与 AdminLoginRestrictionConflict.type 口径一致）。
const (
	LoginRestrictionConflictDuplicate      = "DUPLICATE"      // 同目标类型+目标值+动作重复
	LoginRestrictionConflictCIDRCover      = "CIDR_COVER"     // 网段互相包含（同动作冗余）
	LoginRestrictionConflictDenyOverrides  = "DENY_OVERRIDES" // 同目标 BLOCK 压制 ALLOW
)

// LoginRestrictionDecision 登录限制判定结果（纯函数产出，认证链路/预演/自锁护栏复用）。
type LoginRestrictionDecision struct {
	Verdict     adminV1.AdminLoginRestrictionVerdict // 判定结论
	Allowed     bool                                 // 是否放行
	MatchedRule *adminV1.AdminLoginRestriction       // 决定结果的命中规则（ALLOW_MATCH/BLOCK_MATCH 时非空，供命中统计/报告）
	Reason      string                               // 人类可读原因（错误消息/预演报告直接透出）
}

// IsLoginRestrictionEffective 规则是否参与判定：启用中且未过期。
// status 为空（历史数据/未设置）按 ON 处理；expire_time 为空=永久有效，expire_time<=now 视为过期。
func IsLoginRestrictionEffective(r *adminV1.AdminLoginRestriction, now time.Time) bool {
	if r == nil {
		return false
	}
	if s := r.GetStatus(); s != "" && s != LoginRestrictionStatusON {
		return false
	}
	if et := r.GetExpireTime(); et != nil && et.CheckValid() == nil && !et.AsTime().After(now) {
		return false
	}
	return true
}

// EvaluateLoginRestriction 登录限制判定核心（纯函数，无 IO，可单测）。
// 语义：
//   - 仅启用中且未过期的规则参与判定；
//   - 命中 BLOCK → 拒绝（阻断优先于放行）；
//   - 存在 ALLOW 规则（白名单模式）且未命中任何 ALLOW → 拒绝；
//   - 其余 → 放行；
//   - targetValue 为空按放行处理（取不到 IP 等场景不误拦），与认证链路历史行为一致。
func EvaluateLoginRestriction(rules []*adminV1.AdminLoginRestriction, targetType adminV1.AdminLoginRestrictionTargetType, targetValue string, now time.Time) LoginRestrictionDecision {
	if targetValue == "" {
		return LoginRestrictionDecision{
			Verdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
			Allowed: true,
			Reason:  "目标值为空，跳过登录限制判定",
		}
	}

	var firstAllow, matchedAllow, matchedBlock *adminV1.AdminLoginRestriction
	for _, r := range rules {
		if !IsLoginRestrictionEffective(r, now) {
			continue
		}
		matched := MatchRestrictionTarget(targetType, r.GetTargetValue(), targetValue)
		switch r.GetAction() {
		case adminV1.AdminLoginRestrictionAction_ALLOW:
			if firstAllow == nil {
				firstAllow = r
			}
			if matched && matchedAllow == nil {
				matchedAllow = r
			}
		case adminV1.AdminLoginRestrictionAction_BLOCK:
			if matched && matchedBlock == nil {
				matchedBlock = r
			}
		}
	}

	switch {
	case matchedBlock != nil:
		return LoginRestrictionDecision{
			Verdict:     adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			Allowed:     false,
			MatchedRule: matchedBlock,
			Reason:      fmt.Sprintf("命中登录阻断规则（%s：%s）", loginRestrictionTargetTypeName(targetType), matchedBlock.GetTargetValue()),
		}
	case firstAllow != nil && matchedAllow == nil:
		return LoginRestrictionDecision{
			Verdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_WHITELIST_MISS,
			Allowed: false,
			Reason:  fmt.Sprintf("当前%s不在登录白名单内", loginRestrictionTargetTypeName(targetType)),
		}
	case matchedAllow != nil:
		return LoginRestrictionDecision{
			Verdict:     adminV1.AdminLoginRestrictionVerdict_VERDICT_ALLOW_MATCH,
			Allowed:     true,
			MatchedRule: matchedAllow,
			Reason:      "命中登录放行规则",
		}
	default:
		return LoginRestrictionDecision{
			Verdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
			Allowed: true,
			Reason:  "未命中登录限制规则，默认放行",
		}
	}
}

// FindLoginRestrictionDuplicate 按唯一索引口径（target_type+target_value+action，忽略状态与过期）查找重复规则。
// 与 DB 唯一索引同口径（停用/过期记录同样占位）；排除与 candidate 同 id 的记录（更新场景自比）。未找到返回 nil。
func FindLoginRestrictionDuplicate(rules []*adminV1.AdminLoginRestriction, candidate *adminV1.AdminLoginRestriction) *adminV1.AdminLoginRestriction {
	if candidate == nil {
		return nil
	}
	for _, r := range rules {
		if candidate.GetId() != 0 && r.GetId() == candidate.GetId() {
			continue
		}
		if r.GetTargetType() == candidate.GetTargetType() &&
			r.GetTargetValue() == candidate.GetTargetValue() &&
			r.GetAction() == candidate.GetAction() {
			return r
		}
	}
	return nil
}

// DetectLoginRestrictionConflicts 检测拟变更规则与现存规则的冲突（纯函数）。
// 用于预演报告与 Create/Update 的 409 冲突友好化预检；candidate 自身 id 与现存规则相同（更新场景）时跳过自比。
func DetectLoginRestrictionConflicts(rules []*adminV1.AdminLoginRestriction, candidate *adminV1.AdminLoginRestriction, now time.Time) []*adminV1.AdminLoginRestrictionConflict {
	if candidate == nil {
		return nil
	}
	var conflicts []*adminV1.AdminLoginRestrictionConflict
	for _, r := range rules {
		if r.GetId() != 0 && r.GetId() == candidate.GetId() {
			continue
		}
		if r.GetTargetType() != candidate.GetTargetType() {
			continue
		}
		if !IsLoginRestrictionEffective(r, now) {
			continue
		}

		sameValue := r.GetTargetValue() == candidate.GetTargetValue()
		sameAction := r.GetAction() == candidate.GetAction()

		switch {
		case sameValue && sameAction:
			conflicts = append(conflicts, &adminV1.AdminLoginRestrictionConflict{
				Type:       loginRestrictionStrPtr(LoginRestrictionConflictDuplicate),
				WithRuleId: r.Id,
				Detail:     loginRestrictionStrPtr(fmt.Sprintf("与规则#%d 重复（同目标类型+目标值+动作）", r.GetId())),
			})
		case sameValue && !sameAction:
			conflicts = append(conflicts, &adminV1.AdminLoginRestrictionConflict{
				Type:       loginRestrictionStrPtr(LoginRestrictionConflictDenyOverrides),
				WithRuleId: r.Id,
				Detail:     loginRestrictionStrPtr(fmt.Sprintf("与规则#%d 同目标但动作相反，BLOCK 将压制 ALLOW", r.GetId())),
			})
		case !sameValue && candidate.GetTargetType() == adminV1.AdminLoginRestrictionTargetType_IP:
			if !loginRestrictionIPRangesOverlap(r.GetTargetValue(), candidate.GetTargetValue()) {
				continue
			}
			conflictType := LoginRestrictionConflictCIDRCover
			detail := fmt.Sprintf("与规则#%d 的 IP/网段互相覆盖（同动作冗余）", r.GetId())
			if !sameAction {
				conflictType = LoginRestrictionConflictDenyOverrides
				detail = fmt.Sprintf("与规则#%d 的 IP/网段覆盖且动作相反，覆盖区间内 BLOCK 将压制 ALLOW", r.GetId())
			}
			conflicts = append(conflicts, &adminV1.AdminLoginRestrictionConflict{
				Type:       loginRestrictionStrPtr(conflictType),
				WithRuleId: r.Id,
				Detail:     loginRestrictionStrPtr(detail),
			})
		}
	}
	return conflicts
}

// loginRestrictionIPRangesOverlap 判断两个 IP/CIDR 规则值的覆盖区间是否相交：
// 任一单方解析为网段且包含另一方（地址或网段任一端点），或双方网段互相包含。
func loginRestrictionIPRangesOverlap(a, b string) bool {
	aPrefix, aIsPrefix := parseIPPrefixLoose(a)
	bPrefix, bIsPrefix := parseIPPrefixLoose(b)

	switch {
	case aIsPrefix && bIsPrefix:
		return aPrefix.Contains(bPrefix.Masked().Addr()) || bPrefix.Contains(aPrefix.Masked().Addr())
	case aIsPrefix:
		addr, err := netip.ParseAddr(b)
		return err == nil && aPrefix.Contains(addr)
	case bIsPrefix:
		addr, err := netip.ParseAddr(a)
		return err == nil && bPrefix.Contains(addr)
	default:
		return false // 两个纯地址且不相等（相等场景已在 sameValue 分支处理）
	}
}

// parseIPPrefixLoose 解析 CIDR；纯 IP 视为 /32（IPv4）或 /128（IPv6） host 网段时不按 prefix 处理，保持语义区分。
func parseIPPrefixLoose(s string) (netip.Prefix, bool) {
	prefix, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, false
	}
	return prefix, true
}

func loginRestrictionTargetTypeName(t adminV1.AdminLoginRestrictionTargetType) string {
	switch t {
	case adminV1.AdminLoginRestrictionTargetType_IP:
		return "IP"
	case adminV1.AdminLoginRestrictionTargetType_USER:
		return "用户"
	default:
		return "目标"
	}
}

// loginRestrictionStrPtr 与 license_test.go 的 strPtr 区分，避免测试编译期同包重名。
func loginRestrictionStrPtr(s string) *string { return &s }
