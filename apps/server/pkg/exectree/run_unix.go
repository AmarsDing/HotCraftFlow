//go:build !windows

package exectree

import (
	"os/exec"
	"syscall"
)

// Unix 进程树受控执行：setpgid 立新进程组，killTree 对整个进程组发 SIGKILL，
// 孙进程持有的管道写句柄随之内核回收（调研报告 §contained_subprocess 的教训）。

func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func start(cmd *exec.Cmd) error { return cmd.Start() }

func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// 负 pid = 杀进程组；setpgid 后主进程 pid 即组 id。
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
