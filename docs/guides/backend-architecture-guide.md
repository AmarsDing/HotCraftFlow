# FrameWork 后端开发指导文档

> **强制约束**：后端新增/修改业务必须遵循本文分层与生成流程；**禁止手改** proto 生成代码与 ent 生成代码。  
> Go module：`framework`（原 `twinserver`）。入口应用：`apps/server/app/admin`。

---

## 1. 仓库结构

```
FrameWork/apps/server/
  go.mod                        # module framework
  api/                          # .proto 源（admin/v1、authentication、user…）
    gen/go/                     # buf 生成 Go（go_package_prefix: framework/api/gen/go）
  app/admin/
    cmd/                        # main
    internal/
      conf/                     # 配置结构
      server/                   # HTTP/gRPC/注册、中间件挂载
      service/                  # 接口适配层（含 seed_service.go）
      biz/                      # 用例 Usecase / 领域端口
      data/                     # 仓储实现 + ent schema / 生成代码
        ent/                    # **生成物为主，schema 手写、其余生成**
      middleware/
      core/
      worker/                   # 异步任务等
  pkg/                          # 跨应用共享库（crypto、jwt、entgo 工具等）
```

抽样：`/workspace/framework-study/server/go.mod`、`seed_service.go`；目录说明：`framework-study-zip/server/internal-*.txt`。

平台保留 proto 示例（`proto-list.txt`）：`menu`、`organization`、`user`、`role`、`api_resource`、`dict`、`task`、`tenant`、日志与通知等 **admin/v1** 平台能力。

---

## 2. Kratos 分层

```
api/*.proto
  ── buf generate ──► api/gen/go/**          （Go：pb + http + grpc + errors + validate）
                 └──► apps/web/.../kratos/** （TS HTTP 客户端）

HTTP/gRPC 请求
  → server（路由注册、中间件）
    → service（DTO/proto 进出，调用 usecase）
      → biz（业务规则，依赖 Repository 接口）
        → data（ent / Redis / 外部 API）
```

| 层 | 放什么 | 不放什么 |
|---|---|---|
| `api` proto | 契约、HTTP 注解、字段行为 | 业务逻辑 |
| `service` | 实现 `XxxServer`，参数校验转调 | SQL、ent 细节 |
| `biz` | Usecase、领域策略、Authorizer 端口 | 直接依赖 `ent.Client` 细节（经 repo 接口） |
| `data` | Repo 实现、ent schema、查询 | HTTP 细节 |
| `server` | 创建 HTTP/gRPC Server、Wire 注册服务 | 业务分支 |
| `conf` | yaml/proto 配置结构 | |

依赖注入：Google Wire（`providers` / `wire.go` 惯例）。新增服务需接入 provider 与 `server` 注册。

---

## 3. Buf 生成

### 3.1 配置文件（抽样）

| 文件 | 作用 |
|---|---|
| `scripts/buf.gen.yaml`（或 `api/buf.gen.yaml`） | 全量：go / go-grpc / go-http / go-errors / validate + **typescript-http → web** |
| `scripts/buf.gen.ts-only.yaml` | 仅重生前端 TS 客户端 |
| managed | `go_package_prefix` = `framework/api/gen/go` |

`buf.gen.yaml` 插件要点：

- `protoc-gen-go` / `go-grpc` / `go-http` / `go-errors` / `validate` → `gen/go`
- `protoc-gen-typescript-http` → `../../web/src/services/kratos`

### 3.2 命令约定

```bash
# 在 api 或脚本约定目录
buf generate
# 或 make api / make proto（以仓库 Makefile 为准）
# 仅前端：
buf generate --template buf.gen.ts-only.yaml
```

改 proto 后：**必须**重新 generate，再编译 server 与 web。

### 3.3 禁止手改

| 产物 | 路径 | 规则 |
|---|---|---|
| Go pb | `api/gen/go/**` | 禁止手改 |
| TS 客户端 | `apps/web/src/services/kratos/**` | 禁止手改 |
| ent 生成 | `app/admin/internal/data/ent/**`（除 `schema/` 手写） | 禁止手改；改 schema 后 `go generate` / ent 命令重生 |

---

## 4. Ent

- Schema 手写：`internal/data/ent/schema/*.go`
- 生成代码：`ent` 根包及各 entity 子包、`migrate`、`predicate` 等
- 业务查询封装在 `data` 的 `*_repo.go`，供 `biz` 调用
- 迁移策略遵循仓库既有 Atlas/ent migrate 流程；**不要**手工改生成的 migrate 细节绕过 schema

现有实体示例：`menu`、`role`、`user`、`organization`、`department`、`apiresource`、`dict`、日志表等。

---

## 5. 菜单 Seed 与 API Resource Seed

文件：`app/admin/internal/service/seed_service.go`。

### 5.1 启动行为

`NewSeedService` 构造时调用 `seedAll`：

1. 菜单数为 0 时执行 `seedMenus`（`platformMenuSeedRecords`）
2. `ResetSequence`
3. `seedRoles`（`super`）、组织/部门/岗位/用户、字典分类
4. `authorizer.ResetPolicies`

### 5.2 `platformMenuSeedRecords`

每条：`id, parentID, mType, path, redirect, name, component, title, icon, order`。

```go
{id: 11, parentID: 10, mType: adminV1.MenuType_MENU,
 path: "/opm/organization", name: "OpmOrganization",
 component: "opm/OrganizationPage", title: "组织", icon: "apartment", order: 1},
```

**新增业务菜单**：在该切片追加记录（选未占用 id；FOLDER 与子 MENU 成对），保证：

- `path` = 前端路由
- `component` = `pages` 相对路径（无 `.vue`）
- 角色 seed / 管理台给角色勾选新菜单

### 5.3 API Resource

- 平台有 `ApiResource` 实体与 `ApiResourcePage`
- Seed 中 `allApiIds` / 资源同步应在接入新 HTTP 路由后更新，并赋给 `super`（或同步扫描注册表的既有机制）
- Casbin 策略在 seed 末尾 `ResetPolicies`；改完权限相关 seed 后确认策略重生

---

## 6. HTTP 服务注册

惯例路径（以现有 admin 为准）：

1. 实现 `service` 结构体，嵌入/实现生成的 `XxxServiceHTTPServer` 或注册函数
2. 在 `internal/server/http.go`（或等价文件）调用 `adminV1.RegisterXxxServiceHTTPServer(srv, svc)`
3. Wire 注入 `NewXxxService` → usecase → repo
4. 网关/前端代理前缀常为 `/api/v1/monitor/...`（见 `menu.proto` 的 `google.api.http`）

新业务可：

- **方案 A**：`api/radar/v1/*.proto` 独立 package，admin 进程内注册 Radar HTTP 服务  
- **方案 B**：挂在 `admin/v1` 下独立 service（仍单独 proto 文件，避免单文件膨胀）

推荐 GitHub Radar 用 **方案 A**（与现有 radar 契约 `package radar.v1` 对齐）。

---

## 7. 新业务模块落地步骤（模板）

以「github热门追踪」为例，检查清单见 §9。

### 7.1 Proto

```
apps/server/api/radar/v1/
  trending.proto
  watch.proto
  notify.proto
  user.proto   # 或复用平台用户，不重复造登录
```

- `option go_package` 纳入 `framework/api/gen/go/radar/v1;radarv1`
- HTTP 路径与现网契约对齐，例如：`GET /api/v1/trending`
- 跑 `buf generate`（Go + TS）

### 7.2 Biz / Data / Service

```
internal/biz/radar_*.go          # TrendingUsecase 等
internal/data/radar_*.go         # 仓储；ent schema 如需落库
internal/service/radar_*.go      # 实现 generated server
internal/server/…                # Register
internal/worker 或 scheduler     # 抓取/ingest cron（可参考独立 github-radar 的 ingest）
```

### 7.3 Seed

- `platformMenuSeedRecords` 增加 MENU：title=`github热门追踪`，path=`/github-radar`，component=`github-radar/RadarPage`
- API resource：为 trending/watch/notify 等路由登记并授予 `super`

### 7.4 配置

- `configs` 增加 GitHub token、cron、Redis 缓存等（可移植自 `/workspace/github-radar/backend/configs`）
- **不要**把未鉴权的 config API 暴露到公网；并入 FrameWork 后走平台登录 + 权限

---

## 8. 与前端的契约

| 后端 | 前端 |
|---|---|
| proto HTTP 注解 | `services/kratos/**` 同路径方法 |
| 菜单 seed component | `pages/<component>.vue` |
| ApiResource + Casbin | 按钮/路由权限 |
| pagination / field mask | CrudTablePage `fetchDataFn` 参数 |

前端架构与样式文档对调用方式有强制要求。

---

## 9. 检查清单（后端）

- [ ] `go.mod` module 为 `framework`；新 import 路径正确
- [ ] 只改手写层：`api/*.proto`、`biz`、`data`（schema/repo）、`service`、`server`、`conf`
- [ ] **未**手改 `api/gen/go`、`data/ent` 生成文件、`web/.../kratos`
- [ ] `buf generate` 已跑通；`go build ./app/admin/cmd` 通过
- [ ] HTTP 已 Register；Wire 已接入
- [ ] 菜单 seed：path / name / component / title / icon 齐全
- [ ] API resource + 角色权限已更新；必要时 `ResetPolicies`
- [ ] 敏感配置（GitHub token）不入库明文、不打日志
- [ ] 若含定时任务：放 worker/scheduler，可配置开关（如 cron=`off`）

---

## 10. 参考路径速查

| 用途 | 路径 |
|---|---|
| Module | `apps/server/go.mod` → `module framework` |
| Seed | `app/admin/internal/service/seed_service.go` |
| 菜单契约 | `api/.../menu.proto` |
| Buf | `scripts/buf.gen.yaml`、`buf.gen.ts-only.yaml` |
| Ent | `app/admin/internal/data/ent/` |
| 独立 Radar 参考实现 | `/workspace/github-radar/backend`（ingest/biz/data；迁入时改 module 与注册方式） |

---

*文档版本：2026-10-17 · 基于 FrameWork admin 骨架与抽样 seed/buf*
