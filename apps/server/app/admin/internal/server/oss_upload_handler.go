package server

import (
	"bytes"
	"context"
	"io"

	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/tx7do/go-utils/trans"

	"framework/app/admin/internal/service"

	fileV1 "framework/api/gen/go/file/v1"
)

// registerOssUploadHandler 注册 OSS 文件上传路由。
//
// 说明：proto 的 google.api.http 注解无法表达 multipart/form-data 文件上传，
// 故采用 kratos 社区标准做法——手写 handler，通过 ctx.Request().FormFile("file")
// 解析 multipart 数据，组装成 fileV1.UploadOssFileRequest 后转交 service 处理。
// 路由路径与 admin proto 中 OssService 的 RPC 对齐。
func registerOssUploadHandler(srv *http.Server, svc *service.OssService) {
	r := srv.Route("/")
	r.POST("api/v1/monitor/file:upload", _OssService_PostUploadFile_HTTP_Handler(svc))
	r.PUT("api/v1/monitor/file:upload", _OssService_PutUploadFile_HTTP_Handler(svc))
}

const OperationOssServicePostUploadFile = "/admin.service.v1.OssService/PostUploadFile"
const OperationOssServicePutUploadFile = "/admin.service.v1.OssService/PutUploadFile"

func _OssService_PostUploadFile_HTTP_Handler(svc *service.OssService) func(ctx http.Context) error {
	return func(ctx http.Context) error {
		http.SetOperation(ctx, OperationOssServicePostUploadFile)

		var in fileV1.UploadOssFileRequest
		var err error

		file, header, err := ctx.Request().FormFile("file")
		if err == nil {
			defer file.Close()

			b := new(bytes.Buffer)
			_, err = io.Copy(b, file)
			if err != nil {
				return err
			}

			in.SourceFileName = trans.Ptr(header.Filename)
			in.Mime = trans.Ptr(header.Header.Get("Content-Type"))
			in.File = b.Bytes()
		}

		if err = ctx.BindQuery(&in); err != nil {
			return err
		}

		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			aReq := req.(*fileV1.UploadOssFileRequest)

			var resp *fileV1.UploadOssFileResponse
			resp, err = svc.PostUploadFile(ctx, aReq)
			in.File = nil

			return resp, err
		})

		// 逻辑处理，取数据
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}

		reply := out.(*fileV1.UploadOssFileResponse)

		return ctx.Result(200, reply)
	}
}

func _OssService_PutUploadFile_HTTP_Handler(svc *service.OssService) func(ctx http.Context) error {
	return func(ctx http.Context) error {
		http.SetOperation(ctx, OperationOssServicePutUploadFile)

		var in fileV1.UploadOssFileRequest
		var err error

		file, header, err := ctx.Request().FormFile("file")
		if err == nil {
			defer file.Close()

			b := new(bytes.Buffer)
			_, err = io.Copy(b, file)
			if err != nil {
				return err
			}

			in.SourceFileName = trans.Ptr(header.Filename)
			in.Mime = trans.Ptr(header.Header.Get("Content-Type"))
			in.File = b.Bytes()
		}

		if err = ctx.BindQuery(&in); err != nil {
			return err
		}

		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			aReq := req.(*fileV1.UploadOssFileRequest)

			var resp *fileV1.UploadOssFileResponse
			resp, err = svc.PutUploadFile(ctx, aReq)
			in.File = nil

			return resp, err
		})

		// 逻辑处理，取数据
		out, err := h(ctx, &in)
		if err != nil {
			return err
		}

		reply := out.(*fileV1.UploadOssFileResponse)

		return ctx.Result(200, reply)
	}
}
