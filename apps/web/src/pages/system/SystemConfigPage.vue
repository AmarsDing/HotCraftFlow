<template>
  <div>
    <!-- 系统名称设置（system_configs.system_name 友好编辑）：全站名称统一来源 -->
    <SystemNameCard />
    <!-- 大屏品牌配置（screen.twin.title/title_style/bg_image 三键友好编辑）：机场大屏标题/字体/背景图 -->
    <!-- ScreenBrandingCard removed in FrameWork -->
    <!-- 远程运维设置区（远程运维能力增强 F5，概要设计 §5.3）：ops.* 三键友好编辑 -->
    <!-- OpsRemoteSettingsCard removed in FrameWork -->
    <CrudTablePage
      ref="crudRef"
      :title="t('systemConfig.title')"
      :columns="columns"
      :search-fields="searchFields"
      row-key="id"
      :fetch-data-fn="fetchData"
      @create="onCreate"
    >
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
          v-model="form.configKey"
          :placeholder="t('systemConfig.configKey')"
          dense
          outlined
          :readonly="isEdit"
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-input
          v-model="form.configValue"
          :placeholder="t('systemConfig.configValue')"
          dense
          outlined
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-select
          v-model="form.configType"
          :placeholder="t('systemConfig.configType')"
          dense
          outlined
          emit-value
          map-options
          :options="typeOptions"
        />
        <q-input
          v-model="form.description"
          :placeholder="t('common.description')"
          dense
          outlined
          type="textarea"
          rows="2"
        />
      </FormDrawer>
    </template>
  </CrudTablePage>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import SystemNameCard from './components/SystemNameCard.vue';
import { useSystemConfigStore } from '../../stores';
import { useSystemNameStore } from '../../stores/system-name';
import type { SystemConfigEntity } from '../../services';

const { t } = useI18n();
const store = useSystemConfigStore();
const systemNameStore = useSystemNameStore();

const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

interface FormState {
  id?: string;
  configKey: string;
  configValue: string;
  configType: string;
  description: string;
}

function defaultForm(): FormState {
  return {
    configKey: '',
    configValue: '',
    configType: 'string',
    description: '',
  };
}

const form = ref<FormState>(defaultForm());
const drawerOpen = ref(false);
const saving = ref(false);
const isEdit = ref(false);

const drawerTitle = computed(() =>
  isEdit.value ? t('common.edit') : t('common.create'),
);

const typeOptions = computed(() => [
  { label: t('systemConfig.typeString'), value: 'string' },
  { label: t('systemConfig.typeNumber'), value: 'number' },
  { label: t('systemConfig.typeBool'), value: 'bool' },
  { label: t('systemConfig.typeJson'), value: 'json' },
]);

const columns = computed<QTableColumn[]>(() => [
  { name: 'configKey', label: t('systemConfig.configKey'), field: 'configKey', align: 'left', sortable: true },
  { name: 'configValue', label: t('systemConfig.configValue'), field: 'configValue', align: 'left' },
  { name: 'configType', label: t('systemConfig.configType'), field: 'configType', align: 'center' },
  { name: 'description', label: t('common.description'), field: 'description', align: 'left' },
  { name: 'updateTime', label: t('common.updateTime'), field: 'updateTime', align: 'center', sortable: true },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const searchFields = computed(() => [
  { name: 'configKey', label: t('systemConfig.configKey'), type: 'input' as const },
  { name: 'configType', label: t('systemConfig.configType'), type: 'select' as const, options: typeOptions.value },
]);

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

function onCreate() {
  isEdit.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as SystemConfigEntity;
  isEdit.value = true;
  form.value = {
    id: entity.id ? String(entity.id) : undefined,
    configKey: entity.configKey || '',
    configValue: entity.configValue || '',
    configType: entity.configType || 'string',
    description: entity.description || '',
  };
  drawerOpen.value = true;
}

// 按 configType 校验 configValue；返回错误提示 key，合法时返回空字符串
function validateConfigValue(type: string, value: string): string {
  const v = (value ?? '').trim();
  switch (type) {
    case 'number':
      if (v !== '' && Number.isNaN(Number(v))) return t('systemConfig.invalidNumber');
      return '';
    case 'bool':
      if (v !== '' && v !== 'true' && v !== 'false') return t('systemConfig.invalidBool');
      return '';
    case 'json':
      if (v !== '') {
        try {
          JSON.parse(v);
        } catch {
          return t('systemConfig.invalidJson');
        }
      }
      return '';
    default:
      return '';
  }
}

async function onSubmit() {
  const err = validateConfigValue(form.value.configType, form.value.configValue);
  if (err) {
    Notify.create({ type: 'negative', message: err });
    return;
  }
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
    // 原始表直接改 system_name 时同步全站名称（与 SystemNameCard 同口径）
    if (form.value.configKey === 'system_name' && values.configValue?.trim()) {
      systemNameStore.setName(values.configValue);
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
  const entity = row as unknown as SystemConfigEntity;
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
