// Package sidecars 子进程推理 sidecar 驱动。
//
// 协议（原创规范）：stdio 上的"4 字节小端长度 + JSON 帧"。
// 一帧 = [uint32 LE 长度][JSON 字节]；长度上限由配置给出（默认 64MB），
// 超限视为协议违例，直接判死刑重启不归因给业务。
//
// 握手：进程拉起后 ready_timeout 内必须收到 {"type":"ready","version":"x.y.z"}，
// 否则按启动失败归因。入站 type 白名单仅：ready / progress / chunk / result / error / log，
// 未知 type 丢弃并记日志（向前兼容：sidecar 可加新 type 不破坏旧宿主）。
//
// 本包只定义帧格式与编解码；生命周期管理（看门狗、空转收割）在 driver.go。
package sidecars

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// 帧 type 白名单词表。
const (
	TypeHello    = "hello"    // 宿主 → sidecar：能力询问
	TypeReady    = "ready"    // sidecar → 宿主：握手就绪
	TypeSynthesize = "synthesize"
	TypeProgress = "progress" // sidecar → 宿主：进度（progress 0-1）
	TypeChunk    = "chunk"    // sidecar → 宿主：PCM 字节块（base64 于 data 字段）
	TypeResult   = "result"   // sidecar → 宿主：终态结果元数据
	TypeError    = "error"    // sidecar → 宿主：业务错误（code/msg）
	TypeLog      = "log"      // sidecar → 宿主：日志行（透传进宿主日志）
	TypeCancel   = "cancel"   // 宿主 → sidecar：取消当前生成
)

// 协议违例错误（不可恢复 → 进程判死）。
var (
	ErrFrameTooLarge = errors.New("sidecar frame exceeds max size")
	ErrBadFrame      = errors.New("sidecar frame malformed")
)

// Frame 一帧的通用结构：type 必填；payload 按帧型选择字段。
type Frame struct {
	Type     string          `json:"type"`
	Version  string          `json:"version,omitempty"`
	Progress float64         `json:"progress,omitempty"`
	Data     string          `json:"data,omitempty"`    // chunk 的 PCM（base64）
	Request  json.RawMessage `json:"request,omitempty"` // synthesize 请求载荷
	Result   json.RawMessage `json:"result,omitempty"`  // result 结果载荷
	Code     string          `json:"code,omitempty"`
	Msg      string          `json:"msg,omitempty"`
	MsgText  string          `json:"text,omitempty"` // log 行文本
}

// WriteFrame 写一帧（长度前缀 + JSON）。
func WriteFrame(w io.Writer, f Frame) error {
	body, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// frameReader 帧读取器：带长度上限防内存炸弹。
type frameReader struct {
	r          io.Reader
	maxPayload int
}

func newFrameReader(r io.Reader, maxPayload int) *frameReader {
	if maxPayload <= 0 {
		maxPayload = defaultMaxFrameBytes
	}
	return &frameReader{r: r, maxPayload: maxPayload}
}

const defaultMaxFrameBytes = 64 << 20 // 64MB

// read 读一帧；返回 nil, nil 表示对端正常 EOF（进程退出）。
func (fr *frameReader) read() (*Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(fr.r, hdr[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, nil
		}
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > uint32(fr.maxPayload) {
		return nil, fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, n, fr.maxPayload)
	}
	if n == 0 {
		return nil, ErrBadFrame
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(fr.r, buf); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFrame, err)
	}
	var f Frame
	dec := json.NewDecoder(bytes.NewReader(buf))
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFrame, err)
	}
	if !knownType(f.Type) {
		// 向前兼容：未知 type 返回哨兵帧，由 driver 决定丢弃。
		return &Frame{Type: ""}, nil
	}
	return &f, nil
}

// knownType 入站白名单（hello/synthesize/cancel 是出站帧，不入此表）。
func knownType(t string) bool {
	switch t {
	case TypeReady, TypeProgress, TypeChunk, TypeResult, TypeError, TypeLog:
		return true
	}
	return false
}
