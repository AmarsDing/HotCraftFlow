package iccollector

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/temoto/robotstxt"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data"
)

// =============================================================================
// robots 三态检查（S-7）：allowed / crawl_delay_ms / found。
// 语义钉（RFC 9309 + 评审 B-16）：
//   200 且解析成功 → found=true 按规则判定（路径级 grp.Test）
//   404            → 站点未声明 robots，found=true, allowed=true（不构成"取不到"）
//   401/403        → found=true, allowed=false（RFC: unavailable = disallow all）
//   其余错误/超时  → found=false（"未声明"语义 → 闸门 review，绝不视为放行）
// 缓存键按 host 存 robots.txt 本体（首字节状态类 + 原文），每次调用原地解析判定路径，
// 不缓存判定结论——路径级规则不容近似。Redis（TTL=cfg，多实例共享）+ 进程内兜底。
// =============================================================================

const (
	robotsClassOK      = '1' // 200 拿到本体
	robotsClassAbsent  = '2' // 404：无限制
	robotsClassDenyAll = '3' // 401/403：全禁
	robotsClassUnknown = '0' // 取不到/解析失败
)

// robotsCacheEntry 进程内缓存项。
type robotsCacheEntry struct {
	class     byte
	body      []byte
	expiresAt time.Time
}

// RobotsChecker biz.RobotsCheckerPort 实现。
type RobotsChecker struct {
	http  *httpFetcher
	redis *data.RedisClient
	ttl   time.Duration
	ua    string
	mu    sync.Mutex
	mem   map[string]robotsCacheEntry
	log   *log.Helper
}

func NewRobotsChecker(cfg *biz.IcConfig, rc *data.RedisClient, logger log.Logger) *RobotsChecker {
	ttl := cfg.RobotsTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &RobotsChecker{
		http:  newHTTPFetcher(cfg.RobotsTimeout, cfg.RobotsUserAgent, bootRobotsBodyLimit),
		redis: rc,
		ttl:   ttl,
		ua:    cfg.RobotsUserAgent,
		mem:   map[string]robotsCacheEntry{},
		log:   log.NewHelper(log.With(logger, "module", "ic/robots")),
	}
}

// bootRobotsBodyLimit robots.txt 抓取上限（RFC 9309 解析至少前 500KiB；取保守 256KiB）。
const bootRobotsBodyLimit = 256 << 10

// Check 实现 biz.RobotsCheckerPort。robotsExempt=true 直接放行（S-8 授权豁免；
// 豁免合法性在信源层强校验，这里只负责三态语义不掺判定）。
func (c *RobotsChecker) Check(ctx context.Context, method, rawURL string, exempt bool) (bool, int32, bool, error) {
	if exempt {
		return true, 0, true, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false, 0, false, fmt.Errorf("robots: bad url: %s", rawURL)
	}
	host := u.Scheme + "://" + u.Host

	ent, ok := c.loadCache(ctx, host)
	if !ok {
		ent = c.probe(ctx, host)
		c.storeCache(ctx, host, ent)
	}

	switch ent.class {
	case robotsClassAbsent:
		return true, 0, true, nil
	case robotsClassDenyAll:
		return false, 0, true, nil
	case robotsClassUnknown:
		return false, 0, false, nil
	}

	robots, err := robotstxt.FromBytes(ent.body)
	if err != nil {
		c.log.Warnf("robots parse failed (%s): %s", host, err.Error())
		return false, 0, false, nil // 解析失败按取不到（保守）
	}
	grp := robots.FindGroup(c.ua)
	if grp == nil { // 无匹配组 = 无限制
		return true, 0, true, nil
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !grp.Test(path) {
		return false, int32(grp.CrawlDelay / time.Millisecond), true, nil
	}
	return true, int32(grp.CrawlDelay / time.Millisecond), true, nil
}

// probe 抓取 robots.txt（仅此一次外呼；缓存由调用方负责写入）。
func (c *RobotsChecker) probe(ctx context.Context, host string) robotsCacheEntry {
	frag, err := c.http.fetch(ctx, host+"/robots.txt", nil)
	if err != nil {
		c.log.Warnf("robots fetch failed (%s): %s", host, err.Error())
		return robotsCacheEntry{class: robotsClassUnknown}
	}
	switch {
	case frag.HTTPStatus == 401 || frag.HTTPStatus == 403:
		return robotsCacheEntry{class: robotsClassDenyAll}
	case frag.HTTPStatus == 404:
		return robotsCacheEntry{class: robotsClassAbsent}
	case frag.HTTPStatus < 200 || frag.HTTPStatus >= 300:
		return robotsCacheEntry{class: robotsClassUnknown} // 5xx 等 = 取不到
	}
	return robotsCacheEntry{class: robotsClassOK, body: frag.Content}
}

// ---------- 缓存 ----------

func (c *RobotsChecker) loadCache(ctx context.Context, host string) (robotsCacheEntry, bool) {
	if c.redis.IsConfigured() {
		if v, err := c.redis.Client().Get(ctx, "ic:robots:"+host).Bytes(); err == nil && len(v) >= 1 {
			return robotsCacheEntry{class: v[0], body: v[1:]}, true
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ent, ok := c.mem[host]; ok && time.Now().Before(ent.expiresAt) {
		return ent, true
	}
	return robotsCacheEntry{}, false
}

func (c *RobotsChecker) storeCache(ctx context.Context, host string, ent robotsCacheEntry) {
	if c.redis.IsConfigured() {
		v := make([]byte, 0, len(ent.body)+1)
		v = append(v, ent.class)
		v = append(v, ent.body...)
		if err := c.redis.Client().Set(ctx, "ic:robots:"+host, v, c.ttl).Err(); err != nil {
			c.log.Warnf("robots cache write failed: %s", err.Error())
		}
	}
	c.mu.Lock()
	ent.expiresAt = time.Now().Add(c.ttl)
	c.mem[host] = ent
	c.mu.Unlock()
}

var _ biz.RobotsCheckerPort = (*RobotsChecker)(nil)
