package mia

import (
	"context"
	"errors"
	"testing"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// =============================================================================
// 降级测试（详细设计 §5.3 降级表）：
// FinBERT 挂 → 转 LLM；云 LLM 挂 → 转 Ollama；全挂 → unknown 且**不猜**。
// =============================================================================

// stubEngine 可控情感引擎替身。
type stubEngine struct {
	name      string
	available bool
	res       SentimentResult
	err       error
	calls     int
}

func (s *stubEngine) Available() bool { return s.available }
func (s *stubEngine) Name() string    { return s.name }
func (s *stubEngine) Classify(_ context.Context, _ SentimentInput) (SentimentResult, error) {
	s.calls++
	if s.err != nil {
		return SentimentResult{}, s.err
	}
	return s.res, nil
}

var errEngineDown = errors.New("engine down")

func TestSentimentFinBERTDownFallsToLLM(t *testing.T) {
	fb := &stubEngine{name: "finbert", available: true, err: errEngineDown}
	cloud := &stubEngine{name: "llm-cloud", available: true,
		res: SentimentResult{Label: mediacontract.SentimentNegative, Score: -0.6}}

	o := NewSentimentOrchestrator(fb, cloud, nil, log.DefaultLogger)
	res, engine := o.Classify(context.Background(), SentimentInput{Title: "Fed hikes", Lang: "en"})

	if engine != mediacontract.EngineLLM {
		t.Fatalf("FinBERT 挂应降级到 LLM（契约引擎 llm），got %s", engine)
	}
	if res.Label != mediacontract.SentimentNegative || res.Score != -0.6 {
		t.Fatalf("应返回 LLM 结果，got %+v", res)
	}
	if fb.calls != 1 || cloud.calls != 1 {
		t.Fatalf("应先试 FinBERT 再降级 LLM：finbert=%d llm=%d", fb.calls, cloud.calls)
	}
}

func TestSentimentCloudLLMDownFallsToOllama(t *testing.T) {
	cloud := &stubEngine{name: "llm-cloud", available: true, err: errEngineDown}
	ollama := &stubEngine{name: "llm-ollama", available: true,
		res: SentimentResult{Label: mediacontract.SentimentPositive, Score: 0.8}}

	o := NewSentimentOrchestrator(nil, cloud, ollama, log.DefaultLogger)
	res, engine := o.Classify(context.Background(), SentimentInput{Title: "央行降准释放流动性", Lang: "zh"})

	if engine != mediacontract.EngineLLM {
		t.Fatalf("云 LLM 挂应降级 Ollama（契约仍归 llm），got %s", engine)
	}
	if res.Label != mediacontract.SentimentPositive {
		t.Fatalf("应返回 Ollama 结果，got %+v", res)
	}
	if cloud.calls != 1 || ollama.calls != 1 {
		t.Fatalf("应先试云再降级 Ollama：cloud=%d ollama=%d", cloud.calls, ollama.calls)
	}
}

func TestSentimentAllDownReturnsUnknownNoGuess(t *testing.T) {
	// 🔴 本地 LLM 也不可用 → 不产出方向结论（不猜）：label=unknown、score=0、engine=none。
	fb := &stubEngine{name: "finbert", available: true, err: errEngineDown}
	cloud := &stubEngine{name: "llm-cloud", available: true, err: errEngineDown}
	ollama := &stubEngine{name: "llm-ollama", available: true, err: errEngineDown}

	o := NewSentimentOrchestrator(fb, cloud, ollama, log.DefaultLogger)
	for _, lang := range []string{"en", "zh", "zh-CN"} {
		res, engine := o.Classify(context.Background(), SentimentInput{Title: "任意标题", Lang: lang, IsMacro: lang != "en"})
		if res.Label != mediacontract.SentimentUnknown || res.Score != 0 || engine != mediacontract.EngineNone {
			t.Fatalf("全挂必须 unknown/none 且 score=0（禁止猜测），lang=%s got %+v engine=%s", lang, res, engine)
		}
	}
}

func TestSentimentNotConfiguredTreatedAsUnavailable(t *testing.T) {
	// 未配置（URL/Key 缺）视为不可用直接跳过，不计失败；全未配置 → unknown 不猜。
	fb := &stubEngine{name: "finbert", available: false}
	cloud := &stubEngine{name: "llm-cloud", available: false, err: errEngineDown}

	o := NewSentimentOrchestrator(fb, cloud, nil, log.DefaultLogger)
	res, engine := o.Classify(context.Background(), SentimentInput{Title: "any", Lang: "en"})
	if res.Label != mediacontract.SentimentUnknown || engine != mediacontract.EngineNone {
		t.Fatalf("引擎均未配置应 unknown/none，got %+v engine=%s", res, engine)
	}
	if fb.calls != 0 || cloud.calls != 0 {
		t.Fatalf("未配置引擎不应被调用：finbert=%d llm=%d", fb.calls, cloud.calls)
	}
}

func TestSentimentEngineReturningUnknownIsSkipped(t *testing.T) {
	// 引擎自身返回 unknown（如 LLM 输出非法标签被适配层拒绝）→ 继续降级，不采纳。
	fb := &stubEngine{name: "finbert", available: true,
		res: SentimentResult{Label: mediacontract.SentimentUnknown, Score: 0}}
	cloud := &stubEngine{name: "llm-cloud", available: true,
		res: SentimentResult{Label: mediacontract.SentimentNeutral, Score: 0.05}}

	o := NewSentimentOrchestrator(fb, cloud, nil, log.DefaultLogger)
	res, engine := o.Classify(context.Background(), SentimentInput{Title: "Fed holds rates", Lang: "en"})
	if engine != mediacontract.EngineLLM || res.Label != mediacontract.SentimentNeutral {
		t.Fatalf("FinBERT 返回 unknown 应降级采纳 LLM 结果，got %+v engine=%s", res, engine)
	}
}

func TestSentimentScoreClamped(t *testing.T) {
	// 引擎输出越界分数必须夹在 [-1,1]（适配层防线）。
	fb := &stubEngine{name: "finbert", available: true,
		res: SentimentResult{Label: mediacontract.SentimentPositive, Score: 5.0}}

	o := NewSentimentOrchestrator(fb, nil, nil, log.DefaultLogger)
	res, _ := o.Classify(context.Background(), SentimentInput{Title: "stocks soar", Lang: "en"})
	if res.Score != 1 {
		t.Fatalf("score 越界必须夹到 [-1,1]，got %f", res.Score)
	}
}

func TestSentimentMacroRoutesToLLM(t *testing.T) {
	// 英文但宏观类 → 走 LLM（详细设计 §2.2 选择策略）。
	fb := &stubEngine{name: "finbert", available: true,
		res: SentimentResult{Label: mediacontract.SentimentNeutral, Score: 0.01}}
	cloud := &stubEngine{name: "llm-cloud", available: true,
		res: SentimentResult{Label: mediacontract.SentimentNegative, Score: -0.7}}

	o := NewSentimentOrchestrator(fb, cloud, nil, log.DefaultLogger)
	res, engine := o.Classify(context.Background(), SentimentInput{Title: "Powell signals prolonged restriction", Lang: "en", IsMacro: true})
	if engine != mediacontract.EngineLLM || res.Label != mediacontract.SentimentNegative {
		t.Fatalf("宏观类应优先 LLM，got %+v engine=%s", res, engine)
	}
	if fb.calls != 0 {
		t.Fatalf("宏观类不应先试 FinBERT（除非 LLM 全挂），finbert calls=%d", fb.calls)
	}
}

func TestSentimentLLMDownFallsBackFinBERTForEnglish(t *testing.T) {
	// §5.3 降级表：LLM 全挂时 FinBERT 仍可处理英文。
	fb := &stubEngine{name: "finbert", available: true,
		res: SentimentResult{Label: mediacontract.SentimentPositive, Score: 0.4}}
	cloud := &stubEngine{name: "llm-cloud", available: true, err: errEngineDown}

	o := NewSentimentOrchestrator(fb, cloud, nil, log.DefaultLogger)
	res, engine := o.Classify(context.Background(), SentimentInput{Title: "Fed cuts rates", Lang: "en", IsMacro: true})
	if engine != mediacontract.EngineFinBERT || res.Label != mediacontract.SentimentPositive {
		t.Fatalf("LLM 挂应回退 FinBERT（仅英文），got %+v engine=%s", res, engine)
	}
}
