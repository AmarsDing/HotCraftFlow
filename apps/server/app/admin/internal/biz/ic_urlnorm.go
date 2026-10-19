package biz

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// =============================================================================
// URL 归一化（03 §4.1：去追踪参数 / 尾斜杠 / 大小写；短链先展开由 usecase 经
// ShortlinkExpanderPort 完成后再调本函数）。纯函数、无网络，供单测覆盖。
// =============================================================================

// trackingQueryParams 精确命中即剔除的低信息参数。
var trackingQueryParams = map[string]struct{}{
	"spm": {}, "ved": {}, "feature": {}, "ref": {}, "ref_src": {},
	"wfr": {}, "from": {}, "source": {}, "share_token": {}, "fbclid": {},
	"gclid": {}, "dclid": {}, "yclid": {}, "mc_cid": {}, "mc_eid": {},
	"igshid": {}, "si": {}, "ab_channel": {},
}

// CanonicalizeURL 归一到可比形态：
//  - scheme/host 小写、去默认端口（:80/:443）
//  - 路径去尾斜杠（根路径保留 "/"）
//  - 剔除 utm_* 前缀与 trackingQueryParams 中的参数；其余参数按键名排序
//  - 丢弃 fragment
// 返回错误表示 URL 无法解析（调用方按其失败类别走 normalization 死信）。
func CanonicalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Host == "" {
		return "", fmt.Errorf("missing host: %s", raw)
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	u.User = nil
	u.Fragment = ""

	if u.EscapedPath() == "" {
		u.Path = "/"
		u.RawPath = ""
	} else {
		// 尾斜杠修剪必须作用在**已解码**的 u.Path 上（EscapedPath 是编码形态，
		// 直接赋给 u.Path 会让其中的字面 '%' 被二次转义成 %25——评审坐实：
		// /a%20b/ 归一出 /a%2520b，既与无尾斜杠形态 key 不同，落库 URL 本身也错）。
		p := u.Path
		if strings.HasSuffix(p, "/") && len(p) > 1 {
			p = strings.TrimSuffix(p, "/")
		}
		u.Path = p
		// RawPath 清空，让 String() 从解码后的 Path 统一重编码——编码形态唯一。
		u.RawPath = ""
	}

	if u.RawQuery != "" {
		q := u.Query()
		kept := make(url.Values, len(q))
		for k, vs := range q {
			lk := strings.ToLower(k)
			// url.Values 键区分大小写，聚合重复键后统一小写比较
			if strings.HasPrefix(lk, "utm_") {
				continue
			}
			if _, drop := trackingQueryParams[lk]; drop {
				continue
			}
			// M-8：签名/token 类参数必须剔除（脱敏 + 归一化双重收益）
			if lk == "token" || lk == "access_token" || lk == "signature" || lk == "sign" || strings.Contains(lk, "secret") {
				continue
			}
			kept[k] = append(kept[k], vs...)
		}
		keys := make([]string, 0, len(kept))
		for k := range kept {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for i, k := range keys {
			if i > 0 {
				b.WriteByte('&')
			}
			vs := kept[k]
			sort.Strings(vs)
			for j, v := range vs {
				if j > 0 {
					b.WriteByte('&')
				}
				b.WriteString(url.QueryEscape(k))
				b.WriteByte('=')
				b.WriteString(url.QueryEscape(v))
			}
		}
		u.RawQuery = b.String()
	}
	return u.String(), nil
}

// URLKey sha256(canonical)（03 §4.1 / 存库 url_key 字段）。
func URLKey(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

// NormalizeURLKey 一步到位：canonicalize + hash。空/非法 URL 返回错误。
func NormalizeURLKey(raw string) (urlKey string, canonical string, err error) {
	canonical, err = CanonicalizeURL(raw)
	if err != nil {
		return "", "", err
	}
	return URLKey(canonical), canonical, nil
}

// CycleBucketFor 采集周期桶 = floor(unix(collected_at)/(interval*60))*(interval*60)（评审 B-6）。
// intervalMinutes < 1 时按 1 分钟桶处理（防除零，配置校验已在 service 层拦截）。
func CycleBucketFor(collectedAt time.Time, intervalMinutes int32) int64 {
	if intervalMinutes < 1 {
		intervalMinutes = 1
	}
	span := int64(intervalMinutes) * 60
	u := collectedAt.UTC().Unix()
	return u / span * span
}

// ComputeDedupKey sha256(url_key|source_id|cycle_bucket)（B-6 幂等键）。
func ComputeDedupKey(urlKey, sourceID string, cycleBucket int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", urlKey, sourceID, cycleBucket)))
	return hex.EncodeToString(sum[:])
}

// IsURLCandidateForShortlink 常见的短链域名单；命中才调短链展开端口（免每 URL 一次 HTTP）。
var shortlinkHosts = map[string]struct{}{
	"t.co": {}, "bit.ly": {}, "tinyurl.com": {}, "goo.gl": {},
	"sina.lt": {}, "url.cn": {}, "dwz.cn": {}, "goo.su": {},
	"v.douyin.com": {}, "b23.tv": {}, "xhslink.com": {}, "kuaishou.com": {},
}

func IsURLCandidateForShortlink(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	_, ok := shortlinkHosts[host]
	return ok
}
