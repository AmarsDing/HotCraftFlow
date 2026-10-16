package serviceauth

import (
	"context"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/http"

	authnEngine "github.com/tx7do/kratos-authn/engine"
)

const reason = "SERVICE_TOKEN_FORBIDDEN"

var (
	// ErrServiceTokenDenied 服务令牌访问未登记规则的操作（拒绝默认）。
	ErrServiceTokenDenied = errors.Forbidden(reason, "service token not allowed on this operation")
	// ErrServiceTokenScopeMissing 服务令牌缺少目标操作所需 scope。
	ErrServiceTokenScopeMissing = errors.Forbidden(reason, "service token missing required scope")
	// ErrMissingClaims 中间件装配错误：上游缺少 authn claims。
	ErrMissingClaims = errors.Unauthorized(reason, "missing authn claims in context")
)

// Rule 服务令牌访问规则：HTTP 方法 + 路由模板 → 所需 scope。
type Rule struct {
	Method string // GET / POST / ...
	Path   string // kratos 路由模板（如 /api/v1/monitor/assets/{id}/full）
	Scope  string // 访问该操作所需的 scope
}

// ruleTable (method, pathTemplate) → 所需 scope 的查找表。
type ruleTable map[string]string

func newRuleTable(rules []Rule) ruleTable {
	table := make(ruleTable, len(rules))
	for _, r := range rules {
		table[r.Method+" "+r.Path] = r.Scope
	}
	return table
}

// check 服务令牌访问决策（纯函数，便于单测）：
// scp 含通配 "*" 直接放行（仅 gateway 代签 SYS_ADMIN 绑定 API key，等价绑定用户本人）；
// 否则命中规则要求 scp 含所需 scope；未命中任何规则一律拒绝（拒绝默认）。
func (t ruleTable) check(claims *authnEngine.AuthClaims, method, path string) error {
	scopes, _ := claims.GetStrings(ClaimFieldScope)
	if HasScope(scopes, ScopeWildcard) {
		return nil
	}
	required, registered := t[method+" "+path]
	if !registered {
		return ErrServiceTokenDenied
	}
	if HasScope(scopes, required) {
		return nil
	}
	return ErrServiceTokenScopeMissing
}

// Enforce 服务令牌最小权限强制中间件。
//
// 行为：
//   - 用户令牌（uid != 0）：直接放行（鉴权由 casbin 承担）；
//   - 服务令牌（uid == 0）：按 rules 匹配 (method, pathTemplate)，
//     命中则要求 scp 含规则 scope 或通配 "*"；未命中任何规则一律拒绝（拒绝默认）。
//
// 装配位置：authn.Server 之后（依赖 AuthClaims 已注入 ctx），与白名单 selector 同域。
// 设计动机：服务令牌以共享密钥现签、uid=0 跳过黑名单、roc=super 过 casbin——
// 若不按 scope 围栏，任一服务令牌即获目标服务全量权限（含用户/角色管理等写操作）。
func Enforce(logger log.Logger, rules ...Rule) middleware.Middleware {
	l := log.NewHelper(log.With(logger, "module", "serviceauth/enforce"))
	table := newRuleTable(rules)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			claims, ok := authnEngine.AuthClaimsFromContext(ctx)
			if !ok {
				l.Errorf("authn claims missing, check middleware order (Enforce must run after authn.Server)")
				return nil, ErrMissingClaims
			}
			if !IsServiceToken(claims) {
				return handler(ctx, req) // 用户令牌：casbin 鉴权
			}

			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				// 该分支为服务端装配错误（Enforce 须挂在 HTTP/gRPC server 中间件链上）
				return nil, ErrMissingClaims
			}
			method, path := "", tr.Operation()
			if htr, ok := tr.(*http.Transport); ok {
				method = htr.Request().Method
				path = htr.PathTemplate()
			} else if tr.Kind() == transport.KindGRPC {
				// gRPC：无 HTTP 方法语义，method 固定 "GRPC"；path=Operation 全方法名
				// （/package.Service/Method）。规则查找键 "GRPC /pkg.Svc/Method"，
				// 未登记规则与 HTTP 一致走拒绝默认。
				method = "GRPC"
			}

			if err := table.check(claims, method, path); err != nil {
				sub, _ := claims.GetSubject()
				l.Warnf("service token denied: sub=%s %s %s err=%v", sub, method, path, err)
				return nil, err
			}
			return handler(ctx, req)
		}
	}
}
