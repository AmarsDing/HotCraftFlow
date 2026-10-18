package iccollector

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// api 采集器（官方/合作方 JSON API；配置驱动字段映射，见 config keys）
// 信源 config：
//   url(必)、headers(map)、items_path(必，如 "data.list")
//   title_path、url_path(必)、text_path、author_path、published_path
//   views_path、likes_path、comments_path、rank_from_index(bool)
// =============================================================================

// APICollector 通用 JSON API 采集。
type APICollector struct {
	http *httpFetcher
	log  *log.Helper
}

func NewAPICollector(bc *conf.Bootstrap, ic *biz.IcConfig, logger log.Logger) *APICollector {
	timeout := 10 * time.Second
	if t := bc.GetIc().GetNewsnow().GetTimeout(); t != nil { // api 端点超时借用 newsnow 档位（无独立配置位）
		timeout = t.AsDuration()
	}
	return &APICollector{
		http: newHTTPFetcher(timeout, ic.RobotsUserAgent, ic.EvidenceRawMaxBytes),
		log:  log.NewHelper(log.With(logger, "module", "ic/collector/api")),
	}
}

func (c *APICollector) Method() string { return string(contract.CollectAPI) }

func (c *APICollector) Collect(ctx context.Context, src *biz.IntelSourceRow) ([]*biz.RawCollectItem, error) {
	rawURL := asString(src.Config["url"])
	itemsPath := asString(src.Config["items_path"])
	if rawURL == "" || itemsPath == "" {
		return nil, errors.New("api source config.url / config.items_path 必填")
	}
	vals, _ := src.Config["headers"].(map[string]interface{})
	headers := make(map[string]string, len(vals))
	for k, v := range vals {
		headers[k] = asString(v) // ⚠️ 密文引用禁止进 config：凭据走 credential_ref（NFR-IC-06）
	}

	var payload any
	frag, err := c.http.fetchJSON(ctx, rawURL, headers, &payload)
	if err != nil {
		return nil, err
	}
	list, ok := walkPath(payload, itemsPath)
	if !ok {
		return nil, errors.New("items_path 未命中: " + itemsPath)
	}
	arr, ok := list.([]any)
	if !ok {
		return nil, errors.New("items_path 目标不是数组: " + itemsPath)
	}

	out := make([]*biz.RawCollectItem, 0, len(arr))
	for i, node := range arr {
		title := pickString(node, src.Config, "title_path")
		link := pickString(node, src.Config, "url_path")
		if link == "" || title == "" {
			continue
		}
		item := &biz.RawCollectItem{
			SourceURL: link,
			Title:     title,
			Text:      pickString(node, src.Config, "text_path"),
			Author:    pickString(node, src.Config, "author_path"),
			Fragment:  frag, // 同 hotlist：批次证据共享片段，链哈希按 intel_id 独立
		}
		if ts, ok := asTime(pickValue(node, src.Config, "published_path")); ok {
			item.PublishedAt = &ts
		}
		if v, ok := asInt(pickValue(node, src.Config, "views_path")); ok {
			item.Views = v
		}
		if v, ok := asInt(pickValue(node, src.Config, "likes_path")); ok {
			item.Likes = v
		}
		if v, ok := asInt(pickValue(node, src.Config, "comments_path")); ok {
			item.Comments = v
		}
		if flag, _ := src.Config["rank_from_index"].(bool); flag {
			item.Rank, item.RankTotal = int32(i+1), int32(len(arr))
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, errors.New("api 解析零条目（映射配置或上游结构变更）")
	}
	return out, nil
}

// pickValue 按 config[pathKey] 指定的点号路径取值；pathKey 未配置返回 nil。
func pickValue(node any, cfg map[string]interface{}, pathKey string) any {
	p := asString(cfg[pathKey])
	if p == "" {
		return nil
	}
	v, _ := walkPath(node, p)
	return v
}

func pickString(node any, cfg map[string]interface{}, pathKey string) string {
	return asString(pickValue(node, cfg, pathKey))
}
