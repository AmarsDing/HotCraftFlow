package mme

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// fingerprintSampleSize 指纹采样窗口：头部 64MB（概要设计 FR-MIX-07；
// 兼顾秒级速度与对整文件改动的敏感度，不追求密码学全覆盖）。
const fingerprintSampleSize = 64 << 20

// FingerprintFile 计算素材内容指纹：sha256(头 64MB | size | duration 毫秒)。
// 同文件重复下载必然同指纹；重编码/剪辑导致头部或时长变化则不同指纹。
// 输入 size/durationMS 由调用方提供（os.Stat / ffprobe 结果），本函数不探测。
func FingerprintFile(path string, sizeBytes, durationMS int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: open %s: %v", ErrAssetMissing, path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.CopyN(h, f, fingerprintSampleSize); err != nil && err != io.EOF {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if _, err = fmt.Fprintf(h, "|%d|%d", sizeBytes, durationMS); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
