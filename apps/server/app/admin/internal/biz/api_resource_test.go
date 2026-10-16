package biz

import (
	"context"
	"fmt"
	"io"
	"testing"

	entSQL "entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
)

// API 资源注册表单测：幂等 upsert 同步 diff 分类 + List 富化（role_count / in_openapi）。
// fake 全部手写（项目无 testify/gomock 依赖），与 license_test.go 同风格。

// ========== fake 实现 ==========

type syncUpdateRecord struct {
	id                uint32
	description       string
	module            string
	moduleDescription string
	operation         string
	delisted          bool
}

type fakeApiResourceRepo struct {
	rows        []ApiResourceSyncRow
	created     []*adminV1.CreateApiResourceRequest
	syncUpdates []syncUpdateRecord
	delistedSet map[uint32]bool // SetDelisted(id, true) 调用捕获

	roleCounts map[uint32]int
	listResp   *adminV1.ListApiResourceResponse
}

func newFakeApiResourceRepo() *fakeApiResourceRepo {
	return &fakeApiResourceRepo{delistedSet: map[uint32]bool{}}
}

func (f *fakeApiResourceRepo) Count(context.Context, []func(s *entSQL.Selector)) (int, error) {
	return len(f.rows), nil
}

func (f *fakeApiResourceRepo) List(context.Context, *pagination.PagingRequest) (*adminV1.ListApiResourceResponse, error) {
	return f.listResp, nil
}

func (f *fakeApiResourceRepo) IsExist(context.Context, uint32) (bool, error) { return true, nil }

func (f *fakeApiResourceRepo) Get(context.Context, *adminV1.GetApiResourceRequest) (*adminV1.ApiResource, error) {
	return nil, nil
}

func (f *fakeApiResourceRepo) GetApiResourceByEndpoint(context.Context, string, string) (*adminV1.ApiResource, error) {
	return nil, nil
}

func (f *fakeApiResourceRepo) Create(_ context.Context, req *adminV1.CreateApiResourceRequest) error {
	f.created = append(f.created, req)
	return nil
}

func (f *fakeApiResourceRepo) Update(context.Context, *adminV1.UpdateApiResourceRequest) error {
	return nil
}

func (f *fakeApiResourceRepo) Delete(context.Context, *adminV1.DeleteApiResourceRequest) error {
	return nil
}

func (f *fakeApiResourceRepo) ListAllForSync(context.Context) ([]ApiResourceSyncRow, error) {
	return f.rows, nil
}

func (f *fakeApiResourceRepo) UpdateSyncFields(_ context.Context, id uint32, description, module, moduleDescription, operation string, delisted bool) error {
	f.syncUpdates = append(f.syncUpdates, syncUpdateRecord{
		id: id, description: description, module: module,
		moduleDescription: moduleDescription, operation: operation, delisted: delisted,
	})
	return nil
}

func (f *fakeApiResourceRepo) SetDelisted(_ context.Context, id uint32, delisted bool) error {
	if delisted {
		f.delistedSet[id] = true
	}
	return nil
}

func (f *fakeApiResourceRepo) CountRoleBindingsByApiIDs(context.Context, []uint32) (map[uint32]int, error) {
	return f.roleCounts, nil
}

// ========== 测试基建 ==========

// newSyncUsecase 绕过 NewApiResourceUsecase（避免加载 embed 文档与播种副作用），直接注入文档端点。
func newSyncUsecase(repo ApiResourceRepo, ops []docOperation) *ApiResourceUsecase {
	uc := &ApiResourceUsecase{
		repo:       repo,
		log:        log.NewHelper(log.With(log.NewStdLogger(io.Discard), "module", "api-resource/test")),
		docOps:     ops,
		docPathSet: make(map[string]struct{}, len(ops)),
	}
	for _, op := range ops {
		uc.docPathSet[endpointKey(op.path, op.method)] = struct{}{}
	}
	return uc
}

func docOp(path, method, module, description string) docOperation {
	return docOperation{
		path:              path,
		method:            method,
		module:            module,
		moduleDescription: module + "模块",
		description:       description,
		operation:         method + " " + path,
	}
}

// ========== SyncApiResources diff 分类 ==========

// 一次构造覆盖全部分类：created / updated / unchanged / delisted / relisted / shadowed / manual_not_in_doc。
func TestApiResourceSyncDiff(t *testing.T) {
	repo := newFakeApiResourceRepo()
	repo.rows = []ApiResourceSyncRow{
		// 1. sync 行字段变化 → updated（id 稳定原地更新）
		{ID: 1, Path: "/api/v1/users", Method: "GET", Source: ApiResourceSourceSync, Description: "旧描述", Module: "user"},
		// 2. sync 行与文档一致 → unchanged
		{ID: 2, Path: "/api/v1/roles", Method: "GET", Source: ApiResourceSourceSync, Description: "角色列表", Module: "role", ModuleDescription: "role模块", Operation: "GET /api/v1/roles"},
		// 3. sync 行已 delisted 且端点回到文档 → relisted
		{ID: 3, Path: "/api/v1/menus", Method: "GET", Source: ApiResourceSourceSync, Description: "菜单列表", Module: "menu", ModuleDescription: "menu模块", Operation: "GET /api/v1/menus", Delisted: true},
		// 4. sync 行端点从文档消失 → delisted
		{ID: 4, Path: "/api/v1/legacy", Method: "POST", Source: ApiResourceSourceSync, Description: "旧接口", Module: "legacy"},
		// 5. manual 行遮蔽文档端点 → shadowed（不同步、不计下线）
		{ID: 5, Path: "/api/v1/custom", Method: "PUT", Source: ApiResourceSourceManual, Description: "人工接管", Module: "custom"},
		// 6. manual 行不在文档 → manual_not_in_doc
		{ID: 6, Path: "/api/v1/ghost", Method: "DELETE", Source: ApiResourceSourceManual, Description: "幽灵接口", Module: "ghost"},
		// 7. 已 delisted 的 sync 行端点仍不在文档 → 保持 delisted，不重复计数
		{ID: 7, Path: "/api/v1/gone", Method: "GET", Source: ApiResourceSourceSync, Description: "已下线", Module: "gone", Delisted: true},
	}

	ops := []docOperation{
		docOp("/api/v1/users", "GET", "user", "用户列表"),
		docOp("/api/v1/roles", "GET", "role", "角色列表"),
		docOp("/api/v1/menus", "GET", "menu", "菜单列表"),
		docOp("/api/v1/custom", "PUT", "custom", "自定义接口（文档侧）"),
		docOp("/api/v1/brand-new", "POST", "new", "全新端点"),
	}

	report, err := newSyncUsecase(repo, ops).SyncApiResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("SyncApiResources() err = %v", err)
	}

	// 分类计数断言
	if got := report.GetCreated(); got != 1 {
		t.Errorf("created = %d, want 1", got)
	}
	if got := report.GetUpdated(); got != 1 {
		t.Errorf("updated = %d, want 1", got)
	}
	if got := report.GetUnchanged(); got != 1 {
		t.Errorf("unchanged = %d, want 1", got)
	}
	if got := report.GetRelisted(); got != 1 {
		t.Errorf("relisted = %d, want 1", got)
	}
	if got := report.GetDelisted(); got != 1 {
		t.Errorf("delisted = %d, want 1", got)
	}
	if got := report.GetShadowed(); got != 1 {
		t.Errorf("shadowed = %d, want 1", got)
	}
	if got := report.GetManualNotInDoc(); got != 1 {
		t.Errorf("manual_not_in_doc = %d, want 1", got)
	}

	// created：新端点以 source=sync 插入
	if len(repo.created) != 1 {
		t.Fatalf("Create calls = %d, want 1", len(repo.created))
	}
	data := repo.created[0].GetData()
	if data.GetPath() != "/api/v1/brand-new" || data.GetMethod() != "POST" {
		t.Errorf("created endpoint = %s %s, want POST /api/v1/brand-new", data.GetMethod(), data.GetPath())
	}
	if data.GetSource() != ApiResourceSourceSync {
		t.Errorf("created source = %q, want sync", data.GetSource())
	}

	// updated + relisted：UpdateSyncFields 共 2 次（id=1 更新、id=3 复活），均强制 delisted=false
	if len(repo.syncUpdates) != 2 {
		t.Fatalf("UpdateSyncFields calls = %d, want 2 (updated + relisted)", len(repo.syncUpdates))
	}
	byID := map[uint32]syncUpdateRecord{}
	for _, u := range repo.syncUpdates {
		byID[u.id] = u
		if u.delisted {
			t.Errorf("UpdateSyncFields(id=%d) delisted = true, want false", u.id)
		}
	}
	if u, ok := byID[1]; !ok || u.description != "用户列表" {
		t.Errorf("updated row = %+v, want id=1 description=用户列表", u)
	}
	if _, ok := byID[3]; !ok {
		t.Errorf("relisted row id=3 missing in UpdateSyncFields")
	}

	// delisted：仅 id=4 被标记；id=7 已 delisted 不重复；manual 行（5/6）永不标记
	if len(repo.delistedSet) != 1 || !repo.delistedSet[4] {
		t.Errorf("SetDelisted = %v, want only id=4", repo.delistedSet)
	}
}

// 重复同步幂等：第二次同步全部 unchanged，报告零差异。
func TestApiResourceSyncIdempotent(t *testing.T) {
	repo := newFakeApiResourceRepo()
	repo.rows = []ApiResourceSyncRow{
		{ID: 1, Path: "/api/v1/users", Method: "GET", Source: ApiResourceSourceSync, Description: "用户列表", Module: "user", ModuleDescription: "user模块", Operation: "GET /api/v1/users"},
	}
	ops := []docOperation{docOp("/api/v1/users", "GET", "user", "用户列表")}

	report, err := newSyncUsecase(repo, ops).SyncApiResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("SyncApiResources() err = %v", err)
	}
	if report.GetCreated()+report.GetUpdated()+report.GetDelisted()+report.GetRelisted() != 0 {
		t.Errorf("idempotent sync should produce zero mutations, got %+v", report)
	}
	if report.GetUnchanged() != 1 {
		t.Errorf("unchanged = %d, want 1", report.GetUnchanged())
	}
	if len(report.GetChanges()) != 0 {
		t.Errorf("changes = %d, want 0", len(report.GetChanges()))
	}
}

// 差异报告明细上限：超过 cap 的变更计数照记，但 changes 截断。
func TestApiResourceSyncReportCap(t *testing.T) {
	repo := newFakeApiResourceRepo()
	ops := make([]docOperation, 0, syncChangeReportCap+50)
	for i := 0; i < syncChangeReportCap+50; i++ {
		path := fmt.Sprintf("/api/v1/ep-%d", i)
		ops = append(ops, docOp(path, "GET", "bulk", "批量端点"))
	}

	report, err := newSyncUsecase(repo, ops).SyncApiResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("SyncApiResources() err = %v", err)
	}
	if got := report.GetCreated(); got != uint32(syncChangeReportCap+50) {
		t.Errorf("created = %d, want %d", got, syncChangeReportCap+50)
	}
	if got := len(report.GetChanges()); got != syncChangeReportCap {
		t.Errorf("changes len = %d, want capped at %d", got, syncChangeReportCap)
	}
}

// ========== List 富化 ==========

func TestApiResourceListEnrichment(t *testing.T) {
	repo := newFakeApiResourceRepo()
	repo.listResp = &adminV1.ListApiResourceResponse{
		Items: []*adminV1.ApiResource{
			{Id: trans.Ptr(uint32(1)), Path: trans.Ptr("/api/v1/users"), Method: trans.Ptr("GET")},
			{Id: trans.Ptr(uint32(2)), Path: trans.Ptr("/api/v1/ghost"), Method: trans.Ptr("DELETE")},
		},
	}
	repo.roleCounts = map[uint32]int{1: 3}

	uc := newSyncUsecase(repo, []docOperation{docOp("/api/v1/users", "GET", "user", "用户列表")})
	resp, err := uc.List(context.Background(), &pagination.PagingRequest{})
	if err != nil {
		t.Fatalf("List() err = %v", err)
	}
	if len(resp.GetItems()) != 2 {
		t.Fatalf("items = %d, want 2", len(resp.GetItems()))
	}

	// id=1：角色绑定数 3 + 在文档中
	if got := resp.GetItems()[0].GetRoleCount(); got != 3 {
		t.Errorf("items[0].role_count = %d, want 3", got)
	}
	if !resp.GetItems()[0].GetInOpenapi() {
		t.Errorf("items[0].in_openapi = false, want true")
	}

	// id=2：无绑定不填充（零值缺省）+ 不在文档
	if resp.GetItems()[0].GetId() == 2 {
		t.Fatalf("items order unexpected")
	}
	if resp.GetItems()[1].RoleCount != nil {
		t.Errorf("items[1].role_count = %d, want nil（无绑定不填充）", resp.GetItems()[1].GetRoleCount())
	}
	if resp.GetItems()[1].GetInOpenapi() {
		t.Errorf("items[1].in_openapi = true, want false（幽灵点）")
	}
}
