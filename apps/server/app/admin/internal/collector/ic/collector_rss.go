package iccollector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/mmcdole/gofeed"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// rss 采集器（直连 feed / RSSHub 自部署路由；AGPL 边界：RSSHub 独立服务经 HTTP 调用，
// 禁止 fork/嵌入主仓——02 §5 与 NOTICE 登记同源）
// 信源 config：feed_url（直连）| rsshub_route（如 "/bilibili/user/video/2262573"）
// =============================================================================

// RSSCollector feed 解析。
type RSSCollector struct {
	rsshubBase string
	timeout    time.Duration
	log        *log.Helper
	http       *httpFetcher
}

func NewRSSCollector(bc *conf.Bootstrap, ic *biz.IcConfig, logger log.Logger) *RSSCollector {
	rh := bc.GetIc().GetRsshub()
	timeout := 15 * time.Second
	if rh.GetTimeout() != nil {
		timeout = rh.GetTimeout().AsDuration()
	}
	return &RSSCollector{
		rsshubBase: strings.TrimRight(rh.GetBaseUrl(), "/"),
		timeout:    timeout,
		http:       newHTTPFetcher(timeout, ic.RobotsUserAgent, ic.EvidenceRawMaxBytes),
		log:        log.NewHelper(log.With(logger, "module", "ic/collector/rss")),
	}
}

func (c *RSSCollector) Method() string { return string(contract.CollectRSS) }

func (c *RSSCollector) Collect(ctx context.Context, src *biz.IntelSourceRow) ([]*biz.RawCollectItem, error) {
	feedURL := asString(src.Config["feed_url"])
	if feedURL == "" {
		route := asString(src.Config["rsshub_route"])
		if route == "" {
			return nil, errors.New("rss source config.feed_url / config.rsshub_route 至少一项必填")
		}
		if c.rsshubBase == "" {
			return nil, errors.New("ic.rsshub.base_url 未配置")
		}
		feedURL = c.rsshubBase + "/" + strings.TrimLeft(route, "/")
	}

	frag, err := c.http.fetch(ctx, feedURL, map[string]string{"Accept": "application/rss+xml, application/atom+xml, application/xml, text/xml"})
	if err != nil {
		return nil, err
	}
	if frag.HTTPStatus < 200 || frag.HTTPStatus >= 300 {
		return nil, fmt.Errorf("feed http status %d", frag.HTTPStatus)
	}

	parser := gofeed.NewParser()
	feed, err := parser.ParseString(string(frag.Content))
	if err != nil {
		return nil, err
	}

	out := make([]*biz.RawCollectItem, 0, len(feed.Items))
	for _, it := range feed.Items {
		link := it.Link
		if link == "" || it.Title == "" {
			continue
		}
		text := it.Description
		if text == "" && it.Content != "" { // content:encoded 优先摘要（gofeed 语义）
			text = it.Content
		}
		item := &biz.RawCollectItem{
			SourceURL: link,
			Title:     it.Title,
			Text:      text,
			Fragment:  frag,
			// Rank 恒 0：feed 顺序是时间序非热度序，伪造 rank 会污染 TT rank_pct 通道
		}
		if it.Author != nil {
			item.Author = it.Author.Name // 仅昵称位（PIPL）
		}
		if it.PublishedParsed != nil {
			ts := it.PublishedParsed.UTC()
			item.PublishedAt = &ts
		} else if it.UpdatedParsed != nil {
			ts := it.UpdatedParsed.UTC()
			item.PublishedAt = &ts
		}
		for _, enc := range it.Enclosures {
			if enc.URL != "" {
				item.MediaRefs = append(item.MediaRefs, enc.URL)
			}
		}
		if it.Image != nil && it.Image.URL != "" {
			item.MediaRefs = append(item.MediaRefs, it.Image.URL)
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, errors.New("rss 解析零条目")
	}
	return out, nil
}
