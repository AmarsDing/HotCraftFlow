package retention

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/storage"
)

// ---------- fake Querier / Store ----------

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeExecQuerier struct {
	execs   []string
	execErr error
}

func (f *fakeExecQuerier) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.execs = append(f.execs, query)
	for _, a := range args {
		f.execs[len(f.execs)-1] += fmt.Sprintf("|%v", a)
	}
	if f.execErr != nil {
		return nil, f.execErr
	}
	return fakeResult{}, nil
}

// noopStore 记录 MoveLayer 调用但不真搬文件（T3 包级单测不碰文件系统）。
type noopStore struct {
	moved   []movedCall
	moveErr error
}

type movedCall struct {
	bucket string
	object string
	target storage.Layer
}

func (n *noopStore) Upload(context.Context, string, string, []byte) (string, error) { return "", nil }
func (n *noopStore) UploadStream(context.Context, string, string, io.Reader, int64) (string, error) {
	return "", nil
}
func (n *noopStore) Open(string) (*os.File, os.FileInfo, error)   { return nil, nil, os.ErrNotExist }
func (n *noopStore) Delete(context.Context, string) error          { return nil }
func (n *noopStore) Exists(context.Context, string) (bool, error) { return false, nil }
func (n *noopStore) List(context.Context, string, string, bool) ([]string, error) {
	return nil, nil
}
func (n *noopStore) MoveLayer(_ context.Context, bucket, object string, target storage.Layer) error {
	n.moved = append(n.moved, movedCall{bucket, object, target})
	return n.moveErr
}
func (n *noopStore) LocateLayer(context.Context, string, string) (storage.Layer, error) {
	return storage.LayerHot, nil
}

var _ storage.LayeredStorage = (*noopStore)(nil)

// ---------- DomainConfig Validate ----------

func TestDomainConfigValidate(t *testing.T) {
	ok := DomainConfig{Table: "metar_reports", TimeField: "obs_time", Stages: LegacyStages(30)}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid config should not error: %v", err)
	}
	badCases := []struct {
		name string
		d    DomainConfig
	}{
		{"empty", DomainConfig{}},
		{"bad table", DomainConfig{Table: "bad;table", TimeField: "t", Stages: LegacyStages(1)}},
		{"bad field", DomainConfig{Table: "ok", TimeField: "bad field", Stages: LegacyStages(1)}},
		{"empty stages", DomainConfig{Table: "ok", TimeField: "t", Stages: nil}},
		{"dup days", DomainConfig{Table: "ok", TimeField: "t", Stages: []StageConfig{{AfterDays: 7, Action: ActionHardDelete}, {AfterDays: 7, Action: ActionHardDelete}}}},
		{"desc days", DomainConfig{Table: "ok", TimeField: "t", Stages: []StageConfig{{AfterDays: 14, Action: ActionHardDelete}, {AfterDays: 7, Action: ActionHardDelete}}}},
		{"archive", DomainConfig{Table: "ok", TimeField: "t", Stages: []StageConfig{{AfterDays: 1, Action: ActionArchive}}}},
		{"zero days", DomainConfig{Table: "ok", TimeField: "t", Stages: []StageConfig{{AfterDays: 0, Action: ActionHardDelete}}}},
		{"invalid action", DomainConfig{Table: "ok", TimeField: "t", Stages: []StageConfig{{AfterDays: 1, Action: "nope"}}}},
	}
	for _, c := range badCases {
		if err := c.d.Validate(); err == nil {
			t.Errorf("%s: should reject", c.name)
		}
	}
}

func TestLegacyStages(t *testing.T) {
	s := LegacyStages(30)
	if len(s) != 1 || s[0].AfterDays != 30 || s[0].Action != ActionHardDelete {
		t.Fatalf("LegacyStages(30) = %+v", s)
	}
}

// ---------- Executor ----------

func TestExecutorRun(t *testing.T) {
	var applied []string
	h := map[Action]Handler{
		ActionHardDelete: HandlerFunc(func(_ context.Context, stage StageConfig, cutoff time.Time) error {
			applied = append(applied, fmt.Sprintf("hd:%d", stage.AfterDays))
			return nil
		}),
		ActionDowngrade: HandlerFunc(func(_ context.Context, stage StageConfig, cutoff time.Time) error {
			applied = append(applied, fmt.Sprintf("dg:%d", stage.AfterDays))
			return nil
		}),
	}
	e, err := NewExecutor(h, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	d := DomainConfig{
		Table: "t", TimeField: "tf",
		Stages: []StageConfig{
			{AfterDays: 7, Action: ActionDowngrade, Params: map[string]any{"target_layer": "warm"}},
			{AfterDays: 30, Action: ActionHardDelete},
		},
	}
	if err := e.Run(context.Background(), d); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"dg:7", "hd:30"}
	if len(applied) != len(want) {
		t.Fatalf("applied=%v, want=%v", applied, want)
	}
	for i := range want {
		if applied[i] != want[i] {
			t.Errorf("applied[%d]=%q, want %q", i, applied[i], want[i])
		}
	}
}

func TestExecutorMissingHandler(t *testing.T) {
	// 提供 downgrade handler 使 Executor 非空，但 domain 用 hard_delete（无 handler）。
	e, err := NewExecutor(map[Action]Handler{
		ActionDowngrade: HandlerFunc(func(context.Context, StageConfig, time.Time) error { return nil }),
	}, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	d := DomainConfig{Table: "t", TimeField: "tf", Stages: LegacyStages(7)}
	if err := e.Run(context.Background(), d); err == nil {
		t.Error("missing handler should error")
	}
}

func TestExecutorNoHandlers(t *testing.T) {
	if _, err := NewExecutor(map[Action]Handler{}, log.DefaultLogger); err == nil {
		t.Error("empty handlers should reject")
	}
}

func TestExecutorContinueOnStageError(t *testing.T) {
	h := map[Action]Handler{
		ActionHardDelete: HandlerFunc(func(_ context.Context, stage StageConfig, _ time.Time) error {
			if stage.AfterDays == 7 {
				return errors.New("stage7 boom")
			}
			return nil
		}),
	}
	e, _ := NewExecutor(h, log.DefaultLogger)
	d := DomainConfig{Table: "t", TimeField: "tf", Stages: []StageConfig{
		{AfterDays: 7, Action: ActionHardDelete},
		{AfterDays: 30, Action: ActionHardDelete},
	}}
	if err := e.Run(context.Background(), d); err == nil {
		t.Error("partial failure should return error")
	}
}

// ---------- HardDeleteHandler ----------

func TestHardDeleteHandler(t *testing.T) {
	fq := &fakeExecQuerier{}
	h, err := NewHardDeleteHandler(fq, "metar_reports", "obs_time", log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if err := h.Apply(context.Background(), StageConfig{}, cutoff); err != nil {
		t.Fatal(err)
	}
	if len(fq.execs) != 1 {
		t.Fatalf("execs=%v", fq.execs)
	}
	wantSub := []string{"DELETE FROM metar_reports", "obs_time < $1", cutoff.Format("2006-01-02 15:04:05")}
	for _, w := range wantSub {
		if !strings.Contains(fq.execs[0], w) {
			t.Errorf("DDL 缺 %q\n实际: %s", w, fq.execs[0])
		}
	}
}

func TestHardDeleteHandlerUnsafe(t *testing.T) {
	fq := &fakeExecQuerier{}
	if _, err := NewHardDeleteHandler(fq, "bad;drop", "obs_time", log.DefaultLogger); err == nil {
		t.Error("unsafe table should reject")
	}
	if _, err := NewHardDeleteHandler(fq, "ok", "bad field", log.DefaultLogger); err == nil {
		t.Error("unsafe field should reject")
	}
	if _, err := NewHardDeleteHandler(nil, "ok", "ok", log.DefaultLogger); err == nil {
		t.Error("nil db should reject")
	}
}

// ---------- DowngradeHandler ----------

func TestDowngradeHandler(t *testing.T) {
	store := &noopStore{}
	var listed bool
	lister := func(_ context.Context, cutoff time.Time, limit int) ([]ObjectRef, error) {
		listed = true
		return []ObjectRef{
			{Bucket: "radar", Object: "SINGLE/20260801/m.bin"},
			{Bucket: "radar", Object: "SINGLE/20260801/a.bin"},
		}, nil
	}
	h, err := NewDowngradeHandler(store, lister, 100, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	stage := StageConfig{AfterDays: 7, Action: ActionDowngrade, Params: map[string]any{"target_layer": "warm"}}
	if err := h.Apply(context.Background(), stage, cutoff); err != nil {
		t.Fatal(err)
	}
	if !listed {
		t.Error("lister should be called")
	}
	if len(store.moved) != 2 || store.moved[0].target != storage.LayerWarm {
		t.Fatalf("moved=%v", store.moved)
	}
}

func TestDowngradeHandlerMissingTarget(t *testing.T) {
	h, _ := NewDowngradeHandler(&noopStore{}, func(context.Context, time.Time, int) ([]ObjectRef, error) {
		return nil, nil
	}, 100, log.DefaultLogger)
	if err := h.Apply(context.Background(), StageConfig{Params: map[string]any{}}, time.Now()); err == nil {
		t.Error("missing target_layer should error")
	}
}

func TestDowngradeHandlerMoveError(t *testing.T) {
	store := &noopStore{moveErr: errors.New("boom")}
	h, _ := NewDowngradeHandler(store, func(context.Context, time.Time, int) ([]ObjectRef, error) {
		return []ObjectRef{{Bucket: "a", Object: "b"}}, nil
	}, 100, log.DefaultLogger)
	stage := StageConfig{Params: map[string]any{"target_layer": "warm"}}
	if err := h.Apply(context.Background(), stage, time.Now()); err == nil {
		t.Error("move error should propagate")
	}
}

func TestDowngradeHandlerNilArgs(t *testing.T) {
	if _, err := NewDowngradeHandler(nil, func(context.Context, time.Time, int) ([]ObjectRef, error) { return nil, nil }, 0, log.DefaultLogger); err == nil {
		t.Error("nil store should reject")
	}
	if _, err := NewDowngradeHandler(&noopStore{}, nil, 0, log.DefaultLogger); err == nil {
		t.Error("nil lister should reject")
	}
}

// ---------- AggregateHandler ----------

func TestAggregateHandler(t *testing.T) {
	var called bool
	fn := func(_ context.Context, cutoff time.Time, params map[string]any) error {
		called = true
		if params["granularity"] != "hourly" {
			return errors.New("bad param")
		}
		return nil
	}
	h, err := NewAggregateHandler(fn)
	if err != nil {
		t.Fatal(err)
	}
	stage := StageConfig{AfterDays: 7, Action: ActionAggregate, Params: map[string]any{"granularity": "hourly"}}
	if err := h.Apply(context.Background(), stage, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("aggregate func should be called")
	}
}

func TestAggregateHandlerNil(t *testing.T) {
	if _, err := NewAggregateHandler(nil); err == nil {
		t.Error("nil func should reject")
	}
}

// ---------- Worker backward compatibility ----------

func TestWorkerWithLegacyStages(t *testing.T) {
	var ran bool
	cfg := Config{
		Enabled: true, Cron: "* * * * *",
		Job: func(ctx context.Context) { ran = true },
	}
	w := New(cfg, log.DefaultLogger)
	if w.cfg.Job == nil {
		t.Fatal("legacy Job should be preserved")
	}
	w.cfg.Job(context.Background())
	if !ran {
		t.Error("legacy Job should run")
	}
}

func TestWorkerAutoJobFromDomain(t *testing.T) {
	var applied []string
	handlers := map[Action]Handler{
		ActionHardDelete: HandlerFunc(func(_ context.Context, stage StageConfig, _ time.Time) error {
			applied = append(applied, fmt.Sprintf("hd%d", stage.AfterDays))
			return nil
		}),
	}
	exec, _ := NewExecutor(handlers, log.DefaultLogger)
	cfg := Config{
		Enabled: true, Cron: "* * * * *",
		Domain: &DomainConfig{Table: "metar", TimeField: "obs_time", Stages: LegacyStages(30)},
		Executor: exec,
	}
	w := New(cfg, log.DefaultLogger)
	if w.cfg.Job == nil {
		t.Fatal("Job should be auto-wired from Domain+Executor")
	}
	w.cfg.Job(context.Background())
	if len(applied) != 1 || applied[0] != "hd30" {
		t.Errorf("applied=%v", applied)
	}
}

func TestWorkerStartNoJob(t *testing.T) {
	cfg := Config{Enabled: true, Cron: "* * * * *"}
	w := New(cfg, log.DefaultLogger)
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// should not panic; Job nil means Start logs error and returns nil
}

// ---------- Downgrade 集成测试（真实文件系统 + 真实分层存储） ----------
// 验收口径：文件从 _hot 移到 _warm，storage_layers（此处以内存 resolver 模拟）同步更新，
// 且 DB file_path 不变仍可读取（Open 走 resolver 定位层，调用方无感）。

type mapLayeredCfg struct{ roots map[storage.Layer]string }

func (m mapLayeredCfg) GetLayerRoot(l storage.Layer) string { return m.roots[l] }

// memLayerResolver 内存版 LayerResolver，模拟 storage_layers 表 UPSERT/Update 行为。
type memLayerResolver struct {
	mu sync.Mutex
	m  map[string]storage.Layer
}

func newMemLayerResolver() *memLayerResolver { return &memLayerResolver{m: map[string]storage.Layer{}} }

func (r *memLayerResolver) key(bucket, object string) string { return bucket + "/" + object }

func (r *memLayerResolver) Locate(_ context.Context, bucket, object string) (storage.Layer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.m[r.key(bucket, object)]; ok {
		return l, nil
	}
	return storage.LayerHot, nil
}

func (r *memLayerResolver) Record(_ context.Context, bucket, object string, layer storage.Layer, _ int64, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[r.key(bucket, object)] = layer
	return nil
}

func (r *memLayerResolver) UpdateLayer(_ context.Context, bucket, object string, layer storage.Layer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[r.key(bucket, object)] = layer
	return nil
}

func (r *memLayerResolver) Remove(_ context.Context, bucket, object string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, r.key(bucket, object))
	return nil
}

func TestDowngradeIntegration_HotToWarm(t *testing.T) {
	ctx := context.Background()
	hotRoot, warmRoot := t.TempDir(), t.TempDir()
	resolver := newMemLayerResolver()
	store := storage.NewLayeredLocalStorage(
		mapLayeredCfg{roots: map[storage.Layer]string{storage.LayerHot: hotRoot, storage.LayerWarm: warmRoot}},
		resolver, log.DefaultLogger)
	if store == nil {
		t.Fatal("layered storage init failed")
	}

	// 1) 上传两个对象到 hot 层。
	objs := []string{"SINGLE/20260801/m.bin", "SINGLE/20260801/a.bin"}
	for _, o := range objs {
		if _, err := store.Upload(ctx, "radar", o, []byte("payload-"+o)); err != nil {
			t.Fatalf("upload %s: %v", o, err)
		}
		if l, _ := resolver.Locate(ctx, "radar", o); l != storage.LayerHot {
			t.Fatalf("%s should be hot after upload, got %s", o, l)
		}
	}

	// 2) lister 模拟业务 SQL：返回 cutoff 之前的待降级清单。
	lister := func(context.Context, time.Time, int) ([]ObjectRef, error) {
		return []ObjectRef{{Bucket: "radar", Object: objs[0]}, {Bucket: "radar", Object: objs[1]}}, nil
	}
	h, err := NewDowngradeHandler(store, lister, 100, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	stage := StageConfig{AfterDays: 7, Action: ActionDowngrade, Params: map[string]any{"target_layer": "warm"}}
	if err := h.Apply(ctx, stage, time.Now().Add(-7*24*time.Hour)); err != nil {
		t.Fatalf("downgrade apply: %v", err)
	}

	// 3) 文件物理迁移：hot 消失、warm 出现。
	for _, o := range objs {
		if _, err := os.Stat(filepath.Join(hotRoot, "radar", o)); !os.IsNotExist(err) {
			t.Errorf("%s should be gone from hot, stat err=%v", o, err)
		}
		if _, err := os.Stat(filepath.Join(warmRoot, "radar", o)); err != nil {
			t.Errorf("%s should exist in warm: %v", o, err)
		}
		// 4) 元数据同步：LocateLayer 返回 warm。
		if l, _ := store.LocateLayer(ctx, "radar", o); l != storage.LayerWarm {
			t.Errorf("%s layer = %s, want warm", o, l)
		}
		// 5) file_path 不变仍可读（Open 按 resolver 定位 warm 层）。
		f, _, err := store.Open("radar/" + o)
		if err != nil {
			t.Fatalf("Open %s after downgrade: %v", o, err)
		}
		data, _ := io.ReadAll(f)
		_ = f.Close()
		if string(data) != "payload-"+o {
			t.Errorf("%s content = %q", o, data)
		}
	}
}
