// sidecar_echo：inference sidecar 帧协议的示范实现（占位物）。
//
// 刻意自包含：不 import 宿主 app/media 的任何 internal 包——
// sidecar 在架构上是"可用任意语言重写的独立程序"（Python torch
// 生态的 TTS sidecar 下期接入），本文件证明协议本身不依赖 Go 侧代码。
//
// 协议：stdin/stdout 上 "4 字节小端长度 + JSON 帧"。
// 入站：hello（回 ready）/ synthesize（分块吐 chunk+progress，最后 result）
// / cancel（尽力退出；宿主侧兜底为杀进程树）。
// 诊断输出一律走 stderr（宿主以行泵收编进日志，不上协议面）。
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"time"
	"unicode/utf8"
)

const (
	sampleRate     = 22050
	freq           = 432.0
	fadeMs         = 15
	amplitude      = 0.32
	chunkMs        = 200
	chunkPaceMs    = 30
	minSeconds     = 0.5
	maxSeconds     = 30.0
	secondsPerRune = 0.012
	maxFrameBytes  = 64 << 20
)

// frame 通用帧结构（与宿主 protocol.go 契约同形，独立实现）。
type frame struct {
	Type     string          `json:"type"`
	Version  string          `json:"version,omitempty"`
	Progress float64         `json:"progress,omitempty"`
	Data     string          `json:"data,omitempty"`
	Request  json.RawMessage `json:"request,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Code     string          `json:"code,omitempty"`
	Msg      string          `json:"msg,omitempty"`
	MsgText  string          `json:"text,omitempty"`
}

type synthRequest struct {
	Text    string  `json:"text"`
	Seconds float64 `json:"seconds,omitempty"`
}

type synthResult struct {
	DurationMs   int    `json:"duration_ms"`
	Samples      int    `json:"samples"`
	ArtifactHint string `json:"artifact_hint"`
}

func main() {
	fmt.Fprintln(os.Stderr, "sidecar_echo: started")
	in := bufio.NewReaderSize(os.Stdin, 64*1024)
	for {
		f, err := readFrame(in)
		if err != nil {
			// EOF（宿主关闭 stdin）或读失败都直接退出——
			// 宿主按"exited <reason>"归因。
			fmt.Fprintln(os.Stderr, "sidecar_echo: input closed:", err)
			return
		}
		switch f.Type {
		case "hello":
			if err := writeFrame(frame{Type: "ready", Version: "sidecar_echo/1.0.0"}); err != nil {
				return
			}
		case "synthesize":
			if !runSynthesize(f.Request) {
				return
			}
		case "cancel":
			fmt.Fprintln(os.Stderr, "sidecar_echo: cancel frame received, exiting")
			return
		default:
			fmt.Fprintf(os.Stderr, "sidecar_echo: unknown inbound type %q, ignored\n", f.Type)
		}
	}
}

// runSynthesize 执行一次生成；返回 false 表示协议面已坏应退出。
func runSynthesize(payload json.RawMessage) bool {
	var req synthRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		writeFrame(frame{Type: "error", Code: "BAD_REQUEST", Msg: "payload unmarshal: " + err.Error()})
		return true
	}

	sec := estimateSeconds(req.Text, req.Seconds)
	total := int(sec * sampleRate)
	step := sampleRate * chunkMs / 1000
	if step <= 0 {
		step = total
	}

	writeFrame(frame{Type: "log", MsgText: fmt.Sprintf("synthesize: %d samples (%.2fs)", total, sec)})

	for off := 0; off < total; off += step {
		end := off + step
		if end > total {
			end = total
		}
		chunk := make([]byte, (end-off)*2)
		n := end - off
		for i := 0; i < n; i++ {
			s := sampleAt(off + i)
			binary.LittleEndian.PutUint16(chunk[i*2:], uint16(s))
		}
		progress := float64(end) / float64(total)
		if err := writeFrame(frame{Type: "progress", Progress: progress}); err != nil {
			return false
		}
		if err := writeFrame(frame{Type: "chunk", Data: base64.StdEncoding.EncodeToString(chunk)}); err != nil {
			return false
		}
		time.Sleep(time.Duration(chunkPaceMs) * time.Millisecond)
	}

	res, _ := json.Marshal(synthResult{
		DurationMs:   int(sec * 1000),
		Samples:      total,
		ArtifactHint: "speech.wav",
	})
	return writeFrame(frame{Type: "result", Result: res}) == nil
}

// estimateSeconds 与宿主 echo 引擎同款时长估算（双端协议一致性演示）。
func estimateSeconds(text string, explicit float64) float64 {
	sec := explicit
	if sec <= 0 {
		sec = minSeconds + float64(utf8.RuneCountInString(text))*secondsPerRune
	}
	if sec < minSeconds {
		sec = minSeconds
	}
	if sec > maxSeconds {
		sec = maxSeconds
	}
	return sec
}

// sampleAt 正弦波采样（与宿主 echosynth 同款生成策略，独立实现）。
func sampleAt(i int) int16 {
	t := float64(i) / float64(sampleRate)
	v := math.Sin(2*math.Pi*freq*t) + 0.18*math.Sin(2*math.Pi*freq*2*t)
	return int16(v * amplitude * float64(math.MaxInt16-1))
}

// ---- 帧编解码（4 字节小端长度前缀 + JSON） ----

func readFrame(r io.Reader) (frame, error) {
	var f frame
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return f, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n == 0 || n > maxFrameBytes {
		return f, fmt.Errorf("bad frame size %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return f, err
	}
	if err := json.Unmarshal(buf, &f); err != nil {
		return f, fmt.Errorf("frame json: %w", err)
	}
	return f, nil
}

func writeFrame(f frame) error {
	body, err := json.Marshal(f)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := os.Stdout.Write(hdr[:]); err != nil {
		return err
	}
	_, err = os.Stdout.Write(body)
	return err
}
