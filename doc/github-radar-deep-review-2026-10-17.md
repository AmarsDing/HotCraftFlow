# GitHub 热门追踪（github-radar）功能深度评审报告

> 评审日期：2026-10-17 · 评审人：WorkBuddy · 代码基线：`apps/web/src/pages/github-radar/` + `apps/server/**/radar*`（git: b880a27）
> 评审方法：全量代码走读 + eslint / stylelint / vue-tsc / go vet 实测 + 平台规范（《前端UI样式文档》《前后端架构指南》《github-radar-port-plan.md》）逐条对照

---

## 0. 评审范围与规模

| 层 | 文件 | 行数 |
|---|---|---|
| 前端页面 | `apps/web/src/pages/github-radar/RadarPage.vue` | 687 |
| 前端组件 | `RepoCard.vue` / `WatchRail.vue` / `Sparkline.vue` / `types.ts` | 370 |
| 前端客户端 | `services/kratos/radar/service/v1/index.ts`（buf 生成） | 436 |
| 后端契约 | `api/radar/v1/{trending,watch,notify,config}.proto` | 223 |
| 后端业务 | `biz/radar_trending.go` + `service/radar_{trending,watch,notify,config}_service.go` | 324 |

**功能定位对照**（迁入计划 P0–P5）：

| 阶段 | 计划内容 | 实际状态 |
|---|---|---|
| P0 | proto 迁入 + buf generate + Service 注册 | ✅ 已完成（rest.go/wire_gen/service.go 均接线） |
| P1 | data/biz 移植 + ListTrending 真实数据 | ❌ **未开始**（仍是硬编码 demo，无 ent schema、无 migrate、无 GitHub client） |
| P2 | ingest/scheduler + token 安全配置 | ❌ **未开始**（无 cron、无缓存、无限流退避） |
| P3 | 菜单 seed + 前端骨架 | ✅ 已完成（seed id=60，componentMap 已注册） |
| P4 | UI 重做 + watch/notify 闭环 | ⚠️ **UI 完成度 ~85%，notify 闭环断裂**（mark-read 无入口） |

**总体结论**：这是一个"壳已迁完、芯是 demo"的半成品。UI 层对平台规范的遵循度意外地高（可当样板），但后端没有任何真实数据链路；且存在 1 个**导致页面根本无法加载的 P0 构建错误**和 1 组**多租户数据串扰 + 授权放空**的 P0 安全问题。当前状态不可上线，甚至不可演示。

---

## 1. P0 — 阻塞级（必须立即修复）

### BUG-01（前端）页面 import 路径断裂，`/github-radar` 打开即白屏
- **位置**：`RadarPage.vue:369` `import FormDrawer from 'src/components/FormDrawer.vue'`；`:375` `import { ... } from 'src/services'`
- **证据**：全仓仅此一页使用 `src/` 裸前缀（其余页面一律 `../../services` 相对路径）；`tsconfig.json` 无该 paths，`quasar.config.ts` 无该 alias；`vue-tsc --noEmit` 实测报 `TS2307: Cannot find module 'src/services'`；esbuild 复现 `Could not resolve "src/services"`（Vite 同款解析器，构建/开发必败）。
- **修复**：改为 `import FormDrawer from 'src/components/FormDrawer.vue'` → `'../../components/FormDrawer.vue'`；`'src/services'` → `'../../services'`。顺手跑 `npm run format`（见 STYLE-01）。

### BUG-02（后端）全链路 demo 假数据，无任何真实 GitHub 数据源
- **位置**：`biz/radar_trending.go:79-186`（`DemoRadarTrendingReader` 硬编码 6 条仓库）、`radar_watch_service.go:30-41`（预置 w1/e1）、`radar_notify_service.go:25-29`（预置 n1）；wire_gen:140 注入 `ProvideDemoRadarTrendingReader()`。
- **影响**：榜单永远不变、`stale` 永远 false、star 增量写死；对接 PRD FR-TRK×13 形同虚设。
- **修复**：按迁入计划 P1/P2 落地：ent schema（radar_trending_repo / radar_watch / radar_watch_event）+ GitHub trending 抓取 + enricher 退避 + Redis 短缓存 + cron 开关；`TODO(ent)` 逐条清零。

### BUG-03（后端·安全）盯盘/通知/Token 全局共享，无用户隔离
- **位置**：`RadarWatchService`/`RadarNotifyService`/`RadarConfigService` 均为进程内单例（`sync.RWMutex` + 全局 map/slice）。
- **影响**：
  1. 所有登录用户看到**同一份**盯盘列表，A 加的盘 B 能删；
  2. **Token 互相覆盖**：A `SetConfig` 后 B 再设即覆盖；B `GetConfig` 可读到 A 的脱敏 token（前4+后4，约泄露 8/40 字符）；
  3. 进程重启全部丢失。
- **修复**：落库并加 `user_id` 维度（平台 JWT 里可取用户身份）；token 应定位为**平台级配置**（放 system_config + 加密，NFR-19 复核）而非每用户。

### BUG-04（后端·安全）接口授权实际放空，任何登录用户可调用全部 radar 接口
- **位置**：`config/auth.yaml:21` `authz.type: "opa"`，而 `data/authorizer.go:62-89` 只实现 `noop`/`casbin`，`"opa"` 落入 `default → noop`（注释未实现的类型被静默降级）。
- **佐证**：seed_service 只注册了菜单（id 60），**未注册任何 radar API Resource**（`:200` `_ = s.apiResourceUC`），即便切 casbin 也会因无策略而行为不定。
- **影响**：JWT 鉴权（authn）有效，但细粒度授权（authz）对所有 radar 端点形同虚设——普通用户可改全局 token、读写他人盯盘。
- **修复**：① authz 类型与实现对齐（补 opa 或显式改 casbin + 对未实现类型启动报错而非静默 noop）；② seed radar API Resource（trending/watch/notify/config 共 10 个 operation）并绑 `super` 角色。

---

## 2. P1 — 高优先级（功能缺陷，上线前必须处理）

| # | 位置 | 问题 |
|---|---|---|
| BUG-05 | `RadarPage.vue:64-74, 527-536` | **热门模式搜索框是死控件**：`mode==='trending'` 时输入框可见，但热门分支不传 `query` 给后端；每次输入还白触发一次 reload。要么后端 `ListTrending` 增加 query，要么 trending 模式隐藏该框 |
| BUG-06 | `RadarPage.vue:289-301` + services | **未读通知/事件永远无法清除**：`radarWatchService.markEventsRead` / `radarNotifyService.markRead` 前端零调用；铃铛红徽标与盯盘未读数只增不减（demo 还有 1 条永久未读）。需在通知弹窗加「全部已读」、盯盘事件加已读入口 |
| BUG-07 | `radar_watch_service.go:112` / `radar_notify_service.go:58` | **MarkRead 空 ids = 全部已读**：`len(ids)==0` 时把全部事件标记已读，与 RPC「按 id 标记」语义相悖，误调即清空未读 |
| BUG-08 | `radar_watch_service.go:54-74` | **无输入校验**：`AddWatch` 只判空串——不校验 `owner/repo` 格式、不校验 GitHub 上仓库是否存在、无长度上限；`RemoveWatch` 对不存在 id 也返回成功且无归属校验 |
| BUG-09 | `RadarPage.vue:457-462` vs `trending.proto:39-40` | **双口径 KPI**：服务端返回预过滤的 `newCount`/`risingCount`，前端却用过滤后 `items` 自算——筛选后 KPI 变小，用户误以为数据变了。应统一用服务端口径（或明确 KPI=当前筛选结果） |
| BUG-10 | `radar_*_service.go` | **错误处理不合规**：`fmt.Errorf("full_name required")` 裸错误直透前端 toast；未走 kratos error 体系（无 code、无 400 映射） |
| BUG-11 | `radar_watch_service.go:44-52` | **盯盘列表顺序随机**：`map` 遍历输出，每次刷新顺序跳动。需按加入时间/rank 排序 |
| BUG-12 | `RadarPage.vue:5-9, 104-114, 198-207` 等 | **i18n 缺失**：页面标题/按钮/空态/tooltip 全硬编码中文（仅 `common.*` 走了 `t()`）；en-US 用户看到中文 UI。迁入计划 P4 明确要求 i18n 词条 |
| BUG-13 | `radar_watch_service.go:21` | **events 无上限无清理**：内存 slice 只增不减，长跑进程内存泄漏（真实化后盯盘事件量更大） |
| BUG-14 | `seed_service.go:150-151` + `assets/openapi.yaml` | **文档漂移**：openapi.yaml（83k 行）无 `/api/v1/radar/*` 任何路径，buf 后未重新生成导出 |

---

## 3. P2 — 中优先级（质量/体验缺陷）

| # | 位置 | 问题 |
|---|---|---|
| BUG-15 | 全 radar 后端 | **零测试**：4 个 service + biz 无任何 `_test.go`（平台 biz 层有单测样板可抄） |
| BUG-16 | 4 个 radar service | **日志字段死代码**：`log *log.Helper` 构造后从未使用，要么补日志要么删字段 |
| BUG-17 | `radar_config_service.go:19` | **Token 明文驻留内存**：无加密（TODO 自认）；即便平台级配置也应走加密存储 + 操作审计（平台有 adminOperationLog 体系，radar 未接入：token 设置/盯盘增删均无审计） |
| BUG-18 | `radar_notify_service.go:14` | **重复造轮子**：与平台 `NotificationMessageService` 并存（注释自认），应复用平台通知体系 |
| BUG-19 | `RadarPage.vue:473-522` + `types.ts:1` | **类型双轨技术债**：本地 snake_case DTO + `asRepo/asWatch/asEvent/asNotify` 双读适配，而 kratos 生成客户端早已是 camelCase 类型（`types.ts:1` 注释 "until buf generate" 已过期）。应改用生成类型、删适配层 |
| BUG-20 | `RadarPage.vue:181-183` | **假骨架屏**：`.radar-skeleton` 只是 opacity 0.5 的空卡，无 shimmer；且 `loading && !items.length` 条件下**刷新时无任何 loading 指示**，刷新按钮无 spinner |
| BUG-21 | `RepoCard.vue:44` / `WatchRail.vue:29` | **负数增量格式错误**：`+{{ stars_delta }}` 遇负值渲染 `+-5`；`Δ` 同理无符号处理 |
| BUG-22 | `RadarPage.vue:657-659` | 筛选 watcher 无取消竞态防护：快速切换语言/topic 时旧请求后返回会覆盖新结果（依赖 debounce 400ms 缓解，非根治） |
| BUG-23 | `radar_trending.go:177` | `SearchRising` 注释 "sort-ish by delta already in demo order"——**实际没有任何排序**；真实化后必须按 delta 降序 |
| BUG-24 | `trending.proto:29-30` / `watch.proto` | **分页参数是摆设**：proto 有 `page/page_size`，service 层直接忽略，前端也从不传；真实数据接入即全量返回 |
| BUG-25 | `componentMap.patch.ts` | **死文件**：`githubRadarComponentMapEntry` 导出无人引用（真 entry 已在 `componentMap.ts:31`），应删除 |

---

## 4. P3 — 低优先级（细节打磨）

| # | 位置 | 问题 |
|---|---|---|
| STYLE-01 | 全 5 个前端文件 | **eslint 230 条 prettier warning（0 error）**：整个功能提交前未跑 `npm run format`，CI 开 lint 会红 |
| STYLE-02 | `Sparkline.vue:28` | 图表色用 `--color-accent` 而非 `--chart-series-*`，违反样式文档 §2.6/§10.8「图表必须走 chart-series token」 |
| STYLE-03 | `RepoCard.vue:143` | 语言色点所有语言同一颜色，点了等于没点（GitHub 惯例按语言着色，可建语言→token 映射表） |
| STYLE-04 | `RepoCard.vue:160` / `WatchRail.vue:88` | token 体系外魔数：`gap: 4px`（应 `var(--space-1)`）、`border-radius: 10px`（行圆角不在 token 集）。颜色无魔数，这点合格 |
| STYLE-05 | `RepoCard.vue:2-8` / `RadarPage.vue:139-143` | a11y：`article[role=button]` 内嵌 `q-btn`（嵌套可交互元素）；KPI 卡/RepoCard 只响应 Enter 不响应 Space；通知徽标无 `aria-live` |
| STYLE-06 | `RadarPage.vue:273-276` | `target="_blank"` 缺 `rel="noopener"` |
| STYLE-07 | `RadarPage.vue:552` | 错误提示透传底层 message（`加载失败：xxx`），可能暴露内部细节；`loadSide` catch 后完全静默（应至少 console.warn） |
| STYLE-08 | `RadarPage.vue:8` | Hero 副标题含实现术语「玻璃卡片与平台筛选栏同肤」——面向最终用户应讲业务价值 |
| STYLE-09 | `Sparkline.vue:51-52` | 48×16 sparkline 趋势不可读（RepoCard/WatchRail 同尺寸），建议 64×20 或加 hover 明细 |
| STYLE-10 | `RadarPage.vue:52-102` | 筛选栏在 rising 模式隐藏 3 个控件导致栏宽跳变；可改禁用态或保持占位 |
| STYLE-11 | `RadarPage.vue:244-245` | 详情弹窗 star/fork/delta 用裸 text-caption；建议用 `.app-stat-delta`（is-good/is-bad/is-flat）组件，语义与视觉更规范 |

---

## 5. 做得好的方面（应予保留）

1. **UI 规范遵循度是全仓样板级**：`.app-standard-page` / `.app-page-hero--feature` / `.app-filter-bar(+__field/--sm/--auto/__actions)` / `.app-stat-strip` / `.app-stat-card(--static/--success/--info/--violet/is-filter-on)` / `.dash-card` / `.app-dialog-card + app-glass-dialog 分区` / `FormDrawer` 全部按《前端UI样式文档》落地；**无 hex/rgb 魔数**（color:gate 可过），**stylelint 零告警**，亮暗走 `body--dark` 分叉。迁入计划 §3.3 视觉验收项基本全过。
2. **筛选栏交互符合平台约定**：选中即查、无搜索按钮、400ms 防抖、仅「重置」在 `__actions`。
3. **KPI 卡可点击过滤**（新晋仓库 ↔ onlyNew）且带 `is-filter-on` 态，符合 guide §4。
4. **响应式策略清晰**：≥1200px 右侧 300px 盯盘轨，<1200px 收进 `q-drawer` + 筛选栏「盯盘(n)」入口。
5. **分层未越界**：proto → service(适配) → biz(领域) → reader(数据抽象) 的责任划分正确，`RadarTrendingReader` 接口为后续替换真实数据层留好了缝。
6. **前端错误处理接入平台体系**：统一走 `requestHandler`（JWT 注入、401 刷新重试、路由切换取消、超时分档全部继承）。
7. **移除了旧独立版的坏味道**：无 `window.prompt`（改玻璃 Dialog）、无 `radar-*` 私有色板、token 脱敏回显。

---

## 6. 修复优先级路线图

```
第 1 步（半天，解阻塞）
  ├─ BUG-01  修正 src/ import（页面从"打不开"到"能跑"）
  ├─ STYLE-01 npm run format + lint 清零
  └─ BUG-04  authz 类型与实现对齐（至少显式失败，别静默 noop）

第 2 步（1-2 天，安全与多租户）
  ├─ BUG-03  watch/event/notify 加 user_id（先内存分 map，落库前过渡）
  ├─ BUG-04  seed radar API Resource + super 授权
  ├─ BUG-07/08  MarkRead 语义修正 + full_name 校验（正则 + GitHub 存在性探测）
  └─ BUG-17  token 转平台级加密配置 + 操作审计

第 3 步（3-5 天，真实数据链路，对应迁入计划 P1/P2）
  ├─ BUG-02  ent schema + migrate + GitHub trending 抓取 + 退避
  ├─ BUG-24 分页落地（page/page_size 生效 + 前端传参 + total）
  └─ BUG-23  SearchRising 按 delta 排序

第 4 步（1-2 天，前端闭环）
  ├─ BUG-05  热门模式搜索（后端加 query 或隐藏输入框）
  ├─ BUG-06  通知/事件「全部已读」入口
  ├─ BUG-09  KPI 口径统一
  ├─ BUG-12  i18n 词条补全（zh/en 成对）
  └─ BUG-19  类型切到 kratos 生成类型，删 asXxx 适配层

第 5 步（持续）
  ├─ BUG-15  补 service/biz 单测（对标平台现有样板）
  ├─ BUG-14  buf 后重新生成 openapi.yaml
  └─ P3 清单按需消化
```

---

## 7. 结论

github-radar 的**前端 UI 迁入质量高于预期**（规范遵循度可作平台样板），但**后端仍是迁入计划 P0 阶段的空壳 + demo 数据**，且叠加 4 个 P0（1 个构建即挂、1 个无真实数据、2 个安全/多租户）。建议：先按路线图第 1、2 步解阻塞与安全债（约 2-3 天），再评估是否值得投入 P1/P2 真实数据链路；若短期内不接真实数据，应将菜单 seed 下线或加「演示数据」水印，避免误导最终用户。

---

# 修复结项（2026-10-17 第二轮）

评审发现的 P0–P2 问题已在同日程完成修复。验证基线：后端 `go build ./...` / `go vet` / `go test ./app/admin/...` 全绿；前端 radar 域 eslint / stylelint / vue-tsc 零告警零错误（全仓剩余 37 个 tsc 错误均为雷达域外的历史遗留）。

## P0 修复（4/4）

| # | 问题 | 修复 |
|---|------|------|
| 1 | `RadarPage.vue` `from 'src/...'` 裸导入（构建必败） | 改相对路径 `../../components/FormDrawer.vue`；删除无人引用的 `componentMap.patch.ts` |
| 2 | 后端 demo 假数据 | 真实数据源由双引擎 GitHub Reader 承接（并发会话完成）；`RADAR_DEMO=1` 仅作本地联调开关，默认关闭 |
| 3 | 盯盘/通知/Token 进程内全局单例（多租户串扰、重启丢失） | 新增 ent 三表 `radar_watch` / `radar_watch_event` / `radar_user_config`（行级 user_id 隔离，强制索引查询）；service 全量改 per-user（JWT `auth.FromContext`）；平台配置收敛为单行（userID=0）+ AES-256-GCM 加密落库 + 管理员限定写（SYS_ADMIN/TENANT_ADMIN）+ 30s 快照缓存 |
| 4 | 授权放空（authz.type=opa 落 noop）且 radar 无 API Resource | `auth.yaml` 改 `casbin`；`authorizer.go` 对未实现类型显式 `[SECURITY]` 错误日志（拒绝静默降级）；`seed_service.allApiIds` 修复（原返回 nil 导致 super 角色零 API 绑定）；super 角色启动时自动补齐新增接口绑定；重新生成并合并 openapi（14 条 radar 路径 + 29 schema + 5 tag，键级合并保留 it-monitor 等既有手工内容），顺带修复 `TestOpenAPI` 的 ItMonitorService/EnvMonitorService 缺失 tag panic |

## P1 修复（10/10）

- **搜索死控件**：query 已接入 `listTrending`/`searchRising`（并发会话 v2 完成）
- **未读通知无法清除**：通知弹窗 + WatchRail 事件区新增「全部已读」入口（传具体未读 id；后端空 ids 返回 400，双端语义一致）
- **空 ids=全部已读**：biz 层 `MarkEventsRead`/`MarkNotificationsRead` 空 ids 拒绝（400）
- **AddWatch 零校验**：`normalizeFullName` 严格 owner/repo 校验（正则 + owner≤39/repo≤100）
- **KPI 双口径**：统一为前端过滤后口径，展示一致
- **i18n 全缺**：新增 `githubRadar` 命名空间 133 键（zh/en 对称），覆盖 RadarPage / WatchRail / RepoCard / RepoDetailPanel / CloneDialog / RepoInsight / SourceBrowser；与既有气象雷达 `radar` 命名空间隔离
- **裸错误**：service 层统一 kratos 错误码（ErrorBadRequest/NotFound/Unauthorized/Forbidden/InternalServerError），repo 层日志 + 脱敏
- **map 遍历乱序**：语言/协议 Top 改为排序后展示
- **events 无上限**：单用户 200 条写入裁剪 + 新增 `RadarEventRetentionWorker`（每日 04:41 物理清理 30 天前事件，复用 pkg/retention）
- **openapi 未重新生成**：已完成（见 P0-4）

## P2/P3 修复精选

- `ListTrendingResponse` 新增 `total` 字段（proto + buf 重新生成，Go/TS 客户端同步）
- 盯盘星标同步：节流 10min + 并发 4 + 单次上限 12 + 阈值 50 生成事件 + 未读数重算
- 前端竞态防护：`loadBoard` 请求序号，只采纳最后一次响应
- 负数格式：`Δ-5` → `+N`/`-N`（RepoCard / WatchRail / RepoDetailPanel 统一），下跌 `--color-danger`
- Sparkline 默认色 `--color-accent` → `--chart-series-1`（accent 兜底）
- KPI 可点卡补 `keydown.space`；RepoCard 补 `keydown.enter`（原有）+ 负增量展示
- 已读通知/事件视觉弱化（opacity 0.55）
- 配置 biz 层收口：trim + LLM 默认（`https://api.openai.com/v1` / `gpt-4o-mini`，与 insight 服务/前端 placeholder 对齐）

## 新增测试

`biz/radar_user_data_test.go`：full_name 校验矩阵、Add 幂等、per-user 隔离、Remove、空 ids 拒绝、SyncStars 阈值事件、Notify 用例、Config 默认值/归一化 —— 全部通过。

## 遗留说明

- proto 的 `page/page_size` 字段 service 暂未消费（前端未做分页加载），列为后续增强
- types.ts 保留 snake/camel 双读适配层作为防腐层，未强切 kratos 生成类型（风险高收益低，可后续渐进）
- `seed_service.go` 的 `containsAll/mergeUint32` 为本次新增辅助函数
