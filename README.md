# HotCraftFlow

热点驱动的全媒体内容矩阵：**采集**（GitHub 与国内外平台）→ **追踪/分析** → **二次创作** → **海外媒体发布**。

```
HotCraftFlow/
  apps/web/      # Quasar admin UI
  apps/server/   # Go / Kratos API
  doc/           # PRD（权威需求）
  docs/systems/  # IC / TT / MIA / MME / MAP 工程文档
```

## 五子系统

| 缩写 | 职责 |
|------|------|
| IC | 情报收集 → `IntelItem` |
| TT | 热点追踪 → `TopicCandidate` |
| MIA | 市场影响分析 → `ImpactCard` |
| MME | 短视频矩阵混剪 → `VariantSet` |
| MAP | 媒体自动化发布 |

数据流：`IC → TT → (MIA | MME) → MAP`，表现数据回流 TT/IC。

## Quick start

### Backend

1. Edit placeholders in:
   - `apps/server/app/admin/config/auth.yaml`
   - `apps/server/app/admin/config/server.yaml`
   - `apps/server/conf/data.yaml`
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

- JWT keys and DB passwords in committed YAML are **placeholders**.
- `apps/server/api/gen` is included so a fresh clone can build without regenerating protos.
- Do not hand-edit `api/gen/**`, Ent generated code (except `schema/`), or `apps/web/src/services/kratos/**`.
