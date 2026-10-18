package mia

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
)

// Notifier 高影响推送（FR-MIA-12）：高分条目推送 Webhook / 企业微信。
// 🔴 推送体必须含影响路径与免责声明（NFR-MIA-01 同样约束推送通道）。
type Notifier struct {
	sender WebhookSender
	log    *log.Helper
}

// NewNotifier 构造通知器。sender 为 nil 时 push 一律 no-op。
func NewNotifier(sender WebhookSender, logger log.Logger) *Notifier {
	return &Notifier{
		sender: sender,
		log:    log.NewHelper(log.With(logger, "module", "biz/mia/notifier")),
	}
}

// PushImpactCard 推送高分影响卡。返回是否真正推送。
// 幂等：推送成功后由调用方 MarkPushed；pushed=true 的卡不再重推（上游 scores 防抖）。
func (n *Notifier) PushImpactCard(ctx context.Context, cfg Config, card *ImpactCardRow, event *EventCardRow) (bool, error) {
	if n.sender == nil {
		return false, nil
	}
	if card.Pushed {
		return false, nil
	}
	// 🔴 合规前置：disclaimer 为空拒绝推送（与落库同口径）。
	if strings.TrimSpace(card.Disclaimer) == "" {
		return false, ErrDisclaimerEmpty
	}
	// 🔴 推送前置：每条 affected 必须有 path 与有效 confidence（与落库同口径）。
	if err := ValidateImpactCardForStore(card); err != nil {
		return false, err
	}
	payload := n.buildWebhookPayload(card, event)
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	if err := n.sender.Send(ctx, body); err != nil {
		return false, err
	}
	n.log.WithContext(ctx).Infof("impact card pushed: %s score=%.1f dir=%s", card.ImpactID, card.ImpactScore, card.Direction)
	return true, nil
}

// buildWebhookPayload 组装 Webhook 发送体（含路径与免责声明）。
// 选企业微信 markdown 兼容结构；通用 webhook 亦可用。
func (n *Notifier) buildWebhookPayload(card *ImpactCardRow, event *EventCardRow) map[string]any {
	var paths []string
	for i, a := range card.Affected {
		if i >= 8 { // 推送最多展示 8 条路径，全量见详情页
			paths = append(paths, "…")
			break
		}
		tag := ""
		if a.IsTransmission {
			tag = "（假设非因果）"
		}
		paths = append(paths, "· "+a.Symbol+"["+string(a.AssetClass)+"] "+string(a.Direction)+
			" conf="+formatF2(a.Confidence)+tag+"\n  "+strings.Join(a.Path, " → "))
	}
	content := "**高影响影响卡** " + card.ImpactID + "\n" +
		"标题：" + card.Title + "\n" +
		"方向：" + string(card.Direction) + "，影响分：" + formatF2(card.ImpactScore) +
		"（情感 " + string(card.SentimentLabel) + " by " + string(card.SentimentEngine) + "）\n" +
		"事件时间：" + card.OccurredAt.UTC().Format("2006-01-02 15:04:05 UTC") +
		"，TTL：" + intStr(card.TTLHours) + "h\n" +
		strings.Join(paths, "\n") + "\n" +
		"_" + card.Disclaimer + "_"
	return map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]any{
			"content": content,
		},
		// 结构化附录（非企业微信渠道可消费）。
		"impact_id":  card.ImpactID,
		"event_id":   card.EventID,
		"score":      card.ImpactScore,
		"direction":  card.Direction,
		"disclaimer": card.Disclaimer,
	}
}

func formatF2(v float64) string { return fmt.Sprintf("%.2f", v) }

func intStr(v int) string { return fmt.Sprintf("%d", v) }
