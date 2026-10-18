# 将本补丁应用到 `E:\project\FrameWork`

## 0. 解压覆盖

在 **FrameWork 仓库根**（含 `apps/`、`docs/` 的目录）执行：

```powershell
# 假设 framework-port.tgz 已拷到本机
cd E:\project\FrameWork
tar -xzf path\to\framework-port.tgz
```

或解压 `framework-port/` 目录后，将其中相对路径文件复制进 FrameWork（**不要**覆盖整个 `apps/web/src/router/componentMap.ts`，只按 PATCHES 追加一行）。

本包新增（相对 FrameWork 根）：

```
apps/server/api/radar/v1/*.proto
apps/server/app/admin/internal/service/radar_*.go
apps/server/app/admin/internal/biz/radar_trending.go
apps/web/src/pages/github-radar/**
apps/web/src/router/componentMap.patch.ts   # 参考，非直接替换
docs/guides/*.md
PATCHES.md
APPLY.md
patches/radar_http_stub.go.snippet
```

## 1. 必读指导文档（强制）

- `docs/guides/frontend-ui-style-guide.md`
- `docs/guides/frontend-architecture-guide.md`
- `docs/guides/backend-architecture-guide.md`
- `docs/guides/github-radar-port-plan.md`

凡改前端必须遵守 UI + 架构两份；凡改后端必须遵守后端文档。**禁止手改** `api/gen/**` 与 `services/kratos/**`。

## 2. 手工合并（见 PATCHES.md）

1. `componentMap.ts` 增加 `'github-radar/RadarPage'`
2. `seed_service.go` 的 `platformMenuSeedRecords` 增加菜单 **github热门追踪**（id=60）
3. `service.go` / `biz.go` ProviderSet 注册 Radar 构造函数
4. （可选）generate 前用 stub HTTP；generate 后改 `rest.go` Register*

## 3. buf generate

```bash
cd apps/server   # 以仓库 Makefile / scripts 为准
buf generate
# 或 make api
```

确认 Go gen 与 `apps/web/src/services/kratos` 出现 radar 客户端。  
然后把 `radar_*.go` 方法签名改为 gen 类型，删除 `radar_dto.go`，删掉临时 HTTP stub。

## 4. wire + 编译

```bash
cd apps/server/app/admin/cmd
wire
go build ./...
```

## 5. 重启并验收

1. 启动平台后端 + `apps/web`（按 FrameWork 既有方式）
2. 登录后侧栏应出现 **github热门追踪**（若菜单已 seed 过需清库或手工加菜单）
3. 打开 `/github-radar`：筛选栏 + 卡片墙 + 右盯盘；主题亮/暗与平台一致
4. Network：请求前缀为 `/api/v1/radar/...`

## 6. 回滚

删除本包新增路径，并还原 ProviderSet / rest / seed / componentMap 三处追加即可。
