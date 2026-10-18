package iccollector

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
)

// =============================================================================
// 短链展开（S-6）：手工跟随重定向，每一跳解析 IP 命中私网/回环/链路本地/保留段即拒
// （评审 gap#16：展开任意 URL 是典型内网探测入口，block_private_ip 恒真不配置化）。
// =============================================================================

// privateNets SSRF 拦截段（IPv4 + IPv6 全口径）。
var privateNets = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16",
		"198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

func isPrivateIP(ip net.IP) bool {
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ShortlinkExpander biz.ShortlinkExpanderPort 实现。
type ShortlinkExpander struct {
	maxRedirects int
	timeout      time.Duration
	ua           string
	log          *log.Helper
}

func NewShortlinkExpander(cfg *biz.IcConfig, logger log.Logger) *ShortlinkExpander {
	maxR := int(cfg.ShortlinkMaxRedirects)
	if maxR <= 0 {
		maxR = 5
	}
	return &ShortlinkExpander{
		maxRedirects: maxR,
		timeout:      cfg.RobotsTimeout, // 复用短超时档位（无独立配置位；短链服务理应秒回）
		ua:           cfg.RobotsUserAgent,
		log:          log.NewHelper(log.With(logger, "module", "ic/shortlink")),
	}
}

// Expand 跟随到最终 URL。SSRF 拦截返回 biz.ErrURLBlocked 包装。
func (e *ShortlinkExpander) Expand(ctx context.Context, rawURL string) (string, error) {
	current := rawURL
	for hop := 0; hop <= e.maxRedirects; hop++ {
		if err := e.assertPublic(ctx, current); err != nil {
			return "", err
		}
		final, next, err := e.headOnce(ctx, current)
		if err != nil {
			return "", fmt.Errorf("expand hop %d: %w", hop, err)
		}
		if next == "" {
			return final, nil
		}
		current = next
	}
	return "", fmt.Errorf("%w: redirects > %d", biz.ErrURLBlocked, e.maxRedirects)
}

// assertPublic 解析 URL 主机 IP 并拦截私网（DNS rebinding 防护：每跳重解析）。
func (e *ShortlinkExpander) assertPublic(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("bad url: %s", rawURL)
	}
	host := u.Hostname()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("dns lookup failed for %s: %w", host, err)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("%w: %s resolves to private ip %s", biz.ErrURLBlocked, host, ip.String())
		}
	}
	return nil
}

// headOnce 单跳 HEAD（服务端不支持 HEAD 时回退 GET 不读体）。
func (e *ShortlinkExpander) headOnce(ctx context.Context, rawURL string) (final string, next string, err error) {
	client := &http.Client{
		Timeout: e.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // 手工跟随
		},
	}
	for _, method := range []string{http.MethodHead, http.MethodGet} {
		req, rerr := http.NewRequestWithContext(ctx, method, rawURL, nil)
		if rerr != nil {
			return "", "", rerr
		}
		req.Header.Set("User-Agent", e.ua)
		resp, derr := client.Do(req)
		if derr != nil {
			return "", "", derr
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			if loc == "" {
				return rawURL, "", nil
			}
			// 相对跳转解析
			base, berr := url.Parse(rawURL)
			if berr != nil {
				return "", "", berr
			}
			ref, rerr2 := url.Parse(loc)
			if rerr2 != nil {
				return "", "", rerr2
			}
			return "", base.ResolveReference(ref).String(), nil
		}
		if method == http.MethodHead && resp.StatusCode == http.StatusMethodNotAllowed {
			continue // 回退 GET
		}
		return rawURL, "", nil
	}
	return rawURL, "", nil
}

var _ biz.ShortlinkExpanderPort = (*ShortlinkExpander)(nil)
