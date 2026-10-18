# FrameWork 前端开发样式文档

> **强制约束**：凡触及前端页面 / 组件 / 样式开发，必须遵守本文档与《前端架构指导文档》。评审与合并前按文末检查清单自检。  
> 源码基准：`apps/web/src/css/`（本仓库抽样见 `/workspace/framework-study/css/`、`/workspace/framework-study-zip/css/`）。

---

## 1. 主题入口与文件职责

| 文件（FrameWork 路径约定） | 职责 |
|---|---|
| `apps/web/src/css/quasar-variables.sass` | 构建期 Quasar Sass 变量（亮色 Apple 系默认） |
| `apps/web/src/css/app-theme.sass` | **唯一** design-token 聚合入口；`@import ./theme/*` |
| `theme/_color-palette.sass` | 头像 / Agent / 行业渐变调色板（允许 hex 定义） |
| `theme/_css-vars-light.sass` | 亮色 `:root` CSS 变量 |
| `theme/_css-vars-dark.sass` | 暗色 `body.body--dark` CSS 变量 |
| `theme/_controls.sass` | 按钮 / 徽章 / Material 静态色映射 |
| `theme/_component-density.sass` | 全站控件密度（表格/字段/按钮高度） |
| `theme/_crud-tables.sass` | CRUD 表格、筛选栏、操作列、双栏卡 |
| `theme/_dash-card.sass` | 仪表盘信息卡 `.dash-card` |
| `theme/_stat-cards.sass` | KPI 统计卡 `.app-stat-card` |
| `theme/_glass-dialog.sass` | Dialog 毛玻璃全局层 + `.app-dialog-card` |
| `theme/_page-patterns.sass` | 标准页 / Hero / 分栏等页面模式 |
| `theme/_screen-glass.sass` | 大屏画布 `.screen-canvas` / `.glass-panel` |
| `theme/_color-palette.sass` + 上表 | 新色只能加 token，禁止业务页硬编码 |

**禁止**新增第二套全局 CSS 入口；新 partial 必须挂到 `app-theme.sass`。

---

## 2. 配色体系（亮 / 暗）

### 2.1 构建期品牌色（`quasar-variables.sass`）

| Sass 变量 | 亮色值 | 说明 |
|---|---|---|
| `$primary` / `$accent` | `#007AFF` | Apple Blue |
| `$secondary` | `#5856D6` | Apple Purple |
| `$positive` | `#34C759` | |
| `$negative` | `#FF3B30` | |
| `$info` | `#5AC8FA` | |
| `$warning` | `#FF9500` | |
| `$dark` | `#1D1D1F` | |

暗色运行时由 `--q-*` 覆盖，**不要**再写一套 Quasar 编译色。

### 2.2 核心语义 Token（业务首选）

| Token | 亮色（`:root`） | 暗色（`body.body--dark`） | 用途 |
|---|---|---|---|
| `--color-accent` | `#007AFF` | `#4DD8E8` | 强调 / 激活 / 霓虹青 |
| `--color-accent-hover` | `#0051D5` | `#7AE5F0` | |
| `--color-accent-soft` | `rgba(0,122,255,.12)` | `rgba(77,216,232,.16)` | 软底 |
| `--color-success` | `#34C759` | `#3FE0A0` | |
| `--color-warning` | `#FF9500` | `#FFAF4D` | |
| `--color-danger` | `#FF3B30` | `#FF5E7A` | |
| `--color-info` | （见 light 文件） | `#60A5FA` | |
| `--color-text-primary` | `#1D1D1F` | `var(--tech-text-primary)` ≈ `#D0DCE8` | |
| `--color-text-secondary` | `#86868B` | `var(--tech-text-secondary)` | |
| `--color-text-heading` | （light） | `var(--tech-text-heading)` ≈ `#E0ECFF` | |
| `--color-text-tertiary` | | `var(--tech-text-tertiary)` | |
| `--color-icon-muted` | `#C7C7CC` | 映射 muted | 操作列默认图标色 |
| `--q-primary` | `#007AFF` | `#4DD8E8` | Quasar `color="primary"` |
| `--q-positive` / `--q-negative` / `--q-warning` / `--q-info` | Apple 系 | 霓虹版 | |

### 2.3 玻璃 / 画布 Token

| Token | 说明 |
|---|---|
| `--canvas-base` | 页底画布 |
| `--glass-surface` / `--glass-surface-hover` | 表格容器、表头、悬停 |
| `--glass-elevated` | 卡片提升层（CRUD 卡、dash-card） |
| `--glass-border` / `--glass-border-hover` | 描边 |
| `--glass-blur-default` / `--glass-blur-elevated` | 模糊半径 |
| `--glass-inner-highlight` | 内高光 inset |
| `--gradient-page` | 暗色标准页底渐变 |
| `--interaction-surface-hover` | 幽灵按钮悬停底 |
| `--shadow-accent-focus` | 焦点环 |

### 2.4 间距 / 字号 Token

| Token | 值 |
|---|---|
| `--space-1` … `--space-16` | 4 / 8 / 12 / 16 / 20 / 24 / 32 / 48 / 64 px |
| `--text-xs` … `--text-xl` | 12 / 13 / 14 / 16 / 20 / 25 px |
| `--layout-max-width` | `1440px` |
| `--sidebar-width` / `--sidebar-mini-width` | `232px` / `64px` |

### 2.5 调色板（`_color-palette.sass`）

仅用于头像 / Agent / 行业 monogram，**不要**当业务强调色：

- `--palette-avatar-0` … `--palette-avatar-9`
- `--agent-palette-0` … `--agent-palette-8`（暗色有更亮变体）
- `--palette-industry-N-start` / `-end`

### 2.6 告警 / 图表 / 大屏（按需）

- 告警：`--color-alarm-info|warning|critical|emergency` 及对应 `-soft`
- 孪生状态：`--twin-status-*`
- ECharts：`--chart-series-1` … `-8`，`--chart-axis-label` 等（全站图表强制走 token）
- 大屏（`_screen-glass.sass`）：`--screen-primary`、`--glass-bg`、`--glass-text` 等；作用域限 `.screen-canvas` / `.screen-theme-*`

---

## 3. Table（CRUD）

### 3.1 必须使用的结构 / Class

| Class / 组件 | 用途 |
|---|---|
| `CrudTablePage` | 管理列表页标准壳（搜索 + 表 + 工具栏） |
| `.crud-page` | 页根 |
| `.crud-search-card` | 筛选区卡片（玻璃 elevated） |
| `.crud-table-card` | 表格卡片 |
| `.app-filter-bar` | 筛选栏 flex：条件左、按钮右 |
| `.app-filter-bar__field` / `--sm` / `--lg` / `--auto` | 控件定宽 200 / 140 / 280 / auto |
| `.app-filter-bar__actions` | 右侧操作区（含重置） |
| `.crud-actions` + `.crud-actions__btn` | 行操作幽灵图标按钮组 |
| `.crud-actions__btn--danger` | 删除等危险操作悬停 |
| `.crud-split-card` | 角色/字典等双栏分割卡 |
| `.crud-empty` / `__icon` | 空态 |

全局表格视觉（`_crud-tables.sass`）：

- `table-layout: fixed`；长文本 ellipsis；截断靠 `title` / `boot/table-tooltip`
- 表头：`--glass-surface` + blur + 强调色下沿
- 斑马纹：`color-mix(... accent 4%)`；hover：`--glass-surface-hover`
- 容器背景：`--glass-surface`（禁止暗色回落到 Quasar 近黑）

### 3.2 操作列黄金约定

```vue
<div class="crud-actions">
  <q-btn class="crud-actions__btn" flat dense round icon="edit" @click="onEdit(row)">
    <q-tooltip>{{ t('common.edit') }}</q-tooltip>
  </q-btn>
  <q-btn class="crud-actions__btn crud-actions__btn--danger" flat dense round icon="delete">
    <q-tooltip>{{ t('common.delete') }}</q-tooltip>
    <q-popup-confirm @confirm="onDelete(row)">…</q-popup-confirm>
  </q-btn>
</div>
```

**禁止**：业务页对操作列使用 `outline` / 实底 / sticky 黑条覆写。样板：`OrganizationPage`、注释中的 `CredentialPage`。

### 3.3 筛选栏约定

- **选中即查**，无独立「搜索」按钮；输入框防抖约 400ms（`CrudTablePage`）
- 筛选用 **placeholder**，尽量无 `label`（密度压到 34px）；隐藏 `.q-field__bottom`
- 仅保留「重置」等功能按钮在 `.app-filter-bar__actions`

---

## 4. Card

| Class | 何时用 |
|---|---|
| `.crud-table-card` / `.crud-search-card` | CRUD 列表 |
| `.dash-card` + `__header` / `__title` / `__list` / `__empty` | 仪表盘信息卡（HUD 左刻度条标题） |
| `.app-stat-card` + `--success|danger|warning|info|violet|neutral` | KPI 条；条容器 `.app-stat-strip` |
| `.app-stat-card--static` | 不可点击 KPI |
| `.app-stat-card.is-filter-on` | KPI 作为过滤器选中 |
| `.app-stat-delta` + `.is-good|.is-bad|.is-flat` | 环比行 |
| `.app-pane-card` / `__header` / `__body` | 分栏面板卡（`_page-patterns`） |
| `.crud-split-card` | 双栏管理 |

卡片默认：`border-radius: 16px`（stat 卡 14px）、`background: var(--glass-elevated)`、`border: 1px solid var(--glass-border)`。

---

## 5. Button

| 场景 | 约定 |
|---|---|
| 主操作（新建/保存） | `unelevated color="primary"` |
| 次操作（重置） | `outline` |
| 图标刷新 | `flat dense round` |
| 行操作 | `flat dense round` + `.crud-actions__btn` |
| 抽屉取消 | `flat`；保存 `unelevated color="primary"` |

全局（`_controls.sass` + `_component-density.sass`）：

- 非 round/fab：`border-radius: 10px`，`font-weight: 600`
- 非 outline 默认 `min-height: 30px`；筛选栏内按钮强制 **34px** 与字段对齐
- 焦点：`outline: 2px solid var(--color-accent)`
- 暗色实心按钮：霓虹玻璃化（半透明底 + 同色描边 + 光晕），由 `_controls.sass` 映射 `bg-*`

可用辅助 class：`.app-accent-btn`、`.app-outline-btn`（`_page-patterns`）。

---

## 6. Dialog / Drawer

### 6.1 全局层

所有 `q-dialog` 自动获得玻璃外壳、遮罩 blur、标题栏/底栏 chrome（`_glass-dialog.sass`）。  
昼夜分叉用 `body.body--dark`，**禁止**用 `:not(.q-card--dark)` 排除。

### 6.2 显式精调 Class

| Class | 用途 |
|---|---|
| `.app-dialog-card` | 统一外壳精调 |
| `.app-dialog-card--sm\|md\|lg\|xl\|2xl` / `--900` | 定宽修饰 |
| `.app-glass-dialog` | 分区布局（head / body / actions） |
| `.app-glass-dialog__head` / `__title` / `__subtitle` / `__scroll` / `__body` | 分区 |
| `.form-drawer-dialog` / `.form-drawer-card` | **FormDrawer** 右侧抽屉 |

### 6.3 FormDrawer 约定

- 组件：`apps/web/src/components/FormDrawer.vue`
- `position="right"` 全高；默认宽 **480px**（`width` prop）
- 字段：`dense outlined`；校验走 `q-form`；保存触发 `submit` / 失败 `submit-invalid`
- `dense` prop：密排配置抽屉；非 dense 时无 bottom 字段补 `padding-bottom: 20px`（盒高一致）
- 只读查看：`showSubmit=false`

---

## 7. 表单密度

| 规则 | 值 |
|---|---|
| 无 label 的 dense 字段 control 高 | **34px**（筛选栏） |
| 带 label / textarea | 保持 Quasar dense（约 40px / auto），**禁止**压到 34px |
| 表格单元格 padding | 8×12；`--dense` 时 5×10 |
| 字号 | 表体 `--text-base`；dense 字段 `--text-sm` |

实现文件：`_component-density.sass`；筛选栏特例覆写在 `_crud-tables.sass`。

**红线**：覆盖字段高度时必须 **同步** `height` 与 `min-height`。

---

## 8. 页面模式 / 玻璃 / 大屏

### 8.1 管理页

| Class | 用途 |
|---|---|
| `.app-standard-page` | 标准管理页（padding `--space-6`，max-width `--layout-max-width`） |
| `.app-page-cream` / `.monitor-page` 等 | 同族页底；暗色用 `--gradient-page` |
| `.app-page-hero` / `__` 相关 | 页头 Hero |
| `.app-page-title` / `.app-page-subtitle` / `.app-page-kicker` | 标题阶梯 |
| `.app-split-layout` | 分栏 |
| `.app-metrics-grid` / `--4` / `--5` | 指标栅格 |
| `.app-empty-state-center` | 空态居中 |
| `.app-code-block` | 代码块 |

### 8.2 大屏（`_screen-glass.sass`）

| Class | 层级 |
|---|---|
| `.screen-canvas` / `--grid` | L0 画布 |
| `.screen-theme-dark` / `.screen-theme-light` | 强制画布主题（可与 body 解耦） |
| `.glass-panel` / `--l2` / `--alert` / `--active` | L1 液态玻璃面板 |
| `.screen-perf` | 关 blur 保性能 |
| `.widget-state` / `--loading` / `--error` | 部件三态 |
| `.screen-num` | 大数字 |

**注意**：大屏 token（`--screen-primary` 等）**不要**污染后台管理页；交互增强限定在 `.screen-canvas` / `.screen-theme-*` 祖先内。

---

## 9. 可复制 Token / Class 速查表

### Token（复制用）

```text
var(--color-accent)
var(--color-accent-soft)
var(--color-success) / var(--color-warning) / var(--color-danger) / var(--color-info)
var(--color-text-primary) / var(--color-text-secondary) / var(--color-text-heading)
var(--color-icon-muted)
var(--glass-surface) / var(--glass-elevated) / var(--glass-border)
var(--glass-blur-default) / var(--glass-blur-elevated)
var(--glass-inner-highlight)
var(--interaction-surface-hover)
var(--space-2) / var(--space-3) / var(--space-4) / var(--space-6)
var(--text-sm) / var(--text-base)
var(--gradient-page)
var(--q-primary)
```

### Class（复制用）

```text
app-filter-bar  app-filter-bar__actions  app-filter-bar__field--lg
crud-actions  crud-actions__btn  crud-actions__btn--danger
crud-table-card  crud-search-card  crud-split-card
dash-card  dash-card__header  dash-card__title  dash-card__list
app-stat-strip  app-stat-card  app-stat-card--success  app-stat-card--static
app-dialog-card  app-dialog-card--lg  app-glass-dialog
app-standard-page  app-page-hero  app-pane-card
screen-canvas  glass-panel  glass-panel--l2  screen-theme-dark
```

---

## 10. 禁止事项（红线）

1. **禁止**在业务 Vue/Sass 中写随意新色：`#xxx`、`rgb()`、`rgba()` 魔数（token 定义文件除外，且须 `stylelint-disable color-no-hex` 说明）。
2. **禁止** `style="color:#…"` / `background:#…` 内联魔数色；用 `var(--*)` 或语义 Quasar `color`。
3. **禁止**新增第二套全局主题入口或复制粘贴一套「本页专用调色板」。
4. **禁止**操作列改用 outline/实底/自定义 sticky 黑条。
5. **禁止**筛选栏放「搜索」按钮或给筛选项加会撑开底栏的 hint（除非整栏统一处理）。
6. **禁止**对带 label 的表单字段强制 34px 高度。
7. **禁止**在管理页使用大屏 `--screen-*` / `.glass-panel` 当普通卡片（语义混淆）。
8. **禁止**图表自建色板；走 `--chart-series-*`。
9. 需要新色时：先在 `_css-vars-light.sass` + `_css-vars-dark.sass` **成对**增加 token，再消费 `var(--*)`。

---

## 11. 组件样板对照

| 能力 | 样板 |
|---|---|
| 树形 CRUD + 抽屉 | `pages/opm/OrganizationPage.vue` + `CrudTablePage` + `FormDrawer` |
| 筛选 + 表格 + 新建 | 同上；`creatable` + `@create` |
| 操作列 | `.crud-actions` 结构（见 §3.2） |
| KPI | `.app-stat-strip` > `.app-stat-card--*` |
| 仪表信息卡 | `.dash-card` |

---

## 12. 开发检查清单

- [ ] 颜色全部来自 token / Quasar 语义色，无业务 hex/rgb 魔数
- [ ] 亮暗成对验证（`body` 切换 Dark）
- [ ] 列表页使用 `CrudTablePage` 或同等 class（`.app-filter-bar` + `.crud-table-card`）
- [ ] 操作列：`flat dense round` + `.crud-actions__btn`
- [ ] 主按钮 `unelevated primary`；筛选栏无搜索按钮
- [ ] 表单字段 `dense outlined`；抽屉用 `FormDrawer`
- [ ] 卡片用 glass elevated / 既有 card class，未自造圆角色值
- [ ] 新样式挂到 `app-theme.sass` 的 theme partial，而非页面 scoped 复制大段 token
- [ ] 大屏 class 仅用于大屏路由/查看页

---

*文档版本：2026-10-17 · 基于 FrameWork 抽样 CSS / CrudTablePage / FormDrawer*
