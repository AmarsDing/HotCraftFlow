package biz

import (
	"bytes"
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/xuri/excelize/v2"

	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"
)

// 用户导入导出用例复用 user_repo.go / user_credential_repo.go / role_repo.go /
// department_repo.go / position_repo.go 中声明的仓储端口（方法集为各用例并集），
// 由 data 层具体仓储实现并经 wire.Bind 绑定。
// 注意：biz 层不得 import data 具体类型，否则与 data 层实现 biz 接口形成 import cycle。

// 用户导入导出 Excel 列定义（模板与导出共用同一列序，保证导出文件可直接改后重新导入）。
var userExcelHeaders = []string{
	"用户名*", "昵称", "真实姓名", "工号", "邮箱", "手机号",
	"性别(男/女/保密)", "部门(名称)", "岗位(名称)", "角色码(多个用逗号分隔)",
	"状态(启用/禁用)", "初始密码(默认666666)",
}

// maxExportRows 单次导出最大行数，防止全量导出拖垮服务。
const maxExportRows = 10000

// ImportRowError 单行导入失败明细。
type ImportRowError struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

// ImportUsersResult 导入结果汇总。
type ImportUsersResult struct {
	Total        int              `json:"total"`
	SuccessCount int              `json:"successCount"`
	FailedCount  int              `json:"failedCount"`
	Errors       []*ImportRowError `json:"errors"`
}

// UserIOUsecase 用户批量导入导出用例（1.6：Excel 模板下载 → 批量导入校验 → 错误行提示）。
type UserIOUsecase struct {
	log *log.Helper

	userRepo            UserRepo
	userCredentialsRepo UserCredentialRepo
	roleRepo            RoleRepo
	departmentRepo      DepartmentRepo
	positionRepo        PositionRepo
}

func NewUserIOUsecase(
	userRepo UserRepo,
	userCredentialsRepo UserCredentialRepo,
	roleRepo RoleRepo,
	departmentRepo DepartmentRepo,
	positionRepo PositionRepo,
	logger log.Logger,
) *UserIOUsecase {
	return &UserIOUsecase{
		log:                 log.NewHelper(log.With(logger, "module", "biz/user-io")),
		userRepo:            userRepo,
		userCredentialsRepo: userCredentialsRepo,
		roleRepo:            roleRepo,
		departmentRepo:      departmentRepo,
		positionRepo:        positionRepo,
	}
}

// ---------- 模板 ----------

// UserImportTemplateExcel 生成用户导入模板（表头 + 一行示例）。
func (uc *UserIOUsecase) UserImportTemplateExcel(_ context.Context) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	_ = f.SetSheetRow(sheet, "A1", &userExcelHeaders)
	example := []interface{}{
		"zhangsan", "张三", "张三", "1001", "zhangsan@example.com", "13800000000",
		"男", "信息中心", "运维工程师", "tenant_admin", "启用", "",
	}
	_ = f.SetSheetRow(sheet, "A2", &example)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("generate import template failed: %w", err)
	}
	return buf.Bytes(), nil
}

// ---------- 导出 ----------

// ExportUsersExcel 导出用户列表为 Excel（复用列表查询条件，上限 maxExportRows 行）。
func (uc *UserIOUsecase) ExportUsersExcel(ctx context.Context, query string) ([]byte, error) {
	req := &pagination.PagingRequest{
		Page:     trans.Ptr(int32(1)),
		PageSize: trans.Ptr(int32(maxExportRows)),
	}
	if query != "" {
		req.Query = trans.Ptr(query)
	}

	resp, err := uc.userRepo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	orgMap, err := uc.departmentNameMap(ctx)
	if err != nil {
		return nil, err
	}
	posMap, err := uc.positionNameMap(ctx)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	_ = f.SetSheetRow(sheet, "A1", &userExcelHeaders)

	for i, u := range resp.GetItems() {
		var workID string
		if u.GetWorkId() > 0 {
			workID = fmt.Sprintf("%d", u.GetWorkId())
		}
		row := []interface{}{
			u.GetUsername(),
			u.GetNickname(),
			u.GetRealname(),
			workID,
			u.GetEmail(),
			u.GetMobile(),
			genderToCN(u.GetGender()),
			orgMap[u.GetOrgId()],
			posMap[u.GetPositionId()],
			strings.Join(u.GetRoles(), ","),
			statusToCN(u.GetStatus()),
			"",
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sheet, cell, &row)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("generate users excel failed: %w", err)
	}
	return buf.Bytes(), nil
}

// ---------- 导入 ----------

// ImportUsersExcel 解析 Excel 并批量创建用户。
// 先全量校验再逐行创建；单行失败不回滚其他行，错误行明细随结果返回。
func (uc *UserIOUsecase) ImportUsersExcel(ctx context.Context, fileBytes []byte, operatorID uint32) (*ImportUsersResult, error) {
	f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("parse excel failed: %w", err)
	}
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		return nil, fmt.Errorf("read sheet failed: %w", err)
	}

	result := &ImportUsersResult{Errors: make([]*ImportRowError, 0)}
	if len(rows) <= 1 {
		return result, nil
	}

	// 预加载 部门/岗位/角色 映射
	orgByName, err := uc.departmentIDByNameMap(ctx)
	if err != nil {
		return nil, err
	}
	posByName, err := uc.positionIDByNameMap(ctx)
	if err != nil {
		return nil, err
	}
	roleIDByCode, err := uc.roleIDByCodeMap(ctx)
	if err != nil {
		return nil, err
	}

	type parsedUser struct {
		rowNo    int
		user     *userV1.User
		password string
		roleIDs  []uint32
	}

	seen := make(map[string]struct{})
	valid := make([]*parsedUser, 0, len(rows)-1)

	for i, row := range rows[1:] {
		rowNo := i + 2 // Excel 行号（含表头）
		col := func(idx int) string {
			if idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		username := col(0)
		if username == "" {
			continue // 跳过空行
		}
		result.Total++

		fail := func(msg string) {
			result.FailedCount++
			result.Errors = append(result.Errors, &ImportRowError{Row: rowNo, Username: username, Message: msg})
		}

		if _, dup := seen[username]; dup {
			fail("文件内用户名重复")
			continue
		}
		seen[username] = struct{}{}

		existResp, err := uc.userRepo.UserExists(ctx, &userV1.UserExistsRequest{Username: username})
		if err != nil {
			return nil, err
		}
		if existResp.GetExist() {
			fail("用户名已存在")
			continue
		}

		email := col(4)
		if email != "" {
			if _, err := mail.ParseAddress(email); err != nil {
				fail("邮箱格式不正确")
				continue
			}
		}

		gender, err := cnToGender(col(6))
		if err != nil {
			fail(err.Error())
			continue
		}
		status, err := cnToStatus(col(10))
		if err != nil {
			fail(err.Error())
			continue
		}

		var orgID *uint32
		if name := col(7); name != "" {
			if id, ok := orgByName[name]; ok {
				orgID = trans.Ptr(id)
			} else {
				fail(fmt.Sprintf("部门[%s]不存在", name))
				continue
			}
		}
		var posID *uint32
		if name := col(8); name != "" {
			if id, ok := posByName[name]; ok {
				posID = trans.Ptr(id)
			} else {
				fail(fmt.Sprintf("岗位[%s]不存在", name))
				continue
			}
		}

		var roleIDs []uint32
		if codes := col(9); codes != "" {
			unknown := make([]string, 0)
			for _, code := range strings.Split(codes, ",") {
				code = strings.TrimSpace(code)
				if code == "" {
					continue
				}
				if id, ok := roleIDByCode[code]; ok {
					roleIDs = append(roleIDs, id)
				} else {
					unknown = append(unknown, code)
				}
			}
			if len(unknown) > 0 {
				fail(fmt.Sprintf("角色码[%s]不存在", strings.Join(unknown, ",")))
				continue
			}
		}

		var workID *uint32
		if w := col(3); w != "" {
			var id uint32
			if _, err := fmt.Sscanf(w, "%d", &id); err == nil && id > 0 {
				workID = trans.Ptr(id)
			}
		}

		valid = append(valid, &parsedUser{
			rowNo: rowNo,
			user: &userV1.User{
				Username:   trans.Ptr(username),
				Nickname:   optionalStr(col(1)),
				Realname:   optionalStr(col(2)),
				WorkId:     workID,
				Email:      optionalStr(email),
				Mobile:     optionalStr(col(5)),
				Gender:     gender.Enum(),
				OrgId:      orgID,
				PositionId: posID,
				Status:     status.Enum(),
				Authority:  userV1.UserAuthority_CUSTOMER_USER.Enum(),
				CreateBy:   trans.Ptr(operatorID),
			},
			password: col(11),
			roleIDs:  roleIDs,
		})
	}

	for _, pu := range valid {
		created, err := uc.userRepo.Create(ctx, &userV1.CreateUserRequest{Data: pu.user})
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, &ImportRowError{Row: pu.rowNo, Username: pu.user.GetUsername(), Message: "创建用户失败"})
			continue
		}

		password := pu.password
		if password == "" {
			password = "666666"
		}
		if err = uc.userCredentialsRepo.Create(ctx, &authenticationV1.CreateUserCredentialRequest{
			Data: &authenticationV1.UserCredential{
				UserId:         created.Id,
				TenantId:       created.TenantId,
				IdentityType:   authenticationV1.IdentityType_USERNAME.Enum(),
				Identifier:     pu.user.Username,
				CredentialType: authenticationV1.CredentialType_PASSWORD_HASH.Enum(),
				Credential:     trans.Ptr(password),
				IsPrimary:      trans.Ptr(true),
				Status:         authenticationV1.UserCredentialStatus_ENABLED.Enum(),
			},
		}); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, &ImportRowError{Row: pu.rowNo, Username: pu.user.GetUsername(), Message: "设置初始密码失败"})
			continue
		}

		for _, roleID := range pu.roleIDs {
			if err = uc.roleRepo.EnsureUserRole(ctx, created.GetId(), roleID); err != nil {
				uc.log.Warnf("assign role %d to user %s failed: %v", roleID, pu.user.GetUsername(), err)
			}
		}

		result.SuccessCount++
	}

	return result, nil
}

// ---------- 映射辅助 ----------

func (uc *UserIOUsecase) departmentNameMap(ctx context.Context) (map[uint32]string, error) {
	resp, err := uc.departmentRepo.List(ctx, &pagination.PagingRequest{NoPaging: trans.Ptr(true)})
	if err != nil {
		return nil, err
	}
	m := make(map[uint32]string, len(resp.GetItems()))
	for _, d := range resp.GetItems() {
		m[d.GetId()] = d.GetName()
	}
	return m, nil
}

func (uc *UserIOUsecase) departmentIDByNameMap(ctx context.Context) (map[string]uint32, error) {
	names, err := uc.departmentNameMap(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]uint32, len(names))
	for id, name := range names {
		m[name] = id
	}
	return m, nil
}

func (uc *UserIOUsecase) positionNameMap(ctx context.Context) (map[uint32]string, error) {
	resp, err := uc.positionRepo.List(ctx, &pagination.PagingRequest{NoPaging: trans.Ptr(true)})
	if err != nil {
		return nil, err
	}
	m := make(map[uint32]string, len(resp.GetItems()))
	for _, p := range resp.GetItems() {
		m[p.GetId()] = p.GetName()
	}
	return m, nil
}

func (uc *UserIOUsecase) positionIDByNameMap(ctx context.Context) (map[string]uint32, error) {
	names, err := uc.positionNameMap(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]uint32, len(names))
	for id, name := range names {
		m[name] = id
	}
	return m, nil
}

func (uc *UserIOUsecase) roleIDByCodeMap(ctx context.Context) (map[string]uint32, error) {
	resp, err := uc.roleRepo.List(ctx, &pagination.PagingRequest{NoPaging: trans.Ptr(true)})
	if err != nil {
		return nil, err
	}
	m := make(map[string]uint32, len(resp.GetItems()))
	for _, r := range resp.GetItems() {
		if r.GetCode() != "" {
			m[r.GetCode()] = r.GetId()
		}
	}
	return m, nil
}

func genderToCN(g userV1.UserGender) string {
	switch g {
	case userV1.UserGender_MALE:
		return "男"
	case userV1.UserGender_FEMALE:
		return "女"
	default:
		return "保密"
	}
}

func cnToGender(s string) (userV1.UserGender, error) {
	switch s {
	case "", "保密", "未知":
		return userV1.UserGender_SECRET, nil
	case "男":
		return userV1.UserGender_MALE, nil
	case "女":
		return userV1.UserGender_FEMALE, nil
	default:
		return userV1.UserGender_SECRET, fmt.Errorf("性别[%s]无效，应为 男/女/保密", s)
	}
}

func statusToCN(s userV1.UserStatus) string {
	if s == userV1.UserStatus_ON {
		return "启用"
	}
	return "禁用"
}

func cnToStatus(s string) (userV1.UserStatus, error) {
	switch s {
	case "", "启用":
		return userV1.UserStatus_ON, nil
	case "禁用":
		return userV1.UserStatus_OFF, nil
	default:
		return userV1.UserStatus_ON, fmt.Errorf("状态[%s]无效，应为 启用/禁用", s)
	}
}

func optionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return trans.Ptr(s)
}
