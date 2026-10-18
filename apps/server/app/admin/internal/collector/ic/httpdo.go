package iccollector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 采集器共用 HTTP 执行器：统一 UA / 超时 / 字节上限 / 响应取证（B-21 内存上限）。
// =============================================================================

// httpFetcher 受限 HTTP 抓取（readLimit = conf evidence.raw_max_bytes 同口径）。
type httpFetcher struct {
	client    *http.Client
	userAgent string
	readLimit int64
}

func newHTTPFetcher(timeout time.Duration, userAgent string, readLimit int64) *httpFetcher {
	if userAgent == "" {
		userAgent = "FrameWorkBot/1.0"
	}
	if readLimit <= 0 {
		readLimit = 4 << 20
	}
	return &httpFetcher{
		client:    &http.Client{Timeout: timeout},
		userAgent: userAgent,
		readLimit: readLimit,
	}
}

// fetch GET 并组装 RawFragment（状态码/响应头/终态 URL/受限字节；不做语义解析）。
func (f *httpFetcher) fetch(ctx context.Context, rawURL string, headers map[string]string) (*contract.RawFragment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "*/*")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.readLimit+1)) // +1 探测截断（生成方负责截断语义）
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > f.readLimit { // 限长之上丢弃探测位（截断标记由 evidence store 口径统一）
		body = body[:f.readLimit]
	}

	hdr := make(map[string]string, len(resp.Header))
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			hdr[k] = vs[0]
		}
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return &contract.RawFragment{
		Content:    body,
		HTTPStatus: resp.StatusCode,
		Headers:    hdr,
		RequestURL: rawURL,
		FinalURL:   finalURL,
	}, nil
}

// fetchJSON GET + JSON 解码（共用错误语义：非 2xx 返回状态错误）。
func (f *httpFetcher) fetchJSON(ctx context.Context, rawURL string, headers map[string]string, target any) (*contract.RawFragment, error) {
	frag, err := f.fetch(ctx, rawURL, headers)
	if err != nil {
		return nil, err
	}
	if frag.HTTPStatus < 200 || frag.HTTPStatus >= 300 {
		return frag, fmt.Errorf("http status %d", frag.HTTPStatus)
	}
	return frag, decodeJSON(frag.Content, target)
}
