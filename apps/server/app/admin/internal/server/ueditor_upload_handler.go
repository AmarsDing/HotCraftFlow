package server

import (
	"context"
	"io"
	"strings"

	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/tx7do/go-utils/trans"

	"framework/app/admin/internal/service"

	fileV1 "framework/api/gen/go/file/v1"
)

// registerUEditorUploadHandler 注册 UEditor 富文本编辑器文件上传路由。
//
// 说明：proto 的 google.api.http 注解无法表达 multipart/form-data 文件上传，
// 故采用 kratos 社区标准做法——手写 handler，通过 ctx.Request().FormFile("file")
// 解析 multipart 数据，组装成 fileV1.UEditorUploadRequest 后转交 service 处理。
// 该路由专供前端 UEditor 富文本编辑器上传图片/附件使用。
func registerUEditorUploadHandler(srv *http.Server, svc *service.UEditorService) {
	r := srv.Route("/")
	r.POST("api/v1/monitor/ueditor", _UEditorService_UploadFile_HTTP_Handler(svc))
	r.PUT("api/v1/monitor/ueditor", _UEditorService_UploadFile_HTTP_Handler(svc))
}

const OperationUEditorServiceUploadFile = "/admin.service.v1.UEditorService/UploadFile"

func _UEditorService_UploadFile_HTTP_Handler(svc *service.UEditorService) func(ctx http.Context) error {
	return func(ctx http.Context) error {
		http.SetOperation(ctx, OperationUEditorServiceUploadFile)

		var in fileV1.UEditorUploadRequest
		var err error

		file, header, err := ctx.Request().FormFile("file")
		if err == nil {
			defer file.Close()

			b := new(strings.Builder)
			_, err = io.Copy(b, file)

			in.SourceFileName = trans.Ptr(header.Filename)
			in.Mime = trans.Ptr(header.Header.Get("Content-Type"))
			in.File = []byte(b.String())
		}

		if err = ctx.BindQuery(&in); err != nil {
			return err
		}

		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			var resp *fileV1.UEditorUploadResponse

			resp, err = svc.UploadFile(ctx, req.(*fileV1.UEditorUploadRequest))
			in.File = nil

			return resp, err
		})

		// 逻辑处理，取数据
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}

		reply := out.(*fileV1.UEditorUploadResponse)

		return ctx.Result(200, reply)
	}
}
