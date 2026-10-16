package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/xuri/excelize/v2"

	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 审计日志相关仓储端口（初始为导出用例的窄接口，分层合规重构后扩展至 service 全量所需方法），
// 由 data 层具体仓储实现并经 wire.Bind 绑定。

// AdminLoginLogRepo 登录日志仓储端口。
type AdminLoginLogRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginLogResponse, error)
	Get(ctx context.Context, req *adminV1.GetAdminLoginLogRequest) (*adminV1.AdminLoginLog, error)
	// DeleteOlderThan 物理删除 create_time 早于 cutoff 的日志（保留期清理），返回删除行数。
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}

// AdminOperationLogRepo 操作日志仓储端口。
type AdminOperationLogRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminOperationLogResponse, error)
	Get(ctx context.Context, req *adminV1.GetAdminOperationLogRequest) (*adminV1.AdminOperationLog, error)
	// RecordAudit 写入业务审计日志（§2.6.6 等场景）
	RecordAudit(ctx context.Context, entry *AuditEntry) error
	// DeleteOlderThan 物理删除 create_time 早于 cutoff 的日志（保留期清理），返回删除行数。
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}

// LogExportUsecase 审计日志导出用例（1.6：登录日志/操作日志 Excel 导出）。
type LogExportUsecase struct {
	log *log.Helper

	loginLogRepo     AdminLoginLogRepo
	operationLogRepo AdminOperationLogRepo
}

func NewLogExportUsecase(
	loginLogRepo AdminLoginLogRepo,
	operationLogRepo AdminOperationLogRepo,
	logger log.Logger,
) *LogExportUsecase {
	return &LogExportUsecase{
		log:              log.NewHelper(log.With(logger, "module", "biz/log-export")),
		loginLogRepo:     loginLogRepo,
		operationLogRepo: operationLogRepo,
	}
}

var loginLogExcelHeaders = []string{
	"ID", "用户名", "登录IP", "登录地点", "登录时间", "结果", "状态码", "原因",
	"浏览器", "操作系统", "UserAgent",
}

// ExportLoginLogsExcel 导出登录日志（复用列表查询条件，上限 maxExportRows 行）。
func (uc *LogExportUsecase) ExportLoginLogsExcel(ctx context.Context, query string) ([]byte, error) {
	req := &pagination.PagingRequest{
		Page:     trans.Ptr(int32(1)),
		PageSize: trans.Ptr(int32(maxExportRows)),
	}
	if query != "" {
		req.Query = trans.Ptr(query)
	}

	resp, err := uc.loginLogRepo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	_ = f.SetSheetRow(sheet, "A1", &loginLogExcelHeaders)

	for i, item := range resp.GetItems() {
		row := []interface{}{
			item.GetId(),
			item.GetUsername(),
			item.GetLoginIp(),
			item.GetLocation(),
			formatTimestamp(item.GetLoginTime()),
			boolToCN(item.GetSuccess()),
			item.GetStatusCode(),
			item.GetReason(),
			fmt.Sprintf("%s %s", item.GetBrowserName(), item.GetBrowserVersion()),
			fmt.Sprintf("%s %s", item.GetOsName(), item.GetOsVersion()),
			item.GetUserAgent(),
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sheet, cell, &row)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("generate login logs excel failed: %w", err)
	}
	return buf.Bytes(), nil
}

var operationLogExcelHeaders = []string{
	"ID", "用户名", "模块", "操作", "请求方法", "请求路径", "客户端IP",
	"结果", "状态码", "原因", "耗时(秒)", "时间",
}

// ExportOperationLogsExcel 导出操作日志（复用列表查询条件，上限 maxExportRows 行）。
func (uc *LogExportUsecase) ExportOperationLogsExcel(ctx context.Context, query string) ([]byte, error) {
	req := &pagination.PagingRequest{
		Page:     trans.Ptr(int32(1)),
		PageSize: trans.Ptr(int32(maxExportRows)),
	}
	if query != "" {
		req.Query = trans.Ptr(query)
	}

	resp, err := uc.operationLogRepo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	_ = f.SetSheetRow(sheet, "A1", &operationLogExcelHeaders)

	for i, item := range resp.GetItems() {
		row := []interface{}{
			item.GetId(),
			item.GetUsername(),
			item.GetApiModule(),
			item.GetOperation(),
			item.GetMethod(),
			item.GetPath(),
			item.GetClientIp(),
			boolToCN(item.GetSuccess()),
			item.GetStatusCode(),
			item.GetReason(),
			item.GetCostTime().AsDuration().Seconds(),
			formatTimestamp(item.GetCreateTime()),
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sheet, cell, &row)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("generate operation logs excel failed: %w", err)
	}
	return buf.Bytes(), nil
}

func boolToCN(b bool) string {
	if b {
		return "成功"
	}
	return "失败"
}

func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	t := ts.AsTime()
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
