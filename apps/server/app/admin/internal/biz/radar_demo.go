package biz

import (
	"context"
	"os"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────
// DemoRadarTrendingReader：内存演示数据。
// 仅当环境变量 RADAR_DEMO=1 时启用（本地 UI 联调 / 无外网环境）；
// 生产默认走 GitHubRadarTrendingReader 真实数据源。
// TODO(ent): delete after data layer is wired.
// ─────────────────────────────────────────────────────────────

func RadarDemoEnabled() bool {
	return os.Getenv("RADAR_DEMO") == "1"
}

func demoRepos() []RadarRepoView {
	return []RadarRepoView{
		{
			ID: "1", FullName: "vercel/next.js", Description: "The React Framework for the Web",
			Language: "TypeScript", Topics: []string{"react", "framework", "ssr"},
			Stars: 130000, Forks: 28000, Rank: 1, StarsDelta: 420, IsNew: false,
			LicenseSPDX: "MIT", License: "MIT License",
			HTMLURL: "https://github.com/vercel/next.js", CloneURL: "https://github.com/vercel/next.js.git",
			SSHURL: "git@github.com:vercel/next.js.git", DefaultBranch: "canary",
			Sparkline: []int64{100, 120, 140, 180, 220, 300, 420},
		},
		{
			ID: "2", FullName: "golang/go", Description: "The Go programming language",
			Language: "Go", Topics: []string{"go", "language"},
			Stars: 128000, Forks: 18000, Rank: 2, StarsDelta: 210, IsNew: false,
			LicenseSPDX: "BSD-3-Clause", License: "BSD 3-Clause \"New\" or \"Revised\" License",
			HTMLURL: "https://github.com/golang/go", CloneURL: "https://github.com/golang/go.git",
			SSHURL: "git@github.com:golang/go.git", DefaultBranch: "master",
			Sparkline: []int64{80, 90, 100, 130, 150, 180, 210},
		},
		{
			ID: "3", FullName: "astral-sh/uv", Description: "An extremely fast Python package and project manager",
			Language: "Rust", Topics: []string{"python", "packaging"},
			Stars: 45000, Forks: 1200, Rank: 3, StarsDelta: 890, IsNew: true,
			LicenseSPDX: "Apache-2.0", License: "Apache License 2.0",
			HTMLURL: "https://github.com/astral-sh/uv", CloneURL: "https://github.com/astral-sh/uv.git",
			SSHURL: "git@github.com:astral-sh/uv.git", DefaultBranch: "main",
			Sparkline: []int64{50, 120, 250, 400, 600, 750, 890},
		},
		{
			ID: "4", FullName: "openai/whisper", Description: "Robust Speech Recognition via Large-Scale Weak Supervision",
			Language: "Python", Topics: []string{"ai", "speech"},
			Stars: 82000, Forks: 9800, Rank: 4, StarsDelta: 150, IsNew: false,
			LicenseSPDX: "MIT", License: "MIT License",
			HTMLURL: "https://github.com/openai/whisper", CloneURL: "https://github.com/openai/whisper.git",
			SSHURL: "git@github.com:openai/whisper.git", DefaultBranch: "main",
			Sparkline: []int64{100, 110, 115, 120, 130, 140, 150},
		},
		{
			ID: "5", FullName: "vuejs/core", Description: "Vue.js is a progressive, incrementally-adoptable JavaScript framework",
			Language: "TypeScript", Topics: []string{"vue", "frontend"},
			Stars: 49000, Forks: 8500, Rank: 5, StarsDelta: 95, IsNew: false,
			LicenseSPDX: "MIT", License: "MIT License",
			HTMLURL: "https://github.com/vuejs/core", CloneURL: "https://github.com/vuejs/core.git",
			SSHURL: "git@github.com:vuejs/core.git", DefaultBranch: "main",
			Sparkline: []int64{60, 70, 75, 80, 85, 90, 95},
		},
		{
			ID: "6", FullName: "denoland/deno", Description: "A modern runtime for JavaScript and TypeScript",
			Language: "Rust", Topics: []string{"javascript", "runtime"},
			Stars: 98000, Forks: 5400, Rank: 6, StarsDelta: 310, IsNew: true,
			LicenseSPDX: "MIT", License: "MIT License",
			HTMLURL: "https://github.com/denoland/deno", CloneURL: "https://github.com/denoland/deno.git",
			SSHURL: "git@github.com:denoland/deno.git", DefaultBranch: "main",
			Sparkline: []int64{40, 80, 120, 160, 220, 270, 310},
		},
	}
}

type DemoRadarTrendingReader struct{}

func (d *DemoRadarTrendingReader) ListLatest(_ context.Context, q RadarListQuery) (RadarListResult, error) {
	items := demoRepos()
	out := make([]RadarRepoView, 0, len(items))
	langTop := map[string]int32{}
	licTop := map[string]int32{}
	var rising, neu int32
	for _, it := range items {
		// 窗口级分布（驱动筛选下拉），不受筛选影响
		if it.Language != "" {
			langTop[it.Language]++
		}
		if it.LicenseSPDX != "" {
			licTop[it.LicenseSPDX]++
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
		if q.License != "" && !strings.EqualFold(it.LicenseSPDX, q.License) {
			continue
		}
		if q.OnlyNew && !it.IsNew {
			continue
		}
		ql := strings.ToLower(strings.TrimSpace(q.Query))
		if ql != "" && !strings.Contains(strings.ToLower(it.FullName), ql) &&
			!strings.Contains(strings.ToLower(it.Description), ql) {
			continue
		}
		// 过滤后口径统计（与 GitHubRadarTrendingReader.finalize 一致）
		if it.StarsDelta >= 200 {
			rising++
		}
		if it.IsNew {
			neu++
		}
		out = append(out, it)
	}
	total := int32(len(out))
	sortRadarItems(out, q.Sort)
	out = paginateDemoItems(out, q.Page, q.PageSize)
	return RadarListResult{
		Items: out, Stale: false, RefreshedAt: time.Now().UTC().Format(time.RFC3339),
		LanguageTop: langTop, LicenseTop: licTop, RisingCount: rising, NewCount: neu,
		Total: total, Window: "today",
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
	total := int32(len(out))
	sortRadarItems(out, "hot")
	out = paginateDemoItems(out, q.Page, q.PageSize)
	return RadarListResult{
		Items: out, Stale: false, RefreshedAt: time.Now().UTC().Format(time.RFC3339),
		LanguageTop: map[string]int32{}, LicenseTop: map[string]int32{},
		Total: total, Window: "week",
	}, nil
}

// paginateDemoItems 与 finalize 相同的分页/排名语义。
func paginateDemoItems(items []RadarRepoView, page, size int) []RadarRepoView {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 25
	}
	start := (page - 1) * size
	if start >= len(items) {
		return nil
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	out := items[start:end]
	for i := range out {
		out[i].Rank = int32(start + i + 1)
	}
	return out
}

func ProvideDemoRadarTrendingReader() RadarTrendingReader {
	return &DemoRadarTrendingReader{}
}
