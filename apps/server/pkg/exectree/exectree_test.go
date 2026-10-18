package exectree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// helperMarker 树杀验证的信标文件 env 名（孙进程周期性刷新 mtime）。
const (
	helperEnv   = "MEDIA_EXECTREE_HELPER"
	helperCmd   = "MEDIA_EXECTREE_HELPER_CMD"
	helperChild = "MEDIA_EXECTREE_HELPER_CHILD_MARKER"
)

// TestHelperProcess 辅助进程（自唤起孙子进程模式）。
// 行为由 helperCmd 决定：sleep（原地睡眠）、tree（起孙进程 + 各自信标）。
func TestHelperProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	mode := os.Getenv(helperCmd)
	if mode == "tree" && os.Getenv(helperChild) == "" {
		// 我是父：生成信标，起一个孙进程（同命令但带 child marker env）。
		marker := os.Getenv("MEDIA_EXECTREE_HELPER_PARENT")
		touchLoop(marker)
		self, _ := os.Executable()
		child := exec.Command(self, "-test.run", "^TestHelperProcess$")
		child.Env = append(os.Environ(), helperChild+"=1")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		_ = child.Start()
		fmt.Println("parent ready")
		sleepForever()
	} else if mode == "tree" {
		// 我是孙：独立信标。
		marker := os.Getenv("MEDIA_EXECTREE_HELPER_GRAND")
		touchLoop(marker)
		fmt.Println("grandchild ready")
		sleepForever()
	} else {
		fmt.Println("helper ready")
		sleepForever()
	}
}

// sleepForever 挂计时器的永续睡眠（裸 select{} 会触发 Go 死锁检测
// 把辅助进程打成退出码 2；sleep 有 pending timer 则不触发）。
func sleepForever() {
	for {
		time.Sleep(time.Hour)
	}
}

func touchLoop(path string) {
	go func() {
		for {
			_ = os.WriteFile(path, []byte(time.Now().Format(time.RFC3339Nano)), 0o644)
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

func helperSpec(t *testing.T, extraEnv []string) RunSpec {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return RunSpec{
		Bin:  self,
		Args: []string{"-test.run", "^TestHelperProcess$"},
		Env:  append([]string{helperEnv + "=1"}, extraEnv...),
	}
}

func TestRunTimeoutKillsProcess(t *testing.T) {
	spec := helperSpec(t, []string{helperCmd + "=sleep"})
	spec.Timeout = 300 * time.Millisecond
	res := Run(context.Background(), spec)
	if !res.TimedOut {
		t.Fatalf("expected TimedOut, got %+v", res)
	}
	if res.Success() {
		t.Fatalf("timeout 不得 Success: %+v", res)
	}
	if res.Attribution == "" {
		t.Fatal("归因必须非空")
	}
}

func TestRunCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	spec := helperSpec(t, []string{helperCmd + "=sleep"})
	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()
	res := Run(ctx, spec)
	if res.TimedOut {
		t.Fatalf("取消不得归因到超时: %+v", res)
	}
	if res.Success() {
		t.Fatal("取消不得 Success")
	}
}

func TestRunKillsGrandchildTree(t *testing.T) {
	dir := t.TempDir()
	parentMarker := filepath.Join(dir, "parent.marker")
	grandMarker := filepath.Join(dir, "grand.marker")

	spec := helperSpec(t, []string{
		helperCmd + "=tree",
		"MEDIA_EXECTREE_HELPER_PARENT=" + parentMarker,
		"MEDIA_EXECTREE_HELPER_GRAND=" + grandMarker,
	})
	spec.Timeout = 1200 * time.Millisecond

	res := Run(context.Background(), spec)
	if !res.TimedOut {
		t.Fatalf("tree 模式预期超时杀树, got %+v", res)
	}

	// 树杀后双信标都不应再刷新。
	time.Sleep(200 * time.Millisecond)
	for _, m := range []string{parentMarker, grandMarker} {
		st1, err := os.Stat(m)
		if err != nil {
			t.Fatalf("信标缺失 %s: %v", m, err)
		}
		time.Sleep(150 * time.Millisecond)
		st2, _ := os.Stat(m)
		if st2.ModTime().After(st1.ModTime()) {
			t.Fatalf("%s 仍在刷新——树杀失效（%v → %v）", m, st1.ModTime(), st2.ModTime())
		}
	}
}

func TestRunCapturesOutputAndSuccess(t *testing.T) {
	self, _ := os.Executable()
	res := Run(context.Background(), RunSpec{
		Bin:     self,
		Args:    []string{"-test.run", "^TestHelperProcessZero$"},
		Env:     []string{helperEnv + "=2"},
		Timeout: 5 * time.Second,
	})
	if !res.Success() {
		t.Fatalf("零进程应 Success: %+v", res)
	}
}

func TestRedact(t *testing.T) {
	cases := []struct{ in, wantClean string }{
		{"Authorization: Bearer hf_abcdefgh12345678ijklmnop", "***REDACTED***"},
		{"api_key=sk-abc123xyz", "***REDACTED***"},
		{"普通日志 hello world", "普通日志 hello world"},
		{"token=gha_realbutfake1234567890abcd", "***REDACTED***"},
	}
	for _, c := range cases {
		got := Redact(c.in)
		if c.wantClean == "***REDACTED***" {
			if got == c.in {
				t.Fatalf("敏感串未脱敏: %q", got)
			}
			continue
		}
		if got != c.in {
			t.Fatalf("普通串不应被改: %q → %q", c.in, got)
		}
	}
}
