<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('loginLog.title')"
    :columns="columns"
    :search-fields="searchFields"
    :creatable="false"
    :fetch-data-fn="fetchDataFn"
  >
    <template #search-actions>
      <q-btn outline icon="delete_sweep" :label="t('loginLog.clean')" @click="cleanOpen = true" />
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
  </CrudTablePage>

  <CleanLogsDialog
    v-model="cleanOpen"
    :title="t('loginLog.cleanTitle')"
    :keep-days-label="t('loginLog.keepDays')"
    :keep-days-option-text="(d: number) => t('loginLog.keepDaysOption', { days: d })"
    :warning="t('loginLog.cleanWarning')"
    :default-days="90"
    :clean-fn="onClean"
    @done="onCleaned"
  />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import CleanLogsDialog from './CleanLogsDialog.vue';
import { useLoginLogStore } from '../../stores';
import { userIOService, loginLogService } from '../../services';
import { buildQueryParams } from '../../services/request';

const { t } = useI18n();
const store = useLoginLogStore();

// 表格实例（fetchData 经 defineExpose 暴露，清理完成后刷新列表）
const crudRef = ref<{ fetchData: () => void } | null>(null);

// 最近一次列表查询的筛选条件，导出时复用（与列表 query 序列化格式一致）
const lastFormValues = ref<Record<string, unknown>>({});

const columns = computed<QTableColumn[]>(() => [
  { name: 'username', label: t('loginLog.username'), field: 'username', align: 'left', sortable: true },
  { name: 'success', label: t('loginLog.success'), field: 'success', align: 'left' },
  { name: 'loginTime', label: t('loginLog.loginTime'), field: 'loginTime', align: 'left', sortable: true },
  { name: 'location', label: t('loginLog.location'), field: 'location', align: 'left' },
  { name: 'client', label: t('loginLog.client'), field: 'osName', align: 'left' },
  { name: 'loginIp', label: t('loginLog.loginIp'), field: 'loginIp', align: 'left' },
  { name: 'reason', label: t('loginLog.reason'), field: 'reason', align: 'left' },
]);

const successOptions = computed(() => [
  { label: t('common.success'), value: 'true' },
  { label: t('common.fail'), value: 'false' },
]);

const searchFields = computed(() => [
  { name: 'username', label: t('loginLog.username'), type: 'input' as const },
  { name: 'loginIp', label: t('loginLog.loginIp'), type: 'input' as const },
  { name: 'success', label: t('loginLog.success'), type: 'select' as const, options: successOptions.value },
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

// 手动清理（保留最近 N 天，物理删除更早日志）
const cleanOpen = ref(false);

async function onClean(days: number): Promise<number> {
  const resp = await loginLogService.clean(days);
  return resp.deleted ?? 0;
}

function onCleaned(deleted: number) {
  Notify.create({ type: 'positive', message: t('loginLog.cleanResult', { count: deleted }) });
  crudRef.value?.fetchData();
}

const exporting = ref(false);

async function onExport() {
  exporting.value = true;
  try {
    await userIOService.exportLoginLogs(buildQueryParams(lastFormValues.value).query);
    Notify.create({ type: 'positive', message: t('common.export') + ' OK' });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    exporting.value = false;
  }
}
</script>
