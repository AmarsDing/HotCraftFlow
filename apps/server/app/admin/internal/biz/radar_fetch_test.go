package biz

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestFetchTrending_ErrorPath 验证：当双引擎都失败时，FetchTrending 返回错误，
// 并且 ListLatest 的 error 分支正确走 snapshot 回退或返回 stale 空结果。
func TestFetchTrending_ErrorPath(t *testing.T) {
	c := NewGitHubClient(nil)
	// 强制：超时 1 微秒 → html 和 api 引擎都会立刻超时
	c.http.Timeout = 1 * time.Nanosecond

	entries, err := c.FetchTrending(context.Background(), "daily", "")
	if err == nil {
		t.Fatalf("expected error from both engines, got entries=%d", len(entries))
	}
	t.Logf("got expected error: %v", err)

	// 现在 ListLatest：无 snapshot → 应该 stale=true, items=nil, 不 panic
	reader := NewGitHubRadarTrendingReader(c)
	res, _ := reader.ListLatest(context.Background(), RadarListQuery{Since: "today"})
	if !res.Stale {
		t.Fatalf("expected Stale=true when fetch fails and no snapshot, got stale=%v", res.Stale)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected empty items, got %d", len(res.Items))
	}
}

// TestFetchTrending_CachePoisoning 验证：scrape 失败不会把空结果写入缓存。
func TestFetchTrending_CachePoisoning(t *testing.T) {
	c := NewGitHubClient(nil)
	c.http.Timeout = 1 * time.Nanosecond
	reader := NewGitHubRadarTrendingReader(c)

	// 第一次调用失败
	res1, _ := reader.ListLatest(context.Background(), RadarListQuery{Since: "today"})
	if !res1.Stale {
		t.Fatal("expected stale on first call")
	}

	// 第二次调用（若被缓存了空值，这里就不会是 stale，而是直接返回空 items）
	res2, _ := reader.ListLatest(context.Background(), RadarListQuery{Since: "today"})
	if !res2.Stale {
		t.Fatal("expected stale on second call too — indicates empty result was cached")
	}
}

var _ = errors.New
