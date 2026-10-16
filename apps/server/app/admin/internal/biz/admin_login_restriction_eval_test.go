package biz

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "framework/api/gen/go/admin/v1"
)

// lrRule 构造登录限制规则（proto optional 字段指针化）。
func lrRule(id uint32, targetType adminV1.AdminLoginRestrictionTargetType, value string, action adminV1.AdminLoginRestrictionAction) *adminV1.AdminLoginRestriction {
	return &adminV1.AdminLoginRestriction{
		Id:          &id,
		TargetType:  targetType.Enum(),
		TargetValue: &value,
		Action:      action.Enum(),
	}
}

func lrWithStatus(r *adminV1.AdminLoginRestriction, status string) *adminV1.AdminLoginRestriction {
	r.Status = &status
	return r
}

func lrWithExpire(r *adminV1.AdminLoginRestriction, t time.Time) *adminV1.AdminLoginRestriction {
	r.ExpireTime = timestamppb.New(t)
	return r
}

// TestEvaluateLoginRestriction 判定矩阵：阻断优先 / 白名单模式 / 状态与过期过滤 / CIDR 匹配。
func TestEvaluateLoginRestriction(t *testing.T) {
	now := time.Now()
	ipType := adminV1.AdminLoginRestrictionTargetType_IP
	userType := adminV1.AdminLoginRestrictionTargetType_USER
	block := adminV1.AdminLoginRestrictionAction_BLOCK
	allow := adminV1.AdminLoginRestrictionAction_ALLOW

	cases := []struct {
		name         string
		rules        []*adminV1.AdminLoginRestriction
		targetType   adminV1.AdminLoginRestrictionTargetType
		targetValue  string
		wantAllowed  bool
		wantVerdict  adminV1.AdminLoginRestrictionVerdict
		wantRuleID   uint32 // 0 = 不校验命中规则
	}{
		{
			name:        "空目标值放行（取不到 IP 不误拦）",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "1.2.3.4", block)},
			targetType:  ipType,
			targetValue: "",
			wantAllowed: true,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
		},
		{
			name:        "无规则默认放行",
			rules:       nil,
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: true,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
		},
		{
			name:        "精确 BLOCK 命中拒绝",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(7, ipType, "1.2.3.4", block)},
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  7,
		},
		{
			name:        "CIDR BLOCK 命中拒绝",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(8, ipType, "10.0.0.0/8", block)},
			targetType:  ipType,
			targetValue: "10.20.30.40",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  8,
		},
		{
			name: "阻断优先于放行（同目标双规则）",
			rules: []*adminV1.AdminLoginRestriction{
				lrRule(1, ipType, "1.2.3.4", allow),
				lrRule(2, ipType, "1.2.3.4", block),
			},
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  2,
		},
		{
			name:        "白名单模式未命中拒绝",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(3, ipType, "192.168.4.0/24", allow)},
			targetType:  ipType,
			targetValue: "8.8.8.8",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_WHITELIST_MISS,
		},
		{
			name:        "白名单命中放行",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(4, ipType, "192.168.4.0/24", allow)},
			targetType:  ipType,
			targetValue: "192.168.4.44",
			wantAllowed: true,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_ALLOW_MATCH,
			wantRuleID:  4,
		},
		{
			name:        "OFF 状态规则不参与判定",
			rules:       []*adminV1.AdminLoginRestriction{lrWithStatus(lrRule(5, ipType, "1.2.3.4", block), LoginRestrictionStatusOFF)},
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: true,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
		},
		{
			name:        "已过期规则不参与判定",
			rules:       []*adminV1.AdminLoginRestriction{lrWithExpire(lrRule(6, ipType, "1.2.3.4", block), now.Add(-time.Hour))},
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: true,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
		},
		{
			name:        "未到期规则参与判定",
			rules:       []*adminV1.AdminLoginRestriction{lrWithExpire(lrRule(9, ipType, "1.2.3.4", block), now.Add(time.Hour))},
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  9,
		},
		{
			name:        "status 为空按 ON 处理（历史数据兼容）",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(10, ipType, "1.2.3.4", block)},
			targetType:  ipType,
			targetValue: "1.2.3.4",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  10,
		},
		{
			name:        "USER 精确匹配拒绝",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(11, userType, "admin", block)},
			targetType:  userType,
			targetValue: "admin",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  11,
		},
		{
			name:        "USER 不做网段匹配",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(12, userType, "adm", block)},
			targetType:  userType,
			targetValue: "admin",
			wantAllowed: true,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_NO_RULE,
		},
		{
			name:        "IPv6 CIDR 命中",
			rules:       []*adminV1.AdminLoginRestriction{lrRule(13, ipType, "2001:db8::/32", block)},
			targetType:  ipType,
			targetValue: "2001:db8::1",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_BLOCK_MATCH,
			wantRuleID:  13,
		},
		{
			name: "过期 BLOCK 跳过但白名单模式仍生效",
			rules: []*adminV1.AdminLoginRestriction{
				lrWithExpire(lrRule(14, ipType, "8.8.8.8", block), now.Add(-time.Hour)),
				lrRule(15, ipType, "192.168.0.0/16", allow),
			},
			targetType:  ipType,
			targetValue: "8.8.8.8",
			wantAllowed: false,
			wantVerdict: adminV1.AdminLoginRestrictionVerdict_VERDICT_WHITELIST_MISS,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateLoginRestriction(tc.rules, tc.targetType, tc.targetValue, now)
			if got.Allowed != tc.wantAllowed {
				t.Errorf("Allowed = %v, want %v (reason: %s)", got.Allowed, tc.wantAllowed, got.Reason)
			}
			if got.Verdict != tc.wantVerdict {
				t.Errorf("Verdict = %v, want %v", got.Verdict, tc.wantVerdict)
			}
			if tc.wantRuleID != 0 {
				if got.MatchedRule == nil {
					t.Fatalf("MatchedRule = nil, want rule #%d", tc.wantRuleID)
				}
				if got.MatchedRule.GetId() != tc.wantRuleID {
					t.Errorf("MatchedRule.Id = %d, want %d", got.MatchedRule.GetId(), tc.wantRuleID)
				}
			}
		})
	}
}

// TestFindLoginRestrictionDuplicate 唯一索引口径查重（忽略状态/过期，排除自比）。
func TestFindLoginRestrictionDuplicate(t *testing.T) {
	ipType := adminV1.AdminLoginRestrictionTargetType_IP
	block := adminV1.AdminLoginRestrictionAction_BLOCK
	allow := adminV1.AdminLoginRestrictionAction_ALLOW

	existing := []*adminV1.AdminLoginRestriction{
		lrRule(1, ipType, "1.2.3.4", block),
		lrWithStatus(lrRule(2, ipType, "5.6.7.8", block), LoginRestrictionStatusOFF), // 停用仍占位
	}

	if dup := FindLoginRestrictionDuplicate(existing, lrRule(0, ipType, "1.2.3.4", block)); dup == nil || dup.GetId() != 1 {
		t.Errorf("expected dup rule #1, got %v", dup)
	}
	if dup := FindLoginRestrictionDuplicate(existing, lrRule(0, ipType, "5.6.7.8", block)); dup == nil || dup.GetId() != 2 {
		t.Errorf("OFF rule should still occupy unique key, got %v", dup)
	}
	if dup := FindLoginRestrictionDuplicate(existing, lrRule(1, ipType, "1.2.3.4", block)); dup != nil {
		t.Errorf("self id should be excluded, got %v", dup.GetId())
	}
	if dup := FindLoginRestrictionDuplicate(existing, lrRule(0, ipType, "1.2.3.4", allow)); dup != nil {
		t.Errorf("different action is not a duplicate, got %v", dup.GetId())
	}
	if dup := FindLoginRestrictionDuplicate(existing, lrRule(0, ipType, "9.9.9.9", block)); dup != nil {
		t.Errorf("different value is not a duplicate, got %v", dup.GetId())
	}
}

// TestDetectLoginRestrictionConflicts 冲突检测：重复 / 同目标反向动作 / 网段覆盖。
func TestDetectLoginRestrictionConflicts(t *testing.T) {
	now := time.Now()
	ipType := adminV1.AdminLoginRestrictionTargetType_IP
	block := adminV1.AdminLoginRestrictionAction_BLOCK
	allow := adminV1.AdminLoginRestrictionAction_ALLOW

	conflictTypes := func(cs []*adminV1.AdminLoginRestrictionConflict) map[string]bool {
		out := map[string]bool{}
		for _, c := range cs {
			out[c.GetType()] = true
		}
		return out
	}

	t.Run("同目标同动作 DUPLICATE", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "1.2.3.4", block)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "1.2.3.4", block), now)
		if !conflictTypes(cs)[LoginRestrictionConflictDuplicate] {
			t.Errorf("expected DUPLICATE, got %v", cs)
		}
	})

	t.Run("同目标反向动作 DENY_OVERRIDES", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "1.2.3.4", allow)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "1.2.3.4", block), now)
		if !conflictTypes(cs)[LoginRestrictionConflictDenyOverrides] {
			t.Errorf("expected DENY_OVERRIDES, got %v", cs)
		}
	})

	t.Run("网段互相包含同动作 CIDR_COVER", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "192.168.0.0/16", block)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "192.168.4.0/24", block), now)
		if !conflictTypes(cs)[LoginRestrictionConflictCIDRCover] {
			t.Errorf("expected CIDR_COVER, got %v", cs)
		}
	})

	t.Run("网段覆盖且反向动作 DENY_OVERRIDES", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "192.168.0.0/16", allow)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "192.168.4.0/24", block), now)
		if !conflictTypes(cs)[LoginRestrictionConflictDenyOverrides] {
			t.Errorf("expected DENY_OVERRIDES, got %v", cs)
		}
	})

	t.Run("纯地址落在网段内也算覆盖", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "192.168.0.0/16", block)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "192.168.4.44", block), now)
		if !conflictTypes(cs)[LoginRestrictionConflictCIDRCover] {
			t.Errorf("expected CIDR_COVER, got %v", cs)
		}
	})

	t.Run("不相交网段无冲突", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "192.168.0.0/16", block)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "10.0.0.0/8", block), now)
		if len(cs) != 0 {
			t.Errorf("expected no conflict, got %v", cs)
		}
	})

	t.Run("OFF 规则不参与冲突检测", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrWithStatus(lrRule(1, ipType, "1.2.3.4", block), LoginRestrictionStatusOFF)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(0, ipType, "1.2.3.4", block), now)
		if len(cs) != 0 {
			t.Errorf("expected no conflict with OFF rule, got %v", cs)
		}
	})

	t.Run("更新场景跳过自比", func(t *testing.T) {
		existing := []*adminV1.AdminLoginRestriction{lrRule(1, ipType, "1.2.3.4", block)}
		cs := DetectLoginRestrictionConflicts(existing, lrRule(1, ipType, "1.2.3.4", block), now)
		if len(cs) != 0 {
			t.Errorf("expected self-comparison skipped, got %v", cs)
		}
	})
}
