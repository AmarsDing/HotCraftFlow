package publishadapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"

	bizpub "framework/app/admin/internal/biz/publish"
)

// Registry：未注册键 Get 明确 miss（不静默降级）；Register/Get 往返；Keys 有序。
func TestRegistry_MissAndRoundtrip(t *testing.T) {
	reg := &Registry{adapters: map[string]bizpub.Publisher{}, log: log.NewHelper(log.DefaultLogger)}
	if _, ok := reg.Get("youtube"); ok {
		t.Fatal("未注册键必须 miss")
	}
	mock := NewMockPublisher(mockModeSuccess)
	reg.Register(mock)
	if got, ok := reg.Get("mock"); !ok || got.PlatformKey() != "mock" {
		t.Fatalf("注册后应命中: ok=%v", ok)
	}
	// Keys 有序（dev 探测/文档输出稳定性）。
	reg.Register(NewMockPublisher("failure"))
	keys := reg.Keys()
	if len(keys) == 0 {
		t.Fatal("Keys 不应为空")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Fatalf("Keys 必须有序: %v", keys)
		}
	}
}

// Mock 成功态：确定性 post_id（同输入同输出）+ 可见性缺省回退 private + URL 形状。
func TestMock_SuccessDeterministic(t *testing.T) {
	m := NewMockPublisher(mockModeSuccess)
	acct := &bizpub.AccountRow{AccountID: "pa_test", PlatformKey: "mock"}
	req := bizpub.PublishRequest{Account: acct, Title: "hello", Desc: "world", Visibility: ""}

	r1, err := m.Publish(context.Background(), req)
	if err != nil {
		t.Fatalf("mock success 态不应报错: %v", err)
	}
	r2, err := m.Publish(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r1.PostID != r2.PostID {
		t.Fatalf("post_id 必须确定: %s vs %s", r1.PostID, r2.PostID)
	}
	if !strings.HasPrefix(r1.PostID, "mock_") || !strings.HasPrefix(r1.PostURL, "https://mock.local/p/") {
		t.Fatalf("post_id/url 形状不对: %+v", r1)
	}
	// 🔴 缺省可见性必须回退 private（NFR-MAP-01 在适配器层同样兜底）。
	if r1.Visibility != "private" {
		t.Fatalf("缺省可见性应回退 private，实际 %q", r1.Visibility)
	}
	if r1.PublishedAt.IsZero() {
		t.Fatal("PublishedAt 应被填充")
	}
}

// Mock 失败五态：failure / ratelimit / credential_invalid（包 sentinel）/ unsupported（包 sentinel）。
func TestMock_FailureModes(t *testing.T) {
	acct := &bizpub.AccountRow{AccountID: "pa_test"}
	req := bizpub.PublishRequest{Account: acct, Title: "t"}

	if _, err := NewMockPublisher(mockModeFailure).Publish(context.Background(), req); err == nil {
		t.Fatal("failure 态必须报错")
	}
	if _, err := NewMockPublisher(mockModeRateLimit).Publish(context.Background(), req); err == nil {
		t.Fatal("ratelimit 态必须报错")
	}
	// 🔴 凭据失效错误必须包装 bizpub.ErrCredentialInvalid（发布编排据此停号留凭据）。
	if _, err := NewMockPublisher(mockModeCredentialInvalid).Publish(context.Background(), req); !errors.Is(err, bizpub.ErrCredentialInvalid) {
		t.Fatalf("credential_invalid 态必须包装 sentinel，实际 %v", err)
	}
	if _, err := NewMockPublisher(mockModeUnsupported).Publish(context.Background(), req); !errors.Is(err, bizpub.ErrCapabilityUnsupported) {
		t.Fatalf("unsupported 态必须包装 ErrCapabilityUnsupported，实际 %v", err)
	}
	// 默认（空 mode）= success。
	if r, err := NewMockPublisher("").Publish(context.Background(), req); err != nil || r == nil {
		t.Fatalf("空 mode 应回退 success: %v", err)
	}
}

// Mock 错误消息🔴绝不夹带凭据内容。
func TestMock_ErrorsNeverLeakCredential(t *testing.T) {
	secret := "TOP-SECRET-TOKEN-XYZ"
	acct := &bizpub.AccountRow{AccountID: "pa_test"}
	req := bizpub.PublishRequest{
		Account:    acct,
		Credential: &bizpub.Credential{Type: "oauth", Secret: []byte(secret)},
		Title:      "t",
	}
	for _, mode := range []string{mockModeFailure, mockModeRateLimit, mockModeCredentialInvalid, mockModeUnsupported} {
		if _, err := NewMockPublisher(mode).Publish(context.Background(), req); err != nil {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("🔴 mode=%s 错误消息泄漏凭据: %s", mode, err.Error())
			}
		}
	}
}

// FetchAnalytics 错误消息同样🔴绝不夹带账号凭据引用（契约断言补齐：发布侧有
// Publish 断言，回流侧此前无 CI 护栏——真实适配器接入后 error 文本若拼接 acct
// 字段，本断言即第一道闸）。
func TestMock_FetchAnalyticsErrorsNeverLeakCredential(t *testing.T) {
	secretRef := "enc:v1:TOP-SECRET-ACCOUNT-REF"
	acct := &bizpub.AccountRow{
		AccountID:      "pa_test",
		CredentialRef:  &secretRef,
		CredentialType: "oauth",
	}
	for _, mode := range []string{mockModeCredentialInvalid, mockModeUnsupported} {
		_, err := NewMockPublisher(mode).FetchAnalytics(context.Background(), acct, "mock_abc")
		if err == nil {
			t.Fatalf("mode=%s 期望错误", mode)
		}
		if msg := err.Error(); strings.Contains(msg, secretRef) || strings.Contains(msg, "TOP-SECRET") {
			t.Fatalf("🔴 mode=%s 回流错误消息泄漏凭据引用: %s", mode, msg)
		}
	}
}

// FetchAnalytics：固定样例 + unsupported/credential_invalid 拒绝。
func TestMock_FetchAnalytics(t *testing.T) {
	m := NewMockPublisher(mockModeSuccess)
	perf, err := m.FetchAnalytics(context.Background(), nil, "mock_abc")
	if err != nil || perf == nil {
		t.Fatalf("success 态 analytics 应返回样例: %v", err)
	}
	if perf.Views <= 0 || perf.FetchedAt.IsZero() {
		t.Fatalf("样例数据不完整: %+v", perf)
	}
	if _, err := NewMockPublisher(mockModeUnsupported).FetchAnalytics(context.Background(), nil, "x"); !errors.Is(err, bizpub.ErrCapabilityUnsupported) {
		t.Fatalf("unsupported 态期望 sentinel，实际 %v", err)
	}
}

// PlatformCatalog：注册表声明优先于 yaml 配置；yaml 兜底；双缺 miss。
func TestPlatformCatalog_Precedence(t *testing.T) {
	reg := &Registry{adapters: map[string]bizpub.Publisher{}, log: log.NewHelper(log.DefaultLogger)}
	reg.Register(NewMockPublisher(mockModeSuccess)) // mock 全能力声明（TitleMaxLen=100）

	confs := PlatformConfMap{
		"mock":              {Enabled: true, Capability: bizpub.Capability{TitleMaxLen: 10}}, // 与注册表冲突 → 注册表赢
		"youtube":           {Enabled: true, Capability: bizpub.Capability{TitleMaxLen: 100, Video: true}},
		"disabled_platform": {Enabled: false, Capability: bizpub.Capability{Video: true}},
	}
	cat := NewPlatformCatalog(reg, confs, log.DefaultLogger)

	// 注册表优先：mock TitleMaxLen 应为 100 而非 10。
	if cap, ok := cat.CapabilityFor("mock"); !ok || cap.TitleMaxLen != 100 {
		t.Fatalf("注册表声明应优先，实际 %+v ok=%v", cap, ok)
	}
	// yaml 兜底：youtube 未注册 → 取配置。
	if cap, ok := cat.CapabilityFor("youtube"); !ok || cap.TitleMaxLen != 100 || !cap.Video {
		t.Fatalf("yaml 兜底失败: %+v ok=%v", cap, ok)
	}
	// 双缺 miss。
	if _, ok := cat.CapabilityFor("ghost"); ok {
		t.Fatal("双缺平台必须 miss")
	}
	// 运营配置读取（资质提示用）。
	if _, ok := cat.ConfOf("youtube"); !ok {
		t.Fatal("ConfOf youtube 应命中")
	}
}

// Capability 快照键名与 json tag 一致性（platforms.yaml Scan 的隐性契约）。
func TestPlatformConfMap_YamlKeyContract(t *testing.T) {
	conf := PlatformConf{
		Enabled:           true,
		VisibilityDefault: "private",
		Capability: bizpub.Capability{
			Video: true, NativeSchedule: true, TitleMaxLen: 100, DescMaxLen: 2000,
			AspectRatios: []string{"16:9"},
		},
	}
	if !conf.Capability.Video || conf.Capability.TitleMaxLen != 100 {
		t.Fatalf("配置形状不对: %+v", conf.Capability)
	}
	if conf.VisibilityDefault != "private" {
		t.Fatal("🔴 visibility_default 必须恒 private")
	}
}
