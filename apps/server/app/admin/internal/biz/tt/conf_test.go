package tt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestConfDurationUnmarshalJSON kratos Scan 走 encoding/json 的回归门：
// time.Duration 在 json 下只收纳秒整数，yaml 习惯书写的 "5s"/"24h" 字面量
// 必须经 Conf.UnmarshalJSON 预归一为纳秒数。新增 Duration 字段未登记
// durationJSONPaths 会在起栈时 fail-fast（尝试反序列化即炸）——这正是设计意图。
func TestConfDurationUnmarshalJSON(t *testing.T) {
	b := []byte(`{
		"ingest": {"flush_interval": "5s", "dedup_ttl": "24h", "last_wins_guard_ttl": "48h"},
		"influx": {"timeout": "10s"},
		"buffer": {"replay_interval": "15s"}
	}`)
	var c Conf
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("时长字面量应可解码: %v", err)
	}
	if c.Ingest.FlushInterval != 5*time.Second ||
		c.Ingest.DedupTTL != 24*time.Hour ||
		c.Ingest.LastWinsGuardTTL != 48*time.Hour ||
		c.Influx.Timeout != 10*time.Second ||
		c.Buffer.ReplayInterval != 15*time.Second {
		t.Fatalf("时长解码值不符: %+v", c)
	}

	// 纳秒数字直传（编程式注入/工具链路径）亦须兼容
	var c2 Conf
	if err := json.Unmarshal([]byte(`{"buffer":{"replay_interval":1000}}`), &c2); err != nil {
		t.Fatalf("纳秒直传应兼容: %v", err)
	}
	if c2.Buffer.ReplayInterval != 1000 {
		t.Fatalf("纳秒直传值不符: %v", c2.Buffer.ReplayInterval)
	}

	// 块缺失直通（Normalize 兜底默认值），非法字面量 fail-fast 且带路径定位
	var c3 Conf
	if err := json.Unmarshal([]byte(`{"ingest":{"durable":"x"}}`), &c3); err != nil {
		t.Fatalf("块缺失应直通: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"ingest":{"dedup_ttl":"一天"}}`), &c3); err == nil ||
		!strings.Contains(err.Error(), "ingest.dedup_ttl") {
		t.Fatalf("非法时长应报路径定位错误: %v", err)
	}
}

// TestConfValidateRedLines 红线判据的最小回归（bucket 区间/UTC/滞回）。
func TestConfValidateRedLines(t *testing.T) {
	c := DefaultConf()
	if err := c.Validate(); err != nil {
		t.Fatalf("默认配置应过红线: %v", err)
	}
	bad := c
	bad.Timezone.Storage = "Asia/Shanghai"
	if err := bad.Validate(); err == nil {
		t.Fatal("timezone.storage 非 UTC 必须 fail-fast（NFR-TT-03）")
	}
	bad = c
	bad.Buffer.ResumeDepth = bad.Buffer.PauseDepth
	if err := bad.Validate(); err == nil {
		t.Fatal("resume_depth ≥ pause_depth 必须 fail-fast（背压滞回）")
	}
}
