// Package textutil 提供文本归一化、分词与指纹工具，供 ① IC（去重）与 ④ MME（文案差异化）共用。
// SimHash 规格对应评审报告 IC B-9（可执行规格，保证不同人实现指纹可比）。
package textutil

import (
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
	"strconv"
)

// SHA256Hex 计算多段字符串拼接后的 sha256 hex（段间用 \x00 分隔防注入歧义）。
func SHA256Hex(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// FNV64a 计算字符串的 FNV-1a 64 位哈希（SimHash 的固定哈希函数，评审 B-9：禁止更换）。
func FNV64a(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// DedupKey 计算幂等去重键（评审 IC B-6）。
// dedup_key = sha256(url_key | source_id | cycle_bucket)。
func DedupKey(urlKey, sourceID string, cycleBucket int64) string {
	return SHA256Hex(urlKey, sourceID, strconv.FormatInt(cycleBucket, 10))
}

// CycleBucket 计算采集周期桶（评审 IC B-6）。
// cycle_bucket = floor(unix(t) / (interval_minutes*60)) * (interval_minutes*60)。
func CycleBucket(unixSec, intervalMinutes int64) int64 {
	if intervalMinutes <= 0 {
		intervalMinutes = 30
	}
	span := intervalMinutes * 60
	return (unixSec / span) * span
}
