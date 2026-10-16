# FrameWork

Platform skeleton with frontend + backend only.

```
FrameWork/
  apps/web/      # Quasar (framework-web) admin UI
  apps/server/   # Go / Kratos admin API (module: framework)
```

## Quick start

### Backend

1. Edit placeholders in:
   - `apps/server/app/admin/config/auth.yaml` (`CHANGE_ME_DEV_JWT_SECRET`)
   - `apps/server/app/admin/config/server.yaml` (`CHANGE_ME_DEV_JWT_SECRET`)
   - `apps/server/conf/data.yaml` (`password=CHANGE_ME`)
2. Provide Postgres + Redis matching those configs.
3. From `apps/server/app/admin`:

```bash
go run ./cmd
```

Default HTTP: `http://127.0.0.1:7788`

### Frontend

```bash
cd apps/web
npm install
npx quasar dev
```

## Notes

- JWT keys and DB passwords in committed YAML are **placeholders** — replace before any real deploy.
- `apps/server/api/gen` is included so a fresh clone can build without regenerating protos.
