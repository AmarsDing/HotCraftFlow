package iccollector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// crawl 采集器（Crawl4AI 自部署 REST；B-12 max_attempts 统一含首次 3 次）
// 信源 config：targets: ["https://…"] 或 "url": 单目标；extract_css: 可选选择器。
// TODO(deploy-verify)：Crawl4AI 版本间 REST 负载（markdown 字段命名 media.videos
// 结构）有差异，部署后按实例 OpenAPI 收敛本解码结构（保持宽松解码，勿收窄必填）。
// =============================================================================

// CrawlCollector Crawl4AI 爬取。
type CrawlCollector struct {
	baseURL string
	http    *httpFetcher
	retry   int
	log     *log.Helper
}

func NewCrawlCollector(bc *conf.Bootstrap, ic *biz.IcConfig, logger log.Logger) *CrawlCollector {
	c4 := bc.GetIc().GetCrawl4Ai()
	timeout := 60 * time.Second
	if c4.GetTimeout() != nil {
		timeout = c4.GetTimeout().AsDuration()
	}
	retry := int(c4.GetRetryAttempts())
	if retry <= 0 {
		retry = 3
	}
	return &CrawlCollector{
		baseURL: strings.TrimRight(c4.GetBaseUrl(), "/"),
		http:    newHTTPFetcher(timeout, ic.RobotsUserAgent, ic.EvidenceRawMaxBytes),
		retry:   retry,
		log:     log.NewHelper(log.With(logger, "module", "ic/collector/crawl")),
	}
}

func (c *CrawlCollector) Method() string { return string(contract.CollectCrawl) }

type crawlRequest struct {
	URLs            []string `json:"urls"`
	IncludeMarkdown bool     `json:"include_markdown"`
	CSSSelector     string   `json:"css_selector,omitempty"`
}

type crawlResult struct {
	URL        string `json:"url"`
	Markdown   string `json:"markdown"`
	HTML       string `json:"html"`
	StatusCode int    `json:"status_code"`
	Metadata   *struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		PublishedAt any    `json:"published_at"`
	} `json:"metadata"`
	Media *struct {
		Videos []struct {
			Src string `json:"src"`
		} `json:"videos"`
		Images []struct {
			Src string `json:"src"`
		} `json:"images"`
	} `json:"media"`
}

type crawlResponse struct {
	Success bool          `json:"success"`
	Results []crawlResult `json:"results"`
}

func (c *CrawlCollector) Collect(ctx context.Context, src *biz.IntelSourceRow) ([]*biz.RawCollectItem, error) {
	if c.baseURL == "" {
		return nil, errors.New("ic.crawl4ai.base_url 未配置")
	}
	targets := collectTargets(src.Config)
	if len(targets) == 0 {
		return nil, errors.New("crawl source config.targets / config.url 必填")
	}
	req := &crawlRequest{URLs: targets, IncludeMarkdown: true}
	if v := asString(src.Config["extract_css"]); v != "" {
		req.CSSSelector = v
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// B-12：max_attempts 含首次统一（默认 3）；可恢复错误重试，4xx 不重试。
	var resp crawlResponse
	var lastErr error
	for attempt := 1; attempt <= c.retry; attempt++ {
		lastErr = c.callOnce(ctx, body, &resp)
		if lastErr == nil {
			break
		}
		if errors.Is(lastErr, errCrawlClientError) {
			return nil, lastErr
		}
		if attempt < c.retry {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}

	out := make([]*biz.RawCollectItem, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.URL == "" {
			continue
		}
		item := &biz.RawCollectItem{SourceURL: r.URL, Text: r.Markdown}
		if r.Metadata != nil {
			item.Title = r.Metadata.Title
			if item.Text == "" {
				item.Text = r.Metadata.Description
			}
			if ts, ok := asTime(r.Metadata.PublishedAt); ok {
				item.PublishedAt = &ts
			}
		}
		if r.Media != nil {
			for _, v := range r.Media.Videos {
				if v.Src != "" {
					item.MediaRefs = append(item.MediaRefs, v.Src)
				}
			}
			for _, img := range r.Media.Images {
				if img.Src != "" {
					item.MediaRefs = append(item.MediaRefs, img.Src)
				}
			}
		}
		// raw_html 作为该条目的证据片段（单目标一次抓取，无共享成本）
		item.Fragment = &contract.RawFragment{
			Content:    []byte(r.HTML),
			HTTPStatus: r.StatusCode,
			RequestURL: r.URL,
			FinalURL:   r.URL,
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, errors.New("crawl4ai 解析零结果")
	}
	return out, nil
}

var errCrawlClientError = errors.New("crawl4ai 4xx（配置问题，不重试）")

func (c *CrawlCollector) callOnce(ctx context.Context, body []byte, out *crawlResponse) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/crawl", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.http.userAgent)
	resp, err := c.http.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.http.readLimit+1))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return fmt.Errorf("%w: status %d", errCrawlClientError, resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("crawl4ai status %d", resp.StatusCode)
	}
	return decodeJSON(raw, out)
}

// collectTargets 兼容 config.targets 数组与 config.url 单值。
func collectTargets(cfg map[string]interface{}) []string {
	var out []string
	if arr, ok := cfg["targets"].([]interface{}); ok {
		for _, v := range arr {
			if s := asString(v); s != "" {
				out = append(out, s)
			}
		}
	}
	if s := asString(cfg["url"]); s != "" {
		out = append(out, s)
	}
	return out
}
