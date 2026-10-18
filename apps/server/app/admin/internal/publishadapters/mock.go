package publishadapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	bizpub "framework/app/admin/internal/biz/publish"
)

// MockPlatformKey mock 平台键（dev/test 联调全链路用；生产环境不应建 mock 渠道）。
const MockPlatformKey = "mock"

// mockModeEnv 行为开关环境变量（五态驱动，便于联调与验收脚本覆盖失败路径）。
// 取值：success（默认）/ failure / ratelimit / credential_invalid / unsupported。
const mockModeEnv = "MAP_MOCK_MODE"

// Mock 行为五态。
const (
	mockModeSuccess           = "success"
	mockModeFailure           = "failure"
	mockModeRateLimit         = "ratelimit"
	mockModeCredentialInvalid = "credential_invalid"
	mockModeUnsupported       = "unsupported"
)

// MockPublisher mock 平台适配器（详细设计 §2 接口的形状样板 + dev/test 的确定性替身）。
//
// 纪律：
//   - 错误消息永不回显凭据（PublishRequest.Credential 一个字节也不进 error/日志）。
//   - post_id 确定性（account+title 哈希）：同输入同输出，便于测试断言与回放比对。
//   - 五态经 MAP_MOCK_MODE 切换；测试用 NewMockPublisher(mode) 直接注入，不走环境变量。
type MockPublisher struct {
	mode string
	now  func() time.Time
}

// NewMockPublisher 构造（空 mode 回退 success）。
func NewMockPublisher(mode string) *MockPublisher {
	if mode == "" {
		mode = mockModeSuccess
	}
	return &MockPublisher{mode: mode, now: time.Now}
}

// init 自注册（键 mock；mode 来自环境变量）。
func init() {
	defaultRegistry.Register(NewMockPublisher(os.Getenv(mockModeEnv)))
}

// PlatformKey 平台键。
func (m *MockPublisher) PlatformKey() string { return MockPlatformKey }

// Capability mock 能力声明（全能力 + 常规上限，覆盖文案截断/规格校验的测试路径）。
func (m *MockPublisher) Capability() bizpub.Capability {
	return bizpub.Capability{
		Video:          true,
		ImageText:      true,
		NativeSchedule: true,
		DraftState:     true,
		Analytics:      true,
		MaxDurationSec: 7200,
		MaxSizeBytes:   4 << 30, // 4GiB
		AspectRatios:   []string{"16:9", "9:16", "1:1"},
		TitleMaxLen:    100,
		DescMaxLen:     2000,
	}
}

// Publish 模拟发布（🔴 永不引用 req.Credential 内容——真实适配器同样纪律）。
func (m *MockPublisher) Publish(_ context.Context, req bizpub.PublishRequest) (*bizpub.PublishResult, error) {
	switch m.mode {
	case mockModeFailure:
		return nil, errors.New("mock 平台内部错误（模拟上游 500）")
	case mockModeRateLimit:
		return nil, errors.New("mock 平台限流（模拟 429，请稍后重试）")
	case mockModeCredentialInvalid:
		return nil, fmt.Errorf("%w: mock 平台凭据校验失败（模拟 401）", bizpub.ErrCredentialInvalid)
	case mockModeUnsupported:
		return nil, fmt.Errorf("%w: mock 平台不支持该内容形态", bizpub.ErrCapabilityUnsupported)
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = "private" // 🔴 兜底私密（适配器层同样守默认）
	}
	sum := sha256.Sum256([]byte(req.Account.AccountID + "|" + req.Title + "|" + req.Desc))
	postID := "mock_" + hex.EncodeToString(sum[:])[:12]
	return &bizpub.PublishResult{
		PostID:      postID,
		PostURL:     "https://mock.local/p/" + postID,
		Visibility:  visibility,
		PublishedAt: m.now().UTC(),
		Raw: map[string]any{
			"platform": MockPlatformKey,
			"mode":     m.mode,
			"echo_visibility": visibility,
		},
	}, nil
}

// FetchAnalytics 模拟表现回流（unsupported → 明确报不支持；其余返回固定样例数）。
func (m *MockPublisher) FetchAnalytics(_ context.Context, acct *bizpub.AccountRow, postID string) (*bizpub.Performance, error) {
	switch m.mode {
	case mockModeCredentialInvalid:
		return nil, fmt.Errorf("%w: mock 平台凭据校验失败（模拟 401）", bizpub.ErrCredentialInvalid)
	case mockModeUnsupported:
		return nil, fmt.Errorf("%w: mock 平台不支持 analytics", bizpub.ErrCapabilityUnsupported)
	}
	return &bizpub.Performance{
		Views:          1024,
		Likes:          88,
		Comments:       12,
		Shares:         5,
		Favorites:      30,
		CompletionRate: 0.62,
		FetchedAt:      m.now().UTC(),
	}, nil
}
