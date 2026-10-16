<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('tenant.title')"
    :columns="columns"
    :search-fields="searchFields"
    row-key="id"
    :fetch-data-fn="store.fetchList"
    @create="onCreate"
  >
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
          :rules="[
            (val) => !!val || t('common.required'),
            (val) => /^[A-Za-z0-9_-]{1,64}$/.test(val) || t('tenant.codeInvalid'),
          ]"
        />
        <q-toggle
          :model-value="form.status === 'ON'"
          :label="form.status === 'ON' ? t('common.enabled') : t('common.disabled')"
          @update:model-value="(val) => (form.status = val ? 'ON' : 'OFF')"
        />
        <q-input
          v-model="form.remark"
          :placeholder="t('common.remark')"
          dense
          outlined
          type="textarea"
          rows="2"
        />
      </FormDrawer>
    </template>
  </CrudTablePage>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useTenantStore } from '../../stores';
import type { TenantEntity } from '../../services';

const { t } = useI18n();
const store = useTenantStore();

const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

interface FormState {
  id?: string;
  name: string;
  code: string;
  status: string;
  remark: string;
}

function defaultForm(): FormState {
  return {
    name: '',
    code: '',
    status: 'ON',
    remark: '',
  };
}

const form = ref<FormState>(defaultForm());
const drawerOpen = ref(false);
const saving = ref(false);
const isEdit = ref(false);

const drawerTitle = computed(() =>
  isEdit.value ? t('common.edit') : t('common.create'),
);

const columns = computed<QTableColumn[]>(() => [
  { name: 'name', label: t('common.name'), field: 'name', align: 'left', sortable: true },
  { name: 'code', label: t('common.code'), field: 'code', align: 'left', sortable: true },
  // 成员数为列表实时聚合值（后端 GROUP BY users），排序按库存储字段会与显示值不一致，故不可排序
  { name: 'memberCount', label: t('tenant.memberCount'), field: 'memberCount', align: 'center' },
  { name: 'status', label: t('common.status'), field: 'status', align: 'center' },
  { name: 'createTime', label: t('common.createTime'), field: 'createTime', align: 'center', sortable: true },
  { name: 'remark', label: t('common.remark'), field: 'remark', align: 'left' },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const searchFields = computed(() => [
  { name: 'name', label: t('common.name'), type: 'input' as const, fuzzy: true },
  { name: 'code', label: t('common.code'), type: 'input' as const, fuzzy: true },
  {
    name: 'status',
    label: t('common.status'),
    type: 'select' as const,
    options: [
      { label: t('common.enabled'), value: 'ON' },
      { label: t('common.disabled'), value: 'OFF' },
    ],
  },
]);

function onCreate() {
  isEdit.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as TenantEntity;
  isEdit.value = true;
  form.value = {
    id: entity.id ? String(entity.id) : undefined,
    name: entity.name || '',
    code: entity.code || '',
    status: entity.status ?? 'ON',
    remark: entity.remark || '',
  };
  drawerOpen.value = true;
}

async function onSubmit() {
  saving.value = true;
  try {
    const { id, ...values } = form.value;
    if (isEdit.value && id) {
      await store.update(id, values);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await store.create(values);
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
  const entity = row as unknown as TenantEntity;
  if (!entity.id) return;
  try {
    await store.remove(entity.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}

async function onToggleStatus(row: Record<string, unknown>, val: boolean) {
  const entity = row as unknown as TenantEntity;
  if (!entity.id) return;
  try {
    await store.update(entity.id, { status: val ? 'ON' : 'OFF' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}
</script>

