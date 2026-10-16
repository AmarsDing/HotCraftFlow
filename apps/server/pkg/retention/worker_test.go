package retention

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

func TestWorkerDisabledNoop(t *testing.T) {
	var runs atomic.Int32
	w := New(Config{
		Enabled:      false,
		Module:       "retention/test",
		Cron:         "* * * * *",
		RunOnStart:   true,
		Job:          func(ctx context.Context) { runs.Add(1) },
		DisabledHint: "test.retention.enabled=false",
	}, log.DefaultLogger)

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if runs.Load() != 0 {
		t.Fatal("disabled worker should not run job")
	}
}

func TestWorkerRunOnStart(t *testing.T) {
	var runs atomic.Int32
	w := New(Config{
		Enabled:    true,
		Module:     "retention/test",
		Cron:       "0 0 1 1 *", // 永远不会在最近触发
		RunOnStart: true,
		Job:        func(ctx context.Context) { runs.Add(1) },
	}, log.DefaultLogger)

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = w.Stop(context.Background()) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runs.Load() == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("RunOnStart should have executed job once")
}

func TestWorkerCronFires(t *testing.T) {
	var runs atomic.Int32
	w := New(Config{
		Enabled: true,
		Module:  "retention/test",
		Cron:    "@every 1s",
		Job:     func(ctx context.Context) { runs.Add(1) },
	}, log.DefaultLogger)

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = w.Stop(context.Background()) }()

	time.Sleep(1600 * time.Millisecond)
	if runs.Load() < 1 {
		t.Fatal("cron should have fired at least once")
	}
}

func TestWorkerInvalidCronDoesNotPanic(t *testing.T) {
	w := New(Config{
		Enabled: true,
		Module:  "retention/test",
		Cron:    "not-a-cron",
		Job:     func(ctx context.Context) {},
	}, log.DefaultLogger)
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("invalid cron should not fail start, got %v", err)
	}
	_ = w.Stop(context.Background())
}

func TestWorkerDefaultTimeout(t *testing.T) {
	w := New(Config{Enabled: true, Module: "retention/test", Cron: "0 0 * * *", Job: func(ctx context.Context) {}}, log.DefaultLogger)
	if w.cfg.Timeout != 30*time.Minute {
		t.Fatalf("default timeout should be 30m, got %s", w.cfg.Timeout)
	}
}

func TestCutoff(t *testing.T) {
	c := Cutoff(7)
	diff := time.Since(c)
	if diff < 7*24*time.Hour || diff > 7*24*time.Hour+time.Minute {
		t.Fatalf("cutoff should be ~7 days ago, got %s", diff)
	}
	if c.Location() != time.UTC {
		t.Fatal("cutoff should be UTC")
	}
}

func TestCutoffWall(t *testing.T) {
	if !CutoffWall(0).IsZero() {
		t.Fatal("CutoffWall(0) must be zero (no time filter)")
	}
	if !CutoffWall(-1).IsZero() {
		t.Fatal("CutoffWall(negative) must be zero")
	}
	c := CutoffWall(7)
	diff := time.Since(c)
	if diff < 7*24*time.Hour || diff > 7*24*time.Hour+time.Minute {
		t.Fatalf("CutoffWall(7) should be ~7 days ago, got %s", diff)
	}
	if c.Location() == time.UTC && time.Local != time.UTC {
		// 允许机器本身就是 UTC；非 UTC 机器应返回本地墙钟 Location
		t.Fatal("CutoffWall should use local wall clock, not force UTC")
	}
}
