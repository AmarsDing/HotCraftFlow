// Package mediacontract 是五子系统（IC/TT/MIA/MME/MAP）的唯一跨系统数据契约层。
// 对应文档：docs/systems/00-shared/01-data-contract.md
//
// 纪律：
//  - 跨系统只允许通过本包定义的结构交换数据；出现本包之外的字段即契约违规。
//  - 枚举一律小写下划线字符串，禁止整型魔法数字跨系统传。
//  - 时间一律 UTC（RFC3339 带 Z），展示层才转 Asia/Shanghai。
//  - 本包不引入任何第三方依赖（ULID 自实现），保证零传递污染。
package mediacontract

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// crockfordBase32 是 ULID 的编码字母表（排除 I, L, O, U 以避免混淆）。
const crockfordBase32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// ULID 长度固定 26：前 10 字符为毫秒时间戳，后 16 字符为随机量。
const ulidLen = 26

// ulidMonotonic 保证同一毫秒内生成的 ULID 随机部分单调递增，
// 使字典序严格等于生成顺序（契约 §0.2「字典序≈时间序」的强保证）。
var ulidMonotonic = &monotonic{}

type monotonic struct {
	mu     sync.Mutex
	lastMs uint64
	lastHi uint64 // 高 40 bit 随机
	lastLo uint64 // 低 40 bit 随机（只取低 40 位使用）
}

// ULID 生成一个 26 字符的 ULID 字符串（Crockford base32）。
// 时间戳取 UTC 毫秒，随机量取自 crypto/rand；同一毫秒内单调递增。
func ULID() string {
	now := uint64(time.Now().UTC().UnixMilli())
	return makeULID(now)
}

// ULIDAt 用指定时间生成 ULID（用于测试与回填场景）。
func ULIDAt(t time.Time) string {
	return makeULID(uint64(t.UTC().UnixMilli()))
}

func makeULID(ms uint64) string {
	var buf [ulidLen]byte

	// 时间部分：48 bit，编码为前 10 字符。
	// 48 bit 可表示到公元 10889 年，不会溢出。
	t := ms
	for i := 9; i >= 0; i-- {
		buf[i] = crockfordBase32[t&0x1F]
		t >>= 5
	}

	// 随机部分：80 bit，编码为后 16 字符。
	hi, lo := ulidMonotonic.next(ms)
	// hi 40 bit -> 8 字符；lo 40 bit -> 8 字符。
	for i := 17; i >= 10; i-- {
		buf[i] = crockfordBase32[hi&0x1F]
		hi >>= 5
	}
	for i := 25; i >= 18; i-- {
		buf[i] = crockfordBase32[lo&0x1F]
		lo >>= 5
	}
	return string(buf[:])
}

// next 返回 80 bit 随机量（拆成两个 40 bit）。
// 若与上一次同一毫秒，则在上一值基础上 +1（单调递增）；否则重新取随机。
func (m *monotonic) next(ms uint64) (hi, lo uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ms != m.lastMs {
		var rb [10]byte
		if _, err := rand.Read(rb[:]); err != nil {
			// crypto/rand 失败属严重异常；退化为时间+进程内自增，绝不 panic 阻塞业务。
			m.lastHi = ms*2654435761 ^ 0x9E3779B97F4A7C15
			m.lastLo = ms * 6364136223846793005
		} else {
			m.lastHi = uint64(rb[0])<<32 | uint64(rb[1])<<24 | uint64(rb[2])<<16 | uint64(rb[3])<<8 | uint64(rb[4])
			m.lastLo = uint64(rb[5])<<32 | uint64(rb[6])<<24 | uint64(rb[7])<<16 | uint64(rb[8])<<8 | uint64(rb[9])
		}
		m.lastMs = ms
		return m.lastHi, m.lastLo
	}

	// 同一毫秒：随机量 +1（80 bit 空间，撞库概率忽略）。
	m.lastLo++
	if m.lastLo == 0 {
		m.lastHi++
	}
	return m.lastHi, m.lastLo
}

// NewID 生成带前缀的实体 ID，格式 `{前缀}_{ULID}`。
// 前缀须取自契约 §0.3 前缀分配表（如 PrefixIntelItem）。
func NewID(prefix string) string {
	return prefix + "_" + ULID()
}

// 契约 §0.3 ID 前缀分配表。
const (
	PrefixIntelItem       = "it"  // IntelItem（① IC）
	PrefixCluster         = "cl"  // 去重簇（① IC）
	PrefixEvidence        = "ev"  // EvidenceSnapshot（① IC）
	PrefixTopic           = "tp"  // Topic（② TT）
	PrefixTopicCandidate  = "tc"  // TopicCandidate（② TT）
	PrefixAlert           = "al"  // Alert（② TT）
	PrefixImpactCard      = "im"  // ImpactCard（③ MIA）
	PrefixEventCard       = "ec"  // EventCard（③ MIA）
	PrefixAsset           = "as"  // Asset（④ MME）
	PrefixVariantSet      = "vs"  // VariantSet（④ MME）
	PrefixVariant         = "v"   // Variant（④ MME）
	PrefixDispatchUnit    = "du"  // DispatchUnit（⑤ MAP）
	PrefixPublishAccount  = "pa"  // PublishAccount（⑤ MAP）
	PrefixPublishReceipt  = "pr"  // PublishReceipt（⑤ MAP）
	PrefixPerformanceSnap = "ps"  // PerformanceSnapshot（⑤ MAP）
	PrefixIntelSource     = "src" // IntelSource（① IC，内部实体）
	PrefixSubscription    = "ts"  // TrendSubscription（② TT，内部实体）
	PrefixRule            = "rule" // TransmissionRule / ComplianceRule
	PrefixMontageTask     = "task" // MontageTask（④ MME）
	PrefixRenderJob       = "job" // RenderJob（④ MME）
	PrefixCollectRun      = "run" // CollectRun（① IC，内部实体）
	PrefixCredentialAcct  = "acc" // CredentialAccount（① IC / ⑤ MAP）
	PrefixEvent           = "evt" // EventEnvelope.event_id（跨系统事件）
)

// 便捷构造函数。
func NewIntelItemID() string       { return NewID(PrefixIntelItem) }
func NewClusterID() string         { return NewID(PrefixCluster) }
func NewEvidenceID() string        { return NewID(PrefixEvidence) }
func NewTopicID() string           { return NewID(PrefixTopic) }
func NewCandidateID() string       { return NewID(PrefixTopicCandidate) }
func NewAlertID() string           { return NewID(PrefixAlert) }
func NewImpactCardID() string      { return NewID(PrefixImpactCard) }
func NewEventCardID() string       { return NewID(PrefixEventCard) }
func NewAssetID() string           { return NewID(PrefixAsset) }
func NewVariantSetID() string      { return NewID(PrefixVariantSet) }
func NewVariantID() string         { return NewID(PrefixVariant) }
func NewDispatchUnitID() string    { return NewID(PrefixDispatchUnit) }
func NewPublishAccountID() string  { return NewID(PrefixPublishAccount) }
func NewPublishReceiptID() string  { return NewID(PrefixPublishReceipt) }
func NewPerformanceSnapID() string { return NewID(PrefixPerformanceSnap) }
func NewMontageTaskID() string     { return NewID(PrefixMontageTask) }
func NewRenderJobID() string       { return NewID(PrefixRenderJob) }
func NewEventID() string           { return NewID(PrefixEvent) }

// ValidateIDFormat 校验 ID 是否符合 `{前缀}_{26位ULID}` 格式。
func ValidateIDFormat(id, wantPrefix string) error {
	want := wantPrefix + "_"
	if len(id) != len(want)+ulidLen {
		return fmt.Errorf("mediacontract: id %q 长度非法（期望前缀 %q + 26 位 ULID）", id, want)
	}
	if id[:len(want)] != want {
		return fmt.Errorf("mediacontract: id %q 前缀非法（期望 %q）", id, want)
	}
	for _, c := range id[len(want):] {
		if !isCrockford(byte(c)) {
			return fmt.Errorf("mediacontract: id %q 含非法字符 %q", id, string(c))
		}
	}
	return nil
}

func isCrockford(c byte) bool {
	for i := 0; i < len(crockfordBase32); i++ {
		if crockfordBase32[i] == c {
			return true
		}
	}
	return false
}
