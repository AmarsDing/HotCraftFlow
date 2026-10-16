package logging

import (
	"net/http"
	"testing"
)

// resetTrustedProxies 测试后还原全局可信代理配置（atomic.Value 包级共享，避免用例间串扰）。
func resetTrustedProxies(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetTrustedProxies(nil) })
}

func newRequest(remoteAddr string, headers map[string]string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// TestGetClientRealIP_LegacyTrustXFF 未配置可信代理时保持旧行为：优先采纳 XFF。
func TestGetClientRealIP_LegacyTrustXFF(t *testing.T) {
	resetTrustedProxies(t)
	SetTrustedProxies(nil)

	r := newRequest("10.0.0.1:12345", map[string]string{
		HeaderKeyXForwardedFor: "1.2.3.4, 10.0.0.1",
	})
	if got := GetClientRealIP(r); got != "1.2.3.4" {
		t.Errorf("legacy mode should honor XFF, got %q", got)
	}
}

// TestGetClientRealIP_UntrustedPeerIgnoresXFF 启用可信校验后，直连对端不在可信网段时忽略 XFF（防伪造）。
func TestGetClientRealIP_UntrustedPeerIgnoresXFF(t *testing.T) {
	resetTrustedProxies(t)
	SetTrustedProxies([]string{"172.18.0.0/16"})

	r := newRequest("203.0.113.9:54321", map[string]string{
		HeaderKeyXForwardedFor: "1.2.3.4",
		HeaderKeyXRealIP:       "1.2.3.4",
	})
	if got := GetClientRealIP(r); got != "203.0.113.9" {
		t.Errorf("untrusted peer should fall back to RemoteAddr, got %q", got)
	}
}

// TestGetClientRealIP_TrustedPeerHonorsXFF 直连对端为可信代理时采纳 XFF 首个合法 IP。
func TestGetClientRealIP_TrustedPeerHonorsXFF(t *testing.T) {
	resetTrustedProxies(t)
	SetTrustedProxies([]string{"172.18.0.0/16", "127.0.0.1"})

	r := newRequest("172.18.0.2:8080", map[string]string{
		HeaderKeyXForwardedFor: "1.2.3.4, 172.18.0.2",
	})
	if got := GetClientRealIP(r); got != "1.2.3.4" {
		t.Errorf("trusted peer should honor XFF, got %q", got)
	}
}

// TestGetClientRealIP_TrustedPeerXFFEmptyFallsBackXRealIP 可信代理 + 无 XFF 时采纳 X-Real-IP。
func TestGetClientRealIP_TrustedPeerXFFEmptyFallsBackXRealIP(t *testing.T) {
	resetTrustedProxies(t)
	SetTrustedProxies([]string{"172.18.0.0/16"})

	r := newRequest("172.18.0.2:8080", map[string]string{
		HeaderKeyXRealIP: "5.6.7.8",
	})
	if got := GetClientRealIP(r); got != "5.6.7.8" {
		t.Errorf("trusted peer should honor X-Real-IP, got %q", got)
	}
}

// TestGetClientRealIP_TrustedPeerNoHeadersFallsBackRemoteAddr 可信代理但无任何头部时回退 RemoteAddr。
func TestGetClientRealIP_TrustedPeerNoHeadersFallsBackRemoteAddr(t *testing.T) {
	resetTrustedProxies(t)
	SetTrustedProxies([]string{"172.18.0.0/16"})

	r := newRequest("172.18.0.2:8080", nil)
	if got := GetClientRealIP(r); got != "172.18.0.2" {
		t.Errorf("no headers should fall back to RemoteAddr, got %q", got)
	}
}

// TestSetTrustedProxies_Parsing 纯 IP / CIDR / 空白 / 非法项的解析与开关语义。
func TestSetTrustedProxies_Parsing(t *testing.T) {
	resetTrustedProxies(t)

	SetTrustedProxies(nil)
	if TrustedProxyCheckEnabled() {
		t.Error("nil config should disable trusted proxy check")
	}

	SetTrustedProxies([]string{"", "  ", "not-an-ip"})
	if TrustedProxyCheckEnabled() {
		t.Error("all-invalid config should disable trusted proxy check")
	}

	SetTrustedProxies([]string{" 192.168.4.0/24 ", "127.0.0.1"})
	if !TrustedProxyCheckEnabled() {
		t.Error("valid CIDR + plain IP should enable trusted proxy check")
	}

	// 纯 IP 按 host 网段处理
	r := newRequest("127.0.0.1:9000", map[string]string{HeaderKeyXForwardedFor: "9.9.9.9"})
	if got := GetClientRealIP(r); got != "9.9.9.9" {
		t.Errorf("plain-IP trusted proxy should honor XFF, got %q", got)
	}
}
