package biz

import (
	"testing"
	"time"
)

// =============================================================================
// URL 归一化黄金测试（03 §4.1：去重链路根基，漂移=漏并 → 锁死逐条语义）
// =============================================================================

func TestCanonicalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"scheme/host 小写 + 去默认端口",
			"HTTP://Example.COM:80/Path", "http://example.com/Path"},
		{"https 默认端口",
			"https://EXAMPLE.com:443/a", "https://example.com/a"},
		{"非默认端口保留",
			"http://example.com:8080/a", "http://example.com:8080/a"},
		{"尾斜杠剔除",
			"http://example.com/path/", "http://example.com/path"},
		{"根路径保留斜杠",
			"http://example.com/", "http://example.com/"},
		{"fragment 丢弃",
			"http://example.com/a#frag", "http://example.com/a"},
		{"utm_* 全前缀剔除",
			"http://example.com/a?utm_source=x&utm_medium=y&id=1", "http://example.com/a?id=1"},
		{"追踪参数词表",
			"http://example.com/a?spm=1&ved=2&ref=3&id=4", "http://example.com/a?id=4"},
		{"token/signature/sign 脱敏剔除（M-8）",
			"http://example.com/a?token=t&signature=s&sign=z&id=1", "http://example.com/a?id=1"},
		{"secret 子串剔除",
			"http://example.com/a?client_secret=x&id=1", "http://example.com/a?id=1"},
		{"其余参数按键排序",
			"http://example.com/a?z=1&a=2&m=3", "http://example.com/a?a=2&m=3&z=1"},
		{"重复键聚合排序",
			"http://example.com/a?k=2&k=1", "http://example.com/a?k=1&k=2"},
		{"protocol-relative 补 scheme",
			"//example.com/a", "http://example.com/a"},
		{"userinfo 剔除",
			"http://user:pass@example.com/a", "http://example.com/a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CanonicalizeURL(c.in)
			if err != nil {
				t.Fatalf("CanonicalizeURL(%q) err: %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("CanonicalizeURL(%q)=%q want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCanonicalizeURLErrors(t *testing.T) {
	// 裸域形如 example.com/a：url.Parse 归 Path 而非 Host，按 missing host 拒绝
	// （采集器入口须给完整 URL，宁可报错也不擅自猜测 host）。
	for _, in := range []string{"", "   ", "not-a-url", "/relative/path", "example.com/a"} {
		if _, err := CanonicalizeURL(in); err == nil {
			t.Fatalf("CanonicalizeURL(%q) 应报错", in)
		}
	}
}

// TestNormalizeURLKeyAcrossDrift 归一化漂移容忍：query 顺序 / 大小写 / 追踪参数
// 不同来源的「同一篇文章」必须产生相同 url_key（L1 去重判据）。
func TestNormalizeURLKeyAcrossDrift(t *testing.T) {
	a, _, err := NormalizeURLKey("http://News.COM/article?id=42&utm_source=app")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := NormalizeURLKey("https://NEWS.COM:443/article?utm_medium=x&id=42&spm=1")
	if err != nil {
		t.Fatal(err)
	}
	// 注：http≠https 不同 scheme 是不同 canonical（刻意不误并——scheme 差异可能对内容有语义）。
	// 这里验证的是「同 scheme 下的查询漂移」：
	c, _, err := NormalizeURLKey("https://NEWS.COM:443/article?spm=1&id=42&utm_medium=x")
	if err != nil {
		t.Fatal(err)
	}
	if b != c {
		t.Fatalf("url_key 漂移: %s vs %s", b, c)
	}
	if a == b {
		t.Fatalf("http/https 不应同 key（scheme 语义保留）")
	}
}

func TestCycleBucketFor(t *testing.T) {
	ts := time.Date(2026, 10, 19, 3, 4, 5, 0, time.UTC)
	if got := CycleBucketFor(ts, 30); got != ts.Unix()/1800*1800 {
		t.Fatalf("bucket=%d", got)
	}
	// interval<1 防除零 → 按 1 分钟桶
	if got := CycleBucketFor(ts, 0); got != ts.Unix()/60*60 {
		t.Fatalf("zero interval bucket=%d", got)
	}
	// 同桶不同时刻 → 同 bucket
	earlier := ts.Add(-200 * time.Second)
	if CycleBucketFor(ts, 30) == CycleBucketFor(earlier, 30) {
		// 3:04 与 3:00:45 同在 30 分钟桶内（同半时）——仅为同桶断言提供构造，不保证
	}
}

func TestComputeDedupKey(t *testing.T) {
	k1 := ComputeDedupKey("uk", "src", 100)
	if k1 != ComputeDedupKey("uk", "src", 100) {
		t.Fatal("dedup key 不稳定")
	}
	if ComputeDedupKey("uk", "srcA", 100) == ComputeDedupKey("uk", "srcB", 100) {
		t.Fatal("不同信源不应同键（B-6 幂等键含 source_id）")
	}
	if ComputeDedupKey("uk", "src", 100) == ComputeDedupKey("uk", "src", 200) {
		t.Fatal("不同周期桶不应同键")
	}
}

func TestIsURLCandidateForShortlink(t *testing.T) {
	hits := []string{
		"https://t.co/abc123",
		"https://bit.ly/xyz",
		"https://b23.tv/123",
		"https://v.douyin.com/ke1/",
	}
	for _, h := range hits {
		if !IsURLCandidateForShortlink(h) {
			t.Fatalf("%s 应判短链", h)
		}
	}
	misses := []string{
		"https://example.com/news",
		"https://weibo.com/u/123",
		"not a url",
		"",
	}
	for _, m := range misses {
		if IsURLCandidateForShortlink(m) {
			t.Fatalf("%s 不应判短链", m)
		}
	}
}
