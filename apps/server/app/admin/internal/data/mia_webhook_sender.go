package data

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/conf"

	bizmia "framework/app/admin/internal/biz/mia"
)

// MiaWebhookSender Webhook / 企业微信推送发送器（FR-MIA-12）。
type MiaWebhookSender struct {
	url    string
	client *http.Client
	log    *log.Helper
}

// NewMiaWebhookSender 构造。webhook 未配置时返回 nil（nil 接口注入 biz 层 Notifier 直接 no-op）。
func NewMiaWebhookSender(cfg *conf.Bootstrap, logger log.Logger) bizmia.WebhookSender {
	mcfg := bizmia.ResolveConfig(cfg)
	if mcfg.NotifyWebhookURL == "" {
		log.NewHelper(log.With(logger, "module", "mia/webhook/adapter")).Warn("notify webhook url not configured; push disabled")
		return nil
	}
	return &MiaWebhookSender{
		url:    mcfg.NotifyWebhookURL,
		client: &http.Client{Timeout: mcfg.NotifyTimeout},
		log:    log.NewHelper(log.With(logger, "module", "mia/webhook/adapter")),
	}
}

// Send POST JSON body；非 2xx 视为失败（调用方记录失败、不阻塞主流程）。
func (s *MiaWebhookSender) Send(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook send failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook send status %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}
