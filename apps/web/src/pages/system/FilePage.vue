<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('file.title')"
    :columns="columns"
    :search-fields="searchFields"
    row-key="id"
    :creatable="false"
    :no-paging="true"
    :fetch-data-fn="fetchData"
  >
    <template #body-cell-size="{ row }">
      {{ formatBytes(row.size as number) }}
    </template>

    <template #body-cell-actions="{ row }">
      <div class="crud-actions">
        <q-btn class="crud-actions__btn" flat dense round icon="download" @click="onDownload(row)">
          <q-tooltip>{{ t('common.download') }}</q-tooltip>
        </q-btn>
        <q-btn class="crud-actions__btn crud-actions__btn--danger" flat dense round icon="delete">
          <q-tooltip>{{ t('common.delete') }}</q-tooltip>
          <q-popup-confirm @confirm="onDelete(row)">
            {{ t('common.confirmDelete') }}
          </q-popup-confirm>
        </q-btn>
      </div>
    </template>
  </CrudTablePage>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import { useFileStore } from '../../stores';
import { fileService } from '../../services';
import type { FileEntity } from '../../services';

const { t } = useI18n();
const store = useFileStore();

const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

const columns = computed<QTableColumn[]>(() => [
  { name: 'fileName', label: t('file.name'), field: 'fileName', align: 'left', sortable: true },
  { name: 'size', label: t('file.size'), field: 'size', align: 'right', sortable: true },
  { name: 'bucketName', label: t('file.bucket'), field: 'bucketName', align: 'left' },
  { name: 'fileDirectory', label: t('file.directory'), field: 'fileDirectory', align: 'left' },
  { name: 'createTime', label: t('common.createTime'), field: 'createTime', align: 'center', sortable: true },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const searchFields = computed(() => [
  { name: 'fileName', label: t('file.name'), type: 'input' as const },
]);

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(2)} ${units[i]}`;
}

async function fetchData(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
}) {
  const resp = await store.fetchList({
    page: params.page,
    pageSize: params.pageSize,
    noPaging: params.noPaging,
    formValues: params.formValues,
  });
  return { items: resp.items as unknown as Record<string, unknown>[], total: resp.total };
}

async function onDownload(row: Record<string, unknown>) {
  try {
    await fileService.download(row as unknown as FileEntity);
  } catch {
    // error notification handled by axios interceptor
  }
}

async function onDelete(row: Record<string, unknown>) {
  const entity = row as unknown as FileEntity;
  if (!entity.id) return;
  try {
    await store.remove(entity.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}
</script>

<style lang="sass">
</style>
