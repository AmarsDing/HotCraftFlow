package exectree

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Pipes 流式子进程句柄：sidecar 这类"边写边读"的长连接协议使用
// （Run 是同步阻塞 API，协议驱动的 spawn 需要裸露的 stdin/stdout）。
//
// 纪律与 Run 一致：平台钩子 prepare/start/killTree；ctx 取消杀整树；
// Wait 幂等（多次调用返回同一结果）并在退出后回收平台资源
// （Windows job object handle）。
type Pipes struct {
	Stdin  io.WriteCloser
	Stdout io.Reader

	cmd      *exec.Cmd
	afterCtx func() bool // context.AfterFunc 的注销
	waitOnce sync.Once
	waitErr  error
}

// Kill 杀整树（幂等；进程已退出时为无害 no-op）。
func (p *Pipes) Kill() error {
	killTree(p.cmd)
	return nil
}

// Wait 等待进程退出（幂等）；随后回收平台资源并注销 ctx 看守。
func (p *Pipes) Wait() error {
	p.waitOnce.Do(func() {
		p.waitErr = p.cmd.Wait()
		if p.afterCtx != nil {
			p.afterCtx()
		}
		// 兜底：正常退出也杀掉可能遗漏的子孙进程/job object 句柄。
		killTree(p.cmd)
	})
	return p.waitErr
}

// Spawn 拉起流式子进程：
//   - 三条管道都在 start 之前创建（os/exec 约束）；
//   - stderr 逐行脱敏后回调（可为 nil 丢弃）；
//   - ctx 取消 → 杀整树（与 Run 同一平台钩子）；
//   - 不调 Wait 的资源释放由调用方负责（先读尽 Stdout 再 Wait）。
func Spawn(ctx context.Context, spec RunSpec, stderrLine func(string)) (*Pipes, error) {
	cmd := exec.Command(spec.Bin, spec.Args...)
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	// 进程退出后若仍有子孙握着管道，Wait 至多再等这些时防挂死。
	cmd.WaitDelay = 10 * time.Second

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	prepare(cmd)
	if err := start(cmd); err != nil {
		return nil, err
	}

	if stderrLine != nil {
		go func() {
			sc := bufio.NewScanner(stderr)
			sc.Buffer(make([]byte, 32*1024), 512*1024)
			for sc.Scan() {
				stderrLine(Redact(sc.Text()))
			}
		}()
	}

	p := &Pipes{Stdin: stdin, Stdout: stdout, cmd: cmd}
	p.afterCtx = context.AfterFunc(ctx, func() { killTree(cmd) })
	return p, nil
}
