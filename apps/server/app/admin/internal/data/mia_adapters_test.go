package data

import (
	"context"
	"errors"
	"testing"

	"github.com/go-kratos/kratos/v2/log"

	bizmia "framework/app/admin/internal/biz/mia"
	"framework/pkg/mediacontract"
)

// =============================================================================
// 🔴 数据层合规测试（详细设计 §7）：
// - OpenBB 许可证未复核 → 阻断接入（MIA_OPENBB_LICENSE_UNVERIFIED，恒阻断不降级放行）；
// - LLM 情感输出严格 JSON：未知/非法标签拒绝（禁止猜测的最后防线）。
// =============================================================================

func TestBlockedOpenBBProviderAlwaysRefuses(t *testing.T) {
	p := NewMiaBlockedOpenBBProvider(log.DefaultLogger)
	if p.Name() != "openbb" {
		t.Fatalf("provider 名应为 openbb，got %s", p.Name())
	}
	for _, symbol := range []string{"AAPL", "BTC/USDT", "XAUUSD"} {
		snap, err := p.FetchOHLCV(context.Background(), symbol)
		if !errors.Is(err, bizmia.ErrOpenBBLicenseUnverified) {
			t.Fatalf("OpenBB 未复核必须阻断（%s）：got err=%v", symbol, err)
		}
		if snap != nil {
			t.Fatalf("阻断路径不得返回任何行情数据，got %+v", snap)
		}
	}
}

func TestParseLLMSentiment_StrictLabelOnly(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr bool
		wantLbl mediacontract.SentimentLabel
	}{
		{"strict json ok", `{"label":"positive","score":0.8}`, false, mediacontract.SentimentPositive},
		{"fenced json ok", "```json\n{\"label\":\"negative\",\"score\":-0.3}\n```", false, mediacontract.SentimentNegative},
		{"surrounding whitespace ok", "  \n{\"label\":\"neutral\",\"score\":0}\n ", false, mediacontract.SentimentNeutral},
		{"prose wrapped rejected", `结论如下：{"label":"positive","score":0.8}`, true, ""},
		{"unknown label rejected", `{"label":"unknown","score":0}`, true, ""},
		{"invented label rejected", `{"label":"very_bullish","score":0.9}`, true, ""},
		{"empty label rejected", `{"label":"","score":0}`, true, ""},
		{"not json rejected", `neutral`, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := parseLLMSentiment(tc.content)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("应拒绝输出 %q（禁止猜测），got res=%+v", tc.content, res)
				}
				return
			}
			if err != nil {
				t.Fatalf("合法输出 %q 应通过，got err=%v", tc.content, err)
			}
			if res.Label != tc.wantLbl {
				t.Fatalf("标签不符：want %s got %s", tc.wantLbl, res.Label)
			}
		})
	}
}

func TestParseLLMSentiment_ScoreClamped(t *testing.T) {
	res, err := parseLLMSentiment(`{"label":"positive","score":3.7}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 1 {
		t.Fatalf("score 越界必须夹在 [-1,1]，got %f", res.Score)
	}
	res, err = parseLLMSentiment(`{"label":"negative","score":-9}`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != -1 {
		t.Fatalf("score 越界必须夹在 [-1,1]，got %f", res.Score)
	}
}
