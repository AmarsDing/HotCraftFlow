package mme

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// fakeAssetRepo 去重测试用内存素材仓储。
type fakeAssetRepo struct {
	byFP     *Asset
	updated  *Asset
	refCalls int
	refCount int
}

func (f *fakeAssetRepo) Create(context.Context, *Asset) error          { return nil }
func (f *fakeAssetRepo) Update(_ context.Context, a *Asset) error      { f.updated = a; return nil }
func (f *fakeAssetRepo) Get(context.Context, string) (*Asset, error)   { return nil, nil }
func (f *fakeAssetRepo) GetByFingerprint(context.Context, string) (*Asset, error) {
	return f.byFP, nil
}
func (f *fakeAssetRepo) IncrRefCount(context.Context, string) (int, error) {
	f.refCalls++
	f.refCount++
	return f.refCount, nil
}

// fakeDedupStorage 只记录 Remove 调用。
type fakeDedupStorage struct{ removed string }

func (f *fakeDedupStorage) MediaDir() string                       { return "" }
func (f *fakeDedupStorage) AssetDir(string) string                 { return "" }
func (f *fakeDedupStorage) VariantDir(string) string               { return "" }
func (f *fakeDedupStorage) WorkDir(string) string                  { return "" }
func (f *fakeDedupStorage) IngestFile(context.Context, string, string) (string, int64, error) {
	return "", 0, nil
}
func (f *fakeDedupStorage) ExportZip(context.Context, string, []*Variant) (string, error) {
	return "", nil
}
func (f *fakeDedupStorage) Remove(_ context.Context, p string) error { f.removed = p; return nil }

func writeDedupTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("fake-payload-bytes-for-fingerprint"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAssetDedupBackfillsZeroMetadata(t *testing.T) {
	dir := t.TempDir()
	src := writeDedupTestFile(t, dir, "dup.mp4")
	// 既有行模拟「早期缺探测环境」入库：全 0 元数据。
	existing := &Asset{
		ID:          "as_zero",
		Fingerprint: "fp-x",
		LocalPath:   filepath.Join(dir, "a.mp4"),
		RefCount:    1,
	}
	repo := &fakeAssetRepo{byFP: existing, refCount: 1}
	st := &fakeDedupStorage{}
	d := NewAssetDeduper(testConfig(), repo, st, log.DefaultLogger)

	res, err := d.Ingest(context.Background(), src, "file:///dup.mp4", "实测标题", 123456, 5945, 1320, 2868)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Duplicate {
		t.Fatal("want duplicate=true on fingerprint hit")
	}
	if repo.refCalls != 1 {
		t.Fatalf("want IncrRefCount called once, got %d", repo.refCalls)
	}
	if repo.updated == nil {
		t.Fatal("zero-metadata asset must be backfilled via Update")
	}
	a := repo.updated
	if a.DurationSec != 5.945 {
		t.Fatalf("duration backfill: want 5.945, got %f", a.DurationSec)
	}
	if a.Width != 1320 || a.Height != 2868 {
		t.Fatalf("resolution backfill: want 1320x2868, got %dx%d", a.Width, a.Height)
	}
	if a.SizeBytes != 123456 {
		t.Fatalf("size backfill: want 123456, got %d", a.SizeBytes)
	}
	if a.MetaTitle != "实测标题" {
		t.Fatalf("title backfill: want 实测标题, got %q", a.MetaTitle)
	}
	if a.RefCount != 2 {
		t.Fatalf("ref_count should be 2 after increment, got %d", a.RefCount)
	}
	if st.removed != src {
		t.Fatalf("duplicate file should be removed, got %q", st.removed)
	}
}

func TestAssetDedupNoBackfillWhenMetadataPresent(t *testing.T) {
	dir := t.TempDir()
	src := writeDedupTestFile(t, dir, "dup2.mp4")
	existing := &Asset{
		ID:          "as_full",
		Fingerprint: "fp-y",
		LocalPath:   filepath.Join(dir, "b.mp4"),
		DurationSec: 12.0,
		Width:       1920,
		Height:      1080,
		SizeBytes:   999,
		MetaTitle:   "旧标题",
		RefCount:    3,
	}
	repo := &fakeAssetRepo{byFP: existing, refCount: 3}
	st := &fakeDedupStorage{}
	d := NewAssetDeduper(testConfig(), repo, st, log.DefaultLogger)

	res, err := d.Ingest(context.Background(), src, "file:///dup2.mp4", "新标题", 555, 1000, 640, 360)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Duplicate {
		t.Fatal("want duplicate=true")
	}
	if repo.updated != nil {
		t.Fatal("fully-populated asset must NOT be touched")
	}
	if res.Asset.MetaTitle != "旧标题" || res.Asset.Width != 1920 || res.Asset.DurationSec != 12.0 {
		t.Fatalf("existing real values must survive: %+v", res.Asset)
	}
	if st.removed != src {
		t.Fatalf("duplicate file should be removed, got %q", st.removed)
	}
}
