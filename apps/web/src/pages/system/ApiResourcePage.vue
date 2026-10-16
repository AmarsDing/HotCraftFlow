<template>
  <CrudTablePage
    ref="tableRef"
    :title="t('apiResource.title')"
    :columns="columns"
    :search-fields="searchFields"
    :fetch-data-fn="fetchDataFn"
    :tree="groupByModule"
    :no-paging="groupByModule"
    :row-class-fn="rowClassFn"
    @create="onCreate"
  >
    <!-- 路径：monospace + 点击复制；手工行未在文档登记时叠加琥珀徽标 -->
    <template #body-cell-path="{ row }">
      <div v-if="!row._group" class="api-path-cell">
        <span class="api-path" :title="row.path" @click="copyPath(row.path)">{{ row.path }}</span>
        <q-badge
          v-if="row.inOpenapi === false"
          color="amber-8"
          :label="t('apiResource.notInDoc')"
          class="api-path-flag"
        >
          <q-tooltip>{{ t('apiResource.notInDocHint') }}</q-tooltip>
        </q-badge>
      </div>
    </template>

    <template #body-cell-method="{ row }">
      <q-badge v-if="!row._group" :color="methodColor(row.method)" :label="String(row.method ?? '')" />
    </template>

    <!-- 模块：tooltip 附模块描述 -->
    <template #body-cell-module="{ row }">
      <span class="ellipsis" :title="moduleTooltip(row)">{{ row.module || '-' }}</span>
    </template>

    <!-- 来源：sync=蓝 / manual=橙 -->
    <template #body-cell-source="{ row }">
      <q-badge
        v-if="!row._group"
        :color="row.source === 'sync' ? 'primary' : 'orange-8'"
        :label="row.source === 'sync' ? t('apiResource.sourceSync') : t('apiResource.sourceManual')"
      />
    </template>

    <!-- 状态：启停开关内嵌（状态即操作）；delisted 行禁用开关并叠灰徽标 -->
    <template #body-cell-status="{ row }">
      <div v-if="!row._group" class="api-status-cell">
        <q-toggle
          :model-value="isOn(row)"
          color="positive"
          dense
          :disable="row.delisted === true || togglingId !== null"
          @update:model-value="onToggleStatus(row)"
        >
          <q-tooltip>{{ statusTooltip(row) }}</q-tooltip>
        </q-toggle>
        <q-badge v-if="row.delisted === true" color="grey-7" :label="t('apiResource.delisted')">
          <q-tooltip>{{ t('apiResource.delistedHint') }}</q-tooltip>
        </q-badge>
      </div>
    </template>

    <!-- 绑定角色数：删除影响面可视化 -->
    <template #body-cell-roleCount="{ row }">
      <q-badge
        v-if="!row._group"
        :color="(row.roleCount ?? 0) > 0 ? 'deep-purple' : 'grey-5'"
        :label="String(row.roleCount ?? 0)"
        outline
      />
    </template>

    <template #body-cell-updateTime="{ row }">
      <span v-if="!row._group">{{ formatDateTime(row.updateTime) }}</span>
    </template>

    <template #body-cell-actions="{ row }">
      <div v-if="!row._group" class="crud-actions">
        <q-btn class="crud-actions__btn" flat dense round icon="edit" @click="onEdit(row)">
          <q-tooltip>{{ t('common.edit') }}</q-tooltip>
        </q-btn>
        <q-btn class="crud-actions__btn crud-actions__btn--danger" flat dense round icon="delete">
          <q-tooltip>{{ t('common.delete') }}</q-tooltip>
          <q-popup-confirm @confirm="onDelete(row)">
            <div class="api-delete-confirm">
              <div>{{ t('common.confirmDelete') }}</div>
              <div v-if="(row.roleCount ?? 0) > 0" class="api-delete-confirm__bound">
                {{ t('apiResource.deleteBoundConfirm', { count: row.roleCount }) }}
              </div>
            </div>
          </q-popup-confirm>
        </q-btn>
      </div>
    </template>

    <template #toolbar>
      <q-toggle
        v-model="groupByModule"
        :label="t('apiResource.groupByModule')"
        dense
        class="api-group-toggle"
        @update:model-value="onGroupToggle"
      />
      <q-btn unelevated color="primary" icon="sync" :label="t('common.sync')" :loading="syncing" @click="onSync" />
    </template>

    <template #extra>
      <FormDrawer v-model="drawerOpen" :title="drawerTitle" :saving="saving" @submit="onSubmit">
        <q-banner v-if="editingSource === 'sync'" dense rounded class="api-sync-banner">
          <q-icon name="cloud_sync" class="q-mr-xs" />{{ t('apiResource.syncManagedHint') }}
        </q-banner>
        <q-input
          v-model="form.description"
          :placeholder="t('apiResource.description')"
          dense
          outlined
          :readonly="editingSource === 'sync'"
        />
        <q-input
          v-model="form.module"
          :placeholder="t('common.module')"
          dense
          outlined
          :readonly="editingSource === 'sync'"
        />
        <q-input
          v-model="form.moduleDescription"
          :placeholder="t('apiResource.moduleDesc')"
          dense
          outlined
          :readonly="editingSource === 'sync'"
        />
        <q-input
          v-model="form.path"
          :placeholder="t('common.path')"
          dense
          outlined
          :readonly="editingSource === 'sync'"
        />
        <q-select
          v-model="form.method"
          :placeholder="t('common.method')"
          :options="methodOptions"
          dense
          outlined
          emit-value
          map-options
          :disable="editingSource === 'sync'"
        />
        <q-toggle
          v-if="editingId !== null"
          v-model="form.statusOn"
          :label="form.statusOn ? t('common.enabled') : t('common.disabled')"
          color="positive"
          :disable="editingDelisted"
        >
          <q-tooltip v-if="editingDelisted">{{ t('apiResource.delistedHint') }}</q-tooltip>
        </q-toggle>
      </FormDrawer>

      <!-- 同步差异报告对话框 -->
      <q-dialog v-model="reportOpen">
        <q-card class="api-sync-report">
          <q-card-section class="api-sync-report__header">
            <div class="text-h6">{{ t('apiResource.reportTitle') }}</div>
            <q-btn v-close-popup flat dense round icon="close" />
          </q-card-section>

          <q-card-section class="api-sync-report__stats">
            <div
              v-for="stat in reportStats"
              :key="stat.key"
              class="api-sync-report__stat"
              :class="`api-sync-report__stat--${stat.key}`"
            >
              <div class="api-sync-report__stat-value">{{ stat.value }}</div>
              <div class="api-sync-report__stat-label">{{ stat.label }}</div>
            </div>
          </q-card-section>

          <q-card-section v-if="reportTabs.length > 0" class="api-sync-report__detail">
            <q-tabs v-model="reportTab" dense align="left" active-color="primary" narrow-indicator>
              <q-tab
                v-for="tab in reportTabs"
                :key="tab.type"
                :name="tab.type"
                :label="`${tab.label} (${tab.items.length})`"
              />
            </q-tabs>
            <q-tab-panels v-model="reportTab" animated>
              <q-tab-panel v-for="tab in reportTabs" :key="tab.type" :name="tab.type">
                <div class="api-sync-report__changes">
                  <div v-for="(change, idx) in tab.items" :key="idx" class="api-sync-report__change">
                    <q-badge :color="methodColor(change.method)" :label="change.method" />
                    <span class="api-path">{{ change.path }}</span>
                    <span class="api-sync-report__change-module">{{ change.module || '-' }}</span>
                  </div>
                </div>
              </q-tab-panel>
            </q-tab-panels>
            <div v-if="reportTruncated" class="api-sync-report__truncated">
              {{ t('apiResource.reportTruncated', { cap: SYNC_REPORT_CAP }) }}
            </div>
          </q-card-section>
          <q-card-section v-else class="api-sync-report__empty">
            {{ t('apiResource.reportEmpty') }}
          </q-card-section>

          <q-card-section v-if="(report?.delisted ?? 0) > 0" class="api-sync-report__guide">
            <q-icon name="info" class="q-mr-xs" />{{ t('apiResource.reportDelistedGuide') }}
          </q-card-section>
        </q-card>
      </q-dialog>
    </template>
  </CrudTablePage>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, copyToClipboard, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useApiResourceStore } from '../../stores';
import { apiResourceService, type ApiResourceEntity } from '../../services';
import type { SyncApiResourcesResponse, SyncChangeItem } from '../../services/kratos/admin/service/v1';
import { methodColor } from '../../utils/http-method';
import { formatDateTime } from '../../utils/datetime';

const { t } = useI18n();
const store = useApiResourceStore();

const tableRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

// 与后端 syncChangeReportCap 对齐（报告明细截断提示）
const SYNC_REPORT_CAP = 200;

type ApiRow = ApiResourceEntity & { _group?: boolean; children?: ApiResourceEntity[] };

const columns = computed<QTableColumn[]>(() => [
  { name: 'description', label: t('apiResource.description'), field: 'description', align: 'left', sortable: true },
  { name: 'module', label: t('common.module'), field: 'module', align: 'left', sortable: true },
  { name: 'path', label: t('common.path'), field: 'path', align: 'left' },
  { name: 'method', label: t('common.method'), field: 'method', align: 'left' },
  { name: 'source', label: t('apiResource.source'), field: 'source', align: 'left' },
  { name: 'status', label: t('common.status'), field: 'status', align: 'left' },
  { name: 'roleCount', label: t('apiResource.roleCount'), field: 'roleCount', align: 'center' },
  { name: 'updateTime', label: t('common.updateTime'), field: 'updateTime', align: 'left', sortable: true },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const methodOptions = [
  { label: 'GET', value: 'GET' },
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'DELETE', value: 'DELETE' },
];

const sourceOptions = computed(() => [
  { label: t('apiResource.sourceSync'), value: 'sync' },
  { label: t('apiResource.sourceManual'), value: 'manual' },
]);

// 模块筛选选项：独立全量拉取去重（不走 store.list，避免与表格分页数据互踩）
const moduleOptions = ref<{ label: string; value: string }[]>([]);
async function loadModuleOptions() {
  try {
    const resp = await apiResourceService.list({ noPaging: true });
    const mods = new Set<string>();
    for (const item of resp.items) {
      if (item.module) mods.add(item.module);
    }
    moduleOptions.value = [...mods].sort((a, b) => a.localeCompare(b)).map((m) => ({ label: m, value: m }));
  } catch {
    // 选项缺失不阻塞页面（表格数据由 fetchDataFn 独立加载）
  }
}

const searchFields = computed(() => [
  { name: 'method', label: t('common.method'), type: 'select' as const, options: methodOptions },
  { name: 'module', label: t('common.module'), type: 'select' as const, options: moduleOptions.value },
  { name: 'source', label: t('apiResource.source'), type: 'select' as const, options: sourceOptions.value },
  { name: 'path', label: t('common.path'), type: 'input' as const, fuzzy: true },
  { name: 'description', label: t('apiResource.description'), type: 'input' as const, fuzzy: true },
]);

// === 编辑抽屉 ===
const drawerOpen = ref(false);
const saving = ref(false);
const editingId = ref<string | number | null>(null);
const editingSource = ref<string>('manual');
const editingDelisted = ref(false);
const form = ref({
  description: '',
  module: '',
  moduleDescription: '',
  path: '',
  method: '',
  statusOn: true,
});

const drawerTitle = computed(() =>
  editingId.value !== null ? t('common.edit') : t('common.create'),
);

function resetForm() {
  form.value = { description: '', module: '', moduleDescription: '', path: '', method: '', statusOn: true };
  editingSource.value = 'manual';
  editingDelisted.value = false;
}

function onCreate() {
  editingId.value = null;
  resetForm();
  drawerOpen.value = true;
}

function onEdit(row: Record<string, unknown>) {
  editingId.value = (row.id as string | number) ?? null;
  editingSource.value = (row.source as string) || 'manual';
  editingDelisted.value = row.delisted === true;
  form.value = {
    description: (row.description as string) ?? '',
    module: (row.module as string) ?? '',
    moduleDescription: (row.moduleDescription as string) ?? '',
    path: (row.path as string) ?? '',
    method: (row.method as string) ?? '',
    statusOn: isOn(row),
  };
  drawerOpen.value = true;
}

async function onSubmit() {
  saving.value = true;
  try {
    if (editingId.value !== null) {
      // sync 行仅允许切换启停（文档派生字段由同步托管，防止手改被覆盖造成困惑）
      if (editingSource.value === 'sync') {
        await store.update(editingId.value, { status: form.value.statusOn ? 'ON' : 'OFF' });
      } else {
        await store.update(editingId.value, {
          description: form.value.description,
          module: form.value.module,
          moduleDescription: form.value.moduleDescription,
          path: form.value.path,
          method: form.value.method,
          status: form.value.statusOn ? 'ON' : 'OFF',
        });
      }
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await store.create({
        description: form.value.description,
        module: form.value.module,
        moduleDescription: form.value.moduleDescription,
        path: form.value.path,
        method: form.value.method,
      });
      Notify.create({ type: 'positive', message: t('common.createSuccess') });
    }
    drawerOpen.value = false;
    await tableRef.value?.fetchData();
    void loadModuleOptions();
  } catch {
    // error notification handled by request interceptor
  } finally {
    saving.value = false;
  }
}

// === 启停开关（状态列内嵌，状态即操作） ===
const togglingId = ref<string | number | null>(null);

function isOn(row: Record<string, unknown>): boolean {
  // status 缺省（NULL）视为启用，与后端鉴权展开口径一致
  return (row.status as string | undefined) !== 'OFF';
}

function statusTooltip(row: Record<string, unknown>): string {
  if (row.delisted === true) return t('apiResource.delistedHint');
  return isOn(row) ? t('common.enabled') : t('common.disabled');
}

async function onToggleStatus(row: Record<string, unknown>) {
  if (row.id === undefined || togglingId.value !== null) return;
  togglingId.value = row.id as string | number;
  try {
    await store.update(row.id as string | number, { status: isOn(row) ? 'OFF' : 'ON' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await tableRef.value?.fetchData();
  } catch {
    // handled by interceptor
  } finally {
    togglingId.value = null;
  }
}

// === 删除（级联清理角色绑定，确认框带影响面） ===
async function onDelete(row: Record<string, unknown>) {
  try {
    await store.remove(row.id as string | number);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await tableRef.value?.fetchData();
    void loadModuleOptions();
  } catch {
    // error notification handled by request interceptor
  }
}

// === 同步（一键执行 + 差异报告） ===
const syncing = ref(false);
const report = ref<SyncApiResourcesResponse | null>(null);
const reportOpen = ref(false);
const reportTab = ref('created');

async function onSync() {
  syncing.value = true;
  try {
    report.value = await apiResourceService.sync();
    reportOpen.value = true;
    reportTab.value = reportTabs.value[0]?.type ?? 'created';
    await tableRef.value?.fetchData();
    void loadModuleOptions();
  } catch {
    // error notification handled by request interceptor
  } finally {
    syncing.value = false;
  }
}

const reportStats = computed(() => {
  const r = report.value;
  if (!r) return [];
  return [
    { key: 'created', label: t('apiResource.reportCreated'), value: r.created ?? 0 },
    { key: 'updated', label: t('apiResource.reportUpdated'), value: r.updated ?? 0 },
    { key: 'unchanged', label: t('apiResource.reportUnchanged'), value: r.unchanged ?? 0 },
    { key: 'delisted', label: t('apiResource.reportDelisted'), value: r.delisted ?? 0 },
    { key: 'relisted', label: t('apiResource.reportRelisted'), value: r.relisted ?? 0 },
    { key: 'shadowed', label: t('apiResource.reportShadowed'), value: r.shadowed ?? 0 },
    { key: 'manualNotInDoc', label: t('apiResource.reportManualNotInDoc'), value: r.manualNotInDoc ?? 0 },
  ];
});

const changeTypeLabels: Record<string, () => string> = {
  created: () => t('apiResource.reportCreated'),
  updated: () => t('apiResource.reportUpdated'),
  delisted: () => t('apiResource.reportDelisted'),
  relisted: () => t('apiResource.reportRelisted'),
  shadowed: () => t('apiResource.reportShadowed'),
};

const reportTabs = computed(() => {
  const groups = new Map<string, SyncChangeItem[]>();
  for (const change of report.value?.changes ?? []) {
    const type = change.changeType || 'unknown';
    if (!groups.has(type)) groups.set(type, []);
    groups.get(type)!.push(change);
  }
  return [...groups.entries()].map(([type, items]) => ({
    type,
    items,
    label: changeTypeLabels[type]?.() ?? type,
  }));
});

// changes 达到上限时提示截断（后端 cap=SYNC_REPORT_CAP）
const reportTruncated = computed(() => (report.value?.changes?.length ?? 0) >= SYNC_REPORT_CAP);

// === 按模块分组视图 ===
const groupByModule = ref(false);

function onGroupToggle() {
  void tableRef.value?.fetchData();
}

function moduleTooltip(row: Record<string, unknown>): string {
  const desc = row.moduleDescription as string | undefined;
  const mod = (row.module as string) || '';
  return desc ? `${mod} — ${desc}` : mod;
}

function rowClassFn(row: Record<string, unknown>): string {
  if (row.delisted === true) return 'api-row--delisted';
  if (!isOn(row)) return 'api-row--off';
  return '';
}

async function copyPath(path: unknown) {
  if (!path) return;
  await copyToClipboard(String(path));
  Notify.create({ type: 'positive', message: t('common.copySuccess') });
}

async function fetchDataFn(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
  orderBy?: string;
}) {
  if (!groupByModule.value) {
    return store.fetchList({
      page: params.page,
      pageSize: params.pageSize,
      noPaging: false,
      formValues: params.formValues,
      orderBy: params.orderBy,
    });
  }

  // 分组视图：全量拉取（筛选走后端），客户端按 module 聚合父子行
  const resp = await store.fetchList({
    noPaging: true,
    formValues: params.formValues,
    orderBy: params.orderBy,
  });
  const groups = new Map<string, ApiRow>();
  for (const item of resp.items) {
    const mod = item.module || '-';
    if (!groups.has(mod)) {
      groups.set(mod, { id: `grp-${mod}`, _group: true, description: mod, module: mod, children: [] });
    }
    groups.get(mod)!.children!.push(item);
  }
  const items = [...groups.values()].sort((a, b) => String(a.module).localeCompare(String(b.module)));
  return { items: items as unknown as ApiResourceEntity[], total: resp.total };
}

onMounted(() => {
  void loadModuleOptions();
});
</script>

<style lang="scss" scoped>
.api-path-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.api-path {
  font-family: monospace;
  font-size: 12px;
  cursor: pointer;

  &:hover {
    color: var(--color-accent, #1976d2);
    text-decoration: underline;
  }
}

.api-path-flag {
  flex-shrink: 0;
}

.api-status-cell {
  display: flex;
  align-items: center;
  gap: 6px;
}

.api-delete-confirm {
  max-width: 320px;

  &__bound {
    margin-top: 6px;
    color: #c62828;
    font-size: 12px;
  }
}

.api-group-toggle {
  margin-right: 8px;
}

.api-sync-banner {
  background: rgba(25, 118, 210, 0.08);
  color: #1565c0;
  font-size: 12px;
}

.api-sync-report {
  width: 720px;
  max-width: 90vw;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 4px;
  }

  &__stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(88px, 1fr));
    gap: 8px;
  }

  &__stat {
    text-align: center;
    padding: 8px 4px;
    border-radius: 8px;
    background: rgba(0, 0, 0, 0.04);
  }

  &__stat-value {
    font-size: 20px;
    font-weight: 700;
    line-height: 1.2;
  }

  &__stat-label {
    font-size: 11px;
    opacity: 0.75;
    margin-top: 2px;
  }

  &__stat--created .api-sync-report__stat-value { color: #2e7d32; }
  &__stat--updated .api-sync-report__stat-value { color: #1565c0; }
  &__stat--unchanged .api-sync-report__stat-value { color: #616161; }
  &__stat--delisted .api-sync-report__stat-value { color: #c62828; }
  &__stat--relisted .api-sync-report__stat-value { color: #00838f; }
  &__stat--shadowed .api-sync-report__stat-value { color: #ef6c00; }
  &__stat--manualNotInDoc .api-sync-report__stat-value { color: #ff8f00; }

  &__changes {
    max-height: 320px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  &__change {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
  }

  &__change-module {
    margin-left: auto;
    opacity: 0.6;
  }

  &__truncated {
    margin-top: 8px;
    font-size: 12px;
    opacity: 0.7;
  }

  &__empty {
    text-align: center;
    opacity: 0.7;
  }

  &__guide {
    padding-top: 0;
    font-size: 12px;
    color: #ef6c00;
  }
}

// OFF 行整体灰显；delisted 行进一步淡化（树形模式行 class 由 rowClassFn 注入）
:deep(tr.api-row--off) td {
  opacity: 0.55;
}

:deep(tr.api-row--delisted) td {
  opacity: 0.45;
}
</style>
