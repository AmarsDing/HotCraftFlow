package biz

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"

	entSQL "entgo.io/ent/dialect/sql"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/assets"

	adminV1 "framework/api/gen/go/admin/v1"
)

// ApiResourceSourceManual/ApiResourceSourceSync API 资源来源取值（ent schema 默认 manual）。
const (
	ApiResourceSourceManual = "manual"
	ApiResourceSourceSync   = "sync"
)

// syncChangeReportCap 差异报告明细上限（防止超大报告撑爆响应）。
const syncChangeReportCap = 200

// ApiResourceRepo API 资源仓储端口，由 data 层 ApiResourceRepo 实现并经 wire.Bind 绑定。
type ApiResourceRepo interface {
	Count(ctx context.Context, whereCond []func(s *entSQL.Selector)) (int, error)
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListApiResourceResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *adminV1.GetApiResourceRequest) (*adminV1.ApiResource, error)
	GetApiResourceByEndpoint(ctx context.Context, path, method string) (*adminV1.ApiResource, error)
	Create(ctx context.Context, req *adminV1.CreateApiResourceRequest) error
	Update(ctx context.Context, req *adminV1.UpdateApiResourceRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteApiResourceRequest) error

	// ListAllForSync 返回全部资源的轻量行（同步 upsert diff 用）。
	ListAllForSync(ctx context.Context) ([]ApiResourceSyncRow, error)
	// UpdateSyncFields 仅更新文档派生字段与 delisted（不碰 id/status/create_by）。
	UpdateSyncFields(ctx context.Context, id uint32, description, module, moduleDescription, operation string, delisted bool) error
	// SetDelisted 标记/取消疑似下线。
	SetDelisted(ctx context.Context, id uint32, delisted bool) error
	// CountRoleBindingsByApiIDs 统计各资源的角色直绑数。
	CountRoleBindingsByApiIDs(ctx context.Context, ids []uint32) (map[uint32]int, error)
}

// ApiResourceSyncRow 同步 diff 用的轻量行（biz 层拥有，避免依赖 data/proto DTO）。
type ApiResourceSyncRow struct {
	ID                uint32
	Path              string
	Method            string
	Source            string
	Description       string
	Module            string
	ModuleDescription string
	Operation         string
	Delisted          bool
}

// docOperation OpenAPI 文档中解析出的单个端点（同步的权威输入）。
type docOperation struct {
	path              string
	method            string
	module            string
	moduleDescription string
	description       string
	operation         string
}

// ApiResourceUsecase API 资源业务用例。
// 构造期保持原 service 的播种副作用：表为空时按 OpenAPI 文档全量同步一次。
// 同步为幂等 upsert：id 稳定（授权绑定永不悬空）、人工行与人工启停不受同步影响。
type ApiResourceUsecase struct {
	repo ApiResourceRepo
	log  *log.Helper

	// docOps 构建期聚合 OpenAPI 文档解析出的端点清单；docPathSet 为 "path|method" 索引。
	// 文档加载失败时保持 nil（in_openapi 不计算、同步报错），不阻断服务启动。
	docOps     []docOperation
	docPathSet map[string]struct{}
}

func NewApiResourceUsecase(repo ApiResourceRepo, logger log.Logger) *ApiResourceUsecase {
	uc := &ApiResourceUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "api-resource/usecase/admin-service")),
	}

	if err := uc.loadOpenAPIDoc(); err != nil {
		uc.log.Warnf("加载 OpenAPI 文档失败（同步与 in_openapi 标记暂不可用）: %v", err)
	}

	ctx := context.Background()
	if count, _ := repo.Count(ctx, []func(s *entSQL.Selector){}); count == 0 {
		if _, err := uc.SyncApiResources(ctx, &emptypb.Empty{}); err != nil {
			uc.log.Errorf("构造期播种同步 API 资源失败: %v", err)
		}
	}

	return uc
}

// loadOpenAPIDoc 解析 embed 的聚合 OpenAPI 文档为端点清单。
// 描述回退链：operation.Description → operation.Summary → operationID（消灭展示层的空描述）。
func (uc *ApiResourceUsecase) loadOpenAPIDoc() error {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(assets.OpenApiData)
	if err != nil {
		return fmt.Errorf("load OpenAPI document failed: %w", err)
	}
	if doc == nil || doc.Paths == nil {
		return fmt.Errorf("OpenAPI document paths is nil")
	}

	ops := make([]docOperation, 0, len(doc.Paths.Map()))
	pathSet := make(map[string]struct{}, len(doc.Paths.Map()))

	for path, pathItem := range doc.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			// 每个操作独立解析所属模块，避免上一个操作的 tag 残留到无 tag 的操作上
			var module string
			var moduleDescription string
			if len(operation.Tags) > 0 {
				// 顶层 tags 漏声明该 tag 时 Get 返回 nil（曾因手工追加路径未同步 tags 导致 admin 启动 panic），
				// 回退用 operation 自带的 tag 字符串作模块名，不再阻断启动
				if tag := doc.Tags.Get(operation.Tags[0]); tag != nil {
					module = tag.Name
					moduleDescription = tag.Description
				} else {
					module = operation.Tags[0]
				}
			}

			description := operation.Description
			if description == "" {
				description = operation.Summary
			}
			if description == "" {
				description = operation.OperationID
			}

			ops = append(ops, docOperation{
				path:              path,
				method:            method,
				module:            module,
				moduleDescription: moduleDescription,
				description:       description,
				operation:         operation.OperationID,
			})
			pathSet[endpointKey(path, method)] = struct{}{}
		}
	}

	uc.docOps = ops
	uc.docPathSet = pathSet
	return nil
}

func endpointKey(path, method string) string {
	return path + "|" + method
}

func (uc *ApiResourceUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListApiResourceResponse, error) {
	resp, err := uc.repo.List(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Items) == 0 {
		return resp, nil
	}

	// 富化：角色直绑数（删除影响面）+ 是否存在于构建期文档（幽灵点可见）
	ids := make([]uint32, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.GetId() != 0 {
			ids = append(ids, item.GetId())
		}
	}
	if counts, cerr := uc.repo.CountRoleBindingsByApiIDs(ctx, ids); cerr != nil {
		uc.log.Warnf("统计角色绑定数失败（role_count 不填充）: %v", cerr)
	} else {
		for _, item := range resp.Items {
			if n, ok := counts[item.GetId()]; ok && n > 0 {
				item.RoleCount = trans.Ptr(uint32(n))
			}
		}
	}

	if uc.docPathSet != nil {
		for _, item := range resp.Items {
			_, ok := uc.docPathSet[endpointKey(item.GetPath(), item.GetMethod())]
			item.InOpenapi = trans.Ptr(ok)
		}
	}

	return resp, nil
}

func (uc *ApiResourceUsecase) Get(ctx context.Context, req *adminV1.GetApiResourceRequest) (*adminV1.ApiResource, error) {
	return uc.repo.Get(ctx, req)
}

func (uc *ApiResourceUsecase) Create(ctx context.Context, req *adminV1.CreateApiResourceRequest) error {
	return uc.repo.Create(ctx, req)
}

func (uc *ApiResourceUsecase) Update(ctx context.Context, req *adminV1.UpdateApiResourceRequest) error {
	return uc.repo.Update(ctx, req)
}

func (uc *ApiResourceUsecase) Delete(ctx context.Context, req *adminV1.DeleteApiResourceRequest) error {
	return uc.repo.Delete(ctx, req)
}

// SyncApiResources 按 OpenAPI 文档幂等 upsert 同步资源，返回差异报告。
// 不变式：
//  1. id 稳定——同 (path,method) 的 sync 行原地更新，ap_org_role_api 授权绑定永不悬空；
//  2. 人工数据不被同步触碰——manual 行遮蔽同端点 sync 行；status（人工启停）永不改写；
//  3. 文档中消失的 sync 行仅标记 delisted（可逆），留待人工确认后删除（删除才级联清理绑定）。
func (uc *ApiResourceUsecase) SyncApiResources(ctx context.Context, _ *emptypb.Empty) (*adminV1.SyncApiResourcesResponse, error) {
	if uc.docOps == nil {
		if err := uc.loadOpenAPIDoc(); err != nil {
			uc.log.Errorf("加载 OpenAPI 文档失败: %v", err)
			return nil, adminV1.ErrorInternalServerError("load OpenAPI document failed")
		}
	}

	rows, err := uc.repo.ListAllForSync(ctx)
	if err != nil {
		return nil, err
	}

	byEndpoint := make(map[string]ApiResourceSyncRow, len(rows))
	for _, row := range rows {
		byEndpoint[endpointKey(row.Path, row.Method)] = row
	}

	report := &adminV1.SyncApiResourcesResponse{}
	changes := make([]*adminV1.SyncChangeItem, 0)
	appendChange := func(changeType, path, method, module string) {
		if len(changes) >= syncChangeReportCap {
			return
		}
		changes = append(changes, &adminV1.SyncChangeItem{
			ChangeType: trans.Ptr(changeType),
			Path:       trans.Ptr(path),
			Method:     trans.Ptr(method),
			Module:     trans.Ptr(module),
		})
	}

	matchedSync := make(map[uint32]bool, len(rows))

	for _, op := range uc.docOps {
		key := endpointKey(op.path, op.method)
		row, exists := byEndpoint[key]

		if exists && row.Source == ApiResourceSourceManual {
			// 手工行遮蔽：用户接管该端点的数据维护，跳过且不计入下线
			report.Shadowed = trans.Ptr(report.GetShadowed() + 1)
			appendChange("shadowed", op.path, op.method, op.module)
			continue
		}

		if exists {
			matchedSync[row.ID] = true
			fieldsChanged := row.Description != op.description ||
				row.Module != op.module ||
				row.ModuleDescription != op.moduleDescription ||
				row.Operation != op.operation

			switch {
			case row.Delisted:
				// 端点回到文档：复活（顺带对齐文档字段）
				if uerr := uc.repo.UpdateSyncFields(ctx, row.ID, op.description, op.module, op.moduleDescription, op.operation, false); uerr != nil {
					uc.log.Warnf("复活 API 资源失败 %s %s: %v（跳过）", op.method, op.path, uerr)
					continue
				}
				report.Relisted = trans.Ptr(report.GetRelisted() + 1)
				appendChange("relisted", op.path, op.method, op.module)
			case fieldsChanged:
				if uerr := uc.repo.UpdateSyncFields(ctx, row.ID, op.description, op.module, op.moduleDescription, op.operation, false); uerr != nil {
					uc.log.Warnf("更新 API 资源失败 %s %s: %v（跳过）", op.method, op.path, uerr)
					continue
				}
				report.Updated = trans.Ptr(report.GetUpdated() + 1)
				appendChange("updated", op.path, op.method, op.module)
			default:
				report.Unchanged = trans.Ptr(report.GetUnchanged() + 1)
			}
			continue
		}

		// 新端点登记
		if cerr := uc.repo.Create(ctx, &adminV1.CreateApiResourceRequest{
			Data: &adminV1.ApiResource{
				Path:              trans.Ptr(op.path),
				Method:            trans.Ptr(op.method),
				Module:            trans.Ptr(op.module),
				ModuleDescription: trans.Ptr(op.moduleDescription),
				Description:       trans.Ptr(op.description),
				Operation:         trans.Ptr(op.operation),
				Source:            trans.Ptr(ApiResourceSourceSync),
			},
		}); cerr != nil {
			uc.log.Warnf("插入 API 资源失败 %s %s: %v（跳过）", op.method, op.path, cerr)
			continue
		}
		report.Created = trans.Ptr(report.GetCreated() + 1)
		appendChange("created", op.path, op.method, op.module)
	}

	// 文档中消失的 sync 行：标记 delisted（可逆），不删行、不碰绑定
	for _, row := range rows {
		if row.Source != ApiResourceSourceSync || matchedSync[row.ID] || row.Delisted {
			continue
		}
		if derr := uc.repo.SetDelisted(ctx, row.ID, true); derr != nil {
			uc.log.Warnf("标记下线失败 %s %s: %v（跳过）", row.Method, row.Path, derr)
			continue
		}
		report.Delisted = trans.Ptr(report.GetDelisted() + 1)
		appendChange("delisted", row.Path, row.Method, row.Module)
	}

	// 手工行漂移提示：登记了但不在文档中（手写 handler 未补 yaml / 已下线 / 误录）
	for _, row := range rows {
		if row.Source != ApiResourceSourceManual {
			continue
		}
		if _, ok := uc.docPathSet[endpointKey(row.Path, row.Method)]; !ok {
			report.ManualNotInDoc = trans.Ptr(report.GetManualNotInDoc() + 1)
		}
	}

	report.Changes = changes

	uc.log.Infof("API 资源同步完成: 新建 %d, 更新 %d, 无变化 %d, 下线 %d, 复活 %d, 遮蔽 %d, 手工未登记 %d",
		report.GetCreated(), report.GetUpdated(), report.GetUnchanged(),
		report.GetDelisted(), report.GetRelisted(), report.GetShadowed(), report.GetManualNotInDoc())
	return report, nil
}
