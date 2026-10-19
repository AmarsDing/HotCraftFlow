package mme

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// ---- doctor 测试 fakes（只实现端口方法集，状态用字段开关）----

type fakeDoctorTaskRepo struct{}

func (fakeDoctorTaskRepo) Create(context.Context, *Task) error              { return nil }
func (fakeDoctorTaskRepo) Update(context.Context, *Task) error              { return nil }
func (fakeDoctorTaskRepo) Get(context.Context, string) (*Task, error)       { return nil, nil }
func (fakeDoctorTaskRepo) List(context.Context, TaskQuery) ([]*Task, int, error) {
	return nil, 0, nil
}
func (fakeDoctorTaskRepo) Delete(context.Context, string) error { return nil }

type fakeDoctorAssetRepo struct{}

func (fakeDoctorAssetRepo) Create(context.Context, *Asset) error            { return nil }
func (fakeDoctorAssetRepo) Update(context.Context, *Asset) error            { return nil }
func (fakeDoctorAssetRepo) Get(context.Context, string) (*Asset, error)     { return nil, nil }
func (fakeDoctorAssetRepo) GetByFingerprint(context.Context, string) (*Asset, error) {
	return nil, nil
}
func (fakeDoctorAssetRepo) IncrRefCount(context.Context, string) (int, error) { return 0, nil }

type fakeDoctorCacheRepo struct{}

func (fakeDoctorCacheRepo) Get(context.Context, string) (*CacheEntry, error) { return nil, nil }
func (fakeDoctorCacheRepo) Upsert(context.Context, *CacheEntry) error        { return nil }
func (fakeDoctorCacheRepo) IncrHit(context.Context, string) error            { return nil }
func (fakeDoctorCacheRepo) TotalBytes(context.Context) (int64, error)        { return 0, nil }

type fakeDoctorStorage struct{ mediaDir string }

func (f fakeDoctorStorage) MediaDir() string                 { return f.mediaDir }
func (f fakeDoctorStorage) AssetDir(string) string           { return "" }
func (f fakeDoctorStorage) VariantDir(string) string         { return "" }
func (f fakeDoctorStorage) WorkDir(string) string            { return "" }
func (fakeDoctorStorage) IngestFile(context.Context, string, string) (string, int64, error) {
	return "", 0, nil
}
func (fakeDoctorStorage) ExportZip(context.Context, string, []*Variant) (string, error) {
	return "", nil
}
func (fakeDoctorStorage) Remove(context.Context, string) error { return nil }

type fakeDoctorEnqueuer struct{}

func (fakeDoctorEnqueuer) EnqueueDownload(context.Context, string) error { return nil }
func (fakeDoctorEnqueuer) EnqueueAnalyze(context.Context, string) error  { return nil }
func (fakeDoctorEnqueuer) EnqueueRender(context.Context, string) error   { return nil }

type fakeDoctorDownloader struct{ ok bool }

func (f fakeDoctorDownloader) Available(context.Context) bool { return f.ok }
func (f fakeDoctorDownloader) Download(context.Context, string, string) (*DownloadResult, error) {
	return nil, nil
}

type fakeDoctorProber struct{ ok bool }

func (f fakeDoctorProber) Available(context.Context) bool { return f.ok }
func (f fakeDoctorProber) Probe(context.Context, string) (*ProbeResult, error) {
	return nil, nil
}

type fakeDoctorASR struct{ ok bool }

func (f fakeDoctorASR) Available(context.Context) bool { return f.ok }
func (f fakeDoctorASR) Transcribe(context.Context, string, string) (*TranscriptResult, error) {
	return nil, nil
}

type fakeDoctorLLM struct{ ok bool }

func (f fakeDoctorLLM) Available(context.Context) bool { return f.ok }
func (f fakeDoctorLLM) Generate(context.Context, string) (string, error) {
	return "", nil
}

type fakeDoctorRenderer struct{ ok bool }

func (f fakeDoctorRenderer) Available(context.Context) bool          { return f.ok }
func (f fakeDoctorRenderer) Version(context.Context) string          { return "" }
func (f fakeDoctorRenderer) Render(context.Context, RenderSpec, func(float64)) (*RenderResult, error) {
	return nil, nil
}

// newDoctorTestUsecase 组装最小 TaskUsecase（仅 doctor 路径）。
func newDoctorTestUsecase(cfg Config, llmOK bool) *TaskUsecase {
	return NewTaskUsecase(
		cfg,
		fakeDoctorTaskRepo{},
		fakeDoctorAssetRepo{},
		NewAssetDeduper(cfg, fakeDoctorAssetRepo{}, fakeDoctorStorage{}, log.DefaultLogger),
		NewAnalysisCache(cfg, fakeDoctorCacheRepo{}),
		fakeDoctorStorage{mediaDir: cfg.MediaDir},
		fakeDoctorEnqueuer{},
		fakeDoctorDownloader{ok: true},
		fakeDoctorProber{ok: true},
		fakeDoctorASR{ok: true},
		fakeDoctorLLM{ok: llmOK},
		fakeDoctorRenderer{ok: true},
		log.DefaultLogger,
	)
}

func doctorItemByName(rep *DoctorReport, name string) *DoctorItem {
	for i := range rep.Items {
		if rep.Items[i].Name == name {
			return &rep.Items[i]
		}
	}
	return nil
}

// TestDoctorAllGreen：依赖齐备 + 可写 → 全绿。
func TestDoctorAllGreen(t *testing.T) {
	cfg := Config{Enabled: true, MediaDir: t.TempDir(), CacheEnabled: true}
	u := newDoctorTestUsecase(cfg, true)
	rep, err := u.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("expected OK report, items=%+v", rep.Items)
	}
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "whisper", "llm/ollama", "media_dir", "storage_writable", "cache"} {
		it := doctorItemByName(rep, name)
		if it == nil {
			t.Fatalf("missing doctor item %q", name)
		}
		if !it.OK {
			t.Fatalf("item %q not ok: %+v", name, *it)
		}
	}
}

// TestDoctorLLMDownOptionalRed：LLM 缺失 → 单项红但报告不红（optional）。
func TestDoctorLLMDownOptionalRed(t *testing.T) {
	cfg := Config{Enabled: true, MediaDir: t.TempDir()}
	u := newDoctorTestUsecase(cfg, false)
	rep, err := u.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	it := doctorItemByName(rep, "llm/ollama")
	if it == nil || it.OK {
		t.Fatalf("llm item should be present and red: %+v", it)
	}
	if it.Required {
		t.Fatalf("llm item must stay optional: %+v", *it)
	}
	if !rep.OK {
		t.Fatalf("optional llm red must not fail report: %+v", rep.Items)
	}
	if !strings.Contains(it.Detail, "copy_degraded") {
		t.Fatalf("llm detail should mention copy_degraded fallback: %q", it.Detail)
	}
}

// TestDoctorCookiesConfiguredMissing：配置了 cookies 但文件缺失 → 单项红（optional），报告不红。
func TestDoctorCookiesConfiguredMissing(t *testing.T) {
	cfg := Config{Enabled: true, MediaDir: t.TempDir(), CookiesPath: t.TempDir() + "/no-such-cookies.txt"}
	u := newDoctorTestUsecase(cfg, true)
	rep, err := u.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	it := doctorItemByName(rep, "cookies")
	if it == nil || it.OK {
		t.Fatalf("cookies item should be present and red: %+v", it)
	}
	if !rep.OK {
		t.Fatalf("optional cookies red must not fail report")
	}
}

// TestDoctorCookiesAbsentWhenUnconfigured：未配置 cookies → 不出现该项（防噪音）。
func TestDoctorCookiesAbsentWhenUnconfigured(t *testing.T) {
	cfg := Config{Enabled: true, MediaDir: t.TempDir()}
	u := newDoctorTestUsecase(cfg, true)
	rep, err := u.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doctorItemByName(rep, "cookies") != nil {
		t.Fatalf("cookies item must be absent when unconfigured")
	}
}

// TestDoctorStorageNotWritable：media_dir 被同名文件占据 → 写探针红 + 报告红（required）。
func TestDoctorStorageNotWritable(t *testing.T) {
	dir := t.TempDir()
	blocked := dir + "/blocked-media"
	if err := os.WriteFile(blocked, []byte("i am a file, not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Enabled: true, MediaDir: blocked}
	u := newDoctorTestUsecase(cfg, true)
	rep, err := u.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	it := doctorItemByName(rep, "storage_writable")
	if it == nil || it.OK {
		t.Fatalf("storage_writable should be red when media dir blocked: %+v", it)
	}
	if rep.OK {
		t.Fatalf("required storage_writable red must fail report")
	}
}
