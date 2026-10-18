//go:build windows

package exectree

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 进程树受控执行：
//   - prepare：CREATE_SUSPENDED 挂起态拉起主进程，防止它抢在挂 Job 之前
//     繁衍孙进程导致管道写句柄被占、Run 永远等不到 EOF（调研报告
//     §contained_subprocess 的教训）。
//   - start：挂进 JobObject（仅 KILL_ON_JOB_CLOSE；不开 BREAKAWAY）后
//     ResumeThread。孙进程由嵌套 job 内核托管。
//   - killTree：TerminateJobObject 一次杀干净整树，不落 taskkill。
// Job 句柄挂在 cmdJobs 里，随 killTree/进程退出释放。

var cmdJobs sync.Map // *exec.Cmd -> windows.Handle(JobObject)

func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	cmd.SysProcAttr.HideWindow = true
}

// start 拉起（挂起态）主进程 → 挂 JobObject → ResumeThread。
// 任一中间步骤失败都尽力杀进程后再报 SpawnErr。
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	job, err := newTreeKillJob()
	if err == nil {
		hProc, errO := windows.OpenProcess(
			windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if errO == nil {
			if errA := windows.AssignProcessToJobObject(job, hProc); errA == nil {
				cmdJobs.Store(cmd, job)
			} else {
				_ = windows.CloseHandle(job)
			}
			_ = windows.CloseHandle(hProc)
		} else {
			_ = windows.CloseHandle(job)
		}
	}
	// 无论 Job 挂没挂上都要 Resume，否则挂起态主进程把 Run 卡死。
	if errR := resumeMainThread(uint32(cmd.Process.Pid)); errR != nil {
		_ = cmd.Process.Kill()
		releaseJob(cmd)
		return errR
	}
	return nil
}

// killTree TerminateJobObject 整树杀；Job 没挂成时退化为杀主进程。
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if job, ok := cmdJobs.LoadAndDelete(cmd); ok {
		h := job.(windows.Handle)
		_ = windows.TerminateJobObject(h, 1)
		_ = windows.CloseHandle(h)
		return
	}
	_ = cmd.Process.Kill()
}

func releaseJob(cmd *exec.Cmd) {
	if job, ok := cmdJobs.LoadAndDelete(cmd); ok {
		_ = windows.CloseHandle(job.(windows.Handle))
	}
}

// newTreeKillJob KILL_ON_JOB_CLOSE：句柄关闭/Terminate 时整树被杀。
//
// 只挂 KILL_ON_JOB_CLOSE，绝不带 BREAKAWAY_OK / SILENT_BREAKAWAY_OK：
// 这两个标志（尤其 SILENT 变体）允许 job 成员创建的子进程自动脱离 job
// 托管——任何引擎只要 spawn 一个孙进程就能逃逸树杀，容器纪律形同虚设。
func newTreeKillJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags =
		windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// resumeMainThread 找 owner==pid 的第一条线程 Resume（CREATE_SUSPENDED 只挂主线程）。
func resumeMainThread(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)
	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))
	if err := windows.Thread32First(snap, &te); err != nil {
		return err
	}
	for {
		if te.OwnerProcessID == pid {
			hThread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
			if err != nil {
				return err
			}
			defer windows.CloseHandle(hThread)
			_, err = windows.ResumeThread(hThread)
			return err
		}
		if err := windows.Thread32Next(snap, &te); err != nil {
			return err
		}
	}
}
