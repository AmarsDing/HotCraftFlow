package biz

import (
	"context"
	"errors"
	"testing"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// FetchURL（FR-INT-04 手贴续跑，02 §6.2）：
//   合成信源不入场、治理不旁路（robots 三态 / 平台共享限流桶 / 哈希链 / 幂等键）。
//   platformForURL：契约平台词表闭集（§1 禁造词）+ host 后缀防误命中。
// =============================================================================

// newFetchFixture：crawl/api 两路注册同一只可编程采集器。
func newFetchFixture() *collectFixture {
	f := newCollectFixture("crawl")
	f.reg.by["api"] = f.collector
	f.collector.items = []*RawCollectItem{collectTestItem("https://weibo.com/a/1")}
	f.collector.lastSrc = nil
	f.collector.calls = 0
	return f
}

func TestFetchURL(t *testing.T) {
	ctx := context.Background()

	t.Run("method 词表仅收 api|crawl", func(t *testing.T) {
		for _, m := range []string{"hotlist", "rss", "sidecar", ""} {
			f := newFetchFixture()
			if _, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", m); !errors.Is(err, ErrIcFetchMethod) {
				t.Fatalf("method=%q 哨兵口径: %v", m, err)
			}
		}
	})

	t.Run("URL 无法解析", func(t *testing.T) {
		f := newFetchFixture()
		if _, err := f.u.FetchURL(ctx, "https://bad host.com/x", "crawl"); err == nil {
			t.Fatal("非法 URL 必须报错")
		}
	})

	t.Run("词表外平台不得造词", func(t *testing.T) {
		f := newFetchFixture()
		if _, err := f.u.FetchURL(ctx, "https://example.com/news/1", "crawl"); !errors.Is(err, ErrIcPlatformUnknown) {
			t.Fatalf("哨兵口径: %v", err)
		}
		if f.collector.calls != 0 {
			t.Fatal("平台未知应在采集前被拒")
		}
	})

	t.Run("采集器未注册", func(t *testing.T) {
		f := newCollectFixture("hotlist") // 只注册 hotlist
		if _, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", "crawl"); !errors.Is(err, ErrIcCollectorUnavailable) {
			t.Fatalf("哨兵口径: %v", err)
		}
	})

	t.Run("手采命中平台限流桶（S-10 不豁免）", func(t *testing.T) {
		f := newFetchFixture()
		f.limiter.denials["ic:source:manual:fetch"] = true
		if _, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", "crawl"); !errors.Is(err, ErrIcFetchRateLimited) {
			t.Fatalf("哨兵口径: %v", err)
		}
		// 合成信源未配速率 → 信源桶取默认；双键序均留证。
		want := []string{"ic:platform:weibo=20", "ic:source:manual:fetch=20"}
		if len(f.limiter.calls) != 2 {
			t.Fatalf("限流调用面: %v", f.limiter.calls)
		}
		for i, w := range want {
			if f.limiter.calls[i] != w {
				t.Fatalf("限流口径: got=%v want=%v", f.limiter.calls, want)
			}
		}
	})

	t.Run("采集故障上抛", func(t *testing.T) {
		f := newFetchFixture()
		f.collector.err = errors.New("crawl4ai 500")
		if _, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", "crawl"); err == nil {
			t.Fatal("采集故障必须上抛")
		}
	})

	t.Run("空返回即 EMPTY_CONTENT", func(t *testing.T) {
		f := newFetchFixture()
		f.collector.items = nil
		if _, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", "api"); !errors.Is(err, ErrIcFetchEmpty) {
			t.Fatalf("哨兵口径: %v", err)
		}
	})

	t.Run("全条目落库失败透传首个错误", func(t *testing.T) {
		f := newFetchFixture()
		f.store.failOnContains = "FAILME"
		bad := collectTestItem("https://weibo.com/a/1")
		bad.Fragment.Content = []byte("FAILME")
		f.collector.items = []*RawCollectItem{bad}
		_, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", "crawl")
		if err == nil || err == ErrIcFetchEmpty {
			t.Fatalf("应透传落盘错误而非空内容哨兵: %v", err)
		}
	})

	t.Run("详情读回失败上抛", func(t *testing.T) {
		f := newFetchFixture()
		f.intel.failGet = true
		if _, err := f.u.FetchURL(ctx, "https://weibo.com/a/1", "crawl"); err == nil {
			t.Fatal("GetByID 故障必须上抛")
		}
	})

	t.Run("正常：合成信源语义 + robots 走查 + 幂等重放", func(t *testing.T) {
		f := newFetchFixture()
		row, err := f.u.FetchURL(ctx, "https://weibo.com/a/1?utm_source=x", "crawl")
		if err != nil || row == nil {
			t.Fatalf("手采: row=%v err=%v", row, err)
		}
		// 合成信源：不入场、无 run 开账；目标数组形状喂给 crawl 采集器。
		got := f.collector.lastSrc
		if got == nil || got.ID != "manual:fetch" || got.Method != string(contract.CollectCrawl) {
			t.Fatalf("合成信源: %+v", got)
		}
		if targets, ok := got.Config["targets"].([]string); !ok || len(targets) != 1 {
			t.Fatalf("crawl 路 targets 合约: %+v", got.Config)
		}
		if got.Config["url"] != row.Source.URL {
			t.Fatalf("合成 url 应等于归一后 canonical: %q vs %q", got.Config["url"], row.Source.URL)
		}
		if len(f.runs.created) != 0 {
			t.Fatalf("手采不入运行账本（审计面由证据快照承担）: %v", f.runs.created)
		}
		if len(f.robots.calls) != 1 {
			t.Fatalf("crawl 手采必须走 robots 三态: %v", f.robots.calls)
		}
		if row.PublishState != "pending" || row.SourceID != "manual:fetch" {
			t.Fatalf("读回行面: %+v", row)
		}

		// 幂等重放：dedup_key 命中 → 指向同一行、不重复发布。
		again, err := f.u.FetchURL(ctx, "https://weibo.com/a/1?utm_source=x", "crawl")
		if err != nil || again.IntelID != row.IntelID {
			t.Fatalf("幂等: %q vs %q err=%v", again.IntelID, row.IntelID, err)
		}
		if len(f.pub.calls) != 1 { // 首次 1 条发布；重放 duplicate 不再发布
			t.Fatalf("重放不得重复发布: %v", f.pub.calls)
		}
	})
}

// TestPlatformForURL host 后缀匹配矩阵：
// 词表闭集 + 等值/子域命中 + 「点前缀防缩」（ax.com 不得命中 x.com）。
func TestPlatformForURL(t *testing.T) {
	cases := []struct {
		name, in, platform, region string
		ok                           bool
	}{
		{"主域", "https://weibo.com/hot", "weibo", "cn", true},
		{"子域", "https://m.weibo.com/detail/1", "weibo", "cn", true},
		{"深子域", "https://a.b.x.com/u/1", "x", "intl", true},
		{"大小写主机", "HTTPS://WWW.ZHIHU.COM/question/1", "zhihu", "cn", true},
		{"点前缀防缩·ax≠x", "https://ax.com/u/1", "", "", false},
		{"点前缀防缩·evilweibo", "https://evilweibo.com/x", "", "", false},
		{"短域 youtu.be", "https://youtu.be/abc", "youtube", "intl", true},
		{"推特旧域归一", "https://mobile.twitter.com/i/1", "x", "intl", true},
		{"频道双栈 weixin.qq.com", "https://weixin.qq.com/channels/1", "weixin_channels", "cn", true},
		{"telegram 两域 t.me", "https://t.me/s/news", "telegram", "intl", true},
		{"telegram 两域 telegram.me", "https://core.telegram.me/x", "telegram", "intl", true},
		{"HN 专用子域", "https://news.ycombinator.com/item?id=1", "hackernews", "intl", true},
		{"词表外", "https://example.com/1", "", "", false},
		{"词表外近形·ycombinator 根域", "https://ycombinator.com/1", "", "", false},
		{"空主机", "notaurl", "", "", false},
		{"解析失败", "://bad", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			platform, region, ok := platformForURL(c.in)
			if ok != c.ok || platform != c.platform || region != c.region {
				t.Fatalf("platformForURL(%q) = (%q, %q, %v), want (%q, %q, %v)",
					c.in, platform, region, ok, c.platform, c.region, c.ok)
			}
		})
	}
}
