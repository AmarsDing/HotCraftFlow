<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('organization.title')"
    :columns="columns"
    :search-fields="searchFields"
    row-key="id"
    :tree="true"
    :no-paging="true"
    :fetch-data-fn="fetchData"
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
        <q-select
          v-model="form.parentId"
          :options="parentOptions"
          :placeholder="t('organization.parentId')"
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <q-input
          v-model.number="form.sortId"
          :placeholder="t('common.sort')"
          dense
          outlined
          type="number"
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
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useOrganizationStore } from '../../stores';
import { useTreeCrudPage } from '../../composables/useTreeCrudPage';
import type { OrganizationEntity } from '../../services';

const {
  t, crudRef, form, drawerOpen, saving, drawerTitle,
  columns, searchFields, parentOptions, fetchData, onCreate, onEdit, onSubmit, onDelete, onToggleStatus,
} = useTreeCrudPage<OrganizationEntity>(useOrganizationStore());
</script>

