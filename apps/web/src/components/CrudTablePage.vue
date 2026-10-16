<template>
  <!-- embedded 模式：根节点用 div 以便嵌入库存中心等容器的 q-tab-panel 中（q-page 不能嵌套） -->
  <!-- 注意：:is 必须直接传组件引用；字符串 'q-page' 在按需注册下无法解析，会退化为未注册自定义元素 -->
  <component
    :is="embedded ? 'div' : QPage"
    class="crud-page"
    :class="[
      embedded ? 'crud-page--embedded' : 'q-pa-lg',
      fillHeight ? 'crud-page--fill' : '',
      tree ? 'crud-page--tree' : '',
    ]"
  >
    <slot name="banner" />
    <!-- Search bar（规范见 frontend-dev-guide §4.11：选中即查、条件靠左、按钮靠右） -->
    <q-card v-if="searchFields.length > 0" flat class="crud-search-card q-mb-md">
      <q-card-section class="app-filter-bar">
        <!-- search-prepend 插槽：页面级前置过滤控件（如机场域选择器），与搜索字段同行渲染 -->
        <slot name="search-prepend" />
        <template v-for="field in searchFields" :key="field.name">
          <q-input
            v-if="field.type === 'input'"
            v-model="searchModel[field.name]"
            :placeholder="field.placeholder || field.label"
            dense
            outlined
            clearable
            hide-bottom-space
            @update:model-value="onSearchInput"
            @keyup.enter="onSearch"
          />
          <q-select
            v-else-if="field.type === 'select'"
            v-model="searchModel[field.name]"
            :placeholder="field.placeholder || field.label"
            :options="field.options || []"
            dense
            outlined
            clearable
            hide-bottom-space
            emit-value
            map-options
            :use-input="field.useInput"
            :input-debounce="field.useInput ? 300 : 0"
            :new-value-mode="field.newValueMode"
            @filter="field.filterFn"
            @update:model-value="onSearch"
          />
          <DeviceTreeSelect
            v-else-if="field.type === 'device'"
            :model-value="asDeviceId(searchModel[field.name])"
            class="app-filter-bar__field--lg"
            :placeholder="field.placeholder || field.label"
            @update:model-value="onDeviceSearch(field.name, $event)"
          />
          <q-input
            v-else-if="field.type === 'date'"
            v-model="searchModel[field.name]"
            :placeholder="field.placeholder || field.label"
            dense
            outlined
            clearable
            readonly
          >
            <template #prepend>
              <q-icon name="event" class="cursor-pointer">
                <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                  <q-date v-model="searchModel[field.name]" mask="YYYY-MM-DD" @update:model-value="onSearch">
                    <div class="row items-center justify-end">
                      <q-btn v-close-popup :label="t('common.confirm')" flat />
                    </div>
                  </q-date>
                </q-popup-proxy>
              </q-icon>
            </template>
          </q-input>
        </template>
        <div class="app-filter-bar__actions">
          <!-- search-actions 插槽：页面级操作按钮（如日志清理），渲染在重置按钮左边 -->
          <slot name="search-actions" />
          <q-btn outline icon="restart_alt" :label="t('common.reset')" @click="onResetSearch" />
        </div>
      </q-card-section>
    </q-card>

    <!-- Table
         hide-pagination：no-paging 模式已一次拉全量（树形/分组页），
         分页控件只会展示"翻页无效"的假页码，直接隐藏 -->
    <q-card flat class="crud-table-card">
      <q-card-section class="row items-center justify-between q-pb-sm">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6 text-weight-bold">{{ title }}</div>
          <slot name="title-extra" />
        </div>
        <div class="row q-gutter-sm">
          <q-btn
            v-if="creatable"
            unelevated
            color="primary"
            icon="add"
            :label="t('common.create')"
            @click="$emit('create')"
          />
          <slot name="toolbar" />
          <q-btn flat dense round icon="refresh" @click="fetchData()">
            <q-tooltip>{{ t('common.retry') }}</q-tooltip>
          </q-btn>
        </div>
      </q-card-section>

      <q-table
        v-model:pagination="pagination"
        :rows="visibleRows"
        :columns="normalizedColumns"
        :loading="loading"
        :rows-per-page-options="[10, 20, 50, 100]"
        :row-key="rowKey"
        :binary-state-sort="false"
        :hide-pagination="noPaging"
        :table-row-class-fn="tree ? treeRowClass : rowClassFn"
        :grid="grid"
        :selection="selectable ? 'multiple' : 'none'"
        :selected="selectedRows"
        flat
        @request="onRequest"
        @row-click="onRowClick"
        @update:selected="onUpdateSelected"
      >
        <!-- 全局默认单元格：应用列 format、空值显示 '-'、悬停 title 读完整值（全局 sass 为 table-layout:fixed + ellipsis，截断值必须可读）
             父级提供 body-cell-{列名} 时 Quasar 优先用具体插槽，此默认仅兜底 -->
        <template #body-cell="slotData">
          <q-td :props="slotData" :title="qTableCellText(slotData, false)">
            {{ qTableCellText(slotData, true) }}
          </q-td>
        </template>
        <!-- 表头悬停 title：fixed 布局下列头同样会被 ellipsis 截断（如"计划日期"→"计划日…"） -->
        <template #header-cell="slotData">
          <q-th :props="slotData" :title="String(slotData.col.label)">{{ slotData.col.label }}</q-th>
        </template>
        <!-- 树形模式 + 父级未自定义首列单元格时：注入展开/收起箭头与层级缩进（Quasar q-table 无 tree 能力，此处客户端扁平化渲染） -->
        <template v-if="tree && !$slots[`body-cell-${firstColName}`]" #[`body-cell-${firstColName}`]="slotData">
          <q-td :props="slotData">
            <div
              class="row items-center no-wrap"
              :style="{ paddingLeft: `${Number(slotData.row._level || 0) * 18}px` }"
            >
              <q-btn
                v-if="slotData.row._hasChildren"
                flat
                dense
                round
                size="xs"
                :icon="slotData.row._expanded ? 'expand_more' : 'chevron_right'"
                class="crud-tree-toggle"
                @click.stop="toggleTreeRow(slotData.row)"
              />
              <span v-else class="crud-tree-leaf-spacer" />
              <span class="ellipsis">{{ slotData.value }}</span>
            </div>
          </q-td>
        </template>
        <template v-for="(_, name) in $slots" :key="name" #[name]="slotData">
          <q-td v-if="name === 'body-cell' || name.startsWith('body-cell-')" :props="slotData">
            <slot :name="name" v-bind="slotData || {}" />
          </q-td>
          <slot v-else :name="name" v-bind="slotData || {}" />
        </template>
        <!-- 父级未提供 no-data 插槽时的默认空态（图标+文案），避免 Quasar 默认裸文本 -->
        <template v-if="!$slots['no-data']" #no-data>
          <div class="column items-center justify-center q-pa-xl full-width crud-empty">
            <q-icon name="inbox" size="44px" class="crud-empty__icon q-mb-sm" />
            <div class="text-body2">{{ t('common.noData') }}</div>
          </div>
        </template>
      </q-table>
    </q-card>

    <slot name="extra" />
  </component>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { QPage, type QTableColumn, type QTableProps } from 'quasar';
import { withAutoColWidths, qTableCellText } from '../utils/table-columns';
import DeviceTreeSelect from './DeviceTreeSelect.vue';

const { t } = useI18n();

export interface SearchField {
  name: string;
  label: string;
  type: 'input' | 'select' | 'date' | 'device';
  options?: { label: string; value: string | number }[];
  // 仅 input/date 类型有效：自定义占位文案（缺省用 label；需格式提示如"逗号分隔多选"时使用）
  placeholder?: string;
  // 仅 input 类型有效：true 时查询键转 `${name}__icontains` 走模糊匹配
  fuzzy?: boolean;
  // 仅 select 类型有效：true 时允许键盘输入自定义值（配合 filterFn 过滤/追加选项）
  useInput?: boolean;
  filterFn?: (val: string, update: (cb: () => void) => void) => void;
  // 仅 select+useInput 有效：输入新值处理模式（如 add-unique 允许输入列表外自定义值）
  newValueMode?: 'add' | 'add-unique' | 'toggle';
}

const props = withDefaults(
  defineProps<{
    title: string;
    columns: QTableColumn[];
    searchFields?: SearchField[];
    rowKey?: string;
    creatable?: boolean;
    tree?: boolean;
    noPaging?: boolean;
    // true 时以 div 渲染（嵌入库存中心 q-tab-panel），false 为独立 q-page
    embedded?: boolean;
    // true 时根节点撑满父容器高度，表格区内部滚动（分栏/抽屉等定高容器内与邻栏底边对齐）；
    // 仅在父容器有确定高度时使用，常规独立页面保持 false
    fillHeight?: boolean;
    // true 时 q-table 切网格模式（父级经 #item 插槽自定义卡片，分页/加载/空态保持组件托管）
    grid?: boolean;
    // 默认排序（orderBy 格式：'sort' 升序 / '-sort' 降序）；
    // 初始加载及表头三态排序回到"原序"（无标识）时均按此排序请求后端
    defaultOrderBy?: string;
    // 筛选表单初始值（深链场景：如工作台事件流"查看全部"携带 ?status=active 预置过滤）
    initialSearchModel?: Record<string, string | number | null | undefined>;
    // 行级自定义 class（深链高亮等场景；tree 模式下树形 class 优先）
    rowClassFn?: (row: Record<string, unknown>) => string;
    // true 时启用行多选（selected 数组经 update:selectedRows 同步给父级做批量操作）；
    // 选中集跨刷新保留（rowKey 匹配），父级批量动作完成后应自行清空
    selectable?: boolean;
    fetchDataFn: (params: {
      page: number;
      pageSize: number;
      noPaging: boolean;
      formValues: Record<string, unknown>;
      orderBy?: string;
    }) => Promise<{ items: unknown[]; total: number }>;
  }>(),
  {
    searchFields: () => [],
    rowKey: 'id',
    creatable: true,
    tree: false,
    noPaging: false,
    embedded: false,
    fillHeight: false,
    grid: false,
    defaultOrderBy: undefined,
    initialSearchModel: undefined,
    selectable: false,
  },
);

const emit = defineEmits<{
  create: [];
  // 行点击透传（供线路 360° 抽屉等对象联动场景使用；操作列按钮需 @click.stop 防止误触发）
  rowClick: [row: Record<string, unknown>];
  // 行多选同步（selectable=true 时父级 v-model:selectedRows 接收）
  'update:selectedRows': [rows: Record<string, unknown>[]];
}>();

// 列归一化：未显式设宽的列按表头文案 + 字段语义兜底宽度（fixed 布局下列宽由表头 width 决定，
// 不设宽会被均分容器压窄截断；列总宽超容器时走全局 .q-table__middle 横向滚动）。
const normalizedColumns = computed(() => withAutoColWidths(props.columns));

// 默认单元格文本实现已提取到 utils/table-columns（裸 q-table 页面复用同一逻辑）

function onRowClick(_evt: Event, row: Record<string, unknown>) {
  // 树形模式：点击有子级的行任意位置即折叠/展开（不必精确点箭头）
  if (props.tree && row._hasChildren) {
    toggleTreeRow(row);
    return;
  }
  emit('rowClick', row);
}

// 树形行 class：父级行标记可点击（cursor + hover 反馈由页面/全局样式提供）
function treeRowClass(row: Record<string, unknown>): string {
  return row._hasChildren ? 'crud-tree-row--parent' : '';
}

const rows = ref<Record<string, unknown>[]>([]);
const loading = ref(false);
// 行多选状态（selectable=true 时生效；树形模式不支持多选，grid 模式由 Quasar 自身约束）
const selectedRows = ref<Record<string, unknown>[]>([]);

function onUpdateSelected(val: readonly Record<string, unknown>[]) {
  selectedRows.value = [...val];
  emit('update:selectedRows', selectedRows.value);
}
const searchModel = ref<Record<string, string | number | null | undefined>>({ ...props.initialSearchModel });
const pagination = ref<QTableProps['pagination']>({
  page: 1,
  rowsPerPage: 10,
  rowsNumber: 0,
  sortBy: '',
  descending: false,
});

// === 树形表格（tree=true）：q-table 不支持树数据，客户端按展开状态扁平化可见行 ===
// 首列单元格注入层级缩进与展开箭头；行附带 _level/_hasChildren/_expanded 元数据
const firstColName = computed(() => props.columns[0]?.name ?? '');
const expandedKeys = ref<(string | number)[]>([]);

const visibleRows = computed<Record<string, unknown>[]>(() => {
  if (!props.tree) return rows.value;
  const out: Record<string, unknown>[] = [];
  const walk = (list: Record<string, unknown>[], level: number) => {
    for (const r of list) {
      const children = Array.isArray(r.children) ? (r.children as Record<string, unknown>[]) : [];
      const key = r[props.rowKey] as string | number;
      const expanded = expandedKeys.value.includes(key);
      out.push({ ...r, _level: level, _hasChildren: children.length > 0, _expanded: expanded });
      if (children.length > 0 && expanded) walk(children, level + 1);
    }
  };
  walk(rows.value, 0);
  return out;
});

function toggleTreeRow(row: Record<string, unknown>) {
  const key = row[props.rowKey] as string | number;
  const idx = expandedKeys.value.indexOf(key);
  if (idx >= 0) {
    expandedKeys.value.splice(idx, 1);
  } else {
    expandedKeys.value.push(key);
  }
}

// 树形数据量小且用户预期看到完整层级，每次刷新后默认展开全部父节点
function expandAllTreeRows() {
  const keys: (string | number)[] = [];
  const collect = (list: Record<string, unknown>[]) => {
    for (const r of list) {
      const children = r.children;
      if (Array.isArray(children) && children.length > 0) {
        keys.push(r[props.rowKey] as string | number);
        collect(children as Record<string, unknown>[]);
      }
    }
  };
  collect(rows.value);
  expandedKeys.value = keys;
}

// silent：定时巡检类自动刷新使用，跳过 loading 闪烁，失败静默（用户仍在看旧数据，下一轮再试）
async function fetchData(opts?: { silent?: boolean }) {
  if (!opts?.silent) loading.value = true;
  try {
    const sortBy = pagination.value?.sortBy;
    const descending = pagination.value?.descending;
    // 未选中排序列（初始加载 / 三态排序回到原序）时回退默认排序，保证"序号小在前"的基线顺序
    const orderBy = sortBy ? (descending ? `-${sortBy}` : sortBy) : props.defaultOrderBy;
    // fuzzy 标记的输入型搜索字段转 __icontains 模糊匹配（后端 entgo 过滤器默认精确匹配，
    // 名称/编码类关键字搜索用户预期是"包含即命中"）；其余字段保持原样
    const formValues: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(searchModel.value)) {
      if (v === undefined || v === null || v === '') continue;
      const field = props.searchFields.find((x) => x.name === k);
      formValues[field?.type === 'input' && field.fuzzy ? `${k}__icontains` : k] = v;
    }
    const resp = await props.fetchDataFn({
      page: pagination.value?.page || 1,
      pageSize: pagination.value?.rowsPerPage || 10,
      noPaging: props.noPaging,
      formValues,
      orderBy,
    });
    rows.value = resp.items as Record<string, unknown>[];
    if (props.tree) {
      expandAllTreeRows();
    }
    if (pagination.value) {
      pagination.value.rowsNumber = resp.total;
    }
  } catch {
    // 错误提示由 axios 拦截器统一弹出；此处吞掉避免 onMounted/onRequest 产生未捕获 rejection
  } finally {
    if (!opts?.silent) loading.value = false;
  }
}

function onRequest(params: { pagination: QTableProps['pagination'] }) {
  if (params.pagination) {
    pagination.value = params.pagination;
  }
  void fetchData();
}

function asDeviceId(val: string | number | null | undefined): number | null {
  if (val == null || val === '') return null;
  const n = Number(val);
  return Number.isFinite(n) && n > 0 ? n : null;
}

function onDeviceSearch(name: string, value: number | number[] | null) {
  const id = Array.isArray(value) ? (value[0] ?? null) : value;
  searchModel.value[name] = id;
  onSearch();
}

function onSearch() {
  if (pagination.value) {
    pagination.value.page = 1;
  }
  void fetchData();
}

// 输入框自动查询：防抖 400ms，避免逐键触发请求（规范 §4.11）
let searchInputTimer: ReturnType<typeof setTimeout> | undefined;
function onSearchInput() {
  if (searchInputTimer) clearTimeout(searchInputTimer);
  searchInputTimer = setTimeout(onSearch, 400);
}

function onResetSearch() {
  // 重置回到"初始状态"：含 initialSearchModel 预置值（默认筛选/深链过滤），而非清空
  searchModel.value = { ...props.initialSearchModel };
  onSearch();
}

// 滚轮横向滚动已由 boot/table-wheel.ts 全局委托实现（document 捕获，覆盖所有 q-table），此处不再重复挂载。
onMounted(fetchData);

// rows 暴露给父级做实时增量合并（如 WS 推送按 key 就地更新单元格状态），常规场景仅用 fetchData。
// searchModel 暴露给父级让导出等操作可跟随当前筛选条件。
// applySearchValue 供库存中心等容器注入全局上下文（如 warehouseId）或全局搜索关键字并立即刷新。
function applySearchValue(name: string, value: string | number | null | undefined) {
  searchModel.value[name] = value;
  onSearch();
}

// 批量写入多个筛选值后仅触发一次查询（快捷时间段等联动字段场景，避免逐字段 N 次请求）
function applySearchValues(values: Record<string, string | number | null | undefined>) {
  Object.assign(searchModel.value, values);
  onSearch();
}
// 批量操作完成后父级清空选中（selected 为 q-table 受控值，必须经此处同步清）
function clearSelectedRows() {
  selectedRows.value = [];
  emit('update:selectedRows', []);
}

// onSearch 暴露给父级"数据域切换"场景（如左树选中节点/顶部机场切换）：
// 换域必须重置到第 1 页，否则旧域页码（如第 3 页）带入新域导致空页。
defineExpose({
  fetchData,
  onSearch,
  rows,
  searchModel,
  applySearchValue,
  applySearchValues,
  toggleTreeRow,
  selectedRows,
  clearSelectedRows,
});
</script>

<style lang="sass">
.crud-page
  .crud-search-card,
  .crud-table-card
    border-radius: 16px
    background: var(--glass-elevated)
    border: 1px solid var(--glass-border)
  .crud-table-card
    .q-table__top,
    .q-table__bottom
      padding: 8px 16px
    // 列总宽超过容器时允许横向滚动（Quasar 默认 .q-table__middle 无 overflow，宽表会溢出卡片）
    .q-table__middle
      overflow-x: auto
    // 首末列额外留白，避免内容紧贴卡片左右边缘
    .q-table
      thead tr th:first-child,
      tbody tr td:first-child
        padding-left: 20px
      thead tr th:last-child,
      tbody tr td:last-child
        padding-right: 20px
  .crud-empty
    color: var(--color-text-secondary)
    .crud-empty__icon
      opacity: 0.6

// 定高容器填充模式：搜索卡固定、表格卡 flex 撑满，表体内部滚动，底部分页常驻
// （q-page 有内联 min-height，必须 !important 归零，否则内容超出容器把邻栏顶歪）
.crud-page--fill
  height: 100%
  min-height: 0 !important
  display: flex
  flex-direction: column
  overflow: hidden

  .crud-search-card
    flex-shrink: 0

  .crud-table-card
    flex: 1
    min-height: 0
    display: flex
    flex-direction: column

    > .q-card-section
      flex-shrink: 0

    // flex 撑满目标必须是 q-table 组件根 div（.q-table__container）。
    // 严禁对 .q-table 设 display:flex/block——该类挂在 <table> 元素上，
    // 会触发 flex item 块化把 thead/tbody 变成匿名表格框，列宽排版分裂（表头/内容错位）
    .q-table__container
      flex: 1
      min-height: 0
      display: flex
      flex-direction: column

    .q-table__middle
      flex: 1
      min-height: 0
      overflow: auto

    .q-table__bottom
      flex-shrink: 0
  // 树形模式：展开箭头与叶子占位对齐（与 q-btn size="xs" 同宽）
  .crud-tree-toggle
    margin-right: 4px
    color: var(--color-text-secondary)
  .crud-tree-leaf-spacer
    display: inline-block
    width: 28px
    flex-shrink: 0

// 树形模式：有子级的父行整体可点击（点击行即折叠/展开）
.crud-page--tree .crud-tree-row--parent
  cursor: pointer

// 树形表格行底色统一：全局斑马纹 tr:nth-child(even) + hover（含 dark 模式 body.body--dark）
// 在父子层级交错后会让每行像独立"按钮卡片"，树是连续层级列表，这里禁用斑马纹、统一透明底。
// 用 !important 确保覆盖全局 _crud-tables/app-global 的 tr:hover（glass-surface-hover 面板色）。
body .crud-page--tree .crud-table-card .q-table tbody tr,
body .crud-page--tree .crud-table-card .q-table tbody tr:nth-child(even)
  background: transparent !important
body .crud-page--tree .crud-table-card .q-table tbody tr:hover
  background: color-mix(in srgb, var(--color-accent) 8%, transparent) !important
</style>
