<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('user.title')"
    :columns="columns"
    :search-fields="searchFields"
    row-key="id"
    :fetch-data-fn="fetchData"
    @create="onCreate"
  >
    <template #toolbar>
      <q-btn flat dense no-caps icon="download" :label="t('common.export')" :loading="exporting" @click="onExport" />
      <q-btn flat dense no-caps icon="upload" :label="t('common.import')" @click="onOpenImport" />
    </template>

    <template #body-cell-avatar="{ row }">
      <UserAvatar
        :avatar="rowAvatar(row)"
        :name="(row.realname as string) || (row.username as string)"
        :size="32"
      />
    </template>

    <template #body-cell-authority="{ row }">
      <q-badge :color="authorityColor(row.authority)" :label="authorityLabel(row.authority)" />
    </template>

    <template #body-cell-orgId="{ row }">
      {{ orgMap.get(Number(row.orgId)) || '-' }}
    </template>

    <template #body-cell-positionId="{ row }">
      {{ positionMap.get(Number(row.positionId)) || '-' }}
    </template>

    <template #body-cell-postIds="{ row }">
      <q-badge
        v-for="pid in (row.postIds as number[] | undefined) || []"
        :key="pid"
        class="q-mr-xs"
        color="secondary"
        :label="postMap.get(Number(pid)) || `#${pid}`"
      />
      <span v-if="!(row.postIds as number[] | undefined)?.length">-</span>
    </template>

    <template #body-cell-roles="{ row }">
      <q-badge
        v-for="code in (row.roles as string[] | undefined) || []"
        :key="code"
        class="q-mr-xs"
        color="primary"
        :label="roleMap.get(code) || code"
      />
      <span v-if="!(row.roles as string[] | undefined)?.length">-</span>
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
        <q-btn class="crud-actions__btn" flat dense round icon="vpn_key" @click="onResetPassword(row)">
          <q-tooltip>{{ t('common.changePassword') }}</q-tooltip>
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
        <div class="row items-center q-gutter-md q-mb-xs">
          <UserAvatar :avatar="form.avatar" :name="form.realname || form.username" :size="64" />
          <div class="column q-gutter-xs">
            <q-btn
              unelevated
              color="primary"
              icon="cloud_upload"
              :label="t('user.uploadAvatar')"
              :loading="avatarUploading"
              @click="avatarInputRef?.click()"
            />
            <div class="text-caption text-grey">{{ t('user.avatarUploadHint') }}</div>
          </div>
          <input
            ref="avatarInputRef"
            type="file"
            accept="image/png,image/jpeg,image/webp,image/gif"
            hidden
            @change="onAvatarPicked"
          />
        </div>
        <q-input
          v-model="form.username"
          :placeholder="t('user.username')"
          dense
          outlined
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-input
          v-if="!isEdit"
          v-model="form.password"
          :placeholder="t('common.password')"
          dense
          outlined
          type="password"
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-input
          v-model="form.workId"
          :placeholder="t('user.workId')"
          dense
          outlined
          :rules="[(val) => !val || /^\d+$/.test(val) || t('user.workIdInvalid')]"
        />
        <q-input
          v-model="form.realname"
          :placeholder="t('user.realName')"
          dense
          outlined
        />
        <q-select
          v-model="form.orgId"
          :options="departmentOptions"
          :placeholder="t('user.orgId')"
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <q-select
          v-model="form.positionId"
          :options="positionOptions"
          :placeholder="t('user.positionId')"
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <q-select
          v-model="form.postIds"
          :options="postOptions"
          :placeholder="t('user.postIds')"
          multiple
          use-chips
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <q-select
          v-model="form.roles"
          :options="roleOptions"
          :placeholder="t('user.roles')"
          multiple
          use-chips
          emit-value
          map-options
          dense
          outlined
        />
        <q-input
          v-model="form.email"
          :placeholder="t('user.email')"
          dense
          outlined
        />
        <q-input
          v-model="form.mobile"
          :placeholder="t('user.mobile')"
          dense
          outlined
        />
        <q-select
          v-model="form.gender"
          :placeholder="t('user.gender')"
          :options="genderOptions"
          dense
          outlined
          emit-value
          map-options
        />
        <q-select
          v-model="form.authority"
          :placeholder="t('user.authority')"
          :options="authorityOptions"
          dense
          outlined
          emit-value
          map-options
        />
        <q-toggle
          :model-value="form.status === 'ON'"
          :label="form.status === 'ON' ? t('user.statusEnabled') : t('user.statusDisabled')"
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

  <!-- 批量导入对话框 -->
  <q-dialog v-model="importOpen" persistent>
    <q-card style="min-width: 560px; max-width: 90vw">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">{{ t('user.importTitle') }}</div>
        <q-space />
        <q-btn icon="close" flat round dense v-close-popup />
      </q-card-section>
      <q-card-section class="q-gutter-md">
        <div class="row items-center q-gutter-sm">
          <span class="text-caption text-grey">{{ t('user.importTip') }}</span>
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            icon="download"
            :label="t('user.downloadTemplate')"
            @click="onDownloadTemplate"
          />
        </div>
        <q-file
          v-model="importFile"
          accept=".xlsx"
          outlined
          dense
          clearable
          :label="t('user.selectFile')"
        >
          <template #prepend>
            <q-icon name="attach_file" />
          </template>
        </q-file>
        <div v-if="importResult" class="import-result">
          <div class="text-subtitle2 q-mb-sm">{{ t('user.importResult') }}</div>
          <div class="row q-gutter-md q-mb-sm">
            <q-badge color="primary" :label="`${t('user.importTotal')}: ${importResult.total}`" />
            <q-badge color="green" :label="`${t('user.importSuccess')}: ${importResult.successCount}`" />
            <q-badge color="red" :label="`${t('user.importFailed')}: ${importResult.failedCount}`" />
          </div>
          <q-markup-table v-if="importResult.errors?.length" flat dense class="import-errors">
            <thead>
              <tr>
                <th class="text-left">{{ t('user.importRow') }}</th>
                <th class="text-left">{{ t('user.username') }}</th>
                <th class="text-left">{{ t('common.reason') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="err in importResult.errors" :key="err.row">
                <td>{{ err.row }}</td>
                <td>{{ err.username }}</td>
                <td>{{ err.message }}</td>
              </tr>
            </tbody>
          </q-markup-table>
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="t('common.cancel')" v-close-popup />
        <q-btn
          color="primary"
          :loading="importing"
          :disable="!importFile"
          :label="t('common.confirm')"
          @click="onConfirmImport"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>

  <!-- 头像裁剪弹窗 -->
  <AvatarCropper
    v-model="cropperOpen"
    :src="cropperSrc"
    :confirming="avatarUploading"
    @confirm="onAvatarCropConfirm"
  />

  <!-- 重置密码对话框 -->
  <q-dialog v-model="resetPwdOpen">
    <q-card style="min-width: 360px">
      <q-card-section>
        <div class="text-h6">{{ t('common.changePassword') }}</div>
        <div class="text-caption text-grey">{{ resetTarget?.username }}</div>
      </q-card-section>
      <q-card-section class="q-gutter-md">
        <q-input
          v-model="resetPwdForm.newPassword"
          :placeholder="t('common.newPassword')"
          dense
          outlined
          type="password"
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-input
          v-model="resetPwdForm.confirmPassword"
          :placeholder="t('common.confirmPassword')"
          dense
          outlined
          type="password"
          :rules="[
            (val) => !!val || t('common.required'),
            (val) => val === resetPwdForm.newPassword || t('common.required'),
          ]"
        />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="t('common.cancel')" v-close-popup />
        <q-btn
          color="primary"
          :loading="resettingPwd"
          :label="t('common.confirm')"
          @click="onConfirmResetPassword"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import UserAvatar from '../../components/UserAvatar.vue';
import AvatarCropper from '../../components/AvatarCropper.vue';
import { useUserStore, useDepartmentStore, usePositionStore, useRoleStore } from '../../stores';
import {
  authService,
  userService,
  userIOService,
  airportPostService,
  type UserEntity,
  type AirportPostEntity,
  type ImportUsersResult,
} from '../../services';
import { buildQueryParams } from '../../services/request';

const { t } = useI18n();
const store = useUserStore();
const departmentStore = useDepartmentStore();
const positionStore = usePositionStore();
const roleStore = useRoleStore();

const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

interface FormState {
  id?: string;
  username: string;
  password: string;
  workId: string;
  avatar: string;
  realname: string;
  orgId: number | null;
  positionId: number | null;
  postIds: number[];
  roles: string[];
  email: string;
  mobile: string;
  gender: string;
  authority: string;
  status: string;
  remark: string;
}

function defaultForm(): FormState {
  return {
    username: '',
    password: '',
    workId: '',
    avatar: '',
    realname: '',
    orgId: null,
    positionId: null,
    postIds: [],
    roles: [],
    email: '',
    mobile: '',
    gender: 'SECRET',
    authority: 'CUSTOMER_USER',
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

const genderOptions = computed(() => [
  { label: t('user.genderMale'), value: 'MALE' },
  { label: t('user.genderFemale'), value: 'FEMALE' },
  { label: t('user.genderUnknown'), value: 'SECRET' },
]);

const authorityOptions = computed(() => [
  { label: t('user.authorityCustomer'), value: 'CUSTOMER_USER' },
  { label: t('user.authorityTenantAdmin'), value: 'TENANT_ADMIN' },
  { label: t('user.authoritySysAdmin'), value: 'SYS_ADMIN' },
]);

// 部门下拉（扁平，显示 name）
const departmentOptions = computed<{ label: string; value: number }[]>(() =>
  departmentStore.list.map((d) => ({ label: d.name || '', value: Number(d.id) })),
);
// 职位下拉
const positionOptions = computed<{ label: string; value: number }[]>(() =>
  positionStore.list.map((p) => ({ label: p.name || '', value: Number(p.id) })),
);
// 岗位下拉（airport_posts，原气象岗位；多选挂 ap_org_user_post）
const postRows = ref<AirportPostEntity[]>([]);
const postOptions = computed<{ label: string; value: number }[]>(() =>
  postRows.value
    .filter((p) => p.id != null)
    .map((p) => ({ label: p.code ? `${p.name ?? ''} (${p.code})` : (p.name ?? String(p.id)), value: Number(p.id) })),
);
const postMap = computed(() => new Map(postRows.value.filter((p) => p.id != null).map((p) => [Number(p.id), p.name || ''])));
// 角色下拉（value 为 code）
const roleOptions = computed<{ label: string; value: string }[]>(() =>
  roleStore.list
    .filter((r) => r.code)
    .map((r) => ({ label: r.name || r.code || '', value: r.code as string })),
);

// 列表 id→name 映射
const orgMap = computed(() => new Map(departmentStore.list.map((d) => [Number(d.id), d.name || ''])));
const positionMap = computed(() => new Map(positionStore.list.map((p) => [Number(p.id), p.name || ''])));
const roleMap = computed(() => new Map(roleStore.list.filter((r) => r.code).map((r) => [r.code as string, r.name || r.code || ''])));

function rowAvatar(row: Record<string, unknown>): string | undefined {
  return row.avatar as string | undefined;
}

function authorityColor(authority?: string): string {
  switch (authority) {
    case 'SYS_ADMIN':
      return 'red';
    case 'TENANT_ADMIN':
      return 'orange';
    case 'CUSTOMER_USER':
      return 'blue';
    default:
      return 'grey';
  }
}

function authorityLabel(authority?: string): string {
  switch (authority) {
    case 'SYS_ADMIN':
      return t('user.authoritySysAdmin');
    case 'TENANT_ADMIN':
      return t('user.authorityTenantAdmin');
    case 'CUSTOMER_USER':
      return t('user.authorityCustomer');
    default:
      return t('user.authorityGuest');
  }
}

// 列宽显式指定（fixed 布局以表头宽度为准）：手机号/权限放宽，最后登录收窄，其余按语义协调
const colWidth = (px: number) => `width: ${px}px`;

const columns = computed<QTableColumn[]>(() => [
  { name: 'avatar', label: t('user.avatar'), field: 'avatar', align: 'center', style: colWidth(64), headerStyle: colWidth(64) },
  { name: 'username', label: t('user.username'), field: 'username', align: 'left', sortable: true },
  { name: 'realname', label: t('user.realName'), field: 'realname', align: 'left' },
  { name: 'workId', label: t('user.workId'), field: 'workId', align: 'left', style: colWidth(90), headerStyle: colWidth(90) },
  { name: 'orgId', label: t('user.orgId'), field: 'orgId', align: 'left', style: colWidth(130), headerStyle: colWidth(130) },
  { name: 'positionId', label: t('user.positionId'), field: 'positionId', align: 'left', style: colWidth(110), headerStyle: colWidth(110) },
  { name: 'postIds', label: t('user.postIds'), field: 'postIds', align: 'left', style: colWidth(150), headerStyle: colWidth(150) },
  { name: 'roles', label: t('user.roles'), field: 'roles', align: 'left', style: colWidth(160), headerStyle: colWidth(160) },
  { name: 'mobile', label: t('user.mobile'), field: 'mobile', align: 'left', style: colWidth(150), headerStyle: colWidth(150) },
  { name: 'authority', label: t('user.authority'), field: 'authority', align: 'center', style: colWidth(130), headerStyle: colWidth(130) },
  { name: 'status', label: t('common.status'), field: 'status', align: 'center', style: colWidth(88), headerStyle: colWidth(88) },
  { name: 'lastLoginTime', label: t('user.lastLoginTime'), field: 'lastLoginTime', align: 'center', sortable: true, style: colWidth(170), headerStyle: colWidth(170) },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const statusOptions = computed(() => [
  { label: t('user.statusEnabled'), value: 'ON' },
  { label: t('user.statusDisabled'), value: 'OFF' },
]);

const searchFields = computed(() => [
  { name: 'username', label: t('user.username'), type: 'input' as const },
  { name: 'realname', label: t('user.realName'), type: 'input' as const },
  { name: 'mobile', label: t('user.mobile'), type: 'input' as const },
  {
    name: 'authority',
    label: t('user.authority'),
    type: 'select' as const,
    options: authorityOptions.value,
  },
  {
    name: 'status',
    label: t('common.status'),
    type: 'select' as const,
    options: statusOptions.value,
  },
]);

async function fetchData(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
  orderBy?: string;
}) {
  lastFormValues.value = params.formValues;
  const resp = await store.fetchList({
    page: params.page,
    pageSize: params.pageSize,
    noPaging: params.noPaging,
    formValues: params.formValues,
    orderBy: params.orderBy,
  });
  return { items: resp.items as unknown as Record<string, unknown>[], total: resp.total };
}

function onCreate() {
  isEdit.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

// 头像上传：选择图片后先进入裁剪弹窗（类微信拖拽/缩放），确认裁成 256x256 PNG 再上传，
// form.avatar 存相对路径，随表单提交落库；裁剪输出恒定小体积，输入限制放宽到 10MB
const avatarInputRef = ref<HTMLInputElement | null>(null);
const avatarUploading = ref(false);
const cropperOpen = ref(false);
const cropperSrc = ref('');

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

async function onAvatarPicked(evt: Event) {
  const input = evt.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = ''; // 允许重选同一文件再次触发 change
  if (!file) return;
  if (!/^image\/(png|jpe?g|webp|gif)$/.test(file.type)) {
    Notify.create({ type: 'warning', message: t('user.avatarTypeInvalid') });
    return;
  }
  if (file.size > 10 * 1024 * 1024) {
    Notify.create({ type: 'warning', message: t('user.avatarTooLarge') });
    return;
  }
  cropperSrc.value = await readAsDataURL(file);
  cropperOpen.value = true;
}

async function onAvatarCropConfirm(blob: Blob) {
  avatarUploading.value = true;
  try {
    const file = new File([blob], 'avatar.png', { type: 'image/png' });
    form.value.avatar = await userService.uploadAvatar(file);
    cropperOpen.value = false;
  } catch {
    // error notification handled by axios interceptor
  } finally {
    avatarUploading.value = false;
  }
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as UserEntity;
  isEdit.value = true;
  form.value = {
    id: entity.id != null ? String(entity.id) : undefined,
    username: entity.username || '',
    password: '',
    workId: entity.workId || '',
    avatar: entity.avatar || '',
    realname: entity.realname || '',
    orgId: entity.orgId != null && entity.orgId !== '' ? Number(entity.orgId) : null,
    positionId: entity.positionId != null && entity.positionId !== '' ? Number(entity.positionId) : null,
    postIds: entity.postIds ? entity.postIds.map(Number) : [],
    roles: entity.roles ? [...entity.roles] : [],
    email: entity.email || '',
    mobile: entity.mobile || '',
    gender: entity.gender ?? 'SECRET',
    authority: entity.authority || 'CUSTOMER_USER',
    status: entity.status ?? 'ON',
    remark: entity.remark || '',
  };
  drawerOpen.value = true;
}

async function onSubmit() {
  saving.value = true;
  try {
    const { id, password, orgId, positionId, workId, ...rest } = form.value;
    const values: Record<string, unknown> = { ...rest };
    if (orgId !== null) values.orgId = orgId;
    if (positionId !== null) values.positionId = positionId;
    // workId 后端为 uint32，空串会 400（CODEC），仅在填写时按数字提交
    if (workId.trim() !== '') values.workId = Number(workId);
    if (isEdit.value && id) {
      const updateValues: Partial<UserEntity> = { ...(values as Partial<UserEntity>) };
      if (password) {
        updateValues.password = password;
      }
      await store.update(id, updateValues);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await store.create({ ...(values as Partial<UserEntity>), password });
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
  const entity = row as unknown as UserEntity;
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
  const entity = row as unknown as UserEntity;
  if (!entity.id) return;
  try {
    await store.update(entity.id, { status: val ? 'ON' : 'OFF' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}

// 批量导出（1.6）
const exporting = ref(false);
// 最近一次列表查询的筛选条件，导出时复用（与列表 query 序列化格式一致）
const lastFormValues = ref<Record<string, unknown>>({});

async function onExport() {
  exporting.value = true;
  try {
    await userIOService.exportUsers(buildQueryParams(lastFormValues.value).query);
    Notify.create({ type: 'positive', message: t('common.export') + ' OK' });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    exporting.value = false;
  }
}

// 批量导入（1.6）
const importOpen = ref(false);
const importing = ref(false);
const importFile = ref<File | null>(null);
const importResult = ref<ImportUsersResult | null>(null);

function onOpenImport() {
  importFile.value = null;
  importResult.value = null;
  importOpen.value = true;
}

async function onDownloadTemplate() {
  try {
    await userIOService.downloadImportTemplate();
  } catch {
    // error notification handled by axios interceptor
  }
}

async function onConfirmImport() {
  if (!importFile.value) return;
  importing.value = true;
  try {
    const result = await userIOService.importUsers(importFile.value);
    importResult.value = result;
    if (result.failedCount === 0) {
      Notify.create({ type: 'positive', message: `${t('common.import')}: ${result.successCount}/${result.total}` });
    } else {
      Notify.create({ type: 'warning', message: `${t('user.importFailed')}: ${result.failedCount}` });
    }
    if (result.successCount > 0) {
      await crudRef.value?.fetchData();
    }
  } catch {
    // error notification handled by axios interceptor
  } finally {
    importing.value = false;
  }
}

// 重置密码
const resetPwdOpen = ref(false);
const resettingPwd = ref(false);
const resetTarget = ref<UserEntity | null>(null);
const resetPwdForm = ref({ newPassword: '', confirmPassword: '' });

function onResetPassword(row: Record<string, unknown>) {
  const entity = row as unknown as UserEntity;
  resetTarget.value = entity;
  resetPwdForm.value = { newPassword: '', confirmPassword: '' };
  resetPwdOpen.value = true;
}

async function onConfirmResetPassword() {
  if (!resetTarget.value?.id) return;
  if (resetPwdForm.value.newPassword !== resetPwdForm.value.confirmPassword) {
    Notify.create({ type: 'negative', message: t('common.required') });
    return;
  }
  resettingPwd.value = true;
  try {
    await authService.changePassword({
      newPassword: resetPwdForm.value.newPassword,
      userId: Number(resetTarget.value.id),
    });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    resetPwdOpen.value = false;
  } catch {
    // error notification handled by axios interceptor
  } finally {
    resettingPwd.value = false;
  }
}

onMounted(() => {
  void departmentStore.fetchList({ noPaging: true });
  void positionStore.fetchList({ noPaging: true });
  void roleStore.fetchList({ noPaging: true });
  airportPostService
    .list({ noPaging: true })
    .then((resp) => {
      postRows.value = resp.items ?? [];
    })
    .catch(() => {
      postRows.value = [];
    });
});
</script>

<style lang="sass" scoped>
.import-errors
  max-height: 240px
  overflow: auto
  display: block
  border: 1px solid var(--glass-border)
  border-radius: 8px
</style>

