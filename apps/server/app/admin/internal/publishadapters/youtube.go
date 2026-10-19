package publishadapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	bizpub "framework/app/admin/internal/biz/publish"
)

// YouTubePlatformKey 平台键（须与 platforms.yaml / VariantSet.copy 键一致）。
const YouTubePlatformKey = "youtube"

// YouTube Data API v3 端点。常量而非配置——官方端点是稳定契约；
// 测试用 NewYouTubePublisher(WithYouTubeBaseURL(srv.URL)) 注入 httptest。
const (
	youtubeUploadBase = "https://upload.youtube.com/upload/youtube/v3/videos"
	youtubeAPIBase    = "https://www.googleapis.com/youtube/v3/videos"
	youtubeTokenURL   = "https://oauth2.googleapis.com/token"
)

// youtubeUploadChunkSize resumable upload 单片大小（8MiB，API 要求 256KiB 整数倍）。
const youtubeUploadChunkSize = 8 << 20

// YouTubeCredential OAuth2 凭据形状（🔴 从 Credential.Secret JSON 解出后仅活
// 在函数作用域；绝不进日志/error 消息）。
//
// 凭据 vault 落库形态（enc:v1:... 解密后的明文 JSON）：
//
//	{"client_id":"...","client_secret":"...","refresh_token":"..."}
type YouTubeCredential struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

// YouTubeOption 构造选项（测试注入端点/HTTP client）。
type YouTubeOption func(*YouTubePublisher)

// WithYouTubeHTTPClient 注入自定义 HTTP client（超时/代理/测试）。
func WithYouTubeHTTPClient(hc *http.Client) YouTubeOption {
	return func(p *YouTubePublisher) { p.hc = hc }
}

// WithYouTubeBaseURL 注入测试端点（httptest；覆盖 upload 与 api 两个基址）。
func WithYouTubeBaseURL(uploadBase, apiBase string) YouTubeOption {
	return func(p *YouTubePublisher) {
		p.uploadBase = uploadBase
		p.apiBase = apiBase
	}
}

// WithYouTubeTokenURL 注入 token 交换端点（httptest；与基址同纪律）。
func WithYouTubeTokenURL(tokenURL string) YouTubeOption {
	return func(p *YouTubePublisher) { p.tokenURL = tokenURL }
}

// YouTubePublisher YouTube Data API v3 真实适配器（FR-PUB-01 首个真实平台；
// 平台接入顺序 §4：API 最成熟先做）。
//
// 纪律：
//   - 🔴 凭据纪律：PublishRequest.Credential 内容一个字节不进 error/日志；
//     access_token 同样只活在请求头。适配器内不触 vault——解密由调用方
//     （publish_usecase）完成并 defer Zero()。
//   - 🔴 可见性兜底：空/非法可见性恒落 private（NFR-MAP-01 适配器层防线）。
//   - 🔴 凭据缺失/损坏 → ErrCredentialInvalid 明确报错（ADR-MAP-05 不静默降级）。
//   - 401/403（invalid_grant / 配额 auth 类）→ ErrCredentialInvalid；
//     408/5xx/429 → 一般上游错误（熔断记账，下轮可重试）。
//   - 原生排期：ScheduledFor 未来时刻 → status.publishAt（平台原生排期，
//     概要设计 §4 模式①；此时可见性须 private 语义，发布时点由 publishAt 承载）。
type YouTubePublisher struct {
	hc         *http.Client
	uploadBase string
	apiBase    string
	tokenURL   string
	now        func() time.Time
}

// NewYouTubePublisher 构造（默认生产端点 + 60s client）。
func NewYouTubePublisher(opts ...YouTubeOption) *YouTubePublisher {
	p := &YouTubePublisher{
		hc:         &http.Client{Timeout: 60 * time.Second},
		uploadBase: youtubeUploadBase,
		apiBase:    youtubeAPIBase,
		tokenURL:   youtubeTokenURL,
		now:        time.Now,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// init 自注册（键 youtube；与 mock 同走 defaultRegistry）。
func init() {
	defaultRegistry.Register(NewYouTubePublisher())
}

// PlatformKey 平台键。
func (y *YouTubePublisher) PlatformKey() string { return YouTubePlatformKey }

// Capability 能力声明（与 platforms.yaml youtube 段一致；注册表优先于配置，
// 二者必须同源——改一边必须同步另一边）。
func (y *YouTubePublisher) Capability() bizpub.Capability {
	return bizpub.Capability{
		Video:          true,
		ImageText:      false, // 社区帖 Phase 3
		NativeSchedule: true,  // status.publishAt
		DraftState:     true,
		Analytics:      true,
		MaxDurationSec: 43200,        // 12h
		MaxSizeBytes:   274877906944, // 256GiB
		AspectRatios:   []string{"16:9", "9:16", "1:1"},
		TitleMaxLen:    100,
		DescMaxLen:     5000,
	}
}

// ============================================================================
// Publish：OAuth2 token 交换 → resumable upload 两段（initiate + 分片 PUT）。
// ============================================================================

// Publish 发布视频（Data API v3 videos.insert resumable upload）。
// 前置校验先于任何网络调用：凭据守卫 → 文件可读 → 然后才 token 交换
//（🔴 凭据守卫拒绝与本地文件缺失都不该消耗上游配额/触发网络）。
func (y *YouTubePublisher) Publish(ctx context.Context, req bizpub.PublishRequest) (*bizpub.PublishResult, error) {
	cred, err := y.requireCredential(req)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(req.VideoPath)
	if err != nil {
		return nil, fmt.Errorf("%w: 视频文件不可读", bizpub.ErrBadRequest)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%w: 视频文件状态读取失败", bizpub.ErrBadRequest)
	}

	token, err := y.exchangeToken(ctx, cred)
	if err != nil {
		return nil, err
	}
	visibility := normalizeYouTubeVisibility(req.Visibility)

	uploadURL, err := y.initiateUpload(ctx, token, req, visibility, fi.Size())
	if err != nil {
		return nil, err
	}
	postID, err := y.uploadChunks(ctx, token, uploadURL, f, fi.Size())
	if err != nil {
		return nil, err
	}
	return &bizpub.PublishResult{
		PostID:      postID,
		PostURL:     "https://www.youtube.com/watch?v=" + postID,
		Visibility:  visibility,
		PublishedAt: y.now().UTC(),
		Raw: map[string]any{
			"platform":   YouTubePlatformKey,
			"visibility": visibility,
		},
	}, nil
}

// requireCredential 凭据守卫（🔴 缺失/非 oauth/解 JSON 失败 → ErrCredentialInvalid，
// 明确报错不静默降级；错误消息不含任何凭据字节）。
func (y *YouTubePublisher) requireCredential(req bizpub.PublishRequest) (*YouTubeCredential, error) {
	if req.Credential == nil || len(req.Credential.Secret) == 0 {
		return nil, fmt.Errorf("%w: youtube 发布需 OAuth2 凭据（账号未配置 credential_ref）", bizpub.ErrCredentialInvalid)
	}
	if req.Credential.Type != "" && req.Credential.Type != "oauth" {
		return nil, fmt.Errorf("%w: youtube 凭据类型须为 oauth（当前 %s）", bizpub.ErrCredentialInvalid, req.Credential.Type)
	}
	var cred YouTubeCredential
	if err := json.Unmarshal(req.Credential.Secret, &cred); err != nil {
		return nil, fmt.Errorf("%w: youtube 凭据 JSON 损坏，需重新授权", bizpub.ErrCredentialInvalid)
	}
	if cred.ClientID == "" || cred.ClientSecret == "" || cred.RefreshToken == "" {
		return nil, fmt.Errorf("%w: youtube 凭据缺 client_id/client_secret/refresh_token", bizpub.ErrCredentialInvalid)
	}
	return &cred, nil
}

// exchangeToken refresh_token → access_token（OAuth2 token endpoint）。
// 401/400 invalid_grant → ErrCredentialInvalid（账号停用引导重新授权）。
func (y *YouTubePublisher) exchangeToken(ctx context.Context, cred *YouTubeCredential) (string, error) {
	form := "grant_type=refresh_token&client_id=" + cred.ClientID +
		"&client_secret=" + cred.ClientSecret + "&refresh_token=" + cred.RefreshToken
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		y.tokenURL, strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := y.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube token 交换网络失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		return "", fmt.Errorf("%w: youtube token 交换被拒（refresh_token 失效或应用被撤销）", bizpub.ErrCredentialInvalid)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("youtube token 交换上游错误: HTTP %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", fmt.Errorf("youtube token 交换响应解析失败")
	}
	return body.AccessToken, nil
}

// youtubeVideoResource videos.insert 元数据体。
type youtubeVideoResource struct {
	Snippet struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags,omitempty"`
	} `json:"snippet"`
	Status struct {
		PrivacyStatus string `json:"privacyStatus"`
		PublishAt     string `json:"publishAt,omitempty"`
	} `json:"status"`
}

// initiateUpload resumable 第一段：POST 元数据 → 取 Location 会话 URL。
func (y *YouTubePublisher) initiateUpload(ctx context.Context, token string, req bizpub.PublishRequest, visibility string, size int64) (string, error) {
	var meta youtubeVideoResource
	meta.Snippet.Title = req.Title
	meta.Snippet.Description = req.Desc
	meta.Snippet.Tags = req.Tags
	meta.Status.PrivacyStatus = visibility
	// 原生排期：publishAt 未来时刻（平台承载定时；此时必须 private 语义）。
	if req.ScheduledFor != nil && req.ScheduledFor.After(y.now()) {
		meta.Status.PublishAt = req.ScheduledFor.UTC().Format(time.RFC3339)
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("%w: 元数据序列化失败", bizpub.ErrBadRequest)
	}
	url := y.uploadBase + "?uploadType=resumable&part=snippet,status"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hreq.Header.Set("Authorization", "Bearer "+token)
	hreq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	hreq.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	resp, err := y.hc.Do(hreq)
	if err != nil {
		return "", fmt.Errorf("youtube 上传会话初始化网络失败: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("%w: youtube 上传初始化被拒（scope 不足或凭据失效）", bizpub.ErrCredentialInvalid)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", fmt.Errorf("youtube 上传初始化上游错误: HTTP %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("youtube 上传初始化未返回会话地址")
	}
	// 🔴 Location 含 upload_id 会话令牌——仅内存使用，不进日志/error。
	return loc, nil
}

// uploadChunks resumable 第二段：分片 PUT；末片 201 响应体含 video id。
func (y *YouTubePublisher) uploadChunks(ctx context.Context, token, uploadURL string, r io.Reader, size int64) (string, error) {
	buf := make([]byte, youtubeUploadChunkSize)
	var sent int64
	for {
		n, rerr := io.ReadFull(r, buf)
		if rerr != nil && rerr != io.EOF && rerr != io.ErrUnexpectedEOF {
			return "", fmt.Errorf("youtube 视频读取失败: %w", rerr)
		}
		if n == 0 {
			break
		}
		last := sent+int64(n) >= size
		hreq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(buf[:n]))
		if err != nil {
			return "", err
		}
		hreq.Header.Set("Authorization", "Bearer "+token)
		hreq.Header.Set("Content-Length", strconv.Itoa(n))
		hreq.Header.Set("Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", sent, sent+int64(n)-1, size))
		resp, err := y.hc.Do(hreq)
		if err != nil {
			return "", fmt.Errorf("youtube 分片上传网络失败: %w", err)
		}
		respBody, derr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if derr != nil {
			return "", fmt.Errorf("youtube 分片响应读取失败: %w", derr)
		}
		sent += int64(n)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("%w: youtube 分片上传被拒", bizpub.ErrCredentialInvalid)
		}
		if resp.StatusCode == http.StatusPermanentRedirect || resp.StatusCode == 308 {
			continue // 已收片确认，续下一片
		}
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
			if !last {
				continue // 平台提前完成（单片上情景）
			}
			var out struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(respBody, &out); err != nil || out.ID == "" {
				return "", fmt.Errorf("youtube 完成响应解析失败")
			}
			return out.ID, nil
		}
		return "", fmt.Errorf("youtube 分片上传上游错误: HTTP %d", resp.StatusCode)
	}
	return "", errors.New("youtube 上传流异常终止（无完成响应）")
}

// ============================================================================
// FetchAnalytics（诚实空态）
//
// 详细设计 §6 注 4：「FetchAnalytics(acct, postID) 签名不含凭据；真实适配器
// 接入时由适配器内部经 vault 解密即清」。V1 适配器由 init() 自注册进
// defaultRegistry（database/sql driver 模式），无 DI 入口拿 vault——而签名
// 又不带凭据，因此 analytics 拉取在 V1 **无合法凭据路径**。
//
// 纪律选择（ADR-MAP-05）：明确报错而非静默返回空数据——worker 周期拉取
// 将以 credential_invalid 标账号停用、下轮自然过滤（同 401 路径语义），
// 「发了却拉不回表现」比「诚实报凭据路径未接线」更糟。
//
// 接线方案（留待 MAP 线收口评审）：① Registry 改造为 DI 构造（注入
// CredentialVault），FetchAnalytics 内部解密即清；或 ② M6 worker 侧解密
// 后经接口传参（需动 FetchAnalytics 签名，影响全部适配器）。
// ============================================================================

// FetchAnalytics 表现回流（V1 诚实空态：凭据接线未落位，明确报错）。
func (y *YouTubePublisher) FetchAnalytics(_ context.Context, acct *bizpub.AccountRow, _ string) (*bizpub.Performance, error) {
	if acct == nil || !acct.HasCredential || acct.CredentialRef == nil {
		return nil, fmt.Errorf("%w: youtube analytics 需账号凭据（未配置 credential_ref）", bizpub.ErrCredentialInvalid)
	}
	return nil, fmt.Errorf("%w: youtube analytics 凭据解密接线未落位（init 自注册无 DI；V1 明确报错，不静默降级）", bizpub.ErrCredentialInvalid)
}

// normalizeYouTubeVisibility 可见性归一（🔴 空/非法恒落 private，NFR-MAP-01）。
func normalizeYouTubeVisibility(v string) string {
	switch v {
	case "private", "unlisted", "public":
		return v
	default:
		return "private"
	}
}
