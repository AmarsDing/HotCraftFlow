package mediacontract

import "time"

// 契约 §0.2 时区纪律：存储一律 UTC（RFC3339 带 Z），展示层才转 Asia/Shanghai。
// 本文件提供统一的时间处理入口，禁止在业务代码里直接 time.Now() 后落库。

// UTCNow 返回当前 UTC 时间（截断到秒，避免序列化精度漂移）。
// 业务代码取「当前时间」一律用它，不要用 time.Now() 直接落库。
func UTCNow() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

// FormatRFC3339UTC 把时间格式化为 RFC3339 UTC 带 Z（如 2026-10-17T01:00:00Z）。
// 契约示例一律用这个形态，禁止 +08:00 偏移。
func FormatRFC3339UTC(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// ParseRFC3339 解析 RFC3339 时间，并强制转 UTC。
func ParseRFC3339(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// IsFuture 判断 t 是否明显晚于 now（容忍 5 分钟时钟漂移）。
// 用于校验上游时间戳不可信的场景（评审 IC S-16）。
func IsFuture(t, now time.Time) bool {
	return t.After(now.Add(5 * time.Minute))
}

// IsTooOld 判断 t 是否早于 now 超过 maxAge（迟到数据丢弃阈值）。
func IsTooOld(t, now time.Time, maxAge time.Duration) bool {
	return t.Before(now.Add(-maxAge))
}
