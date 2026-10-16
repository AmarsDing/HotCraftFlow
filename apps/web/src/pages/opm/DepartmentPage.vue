<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('department.title')"
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
          v-model="form.organizationId"
          :options="organizationOptions"
          :placeholder="t('department.organizationId')"
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <q-select
          v-model="form.parentId"
          :options="parentOptions"
          :placeholder="t('department.parentId')"
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <!-- 部门领导：显式指定（department.leader_id），中心值班页值班领导识别主口径；支持搜索过滤 -->
        <q-select
          v-model="form.leaderId"
          :options="leaderOptions"
          :placeholder="t('department.leaderId')"
          use-input
          input-debounce="150"
          :filter="filterLeader"
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
import { ref, computed, onMounted } from 'vue';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useDepartmentStore, useOrganizationStore } from '../../stores';
import { useTreeCrudPage } from '../../composables/useTreeCrudPage';
import { userService, type DepartmentEntity, type UserEntity } from '../../services';

const {
  t, crudRef, form, drawerOpen, saving, drawerTitle,
  columns, searchFields, parentOptions, fetchData, onCreate, onEdit, onSubmit, onDelete, onToggleStatus,
} = useTreeCrudPage<DepartmentEntity>(useDepartmentStore(), { showOrganization: true, showLeader: true });

// 组织下拉选项（部门归属组织）
const organizationStore = useOrganizationStore();
const organizationOptions = computed<{ label: string; value: number }[]>(() =>
  organizationStore.list.map((o) => ({ label: o.name || '', value: Number(o.id) })),
);

// 部门领导下拉选项：全量用户（leader_id 显式指定口径），本部门成员排前便于选择；
// q-select use-input 过滤走标准反应式：filter 回调只记录搜索词，options computed 随之重算
const users = ref<UserEntity[]>([]);
const leaderFilter = ref('');
const leaderOptions = computed<{ label: string; value: number }[]>(() => {
  const deptId = form.value.id != null ? String(form.value.id) : '';
  const kw = leaderFilter.value.trim().toLowerCase();
  const opt = (u: UserEntity): { label: string; value: number } => ({
    label: u.realname || u.nickname || `#${u.id}`,
    value: Number(u.id),
  });
  const all = users.value.filter((u) => u.id !== undefined && u.id !== null);
  const matched = kw ? all.filter((u) => opt(u).label.toLowerCase().includes(kw)) : all;
  if (!deptId) return matched.map(opt);
  const mine = matched.filter((u) => String(u.orgId ?? '') === deptId).map(opt);
  const rest = matched.filter((u) => String(u.orgId ?? '') !== deptId).map(opt);
  return [...mine, ...rest];
});
function filterLeader(val: string, update: (fn: () => void) => void) {
  update(() => {
    leaderFilter.value = val;
  });
}

onMounted(() => {
  void organizationStore.fetchList({ noPaging: true });
  void userService
    .list({ noPaging: true, fieldMask: 'id,realname,nickname,orgId' })
    .then((r) => (users.value = (r.items || []) as UserEntity[]))
    .catch(() => undefined);
});
</script>

