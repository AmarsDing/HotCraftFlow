package biz

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// RadarLanguageStat 仓库语言构成。
type RadarLanguageStat struct {
	Name    string
	Bytes   int64
	Percent float64
}

// RadarRepoView is the domain view for a GitHub repo on the radar board.
type RadarRepoView struct {
	ID          string
	FullName    string
	Description string
	Language    string
	Topics      []string
	Stars       int64
	Forks       int64
	Rank        int32
	StarsDelta  int64
	IsNew       bool
	HTMLURL     string
	Sparkline   []int64

	// ── v2 新增（GitHub 官网卡片密度） ──
	License       string
	LicenseSPDX   string
	Homepage      string
	OpenIssues    int64
	Watchers      int64
	Contributors  []string
	CreatedAt     string
	UpdatedAt     string
	PushedAt      string
	CloneURL      string
	SSHURL        string
	ZipURL        string
	DefaultBranch string
	Languages     []RadarLanguageStat
	ReadmeExcerpt string
}

type RadarListQuery struct {
	Since    string
	Language string
	Topic    string
	OnlyNew  bool
	Page     int
	PageSize int
	Sort     string // hot | stars | updated | new
	Query    string
	License  string
}

type RadarRisingQuery struct {
	Query    string
	Language string
	Page     int
	PageSize int
}

type RadarListResult struct {
	Items       []RadarRepoView
	Stale       bool
	StaleReason string
	RefreshedAt string
	LanguageTop map[string]int32
	LicenseTop  map[string]int32
	RisingCount int32
	NewCount    int32
	Window      string
	Total       int32 // 过滤后总数（分页用）
}

// RadarTrendingReader abstracts persistence / cache.
type RadarTrendingReader interface {
	ListLatest(ctx context.Context, q RadarListQuery) (RadarListResult, error)
	SearchRising(ctx context.Context, q RadarRisingQuery) (RadarListResult, error)
}

type RadarTrendingUsecase struct {
	repo RadarTrendingReader
}

func NewRadarTrendingUsecase(repo RadarTrendingReader) *RadarTrendingUsecase {
	if repo == nil {
		repo = &DemoRadarTrendingReader{}
	}
	return &RadarTrendingUsecase{repo: repo}
}

func (uc *RadarTrendingUsecase) List(ctx context.Context, q RadarListQuery) (RadarListResult, error) {
	return uc.repo.ListLatest(ctx, q)
}

func (uc *RadarTrendingUsecase) Rising(ctx context.Context, q RadarRisingQuery) (RadarListResult, error) {
	return uc.repo.SearchRising(ctx, q)
}

// ─────────────────────────────────────────────────────────────
// GitHubRadarTrendingReader：真实 GitHub 数据源（双引擎 + 快照 + 缓存）。
// 抓取失败时返回上一个好快照并标 stale。
// ─────────────────────────────────────────────────────────────

type GitHubRadarTrendingReader struct {
	gh *GitHubClient
}

func NewGitHubRadarTrendingReader(gh *GitHubClient) *GitHubRadarTrendingReader {
	return &GitHubRadarTrendingReader{gh: gh}
}

func normalizeSince(since string) string {
	switch since {
	case "week", "month":
		return since
	default:
		return "today"
	}
}

func snapshotKey(since, language string) string {
	return since + "|" + language
}

// ListLatest 热门榜：HTML 优先 → Search 兜底 → 并发补全详情 → 过滤/排序/分页。
func (r *GitHubRadarTrendingReader) ListLatest(ctx context.Context, q RadarListQuery) (RadarListResult, error) {
	since := normalizeSince(q.Since)
	key := snapshotKey(since, q.Language)

	if v, ok := r.gh.cacheGet("trending:" + key); ok {
		return r.finalize(v.(*RadarListResult), q, since), nil
	}

	entries, err := r.gh.FetchTrending(ctx, since, q.Language)
	if err != nil {
		// 回退上一个好快照
		if snap, ok := r.gh.loadSnapshot(key); ok {
			res := &RadarListResult{Items: snap, Stale: true, StaleReason: "抓取失败，展示最近一次成功快照：" + truncateStr(err.Error(), 120)}
			return r.finalize(res, q, since), nil
		}
		return RadarListResult{Stale: true, StaleReason: truncateStr(err.Error(), 160)}, nil
	}

	views := r.enrichEntries(ctx, entries, key, since)
	res := &RadarListResult{Items: views, RefreshedAt: time.Now().UTC().Format(time.RFC3339)}
	r.gh.saveSnapshot(key, views)
	r.gh.cacheSet("trending:"+key, res, trendingCacheTTL)
	return r.finalize(res, q, since), nil
}

// enrichEntries 并发补全详情（限并发 4 / 上限 25），单点失败保留基础字段。
func (r *GitHubRadarTrendingReader) enrichEntries(ctx context.Context, entries []ghTrendingEntry, key, since string) []RadarRepoView {
	prev, _ := r.gh.loadSnapshot(key)
	prevMap := map[string]*RadarRepoView{}
	for i := range prev {
		prevMap[prev[i].FullName] = &prev[i]
	}

	limit := len(entries)
	if limit > trendingEnrichLimit {
		limit = trendingEnrichLimit
	}
	detailed := make([]*RadarRepoView, limit)
	sem := make(chan struct{}, trendingEnrichConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			d, err := r.gh.GetRepoDetail(ctx, entries[i].FullName)
			if err != nil {
				return // 保留基础字段
			}
			detailed[i] = d
		}(i)
	}
	wg.Wait()

	out := make([]RadarRepoView, 0, len(entries))
	for i, e := range entries {
		var v RadarRepoView
		if i < limit && detailed[i] != nil {
			v = *detailed[i]
		} else {
			v = RadarRepoView{FullName: e.FullName}
		}
		// HTML 基础字段回填（detail 缺失时的保底，且增量只有 HTML 引擎有）
		if v.Description == "" {
			v.Description = e.Description
		}
		if v.Language == "" {
			v.Language = e.Language
		}
		if v.Stars == 0 {
			v.Stars = e.Stars
		}
		if v.Forks == 0 {
			v.Forks = e.Forks
		}
		if v.HTMLURL == "" {
			v.HTMLURL = "https://github.com/" + e.FullName
		}
		v.StarsDelta = e.StarsDelta
		if len(v.Contributors) == 0 {
			v.Contributors = e.BuiltBy
		}
		// 快照差计算 sparkline（最近 7 个点）
		if p, ok := prevMap[e.FullName]; ok {
			v.Sparkline = appendSparkline(p.Sparkline, e.StarsDelta)
		} else if e.StarsDelta > 0 {
			v.Sparkline = appendSparkline(nil, e.StarsDelta)
		}
		// is_new：窗口内创建
		if v.CreatedAt != "" && parseTime(v.CreatedAt).After(windowStart(since)) {
			v.IsNew = true
		}
		out = append(out, v)
	}
	return out
}

func appendSparkline(prev []int64, delta int64) []int64 {
	next := append(append([]int64{}, prev...), delta)
	if len(next) > 7 {
		next = next[len(next)-7:]
	}
	return next
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// finalize 过滤 + 排序 + 排名 + 分页 + 统计。
func (r *GitHubRadarTrendingReader) finalize(res *RadarListResult, q RadarListQuery, since string) RadarListResult {
	if res == nil {
		return RadarListResult{Window: since}
	}
	out := RadarListResult{
		Stale: res.Stale, StaleReason: res.StaleReason,
		RefreshedAt: res.RefreshedAt,
		LanguageTop: map[string]int32{}, LicenseTop: map[string]int32{},
		Window: since,
	}
	ql := strings.ToLower(strings.TrimSpace(q.Query))
	var rising, neu int32
	for _, it := range res.Items {
		// LanguageTop/LicenseTop 为窗口级分布（驱动筛选下拉选项），不受筛选条件影响。
		if it.Language != "" {
			out.LanguageTop[it.Language]++
		}
		if it.LicenseSPDX != "" {
			out.LicenseTop[it.LicenseSPDX]++
		}
		if q.Language != "" && !strings.EqualFold(it.Language, q.Language) {
			continue
		}
		if q.Topic != "" && !hasTopic(it.Topics, q.Topic) {
			continue
		}
		if q.License != "" && !strings.EqualFold(it.LicenseSPDX, q.License) {
			continue
		}
		if q.OnlyNew && !it.IsNew {
			continue
		}
		if ql != "" && !matchesQuery(it, ql) {
			continue
		}
		// rising/neu/total 均为过滤后口径，与前端 KPI/分页一致。
		if it.StarsDelta > 0 {
			rising++
		}
		if it.IsNew {
			neu++
		}
		out.Items = append(out.Items, it)
	}
	out.RisingCount, out.NewCount = rising, neu
	out.Total = int32(len(out.Items))

	sortRadarItems(out.Items, q.Sort)

	page, size := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 25
	}
	start := (page - 1) * size
	if start >= len(out.Items) {
		out.Items = nil
	} else {
		end := start + size
		if end > len(out.Items) {
			end = len(out.Items)
		}
		out.Items = out.Items[start:end]
	}
	for i := range out.Items {
		out.Items[i].Rank = int32(start + i + 1)
	}
	return out
}

func hasTopic(topics []string, want string) bool {
	for _, t := range topics {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func matchesQuery(it RadarRepoView, q string) bool {
	ql := strings.ToLower(strings.TrimSpace(q))
	if ql == "" {
		return true
	}
	if strings.Contains(strings.ToLower(it.FullName), ql) ||
		strings.Contains(strings.ToLower(it.Description), ql) {
		return true
	}
	for _, t := range it.Topics {
		if strings.Contains(strings.ToLower(t), ql) {
			return true
		}
	}
	return false
}

// sortRadarItems 排序：hot（窗口增量，默认）| stars | updated | new。
func sortRadarItems(items []RadarRepoView, sortBy string) {
	switch sortBy {
	case "stars":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Stars > items[j].Stars })
	case "updated":
		sort.SliceStable(items, func(i, j int) bool { return parseTime(items[i].PushedAt).After(parseTime(items[j].PushedAt)) })
	case "new":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].IsNew != items[j].IsNew {
				return items[i].IsNew
			}
			return items[i].StarsDelta > items[j].StarsDelta
		})
	default: // hot
		sort.SliceStable(items, func(i, j int) bool { return items[i].StarsDelta > items[j].StarsDelta })
	}
}

// SearchRising 新星发现：Search API（stars 排序兜底 trending HTML）。
func (r *GitHubRadarTrendingReader) SearchRising(ctx context.Context, q RadarRisingQuery) (RadarListResult, error) {
	key := "rising|" + q.Language
	if v, ok := r.gh.cacheGet("rising:" + key); ok {
		return r.finalizeRising(v.(*RadarListResult), q), nil
	}
	entries, err := r.gh.FetchTrending(ctx, "week", q.Language)
	if err != nil {
		if snap, ok := r.gh.loadSnapshot(key); ok {
			res := &RadarListResult{Items: snap, Stale: true, StaleReason: "抓取失败，展示最近一次成功快照"}
			return r.finalizeRising(res, q), nil
		}
		return RadarListResult{Stale: true, StaleReason: truncateStr(err.Error(), 160)}, nil
	}
	views := r.enrichEntries(ctx, entries, key, "week")
	res := &RadarListResult{Items: views, RefreshedAt: time.Now().UTC().Format(time.RFC3339)}
	r.gh.saveSnapshot(key, views)
	r.gh.cacheSet("rising:"+key, res, trendingCacheTTL)
	return r.finalizeRising(res, q), nil
}

func (r *GitHubRadarTrendingReader) finalizeRising(res *RadarListResult, q RadarRisingQuery) RadarListResult {
	fq := RadarListQuery{Since: "week", Language: q.Language, Query: q.Query, Sort: "hot", Page: q.Page, PageSize: q.PageSize}
	out := r.finalize(res, fq, "week")
	return out
}
