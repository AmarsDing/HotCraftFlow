package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	fileV1 "framework/api/gen/go/file/v1"

	"framework/pkg/storage"
)

// registerFileDownloadHandler 注册通用文件鉴权下载路由（决策 D6：替代 MinIO 预签名 URL）。
//
// GET /api/v1/monitor/file:download?path=<相对路径>
// 走标准 middleware 链（JWT 认证 + casbin 鉴权 + 数据权限），与 OSS 上传手写 handler 同模式；
// 路由路径已同步进 openapi.yaml，保证 api_resource 同步与 casbin 鉴权生效。
// 使用 http.ServeContent 流式输出，原生支持 Range/If-Modified-Since（UE5 大模型分块下载）。
func registerFileDownloadHandler(srv *khttp.Server, store *storage.LocalStorage) {
	r := srv.Route("/")
	r.GET("api/v1/monitor/file:download", _FileService_DownloadFile_HTTP_Handler(store))
}

const OperationFileServiceDownloadFile = "/admin.service.v1.FileService/DownloadFile"

// openedFile 从 middleware 链传出的已打开文件（调用方负责关闭）。
type openedFile struct {
	f    *os.File
	info os.FileInfo
	name string
}

func _FileService_DownloadFile_HTTP_Handler(store *storage.LocalStorage) func(ctx khttp.Context) error {
	return func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, OperationFileServiceDownloadFile)

		relPath := ctx.Request().URL.Query().Get("path")

		// 认证/鉴权在 middleware 链内完成后再打开文件，未通过时不会触碰文件系统
		h := ctx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
			if store == nil {
				return nil, fileV1.ErrorInternalServerError("storage not configured")
			}
			if relPath == "" {
				return nil, fileV1.ErrorBadRequest("path is required")
			}

			f, info, err := store.Open(relPath)
			if err != nil {
				if os.IsNotExist(err) {
					return nil, fileV1.ErrorFileNotFound("file not found")
				}
				return nil, fileV1.ErrorBadRequest("invalid file path")
			}
			return &openedFile{f: f, info: info, name: filepath.Base(relPath)}, nil
		})

		out, err := h(ctx, nil)
		if err != nil {
			return err
		}

		of := out.(*openedFile)
		defer of.f.Close()

		// ServeContent 自动处理 Content-Type（按扩展名）、Content-Length、Range 与条件请求
		http.ServeContent(ctx.Response(), ctx.Request(), of.name, of.info.ModTime(), of.f)
		return nil
	}
}
