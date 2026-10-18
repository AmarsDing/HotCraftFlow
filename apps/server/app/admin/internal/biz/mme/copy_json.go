package mme

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseSimpleCopyJSON 解析 LLM 输出的 {"title","desc","tags"} 文案对象。
// title 为空视为解析失败（调用方触发该平台模板降级）；tags 去空白、剥 # 前缀。
func parseSimpleCopyJSON(raw string) (PlatformCopy, error) {
	var m PlatformCopy
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return PlatformCopy{}, err
	}
	m.Title = strings.TrimSpace(m.Title)
	m.Desc = strings.TrimSpace(m.Desc)
	if m.Title == "" {
		return PlatformCopy{}, fmt.Errorf("copy json: empty title")
	}
	tags := make([]string, 0, len(m.Tags))
	for _, t := range m.Tags {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t != "" {
			tags = append(tags, t)
		}
	}
	m.Tags = tags
	return m, nil
}
