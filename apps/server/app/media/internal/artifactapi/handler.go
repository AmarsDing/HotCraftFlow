// Package artifactapi 任务产物下载裸端点。
//
// 端点：GET /api/v1/media/artifacts/{job_id}/{name}
//
// 安全纪律：
//   - job_id 与 name 都走形态白名单（防目录穿越：禁分隔符、禁 ..、禁绝对路径）；
//   - 解析出的最终路径必须落在 artifact_dir 之内（Clean 后前缀判定）；
//   - 文件不存在一律 404，不区分"job 在/文件不在"（防枚举）。
package artifactapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"framework/app/media/internal/authnutil"
)

const pathPrefix = "/api/v1/media/artifacts/"

// Deps 构造依赖。
type Deps struct {
	Verifier *authnutil.Verifier
	BaseDir  string
}

// Handler 返回 http.HandlerFunc。
func Handler(dep Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := dep.Verifier.Verify(r); err != nil {
			http.Error(w, `{"code":401,"reason":"UNAUTHORIZED","message":"unauthorized"}`+"\n", http.StatusUnauthorized)
			return
		}
		jobID, name, ok := parse(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		base, err := filepath.Abs(dep.BaseDir)
		if err != nil {
			http.Error(w, `{"code":500,"reason":"MEDIA_INTERNAL","message":"storage error"}`+"\n", http.StatusInternalServerError)
			return
		}
		full := filepath.Clean(filepath.Join(base, jobID, name))
		// 最终落点必须在 base 之内（形如 base\ 前缀或 base 本身）。
		if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}

		f, err := os.Open(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil || fi.IsDir() {
			http.NotFound(w, r)
			return
		}

		if ct := contentType(filepath.Ext(name)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		http.ServeContent(w, r, name, fi.ModTime(), f)
	}
}

// parse 拆分 /{job_id}/{name}（各自过形态白名单）。
func parse(path string) (jobID, name string, ok bool) {
	if !strings.HasPrefix(path, pathPrefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(path, pathPrefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	jobID, name = parts[0], parts[1]
	if !validSegment(jobID) || !validSegment(name) {
		return "", "", false
	}
	return jobID, name, true
}

// validSegment 段落形态白名单：非空、无分隔符、无 ..、无控制符。
func validSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

// contentType 已知扩展名的收敛表（其余回落默认嗅探）。
func contentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".srt":
		return "application/x-subrip; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	}
	return ""
}
