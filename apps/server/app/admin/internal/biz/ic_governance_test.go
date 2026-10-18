package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 合规闸门测试（FR-INT-12 🔴 无旁路 / B-16 fail-closed，S-9 审计）
// 核心断言：任何错误 → review，绝不静默放行；严格者胜出。
// =============================================================================

// stubRuleRepo 规则仓储桩（仅闸门用得到的两个方法有行为）。
type stubRuleRepo struct {
	rows    []*ComplianceRuleRow
	err     error
	hitBump [][]string
	hitErr  error
}

func (s *stubRuleRepo) Create(_ context.Context, _ *ComplianceRuleRow) (*ComplianceRuleRow, error) {
	panic("not used in gate tests")
}
func (s *stubRuleRepo) GetByID(_ context.Context, _ string) (*ComplianceRuleRow, error) {
	panic("not used in gate tests")
}
func (s *stubRuleRepo) Update(_ context.Context, _ *ComplianceRuleRow) (*ComplianceRuleRow, error) {
	panic("not used in gate tests")
}
func (s *stubRuleRepo) Delete(_ context.Context, _ string) (bool, error) {
	panic("not used in gate tests")
}
func (s *stubRuleRepo) List(_ context.Context, _ *ComplianceRuleListQuery) ([]*ComplianceRuleRow, int, error) {
	panic("not used in gate tests")
}
func (s *stubRuleRepo) ListEffective(_ context.Context, _ time.Time) ([]*ComplianceRuleRow, error) {
	return s.rows, s.err
}
func (s *stubRuleRepo) IncrementHitCount(_ context.Context, ids []string) error {
	s.hitBump = append(s.hitBump, ids)
	return s.hitErr
}

// stubAuthzRepo 授权台账桩。
type stubAuthzRepo struct {
	rows map[string]*AuthorizationRecordRow // key = subject+"|"+subjectKey
	err  error
}

func (s *stubAuthzRepo) Upsert(_ context.Context, _ *AuthorizationRecordRow) (*AuthorizationRecordRow, error) {
	panic("not used in gate tests")
}
func (s *stubAuthzRepo) Get(_ context.Context, subject, subjectKey string, _ time.Time) (*AuthorizationRecordRow, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rows[subject+"|"+subjectKey], nil
}
func (s *stubAuthzRepo) List(_ context.Context, _ string, _, _ int32) ([]*AuthorizationRecordRow, int, error) {
	panic("not used in gate tests")
}
func (s *stubAuthzRepo) Delete(_ context.Context, _ string) (bool, error) {
	panic("not used in gate tests")
}

func newGateForTest(rules *stubRuleRepo, authz *stubAuthzRepo) *GovernanceGate {
	return NewGovernanceGate(rules, authz, log.DefaultLogger)
}

func testItem(platform, method, host, author string) *contract.IntelItem {
	return &contract.IntelItem{
		IntelID: "it_test",
		Source: contract.IntelSource{
			Platform:      platform,
			CollectMethod: contract.CollectMethod(method),
			URL:           "https://" + host + "/news/1",
			Author:        author,
		},
		Payload: contract.IntelPayload{
			Title: "某平台发布新规定",
			Text:  "正文内容",
			Lang:  "zh",
		},
	}
}

func rule(id, typ, action, pattern string) *ComplianceRuleRow {
	return &ComplianceRuleRow{
		ID: id, RuleType: typ, Action: action, Pattern: pattern,
		Scope: "global", Enabled: true, Priority: 100,
	}
}

func TestGateRobotsThreeState(t *testing.T) {
	cs := []struct {
		name string
		gc   GateContext
		want string
	}{
		{"crawl + robots 不可得 → review（fail-closed）",
			GateContext{Method: "crawl"}, "review"},
		{"crawl + disallow → block",
			GateContext{Method: "crawl", RobotsChecked: true, RobotsAllowed: false}, "block"},
		{"crawl + allow → pass",
			GateContext{Method: "crawl", RobotsChecked: true, RobotsAllowed: true}, "pass"},
		{"crawl + 授权豁免 → pass",
			GateContext{Method: "crawl", RobotsExemptSource: true}, "pass"},
		{"hotlist 不适用 robots → pass",
			GateContext{Method: "hotlist"}, "pass"},
		{"rss 不适用 robots → pass",
			GateContext{Method: "rss"}, "pass"},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			g := newGateForTest(&stubRuleRepo{}, &stubAuthzRepo{})
			out, err := g.Evaluate(context.Background(), testItem("weibo", c.gc.Method, "example.com", ""), c.gc)
			if err != nil {
				t.Fatalf("evaluate err: %v", err)
			}
			if out.Gate != c.want {
				t.Fatalf("gate=%s want %s reasons=%v", out.Gate, c.want, out.Reasons)
			}
			if len(out.Reasons) == 0 {
				t.Fatal("reasons 不应为空（无旁路要求可解释）")
			}
		})
	}
}

func TestGateSensitiveBlockWins(t *testing.T) {
	rules := &stubRuleRepo{rows: []*ComplianceRuleRow{
		rule("cr_1", "sensitive_category", "review", "新规定"),
		rule("cr_2", "sensitive_category", "block", "发布新规定"),
	}}
	g := newGateForTest(rules, &stubAuthzRepo{})
	out, err := g.Evaluate(context.Background(), testItem("weibo", "hotlist", "example.com", ""),
		GateContext{Method: "hotlist"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Gate != "block" {
		t.Fatalf("严格者应胜出 block: %s", out.Gate)
	}
	if len(out.RuleIDs) != 2 {
		t.Fatalf("RuleIDs 应含全部命中: %v", out.RuleIDs)
	}
	if len(rules.hitBump) == 0 || len(rules.hitBump[0]) != 2 {
		t.Fatalf("命中计数未同步: %v", rules.hitBump)
	}
}

func TestGateRuleFetchFailureFailClosed(t *testing.T) {
	rules := &stubRuleRepo{err: errors.New("db down")}
	g := newGateForTest(rules, &stubAuthzRepo{})
	out, err := g.Evaluate(context.Background(), testItem("weibo", "hotlist", "example.com", ""),
		GateContext{Method: "hotlist"})
	if err == nil {
		t.Fatal("规则拉取失败必须上报 error（B-16）")
	}
	if out.Gate != "review" {
		t.Fatalf("错误时闸门必须 review 而非放行: %s", out.Gate)
	}
}

func TestGateScopePlatformMismatch(t *testing.T) {
	r := rule("cr_p", "sensitive_category", "block", "新规定")
	r.Scope = "platform"
	r.Platform = "douyin" // item 平台 weibo → 不命中
	g := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{r}}, &stubAuthzRepo{})
	out, err := g.Evaluate(context.Background(), testItem("weibo", "hotlist", "example.com", ""),
		GateContext{Method: "hotlist"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Gate != "pass" {
		t.Fatalf("平台作用域不命中应 pass: %s reasons=%v", out.Gate, out.Reasons)
	}
}

func TestGateAuthorizationLedger(t *testing.T) {
	ruleRow := rule("cr_a", "authorization", "review", "example.com")
	newAuthz := func(status string) *stubAuthzRepo {
		rows := map[string]*AuthorizationRecordRow{}
		if status != "" {
			rows["domain|example.com"] = &AuthorizationRecordRow{Subject: "domain", SubjectKey: "example.com", Status: status}
		}
		return &stubAuthzRepo{rows: rows}
	}

	cases := []struct {
		name   string
		status string // "" = 无记录
		want   string
	}{
		{"无台账 → review（未声明不等于放行）", "", "review"},
		{"granted → 不命中该规则", "granted", "pass"},
		{"denied → block", "denied", "block"},
		{"unknown → review", "unknown", "review"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newGateForTest(&stubRuleRepo{rows: []*ComplianceRuleRow{ruleRow}}, newAuthz(c.status))
			out, err := g.Evaluate(context.Background(), testItem("weibo", "hotlist", "example.com", ""),
				GateContext{Method: "hotlist"})
			if err != nil {
				t.Fatal(err)
			}
			if out.Gate != c.want {
				t.Fatalf("authz=%q gate=%s want %s reasons=%v", c.status, out.Gate, c.want, out.Reasons)
			}
		})
	}
}

func TestGateRightsHolderBlacklist(t *testing.T) {
	rules := &stubRuleRepo{rows: []*ComplianceRuleRow{
		rule("cr_r", "rights_holder_blacklist", "block", "example.com"),
	}}
	g := newGateForTest(rules, &stubAuthzRepo{})
	out, err := g.Evaluate(context.Background(), testItem("weibo", "hotlist", "example.com", "某作者"),
		GateContext{Method: "hotlist"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Gate != "block" {
		t.Fatalf("权利方黑名单应 block: %s", out.Gate)
	}
}
