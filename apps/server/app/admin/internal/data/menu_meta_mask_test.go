package data

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	adminV1 "framework/api/gen/go/admin/v1"
)

// TestClassifyMetaMask 回归「菜单改名保存不生效」：
// 前端表单 buildUpdateMask 只产出顶层键（含整体 "meta"），旧实现仅识别 meta.* 细粒度
// 路径，metaPaths 为空 → updateMetaField changed=false 不写 SQL，title/icon/order/
// authority/hideInMenu 等 meta 子字段修改被静默丢弃。整体 "meta" 必须走 SetMeta 替换。
func TestClassifyMetaMask(t *testing.T) {
	mask := func(paths ...string) *fieldmaskpb.FieldMask {
		return &fieldmaskpb.FieldMask{Paths: paths}
	}
	cases := []struct {
		name      string
		fm        *fieldmaskpb.FieldMask
		wantWhole bool
		wantPaths []string
	}{
		{"nil mask = full update", nil, true, nil},
		{"form whole-meta mask", mask("name", "type", "path", "status", "meta"), true, nil},
		{"status-only toggle", mask("status"), false, nil},
		{"seed fine-grained mask", mask("path", "name", "meta.title", "meta.icon", "meta.order", "meta.authority", "meta.hideInMenu"),
			false, []string{"title", "icon", "order", "authority", "hideInMenu"}},
		{"mixed whole + fine-grained", mask("meta", "meta.title"), true, []string{"title"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			whole, paths := classifyMetaMask(c.fm)
			if whole != c.wantWhole {
				t.Fatalf("wholeMeta = %v, want %v", whole, c.wantWhole)
			}
			if len(paths) != len(c.wantPaths) {
				t.Fatalf("metaPaths = %v, want %v", paths, c.wantPaths)
			}
			for i := range paths {
				if paths[i] != c.wantPaths[i] {
					t.Fatalf("metaPaths = %v, want %v", paths, c.wantPaths)
				}
			}
		})
	}
}

// TestWholeMetaMaskSurvivesProtojson 锁定调用链前提：表单提交的 updateMask 经
// protojson 解码后，整体 "meta" 路径必须原样存活（JSONSnakeCase 不含下划线不转换），
// 且 fieldmaskpb.IsValid 对 Menu.meta 整体路径与 seed 的 camelCase meta.* 路径均通过。
// 若 RouteMeta 未来改成 snake_case 字段声明，seed mask 与该校验将同时失效，测试兜底。
func TestWholeMetaMaskSurvivesProtojson(t *testing.T) {
	var req adminV1.UpdateMenuRequest
	body := `{"data": {"name": "Collector", "meta": {"title": "监控采集"}}, "updateMask": "name,meta"}`
	if err := protojson.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("protojson.Unmarshal: %v", err)
	}
	whole, _ := classifyMetaMask(req.UpdateMask)
	if !whole {
		t.Fatalf("wholeMeta = false, protojson 解码后整体 meta 路径丢失: %v", req.UpdateMask.GetPaths())
	}
	req.UpdateMask.Normalize()
	if !req.UpdateMask.IsValid(req.Data) {
		t.Fatalf("IsValid rejected whole-meta mask: %v", req.UpdateMask.GetPaths())
	}

	// seed 细粒度 mask（进程内构造，不经 JSON）须继续通过 IsValid
	seedMask := &fieldmaskpb.FieldMask{Paths: []string{
		"parent_id", "type", "path", "redirect", "name", "component", "status",
		"meta.title", "meta.icon", "meta.order", "meta.authority", "meta.hideInMenu",
	}}
	seedMask.Normalize()
	if !seedMask.IsValid(&adminV1.Menu{}) {
		t.Fatalf("IsValid rejected seed fine-grained mask: %v", seedMask.GetPaths())
	}
}
