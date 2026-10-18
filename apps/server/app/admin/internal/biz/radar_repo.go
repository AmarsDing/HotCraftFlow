package biz

import (
	"context"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────
// RadarRepoUsecase：仓库详情 / 文件树 / 文件内容 / 克隆 / AI 总结编排。
// ─────────────────────────────────────────────────────────────

// RadarRepoUsecase 编排 GitHub 客户端与 Insight 服务。
type RadarRepoUsecase struct {
	gh      *GitHubClient
	insight *InsightService
	cfg     RadarConfigProvider
}

func NewRadarRepoUsecase(gh *GitHubClient, insight *InsightService, cfg RadarConfigProvider) *RadarRepoUsecase {
	return &RadarRepoUsecase{gh: gh, insight: insight, cfg: cfg}
}

// RadarInsight AI 总结视图。
type RadarInsight struct {
	FullName        string
	Level           string
	ContentMarkdown string
	Model           string
	GeneratedAt     string
	Cached          bool
	DurationMs      int64
	Error           string
}

// RadarTreeNode 文件树节点。
type RadarTreeNode struct {
	Path string
	Type string // blob | tree
	Size int64
}

// RadarRepoContent 文件内容视图。
type RadarRepoContent struct {
	Path        string
	Name        string
	Content     string
	Encoding    string
	Size        int64
	Truncated   bool
	HTMLURL     string
	DownloadURL string
	IsText      bool
}

// RadarCloneResult 克隆信息。
type RadarCloneResult struct {
	CloneURL   string
	SSHURL     string
	ZipURL     string
	Command    string
	SSHCommand string
	LocalPath  string
	Cloned     bool
	Message    string
}

// GetRepo 详情（可选 README + 指定等级 Insight）。
// insightCacheOnly 为 true 时仅回读 24h 本地缓存快照（不触发生成），未命中 insight 为空。
func (uc *RadarRepoUsecase) GetRepo(ctx context.Context, fullName string, includeReadme bool, insightLevel string, insightCacheOnly bool) (repo *RadarRepoView, readme string, insight *RadarInsight, stale bool, staleReason string, err error) {
	normalized, verr := validateFullName(fullName)
	if verr != nil {
		return nil, "", nil, false, "", verr
	}
	fullName = normalized
	repo, err = uc.gh.GetRepoDetail(ctx, fullName)
	if err != nil {
		return nil, "", nil, true, truncateStr(err.Error(), 160), err
	}
	if includeReadme {
		if txt, rerr := uc.gh.GetReadme(ctx, fullName); rerr == nil {
			readme = txt
			repo.ReadmeExcerpt = truncateRunes(txt, 2000)
		}
	}
	if insightLevel != "" {
		if insightCacheOnly {
			insight = uc.insight.GetCached(fullName, insightLevel)
		} else {
			ins, ierr := uc.insight.Generate(ctx, fullName, insightLevel)
			if ierr != nil {
				return nil, "", nil, false, "", ierr
			}
			insight = ins
		}
	}
	return repo, readme, insight, false, "", nil
}

// GetRepoTree 文件树。
func (uc *RadarRepoUsecase) GetRepoTree(ctx context.Context, fullName, branch string) (nodes []RadarTreeNode, defaultBranch string, truncated bool, err error) {
	normalized, verr := validateFullName(fullName)
	if verr != nil {
		return nil, "", false, verr
	}
	ghNodes, def, trunc, err := uc.gh.GetRepoTree(ctx, normalized, branch)
	if err != nil {
		return nil, "", false, err
	}
	nodes = make([]RadarTreeNode, 0, len(ghNodes))
	for _, n := range ghNodes {
		nodes = append(nodes, RadarTreeNode{Path: n.Path, Type: n.Type, Size: n.Size})
	}
	return nodes, def, trunc, nil
}

// GetRepoContent 文件内容。
func (uc *RadarRepoUsecase) GetRepoContent(ctx context.Context, fullName, path, ref string) (*RadarRepoContent, error) {
	fullName, err := validateFullName(fullName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path required")
	}
	if strings.Contains(path, "..") {
		return nil, fmt.Errorf("invalid path")
	}
	ct, err := uc.gh.GetRepoContent(ctx, fullName, path, ref)
	if err != nil {
		return nil, err
	}
	out := &RadarRepoContent{
		Path: ct.Path, Name: ct.Name, Content: ct.Content,
		Encoding: ct.Encoding, Size: ct.Size,
		HTMLURL: ct.HTMLURL, DownloadURL: ct.DownloadURL,
	}
	if text, isText := DecodeContent(ct.Encoding, ct.Content); isText {
		out.IsText = true
		out.Encoding = "utf-8"
		out.Content = text
		if int64(len(text)) >= contentTruncateLimit {
			out.Truncated = true
		}
	} else {
		out.Encoding = "binary"
		out.Content = ""
	}
	return out, nil
}

// CloneRepo 克隆信息；toServer 且配置开启时执行服务端克隆。
func (uc *RadarRepoUsecase) CloneRepo(ctx context.Context, fullName string, toServer bool) (*RadarCloneResult, error) {
	fullName, err := validateFullName(fullName)
	if err != nil {
		return nil, err
	}
	cloneURL := fmt.Sprintf("https://github.com/%s.git", fullName)
	res := &RadarCloneResult{
		CloneURL:   cloneURL,
		SSHURL:     fmt.Sprintf("git@github.com:%s.git", fullName),
		ZipURL:     fmt.Sprintf("https://github.com/%s/archive/refs/heads/HEAD.zip", fullName),
		Command:    "git clone " + cloneURL,
		SSHCommand: fmt.Sprintf("git clone git@github.com:%s.git", fullName),
	}
	if !toServer {
		return res, nil
	}
	enabled, dir := uc.cfg.CloneSettings()
	if !enabled {
		res.Message = "服务端克隆未开启：请在雷达设置中开启并配置目录"
		return res, nil
	}
	local, err := uc.gh.CloneRepo(ctx, fullName, dir)
	if err != nil {
		res.Message = err.Error()
		return res, nil
	}
	res.Cloned, res.LocalPath = true, local
	res.Message = "已克隆到服务端目录"
	return res, nil
}

// GenerateInsight 生成/取缓存 AI 总结。
func (uc *RadarRepoUsecase) GenerateInsight(ctx context.Context, fullName, level string) (*RadarInsight, error) {
	fullName, err := validateFullName(fullName)
	if err != nil {
		return nil, err
	}
	return uc.insight.Generate(ctx, fullName, level)
}

// validateFullName 必须是 owner/repo 形态（公开仓库）。
// 复用 biz 层统一校验：字符集严格（防 GitHub URL 路径穿越/查询参数注入）+ 长度上限，错误码为 400。
func validateFullName(fullName string) (string, error) {
	return normalizeFullName(fullName)
}
