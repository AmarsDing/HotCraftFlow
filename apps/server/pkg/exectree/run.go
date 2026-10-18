// Package exectree 提供"受容器的子进程"执行：取消/超时杀整棵进程树、
// 输出行级采集+脱敏、退出归因（exit code / timeout / cancelled / spawn 失败）。
//
// 纪律：业务代码禁止裸 exec.Command——任何外部可执行文件（ffmpeg/yt-dlp/
// Whisper sidecar 等）都必须走这里，否则挂掉的孙进程会占着管道写句柄，
// 父进程永远等不到 EOF（调研报告 §contained_subprocess 的教训）。
package exectree

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RunSpec 一次受控执行。
type RunSpec struct {
	Bin     string        // 可执行文件（建议先 exec.LookPath 校验）
	Args    []string
	Dir     string        // 工作目录（空=继承）
	Env     []string      // 进程环境（非空时整体替换；需继承请显式 os.Environ() 拼接）
	Timeout time.Duration // 0=不限时（业务侧仍应设置看门狗）
	// OutputLines stdout/stderr 各保留的末尾行数（0 用默认 200）。
	OutputLines int
	// Stdin 非空时在启动后写入并关闭 stdin（长连接管道协议不走这里）。
	Stdin string
}

// Result 执行结果。Success = !TimedOut && !Cancelled && ExitCode==0 && SpawnErr==nil。
type Result struct {
	ExitCode    int // 未成功 spawn 时为 -1
	TimedOut    bool
	Cancelled   bool
	SpawnErr    error
	Duration    time.Duration
	StdoutTail  string // 已脱敏的末 N 行
	StderrTail  string
	Attribution string // 一句话归因（日志/事件载荷直接用）
}

// Success 结果判定。
func (r Result) Success() bool {
	return r.SpawnErr == nil && !r.TimedOut && !r.Cancelled && r.ExitCode == 0
}

const defaultOutputLines = 200

// Run 同步执行至结束/超时/取消。ctx 取消与 Timeout 都会杀整树后返回。
func Run(ctx context.Context, spec RunSpec) Result {
	startAt := time.Now()
	res := Result{ExitCode: -1}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if spec.Timeout > 0 {
		var tCancel context.CancelFunc
		runCtx, tCancel = context.WithTimeout(runCtx, spec.Timeout)
		defer tCancel()
	}

	cmd := exec.Command(spec.Bin, spec.Args...)
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	}
	prepare(cmd) // 平台文件：setpgid / CREATE_NEW_PROCESS_GROUP

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		res.SpawnErr = err
		res.Attribution = fmt.Sprintf("stdout pipe 失败: %v", err)
		res.Duration = time.Since(startAt)
		return res
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		res.SpawnErr = err
		res.Attribution = fmt.Sprintf("stderr pipe 失败: %v", err)
		res.Duration = time.Since(startAt)
		return res
	}
	// StdinPipe 必须在 Start 之前创建（os/exec 约束）。
	var stdinW io.WriteCloser
	if spec.Stdin != "" {
		stdinW, err = cmd.StdinPipe()
		if err != nil {
			res.SpawnErr = err
			res.Attribution = fmt.Sprintf("stdin pipe 失败: %v", err)
			res.Duration = time.Since(startAt)
			return res
		}
	}

	if err := start(cmd); err != nil {
		res.SpawnErr = err
		res.Attribution = fmt.Sprintf("启动失败: %v", err)
		res.Duration = time.Since(startAt)
		return res
	}

	keep := spec.OutputLines
	if keep <= 0 {
		keep = defaultOutputLines
	}
	outRing, errRing := newLineRing(keep), newLineRing(keep)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pumpLines(stdout, outRing) }()
	go func() { defer wg.Done(); pumpLines(stderr, errRing) }()

	if stdinW != nil {
		go func() {
			_, _ = io.WriteString(stdinW, spec.Stdin)
			_ = stdinW.Close()
		}()
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case waitErr := <-waitCh:
		res = fillExit(res, waitErr)
	case <-runCtx.Done():
		killTree(cmd)
		waitErr := <-waitCh
		res = fillExit(res, waitErr)
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			res.TimedOut = true
			res.Attribution = fmt.Sprintf("超时 %s，已杀进程树（exit=%d）", spec.Timeout, res.ExitCode)
		} else {
			res.Cancelled = true
			res.Attribution = fmt.Sprintf("已取消，杀进程树（exit=%d）", res.ExitCode)
		}
	}
	wg.Wait()

	res.Duration = time.Since(startAt)
	res.StdoutTail = Redact(outRing.String())
	res.StderrTail = Redact(errRing.String())
	if res.Attribution == "" {
		if res.Success() {
			res.Attribution = fmt.Sprintf("正常退出（%s）", res.Duration.Round(time.Millisecond))
		} else {
			res.Attribution = fmt.Sprintf("退出码 %d（%s）", res.ExitCode, res.Duration.Round(time.Millisecond))
		}
	}
	return res
}

func fillExit(res Result, err error) Result {
	if err == nil {
		res.ExitCode = 0
		return res
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res
	}
	res.SpawnErr = err
	res.Attribution = fmt.Sprintf("wait 失败: %v", err)
	return res
}

func pumpLines(r io.Reader, ring *lineRing) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			ring.Push(strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			return
		}
	}
}

// LookPath probe 是否可执行（doctor 复用）。
func LookPath(bin string) (string, error) { return exec.LookPath(bin) }

// lineRing 定长行环形缓冲。
type lineRing struct {
	mu    sync.Mutex
	lines []string
	head  int
	full  bool
	cap   int
}

func newLineRing(cap int) *lineRing { return &lineRing{lines: make([]string, cap), cap: cap} }

func (r *lineRing) Push(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines[r.head] = s
	r.head = (r.head + 1) % r.cap
	if r.head == 0 {
		r.full = true
	}
}

func (r *lineRing) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.cap
	if !r.full {
		n = r.head
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		idx := i
		if r.full {
			idx = (r.head + i) % r.cap
		}
		out = append(out, r.lines[idx])
	}
	return strings.Join(out, "\n")
}
