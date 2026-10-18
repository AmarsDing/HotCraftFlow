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

// MiaLLMEngine OpenAI 兼容 LLM 情感分析适配器（中文与宏观议题路径，详细设计 §2.2）。
// 云实例带日调用上限（🔴 NFR：超限熔断，返回 ErrLLMCapExceeded 由编排降级 Ollama）；
// Ollama 本地实例无调用上限。🔴 全部失败时编排层 label=unknown，本适配器绝不猜测。
type MiaLLMEngine struct {
	name      string // llm-cloud | llm-ollama
	baseURL   string
	apiKey    string
	model     string
	dailyCap  int // 0 = 不限（本地 Ollama）
	redis     *RedisClient
	client    *http.Client
	log       *log.Helper
}

// NewMiaLLMCloudEngine 云 LLM（带日调用帽）。
func NewMiaLLMCloudEngine(cfg *conf.Bootstrap, redis *RedisClient, logger log.Logger) *MiaLLMEngine {
	mcfg := bizmia.ResolveConfig(cfg)
	e := &MiaLLMEngine{
		name:     "llm-cloud",
		baseURL:  strings.TrimRight(mcfg.LLMCloudBaseURL, "/"),
		apiKey:   mcfg.LLMCloudAPIKey,
		model:    mcfg.LLMCloudModel,
		dailyCap: mcfg.LLMDailyCallCap,
		redis:    redis,
		client:   &http.Client{Timeout: 60 * time.Second},
		log:      log.NewHelper(log.With(logger, "module", "mia/llm-cloud/adapter")),
	}
	if e.baseURL == "" || e.apiKey == "" {
		e.log.Warn("llm cloud not configured; engine disabled (degrade to ollama)")
	}
	return e
}

// NewMiaOllamaEngine 本地 Ollama（降级兜底，无调用上限）。
func NewMiaOllamaEngine(cfg *conf.Bootstrap, logger log.Logger) *MiaLLMEngine {
	mcfg := bizmia.ResolveConfig(cfg)
	e := &MiaLLMEngine{
		name:    "llm-ollama",
		baseURL: strings.TrimRight(mcfg.LLMOllamaURL, "/"),
		model:   mcfg.LLMOllamaModel,
		client:  &http.Client{Timeout: 120 * time.Second},
		log:     log.NewHelper(log.With(logger, "module", "mia/llm-ollama/adapter")),
	}
	if e.baseURL == "" {
		e.log.Warn("ollama url not configured; engine disabled (label=unknown path)")
	}
	return e
}

func (e *MiaLLMEngine) Available() bool {
	return e.baseURL != "" && (e.name != "llm-cloud" || e.apiKey != "")
}
func (e *MiaLLMEngine) Name() string { return e.name }

type llmChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmChatRequest struct {
	Model       string          `json:"model"`
	Messages    []llmChatMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
}

type llmChatResponse struct {
	Choices []struct {
		Message llmChatMessage `json:"message"`
	} `json:"choices"`
}

// llmSentimentSystemPrompt 严格 JSON 输出约束（解析失败视为引擎失败，不猜）。
const llmSentimentSystemPrompt = `你是金融情感分析器。对给定标题与正文输出情感结论。
只输出一行 JSON：{"label":"positive|negative|neutral","score":<-1到1的浮点>}。
label 只能是 positive / negative / neutral；score 正为利好、负为利空、0 为中性。
禁止输出任何其他文本、解释或 markdown。`

// Classify 调用 OpenAI 兼容 chat/completions；🔴 云调用前检查日调用帽。
func (e *MiaLLMEngine) Classify(ctx context.Context, in bizmia.SentimentInput) (bizmia.SentimentResult, error) {
	if err := e.checkDailyCap(ctx); err != nil {
		return bizmia.SentimentResult{}, err
	}

	userPrompt := fmt.Sprintf("标题：%s\n正文：%s", in.Title, truncateText(in.Text, 3000))
	body, err := json.Marshal(llmChatRequest{
		Model: e.model,
		Messages: []llmChatMessage{
			{Role: "system", Content: llmSentimentSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return bizmia.SentimentResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return bizmia.SentimentResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return bizmia.SentimentResult{}, fmt.Errorf("llm classify request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return bizmia.SentimentResult{}, fmt.Errorf("llm classify status %d: %s", resp.StatusCode, string(snippet))
	}

	var chat llmChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&chat); err != nil {
		return bizmia.SentimentResult{}, fmt.Errorf("llm classify decode failed: %w", err)
	}
	if len(chat.Choices) == 0 {
		return bizmia.SentimentResult{}, fmt.Errorf("llm classify empty choices")
	}

	return parseLLMSentiment(chat.Choices[0].Message.Content)
}

// checkDailyCap 仅云实例有日调用帽；计数线程安全（Redis INCR 原子）。
// Redis 未配置时无法计数——按放行处理并告警（本地环境可接受，生产必配 Redis）。
func (e *MiaLLMEngine) checkDailyCap(ctx context.Context) error {
	if e.dailyCap <= 0 {
		return nil
	}
	if e.redis == nil || !e.redis.IsConfigured() {
		e.log.Warn("redis not configured; llm daily call cap not enforced")
		return nil
	}
	key := "mia:llm:calls:" + time.Now().UTC().Format("20060102")
	count, err := e.redis.IncrWithExpire(ctx, key, 25*time.Hour)
	if err != nil {
		e.log.Warnf("llm daily cap counter failed: %s; allowing call", err.Error())
		return nil // 计数故障不阻断业务（cap 是成本闸不是正确性闸）
	}
	if count > int64(e.dailyCap) {
		return bizmia.ErrLLMCapExceeded
	}
	return nil
}

// parseLLMSentiment 严格 JSON 提取；首尾允许空白，禁止包裹文本。
func parseLLMSentiment(content string) (bizmia.SentimentResult, error) {
	content = strings.TrimSpace(content)
	// 常见模型输出剥壳：```json ... ``` 围栏。
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed struct {
		Label string  `json:"label"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return bizmia.SentimentResult{}, fmt.Errorf("llm classify output not strict json: %w", err)
	}
	label := mediacontract.SentimentLabel(parsed.Label)
	if !label.Valid() || label == mediacontract.SentimentUnknown {
		return bizmia.SentimentResult{}, fmt.Errorf("llm classify invalid label %q", parsed.Label)
	}
	return bizmia.SentimentResult{
		Label: label,
		Score: clampFloat(parsed.Score, -1, 1),
	}, nil
}
