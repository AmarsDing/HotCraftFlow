package biz

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 合规闸门（FR-INT-12 🔴 无旁路 / B-16 fail-closed / S-9 操作审计）
// 规则四类：sensitive_category | rights_holder_blacklist | authorization | robots | platform_tos
// 判定纪律：严格者胜出（block > review > pass）；任何评估错误 → review，
// 绝不静默放行。输出附 RuleIDs（可解释，命中计数同步）。
// =============================================================================

// GateContext 判定上下文（robots 结论来自取证快照，S-7；豁免来自信源 S-8）。
type GateContext struct {
	RobotsChecked      bool
	RobotsAllowed      bool // robots 三态：未声明=false（found=false ⇒ checked=false）
	RobotsExemptSource bool // 信源 robots_exempt=true（service 层已强制要求 reason 与授权台账）
	Method             string
}

// GateOutcome 闸门结论（reasons 携带可见名与规则 ID，落库 intel.compliance_reasons）。
type GateOutcome struct {
	Gate    string   // pass|review|block
	Reasons []string // 形如 "robots:目标站点 disallow" / "<rule_id>:敏感类目命中"
	RuleIDs []string // 命中规则 ID（审计 + IncrementHitCount）
}

// GovernanceGate 注入规则的执行器。
type GovernanceGate struct {
	rules ComplianceRuleRepo
	authz AuthorizationRecordRepo
	log   *log.Helper
}

func NewGovernanceGate(rules ComplianceRuleRepo, authz AuthorizationRecordRepo, logger log.Logger) *GovernanceGate {
	return &GovernanceGate{
		rules: rules,
		authz: authz,
		log:   log.NewHelper(log.With(logger, "module", "ic/governance/biz/admin-service")),
	}
}

// gateRank 严格度排序。
func gateRank(g string) int {
	switch g {
	case "block":
		return 3
	case "review":
		return 2
	case "pass":
		return 1
	}
	return 0
}

// strictest 合成最终闸门。
func strictest(gates []string) string {
	best := "pass"
	for _, g := range gates {
		if gateRank(g) > gateRank(best) {
			best = g
		}
	}
	return best
}

// Evaluate 全量评估。返回 (outcome, error)：error 非空时 outcome 一定为 review
// （B-16 fail-closed：错误不放过行），调用方知晓故障并告警即可复用 outcome。
func (g *GovernanceGate) Evaluate(ctx context.Context, item *contract.IntelItem, gc GateContext) (*GateOutcome, error) {
	var firstErr error
	markErr := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	out := &GateOutcome{Gate: "pass"}
	var gates []string

	// ---- robots 三态（B-16；先于规则表，物理合规优先于运营规则）----
	robotsGate := g.evalRobots(gc, &out.Reasons)
	gates = append(gates, robotsGate)

	// ---- 规则表（enabled + 生效窗口内，按 priority 降序）----
	rules, err := g.rules.ListEffective(ctx, time.Now().UTC())
	if err != nil {
		markErr(err)
		gates = append(gates, "review") // 规则拉取失败 → fail-closed
		out.Reasons = append(out.Reasons, "internal:规则查询失败")
	}

	for _, rule := range rules {
		if !scopeMatch(rule, item) {
			continue
		}
		gate, reason, matched, e := g.evalRule(ctx, rule, item)
		if e != nil {
			markErr(e)
		}
		if !matched {
			continue
		}
		if gate == "" { // 模式等配置错误 → 保守 review（配置错误不等于放行）
			gate = "review"
			reason = "config:规则模式非法"
		}
		gates = append(gates, gate)
		out.Reasons = append(out.Reasons, rule.ID+":"+reason)
		out.RuleIDs = append(out.RuleIDs, rule.ID)
	}

	out.Gate = strictest(gates)

	// 命中计数（审计；失败仅记日志不阻塞闸门结论）
	if len(out.RuleIDs) > 0 {
		if err := g.rules.IncrementHitCount(ctx, out.RuleIDs); err != nil {
			g.log.Warnf("increment rule hit count failed: %s", err.Error())
		}
	}
	if out.Gate == "pass" && len(out.Reasons) == 0 {
		out.Reasons = append(out.Reasons, "pass:未命中任何阻断/复核规则")
	}
	return out, firstErr
}

// evalRobots robots 三态判定（S-7/S-8/B-16）。
func (g *GovernanceGate) evalRobots(gc GateContext, reasons *[]string) string {
	if gc.Method != "crawl" && gc.Method != "api" {
		return "pass" // hotlist/rss/侧车：robots 不适用（RSSHub/newsnow 自部署合规已在部署门禁）
	}
	if gc.RobotsExemptSource {
		*reasons = append(*reasons, "robots:授权豁免(S-8)")
		return "pass"
	}
	switch {
	case !gc.RobotsChecked:
		// 取不到 robots.txt → 未声明语义（非放行）：复核（fail-closed）
		*reasons = append(*reasons, "robots:robots.txt 不可得，按未声明处理")
		return "review"
	case !gc.RobotsAllowed:
		*reasons = append(*reasons, "robots:目标站点 disallow")
		return "block"
	}
	return "pass"
}

// scopeMatch 规则作用域：global 恒中；platform 要求平台一致。
func scopeMatch(rule *ComplianceRuleRow, item *contract.IntelItem) bool {
	if rule.Scope == "platform" {
		return rule.Platform != "" && rule.Platform == item.Source.Platform
	}
	return true
}

type ruleEvaluator func(ctx context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (gate, reason string, matched bool, err error)

// evalRule 按类型分派。
func (g *GovernanceGate) evalRule(ctx context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (string, string, bool, error) {
	var eval ruleEvaluator
	switch rule.RuleType {
	case "sensitive_category":
		eval = g.evalSensitive
	case "rights_holder_blacklist":
		eval = g.evalRightsHolder
	case "authorization":
		eval = g.evalAuthorization
	case "robots":
		eval = g.evalRobotsRule
	case "platform_tos":
		eval = g.evalPlatformTOS
	default:
		return "", "", false, nil // 未知类型跳过（规则类型白名单）
	}
	return eval(ctx, rule, item)
}

// matchPattern 统一模式匹配：优先 regexp，编译失败退化子串（配置错误可按 review 兜底在上层处理）。
func matchPattern(pattern, haystack string) (bool, error) {
	if pattern == "" {
		return false, nil
	}
	re, err := regexp.Compile(pattern)
	if err == nil {
		return re.MatchString(haystack), nil
	}
	// 退化子串匹配，并把编译错误上报（上层 rating review）
	return strings.Contains(haystack, pattern), err
}

func (g *GovernanceGate) evalSensitive(_ context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (string, string, bool, error) {
	hay := item.Payload.Title + "\n" + item.Payload.Text
	ok, err := matchPattern(rule.Pattern, hay)
	return rule.Action, "敏感类目命中", ok, err
}

func (g *GovernanceGate) evalRightsHolder(_ context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (string, string, bool, error) {
	host := ""
	if u, err := url.Parse(item.Source.URL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	hay := host + "\n" + strings.ToLower(item.Source.Author)
	ok, err := matchPattern(strings.ToLower(rule.Pattern), hay)
	return rule.Action, "权利方黑名单命中", ok, err
}

// evalAuthorization B-14 #12：pattern 先限定适用范围（对 host/author/asset 引用匹配），
// 命中后查台账：granted → pass（不构成命中）；denied → block；
// unknown/过期/无记录 → review（B-16 fail-closed：未声明授权不等于放行）。
func (g *GovernanceGate) evalAuthorization(ctx context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (string, string, bool, error) {
	subjects := authorizationSubjectsOf(item)
	for _, s := range subjects {
		ok, err := matchPattern(strings.ToLower(rule.Pattern), strings.ToLower(s.key))
		if err != nil {
			return "", "", true, err
		}
		if !ok {
			continue
		}
		rec, qerr := g.authz.Get(ctx, s.subject, s.key, time.Now().UTC())
		if qerr != nil {
			return "", "", false, qerr
		}
		switch {
		case rec == nil:
			return "review", "授权无台账记录（" + s.subject + ":" + s.key + "）", true, nil
		case rec.Status == "granted":
			continue
		case rec.Status == "denied":
			return "block", "授权状态 denied（" + s.subject + ":" + s.key + "）", true, nil
		default: // unknown（含过期，data 层已折算）
			return "review", "授权状态 unknown（" + s.subject + ":" + s.key + "）", true, nil
		}
	}
	return "", "", false, nil
}

type authSubject struct{ subject, key string }

// authorizationSubjectsOf 生成主体查询序列：domain 与 author（PIPL：author 仅昵称）。
func authorizationSubjectsOf(item *contract.IntelItem) []authSubject {
	var out []authSubject
	if u, err := url.Parse(item.Source.URL); err == nil && u.Hostname() != "" {
		out = append(out, authSubject{subject: "domain", key: strings.ToLower(u.Hostname())})
	}
	if item.Source.Author != "" {
		out = append(out, authSubject{subject: "author", key: item.Source.Author})
	}
	for _, ref := range item.Payload.MediaRefs {
		if strings.HasPrefix(ref, "asset://") {
			out = append(out, authSubject{subject: "asset", key: ref})
		}
	}
	return out
}

func (g *GovernanceGate) evalRobotsRule(_ context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (string, string, bool, error) {
	host := ""
	if u, err := url.Parse(item.Source.URL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	ok, err := matchPattern(strings.ToLower(rule.Pattern), host)
	// robots 类型规则用于运营声明某些 host 的特殊策略（如强制 review）
	return rule.Action, "robots 规则命中", ok, err
}

func (g *GovernanceGate) evalPlatformTOS(_ context.Context, rule *ComplianceRuleRow, item *contract.IntelItem) (string, string, bool, error) {
	host := ""
	if u, err := url.Parse(item.Source.URL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	hay := item.Source.Platform + "\n" + host + "\n" + string(item.Source.CollectMethod)
	ok, err := matchPattern(strings.ToLower(rule.Pattern), strings.ToLower(hay))
	return rule.Action, "平台 ToS 规则命中", ok, err
}
