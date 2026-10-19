package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// =============================================================================
// 大门规则表边角（S-35 覆盖率爬坡）：类型分派全枚举、作用域、模式退化、
// 授权台账多主体、命中计数失败不阻塞。
// =============================================================================

func TestGateRuleTypeDispatch(t *testing.T) {
	ctx := context.Background()
	it := testItem("weibo", "api", "example.com", "张三")

	cs := []struct {
		name string
		rule *ComplianceRuleRow
		want string
	}{
		{"rights_holder_blacklist 命中 host → block",
			rule("r1", "rights_holder_blacklist", "block", "example\\.com"), "block"},
		{"robots 类型规则命中 host → 运营强制 review",
			rule("r2", "robots", "review", "example"), "review"},
		{"platform_tos 命中 platform+method → review",
			rule("r3", "platform_tos", "review", "(?s)weibo.*api"), "review"},
		{"未知规则类型 → 跳过按 pass（类型白名单）",
			rule("r4", "bogus_type", "block", ".*"), "pass"},
		{"platform 作用域不匹配 → 跳过",
			&ComplianceRuleRow{ID: "r5", RuleType: "sensitive_category", Action: "block", Pattern: "新规定",
				Scope: "platform", Platform: "douyin", Enabled: true, Priority: 10}, "pass"},
		{"platform 作用域匹配 → 命中",
			&ComplianceRuleRow{ID: "r6", RuleType: "sensitive_category", Action: "block", Pattern: "新规定",
				Scope: "platform", Platform: "weibo", Enabled: true, Priority: 10}, "block"},
		{"空 pattern 不命中（匹配前置短路）",
			rule("r7", "sensitive_category", "block", ""), "pass"},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			g := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{c.rule}}, &stubAuthzRepo{})
			out, err := g.Evaluate(ctx, it, GateContext{Method: "hotlist"})
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if out.Gate != c.want {
				t.Fatalf("gate=%s want %s (reasons=%v)", out.Gate, c.want, out.Reasons)
			}
		})
	}
}

// TestGatePatternDegradeToConfigReview 非法 regexp：退化子串 + 配置错误按 review 兜底
// （配置错误不等于放行；authorization 求值先行返回 err，覆盖 markErr/gate 空回落链）。
func TestGatePatternDegradeToConfigReview(t *testing.T) {
	out, err := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{
		rule("r-bad", "authorization", "block", "["), // 非法 regexp
	}}, &stubAuthzRepo{}).Evaluate(context.Background(), testItem("weibo", "api", "example.com", ""), GateContext{Method: "hotlist"})
	if out.Gate != "review" {
		t.Fatalf("非法模式应按 review 兜底（配置错误不等于放行）: %s", out.Gate)
	}
	if err == nil {
		t.Fatal("模式编译错误必须随 firstErr 透出（观测链）")
	}
	found := false
	for _, r := range out.Reasons {
		if strings.Contains(r, "config:规则模式非法") {
			found = true
		}
	}
	if !found {
		t.Fatalf("缺 config 配置错误原因: %v", out.Reasons)
	}
}

// TestGateAuthorizationMultiSubjectGranted 多主体（domain/author/asset）全部 granted →
// 不构成命中，整体 pass；覆盖 granted continue 链与 asset 主体生成（PIPL：author 仅昵称）。
func TestGateAuthorizationMultiSubjectGranted(t *testing.T) {
	it := testItem("weibo", "api", "example.com", "张三")
	it.Payload.MediaRefs = []string{"asset://m_01", "https://cdn.example.com/x.jpg"} // 仅 asset:// 入主体
	authz := &stubAuthzRepo{rows: map[string]*AuthorizationRecordRow{
		"domain|example.com": {Status: "granted"},
		"author|张三":        {Status: "granted"},
		"asset|asset://m_01": {Status: "granted"},
	}}
	g := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{
		rule("r-authz", "authorization", "block", "example\\.com|张三|asset://m_01"),
	}}, authz)

	out, err := g.Evaluate(context.Background(), it, GateContext{Method: "hotlist"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if out.Gate != "pass" {
		t.Fatalf("全主体 granted 应 pass: %s (reasons=%v)", out.Gate, out.Reasons)
	}
}

// TestGateAuthorizationLedgerDown 台账查询故障 → firstErr 透出且保守 review。
// （B-16 fail-closed 对偶面：规则后端故障不得按「规则不存在」放行。）
func TestGateAuthorizationLedgerDown(t *testing.T) {
	authz := &stubAuthzRepo{err: errors.New("ledger down")}
	g := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{
		rule("r-authz", "authorization", "block", "example\\.com"),
	}}, authz)

	out, err := g.Evaluate(context.Background(), testItem("weibo", "api", "example.com", ""), GateContext{Method: "hotlist"})
	if err == nil {
		t.Fatal("台账故障必须透出（审计链）")
	}
	if out.Gate != "review" {
		t.Fatalf("台账故障必须保守 review（fail-closed）: %s", out.Gate)
	}
	found := false
	for _, r := range out.Reasons {
		if strings.Contains(r, "internal:规则求值失败(r-authz)") {
			found = true
		}
	}
	if !found {
		t.Fatalf("缺规则求值失败原因: %v", out.Reasons)
	}
}

// TestGateHitCountFailureNonBlocking 命中计数失败不阻塞闸门结论（审计尽力而为）。
func TestGateHitCountFailureNonBlocking(t *testing.T) {
	rules := &stubRuleRepo{
		rows:   []*ComplianceRuleRow{rule("r1", "sensitive_category", "block", "新规定")},
		hitErr: errors.New("counter down"),
	}
	out, err := newGateForTest(rules, &stubAuthzRepo{}).Evaluate(
		context.Background(), testItem("weibo", "api", "example.com", ""), GateContext{Method: "hotlist"})
	if err != nil {
		t.Fatalf("计数失败不得上抛: %v", err)
	}
	if out.Gate != "block" {
		t.Fatalf("闸门结论不受计数影响: %s", out.Gate)
	}
	if len(rules.hitBump) != 1 || rules.hitBump[0][0] != "r1" {
		t.Fatalf("命中计数调用异常: %v", rules.hitBump)
	}
}

// TestGateAuthorizationDeniedVsDenied 授权 denied → block（B-16 对偶面；granted 已有用例）。
func TestGateAuthorizationDeniedVsDenied(t *testing.T) {
	authz := &stubAuthzRepo{rows: map[string]*AuthorizationRecordRow{
		"domain|example.com": {Status: "denied"},
	}}
	g := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{
		rule("r-authz", "authorization", "block", "example\\.com"),
	}}, authz)
	out, err := g.Evaluate(context.Background(), testItem("weibo", "api", "example.com", ""), GateContext{Method: "hotlist"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if out.Gate != "block" {
		t.Fatalf("denied 必须 block: %s (reasons=%v)", out.Gate, out.Reasons)
	}
}
