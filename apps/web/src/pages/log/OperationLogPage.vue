<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('operationLog.title')"
    :columns="columns"
    :search-fields="searchFields"
    :creatable="false"
    :fetch-data-fn="fetchDataFn"
  >
    <template #search-actions>
      <q-btn outline icon="delete_sweep" :label="t('operationLog.clean')" @click="cleanOpen = true" />
    </template>
    <template #toolbar>
      <q-btn flat dense no-caps icon="download" :label="t('common.export')" :loading="exporting" @click="onExport" />
    </template>
    <template #body-cell-success="{ row }">
      <q-badge
        v-if="row.success === true || row.success === false"
        :color="row.success ? 'green' : 'red'"
        :label="row.success ? t('common.success') : t('common.fail')"
      />
      <span v-else>-</span>
    </template>
    <template #body-cell-client="{ row }">
      <span>{{ row.osName }} {{ row.browserName }}</span>
    </template>
    <template #body-cell-actions="{ row }">
      <div class="crud-actions">
        <q-btn class="crud-actions__btn" flat dense round icon="visibility" @click="onViewDetail(row)">
          <q-tooltip>{{ t('common.detail') }}</q-tooltip>
        </q-btn>
      </div>
    </template>
  </CrudTablePage>

  <CleanLogsDialog
    v-model="cleanOpen"
    :title="t('operationLog.cleanTitle')"
    :keep-days-label="t('operationLog.keepDays')"
    :keep-days-option-text="(d: number) => t('operationLog.keepDaysOption', { days: d })"
    :warning="t('operationLog.cleanWarning')"
    :default-days="30"
    :clean-fn="onClean"
    @done="onCleaned"
  />

  <q-dialog v-model="detailOpen">
    <q-card style="min-width: 720px; max-width: 90vw">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">{{ t('common.detail') }}</div>
        <q-space />
        <q-btn icon="close" flat round dense v-close-popup />
      </q-card-section>
      <q-card-section>
        <div class="text-caption text-grey q-mb-sm">
          {{ detailRow?.method }} {{ detailRow?.path }}
        </div>
        <div class="text-subtitle2 q-mt-sm">Request Body</div>
        <pre class="detail-json">{{ formatJson(detailRow?.requestBody) }}</pre>
        <div class="text-subtitle2 q-mt-md">Response</div>
        <pre class="detail-json">{{ formatJson(detailRow?.response) }}</pre>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import CleanLogsDialog from './CleanLogsDialog.vue';
import { useOperationLogStore } from '../../stores';
import { userIOService, operationLogService } from '../../services';
import { buildQueryParams } from '../../services/request';
import type { OperationLogEntity } from '../../services';

const { t } = useI18n();
const store = useOperationLogStore();

// 表格实例（fetchData 经 defineExpose 暴露，清理完成后刷新列表）
const crudRef = ref<{ fetchData: () => void } | null>(null);

// 最近一次列表查询的筛选条件，导出时复用（与列表 query 序列化格式一致）
const lastFormValues = ref<Record<string, unknown>>({});

const detailOpen = ref(false);
const detailRow = ref<OperationLogEntity | null>(null);

const columns = computed<QTableColumn[]>(() => [
  { name: 'username', label: t('operationLog.username'), field: 'username', align: 'left', sortable: true },
  { name: 'success', label: t('operationLog.success'), field: 'success', align: 'left' },
  { name: 'createTime', label: t('operationLog.createTime'), field: 'createTime', align: 'left', sortable: true },
  { name: 'operation', label: t('operationLog.operation'), field: 'operation', align: 'left' },
  { name: 'method', label: t('operationLog.method'), field: 'method', align: 'left' },
  { name: 'path', label: t('operationLog.path'), field: 'path', align: 'left' },
  { name: 'costTime', label: t('operationLog.costTime'), field: 'costTime', align: 'left', sortable: true },
  { name: 'location', label: t('operationLog.location'), field: 'location', align: 'left' },
  { name: 'client', label: t('operationLog.client'), field: 'osName', align: 'left' },
  { name: 'clientIp', label: t('operationLog.clientIp'), field: 'clientIp', align: 'left' },
  { name: 'reason', label: t('operationLog.reason'), field: 'reason', align: 'left' },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const successOptions = computed(() => [
  { label: t('common.success'), value: 'true' },
  { label: t('common.fail'), value: 'false' },
]);

const searchFields = computed(() => [
  { name: 'username', label: t('operationLog.username'), type: 'input' as const },
  { name: 'operation', label: t('operationLog.operation'), type: 'input' as const },
  { name: 'method', label: t('operationLog.method'), type: 'input' as const },
  { name: 'success', label: t('operationLog.success'), type: 'select' as const, options: successOptions.value },
]);

async function fetchDataFn(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
  orderBy?: string;
}) {
  lastFormValues.value = params.formValues;
  return store.fetchList({
    page: params.page,
    pageSize: params.pageSize,
    noPaging: params.noPaging,
    formValues: params.formValues,
    orderBy: params.orderBy,
  });
}

function onViewDetail(row: Record<string, unknown>) {
  detailRow.value = row as unknown as OperationLogEntity;
  detailOpen.value = true;
}

function formatJson(value: unknown): string {
  if (value == null || value === '') return '-';
  const s = String(value);
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}

// 手动清理（保留最近 N 天，物理删除更早日志）
const cleanOpen = ref(false);

async function onClean(days: number): Promise<number> {
  const resp = await operationLogService.clean(days);
  return resp.deleted ?? 0;
}

function onCleaned(deleted: number) {
  Notify.create({ type: 'positive', message: t('operationLog.cleanResult', { count: deleted }) });
  crudRef.value?.fetchData();
}

const exporting = ref(false);

async function onExport() {
  exporting.value = true;
  try {
    await userIOService.exportOperationLogs(buildQueryParams(lastFormValues.value).query);
    Notify.create({ type: 'positive', message: t('common.export') + ' OK' });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    exporting.value = false;
  }
}
</script>

<style lang="sass" scoped>
.detail-json
  max-height: 320px
  overflow: auto
  padding: 12px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  border-radius: 8px
  font-family: 'JetBrains Mono', 'Fira Code', monospace
  font-size: 12px
  white-space: pre-wrap
  word-break: break-all
</style>
