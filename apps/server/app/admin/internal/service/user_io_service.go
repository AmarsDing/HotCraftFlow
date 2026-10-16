package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/pkg/middleware/auth"
)

// xlsxMime Excel 文件 MIME 类型。
const xlsxMime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

// ExcelFile 待下载的 Excel 文件（内容由 biz 层生成）。
type ExcelFile struct {
	Data        []byte
	FileName    string
	ContentType string
}

// UserIOService 用户批量导入导出 + 审计日志导出服务（1.6）。
// 该服务不走 proto 注册（二进制下载/multipart 上传无法由 google.api.http 注解表达），
// 由 server 层手写 handler 直接调用，与 OSS 上传同模式。
type UserIOService struct {
	log *log.Helper

	userIO    *biz.UserIOUsecase
	logExport *biz.LogExportUsecase
}

func NewUserIOService(
	logger log.Logger,
	userIO *biz.UserIOUsecase,
	logExport *biz.LogExportUsecase,
) *UserIOService {
	return &UserIOService{
		log:       log.NewHelper(log.With(logger, "module", "service/user-io")),
		userIO:    userIO,
		logExport: logExport,
	}
}

func excelFileName(prefix string) string {
	return fmt.Sprintf("%s_%s.xlsx", prefix, time.Now().Format("20060102150405"))
}

// ExportUsers 导出用户 Excel。
func (s *UserIOService) ExportUsers(ctx context.Context, query string) (*ExcelFile, error) {
	data, err := s.userIO.ExportUsersExcel(ctx, query)
	if err != nil {
		return nil, err
	}
	return &ExcelFile{Data: data, FileName: excelFileName("users"), ContentType: xlsxMime}, nil
}

// DownloadUserImportTemplate 下载用户导入模板。
func (s *UserIOService) DownloadUserImportTemplate(ctx context.Context) (*ExcelFile, error) {
	data, err := s.userIO.UserImportTemplateExcel(ctx)
	if err != nil {
		return nil, err
	}
	return &ExcelFile{Data: data, FileName: "user_import_template.xlsx", ContentType: xlsxMime}, nil
}

// ImportUsers 批量导入用户（fileBytes 为 xlsx 内容）。
func (s *UserIOService) ImportUsers(ctx context.Context, fileBytes []byte) (*biz.ImportUsersResult, error) {
	var operatorID uint32
	if operator, err := auth.FromContext(ctx); err == nil {
		operatorID = operator.UserId
	}
	return s.userIO.ImportUsersExcel(ctx, fileBytes, operatorID)
}

// ExportLoginLogs 导出登录日志 Excel。
func (s *UserIOService) ExportLoginLogs(ctx context.Context, query string) (*ExcelFile, error) {
	data, err := s.logExport.ExportLoginLogsExcel(ctx, query)
	if err != nil {
		return nil, err
	}
	return &ExcelFile{Data: data, FileName: excelFileName("login_logs"), ContentType: xlsxMime}, nil
}

// ExportOperationLogs 导出操作日志 Excel。
func (s *UserIOService) ExportOperationLogs(ctx context.Context, query string) (*ExcelFile, error) {
	data, err := s.logExport.ExportOperationLogsExcel(ctx, query)
	if err != nil {
		return nil, err
	}
	return &ExcelFile{Data: data, FileName: excelFileName("operation_logs"), ContentType: xlsxMime}, nil
}
