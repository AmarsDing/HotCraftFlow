//go:build windows

package exectree

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

// TestProbeAssignToJob 诊断：本环境下 AssignProcessToJobObject 是否成功。
// start() 会把 Assign 失败静默吞掉，killTree 随之退化为只杀主进程——
// 本探针把吞掉的错误拿到明面，供 Windows 树杀失效排查用。
func TestProbeAssignToJob(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run", "^TestHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"=1", helperCmd+"=sleep")
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	job, err := newTreeKillJob()
	if err != nil {
		t.Fatalf("CreateJobObject: %v", err)
	}
	defer windows.CloseHandle(job)

	hProc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatalf("OpenProcess: %v", err)
	}
	defer windows.CloseHandle(hProc)

	if err := windows.AssignProcessToJobObject(job, hProc); err != nil {
		// Assign 失败 = 树杀退化为单杀主进程的环境（CI/托管 shell 常见：
		// 外层已挂 Job 且禁嵌套）。实现需走 taskkill /T 兜底。
		t.Errorf("AssignProcessToJobObject 失败: %v（本环境树杀需 taskkill /T 兜底）", err)
	}
}
