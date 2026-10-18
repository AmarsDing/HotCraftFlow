package biz

import (
	"context"
	"testing"
	"time"
)

func TestParseGhCount(t *testing.T) {
	cases := map[string]int64{
		"1,234": 1234, "132k": 132000, "12.5k": 12500,
		"1.2M": 1200000, "": 0, " 42 ": 42,
	}
	for in, want := range cases {
		if got := parseGhCount(in); got != want {
			t.Errorf("parseGhCount(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestWindowStart(t *testing.T) {
	day := windowStart("today")
	week := windowStart("week")
	month := windowStart("month")
	if !week.Before(day) || !month.Before(week) {
		t.Fatalf("window order wrong: day=%v week=%v month=%v", day, week, month)
	}
	if d := time.Since(day); d < 20*time.Hour || d > 28*time.Hour {
		t.Errorf("today window start off: %v", day)
	}
}

func TestSortRadarItems(t *testing.T) {
	items := []RadarRepoView{
		{FullName: "a/b", Stars: 10, StarsDelta: 5, IsNew: false, PushedAt: "2026-01-01T00:00:00Z"},
		{FullName: "c/d", Stars: 100, StarsDelta: 1, IsNew: true, PushedAt: "2026-02-01T00:00:00Z"},
	}
	sortRadarItems(items, "hot")
	if items[0].FullName != "a/b" {
		t.Errorf("hot sort want a/b first, got %s", items[0].FullName)
	}
	sortRadarItems(items, "stars")
	if items[0].FullName != "c/d" {
		t.Errorf("stars sort want c/d first, got %s", items[0].FullName)
	}
	sortRadarItems(items, "updated")
	if items[0].FullName != "c/d" {
		t.Errorf("updated sort want c/d first, got %s", items[0].FullName)
	}
	sortRadarItems(items, "new")
	if items[0].FullName != "c/d" || !items[0].IsNew {
		t.Errorf("new sort want c/d first, got %s", items[0].FullName)
	}
}

func TestValidateFullName(t *testing.T) {
	if _, err := validateFullName("vercel/next.js"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	for _, bad := range []string{"", "noslash", "a/b/c", " / ", "a/../etc", "a/b?x=1"} {
		if _, err := validateFullName(bad); err == nil {
			t.Errorf("invalid name %q accepted", bad)
		}
	}
}

func TestMatchesQuery(t *testing.T) {
	it := RadarRepoView{FullName: "vercel/next.js", Description: "The React Framework", Topics: []string{"react", "ssr"}}
	for _, q := range []string{"next", "react", "framework", "VERCEL"} {
		if !matchesQuery(it, q) {
			t.Errorf("query %q should match", q)
		}
	}
	if matchesQuery(it, "kubernetes") {
		t.Error("query kubernetes should not match")
	}
}

// TestFetchTrendingSmoke 真实网络冒烟（无 token 也能跑；失败仅提示不判失败，
// 因为 CI / 无外网环境应跳过）。
func TestFetchTrendingSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	c := NewGitHubClient(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	entries, err := c.FetchTrending(ctx, "today", "")
	if err != nil {
		t.Skipf("github unreachable from this env: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("empty trending entries")
	}
	first := entries[0]
	t.Logf("top1: %s stars=%d delta=%d lang=%s", first.FullName, first.Stars, first.StarsDelta, first.Language)
	if first.FullName == "" {
		t.Error("missing full_name")
	}
}

func TestGetRepoDetailSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	c := NewGitHubClient(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	v, err := c.GetRepoDetail(ctx, "vercel/next.js")
	if err != nil {
		t.Skipf("github unreachable from this env: %v", err)
	}
	if v.Stars <= 0 || v.License == "" || v.DefaultBranch == "" {
		t.Errorf("detail incomplete: %+v", v)
	}
}
