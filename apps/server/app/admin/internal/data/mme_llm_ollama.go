package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
)

// OllamaLLM Ollama 本地 HTTP 客户端（/api/generate + /api/tags 探活）。
// 🔴 不可达/超时 → Available=false，Generate 返回 ErrLLMUnavailable 包裹；
// 绝不编造文案（copy 通路会改走模板降级并标注 copy_degraded）。
type OllamaLLM struct {
	cfg bizmme.Config
	hc  *http.Client
	log *log.Helper
}

// NewOllamaLLM 构造。
func NewOllamaLLM(cfg bizmme.Config, logger log.Logger) bizmme.LLMClient {
	return &OllamaLLM{
		cfg: cfg,
		hc:  &http.Client{Timeout: 5 * time.Second}, // 探活短超时；Generate 用 cfg.LLMTimeout ctx
		log: log.NewHelper(log.With(logger, "module", "mme/ollama")),
	}
}

// Available /api/tags 探活（2s）。
func (o *OllamaLLM) Available(ctx context.Context) bool {
	if o.cfg.OllamaURL == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(o.cfg.OllamaURL, "/")+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := o.hc.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// ollamaGenerateResp /api/generate 非流式响应。
type ollamaGenerateResp struct {
	Response string `json:"response"`
	Error    string `json:"error"`
}

// Generate 生成文案（超时走 cfg.LLMTimeout）。
func (o *OllamaLLM) Generate(ctx context.Context, prompt string) (string, error) {
	if !o.Available(ctx) {
		return "", fmt.Errorf("%w: ollama unreachable at %s", bizmme.ErrLLMUnavailable, o.cfg.OllamaURL)
	}
	ctx, cancel := context.WithTimeout(ctx, o.cfg.LLMTimeout)
	defer cancel()
	body, err := json.Marshal(map[string]any{
		"model":  o.cfg.OllamaModel,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.7,
			"num_predict": 600,
		},
	})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(o.cfg.OllamaURL, "/") + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Timeout: o.cfg.LLMTimeout + 10*time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: ollama generate: %v", bizmme.ErrLLMUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w: read ollama response: %v", bizmme.ErrLLMUnavailable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: ollama status %d: %s", bizmme.ErrLLMUnavailable, resp.StatusCode, tail(string(raw), 200))
	}
	var gr ollamaGenerateResp
	if err = json.Unmarshal(raw, &gr); err != nil {
		return "", fmt.Errorf("%w: ollama response unparseable", bizmme.ErrLLMUnavailable)
	}
	if gr.Error != "" {
		return "", fmt.Errorf("%w: ollama model error: %s", bizmme.ErrLLMUnavailable, gr.Error)
	}
	if strings.TrimSpace(gr.Response) == "" {
		return "", fmt.Errorf("%w: ollama empty response", bizmme.ErrLLMUnavailable)
	}
	return gr.Response, nil
}
