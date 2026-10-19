package biz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────
// GitHub REST 客户端（radar v2）
// 双引擎榜单：trending HTML 优先（唯一可拿官方口径窗口增量的途径），
// Search API 兜底；全部产物走 TTL 缓存；失败回退上一个好快照并标 stale。
// ─────────────────────────────────────────────────────────────

const (
	githubAPIBase   = "https://api.github.com"
	githubHTMLBase  = "https://github.com"
	githubUserAgent = "FrameWork-Radar/1.0 (+https://github.com)"

	trendingCacheTTL = 10 * time.Minute
	repoCacheTTL     = 30 * time.Minute
	readmeCacheTTL   = 60 * time.Minute
	treeCacheTTL     = 10 * time.Minute
	contentCacheTTL  = 10 * time.Minute

	trendingEnrichConcurrency = 4
	trendingEnrichLimit       = 25 // 单次榜单最多并发补全的仓库数
	treeTruncateLimit         = 1000
	contentTruncateLimit      = 512 * 1024
	readmeTruncateLimit       = 200 * 1024
	cloneTimeout              = 60 * time.Second
)

// RadarTokenProvider 提供 GitHub Token（由 RadarConfigService 实现，避免循环依赖）。
type RadarTokenProvider interface {
	GitHubToken() string
}

// GitHubClient 聚合 trending / repo / readme / tree / content / clone。
type GitHubClient struct {
	http    *http.Client
	tokenFn func() string

	mu        sync.RWMutex
	cache     map[string]cacheEntry
	snapshots map[string]*snapshotEntry
	genCount  map[string]int // AI 生成频控 key: date|full|level
	genDay    string
}

type cacheEntry struct {
	v   any
	exp time.Time
}

type snapshotEntry struct {
	items []RadarRepoView
	at    time.Time
}

// NewGitHubClient 构造客户端；tokenFn 可为 nil（匿名 60 次/小时限额）。
func NewGitHubClient(tokenFn func() string) *GitHubClient {
	return &GitHubClient{
		http:      &http.Client{Timeout: 15 * time.Second},
		tokenFn:   tokenFn,
		cache:     map[string]cacheEntry{},
		snapshots: map[string]*snapshotEntry{},
		genCount:  map[string]int{},
	}
}

func (c *GitHubClient) cacheGet(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		return nil, false
	}
	return e.v, true
}

func (c *GitHubClient) cacheSet(key string, v any, ttl time.Duration) {
	c.mu.Lock()
	c.cache[key] = cacheEntry{v: v, exp: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// ── 底层请求 ──

func (c *GitHubClient) token() string {
	if c.tokenFn == nil {
		return ""
	}
	return c.tokenFn()
}

func (c *GitHubClient) newRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", githubUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := c.token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	return req, nil
}

// doJSON 带一次限流退避的 GET；返回 body 与限流信息。
func (c *GitHubClient) doJSON(ctx context.Context, url string, out any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			}
		}
		req, err := c.newRequest(ctx, url)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("github rate limited (status %d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("github api %s -> %d: %s", url, resp.StatusCode, truncateStr(string(body), 200))
		}
		if out != nil {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("github api %s decode: %w", url, err)
			}
		}
		return nil
	}
	return lastErr
}

// ── trending 双引擎 ──

type ghTrendingEntry struct {
	FullName    string
	Description string
	Language    string
	Stars       int64
	Forks       int64
	StarsDelta  int64
	BuiltBy     []string
}

// windowStart 返回时间窗口起点（UTC）。
func windowStart(since string) time.Time {
	now := time.Now().UTC()
	switch since {
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, 0, -30)
	default:
		return now.AddDate(0, 0, -1)
	}
}

func windowSince(since string) string {
	switch since {
	case "week":
		return "weekly"
	case "month":
		return "monthly"
	default:
		return "daily"
	}
}

// FetchTrending 双引擎抓取 trending；返回基础条目（未补全详情字段）。
func (c *GitHubClient) FetchTrending(ctx context.Context, since, language string) ([]ghTrendingEntry, error) {
	entries, err := c.fetchTrendingHTML(ctx, since, language)
	if err == nil && len(entries) > 0 {
		return entries, nil
	}
	apiEntries, apiErr := c.fetchTrendingSearch(ctx, since, language)
	if apiErr != nil {
		if err != nil {
			return nil, fmt.Errorf("html engine: %v; api engine: %v", err, apiErr)
		}
		return nil, apiErr
	}
	return apiEntries, nil
}

var (
	reArticle     = regexp.MustCompile(`(?s)<article[^>]*>(.*?)</article>`)
	reHref        = regexp.MustCompile(`href="/([^"?#]+)"`)
	reDesc        = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	reProgLang    = regexp.MustCompile(`(?s)itemprop="programmingLanguage"[^>]*>([^<]+)<`)
	reStargazers  = regexp.MustCompile(`(?s)href="/[^"]+/stargazers"[^>]*>(.*?)</a>`)
	reForks       = regexp.MustCompile(`(?s)href="/[^"]+/forks"[^>]*>(.*?)</a>`)
	reStarsWindow = regexp.MustCompile(`(?s)([\d,]+)\s+stars?\s+(today|this week|this month)`)
	reAvatarAlt   = regexp.MustCompile(`alt="@([^"]+)"`)
	reLicense     = regexp.MustCompile(`(?s)octicon-law.*?</svg>\s*([^<]+)`)
	reHTMLTag     = regexp.MustCompile(`<[^>]*>`)
)

func (c *GitHubClient) fetchTrendingHTML(ctx context.Context, since, language string) ([]ghTrendingEntry, error) {
	url := fmt.Sprintf("%s/trending?since=%s", githubHTMLBase, windowSince(since))
	if language != "" {
		url += "&language=" + strings.ToLower(strings.ReplaceAll(language, " ", "-"))
	}
	req, err := c.newRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("trending html status %d", resp.StatusCode)
	}
	html := string(body)
	articles := reArticle.FindAllStringSubmatch(html, -1)
	if len(articles) == 0 {
		return nil, errors.New("trending html: no <article> parsed (structure changed?)")
	}
	out := make([]ghTrendingEntry, 0, len(articles))
	for _, a := range articles {
		block := a[1]
		name := ""
		if m := reHref.FindStringSubmatch(block); m != nil {
			name = m[1]
		}
		if name == "" || !strings.Contains(name, "/") || strings.HasPrefix(name, "login") || strings.HasPrefix(name, "sponsors/") {
			continue
		}
		e := ghTrendingEntry{FullName: name}
		if m := reDesc.FindStringSubmatch(block); m != nil {
			e.Description = strings.Join(strings.Fields(reHTMLTag.ReplaceAllString(m[1], "")), " ")
		}
		if m := reProgLang.FindStringSubmatch(block); m != nil {
			e.Language = strings.TrimSpace(m[1])
		}
		if m := reStargazers.FindStringSubmatch(block); m != nil {
			e.Stars = parseGhCount(reHTMLTag.ReplaceAllString(m[1], ""))
		}
		if m := reForks.FindStringSubmatch(block); m != nil {
			e.Forks = parseGhCount(reHTMLTag.ReplaceAllString(m[1], ""))
		}
		if m := reStarsWindow.FindStringSubmatch(block); m != nil {
			e.StarsDelta = parseGhCount(m[1])
		}
		for _, av := range reAvatarAlt.FindAllStringSubmatch(block, -1) {
			if len(e.BuiltBy) < 8 {
				e.BuiltBy = append(e.BuiltBy, av[1])
			}
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, errors.New("trending html: no entries parsed")
	}
	return out, nil
}

func (c *GitHubClient) fetchTrendingSearch(ctx context.Context, since, language string) ([]ghTrendingEntry, error) {
	start := windowStart(since).Format("2006-01-02")
	q := fmt.Sprintf("created:>%s", start)
	if language != "" {
		q += " language:" + strings.ReplaceAll(language, " ", "-")
	}
	url := fmt.Sprintf("%s/search/repositories?q=%s&sort=stars&order=desc&per_page=50", githubAPIBase, urlQueryEscape(q))
	var payload struct {
		Items []struct {
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			Language    string `json:"language"`
			Stars       int64  `json:"stargazers_count"`
			Forks       int64  `json:"forks_count"`
		} `json:"items"`
	}
	if err := c.doJSON(ctx, url, &payload); err != nil {
		return nil, err
	}
	out := make([]ghTrendingEntry, 0, len(payload.Items))
	for _, it := range payload.Items {
		out = append(out, ghTrendingEntry{
			FullName: it.FullName, Description: it.Description,
			Language: it.Language, Stars: it.Stars, Forks: it.Forks,
		})
	}
	return out, nil
}

// parseGhCount 解析 "132k" / "1,234" / "12.5k"。
func parseGhCount(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	mult := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'k', 'K':
		mult, s = 1000, s[:len(s)-1]
	case 'm', 'M':
		mult, s = 1000000, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int64(f * float64(mult))
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), ":", "%3A")
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── 仓库详情 / README / Tree / Content ──

type ghRepoResponse struct {
	ID          int64  `json:"id"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Language    string `json:"language"`
	License     *struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		SPDX string `json:"spdx_id"`
	} `json:"license"`
	Topics            []string `json:"topics"`
	StargazersCount   int64    `json:"stargazers_count"`
	ForksCount        int64    `json:"forks_count"`
	OpenIssuesCount   int64    `json:"open_issues_count"`
	WatchersCount     int64    `json:"subscribers_count"`
	DefaultBranch     string   `json:"default_branch"`
	Size              int64    `json:"size"`
	CloneURL          string   `json:"clone_url"`
	SSHURL            string   `json:"ssh_url"`
	ZipballURL        string   `json:"zipball_url"`
	HTMLURL           string   `json:"html_url"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
	PushedAt          string   `json:"pushed_at"`
	ContributorsURL   string   `json:"contributors_url"`
	StargazersRESTURL string   `json:"stargazers_url"`
}

type ghContributor struct {
	Login string `json:"login"`
}

// GetRepoDetail 拉取仓库完整元数据（含语言构成，并发两次请求），带 30min 缓存。
func (c *GitHubClient) GetRepoDetail(ctx context.Context, fullName string) (*RadarRepoView, error) {
	key := "repo:" + fullName
	if v, ok := c.cacheGet(key); ok {
		return v.(*RadarRepoView), nil
	}
	var repo ghRepoResponse
	if err := c.doJSON(ctx, fmt.Sprintf("%s/repos/%s", githubAPIBase, fullName), &repo); err != nil {
		return nil, err
	}
	view := ghRepoToView(&repo)
	// 语言构成（失败不阻塞）
	var langs map[string]int64
	if err := c.doJSON(ctx, fmt.Sprintf("%s/repos/%s/languages", githubAPIBase, fullName), &langs); err == nil && len(langs) > 0 {
		view.Languages = langStats(langs)
	}
	// Top 贡献者（失败不阻塞）
	var contribs []ghContributor
	if err := c.doJSON(ctx, fmt.Sprintf("%s/repos/%s/contributors?per_page=8", githubAPIBase, fullName), &contribs); err == nil {
		for _, ct := range contribs {
			if len(view.Contributors) < 8 {
				view.Contributors = append(view.Contributors, ct.Login)
			}
		}
	}
	c.cacheSet(key, view, repoCacheTTL)
	return view, nil
}

func ghRepoToView(r *ghRepoResponse) *RadarRepoView {
	v := &RadarRepoView{
		ID:            strconv.FormatInt(r.ID, 10),
		FullName:      r.FullName,
		Description:   r.Description,
		Language:      r.Language,
		Topics:        r.Topics,
		Stars:         r.StargazersCount,
		Forks:         r.ForksCount,
		OpenIssues:    r.OpenIssuesCount,
		Watchers:      r.WatchersCount,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
		PushedAt:      r.PushedAt,
		CloneURL:      r.CloneURL,
		SSHURL:        r.SSHURL,
		ZipURL:        r.ZipballURL,
		HTMLURL:       r.HTMLURL,
		DefaultBranch: r.DefaultBranch,
	}
	if r.License != nil {
		v.License = r.License.Name
		v.LicenseSPDX = r.License.SPDX
	}
	return v
}

func langStats(m map[string]int64) []RadarLanguageStat {
	var total int64
	for _, b := range m {
		total += b
	}
	out := make([]RadarLanguageStat, 0, len(m))
	for name, b := range m {
		pct := 0.0
		if total > 0 {
			pct = float64(b) / float64(total) * 100
		}
		out = append(out, RadarLanguageStat{Name: name, Bytes: b, Percent: pct})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// GetReadme 拉取 README 原文（raw），截断到 200KB。
func (c *GitHubClient) GetReadme(ctx context.Context, fullName string) (string, error) {
	key := "readme:" + fullName
	if v, ok := c.cacheGet(key); ok {
		return v.(string), nil
	}
	url := fmt.Sprintf("%s/repos/%s/readme", githubAPIBase, fullName)
	req, err := c.newRequest(ctx, url)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, readmeTruncateLimit+1))
	resp.Body.Close()
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("readme %s -> %d", fullName, resp.StatusCode)
	}
	text := string(body)
	if len(text) > readmeTruncateLimit {
		text = text[:readmeTruncateLimit]
	}
	c.cacheSet(key, text, readmeCacheTTL)
	return text, nil
}

type ghTreeResponse struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"tree"`
	Truncated bool `json:"truncated"`
}

// GetRepoTree 递归文件树；>1000 条截断。
func (c *GitHubClient) GetRepoTree(ctx context.Context, fullName, branch string) (nodes []RadarTreeNode, defaultBranch string, truncated bool, err error) {
	if branch == "" {
		var repo ghRepoResponse
		if err := c.doJSON(ctx, fmt.Sprintf("%s/repos/%s", githubAPIBase, fullName), &repo); err != nil {
			return nil, "", false, err
		}
		branch, defaultBranch = repo.DefaultBranch, repo.DefaultBranch
	} else {
		defaultBranch = branch
	}
	key := fmt.Sprintf("tree:%s:%s", fullName, branch)
	if v, ok := c.cacheGet(key); ok {
		t := v.(*ghTreeResponse)
		return ghTreeToNodes(t), defaultBranch, t.Truncated, nil
	}
	var tree ghTreeResponse
	if err := c.doJSON(ctx, fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", githubAPIBase, fullName, branch), &tree); err != nil {
		return nil, defaultBranch, false, err
	}
	cp := tree
	c.cacheSet(key, &cp, treeCacheTTL)
	nodes = ghTreeToNodes(&tree)
	if len(nodes) > treeTruncateLimit {
		nodes, truncated = nodes[:treeTruncateLimit], true
	}
	return nodes, defaultBranch, truncated, nil
}

func ghTreeToNodes(t *ghTreeResponse) []RadarTreeNode {
	out := make([]RadarTreeNode, 0, len(t.Tree))
	for _, n := range t.Tree {
		out = append(out, RadarTreeNode{Path: n.Path, Type: n.Type, Size: n.Size})
	}
	return out
}

type ghContentResponse struct {
	Type        string `json:"type"`
	Encoding    string `json:"encoding"`
	Size        int64  `json:"size"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	HTMLURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
}

// GetRepoContent 文件内容（base64，与 GitHub 一致）。
func (c *GitHubClient) GetRepoContent(ctx context.Context, fullName, path, ref string) (*ghContentResponse, error) {
	key := fmt.Sprintf("content:%s:%s:%s", fullName, path, ref)
	if v, ok := c.cacheGet(key); ok {
		return v.(*ghContentResponse), nil
	}
	url := fmt.Sprintf("%s/repos/%s/contents/%s", githubAPIBase, fullName, path)
	if ref != "" {
		url += "?ref=" + ref
	}
	var ct ghContentResponse
	if err := c.doJSON(ctx, url, &ct); err != nil {
		return nil, err
	}
	c.cacheSet(key, &ct, contentCacheTTL)
	return &ct, nil
}

// DecodeContent 尝试 base64 解码；返回 (文本, 是否文本, 错误)。
func DecodeContent(encoding, content string) (string, bool) {
	if encoding == "base64" {
		raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
		if err != nil {
			return "", false
		}
		if len(raw) > contentTruncateLimit {
			raw = raw[:contentTruncateLimit]
		}
		if bytesIndexByte(raw, 0) >= 0 {
			return "", false // 含 NUL → 二进制
		}
		return string(raw), true
	}
	return content, true
}

func bytesIndexByte(b []byte, ch byte) int {
	for i := range b {
		if b[i] == ch {
			return i
		}
	}
	return -1
}

// CloneRepo 服务端克隆（默认关，由配置控制）。
func (c *GitHubClient) CloneRepo(ctx context.Context, fullName, cloneDir string) (localPath string, err error) {
	if cloneDir == "" {
		return "", errors.New("clone dir not configured")
	}
	if err := os.MkdirAll(cloneDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir clone dir: %w", err)
	}
	dst := filepath.Join(cloneDir, strings.ReplaceAll(fullName, "/", "__"))
	if _, statErr := os.Stat(dst); statErr == nil {
		return dst, nil // 已存在则复用
	}
	ctx2, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx2, "git", "clone", "--depth", "1",
		fmt.Sprintf("https://github.com/%s.git", fullName), dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %v: %s", err, truncateStr(string(out), 300))
	}
	return dst, nil
}

// ── 快照与 AI 频控 ──

func (c *GitHubClient) saveSnapshot(key string, items []RadarRepoView) {
	c.mu.Lock()
	c.snapshots[key] = &snapshotEntry{items: items, at: time.Now()}
	c.mu.Unlock()
}

func (c *GitHubClient) loadSnapshot(key string) ([]RadarRepoView, bool) {
	c.mu.RLock()
	s, ok := c.snapshots[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return s.items, true
}

// allowInsightGen AI 生成频控：每 (repo, level) 每日 3 次。
func (c *GitHubClient) allowInsightGen(fullName, level string) bool {
	day := time.Now().UTC().Format("2006-01-02")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.genDay != day {
		c.genDay, c.genCount = day, map[string]int{}
	}
	k := fullName + "|" + level
	if c.genCount[k] >= 3 {
		return false
	}
	c.genCount[k]++
	return true
}

func (c *GitHubClient) insightCacheKey(fullName, level string) string {
	return "insight:" + fullName + ":" + level
}
