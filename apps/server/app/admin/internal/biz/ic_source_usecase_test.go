package biz

import (
	"errors"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// =============================================================================
// 信源规格校验（S-20 kind×method 矩阵 + S-18 interval 下限 + S-8 豁免纪律）
// 这些矩阵是合规边界的可执行规格——改动前先过这里。
// =============================================================================

// repo 不参与校验路径，nil 安全（校验只走静态矩阵）。
func newSourceUC() *IcSourceUsecase { return NewIcSourceUsecase(nil, log.DefaultLogger) }

func TestValidateSourceSpecMatrix(t *testing.T) {
	uc := newSourceUC()
	legal := []struct{ kind, method string }{
		{"hotlist", "hotlist"},
		{"hotlist", "api"},
		{"keyword", "api"}, {"keyword", "crawl"}, {"keyword", "hotlist"},
		{"creator", "api"}, {"creator", "rss"}, {"creator", "crawl"},
		{"rss", "rss"},
		{"competitor", "api"}, {"competitor", "rss"}, {"competitor", "crawl"},
	}
	for _, c := range legal {
		if err := uc.ValidateSourceSpec(c.kind, c.method, 99999, false, ""); err != nil {
			t.Fatalf("合法组合被判非法 %s×%s: %v", c.kind, c.method, err)
		}
	}
	// sidecar 是矩阵内合法组合，但永远要求豁免链条（S-20）——带全豁免参数才放行
	if err := uc.ValidateSourceSpec("creator", "sidecar", 99999, true, "授权台账 A-2026-001"); err != nil {
		t.Fatalf("creator×sidecar + 完整授权链应放行: %v", err)
	}

	illegal := []struct{ kind, method string }{
		{"hotlist", "crawl"}, {"hotlist", "rss"}, {"hotlist", "sidecar"},
		{"keyword", "sidecar"}, {"keyword", "rss"},
		{"creator", "hotlist"},
		{"rss", "api"}, {"rss", "crawl"},
		{"competitor", "hotlist"}, {"competitor", "sidecar"},
		{"unknown_kind", "api"},
	}
	for _, c := range illegal {
		if err := uc.ValidateSourceSpec(c.kind, c.method, 99999, false, ""); !errors.Is(err, ErrIcKindMethod) {
			t.Fatalf("非法组合未拦截 %s×%s: %v", c.kind, c.method, err)
		}
	}
}

func TestValidateSourceSpecIntervalFloor(t *testing.T) {
	uc := newSourceUC()
	cases := []struct {
		method   string
		ok, ceil int32 // ok=刚好达标, ceil=低于下限必拒
	}{
		{"hotlist", 30, 29}, {"rss", 5, 4}, {"api", 10, 9}, {"crawl", 60, 59}, {"sidecar", 1, 0},
	}
	for _, c := range cases {
		kind := kindForMethod(c.method)
		// sidecar 永远要求豁免链（floor 测试带全豁免参数聚焦下限语义）
		exempt, reason := false, ""
		if c.method == "sidecar" {
			exempt, reason = true, "授权台账 A-2026-001"
		}
		if err := uc.ValidateSourceSpec(kind, c.method, c.ok, exempt, reason); err != nil {
			t.Fatalf("interval=%d（=下限）应通过 %s: %v", c.ok, c.method, err)
		}
		if err := uc.ValidateSourceSpec(kind, c.method, c.ceil, exempt, reason); !errors.Is(err, ErrIcIntervalFloor) {
			t.Fatalf("interval=%d（<下限）应拒 %s: %v", c.ceil, c.method, err)
		}
	}
}

// kindForMethod 测试辅助：选一个该方法合法的 kind。
func kindForMethod(method string) string {
	for kind, methods := range icKindMethodMatrix {
		if methods[method] {
			return kind
		}
	}
	return "hotlist"
}

func TestValidateSourceSpecSidecarDiscipline(t *testing.T) {
	uc := newSourceUC()
	// sidecar 未豁免 → 拒绝（S-20：sidecar 必须授权）
	if err := uc.ValidateSourceSpec("creator", "sidecar", 1, false, ""); !errors.Is(err, ErrIcSidecarNeedsAuth) {
		t.Fatalf("sidecar 无豁免应拒: %v", err)
	}
	// sidecar + 豁免但无原因 → 拒绝（S-8 审计）
	if err := uc.ValidateSourceSpec("creator", "sidecar", 1, true, ""); !errors.Is(err, ErrIcRobotsExemptWhy) {
		t.Fatalf("豁免无原因应拒: %v", err)
	}
	// sidecar + 豁免 + 原因 → 放行
	if err := uc.ValidateSourceSpec("creator", "sidecar", 1, true, "权利方书面授权 A-2026-001"); err != nil {
		t.Fatalf("完整授权链应放行: %v", err)
	}
	// 非 sidecar 也不必豁免；但豁免即需原因（任意 method）
	if err := uc.ValidateSourceSpec("rss", "rss", 5, true, ""); !errors.Is(err, ErrIcRobotsExemptWhy) {
		t.Fatalf("任意 method 豁免无原因都应拒: %v", err)
	}
}
