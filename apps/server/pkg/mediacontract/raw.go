package mediacontract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// =============================================================================
// 采集原始片段与取证哈希链（评审 S-22 / B-17 / B-21）
// S-22：RawFragment 下沉到契约包，biz 与 data 共同依赖，避免依赖倒置。
// B-21：原始片段绝不进 NATS 消息体（NATS 载荷只带 RawMeta）。
// =============================================================================

// RawFragment 一次采样的内存视图。生存期：单条归一化处理流内。
type RawFragment struct {
	Content    []byte            // 内存上限 4MB（conf ic.evidence.raw_max_bytes）
	HTTPStatus int               // 无 HTTP 采集（手贴/侧车）为 0
	Headers    map[string]string // 原始响应头（落盘前须脱敏，见 SanitizedHeaders）
	RequestURL string
	FinalURL   string // 302 落点；无重定向同 RequestURL
}

// RawMeta 是 RawFragment 的 NATS 可携带形态（B-21：不带字节本体）。
type RawMeta struct {
	RawPath      string `json:"raw_path"`       // 证据盘相对路径（S-27 本机目录）
	RawHash      string `json:"raw_hash"`       // sha256 hex
	RawSize      int64  `json:"raw_size"`       // 原始字节数（截断前）
	RawTruncated bool   `json:"raw_truncated"`  // 是否被截断
	PrevHash     string `json:"prev_hash"`      // B-17 证据链
	ChainHash    string `json:"chain_hash"`
	EvidenceID   string `json:"evidence_id"` // ev_ 前缀
}

// RawIntelNotice mm.intel.raw 载荷（B-8 第 4 步）。
// 携带归一化消费者所需的最小上下文；原始字节永不进消息体（B-21）。
type RawIntelNotice struct {
	APIVersion  string    `json:"api_version"`  // 恒 "1.0"
	IntelID     string    `json:"intel_id"`
	SourceID    string    `json:"source_id"`
	CollectedAt time.Time `json:"collected_at"` // 本地接收时间 UTC
	Raw         RawMeta   `json:"raw"`
}

// RawSHA256 计算原始内容的 sha256（小写 hex）。
func RawSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ChainHash B-17：chain = sha256(prev_hash || raw_hash || intel_id || collectedAt.UnixNano())。
// prev 为空表示链首条；链哈希是取证完整性结论，导出包附 prev/raw/chain FOR 复核。
func ChainHash(prevHash, rawHash, intelID string, collectedAt time.Time) string {
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write([]byte(rawHash))
	h.Write([]byte(intelID))
	var ns [8]byte
	binary.BigEndian.PutUint64(ns[:], uint64(collectedAt.UTC().UnixNano()))
	h.Write(ns[:])
	return hex.EncodeToString(h.Sum(nil))
}

// 脱敏头黑名单（NFR-IC-06 与日志纪律一致：禁 Cookie/Authorization/Set-Cookie/X-CSRF-Token）。
var redactedHeaderKeys = map[string]struct{}{
	"cookie":        {},
	"authorization": {},
	"set-cookie":    {},
	"x-csrf-token":  {},
	"proxy-authorization": {},
}

// SanitizedHeaders 留受限集合做证据指纹（契约 §1 evidence.http_headers：脱敏受限集合）。
// 白名单取舍：仅保留与证据性相关的内容协商/校验头。
var keptHeaderKeys = map[string]struct{}{
	"content-type":     {},
	"content-length":   {},
	"etag":             {},
	"last-modified":    {},
	"cache-control":    {},
	"server":           {},
	"retry-after":      {},
	"x-ratelimit-remaining": {},
}

func SanitizedHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		lk := lowerASCII(k)
		if _, blocked := redactedHeaderKeys[lk]; blocked {
			continue
		}
		if _, kept := keptHeaderKeys[lk]; !kept {
			continue
		}
		out[k] = v
	}
	return out
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
