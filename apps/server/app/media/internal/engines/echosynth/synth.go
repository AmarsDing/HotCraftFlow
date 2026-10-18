// Package echosynth 回声合成器：用最基础的 PCM 正弦波生成一段可播放的
// WAV（22050Hz 单声道 16bit），供 echo 引擎与 sidecar 示范程序共用。
//
// 它只是链路占位物：验证"分块 sink 回调 → SSE 进度事件 → 产物落盘"
// 全路径，不承担任何真实 TTS 语义。
package echosynth

import (
	"bytes"
	"encoding/binary"
	"math"
)

const (
	// SampleRate 固定采样率（占位物，不做高质量重采样）。
	SampleRate = 22050
	// freq 基准频率（A4 附近，听感不刺耳）。
	freq = 432.0
	// fadeMs 首尾淡入淡出，避免爆音。
	fadeMs = 15
	// amplitude 振幅（留足 headroom）。
	amplitude = 0.32
)

// PCM 全部样本（总计 samples 个 int16）。
func PCM(samples int) []int16 {
	if samples <= 0 {
		return nil
	}
	pcm := make([]int16, samples)
	fadeN := SampleRate * fadeMs / 1000
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(SampleRate)
		// 轻微二次谐波让听感不像报警器。
		v := math.Sin(2*math.Pi*freq*t) + 0.18*math.Sin(2*math.Pi*freq*2*t)
		gain := amplitude
		if i < fadeN {
			gain *= float64(i) / float64(fadeN)
		}
		if d := samples - i; d < fadeN {
			gain *= float64(d) / float64(fadeN)
		}
		pcm[i] = int16(v * gain * float64(math.MaxInt16-1))
	}
	return pcm
}

// EncodeWAV PCM16 → RIFF/WAVE 字节流（44 字节头 + 数据）。
func EncodeWAV(pcm []int16) []byte {
	dataBytes := len(pcm) * 2
	var buf bytes.Buffer
	buf.Grow(44 + dataBytes)

	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+dataBytes))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))           // fmt 块大小
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))            // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))            // mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(SampleRate))   // sample rate
	_ = binary.Write(&buf, binary.LittleEndian, uint32(SampleRate*2)) // byte rate = sr*ch*2
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))            // block align = ch*2
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))           // bits per sample
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(dataBytes))
	for _, s := range pcm {
		_ = binary.Write(&buf, binary.LittleEndian, s)
	}
	return buf.Bytes()
}

// SamplesForSeconds 秒数 → 样本数（向下取整）。
func SamplesForSeconds(sec float64) int {
	if sec <= 0 {
		return 0
	}
	return int(sec * float64(SampleRate))
}

// ChunkSamples 每块样本数（块时长 ms）。
func ChunkSamples(chunkMs int) int {
	return SampleRate * chunkMs / 1000
}
