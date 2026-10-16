package biz

import (
	"context"
	"strings"
	"time"
)

// RadarRepoView is the domain view for a GitHub repo on the radar board.
// TODO(ent): replace demo source with data.RadarTrendingRepo (ent / Postgres).
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
}

type RadarListQuery struct {
	Since    string
	Language string
	Topic    string
	OnlyNew  bool
	Page     int
	PageSize int
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
	RisingCount int32
	NewCount    int32
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

// DemoRadarTrendingReader returns in-memory sample data for UI / smoke.
// TODO(ent): delete after data layer is wired.
type DemoRadarTrendingReader struct{}

func demoRepos() []RadarRepoView {
	return []RadarRepoView{
		{
			ID: "1", FullName: "vercel/next.js", Description: "The React Framework for the Web",
			Language: "TypeScript", Topics: []string{"react", "framework", "ssr"},
			Stars: 130000, Forks: 28000, Rank: 1, StarsDelta: 420, IsNew: false,
			HTMLURL: "https://github.com/vercel/next.js", Sparkline: []int64{100, 120, 140, 180, 220, 300, 420},
		},
		{
			ID: "2", FullName: "golang/go", Description: "The Go programming language",
			Language: "Go", Topics: []string{"go", "language"},
			Stars: 128000, Forks: 18000, Rank: 2, StarsDelta: 210, IsNew: false,
			HTMLURL: "https://github.com/golang/go", Sparkline: []int64{80, 90, 100, 130, 150, 180, 210},
		},
		{
			ID: "3", FullName: "astral-sh/uv", Description: "An extremely fast Python package and project manager",
			Language: "Rust", Topics: []string{"python", "packaging"},
			Stars: 45000, Forks: 1200, Rank: 3, StarsDelta: 890, IsNew: true,
			HTMLURL: "https://github.com/astral-sh/uv", Sparkline: []int64{50, 120, 250, 400, 600, 750, 890},
		},
		{
			ID: "4", FullName: "openai/whisper", Description: "Robust Speech Recognition via Large-Scale Weak Supervision",
			Language: "Python", Topics: []string{"ai", "speech"},
			Stars: 82000, Forks: 9800, Rank: 4, StarsDelta: 150, IsNew: false,
			HTMLURL: "https://github.com/openai/whisper", Sparkline: []int64{100, 110, 115, 120, 130, 140, 150},
		},
		{
			ID: "5", FullName: "vuejs/core", Description: "Vue.js is a progressive, incrementally-adoptable JavaScript framework",
			Language: "TypeScript", Topics: []string{"vue", "frontend"},
			Stars: 49000, Forks: 8500, Rank: 5, StarsDelta: 95, IsNew: false,
			HTMLURL: "https://github.com/vuejs/core", Sparkline: []int64{60, 70, 75, 80, 85, 90, 95},
		},
		{
			ID: "6", FullName: "denoland/deno", Description: "A modern runtime for JavaScript and TypeScript",
			Language: "Rust", Topics: []string{"javascript", "runtime"},
			Stars: 98000, Forks: 5400, Rank: 6, StarsDelta: 310, IsNew: true,
			HTMLURL: "https://github.com/denoland/deno", Sparkline: []int64{40, 80, 120, 160, 220, 270, 310},
		},
	}
}

func (d *DemoRadarTrendingReader) ListLatest(_ context.Context, q RadarListQuery) (RadarListResult, error) {
	items := demoRepos()
	out := make([]RadarRepoView, 0, len(items))
	langTop := map[string]int32{}
	var rising, neu int32
	for _, it := range items {
		langTop[it.Language]++
		if it.StarsDelta >= 200 {
			rising++
		}
		if it.IsNew {
			neu++
		}
		if q.Language != "" && !strings.EqualFold(it.Language, q.Language) {
			continue
		}
		if q.Topic != "" {
			ok := false
			for _, t := range it.Topics {
				if strings.EqualFold(t, q.Topic) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if q.OnlyNew && !it.IsNew {
			continue
		}
		out = append(out, it)
	}
	return RadarListResult{
		Items: out, Stale: false, RefreshedAt: time.Now().UTC().Format(time.RFC3339),
		LanguageTop: langTop, RisingCount: rising, NewCount: neu,
	}, nil
}

func (d *DemoRadarTrendingReader) SearchRising(_ context.Context, q RadarRisingQuery) (RadarListResult, error) {
	items := demoRepos()
	out := make([]RadarRepoView, 0)
	for _, it := range items {
		if q.Language != "" && !strings.EqualFold(it.Language, q.Language) {
			continue
		}
		if q.Query != "" {
			ql := strings.ToLower(q.Query)
			if !strings.Contains(strings.ToLower(it.FullName), ql) &&
				!strings.Contains(strings.ToLower(it.Description), ql) {
				continue
			}
		}
		out = append(out, it)
	}
	// sort-ish by delta already in demo order for rising
	return RadarListResult{
		Items: out, Stale: false, RefreshedAt: time.Now().UTC().Format(time.RFC3339),
		LanguageTop: map[string]int32{},
	}, nil
}

func ProvideDemoRadarTrendingReader() RadarTrendingReader {
	return &DemoRadarTrendingReader{}
}

