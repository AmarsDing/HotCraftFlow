package serviceauth

import (
	"context"
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	authnEngine "github.com/tx7do/kratos-authn/engine"

	pkgjwt "framework/pkg/jwt"
)

// enforce_test.go Enforce 中间件单测：fake transport.Transporter（非 http）覆盖 gRPC 分支
// （真实 grpc.Transport 字段未导出，包外无法构造，故以 Kind()=KindGRPC 桩替代）。
// HTTP 分支行为零变化，既有 TestRuleTableCheck（纯函数 check）继续覆盖，不在此重复。

// fakeTransporter kind/operation 可控的 Transporter 桩。
type fakeTransporter struct {
	kind      transport.Kind
	operation string
}

func (f *fakeTransporter) Kind() transport.Kind            { return f.kind }
func (f *fakeTransporter) Endpoint() string                { return "" }
func (f *fakeTransporter) Operation() string               { return f.operation }
func (f *fakeTransporter) RequestHeader() transport.Header { return nil }
func (f *fakeTransporter) ReplyHeader() transport.Header   { return nil }

// runEnforce 以给定 claims + transporter 执行 Enforce 中间件，返回下游 handler 是否被放行及错误。
func runEnforce(t *testing.T, rules []Rule, claims *authnEngine.AuthClaims, tr transport.Transporter) (bool, error) {
	t.Helper()
	ctx := authnEngine.ContextWithAuthClaims(context.Background(), claims)
	if tr != nil {
		ctx = transport.NewServerContext(ctx, tr)
	}
	called := false
	h := Enforce(log.NewStdLogger(io.Discard), rules...)(func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "ok", nil
	})
	_, err := h(ctx, nil)
	return called, err
}

func TestEnforceGRPC(t *testing.T) {
	const op = "/opstools.v1.OpsTools/ExecuteCommand"
	rules := []Rule{{Method: "GRPC", Path: op, Scope: ScopeAdminAssetRead}}
	grpcTr := &fakeTransporter{kind: transport.KindGRPC, operation: op}
	svcClaims := func(scopes ...string) *authnEngine.AuthClaims {
		return &authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(0), ClaimFieldScope: scopes}
	}

	// 命中 GRPC 规则 + scope 匹配 → 放行
	called, err := runEnforce(t, rules, svcClaims(ScopeAdminAssetRead), grpcTr)
	if !called || err != nil {
		t.Fatalf("GRPC 命中应放行: called=%v err=%v", called, err)
	}

	// 通配 scope → 放行
	called, err = runEnforce(t, rules, svcClaims(ScopeWildcard), grpcTr)
	if !called || err != nil {
		t.Fatalf("通配 scope 应放行: called=%v err=%v", called, err)
	}

	// 命中规则 + scope 缺失 → ErrServiceTokenScopeMissing
	called, err = runEnforce(t, rules, svcClaims(ScopeAdminUserRead), grpcTr)
	if called || err != ErrServiceTokenScopeMissing {
		t.Fatalf("scope 缺失应拒绝: called=%v err=%v", called, err)
	}

	// 未登记的 gRPC 方法 → 拒绝默认（与 HTTP 一致）
	called, err = runEnforce(t, rules, svcClaims(ScopeAdminAssetRead),
		&fakeTransporter{kind: transport.KindGRPC, operation: "/opstools.v1.OpsTools/Unregistered"})
	if called || err != ErrServiceTokenDenied {
		t.Fatalf("未登记应拒绝默认: called=%v err=%v", called, err)
	}

	// 用户令牌（uid != 0）经 gRPC transport → 直接放行（鉴权由 casbin 承担，与 transport 无关）
	called, err = runEnforce(t, rules,
		&authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(7)}, grpcTr)
	if !called || err != nil {
		t.Fatalf("用户令牌应放行: called=%v err=%v", called, err)
	}

	// 无 transport 上下文（服务端装配错误）→ ErrMissingClaims
	called, err = runEnforce(t, rules, svcClaims(ScopeWildcard), nil)
	if called || err != ErrMissingClaims {
		t.Fatalf("缺 transport 应报装配错误: called=%v err=%v", called, err)
	}
}
