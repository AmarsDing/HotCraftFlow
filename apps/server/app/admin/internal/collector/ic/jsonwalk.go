package iccollector

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// =============================================================================
// JSON 取值工具（api 采集器的配置驱动字段映射用；不引第三方 jsonpath 依赖，
// 仅支持 `a.b.0.c` 点号路径——V1 够用，复杂 JSON 交给 crawl/rss 编排）
// =============================================================================

// decodeJSON 反序列化（容忍 UTF-8 BOM）。
func decodeJSON(b []byte, target any) error {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	return json.Unmarshal(b, target)
}

// walkPath 按点号路径取值（数字段按数组下标）。
func walkPath(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}
	cur := v
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// asString 宽口径转字符串（number/bool 转描述；对象 JSON 化）。
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// asInt 宽口径转 int64（字符串数字兼容）。
func asInt(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case string:
		t = strings.ReplaceAll(t, ",", "")
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return int64(f), true
		}
		return 0, false
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i, true
		}
		if f, err := t.Float64(); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

// asTime 宽口径时间解析（RFC3339 / unix 秒 / unix 毫秒 / 常见中文格式）。
func asTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case float64: // heuristics：大于 1e12 按毫秒
		sec := int64(t)
		if sec > 1e12 {
			return time.UnixMilli(sec).UTC(), true
		}
		if sec > 1e9 {
			return time.Unix(sec, 0).UTC(), true
		}
		return time.Time{}, false
	case string:
		s := strings.TrimSpace(t)
		for _, layout := range []string{
			time.RFC3339, time.RFC3339Nano,
			"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02",
			time.RFC1123, time.RFC1123Z, time.ANSIC,
		} {
			if ts, err := time.Parse(layout, s); err == nil {
				return ts.UTC(), true
			}
		}
		if i, ok := asInt(s); ok {
			return asTime(float64(i))
		}
	}
	return time.Time{}, false
}
