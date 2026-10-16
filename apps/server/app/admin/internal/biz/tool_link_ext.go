package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "framework/api/gen/go/admin/v1"
)

const (
	// toolLinkIconStorageBucket favicon 抓取落地目录（相对 storage.root）。
	toolLinkIconStorageBucket = "tool-link-icons"

	// toolLinkHealthTimeout 健康探测超时。
	toolLinkHealthTimeout = 5 * time.Second
	// toolLinkFetchTimeout 页面/图标抓取超时。
	toolLinkFetchTimeout = 8 * time.Second
	// toolLinkMaxPageBytes 页面 HTML 最大读取字节（1MB）。
	toolLinkMaxPageBytes = 1 << 20
	// toolLinkMaxIconBytes 图标文件最大字节（512KB）。
	toolLinkMaxIconBytes = 512 * 1024

	// toolLinkSsoTicketTTL SSO 票据有效期。
	toolLinkSsoTicketTTL = 5 * time.Minute
	// toolLinkSsoTicketParam 拼接到目标 URL 的票据参数名。
	toolLinkSsoTicketParam = "sso_ticket"
)

// Reorder 批量重排（拖拽排序）。
func (uc *ToolLinkUsecase) Reorder(ctx context.Context, req *adminV1.ReorderToolLinkRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid request")
	}
	return uc.repo.Reorder(ctx, req.GetIds())
}

// Click 记录一次点击：全站计数 +1 与当前用户标记（常用工具数据源）。
func (uc *ToolLinkUsecase) Click(ctx context.Context, userID uint32, req *adminV1.ClickToolLinkRequest) error {
	if req == nil || req.GetId() == 0 {
		return adminV1.ErrorBadRequest("invalid request")
	}

	if err := uc.repo.IncrementClick(ctx, req.GetId()); err != nil {
		return err
	}

	// 用户标记失败仅记日志，不影响主流程。
	if userID != 0 && uc.markRepo != nil {
		if err := uc.markRepo.RecordClick(ctx, userID, req.GetId()); err != nil {
			uc.log.Warnf("record user click failed (user_id=%d, tool_link_id=%d): %v", userID, req.GetId(), err)
		}
	}
	return nil
}

// Favorite 收藏/取消收藏（当前用户）。
func (uc *ToolLinkUsecase) Favorite(ctx context.Context, userID uint32, req *adminV1.FavoriteToolLinkRequest) error {
	if req == nil || req.GetId() == 0 || userID == 0 {
		return adminV1.ErrorBadRequest("invalid request")
	}
	if uc.markRepo == nil {
		return adminV1.ErrorInternalServerError("user mark repo unavailable")
	}

	// 确认链接存在，避免对脏数据打标。
	if _, err := uc.repo.Get(ctx, &adminV1.GetToolLinkRequest{Id: req.GetId()}); err != nil {
		return err
	}

	if err := uc.markRepo.SetFavorite(ctx, userID, req.GetId(), req.GetFavorite()); err != nil {
		uc.log.Errorf("set favorite failed (user_id=%d, tool_link_id=%d): %s", userID, req.GetId(), err.Error())
		return adminV1.ErrorInternalServerError("set favorite failed")
	}
	return nil
}

// CheckHealth 探测目标 URL 可达性并写回结果；返回更新后的记录。
func (uc *ToolLinkUsecase) CheckHealth(ctx context.Context, req *adminV1.CheckToolLinkHealthRequest) (*adminV1.ToolLink, error) {
	if req == nil || req.GetId() == 0 {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	link, err := uc.repo.Get(ctx, &adminV1.GetToolLinkRequest{Id: req.GetId()})
	if err != nil {
		return nil, err
	}

	status := probeToolLinkHealth(link.GetUrl())
	checkedAt := time.Now()

	if err = uc.repo.UpdateHealth(ctx, req.GetId(), status, checkedAt); err != nil {
		return nil, err
	}

	link.HealthStatus = &status
	link.LastCheckAt = timestamppb.New(checkedAt)
	return link, nil
}

// probeToolLinkHealth HEAD 探测（不支持 HEAD 的站点回退 GET）；HTTP 状态码 < 400 视为 UP。
// 非 http/https 协议（如 file:// 本地共享盘）不具备服务端可探测性，置 UNKNOWN 而非误报 DOWN。
func probeToolLinkHealth(rawURL string) string {
	if rawURL == "" {
		return "UNKNOWN"
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "UNKNOWN"
	}

	client := &http.Client{Timeout: toolLinkHealthTimeout}

	req, err := http.NewRequest(http.MethodHead, rawURL, nil)
	if err != nil {
		return "UNKNOWN"
	}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		// HEAD 方法不被支持时回退 GET
		if resp.StatusCode != http.StatusMethodNotAllowed {
			return healthStatusOf(resp.StatusCode)
		}
	}

	getReq, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "DOWN"
	}
	getResp, err := client.Do(getReq)
	if err != nil {
		return "DOWN"
	}
	_ = getResp.Body.Close()
	return healthStatusOf(getResp.StatusCode)
}

func healthStatusOf(code int) string {
	if code < http.StatusBadRequest {
		return "UP"
	}
	return "DOWN"
}

// FetchIcon 抓取目标站点 favicon 并落地本地存储，返回相对路径（写入 iconValue，iconType=IMAGE）。
// 解析顺序：页面 <link rel~=icon> → origin/favicon.ico。
func (uc *ToolLinkUsecase) FetchIcon(ctx context.Context, req *adminV1.FetchToolLinkIconRequest) (*adminV1.FetchToolLinkIconResponse, error) {
	if req == nil || req.GetUrl() == "" {
		return nil, adminV1.ErrorBadRequest("url required")
	}
	if uc.store == nil {
		return nil, adminV1.ErrorInternalServerError("local storage unavailable")
	}

	target, err := url.Parse(req.GetUrl())
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, adminV1.ErrorBadRequest("invalid url")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, adminV1.ErrorBadRequest("only http/https supported")
	}

	client := &http.Client{Timeout: toolLinkFetchTimeout}

	// 1. 抓页面解析 icon 地址；失败回退 origin/favicon.ico
	iconURL := fetchIconURLFromPage(client, target)
	if iconURL == "" {
		iconURL = fmt.Sprintf("%s://%s/favicon.ico", target.Scheme, target.Host)
	}

	// 2. 下载图标
	data, contentType, err := downloadLimited(client, iconURL, toolLinkMaxIconBytes)
	if err != nil {
		uc.log.Warnf("download favicon failed [%s]: %v", iconURL, err)
		return nil, adminV1.ErrorNotFound("fetch favicon failed")
	}

	// 3. 落地本地存储
	ext := iconFileExt(iconURL, contentType)
	objectName := uuid.NewString() + ext
	relPath, err := uc.store.Upload(ctx, toolLinkIconStorageBucket, objectName, data)
	if err != nil {
		uc.log.Errorf("store favicon failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("store favicon failed")
	}

	return &adminV1.FetchToolLinkIconResponse{IconValue: &relPath}, nil
}

// fetchIconURLFromPage 抓取页面 HTML 并解析 <link rel="icon|shortcut icon|apple-touch-icon"> 的 href。
// 任意一步失败返回空串（调用方回退 /favicon.ico）。
func fetchIconURLFromPage(client *http.Client, pageURL *url.URL) string {
	data, _, err := downloadLimited(client, pageURL.String(), toolLinkMaxPageBytes)
	if err != nil {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return ""
	}

	var iconHref string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if iconHref != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href string
			for _, attr := range n.Attr {
				switch attr.Key {
				case "rel":
					rel = strings.ToLower(attr.Val)
				case "href":
					href = attr.Val
				}
			}
			if href != "" && (strings.Contains(rel, "icon")) {
				iconHref = href
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if iconHref == "" {
		return ""
	}

	// 相对地址转绝对地址
	ref, err := url.Parse(iconHref)
	if err != nil {
		return ""
	}
	return pageURL.ResolveReference(ref).String()
}

// downloadLimited GET 并限制读取字节数；返回 body 与 Content-Type。
func downloadLimited(client *http.Client, rawURL string, maxBytes int64) ([]byte, string, error) {
	resp, err := client.Get(rawURL) //nolint:gosec // 目标 URL 来自管理员配置的工具链接
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// iconFileExt 依 URL 路径后缀或 Content-Type 推断图标扩展名，缺省 .ico。
func iconFileExt(iconURL, contentType string) string {
	if u, err := url.Parse(iconURL); err == nil {
		ext := strings.ToLower(path.Ext(u.Path))
		switch ext {
		case ".ico", ".png", ".svg", ".jpg", ".jpeg", ".gif", ".webp":
			return ext
		}
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "svg"):
		return ".svg"
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "gif"):
		return ".gif"
	case strings.Contains(ct, "webp"):
		return ".webp"
	}
	return ".ico"
}

// toolLinkExportFile 导出文件结构（JSON/YAML 同构）。
type toolLinkExportFile struct {
	Version int                    `json:"version" yaml:"version"`
	Items   []*toolLinkExportItem  `json:"items" yaml:"items"`
}

type toolLinkExportItem struct {
	Name            string            `json:"name" yaml:"name"`
	URL             string            `json:"url" yaml:"url"`
	IconType        string            `json:"iconType,omitempty" yaml:"iconType,omitempty"`
	IconValue       string            `json:"iconValue,omitempty" yaml:"iconValue,omitempty"`
	Category        string            `json:"category,omitempty" yaml:"category,omitempty"`
	Description     string            `json:"description,omitempty" yaml:"description,omitempty"`
	Sort            int32             `json:"sort,omitempty" yaml:"sort,omitempty"`
	External        *bool             `json:"external,omitempty" yaml:"external,omitempty"`
	Status          string            `json:"status,omitempty" yaml:"status,omitempty"`
	LinkType        string            `json:"linkType,omitempty" yaml:"linkType,omitempty"`
	VisibleRoles    []string          `json:"visibleRoles,omitempty" yaml:"visibleRoles,omitempty"`
	NameI18n        map[string]string `json:"nameI18n,omitempty" yaml:"nameI18n,omitempty"`
	DescriptionI18n map[string]string `json:"descriptionI18n,omitempty" yaml:"descriptionI18n,omitempty"`
}

// Export 导出全部工具链接为 JSON/YAML。
func (uc *ToolLinkUsecase) Export(ctx context.Context, req *adminV1.ExportToolLinksRequest) (*adminV1.ExportToolLinksResponse, error) {
	format := strings.ToLower(req.GetFormat())
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "yaml" {
		return nil, adminV1.ErrorBadRequest("unsupported format (json/yaml)")
	}

	items, err := uc.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	file := &toolLinkExportFile{Version: 1, Items: make([]*toolLinkExportItem, 0, len(items))}
	for _, item := range items {
		exportItem := &toolLinkExportItem{
			Name:            item.GetName(),
			URL:             item.GetUrl(),
			Category:        item.GetCategory(),
			Description:     item.GetDescription(),
			Sort:            item.GetSort(),
			External:        item.External,
			Status:          item.GetStatus(),
			LinkType:        item.GetLinkType(),
			VisibleRoles:    item.GetVisibleRoles(),
			NameI18n:        item.GetNameI18N(),
			DescriptionI18n: item.GetDescriptionI18N(),
		}
		if item.GetIconType() != adminV1.ToolLinkIconType_TOOL_LINK_ICON_TYPE_UNSPECIFIED {
			exportItem.IconType = item.GetIconType().String()
		}
		exportItem.IconValue = item.GetIconValue()
		file.Items = append(file.Items, exportItem)
	}

	var content []byte
	switch format {
	case "yaml":
		content, err = yaml.Marshal(file)
	default:
		content, err = json.MarshalIndent(file, "", "  ")
	}
	if err != nil {
		uc.log.Errorf("marshal export content failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("export failed")
	}

	return &adminV1.ExportToolLinksResponse{
		Content: ptrString(string(content)),
		Format:  ptrString(format),
		Count:   ptrUint32(uint32(len(file.Items))),
	}, nil
}

// Import 导入工具链接（JSON/YAML 导出格式或浏览器书签 HTML）；URL 已存在的跳过。
func (uc *ToolLinkUsecase) Import(ctx context.Context, req *adminV1.ImportToolLinksRequest) (*adminV1.ImportToolLinksResponse, error) {
	if req == nil || req.GetContent() == "" {
		return nil, adminV1.ErrorBadRequest("content required")
	}

	format := strings.ToLower(req.GetFormat())
	if format == "" {
		format = "json"
	}

	var items []*toolLinkExportItem
	var parseErr error
	switch format {
	case "json", "yaml":
		items, parseErr = parseToolLinkExportFile(req.GetContent(), format)
	case "html":
		items, parseErr = parseBookmarkHTML(req.GetContent())
	default:
		return nil, adminV1.ErrorBadRequest("unsupported format (json/yaml/html)")
	}
	if parseErr != nil {
		uc.log.Warnf("parse import content failed: %v", parseErr)
		return nil, adminV1.ErrorBadRequest("parse content failed")
	}
	if len(items) == 0 {
		return nil, adminV1.ErrorBadRequest("no valid item found")
	}

	// URL 去重（库内已存在跳过）
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.URL != "" {
			urls = append(urls, item.URL)
		}
	}
	existing, err := uc.repo.FilterExistingURLs(ctx, urls)
	if err != nil {
		return nil, err
	}

	resp := &adminV1.ImportToolLinksResponse{}
	for _, item := range items {
		if item.Name == "" || item.URL == "" {
			resp.Failed = ptrUint32(resp.GetFailed() + 1)
			continue
		}
		if existing[item.URL] {
			resp.Skipped = ptrUint32(resp.GetSkipped() + 1)
			continue
		}

		createReq := &adminV1.CreateToolLinkRequest{Data: &adminV1.ToolLink{
			Name:            &item.Name,
			Url:             &item.URL,
			Category:        optionalString(item.Category),
			Description:     optionalString(item.Description),
			External:        item.External,
			VisibleRoles:    item.VisibleRoles,
			NameI18N:        item.NameI18n,
			DescriptionI18N: item.DescriptionI18n,
		}}
		if item.IconType != "" {
			if iconType, ok := adminV1.ToolLinkIconType_value[item.IconType]; ok {
				createReq.Data.IconType = adminV1.ToolLinkIconType(iconType).Enum()
			}
		}
		createReq.Data.IconValue = optionalString(item.IconValue)
		if item.Status != "" {
			createReq.Data.Status = &item.Status
		}
		if item.LinkType != "" {
			createReq.Data.LinkType = &item.LinkType
		}
		if item.Sort != 0 {
			createReq.Data.Sort = &item.Sort
		}

		if err = uc.repo.Create(ctx, createReq); err != nil {
			resp.Failed = ptrUint32(resp.GetFailed() + 1)
			continue
		}
		resp.Created = ptrUint32(resp.GetCreated() + 1)
	}

	return resp, nil
}

// parseToolLinkExportFile 解析 JSON/YAML 导出文件。
func parseToolLinkExportFile(content, format string) ([]*toolLinkExportItem, error) {
	var file toolLinkExportFile
	var err error
	if format == "yaml" {
		err = yaml.Unmarshal([]byte(content), &file)
	} else {
		err = json.Unmarshal([]byte(content), &file)
	}
	if err != nil {
		return nil, err
	}
	return file.Items, nil
}

// parseBookmarkHTML 解析浏览器书签导出文件（Netscape Bookmark File Format）。
// <H3> 目录名作为分类，<DT><A HREF="...">名称</A> 为条目。
func parseBookmarkHTML(content string) ([]*toolLinkExportItem, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, err
	}

	items := make([]*toolLinkExportItem, 0)
	category := ""

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h3":
				category = textOfNode(n)
			case "a":
				var href string
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href = attr.Val
						break
					}
				}
				name := textOfNode(n)
				if href != "" && name != "" && (strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")) {
					items = append(items, &toolLinkExportItem{
						Name:     name,
						URL:      href,
						Category: category,
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return items, nil
}

// textOfNode 提取节点全部文本内容（去空白）。
func textOfNode(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// IssueSsoTicket 签发 SSO 一次性跳转票据，返回拼接票据后的最终 URL。
// 与 List 同一可见性口径：非管理员对角色不可见的链接拒签，防止越权拿到跳转地址。
func (uc *ToolLinkUsecase) IssueSsoTicket(ctx context.Context, userID uint32, roles []string, isAdmin bool, req *adminV1.IssueToolLinkSsoTicketRequest) (*adminV1.IssueToolLinkSsoTicketResponse, error) {
	if req == nil || req.GetId() == 0 || userID == 0 {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	if uc.ssoTicketRepo == nil {
		return nil, adminV1.ErrorInternalServerError("sso ticket repo unavailable")
	}

	link, err := uc.repo.Get(ctx, &adminV1.GetToolLinkRequest{Id: req.GetId()})
	if err != nil {
		return nil, err
	}
	if !isAdmin && !toolLinkVisibleTo(link.GetVisibleRoles(), roles) {
		return nil, adminV1.ErrorForbidden("tool link not visible to current roles")
	}
	if link.GetLinkType() != "SSO" {
		return nil, adminV1.ErrorBadRequest("not an SSO tool link")
	}

	expireAt := time.Now().Add(toolLinkSsoTicketTTL)
	ticket := &ToolLinkSsoTicket{
		Ticket:     uuid.NewString(),
		UserID:     userID,
		ToolLinkID: req.GetId(),
		ExpireAt:   expireAt,
	}
	if err = uc.ssoTicketRepo.Create(ctx, ticket); err != nil {
		return nil, err
	}

	finalURL := appendQueryParam(link.GetUrl(), toolLinkSsoTicketParam, ticket.Ticket)

	return &adminV1.IssueToolLinkSsoTicketResponse{
		Ticket:   &ticket.Ticket,
		Url:      &finalURL,
		ExpireAt: timestamppb.New(expireAt),
	}, nil
}

// VerifySsoTicket 校验票据：未过期且未使用则核销并返回有效。
func (uc *ToolLinkUsecase) VerifySsoTicket(ctx context.Context, req *adminV1.VerifyToolLinkSsoTicketRequest) (*adminV1.VerifyToolLinkSsoTicketResponse, error) {
	if req == nil || req.GetTicket() == "" {
		return nil, adminV1.ErrorBadRequest("ticket required")
	}
	if uc.ssoTicketRepo == nil {
		return nil, adminV1.ErrorInternalServerError("sso ticket repo unavailable")
	}

	invalid := &adminV1.VerifyToolLinkSsoTicketResponse{Valid: ptrBool(false)}

	ticket, err := uc.ssoTicketRepo.GetByTicket(ctx, req.GetTicket())
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return invalid, nil
	}
	if ticket.UsedAt != nil || time.Now().After(ticket.ExpireAt) {
		return invalid, nil
	}

	// 核销（一次性语义）；并发核验时仅首个请求成功。
	if err = uc.ssoTicketRepo.MarkUsed(ctx, req.GetTicket(), time.Now()); err != nil {
		return invalid, nil
	}

	return &adminV1.VerifyToolLinkSsoTicketResponse{
		Valid:      ptrBool(true),
		ToolLinkId: &ticket.ToolLinkID,
		UserId:     &ticket.UserID,
	}, nil
}

// appendQueryParam 向 URL 追加查询参数（已存在同键参数时覆盖）。
func appendQueryParam(rawURL, key, value string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func ptrUint32(v uint32) *uint32 { return &v }

// ptrBool 复用 biz 包内已有声明（license.go）。

// optionalString 空串返回 nil（避免落库空字符串覆盖）。
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// 确保 log 包被引用（ToolLinkUsecase.log 的使用跨文件共享）。
var _ = log.NewHelper
