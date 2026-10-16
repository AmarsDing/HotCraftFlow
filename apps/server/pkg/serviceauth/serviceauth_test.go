package serviceauth

import (
	"testing"

	authnEngine "github.com/tx7do/kratos-authn/engine"

	pkgjwt "framework/pkg/jwt"
)

func TestHasScope(t *testing.T) {
	scopes := []string{"admin:asset:read", "admin:user:read"}
	if !HasScope(scopes, "admin:asset:read") {
		t.Fatal("expected scope hit")
	}
	if HasScope(scopes, "admin:asset:write") {
		t.Fatal("expected scope miss")
	}
	if !HasScope([]string{ScopeWildcard}, ScopeWildcard) {
		t.Fatal("expected wildcard present")
	}
}

func TestIsServiceToken(t *testing.T) {
	svc := &authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(0)}
	usr := &authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(7)}
	if !IsServiceToken(svc) {
		t.Fatal("uid=0 should be service token")
	}
	if IsServiceToken(usr) {
		t.Fatal("uid=7 should not be service token")
	}
}

func TestParse(t *testing.T) {
	claims := &authnEngine.AuthClaims{
		pkgjwt.ClaimFieldUserName: "voice-service",
		pkgjwt.ClaimFieldUserID:   uint32(0),
		ClaimFieldAudience:        "admin",
		ClaimFieldScope:           []string{"admin:asset:alias"},
	}
	tok, err := Parse(claims)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tok.Subject != "voice-service" || tok.Audience != "admin" {
		t.Fatalf("unexpected token: %+v", tok)
	}
	if !HasScope(tok.Scopes, "admin:asset:alias") {
		t.Fatalf("scopes not parsed: %+v", tok.Scopes)
	}
}

func TestRuleTableCheck(t *testing.T) {
	table := newRuleTable([]Rule{
		{Method: "GET", Path: "/api/v1/monitor/alarm/active-count", Scope: ScopeAdminAssetRead},
	})

	// 命中规则 + scope 匹配 → 放行
	claimsOK := &authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(0), ClaimFieldScope: []string{ScopeAdminAssetRead}}
	if err := table.check(claimsOK, "GET", "/api/v1/monitor/alarm/active-count"); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}

	// 命中规则 + 通配 → 放行
	claimsWild := &authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(0), ClaimFieldScope: []string{ScopeWildcard}}
	if err := table.check(claimsWild, "GET", "/api/v1/monitor/alarm/active-count"); err != nil {
		t.Fatalf("expected wildcard allow, got %v", err)
	}

	// 通配 + 未登记路由 → 放行（gateway 代签 SYS_ADMIN 绑定 API key 语义）
	if err := table.check(claimsWild, "GET", "/api/v1/monitor/roles"); err != nil {
		t.Fatalf("expected wildcard allow on unregistered route, got %v", err)
	}

	// 命中规则 + scope 缺失 → 拒绝
	claimsMiss := &authnEngine.AuthClaims{pkgjwt.ClaimFieldUserID: uint32(0), ClaimFieldScope: []string{"admin:user:read"}}
	if err := table.check(claimsMiss, "GET", "/api/v1/monitor/alarm/active-count"); err != ErrServiceTokenScopeMissing {
		t.Fatalf("expected ErrServiceTokenScopeMissing, got %v", err)
	}

	// 未命中规则 → 拒绝默认
	if err := table.check(claimsOK, "POST", "/api/v1/monitor/users"); err != ErrServiceTokenDenied {
		t.Fatalf("expected ErrServiceTokenDenied, got %v", err)
	}
}
