package mme

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeCacheRepo 内存仓储。
type fakeCacheRepo struct {
	mu    sync.Mutex
	store map[string]*CacheEntry
}

func newFakeCacheRepo() *fakeCacheRepo { return &fakeCacheRepo{store: map[string]*CacheEntry{}} }

func (f *fakeCacheRepo) Get(_ context.Context, key string) (*CacheEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.store[key], nil
}
func (f *fakeCacheRepo) Upsert(_ context.Context, e *CacheEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[e.CacheKey] = e
	return nil
}
func (f *fakeCacheRepo) IncrHit(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key].HitCount++
	return nil
}
func (f *fakeCacheRepo) TotalBytes(context.Context) (int64, error) { return 0, nil }

func TestCacheKeySensitivity(t *testing.T) {
	base := CacheKey("as_1", CacheKindTranscript, "whisper", "base", "v1.0", "", "")
	// prompt_version 变化 → 键必变（FR-MIX-02 核心：prompt 升级全量失效）
	if k := CacheKey("as_1", CacheKindTranscript, "whisper", "base", "v1.1", "", ""); k == base {
		t.Fatal("cache key must change with prompt_version")
	}
	// intent 变化 → 键必变
	if k := CacheKey("as_1", CacheKindTranscript, "whisper", "base", "v1.0", "带货", ""); k == base {
		t.Fatal("cache key must change with intent")
	}
	// engine version 变化 → 键必变
	if k := CacheKey("as_1", CacheKindTranscript, "whisper", "large-v3", "v1.0", "", ""); k == base {
		t.Fatal("cache key must change with engine_version")
	}
	// 同参必同键（命中前提）
	if k := CacheKey("as_1", CacheKindTranscript, "whisper", "base", "v1.0", "", ""); k != base {
		t.Fatal("cache key must be deterministic")
	}
}

func TestAnalysisCacheHitZeroCalls(t *testing.T) {
	repo := newFakeCacheRepo()
	c := NewAnalysisCache(testConfig(), repo)
	ctx := context.Background()
	key := CacheKey("as_1", CacheKindTranscript, "whisper", "base", "v1.0", "", "")
	if err := c.Store(ctx, &CacheEntry{
		CacheKey: key, Kind: CacheKindTranscript, AssetID: "as_1",
		PayloadPath: "/tmp/x.srt", PayloadMeta: map[string]any{"text": "hello"},
	}); err != nil {
		t.Fatalf("store: %v", err)
	}
	// 第一次命中
	hit, err := c.Lookup(ctx, key)
	if err != nil || hit == nil {
		t.Fatalf("lookup want hit, got %v %v", hit, err)
	}
	// 第二次命中 hit_count 应累计（观测复用率）
	_, _ = c.Lookup(ctx, key)
	e, _ := repo.Get(ctx, key)
	if e.HitCount < 2 {
		t.Fatalf("hit_count want >=2, got %d", e.HitCount)
	}
}

func TestAnalysisCacheExpired(t *testing.T) {
	repo := newFakeCacheRepo()
	c := NewAnalysisCache(testConfig(), repo)
	ctx := context.Background()
	key := "k_expired"
	e := &CacheEntry{CacheKey: key, Kind: CacheKindReport, ExpiresAt: time.Now().Add(-time.Hour)}
	_ = repo.Upsert(ctx, e)
	if hit, _ := c.Lookup(ctx, key); hit != nil {
		t.Fatal("expired entry must not hit")
	}
}

func TestAnalysisCacheDisabledHonest(t *testing.T) {
	cfg := testConfig()
	cfg.CacheEnabled = false
	c := NewAnalysisCache(cfg, newFakeCacheRepo())
	if err := c.Store(context.Background(), &CacheEntry{CacheKey: "k", Kind: CacheKindReport}); err != ErrCacheDisabled {
		t.Fatalf("disabled cache store want ErrCacheDisabled, got %v", err)
	}
	if hit, _ := c.Lookup(context.Background(), "k"); hit != nil {
		t.Fatal("disabled cache must return miss (not error)")
	}
}
