package server

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/go-kratos/kratos/v2/transport/http"

	"framework/app/admin/internal/service"
)

// 用户导入导出 + 审计日志导出手写路由（1.6）。
//
// 说明：二进制 Excel 下载与 multipart 上传无法由 proto google.api.http 注解表达，
// 采用与 OSS 上传相同的手写 handler 模式。路由路径已同步进 openapi.yaml，
// 保证 api_resource 同步与 casbin 鉴权生效。
func registerUserIOHandlers(srv *http.Server, svc *service.UserIOService) {
	r := srv.Route("/")
	r.GET("api/v1/monitor/users:export", _UserIOService_ExportUsers_HTTP_Handler(svc))
	r.GET("api/v1/monitor/users:import-template", _UserIOService_DownloadTemplate_HTTP_Handler(svc))
	r.POST("api/v1/monitor/users:import", _UserIOService_ImportUsers_HTTP_Handler(svc))
	r.GET("api/v1/monitor/admin-login-logs:export", _UserIOService_ExportLoginLogs_HTTP_Handler(svc))
	r.GET("api/v1/monitor/admin-operation-logs:export", _UserIOService_ExportOperationLogs_HTTP_Handler(svc))
}

const (
	OperationUserIOServiceExportUsers          = "/admin.service.v1.UserIOService/ExportUsers"
	OperationUserIOServiceDownloadTemplate     = "/admin.service.v1.UserIOService/DownloadUserImportTemplate"
	OperationUserIOServiceImportUsers          = "/admin.service.v1.UserIOService/ImportUsers"
	OperationUserIOServiceExportLoginLogs      = "/admin.service.v1.UserIOService/ExportLoginLogs"
	OperationUserIOServiceExportOperationLogs  = "/admin.service.v1.UserIOService/ExportOperationLogs"
)

// writeExcelFile 将 ExcelFile 写入响应（带 Content-Disposition 下载头）。
func writeExcelFile(ctx http.Context, file *service.ExcelFile) error {
	w := ctx.Response()
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(file.FileName)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(file.Data)))
	_, err := w.Write(file.Data)
	return err
}

// excelDownloadHandler 通用导出 handler：run 返回 ExcelFile 后写二进制响应。
func excelDownloadHandler(operation string, run func(ctx context.Context, query string) (*service.ExcelFile, error)) func(ctx http.Context) error {
	return func(ctx http.Context) error {
		http.SetOperation(ctx, operation)

		query := ctx.Request().URL.Query().Get("query")

		h := ctx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
			return run(ctx, query)
		})

		out, err := h(ctx, nil)
		if err != nil {
			return err
		}
		return writeExcelFile(ctx, out.(*service.ExcelFile))
	}
}

func _UserIOService_ExportUsers_HTTP_Handler(svc *service.UserIOService) func(ctx http.Context) error {
	return excelDownloadHandler(OperationUserIOServiceExportUsers, svc.ExportUsers)
}

func _UserIOService_DownloadTemplate_HTTP_Handler(svc *service.UserIOService) func(ctx http.Context) error {
	return excelDownloadHandler(OperationUserIOServiceDownloadTemplate,
		func(ctx context.Context, _ string) (*service.ExcelFile, error) {
			return svc.DownloadUserImportTemplate(ctx)
		})
}

func _UserIOService_ExportLoginLogs_HTTP_Handler(svc *service.UserIOService) func(ctx http.Context) error {
	return excelDownloadHandler(OperationUserIOServiceExportLoginLogs, svc.ExportLoginLogs)
}

func _UserIOService_ExportOperationLogs_HTTP_Handler(svc *service.UserIOService) func(ctx http.Context) error {
	return excelDownloadHandler(OperationUserIOServiceExportOperationLogs, svc.ExportOperationLogs)
}

func _UserIOService_ImportUsers_HTTP_Handler(svc *service.UserIOService) func(ctx http.Context) error {
	return func(ctx http.Context) error {
		http.SetOperation(ctx, OperationUserIOServiceImportUsers)

		file, _, err := ctx.Request().FormFile("file")
		if err != nil {
			return err
		}
		defer file.Close()

		fileBytes, err := io.ReadAll(io.LimitReader(file, 10<<20)) // 上限 10MB
		if err != nil {
			return err
		}

		h := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			return svc.ImportUsers(ctx, req.([]byte))
		})

		out, err := h(ctx, fileBytes)
		if err != nil {
			return err
		}

		return ctx.Result(200, out)
	}
}
