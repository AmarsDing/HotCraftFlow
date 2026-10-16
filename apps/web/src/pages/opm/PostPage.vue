<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('menu.airportPost')"
    :columns="columns"
    :search-fields="searchFields"
    row-key="id"
    :fetch-data-fn="fetchData"
    @create="onCreate"
  >
    <template #body-cell-postType="{ row }">
      {{ postTypeLabels[row.postType as string] ?? row.postType }}
    </template>

    <template #body-cell-airportName="{ row }">
      {{ (row.airportId != null && airportNameMap[Number(row.airportId)]) || '-' }}
    </template>

    <template #body-cell-status="{ row }">
      <q-toggle
        :model-value="row.status === 'ON'"
        @update:model-value="(val) => onToggleStatus(row, !!val)"
      />
    </template>

    <template #body-cell-actions="{ row }">
      <div class="crud-actions">
        <q-btn class="crud-actions__btn" flat dense round icon="edit" @click="onEdit(row)">
          <q-tooltip>{{ t('common.edit') }}</q-tooltip>
        </q-btn>
        <q-btn class="crud-actions__btn crud-actions__btn--danger" flat dense round icon="delete">
          <q-tooltip>{{ t('common.delete') }}</q-tooltip>
          <q-popup-confirm @confirm="onDelete(row)">
            {{ t('common.confirmDelete') }}
          </q-popup-confirm>
        </q-btn>
      </div>
    </template>

    <template #extra>
      <FormDrawer
        v-model="drawerOpen"
        :title="drawerTitle"
        :saving="saving"
        @submit="onSubmit"
      >
        <q-select
          v-model="form.airportId"
          :options="airportOptions"
          :placeholder="t('airportPost.airport')"
          dense
          outlined
          emit-value
          map-options
          :rules="[(val) => !!val || t('airportPost.airportRequired')]"
        />
        <q-input
          v-model="form.name"
          :placeholder="t('common.name')"
          dense
          outlined
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-input
          v-model="form.code"
          :placeholder="t('common.code')"
          dense
          outlined
        />
        <q-select
          v-model="form.postType"
          :options="postTypeOptions"
          :placeholder="t('airportPost.postType')"
          dense
          outlined
          emit-value
          map-options
        />
        <q-input
          v-model.number="form.sortId"
          :placeholder="t('common.sort')"
          dense
          outlined
          type="number"
        />
        <q-input
          v-model="form.description"
          :placeholder="t('common.description')"
          dense
          outlined
          type="textarea"
          rows="2"
        />
        <q-toggle
          :model-value="form.status === 'ON'"
          :label="form.status === 'ON' ? t('common.enabled') : t('common.disabled')"
          @update:model-value="(val) => (form.status = val ? 'ON' : 'OFF')"
        />
      </FormDrawer>
    </template>
  </CrudTablePage>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { airportService, airportPostService, type AirportEntity, type AirportPostEntity } from '../../services';

const { t } = useI18n();
const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

const postTypeLabels = computed<Record<string, string>>(() => ({
  OBSERVATION: t('airportPost.postTypeObservation'),
  FORECAST: t('airportPost.postTypeForecast'),
  MAINTENANCE: t('airportPost.postTypeMaintenance'),
  INFO: t('airportPost.postTypeInfo'),
  SUPERVISION: t('airportPost.postTypeSupervision'),
}));

const postTypeOptions = computed(() =>
  Object.entries(postTypeLabels.value).map(([value, label]) => ({ label, value })),
);

const airportOptions = ref<{ label: string; value: number }[]>([]);
// airportId → 机场名称映射，用于表格展示机场列（后端仅返回 airportId）
const airportNameMap = ref<Record<number, string>>({});

onMounted(async () => {
  try {
    const resp = await airportService.list({ noPaging: true });
    airportOptions.value = resp.items.map((a: AirportEntity) => ({
      label: `${a.code ?? ''} ${a.name ?? ''}`.trim(),
      value: a.id ? Number(a.id) : 0,
    }));
    const map: Record<number, string> = {};
    for (const a of resp.items) {
      if (a.id != null) map[Number(a.id)] = `${a.code ?? ''} ${a.name ?? ''}`.trim();
    }
    airportNameMap.value = map;
  } catch {
    // ignore - airport selector will be empty
  }
});

interface FormState {
  id?: string;
  airportId: number;
  name: string;
  code: string;
  postType: string;
  sortId?: number;
  description: string;
  status: string;
}

function defaultForm(): FormState {
  return {
    airportId: 0,
    name: '',
    code: '',
    postType: 'OBSERVATION',
    sortId: 1,
    description: '',
    status: 'ON',
  };
}

const form = ref<FormState>(defaultForm());
const drawerOpen = ref(false);
const saving = ref(false);
const isEdit = ref(false);

const drawerTitle = computed(() => (isEdit.value ? t('common.edit') : t('common.create')));

const columns = computed<QTableColumn[]>(() => [
  { name: 'name', label: t('common.name'), field: 'name', align: 'left', sortable: true },
  { name: 'code', label: t('common.code'), field: 'code', align: 'left' },
  { name: 'airportName', label: t('airportPost.airport'), field: 'airportName', align: 'left' },
  { name: 'postType', label: t('airportPost.postType'), field: 'postType', align: 'left' },
  { name: 'sortId', label: t('common.sort'), field: 'sortId', align: 'right' },
  { name: 'description', label: t('common.description'), field: 'description', align: 'left' },
  { name: 'status', label: t('common.status'), field: 'status', align: 'center' },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const searchFields = computed(() => [
  { name: 'name', label: t('common.name'), type: 'input' as const },
  { name: 'code', label: t('common.code'), type: 'input' as const },
]);

async function fetchData(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
}) {
  const resp = await airportPostService.list({
    page: params.page,
    pageSize: params.pageSize,
    noPaging: params.noPaging,
    formValues: params.formValues,
  });
  return {
    items: resp.items as unknown as Record<string, unknown>[],
    total: resp.total,
  };
}

function onCreate() {
  isEdit.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as AirportPostEntity;
  isEdit.value = true;
  form.value = {
    id: entity.id ? String(entity.id) : undefined,
    airportId: entity.airportId ? Number(entity.airportId) : 0,
    name: entity.name || '',
    code: entity.code || '',
    postType: entity.postType || 'OBSERVATION',
    sortId: entity.sortId,
    description: entity.description || '',
    status: entity.status ?? 'ON',
  };
  drawerOpen.value = true;
}

async function onSubmit() {
  saving.value = true;
  try {
    const { id, ...values } = form.value;
    if (isEdit.value && id) {
      await airportPostService.update(id, values);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await airportPostService.create(values);
      Notify.create({ type: 'positive', message: t('common.createSuccess') });
    }
    drawerOpen.value = false;
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  } finally {
    saving.value = false;
  }
}

async function onDelete(row: Record<string, unknown>) {
  const entity = row as unknown as AirportPostEntity;
  if (!entity.id) return;
  try {
    await airportPostService.delete(entity.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}

async function onToggleStatus(row: Record<string, unknown>, val: boolean) {
  const entity = row as unknown as AirportPostEntity;
  if (!entity.id) return;
  try {
    await airportPostService.update(entity.id, { status: val ? 'ON' : 'OFF' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}
</script>

