package events

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// gzip 魔数（RFC 1952）：发布端开启压缩时消息体以此开头。
var gzipMagic = []byte{0x1f, 0x8b}

// Encode 序列化信封；compress 为 true 时 gzip 压缩。
func Encode(env *Envelope, compress bool) ([]byte, error) {
	msg, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal event envelope: %w", err)
	}
	if !compress {
		return msg, nil
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err = w.Write(msg); err != nil {
		return nil, fmt.Errorf("gzip encode: %w", err)
	}
	if err = w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// Decode 反序列化信封，按魔数嗅探自动解压 gzip。
//
// 修复历史陷阱：monitoralarm/collector 的 publisher 可配置 gzip 压缩，
// 但存量消费端直接 json.Unmarshal 不解压，压缩一开全链路解析失败。
// 新增消费端一律用本函数解码；存量消费端迁移期间保持原样（当前全站压缩配置均为关闭）。
func Decode(data []byte) (*Envelope, error) {
	if len(data) == 0 {
		return &Envelope{}, nil
	}
	if bytes.HasPrefix(data, gzipMagic) {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer r.Close()
		if data, err = io.ReadAll(r); err != nil {
			return nil, fmt.Errorf("gzip decode: %w", err)
		}
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("unmarshal event envelope: %w", err)
	}
	return &env, nil
}
