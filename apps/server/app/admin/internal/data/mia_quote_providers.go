package data

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/conf"

	bizmia "framework/app/admin/internal/biz/mia"
)

// MiaCCXTQuoteProvider ccxt sidecar 行情适配器（FR-MIA-07）。
// 🔴 只读：sidecar 仅暴露 OHLCV 读取接口，本适配器没有任何下单/撤单方法（NFR-MIA-02）。
// sidecar 协议：GET {url}/ohlcv?symbol=BTC/USDT → {"symbol","bars"[[ts_ms,o,h,l,c,v],...]}。
type MiaCCXTQuoteProvider struct {
	url    string
	client *http.Client
	log    *log.Helper
}

// NewMiaCCXTQuoteProvider 构造。
func NewMiaCCXTQuoteProvider(cfg *conf.Bootstrap, logger log.Logger) *MiaCCXTQuoteProvider {
	mcfg := bizmia.ResolveConfig(cfg)
	p := &MiaCCXTQuoteProvider{
		url:    trimRightSlash(mcfg.CCXTSidecarURL),
		client: &http.Client{Timeout: 10 * time.Second},
		log:    log.NewHelper(log.With(logger, "module", "mia/ccxt/adapter")),
	}
	if p.url == "" {
		p.log.Warn("ccxt sidecar url not configured; quote fetch will fail over to cache")
	}
	return p
}

func (p *MiaCCXTQuoteProvider) Name() string { return "ccxt" }

type ccxtOHLCVResponse struct {
	Symbol string        `json:"symbol"`
	Bars   [][]float64   `json:"bars"`
}

func (p *MiaCCXTQuoteProvider) FetchOHLCV(ctx context.Context, symbol string) (*bizmia.QuoteSnapshot, error) {
	if p.url == "" {
		return nil, bizmia.ErrQuoteReadFail
	}
	endpoint := p.url + "/ohlcv?symbol=" + url.QueryEscape(symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ccxt ohlcv request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ccxt ohlcv status %d", resp.StatusCode)
	}

	var out ccxtOHLCVResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("ccxt ohlcv decode failed: %w", err)
	}
	if len(out.Bars) == 0 {
		return nil, fmt.Errorf("ccxt ohlcv empty for %s", symbol)
	}

	bars := make([]bizmia.OhlcvBar, 0, len(out.Bars))
	for _, row := range out.Bars {
		if len(row) < 6 {
			continue
		}
		bars = append(bars, bizmia.OhlcvBar{
			T:      time.UnixMilli(int64(row[0])).UTC(),
			Open:   row[1],
			High:   row[2],
			Low:    row[3],
			Close:  row[4],
			Volume: row[5],
		})
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("ccxt ohlcv malformed for %s", symbol)
	}
	return &bizmia.QuoteSnapshot{
		Symbol:    symbol,
		Provider:  p.Name(),
		Bars:      bars,
		FetchedAt: time.Now().UTC(),
	}, nil
}

// MiaBlockedOpenBBProvider OpenBB 阻断适配器。
// 🔴 OpenBB 许可证未完成合规复核前不得接入（README 红线 / 详细设计 §2.5）：
// 所有调用一律返回 ErrOpenBBLicenseUnverified，上游降级到其他提供方。
type MiaBlockedOpenBBProvider struct {
	log *log.Helper
}

// NewMiaBlockedOpenBBProvider 构造。
func NewMiaBlockedOpenBBProvider(logger log.Logger) *MiaBlockedOpenBBProvider {
	return &MiaBlockedOpenBBProvider{
		log: log.NewHelper(log.With(logger, "module", "mia/openbb/adapter")),
	}
}

func (p *MiaBlockedOpenBBProvider) Name() string { return "openbb" }

func (p *MiaBlockedOpenBBProvider) FetchOHLCV(ctx context.Context, symbol string) (*bizmia.QuoteSnapshot, error) {
	p.log.Warnf("openbb quote fetch blocked (license unverified): %s", symbol)
	return nil, bizmia.ErrOpenBBLicenseUnverified
}

func trimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
