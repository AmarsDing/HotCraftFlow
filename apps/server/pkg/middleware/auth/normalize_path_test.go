package auth

import "testing"

// 路由模板参数名归一：snake_case → gnostic 小驼峰；单词参数、静态段、自定义动词不受影响。
func TestNormalizePathTemplate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"多词参数 camel 化", "/api/v1/media/mme/tasks/{task_id}/expand", "/api/v1/media/mme/tasks/{taskId}/expand"},
		{"role_id", "/api/v1/monitor/roles/{role_id}/depts", "/api/v1/monitor/roles/{roleId}/depts"},
		{"user_id", "/api/v1/monitor/sessions/{user_id}", "/api/v1/monitor/sessions/{userId}"},
		{"单词参数原样", "/api/v1/media/mme/tasks/{id}", "/api/v1/media/mme/tasks/{id}"},
		{"自定义动词不动", "/api/v1/media/mme/variant-sets/{id}:render", "/api/v1/media/mme/variant-sets/{id}:render"},
		{"无参数原样", "/api/v1/media/mme/doctor", "/api/v1/media/mme/doctor"},
		{"静态段下划线不动", "/api/v1/foo_bar/items/{item_id}", "/api/v1/foo_bar/items/{itemId}"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizePathTemplate(c.in); got != c.want {
				t.Fatalf("normalizePathTemplate(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
