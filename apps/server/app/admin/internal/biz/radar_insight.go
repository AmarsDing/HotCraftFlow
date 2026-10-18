package biz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────
// Radar Insight：OpenAI 兼容 LLM 客户端 + 三级总结 Prompt + 缓存/频控。
// 等级：brief（一句话） / summary（速览） / deep（深度）。
// ─────────────────────────────────────────────────────────────

// Insight levels.
const (
	InsightLevelBrief   = "brief"
	InsightLevelSummary = "summary"
	InsightLevelDeep    = "deep"
)

// RadarLLMConfig LLM 配置快照（由 RadarConfigService 提供）。
type RadarLLMConfig struct {
	Enabled bool
	BaseURL string
	APIKey  string
	Model   string
}

// RadarConfigProvider 雷达配置提供者（service 层实现）。
type RadarConfigProvider interface {
	GitHubToken() string
	RadarLLM() RadarLLMConfig
	CloneSettings() (enabled bool, dir string)
}

// InsightService AI 总结服务。
type InsightService struct {
	gh  *GitHubClient
	cfg RadarConfigProvider
}

func NewInsightService(gh *GitHubClient, cfg RadarConfigProvider) *InsightService {
	return &InsightService{gh: gh, cfg: cfg}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// complete 调用 OpenAI 兼容 chat/completions。
func (s *InsightService) complete(ctx context.Context, llm RadarLLMConfig, system, user string, maxTokens int) (string, error) {
	base := strings.TrimRight(llm.BaseURL, "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if !strings.HasSuffix(base, "/v1") && !strings.Contains(base, "/v1/") {
		base += "/v1"
	}
	reqBody := chatRequest{
		Model:       llm.Model,
		Messages:    []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	}
	if reqBody.Model == "" {
		reqBody.Model = "gpt-4o-mini"
	}
	buf, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+llm.APIKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("llm decode: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm empty choices (status %d)", resp.StatusCode)
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// Generate 生成指定等级总结（缓存 24h；频控每日 3 次/级）。
func (s *InsightService) Generate(ctx context.Context, fullName, level string) (*RadarInsight, error) {
	switch level {
	case InsightLevelBrief, InsightLevelSummary, InsightLevelDeep:
	default:
		return nil, fmt.Errorf("invalid insight level: %s (brief|summary|deep)", level)
	}
	llm := s.cfg.RadarLLM()
	if !llm.Enabled || strings.TrimSpace(llm.APIKey) == "" {
		return &RadarInsight{
			FullName: fullName, Level: level,
			Error: "未配置 LLM：请在雷达设置中填写 OpenAI 兼容服务的 Base URL / API Key / 模型",
		}, nil
	}
	key := s.gh.insightCacheKey(fullName, level)
	if v, ok := s.gh.cacheGet(key); ok {
		ins := v.(*RadarInsight)
		ins.Cached = true
		return ins, nil
	}
	if !s.gh.allowInsightGen(fullName, level) {
		return &RadarInsight{
			FullName: fullName, Level: level,
			Error: "该仓库该等级今日生成次数已达上限（3 次），请稍后重试",
		}, nil
	}

	started := time.Now()
	repo, err := s.gh.GetRepoDetail(ctx, fullName)
	if err != nil {
		return nil, err
	}
	readme := ""
	if txt, rerr := s.gh.GetReadme(ctx, fullName); rerr == nil {
		readme = txt
	}
	system, user, maxTok := buildInsightPrompt(level, repo, readme)
	content, err := s.complete(ctx, llm, system, user, maxTok)
	ins := &RadarInsight{
		FullName: fullName, Level: level, Model: llm.Model,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		DurationMs:  time.Since(started).Milliseconds(),
	}
	if err != nil {
		ins.Error = err.Error()
		return ins, nil
	}
	ins.ContentMarkdown = content
	s.gh.cacheSet(key, ins, 24*time.Hour)
	return ins, nil
}

// buildInsightPrompt 构造三级 Prompt。
func buildInsightPrompt(level string, repo *RadarRepoView, readme string) (system, user string, maxTokens int) {
	system = "你是一位资深开源生态分析师，擅长用简体中文解读 GitHub 项目。" +
		"输出使用 Markdown，语言简洁专业，不编造无法从给定材料推断的事实；不确定处明确说\"材料不足，无法判断\"。"

	meta := fmt.Sprintf(`仓库：%s
描述：%s
主语言：%s
Topics：%s
Stars：%d（窗口增量 %d） Forks：%d Open Issues：%d Watchers：%d
License：%s
创建：%s 最近推送：%s
语言构成：%s`,
		repo.FullName, repo.Description, repo.Language, strings.Join(repo.Topics, ", "),
		repo.Stars, repo.StarsDelta, repo.Forks, repo.OpenIssues, repo.Watchers,
		firstNonEmpty(repo.LicenseSPDX, repo.License, "未知"),
		repo.CreatedAt, repo.PushedAt, langStatsText(repo.Languages))

	readmePart := "（无 README 或获取失败，以下结论仅基于元数据推断，请注明）"
	if strings.TrimSpace(readme) != "" {
		readmePart = truncateRunes(stripMarkdownNoise(readme), 8000)
	}

	switch level {
	case InsightLevelBrief:
		system += "本轮任务：用一句话（不超过 50 字）概括这个项目是什么、解决什么问题。只输出这一句话。"
		return system, meta + "\n\nREADME 摘录：\n" + readmePart, 120
	case InsightLevelSummary:
		system += "本轮任务：输出结构化速览，固定五节：**是什么** / **技术栈** / **核心亮点** / **适用场景** / **热度信号**（结合 Stars 与增量）。总长 200-400 字。"
		return system, meta + "\n\nREADME 摘录：\n" + readmePart, 700
	default: // deep
		system += "本轮任务：输出深度分析，固定五节：**架构与设计推测** / **优势** / **风险与局限** / **社区健康度**（结合 Issues 数、贡献者数、推送时间推断） / **上手建议**。总长 500-900 字，观点给出依据。"
		return system, meta + "\n\nREADME 摘录：\n" + readmePart, 1400
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func langStatsText(stats []RadarLanguageStat) string {
	if len(stats) == 0 {
		return "未知"
	}
	parts := make([]string, 0, len(stats))
	for i, s := range stats {
		if i >= 5 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %.1f%%", s.Name, s.Percent))
	}
	return strings.Join(parts, "，")
}

// stripMarkdownNoise 去徽章/图片/HTML 噪声，压缩空行。
func stripMarkdownNoise(md string) string {
	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "![") || strings.HasPrefix(t, "<img") ||
			strings.HasPrefix(t, "<a ") || strings.HasPrefix(t, "<p>") ||
			strings.HasPrefix(t, "[!") || strings.HasPrefix(t, "---") ||
			strings.HasPrefix(t, "===") {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// truncateRunes 按字符截断（避免截断多字节字符产生乱码）。
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
