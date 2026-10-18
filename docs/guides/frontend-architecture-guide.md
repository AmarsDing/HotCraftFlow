# FrameWork 前端架构指导文档

> **强制约束**：牵扯到前端开发，必须同时遵循《前端开发样式文档》与本文档。  
> Mono 形态：`FrameWork/apps/web`（包名 `framework-web`）+ `apps/server`（module `framework`）。

---

## 1. 仓库与目录总览

```
FrameWork/
  apps/web/                 # Quasar + Vue3 + TS 平台壳
    src/
      boot/                 # pinia / theme / i18n / components / table-tooltip / table-wheel
      css/                  # quasar-variables + app-theme + theme/*
      layouts/              # BlankLayout / MainLayout
      pages/                # 业务页（按域分子目录）
      components/           # 共享组件（CrudTablePage、FormDrawer…）
      composables/          # 如 useTreeCrudPage
      stores/               # Pinia
      services/             # 手写门面 + request；kratos/** 为生成物
      router/               # routes.ts + 动态菜单装配
      i18n/                 # 文案
      utils/                # table-columns 等
    quasar.config.ts
  apps/server/              # 见后端文档
  infra/
  scripts/                  # buf.gen*.yaml 等
  docs/                     # 本类指导文档应落入 docs/ 或团队约定目录
```

抽样对照：`/workspace/framework-study/`、`/workspace/framework-study-zip/`。

### 1.1 Quasar 启动要点（`quasar.config.ts`）

- `boot`: `pinia`, `theme`, `i18n`, `components`, `table-tooltip`, `table-wheel`
- `css`: 入口样式链到 `app-theme` / global
- `vueRouterMode`: `history`
- 开发端口示例：`9001`；`/api` 代理到网关（如 `:7788` / `:8000`）

---

## 2. 分层职责

| 层 | 路径 | 职责 | 禁止 |
|---|---|---|---|
| Pages | `src/pages/**` | 页面编排、插槽、调用 store/composables | 直接拼 axios URL、内联魔数色 |
| Components | `src/components/**` | 可复用 UI（CRUD 壳、抽屉） | 写死某一业务 API |
| Composables | `src/composables/**` | 树形 CRUD 等逻辑复用 | |
| Stores | `src/stores/**` | 状态 + 调用 services | 复制一份 HTTP |
| Services 门面 | `src/services/*.ts`、`index.ts` | 对页面暴露领域 API | 手改 `kratos/**` |
| Services 生成 | `src/services/kratos/**` | **proto → TS HTTP 客户端** | **禁止手改** |
| Request | `src/services/request.ts` | Axios 实例、鉴权、错误 toast | |
| Router | `src/router/**` | 静态壳路由 + 动态菜单路由 | 为每个业务页硬编码重复权限逻辑 |
| CSS | `src/css/**` | Token 与模式 | 第二套主题入口 |

---

## 3. 动态菜单与 component 路径约定

### 3.1 后端菜单字段（`menu.proto` / seed）

关键字段：

| 字段 | 含义 | 示例（`platformMenuSeedRecords`） |
|---|---|---|
| `path` | 前端路由 path | `/opm/organization` |
| `name` | 路由 name | `OpmOrganization` |
| `component` | **相对 `pages/` 的组件路径**（无 `.vue`） | `opm/OrganizationPage` |
| `type` | `FOLDER` / `MENU` / `BUTTON` | |
| `meta.title` / `meta.icon` / `meta.order` | 侧栏标题、图标、排序 | |

Seed 样例：

```text
component: "DashboardPage"           → pages/DashboardPage.vue
component: "opm/OrganizationPage"    → pages/opm/OrganizationPage.vue
component: "system/ApiResourcePage"  → pages/system/ApiResourcePage.vue
```

### 3.2 前端解析规则（必须遵守）

1. 菜单 `component` 字符串映射到：`src/pages/${component}.vue`（或项目内既有的 `import.meta.glob('../pages/**/*.vue')` 查找表）。
2. **新功能模块**放在 `pages/<domain>/`，seed / 菜单里的 component 写成 `<domain>/<PageName>`。
3. 静态 `routes.ts` 仅保留：登录、布局壳、dashboard 等可 boot 的平台页、以及兜底 `not-found`。业务菜单以 **后端菜单 + 动态注册** 为准；若暂时双写静态路由，path/name/component 必须与 seed **一致**。
4. `FOLDER` 只做侧栏分组（可有 `redirect`），不挂页面组件；`MENU` 挂页面；`BUTTON` 做权限点，不进路由。

### 3.3 新模块目录约定（示例：GitHub Radar）

```
apps/web/src/pages/github-radar/
  RadarPage.vue              # 菜单 component: github-radar/RadarPage
  components/                # 仅本域组件（可选）
  composables/               # 仅本域（可选）
```

菜单建议：

| 项 | 值 |
|---|---|
| title | github热门追踪 |
| path | `/github-radar` |
| name | `GithubRadar`（或 `GithubRadarPage`，全仓统一即可） |
| component | `github-radar/RadarPage` |
| icon | 如 `radar` / `travel_explore` |

**不要**再起一个独立 Quasar 壳（禁止迁入后保留 `/workspace/github-radar/frontend` 作为运行壳）。

---

## 4. `services/kratos/**` — 生成物红线

- 生成命令：仓库根/`apps/server` 侧 `buf generate`（见 `scripts/buf.gen.yaml`、`buf.gen.ts-only.yaml`）。
- TS 插件输出：`../../web/src/services/kratos`（相对 api 目录）。
- 现有包示例：
  - `services/kratos/admin/service/v1/`
  - `services/kratos/authentication/service/v1/`
  - `services/kratos/user/service/v1/`
  - `services/kratos/file/service/v1/`
  - `services/kratos/internal_message/service/v1/`
- **禁止手改**生成文件；变更流程：改 `.proto` → `buf generate` → 提交 proto + 生成物策略（若 gitignore 则 CI/`make api` 必跑）。
- 页面 / store **禁止** `fetch('/api/v1/...')` 手写路径字符串绕过客户端（调试临时除外且不得合入）。

---

## 5. 业务页如何调 API

推荐链路：

```
Page / Composable
  → Pinia store（或 composable 直接）
    → services 门面（apps/web/src/services/index.ts 导出的 *Service / Entity）
      → services/kratos/** 生成客户端
        → request.ts（Bearer、错误拦截）
```

样板：`OrganizationPage.vue`

```ts
import { useOrganizationStore } from '../../stores';
import { useTreeCrudPage } from '../../composables/useTreeCrudPage';
import type { OrganizationEntity } from '../../services';

const { …, fetchData, onCreate, onEdit, onSubmit, onDelete } =
  useTreeCrudPage<OrganizationEntity>(useOrganizationStore());
```

约定：

1. 列表参数与后端 pagination / ent 过滤对齐：`page`、`pageSize`、`orderBy`、`formValues`；模糊字段由 `CrudTablePage` 把 `fuzzy` 输入转为 `field__icontains`。
2. 错误提示由 axios 拦截器统一弹出；页面 `catch` 可吞掉避免未处理 rejection。
3. 实体类型从 `services` 导出，不在页面重复定义与 proto 冲突的接口。

---

## 6. CrudTablePage 模式

组件：`apps/web/src/components/CrudTablePage.vue`。

### 6.1 核心 Props

| Prop | 说明 |
|---|---|
| `title` | 表头标题 |
| `columns` | `QTableColumn[]`（建议设宽；未设宽走 `withAutoColWidths`） |
| `searchFields` | `input` / `select` / `date` / `device` |
| `fetchDataFn` | `(page, pageSize, noPaging, formValues, orderBy) => { items, total }` |
| `creatable` | 显示新建按钮并发 `create` |
| `tree` / `noPaging` | 树形全量 |
| `embedded` / `fillHeight` / `grid` / `selectable` | 嵌入 Tab、定高、网格、多选 |

### 6.2 插槽

- `#body-cell-<name>`：自定义列（操作列务必 `@click.stop` 若监听了 `rowClick`）
- `#search-prepend` / `#search-actions` / `#toolbar` / `#title-extra` / `#extra`（放 FormDrawer）
- `#no-data`：可覆盖默认空态

### 6.3 与 FormDrawer

在 `#extra` 中挂载：

```vue
<FormDrawer v-model="drawerOpen" :title="drawerTitle" :saving="saving" @submit="onSubmit">
  <q-input dense outlined … />
</FormDrawer>
```

样式约束见《前端开发样式文档》§3–§6。

---

## 7. i18n 与权限菜单关系

| 主题 | 约定 |
|---|---|
| i18n | 用户可见文案走 `vue-i18n`（`t('organization.title')` 等）；菜单标题来自后端 `meta.title`，可与 i18n key 策略并存，但**侧栏以菜单数据为准** |
| 登录后菜单 | 按角色拉取菜单树 → 动态注册路由 → 侧栏渲染 |
| 权限 | `MENU`/`BUTTON` 绑定角色；API 另有 ApiResource + Casbin；前端按钮级用权限指令/辅助函数，不靠「藏 UI」代替后端鉴权 |
| 超级管理员 | seed 角色 `super`，挂全量菜单 ID |
| 本地存储 key | `framework_*`（勿再用 twin_*） |

新增页时同步：

1. 页面文件 +（可选）静态 route  
2. `platformMenuSeedRecords`（或菜单管理 UI）写入 path/name/component  
3. 角色授权 / API resource seed  
4. i18n 词条  

---

## 8. 样式与架构的强制耦合

- 新页面默认：`CrudTablePage` 或 `.app-standard-page` + 既有 card/table class。
- 颜色 / 密度 / Dialog：**只**用样式文档中的 token 与 class。
- Code review 同时检查：目录约定、是否手改 kratos、是否违反样式红线。

---

## 9. 新功能模块检查清单（前端）

- [ ] 目录：`pages/<domain>/XxxPage.vue`
- [ ] 菜单 component = `<domain>/XxxPage`，path/name 与之一致
- [ ] API 经 `services` → `kratos` 生成客户端；proto 已改且已 `buf generate`
- [ ] 列表优先 `CrudTablePage`；表单优先 `FormDrawer`
- [ ] 操作列 / 筛选栏符合样式文档
- [ ] i18n 词条已加；角色菜单与 API 资源已授权
- [ ] **未**引入独立 Quasar 工程或第二套 CSS 主题
- [ ] 已读并遵守 `frontend-ui-style-guide.md`

---

## 10. 参考路径速查

| 用途 | 路径 |
|---|---|
| 路由抽样 | `apps/web/src/router/routes.ts` |
| CRUD 壳 | `apps/web/src/components/CrudTablePage.vue` |
| 抽屉 | `apps/web/src/components/FormDrawer.vue` |
| 组织页样板 | `apps/web/src/pages/opm/OrganizationPage.vue` |
| 生成客户端 | `apps/web/src/services/kratos/**` |
| 主题入口 | `apps/web/src/css/app-theme.sass` |
| 菜单 proto | `apps/server/api/.../menu.proto`（抽样 `framework-study/proto/menu.proto`） |
| 菜单 seed | `apps/server/app/admin/internal/service/seed_service.go` → `platformMenuSeedRecords` |

---

*文档版本：2026-10-17 · 基于 FrameWork 骨架与抽样组件*
