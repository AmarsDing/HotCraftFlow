package iccollector

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data"
)

// =============================================================================
// robots 三态检查（S-7 + RFC 9309 + B-16 语义钉）
//
// 五分类矩阵（http 状态 → 三态结论）与「判定结论不缓存、原文缓存」纪律：
// 每条路径结论必须从新解析中得来（路径级规则不容近似）。
// =============================================================================

func newRobotsCheckerForTest(ua string) *RobotsChecker {
	cfg := biz.ResolveIcConfig(nil)
	if ua != "" {
		cfg.RobotsUserAgent = ua
	}
	return NewRobotsChecker(cfg, &data.RedisClient{}, log.NewStdLogger(io.Discard)) // 无 Redis → 进程内缓存兜底
}

func TestRobotsCheckMatrix(t *testing.T) {
	pathDisallow := "/private/zone"
	bodies := map[string]string{
		"deny_path": "User-agent: *\nDisallow: /private\n",
		"allow_all": "User-agent: *\nDisallow:\n",
		"group_miss": "User-agent: SomeOtherBot\nDisallow: /everything\n",
		"delay":     "User-agent: *\nCrawl-delay: 3\nDisallow:\n",
		"garbage":   "Disallow *\n", // 指令先于 User-agent → robotstxt 解析失败
	}
	status := 200
	bodyKind := "allow_all"
	var hits int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		if r.URL.Path != "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		if status == 200 {
			_, _ = w.Write([]byte(bodies[bodyKind]))
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	cs := []struct {
		name                                 string
		httpStatus                           int
		bodyKind                             string
		path                                 string
		wantAllowed, wantFound               bool
	}{
		{"200 + 路径禁抓 → 不允许且已声明", 200, "deny_path", pathDisallow, false, true},
		{"200 + 公开路径 → 允许且已声明", 200, "deny_path", "/public/story", true, true},
		{"200 + 空规则 → 允许且已声明", 200, "allow_all", "/anything", true, true},
		{"200 + 组不匹配 → 允许（无适用组即无限制）", 200, "group_miss", "/everything/else", true, true},
		{"404 → 站点未声明 robots，允许但不构成「取不到」", 404, "", "/private/zone", true, true},
		{"403 → RFC: unavailable = disallow all", 403, "", "/whatever", false, true},
		{"500 → 取不到，found=false（闸门 review 语义）", 500, "", "/whatever", false, false},
		{"200 + 本体无法解析 → 保守取不到", 200, "garbage", "/whatever", false, false},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			status, bodyKind = c.httpStatus, c.bodyKind
			checker := newRobotsCheckerForTest("") // 每案新实例 = 冷缓存，隔离状态类
			allowed, _, found, err := checker.Check(ctx, "crawl", srv.URL+c.path, false)
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if allowed != c.wantAllowed || found != c.wantFound {
				t.Fatalf("got allowed=%v found=%v, want %v/%v", allowed, found, c.wantAllowed, c.wantFound)
			}
		})
	}

	t.Run("Crawl-delay 透传毫秒", func(t *testing.T) {
		status, bodyKind = 200, "delay"
		checker := newRobotsCheckerForTest("")
		allowed, delay, found, err := checker.Check(ctx, "crawl", srv.URL+"/x", false)
		if err != nil || !allowed || !found {
			t.Fatalf("allowed=%v found=%v err=%v", allowed, found, err)
		}
		if delay != 3000 {
			t.Fatalf("crawl-delay 3s 应为 3000ms: %d", delay)
		}
	})

	t.Run("401 同 403 语义（全禁）", func(t *testing.T) {
		status = 401
		checker := newRobotsCheckerForTest("")
		allowed, _, found, _ := checker.Check(ctx, "crawl", srv.URL+"/x", false)
		if allowed || !found {
			t.Fatalf("401 应全禁且已声明: allowed=%v found=%v", allowed, found)
		}
	})

	t.Run("豁免短路（S-8）：不发请求直接放行", func(t *testing.T) {
		before := atomic.LoadInt64(&hits)
		status = 500 // 即便站点 500 也不该被碰到
		checker := newRobotsCheckerForTest("")
		allowed, delay, found, err := checker.Check(ctx, "crawl", srv.URL+"/anything", true)
		if err != nil || !allowed || !found || delay != 0 {
			t.Fatalf("豁免语义: allowed=%v delay=%d found=%v err=%v", allowed, delay, found, err)
		}
		if atomic.LoadInt64(&hits) != before {
			t.Fatal("豁免不得发起 robots 抓取")
		}
	})

	t.Run("原文缓存：二次查询不同路径不再外呼，结论各自现判", func(t *testing.T) {
		status, bodyKind = 200, "deny_path"
		checker := newRobotsCheckerForTest("")
		before := atomic.LoadInt64(&hits)
		a1, _, _, err1 := checker.Check(ctx, "crawl", srv.URL+pathDisallow, false)
		a2, _, _, err2 := checker.Check(ctx, "crawl", srv.URL+"/public/story", false)
		if err1 != nil || err2 != nil {
			t.Fatalf("check err: %v/%v", err1, err2)
		}
		if got := atomic.LoadInt64(&hits) - before; got != 1 {
			t.Fatalf("缓存纪律失守：两次 Check 应只外呼一次 robots.txt，实际 %d 次", got)
		}
		if a1 || !a2 {
			t.Fatalf("同一份 robots 原文须按各自路径现判: a1=%v a2=%v", a1, a2)
		}
	})

	t.Run("非法 URL → 错误上抛", func(t *testing.T) {
		checker := newRobotsCheckerForTest("")
		if _, _, _, err := checker.Check(ctx, "crawl", "://not-a-url", false); err == nil {
			t.Fatal("非法 URL 必须报错（上游数据异常）")
		}
		if _, _, _, err := checker.Check(ctx, "crawl", "", false); err == nil {
			t.Fatal("空 URL 必须报错")
		}
	})

	t.Run("网络层故障 → 取不到（found=false）", func(t *testing.T) {
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		deadURL := dead.URL
		dead.Close() // 立即断供
		checker := newRobotsCheckerForTest("")
		allowed, _, found, err := checker.Check(ctx, "crawl", deadURL+"/x", false)
		if err != nil {
			t.Fatalf("网络故障不应报错（三态语义由 found 承担）: %v", err)
		}
		if allowed || found {
			t.Fatalf("取不到必须 found=false 且 allowed=false: %v/%v", allowed, found)
		}
	})

	t.Run("TTL 兜底：零值配置落 24h", func(t *testing.T) {
		cfg := biz.ResolveIcConfig(nil)
		cfg.RobotsTTL = 0
		checker := NewRobotsChecker(cfg, &data.RedisClient{}, log.NewStdLogger(io.Discard))
		if checker.ttl != 24*time.Hour {
			t.Fatalf("TTL 兜底漂移: %v", checker.ttl)
		}
	})
}
