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

	"framework/app/admin/internal/conf"

	bizmia "framework/app/admin/internal/biz/mia"
	"framework/pkg/mediacontract"
)

// MiaFinBERTEngine FinBERT 自托管推理服务适配器（详细设计 §2.2：英文新闻走 FinBERT）。
// 内部推理服务协议：POST {url}/classify {"title","text","lang"} → {"label","score"}。
// 🔴 服务不可达返回错误，由 biz 编排降级到 LLM；绝不在此猜测情感。
type MiaFinBERTEngine struct {
	url     string
	timeout time.Duration
	client  *http.Client
	log     *log.Helper
}

// NewMiaFinBERTEngine 构造（URL 未配置时 Available()=false，编排层直接跳过）。
func NewMiaFinBERTEngine(cfg *conf.Bootstrap, logger log.Logger) *MiaFinBERTEngine {
	mcfg := bizmia.ResolveConfig(cfg)
	e := &MiaFinBERTEngine{
		url:     strings.TrimRight(mcfg.FinBERTURL, "/"),
		timeout: mcfg.FinBERTTimeout,
		client:  &http.Client{Timeout: mcfg.FinBERTTimeout},
		log:     log.NewHelper(log.With(logger, "module", "mia/finbert/adapter")),
	}
	if e.url == "" {
		e.log.Warn("finbert url not configured; engine disabled (degrade to llm)")
	}
	return e
}

func (e *MiaFinBERTEngine) Available() bool { return e.url != "" }
func (e *MiaFinBERTEngine) Name() string    { return "finbert" }

type finbertClassifyRequest struct {
	Title string `json:"title"`
	Text  string `json:"text"`
	Lang  string `json:"lang"`
}

type finbertClassifyResponse struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func (e *MiaFinBERTEngine) Classify(ctx context.Context, in bizmia.SentimentInput) (bizmia.SentimentResult, error) {
	body, err := json.Marshal(finbertClassifyRequest{
		Title: in.Title,
		Text:  truncateText(in.Text, 2000),
		Lang:  in.Lang,
	})
	if err != nil {
		return bizmia.SentimentResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url+"/classify", bytes.NewReader(body))
	if err != nil {
		return bizmia.SentimentResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return bizmia.SentimentResult{}, fmt.Errorf("finbert classify request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return bizmia.SentimentResult{}, fmt.Errorf("finbert classify status %d", resp.StatusCode)
	}

	var out finbertClassifyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return bizmia.SentimentResult{}, fmt.Errorf("finbert classify decode failed: %w", err)
	}
	label := mediacontract.SentimentLabel(out.Label)
	if !label.Valid() || label == mediacontract.SentimentUnknown {
		return bizmia.SentimentResult{}, fmt.Errorf("finbert classify invalid label %q", out.Label)
	}
	return bizmia.SentimentResult{
		Label: label,
		Score: clampFloat(out.Score, -1, 1),
	}, nil
}

// truncateText 截断超长正文（FinBERT 输入长度有限）。
func truncateText(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// NewMiaSentimentOrchestrator 聚合构造：三个引擎在 data 层实例化后交给 biz 编排器。
// wire 无法向三个同型 SentimentEngine 参数注入不同实现，故在此处组装。
func NewMiaSentimentOrchestrator(cfg *conf.Bootstrap, redis *RedisClient, logger log.Logger) *bizmia.SentimentOrchestrator {
	finbert := NewMiaFinBERTEngine(cfg, logger)
	cloud := NewMiaLLMCloudEngine(cfg, redis, logger)
	ollama := NewMiaOllamaEngine(cfg, logger)
	return bizmia.NewSentimentOrchestrator(finbert, cloud, ollama, logger)
}
