# GitHub 热门追踪迁入 FrameWork 计划

> 目标：将 `/workspace/github-radar` 迁入 FrameWork 平台壳，**废弃独立 Quasar 壳**，前后端均走 buf 生成与平台菜单/权限。  
> 必读：`frontend-ui-style-guide.md`、`frontend-architecture-guide.md`、`backend-architecture-guide.md`。

---

## 1. 产品落点（菜单 / 路由）

| 项 | 建议值 |
|---|---|
| 菜单标题 | **github热门追踪** |
| path | `/github-radar` |
| route name | `GithubRadar` |
| component | `github-radar/RadarPage` |
| 文件 | `apps/web/src/pages/github-radar/RadarPage.vue` |
| icon | `travel_explore` 或 `radar`（Material Icons） |
| type | `MENU`（可挂在独立顶级或「工具」FOLDER 下） |

Seed 追加示例（`platformMenuSeedRecords`）：

```go
{id: <未占用>, mType: adminV1.MenuType_MENU,
 path: "/github-radar", name: "GithubRadar",
 component: "github-radar/RadarPage",
 title: "github热门追踪", icon: "travel_explore", order: 20},
```

---

## 2. 后端迁入

### 2.1 Proto

- **推荐包**：`api/radar/v1`（与现契约 `package radar.v1` 一致）
- 源文件可参考：`github-radar/backend/api/radar/v1/{trending,watch,notify,user}.proto`
- 调整 `go_package` → `framework/api/gen/go/radar/v1;radarv1`
- 现独立项目中 proto 为**契约草稿**、运行时手写 stub → 迁入后改为 **真正 buf codegen**，删除手写重复 DTO

### 2.2 分层落位

| Radar 原目录 | FrameWork 落位 |
|---|---|
| `internal/service` | `app/admin/internal/service/radar_*.go` |
| `internal/biz` | `…/biz/radar_*.go` |
| `internal/data` + migrations | `…/data` + ent schema / SQL migrate |
| `internal/scraper|ingest|enricher|scheduler|notify` | `worker` / `scheduler` 包，配置开关 |
| `cmd` | 并入 `app/admin/cmd`（或 admin 同进程挂载） |
| `configs` | 合并 `app/admin/configs`（GITHUB_TOKEN、cron、Redis TTL） |

- 在 `server` **Register** `TrendingService` / Watch / Notify HTTP
- Wire 注入；API Resource seed + `super` 授权
- 登录：**删除**独立弱鉴权 config API 公网暴露假设；改用平台 JWT + 权限

### 2.3 Buf

```bash
buf generate          # Go → api/gen/go/radar/v1 + TS → apps/web/src/services/kratos/...
```

禁止手改 gen。

---

## 3. 前端迁入

### 3.1 不做的事

- ❌ 不把 `github-radar/frontend` 整仓当子应用跑
- ❌ 不保留独立 `radar-*` CSS 变量体系替代平台 token
- ❌ 不手写 `/api/v1/trending` 绕过 kratos 客户端（合入主干时）

### 3.2 要做的事

1. 新建 `pages/github-radar/RadarPage.vue`（可由现 `RadarPage.vue` 拆逻辑迁入）
2. 本域小组件放到 `pages/github-radar/components/`（Header、WatchRail、NotifyBell、Repo 卡等）
3. **UI 重做**：使用 FrameWork
   - KPI：`.app-stat-strip` / `.app-stat-card`
   - 列表/表格：`CrudTablePage` 或 `.crud-table-card` + 全局 table token
   - 信息块：`.dash-card` / `.glass-panel`（若做大屏风仍限 screen 作用域）
   - 设置抽屉：`FormDrawer` 或 `.app-dialog-card`
   - 颜色：`--color-accent`、`--glass-*`、`--chart-series-*` 等（见样式文档）
4. Store/API：`stores` + `services/kratos` 生成的 Radar 客户端
5. i18n 词条；菜单可见性跟角色走

### 3.3 视觉验收

- [ ] 亮/暗主题与平台一致（无「另一套霓虹变量」冲突）
- [ ] 无业务 hex 魔数
- [ ] 筛选栏符合 `.app-filter-bar`（语言/topic/since 等）
- [ ] 盯盘侧栏用平台玻璃卡，而非旧 `radar-shell` 独占布局硬编码

---

## 4. 分阶段清单

| 阶段 | 内容 | 完成标准 |
|---|---|---|
| P0 | proto 迁入 + buf generate + 空 Service 注册 | `go build` 过；TS 客户端出现 |
| P1 | data/biz 迁移植库与 ListTrending | curl 带平台 token 可拉榜 |
| P2 | ingest/scheduler 接入 worker | cron 可关；token 进安全配置 |
| P3 | 菜单 seed + 前端 RadarPage 骨架 | 侧栏进入 `/github-radar` |
| P4 | UI 按样式文档重做 + watch/notify | 样式/架构检查清单全过 |
| P5 | 下线独立 frontend/backend 进程文档 | 只保留 FrameWork 启动方式 |

---

## 5. 风险与注意

- DB：独立库 `radar` vs 平台 `framework` — 建议 schema 前缀或独立 schema，迁移脚本改连平台 Postgres。
- Redis：仅作榜单短缓存；可复用平台 Redis，key 加 `radar:` 前缀。
- GitHub API 限流：保留 enricher 退避；token 不回显明文。
- 现 README 中「config API 无鉴权」**不得**带入生产平台。

---

## 6. 文档与代码索引

| 资源 | 路径 |
|---|---|
| 独立产品 | `/workspace/github-radar` |
| 设计/计划 | `github-radar/docs/superpowers/specs|plans/*` |
| 样式强制 | `/workspace/framework-docs/frontend-ui-style-guide.md` |
| 前端架构 | `/workspace/framework-docs/frontend-architecture-guide.md` |
| 后端架构 | `/workspace/framework-docs/backend-architecture-guide.md` |

---

*文档版本：2026-10-17*
