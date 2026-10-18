package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	adminV1 "framework/api/gen/go/admin/v1"
)

// registerMmeMediaHandler 注册 MME 变体媒体流路由（审核台预览/封面/字幕）。
//
// GET /api/v1/media/mme/media:get?path=<相对 media_dir 的路径>
// 走标准 middleware 链（JWT + casbin + 数据权限），与 file_download_handler.go 同模式；
// 路径强制收敛在 media_dir 内（拒绝 ../ 越层与绝对路径注入）。
// http.ServeContent 原生支持 Range（前端 <video> 拖赞同零成本）。
func registerMmeMediaHandler(srv *khttp.Server, mediaDir string) {
	if strings.TrimSpace(mediaDir) == "" {
		return
	}
	r := srv.Route("/")
	r.GET("api/v1/media/mme/media:get", _MmeMedia_HTTP_Handler(mediaDir))
}

// openedMedia 从 middleware 链传出的已打开文件（调用方负责关闭）。
type openedMedia struct {
	f    *os.File
	info os.FileInfo
	name string
}

func _MmeMedia_HTTP_Handler(mediaDir string) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, OperationMmeMediaGet)

		relPath := ctx.Request().URL.Query().Get("path")

		h := ctx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
			if relPath == "" {
				return nil, adminV1.ErrorBadRequest("path is required")
			}
			abs, err := resolveMmeMediaPath(mediaDir, relPath)
			if err != nil {
				return nil, adminV1.ErrorBadRequest("invalid media path")
			}
			f, err := os.Open(filepath.Clean(abs))
			if err != nil {
				if os.IsNotExist(err) {
					return nil, adminV1.ErrorNotFound("media not found")
				}
				return nil, adminV1.ErrorBadRequest("invalid media path")
			}
			info, err := f.Stat()
			if err != nil || info.IsDir() {
				_ = f.Close()
				return nil, adminV1.ErrorNotFound("media not found")
			}
			return &openedMedia{f: f, info: info, name: filepath.Base(abs)}, nil
		})

		out, err := h(ctx, nil)
		if err != nil {
			return err
		}

		om := out.(*openedMedia)
		defer om.f.Close()

		// ServeContent 自动处理 Content-Type（按扩展名）、Content-Length、Range 与条件请求
		http.ServeContent(ctx.Response(), ctx.Request(), om.name, om.info.ModTime(), om.f)
		return nil
	}
}

// resolveMmeMediaPath 把相对路径解析为 media_dir 内绝对路径；越层/非法返回错误。
func resolveMmeMediaPath(mediaDir, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) {
		return "", os.ErrInvalid
	}
	if strings.Contains(rel, "..") {
		return "", os.ErrInvalid
	}
	root, err := filepath.Abs(mediaDir)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(root, rel)
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", os.ErrInvalid
	}
	return abs, nil
}

const OperationMmeMediaGet = "/api/v1/media/mme/media:get"
