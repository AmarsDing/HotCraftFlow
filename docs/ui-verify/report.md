# FrameWork UI 验证报告

- 时间: 2026-10-16 16:07:45 (Asia/Shanghai UTC+8)
- 前端: http://127.0.0.1:9001/
- 后端: http://127.0.0.1:7788
- 账号: admin / 123456
- 方式: Playwright headless（`apps/web/run-ui-verify.mjs`）
- 截图目录: `E:\project\FrameWork\docs\ui-verify\`
- 汇总: **通过 19 / 失败 1 / 共 20**

## 验证过程中的关键修复

1. **主内容空白根因（本次）**: `MainLayout.vue` 仅在 `route.meta.layout === 'main'` 时渲染子页，但静态子路由未写 `layout`，导致 `Component` 恒为 `null`、主区空白。已改为「未设 layout 或为 main 则渲染」，并在 `/` 父路由补 `meta.layout: 'main'`。
2. 此前骨架裁剪中已做: `resolveComponent` 别名、`myNoticeService` stub、登录 `meta.layout: blank` 等。

## 通过清单

| 功能 | 路径 | 说明 | 截图 |
| --- | --- | --- | --- |
| 登录 API | `/api/v1/monitor/auth/login` | http=200，返回 token | - |
| 登录页 | `/login` | 表单可见 | 01-login.png |
| 登录提交 | → `/dashboard` | 进入应用 | 02-after-login.png |
| 仪表盘 | `/dashboard` | 骨架引导卡片 | 03-dashboard.png |
| 组织 | `/opm/organizations` | 列表+新建对话框 | 04-opm_organizations.png |
| 岗位 | `/opm/positions` | 列表+新建对话框 | 06-opm_positions.png |
| 用户 | `/opm/users` | 列表含 admin，新建对话框 | 07-opm_users.png |
| 角色 | `/permission/roles` | 列表+新建对话框 | 08-permission_roles.png |
| 菜单 | `/permission/menus` | 树表+新建对话框 | 09-permission_menus.png |
| 字典 | `/system/dicts` | 列表+新建对话框 | 10-system_dicts.png |
| 文件 | `/system/files` | 内容可见（无标准「新建」按钮） | 11-system_files.png |
| 任务 | `/system/tasks` | 列表+新建对话框 | 12-system_tasks.png |
| API 资源 | `/system/api-resources` | 列表+新建对话框 | 13-system_api-resources.png |
| 系统配置 | `/system/configs` | 列表+新建对话框 | 14-system_configs.png |
| 登录限制 | `/system/login-restrictions` | 列表+新建对话框 | 15-system_login-restrictions.png |
| 在线用户 | `/system/online-users` | 内容可见 | 16-system_online-users.png |
| 租户 | `/tenant` | 列表+新建对话框 | 17-tenant.png |
| 登录日志 | `/log/login` | 内容可见 | 18-log_login.png |
| 操作日志 | `/log/operation` | 内容可见 | 19-log_operation.png |

## 失败清单

| 功能 | 路径 | 说明 | 截图 |
| --- | --- | --- | --- |
| 部门 | `/opm/departments` | **页面可开、有表格与「新建」对话框**；关联接口 `GET /api/v1/monitor/users?noPaging=true&fieldMask=id,realname,...` 返回 **500**（`query list failed`），页面弹出 negative 通知。判定为失败。 | 05-opm_departments.png |

## 非阻塞观察

1. **侧栏 i18n**: 菜单显示为 `nav.users` / `NAV.OPM` 等 key，未落到中文（产品名「Framework 管理平台」正常）。
2. **NATS/WebSocket**: 未起 NATS 时站内消息订阅可能报错，不影响页面打开（已过滤）。
3. **登录页脚 API 端口文案**仍可能写 8888，实际代理到 7788（不影响登录）。
4. 首轮脚本曾把「仅壳+侧栏」误判为通过；已改为必须存在 `.q-page` 内容。

## 关键阻塞

1. ~~主内容区空白（layout meta）~~ — **已修，本次验证前已生效**。
2. **部门页用户下拉数据接口 500** — 阻塞部门「负责人」等依赖用户列表的交互；列表本身可用。
3. **侧栏文案未本地化** — 不影响功能，影响观感。

## 截图文件

均位于 `E:\project\FrameWork\docs\ui-verify\`：

- `01-login.png` … `19-log_operation.png`（主流程）
- `*-create.png`（部分新建对话框，若生成）
- `results.json` / `deep-probe.json`（机器可读）

未 commit。
