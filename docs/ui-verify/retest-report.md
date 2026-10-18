# UI Retest 2026-10-16 (department 500 + sidebar i18n)

## API
- PASS `GET /api/v1/monitor/users?noPaging=true&fieldMask=id,realname,nickname,orgId` → **HTTP 200**
  - Sample: `{"items":[{"id":1,"nickname":"...","realname":"..."}],"total":1}`

## Playwright
- PASS login → /dashboard
- PASS sidebar: no raw `nav.*` / `NAV.*` keys; q-item labels include 仪表盘/组织权限/组织/部门/岗位/用户/权限/系统/租户/日志…
- PASS department create: users list request **200**; dialog opens (新建 / 取消 / 保存)

## Notes
- Drawer `innerText` may be empty under Quasar; assert via `.q-item` texts.
- Admin restarted with `apps/server/bin/admin.exe` (PID listening :7788).
