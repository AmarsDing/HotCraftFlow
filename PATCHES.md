# FrameWork 迁入补丁说明（需手工合并到现有文件）

本包只新增独立文件；下列平台文件需在目标仓 **手工追加**（勿整文件覆盖，避免冲掉本地改动）。

生成物 **禁止手改**：`apps/server/api/gen/**`、`apps/web/src/services/kratos/**`。

---

## 1. Proto 已就位

路径：`apps/server/api/radar/v1/{trending,watch,notify,config}.proto`  
package：`radar.service.v1`  
HTTP 前缀：`/api/v1/radar/...`（避免与平台 `/api/v1/notifications`、`/api/v1/config` 冲突）

确认 `buf.yaml` / `buf.gen.yaml` 的 `inputs` 覆盖 `api/radar`（与 `api/admin` 同级）。若使用目录扫描，通常无需改配置。

```bash
cd apps/server   # 或仓库约定的 api 目录
buf generate
# 或 make api
```

生成后应出现类似：

- `api/gen/go/radar/service/v1/*.pb.go`（及 `_http.pb.go` / `_grpc.pb.go`）
- `apps/web/src/services/kratos/radar/...`（TS 客户端）

然后将 `service/radar_*.go` 中的临时 DTO（`radar_dto.go`）改为 gen 类型，并删除 `radar_dto.go`。

---

## 2. `internal/service/service.go` ProviderSet

在 `wire.NewSet(` 内追加：

```go
	NewRadarTrendingService,
	NewRadarWatchService,
	NewRadarNotifyService,
	NewRadarConfigService,
```

---

## 3. `internal/biz/biz.go` ProviderSet

追加：

```go
	NewRadarTrendingUsecase,
	// Demo 读库绑定：wire 可用 wire.Bind 或提供 NewDemo 工厂
	ProvideDemoRadarTrendingReader,
```

在 `biz/radar_trending.go` 旁可加：

```go
func ProvideDemoRadarTrendingReader() RadarTrendingReader {
	return &DemoRadarTrendingReader{}
}
```

（若已有 wire 绑定惯例，按仓内 Organization 等样例改。）

---

## 4. `internal/server/rest.go` 注册

### 4.A buf generate 之后（推荐）

1. `NewRESTServer` 参数增加：

```go
	radarTrendingSvc *service.RadarTrendingService,
	radarWatchSvc *service.RadarWatchService,
	radarNotifySvc *service.RadarNotifyService,
	radarConfigSvc *service.RadarConfigService,
```

2. import gen 包（路径以 generate 结果为准，常见为）：

```go
	radarV1 "framework/api/gen/go/radar/service/v1"
```

3. 在其它 `Register*HTTPServer` 附近追加：

```go
	radarV1.RegisterTrendingServiceHTTPServer(srv, radarTrendingSvc)
	radarV1.RegisterWatchServiceHTTPServer(srv, radarWatchSvc)
	radarV1.RegisterRadarNotifyServiceHTTPServer(srv, radarNotifySvc)
	radarV1.RegisterRadarConfigServiceHTTPServer(srv, radarConfigSvc)
```

> 注意：generate 后 Service 方法签名需改为 gen 的 `*ListTrendingRequest` 等，并实现接口。  
> 当前骨架用临时 DTO，**generate 后必须改签名** 才能通过编译。

### 4.B generate 前临时 HTTP（可选烟雾）

若需在未 generate 时先打通前端，可临时用 `patches/radar_http_stub.go.snippet` 里的 `HandleFunc` 注册（见同名文件），generate 后 **删除 stub**，改走 4.A。

---

## 5. `seed_service.go` 菜单

在 `platformMenuSeedRecords()` 切片末尾追加（id **60** 当前平台 seed 未占用；若冲突请改）：

```go
		{id: 60, mType: adminV1.MenuType_MENU, path: "/github-radar", name: "GithubRadar",
			component: "github-radar/RadarPage", title: "github热门追踪", icon: "travel_explore", order: 20},
```

若菜单已 seed（`menuCount != 0` 会跳过），需：

- 清菜单表后重启，或
- 在管理端「菜单」手工新增同名字段，或
- 增加幂等 upsert（按 path）——超出本补丁范围。

API Resource：generate 后建议把 `/api/v1/radar/*` 扫入 API 资源并授权给 `super`。

---

## 6. Wire 重跑

```bash
cd apps/server/app/admin/cmd
wire
# 或 make wire
go build -o /tmp/framework-admin ./...
```

---

## 7. 前端 `componentMap.ts`

在 `componentMap` 对象中追加一行（完整补丁文件见 `apps/web/src/router/componentMap.patch.ts`）：

```ts
  'github-radar/RadarPage': () => import('../pages/github-radar/RadarPage.vue'),
```

---

## 8. 前端 API 客户端

`RadarPage.vue` 暂用 `fetch('/api/v1/radar/...')` + 本地类型。  
**buf generate 后**：改为 `services/kratos` 生成客户端，删除页面内手写 URL；**禁止手改 kratos 生成文件**。

---

## 9. 已知缺口

| 项 | 状态 |
|---|---|
| ent schema / 真库 | TODO，现为内存 demo |
| ingest / cron / Redis 缓存 | 未迁入；后续 worker |
| 平台 JWT 取 user_id 绑定盯盘 | TODO |
| API Resource seed | generate 后补 |
| i18n 词条 | 页面中文写死，可再抽 i18n |
| 独立 github-radar 进程 | 迁入后应下线 |

