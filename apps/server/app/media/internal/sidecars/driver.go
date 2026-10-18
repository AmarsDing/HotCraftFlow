package sidecars

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"framework/pkg/exectree"
)

// Config sidecar 驱动配置（来自 media.yaml sidecar 段）。
type Config struct {
	ReadyTimeout  time.Duration // 握手时限（默认 30s）
	GenWatchdog   time.Duration // 单次生成看门狗（默认 600s）
	IdleReap      time.Duration // 空转收割（默认 300s；随生成即回收模式下为常驻预留）
	MaxFrameBytes int64         // 帧上限（默认 64MB）
	// Env 白名单透传子进程的额外环境变量（KEY=VALUE；secret 值在日志层脱敏）。
	Env []string
}

// normalize 归零值回退默认。
func (c Config) normalize() Config {
	if c.ReadyTimeout <= 0 {
		c.ReadyTimeout = 30 * time.Second
	}
	if c.GenWatchdog <= 0 {
		c.GenWatchdog = 600 * time.Second
	}
	if c.IdleReap <= 0 {
		c.IdleReap = 300 * time.Second
	}
	if c.MaxFrameBytes <= 0 {
		c.MaxFrameBytes = defaultMaxFrameBytes
	}
	return c
}

// Events 一次生成过程的回调集（与 engines.ChunkSink 解耦，避免包环）。
type Events struct {
	OnProgress func(progress float64)
	OnChunk    func(pcm []byte)
	OnLog      func(line string)
}

// 驱动生命周期错误词表。
var (
	ErrHandshakeTimeout = errors.New("sidecar ready handshake timeout")
	ErrSidecarDied      = errors.New("sidecar process exited unexpectedly")
	ErrProtocol         = errors.New("sidecar protocol violation")
	ErrGenWatchdog      = errors.New("sidecar generation watchdog fired")
	ErrBusy             = errors.New("sidecar busy: concurrent generation rejected")
)

// ev 事件通道元素：一帧或一次"流终"（唯一写者，消竞态）。
// lock==false 表示进程/协议层终结：exit 非 nil 为错误归因，
// exit 为 nil 表示干净退出但生成未回 result。
type ev struct {
	f    *Frame
	term bool
	exit error
}

// Driver 单个 sidecar 子进程的生命周期管理：
// 拉起 → ready 握手 → synthesize 请求/事件流 → 终态回收。
//
// 纪律：一个 Driver 同一时刻只跑一个生成（单引擎驻留的进程级落点）；
// 并发 RunGeneration 直接报 ErrBusy，由上层 lane 信号量保证串行。
type Driver struct {
	bin string
	cfg Config
	log *slog.Logger

	mu      sync.Mutex
	running bool
	proc    *exectree.Pipes
}

// NewDriver 构造驱动（bin 为 sidecar 可执行文件绝对路径或 PATH 名）。
func NewDriver(bin string, cfg Config, log *slog.Logger) *Driver {
	return &Driver{bin: bin, cfg: cfg.normalize(), log: log}
}

// RunGeneration 跑完一次完整生成：spawn → 握手 → synthesize → 事件流到
// result/error。任何路径返回前进程树必定被回收。
func (d *Driver) RunGeneration(ctx context.Context, req any, evs Events) (json.RawMessage, error) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return nil, ErrBusy
	}
	d.running = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.running = false
		d.proc = nil
		d.mu.Unlock()
	}()

	bin, err := exectree.LookPath(d.bin)
	if err != nil {
		return nil, fmt.Errorf("%w: binary %q: %v", ErrSidecarDied, d.bin, err)
	}

	pipes, err := exectree.Spawn(ctx, exectree.RunSpec{
		Bin: bin,
		Env: append([]string{}, d.cfg.Env...),
	}, func(line string) {
		d.log.Debug("sidecar stderr", "line", line)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: spawn: %v", ErrSidecarDied, err)
	}
	d.mu.Lock()
	d.proc = pipes
	d.mu.Unlock()

	done := make(chan struct{})
	defer close(done)
	defer func() { _ = pipes.Kill() }()

	events := d.pumpFrames(pipes, done)

	// ---- 握手 ----
	if err := WriteFrame(pipes.Stdin, Frame{Type: TypeHello, Version: "media/1"}); err != nil {
		return nil, fmt.Errorf("%w: write hello: %v", ErrSidecarDied, err)
	}
	if err := d.waitReady(ctx, events); err != nil {
		return nil, err
	}

	// ---- 下发 synthesize ----
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProtocol, err)
	}
	if err := WriteFrame(pipes.Stdin, Frame{Type: TypeSynthesize, Request: payload}); err != nil {
		return nil, fmt.Errorf("%w: write synthesize: %v", ErrSidecarDied, err)
	}

	// ---- 生成事件循环（带看门狗）----
	watchdog := time.NewTimer(d.cfg.GenWatchdog)
	defer watchdog.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-watchdog.C:
			return nil, ErrGenWatchdog
		case e, open := <-events:
			if !open {
				return nil, fmt.Errorf("%w: event stream closed", ErrSidecarDied)
			}
			if e.term {
				return nil, fmt.Errorf("%w: %s", ErrSidecarDied, termReason(e.exit))
			}
			res, doneGen, err := d.dispatch(e.f, evs)
			if doneGen {
				return res, err
			}
		}
	}
}

// pumpFrames 事件泵（事件通道唯一写者）：
// 读帧 → 投递；流耗尽 → Wait 取退出归因；协议违例 → 杀树并上报。
// 背压策略：progress/chunk/log 满了可丢（进度丢几帧无害），
// ready/result/error/终结事件必须送达（阻塞到 done 为止）。
func (d *Driver) pumpFrames(pipes *exectree.Pipes, done <-chan struct{}) <-chan ev {
	out := make(chan ev, 32)
	go func() {
		defer close(out)
		fr := newFrameReader(pipes.Stdout, int(d.cfg.MaxFrameBytes))

		sendMust := func(e ev) bool {
			select {
			case out <- e:
				return true
			case <-done:
				return false
			}
		}
		sendDroppable := func(e ev) bool {
			select {
			case out <- e:
				return true
			case <-done:
				return false
			default:
				return true // 丢弃
			}
		}

		for {
			f, err := fr.read()
			if err != nil {
				_ = pipes.Kill()
				sendMust(ev{term: true, exit: fmt.Errorf("%w: %v", ErrProtocol, err)})
				return
			}
			if f == nil { // 流耗尽：进程关闭了 stdout → 取退出归因
				werr := pipes.Wait()
				sendMust(ev{term: true, exit: werr})
				return
			}
			if f.Type == "" {
				continue // 未知 type 哨兵帧：丢弃
			}
			var ok bool
			switch f.Type {
			case TypeReady, TypeResult, TypeError:
				ok = sendMust(ev{f: f})
			default:
				ok = sendDroppable(ev{f: f})
			}
			if !ok {
				return
			}
		}
	}()
	return out
}

// dispatch 处理生成期单帧。doneGen=true 表示生成终局。
func (d *Driver) dispatch(f *Frame, evs Events) (json.RawMessage, bool, error) {
	switch f.Type {
	case TypeProgress:
		if evs.OnProgress != nil {
			evs.OnProgress(f.Progress)
		}
	case TypeChunk:
		if evs.OnChunk != nil && f.Data != "" {
			pcm, decErr := base64.StdEncoding.DecodeString(f.Data)
			if decErr != nil {
				return nil, true, fmt.Errorf("%w: chunk base64: %v", ErrProtocol, decErr)
			}
			evs.OnChunk(pcm)
		}
	case TypeLog:
		if evs.OnLog != nil {
			evs.OnLog(f.MsgText)
		}
		d.log.Debug("sidecar log", "line", f.MsgText)
	case TypeResult:
		return f.Result, true, nil
	case TypeError:
		return nil, true, &SidecarError{Code: f.Code, Msg: f.Msg}
	}
	return nil, false, nil
}

// waitReady 握手等待：超时或进程先死都归为启动失败。
func (d *Driver) waitReady(ctx context.Context, events <-chan ev) error {
	ctx, cancel := context.WithTimeout(ctx, d.cfg.ReadyTimeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return ErrHandshakeTimeout
		case e, open := <-events:
			if !open {
				return fmt.Errorf("%w: stream closed before ready", ErrSidecarDied)
			}
			if e.term {
				return fmt.Errorf("%w: exited before ready: %s", ErrSidecarDied, termReason(e.exit))
			}
			switch e.f.Type {
			case TypeReady:
				return nil
			case TypeError:
				return &SidecarError{Code: e.f.Code, Msg: e.f.Msg}
			}
			// 握手期的 progress/chunk 属异常帧，宽容忽略。
		}
	}
}

// termReason 终局归因展示。
func termReason(err error) string {
	if err == nil {
		return "clean exit without result"
	}
	return err.Error()
}

// SidecarError sidecar 上报的业务错误（jobrun 按 Code 映射公开错误码）。
type SidecarError struct {
	Code string
	Msg  string
}

func (e *SidecarError) Error() string {
	if e.Code == "" {
		return "sidecar error: " + e.Msg
	}
	return fmt.Sprintf("sidecar error %s: %s", e.Code, e.Msg)
}

// Cancel 向当前生成发取消帧（尽力而为；进程层兜底仍由 ctx/看门狗杀树）。
func (d *Driver) Cancel() {
	d.mu.Lock()
	p := d.proc
	d.mu.Unlock()
	if p != nil {
		_ = WriteFrame(p.Stdin, Frame{Type: TypeCancel})
	}
}

// ReapIdle 空转收割：单引擎驻留纪律下换引擎时由 Registry 回调触发。
// 当前 v1 进程随生成结束即回收，本方法为常驻模式预留的钩子。
func (d *Driver) ReapIdle() {
	d.mu.Lock()
	p := d.proc
	d.mu.Unlock()
	if p != nil {
		_ = p.Kill()
	}
}
