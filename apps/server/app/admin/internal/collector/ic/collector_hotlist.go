package iccollector

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// hotlist 采集器（newsnow 自部署实例；公共实例禁止——02 §5 部署纪律）
// 信源 config：{"source": "weibo", "max_items": 50, "path_template": "/api/s/{source}"}
// TODO(deploy-verify)：newsnow 版本间 API 路径存在差异（/api/s vs /api/news?id=），
// 部署后按实例文档收敛 path_template 默认值（保持配置可覆盖，勿改代码）。
// =============================================================================

// HotlistCollector newsnow 热榜聚合。
type HotlistCollector struct {
	baseURL string
	http    *httpFetcher
	log     *log.Helper
}

func NewHotlistCollector(bc *conf.Bootstrap, ic *biz.IcConfig, logger log.Logger) *HotlistCollector {
	ns := bc.GetIc().GetNewsnow()
	timeout := 10 * time.Second
	if ns.GetTimeout() != nil {
		timeout = ns.GetTimeout().AsDuration()
	}
	return &HotlistCollector{
		baseURL: strings.TrimRight(ns.GetBaseUrl(), "/"),
		http:    newHTTPFetcher(timeout, ic.RobotsUserAgent, ic.EvidenceRawMaxBytes),
		log:     log.NewHelper(log.With(logger, "module", "ic/collector/hotlist")),
	}
}

func (c *HotlistCollector) Method() string { return string(contract.CollectHotlist) }

// newsnowItem 宽容解码（字段多版本并存的 union）。
type newsnowItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	MobileURL string `json:"mobile_url"`
	PubDate   any    `json:"pub_date"`
	Extra     *struct {
		Info string `json:"info"`
		Date any    `json:"date"`
	} `json:"extra"`
}

type newsnowResponse struct {
	Status string         `json:"status"`
	Items  []newsnowItem  `json:"items"`
}

func (c *HotlistCollector) Collect(ctx context.Context, src *biz.IntelSourceRow) ([]*biz.RawCollectItem, error) {
	sourceID := asString(src.Config["source"])
	if sourceID == "" {
		return nil, errors.New("hotlist source config.source 必填（newsnow 平板标识）")
	}
	if c.baseURL == "" {
		return nil, errors.New("ic.newsnow.base_url 未配置")
	}
	pathTemplate := asString(src.Config["path_template"])
	if pathTemplate == "" {
		pathTemplate = "/api/s/{source}"
	}
	u := c.baseURL + strings.ReplaceAll(pathTemplate, "{source}", sourceID)

	var resp newsnowResponse
	frag, err := c.http.fetchJSON(ctx, u, nil, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Status != "" && resp.Status != "success" && resp.Status != "cache" {
		return nil, errors.New("newsnow status=" + resp.Status)
	}

	maxItems := 50
	if v, ok := asInt(src.Config["max_items"]); ok && v > 0 {
		maxItems = int(v)
	}
	collected := make([]*biz.RawCollectItem, 0, min(len(resp.Items), maxItems))
	rankTotal := int32(len(resp.Items))
	for i, it := range resp.Items {
		if i >= maxItems {
			break
		}
		rawURL := it.URL
		if rawURL == "" {
			rawURL = it.MobileURL
		}
		if rawURL == "" || it.Title == "" {
			continue // 热榜条目无链接/标题 = 解析失败信号，跳过留痕（降准确度不污染数据）
		}
		item := &biz.RawCollectItem{
			SourceURL: rawURL,
			Title:     it.Title,
			Rank:      int32(i + 1),
			RankTotal: rankTotal,
			// 证据：榜单响应体即名次快照，全体条目共享同一片段（B-17 链哈希含 intel_id，
			// 各自独立成链；字节重复换取"单条目取证包可独立导出"的纪律）。
			Fragment: frag,
		}
		if it.Extra != nil {
			item.Text = it.Extra.Info
			if ts, ok := asTime(it.Extra.Date); ok {
				item.PublishedAt = &ts
			}
		}
		if ts, ok := asTime(it.PubDate); ok {
			item.PublishedAt = &ts
		}
		collected = append(collected, item)
	}
	if len(collected) == 0 {
		return nil, errors.New("newsnow 解析零条目（结构变更信号，请核对 path_template）")
	}
	return collected, nil
}
