package mme

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"
)

// 缓存类别（详细设计 §2.2，4 级分析缓存）。
const (
	CacheKindTranscript = "transcript"
	CacheKindChapters   = "chapters"
	CacheKindReport     = "report"
	CacheKindKeyframes  = "keyframes"
	CacheKindReframed   = "reframed"
)

// CacheKey 计算缓存键：sha256(asset_id|kind|engine|engine_version|prompt_version|intent|lang)。
// 🔴 prompt_version 变化即全量失效（FR-MIX-02 的核心约束）——键必须对 prompt 敏感。
func CacheKey(assetID, kind, engine, engineVersion, promptVersion, intent, lang string) string {
	h := sha256.New()
	h.Write([]byte(assetID + "|" + kind + "|" + engine + "|" + engineVersion + "|" + promptVersion + "|" + intent + "|" + lang))
	return hex.EncodeToString(h.Sum(nil))[:40]
}

// AnalysisCache 4 级分析缓存（FR-MIX-02）：同素材重渲染时 ASR/LLM 调用 = 0。
type AnalysisCache struct {
	cfg  Config
	repo CacheRepo
}

// NewAnalysisCache 构造。
func NewAnalysisCache(cfg Config, repo CacheRepo) *AnalysisCache {
	return &AnalysisCache{cfg: cfg, repo: repo}
}

// ttlOf 各类别 TTL；transcript 寿命更长（转写对 prompt 不敏感）。
func (c *AnalysisCache) ttlOf(kind string) time.Duration {
	if kind == CacheKindTranscript && c.cfg.TranscriptTTLDays > 0 {
		return time.Duration(c.cfg.TranscriptTTLDays) * 24 * time.Hour
	}
	return time.Duration(maxInt(c.cfg.CacheTTLDays, 1)) * 24 * time.Hour
}

// Lookup 命中返回条目并记 hit_count；未命中/关闭/过期返回 nil。
func (c *AnalysisCache) Lookup(ctx context.Context, key string) (*CacheEntry, error) {
	if !c.cfg.CacheEnabled {
		return nil, nil // 缓存关闭是合法配置，不是错误
	}
	e, err := c.repo.Get(ctx, key)
	if err != nil || e == nil {
		return nil, err
	}
	if !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt) {
		return nil, nil
	}
	// 命中统计失败不阻断主流程（观测丢失可接受）。
	_ = c.repo.IncrHit(ctx, key)
	e.HitCount++
	return e, nil
}

// Store 幂等写入；超限（max_cache_bytes）时仍写入但向上返回 ErrStorageFull 供任务侧提示治理。
func (c *AnalysisCache) Store(ctx context.Context, e *CacheEntry) error {
	if !c.cfg.CacheEnabled {
		return ErrCacheDisabled
	}
	e.ExpiresAt = time.Now().Add(c.ttlOf(e.Kind))
	if err := c.repo.Upsert(ctx, e); err != nil {
		return err
	}
	if c.cfg.MaxCacheBytes > 0 {
		if total, err := c.repo.TotalBytes(ctx); err == nil && total > c.cfg.MaxCacheBytes {
			return ErrStorageFull
		}
	}
	return nil
}

// SortPairs 相似度对排序（先 overall 降序再 A/B 字典序）——审核台展示要稳定。
func SortPairs(pairs []PairSimilarity) {
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Overall != pairs[j].Overall {
			return pairs[i].Overall > pairs[j].Overall
		}
		if pairs[i].VariantA != pairs[j].VariantA {
			return pairs[i].VariantA < pairs[j].VariantA
		}
		return pairs[i].VariantB < pairs[j].VariantB
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
