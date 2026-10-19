package publishadapters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	bizpub "framework/app/admin/internal/biz/publish"
)

// 🔴 测试凭据字节——贯穿全部用例的泄漏探针（错误消息/日志断言不得出现）。
const (
	ytTestClientSecret   = "cs-secret-7"
	ytTestRefreshToken   = "rt-secret-9"
	ytTestAccessToken    = "at-123"
	ytTestVideoID        = "vid-abc"
	ytTestUploadLocation = "/session/s1"
)

func ytTestCredential(t *testing.T) *bizpub.Credential {
	t.Helper()
	raw, err := json.Marshal(map[string]string{
		"client_id":     "ci-1",
		"client_secret": ytTestClientSecret,
		"refresh_token": ytTestRefreshToken,
	})
	if err != nil {
		t.Fatalf("marshal credential: %v", err)
	}
	return &bizpub.Credential{Type: "oauth", Secret: raw}
}

// ytServer httptest 三端点替身：/token、/upload（initiate）、/session/*（分片 PUT）。
type ytServer struct {
	*httptest.Server
	mu           sync.Mutex
	tokenStatus  int
	initStatus   int
	chunkStatus  int
	initiateBody []byte // 捕获的 videos.insert 元数据
	authHeaders  []string
	uploadLen    string
}

func newYTServer(t *testing.T) *ytServer {
	t.Helper()
	s := &ytServer{tokenStatus: 200, initStatus: 200, chunkStatus: 201}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		code := s.tokenStatus
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code == 200 {
			_, _ = w.Write([]byte(`{"access_token":"` + ytTestAccessToken + `","expires_in":3600}`))
		} else {
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		}
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		s.mu.Lock()
		s.initiateBody, _ = readAll(r)
		s.uploadLen = r.Header.Get("X-Upload-Content-Length")
		s.authHeaders = append(s.authHeaders, r.Header.Get("Authorization"))
		code := s.initStatus
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if code == 200 {
			w.Header().Set("Location", s.Server.URL+ytTestUploadLocation)
		}
		w.WriteHeader(code)
	})
	mux.HandleFunc(ytTestUploadLocation, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(405)
			return
		}
		s.mu.Lock()
		s.authHeaders = append(s.authHeaders, r.Header.Get("Authorization"))
		code := s.chunkStatus
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if code == 201 {
			_, _ = w.Write([]byte(`{"id":"` + ytTestVideoID + `"}`))
		}
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Server.Close)
	return s
}

func readAll(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

func ytNewPublisher(s *ytServer) *YouTubePublisher {
	return NewYouTubePublisher(
		WithYouTubeBaseURL(s.Server.URL+"/upload", s.Server.URL+"/api"),
		WithYouTubeTokenURL(s.Server.URL+"/token"),
	)
}

func ytWriteVideo(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "v.mp4")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write video: %v", err)
	}
	return p
}

// TestYouTubeRegistrySelfRegister：init() 自注册落 defaultRegistry（同 mock 模式）。
func TestYouTubeRegistrySelfRegister(t *testing.T) {
	p, ok := defaultRegistry.Get(YouTubePlatformKey)
	if !ok {
		t.Fatalf("youtube 适配器未自注册进 defaultRegistry")
	}
	if p.PlatformKey() != "youtube" {
		t.Fatalf("PlatformKey = %s", p.PlatformKey())
	}
}

// TestYouTubeCapabilityMatchesPlatformsYAML：适配器声明与 platforms.yaml youtube 段同源。
func TestYouTubeCapabilityMatchesPlatformsYAML(t *testing.T) {
	got := NewYouTubePublisher().Capability()
	if !got.Video || got.ImageText || !got.NativeSchedule || !got.DraftState || !got.Analytics {
		t.Fatalf("capability 布尔面与 platforms.yaml 不一致: %+v", got)
	}
	if got.MaxDurationSec != 43200 || got.MaxSizeBytes != 274877906944 {
		t.Fatalf("上限与 platforms.yaml 不一致: %+v", got)
	}
	if got.TitleMaxLen != 100 || got.DescMaxLen != 5000 {
		t.Fatalf("文案上限与 platforms.yaml 不一致: %+v", got)
	}
}

// TestYouTubePublishSuccess：token 交换 → initiate（捕获元数据/Auth）→ 单分片 201 → 回执。
func TestYouTubePublishSuccess(t *testing.T) {
	s := newYTServer(t)
	p := ytNewPublisher(s)
	content := "fake-video-bytes-0123456789"
	video := ytWriteVideo(t, content)

	res, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Account:    &bizpub.AccountRow{AccountID: "a1"},
		Credential: ytTestCredential(t),
		Title:      "Hello 标题",
		Desc:       "desc 中文",
		Tags:       []string{"a", "b"},
		Visibility: "unlisted",
		VideoPath:  video,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.PostID != ytTestVideoID {
		t.Fatalf("PostID = %s", res.PostID)
	}
	if res.PostURL != "https://www.youtube.com/watch?v="+ytTestVideoID {
		t.Fatalf("PostURL = %s", res.PostURL)
	}
	if res.Visibility != "unlisted" {
		t.Fatalf("Visibility = %s", res.Visibility)
	}
	if res.PublishedAt.IsZero() {
		t.Fatalf("PublishedAt 未填")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// 两处请求（initiate + chunk）都必须带同一 Bearer access token。
	if len(s.authHeaders) != 2 || s.authHeaders[0] != "Bearer "+ytTestAccessToken || s.authHeaders[1] != "Bearer "+ytTestAccessToken {
		t.Fatalf("Authorization 头不对: %v", s.authHeaders)
	}
	if s.uploadLen != strconv.Itoa(len(content)) {
		t.Fatalf("X-Upload-Content-Length = %q, want %d", s.uploadLen, len(content))
	}
	var meta map[string]any
	if err := json.Unmarshal(s.initiateBody, &meta); err != nil {
		t.Fatalf("initiate 元数据不是 JSON: %v", err)
	}
	sn := meta["snippet"].(map[string]any)
	if sn["title"] != "Hello 标题" || sn["description"] != "desc 中文" {
		t.Fatalf("snippet 不对: %v", sn)
	}
	st := meta["status"].(map[string]any)
	if st["privacyStatus"] != "unlisted" {
		t.Fatalf("privacyStatus = %v", st["privacyStatus"])
	}
	if _, hasPublishAt := st["publishAt"]; hasPublishAt {
		t.Fatalf("无排期不应出现 publishAt: %v", st)
	}
}

// TestYouTubePublishDefaultPrivate：空可见性与垃圾可见性都落 private（NFR-MAP-01）。
func TestYouTubePublishDefaultPrivate(t *testing.T) {
	for _, vis := range []string{"", "garbage", "PUBLIC"} {
		s := newYTServer(t)
		p := ytNewPublisher(s)
		video := ytWriteVideo(t, "x")
		res, err := p.Publish(context.Background(), bizpub.PublishRequest{
			Credential: ytTestCredential(t),
			Title:      "t",
			Visibility: vis,
			VideoPath:  video,
		})
		if err != nil {
			t.Fatalf("vis=%q Publish: %v", vis, err)
		}
		if res.Visibility != "private" {
			t.Fatalf("vis=%q → %s, want private", vis, res.Visibility)
		}
		s.mu.Lock()
		var meta map[string]any
		_ = json.Unmarshal(s.initiateBody, &meta)
		st := meta["status"].(map[string]any)
		s.mu.Unlock()
		if st["privacyStatus"] != "private" {
			t.Fatalf("vis=%q 平台侧 privacyStatus = %v", vis, st["privacyStatus"])
		}
	}
}

// TestYouTubePublishNativeSchedule：ScheduledFor 未来 → status.publishAt（平台原生排期）。
func TestYouTubePublishNativeSchedule(t *testing.T) {
	s := newYTServer(t)
	p := ytNewPublisher(s)
	video := ytWriteVideo(t, "x")
	when := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	_, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Credential:   ytTestCredential(t),
		Title:        "t",
		Visibility:   "private",
		ScheduledFor: &when,
		VideoPath:    video,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var meta map[string]any
	if err := json.Unmarshal(s.initiateBody, &meta); err != nil {
		t.Fatalf("initiate 元数据解析: %v", err)
	}
	st := meta["status"].(map[string]any)
	if got := st["publishAt"]; got != when.Format(time.RFC3339) {
		t.Fatalf("publishAt = %v, want %v", got, when.Format(time.RFC3339))
	}
}

// TestYouTubePublishCredentialErrors：凭据缺失/类型错/JSON 坏/字段缺 → ErrCredentialInvalid。
func TestYouTubePublishCredentialErrors(t *testing.T) {
	video := ytWriteVideo(t, "x")
	cases := []struct {
		name string
		cred *bizpub.Credential
	}{
		{"nil", nil},
		{"empty-secret", &bizpub.Credential{Type: "oauth"}},
		{"wrong-type", &bizpub.Credential{Type: "cookie", Secret: []byte(`{"client_id":"a","client_secret":"b","refresh_token":"c"}`)}},
		{"bad-json", &bizpub.Credential{Type: "oauth", Secret: []byte("not-json")}},
		{"missing-refresh", &bizpub.Credential{Type: "oauth", Secret: []byte(`{"client_id":"a","client_secret":"b"}`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewYouTubePublisher() // 无服务器也会被凭据守卫拦截
			_, err := p.Publish(context.Background(), bizpub.PublishRequest{
				Credential: tc.cred,
				Title:      "t",
				VideoPath:  video,
			})
			if !errors.Is(err, bizpub.ErrCredentialInvalid) {
				t.Fatalf("want ErrCredentialInvalid, got %v", err)
			}
		})
	}
}

// TestYouTubePublishTokenInvalidGrant：token 交换 400 → ErrCredentialInvalid。
func TestYouTubePublishTokenInvalidGrant(t *testing.T) {
	s := newYTServer(t)
	s.mu.Lock()
	s.tokenStatus = 400
	s.mu.Unlock()
	p := ytNewPublisher(s)
	_, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t),
		Title:      "t",
		VideoPath:  ytWriteVideo(t, "x"),
	})
	if !errors.Is(err, bizpub.ErrCredentialInvalid) {
		t.Fatalf("want ErrCredentialInvalid, got %v", err)
	}
}

// TestYouTubePublishInitiate403：上传初始化 403 → ErrCredentialInvalid。
func TestYouTubePublishInitiate403(t *testing.T) {
	s := newYTServer(t)
	s.mu.Lock()
	s.initStatus = 403
	s.mu.Unlock()
	p := ytNewPublisher(s)
	_, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t),
		Title:      "t",
		VideoPath:  ytWriteVideo(t, "x"),
	})
	if !errors.Is(err, bizpub.ErrCredentialInvalid) {
		t.Fatalf("want ErrCredentialInvalid, got %v", err)
	}
}

// TestYouTubePublishChunk500：分片 5xx → 一般上游错误（熔断记账重试路径，非凭据）。
func TestYouTubePublishChunk500(t *testing.T) {
	s := newYTServer(t)
	s.mu.Lock()
	s.chunkStatus = 500
	s.mu.Unlock()
	p := ytNewPublisher(s)
	_, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t),
		Title:      "t",
		VideoPath:  ytWriteVideo(t, "x"),
	})
	if err == nil {
		t.Fatalf("want error, got nil")
	}
	if errors.Is(err, bizpub.ErrCredentialInvalid) {
		t.Fatalf("5xx 不得映射为凭据失效: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("错误应含状态码: %v", err)
	}
}

// TestYouTubePublishMissingVideoFile：视频文件不存在 → ErrBadRequest（明确报错）。
func TestYouTubePublishMissingVideoFile(t *testing.T) {
	p := NewYouTubePublisher()
	_, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t),
		Title:      "t",
		VideoPath:  filepath.Join(t.TempDir(), "不存在.mp4"),
	})
	if !errors.Is(err, bizpub.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
}

// TestYouTubeErrorMessagesNeverLeakCredential：所有失败路径错误消息不得含凭据字节
// （refresh_token/client_secret/access_token）——对齐 mock 适配器同纪律。
func TestYouTubeErrorMessagesNeverLeakCredential(t *testing.T) {
	video := ytWriteVideo(t, "x")
	s := newYTServer(t)
	p := ytNewPublisher(s)

	errs := []error{}
	// 1. token 400
	s.mu.Lock()
	s.tokenStatus = 400
	s.mu.Unlock()
	_, err := p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t), Title: "t", VideoPath: video,
	})
	errs = append(errs, err)
	// 2. initiate 403
	s.mu.Lock()
	s.tokenStatus = 200
	s.initStatus = 403
	s.mu.Unlock()
	_, err = p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t), Title: "t", VideoPath: video,
	})
	errs = append(errs, err)
	// 3. chunk 500
	s.mu.Lock()
	s.initStatus = 200
	s.chunkStatus = 500
	s.mu.Unlock()
	_, err = p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: ytTestCredential(t), Title: "t", VideoPath: video,
	})
	errs = append(errs, err)
	// 4. 凭据守卫
	_, err = p.Publish(context.Background(), bizpub.PublishRequest{
		Credential: &bizpub.Credential{Type: "oauth", Secret: []byte("broken")},
		Title:      "t", VideoPath: video,
	})
	errs = append(errs, err)
	// 5. analytics 两路径
	_, err = p.FetchAnalytics(context.Background(), &bizpub.AccountRow{AccountID: "a1"}, "x")
	errs = append(errs, err)
	_, err = p.FetchAnalytics(context.Background(), &bizpub.AccountRow{
		AccountID: "a2", HasCredential: true, CredentialRef: strPtr("enc:v1:zz"),
	}, "x")
	errs = append(errs, err)

	for _, e := range errs {
		if e == nil {
			t.Fatalf("失败路径必须返回 error")
		}
		for _, leak := range []string{ytTestClientSecret, ytTestRefreshToken, ytTestAccessToken, "enc:v1:zz"} {
			if strings.Contains(e.Error(), leak) {
				t.Fatalf("错误消息泄漏凭据字节 %q: %s", leak, e.Error())
			}
		}
	}
}

// TestYouTubeFetchAnalyticsHonestEmptyState：V1 凭据接线未落位——两路径都明确
// 报 ErrCredentialInvalid，绝不静默返回空 Performance（ADR-MAP-05）。
func TestYouTubeFetchAnalyticsHonestEmptyState(t *testing.T) {
	p := NewYouTubePublisher()
	ref := "enc:v1:zz"
	cases := []struct {
		name string
		acct *bizpub.AccountRow
	}{
		{"no-credential", &bizpub.AccountRow{AccountID: "a1"}},
		{"with-credential", &bizpub.AccountRow{AccountID: "a2", HasCredential: true, CredentialRef: &ref}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := p.FetchAnalytics(context.Background(), tc.acct, "vid-x")
			if err == nil {
				t.Fatalf("必须明确报错，不得静默返回")
			}
			if !errors.Is(err, bizpub.ErrCredentialInvalid) {
				t.Fatalf("want ErrCredentialInvalid, got %v", err)
			}
			if res != nil {
				t.Fatalf("失败不得带结果: %+v", res)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
