<template>
  <CrudTablePage
    ref="tableRef"
    :title="t('loginRestriction.title')"
    :columns="columns"
    :search-fields="searchFields"
    :fetch-data-fn="fetchDataFn"
    @create="onCreate"
  >
    <template #banner>
      <q-banner dense class="bg-info text-white q-mb-sm rounded-borders">
        <template #avatar>
          <q-icon name="info" />
        </template>
        {{ t('loginRestriction.gatewayBanner') }}
        <q-btn
          flat
          dense
          no-caps
          class="q-ml-sm text-white"
          icon="open_in_new"
          :label="t('loginRestriction.goGatewayIpRules')"
          @click="goGatewayIpRules"
        />
      </q-banner>
    </template>
    <template #body-cell-targetType="{ row }">
      <q-badge :color="targetTypeColor(row.targetType)" :label="targetTypeLabel(row.targetType)" />
    </template>
    <template #body-cell-action="{ row }">
      <q-badge :color="actionColor(row.action)" :label="actionLabel(row.action)" />
    </template>
    <template #body-cell-status="{ row }">
      <!-- 启用状态列内嵌开关：状态即操作，一点切换（约定：表格启停开关一律放状态列，操作列不再重复） -->
      <q-toggle
        :model-value="(row.status ?? 'ON') === 'ON'"
        color="positive"
        dense
        :disable="togglingId === row.id"
        @update:model-value="onToggle(row)"
      >
        <q-tooltip>{{ (row.status ?? 'ON') === 'ON' ? t('common.enabled') : t('common.disabled') }}</q-tooltip>
      </q-toggle>
    </template>
    <template #body-cell-expireTime="{ row }">
      <span v-if="!row.expireTime">{{ t('loginRestriction.neverExpire') }}</span>
      <span v-else :class="{ 'text-negative': isExpired(row.expireTime) }">
        {{ formatDateTime(row.expireTime) }}
        <q-badge v-if="isExpired(row.expireTime)" color="grey" :label="t('loginRestriction.expired')" class="q-ml-xs" />
      </span>
    </template>
    <template #body-cell-hitCount="{ row }">
      <span>{{ row.hitCount ?? 0 }}</span>
      <q-tooltip v-if="row.lastHitTime">{{ t('loginRestriction.lastHitTime') }}: {{ formatDateTime(row.lastHitTime) }}</q-tooltip>
      <q-tooltip v-else>{{ t('loginRestriction.neverHit') }}</q-tooltip>
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
      <FormDrawer v-model="drawerOpen" :title="drawerTitle" :saving="saving" @submit="onSubmit">
        <q-select
          v-model="form.targetType"
          :placeholder="t('loginRestriction.targetType')"
          :options="targetTypeOptions"
          dense
          outlined
          emit-value
          map-options
          @update:model-value="onTargetTypeChange"
        />
        <!-- USER 目标：下拉选择代替手填（服务端兜底校验用户存在性） -->
        <q-select
          v-if="form.targetType === 'USER'"
          v-model="form.targetValue"
          :placeholder="t('loginRestriction.selectUser')"
          :options="userOptions"
          dense
          outlined
          emit-value
          map-options
          use-input
          input-debounce="0"
          :hint="t('loginRestriction.targetValueUserHint')"
          :rules="[(val: string) => !!val || t('loginRestriction.targetValue')]"
          @filter="onFilterUser"
        />
        <q-input
          v-else
          v-model="form.targetValue"
          :placeholder="t('loginRestriction.targetValue')"
          dense
          outlined
          :hint="t('loginRestriction.targetValueHint')"
          :rules="ipTargetRules"
        />
        <q-select
          v-model="form.action"
          :placeholder="t('loginRestriction.action')"
          :options="actionOptions"
          dense
          outlined
          emit-value
          map-options
        />
        <q-banner v-if="form.action === 'ALLOW'" dense class="bg-warning text-white rounded-borders">
          <template #avatar>
            <q-icon name="warning" />
          </template>
          {{ t('loginRestriction.allowWhitelistWarning') }}
        </q-banner>
        <q-input
          v-model="form.expireTimeLocal"
          :placeholder="t('loginRestriction.expireTime')"
          dense
          outlined
          type="datetime-local"
          clearable
          :hint="t('loginRestriction.neverExpire')"
        />
        <q-input
          v-model="form.reason"
          :placeholder="t('common.reason')"
          dense
          outlined
          type="textarea"
        />
      </FormDrawer>

      <!-- 变更影响报告（Simulate 预演结果）：操作者判定 / 受影响会话 / 规则冲突 -->
      <q-dialog v-model="simulateDialogOpen">
        <q-card style="min-width: 420px; max-width: 560px">
          <q-card-section class="row items-center q-pb-none">
            <q-icon name="assessment" color="primary" size="sm" class="q-mr-sm" />
            <div class="text-h6">{{ t('loginRestriction.simulateTitle') }}</div>
          </q-card-section>
          <q-card-section class="q-gutter-sm">
            <div class="row items-center q-gutter-xs">
              <span class="text-weight-medium">{{ t('loginRestriction.simulateOperator') }}:</span>
              <q-badge
                :color="simulateResult?.operatorAllowed ? 'positive' : 'negative'"
                :label="simulateResult?.operatorAllowed ? t('loginRestriction.simulateOperatorAllowed') : t('loginRestriction.simulateOperatorDenied')"
              />
            </div>
            <div class="row items-center q-gutter-xs">
              <span class="text-weight-medium">{{ t('loginRestriction.simulateAffected') }}:</span>
              <span v-if="(simulateResult?.affectedOnlineSessions ?? 0) > 0" class="text-warning">
                {{ simulateResult?.affectedOnlineSessions }}
              </span>
              <span v-else>{{ t('loginRestriction.simulateAffectedNone') }}</span>
            </div>
            <div>
              <span class="text-weight-medium">{{ t('loginRestriction.simulateConflicts') }}:</span>
              <div v-if="(simulateResult?.conflicts?.length ?? 0) > 0" class="q-mt-xs">
                <div
                  v-for="(c, idx) in simulateResult?.conflicts"
                  :key="idx"
                  class="text-caption q-pa-xs q-mb-xs rounded-borders bg-orange-1"
                >
                  <q-badge outline color="warning" :label="c.type" class="q-mr-xs" />
                  {{ c.detail }}
                </div>
              </div>
              <span v-else class="q-ml-xs">{{ t('loginRestriction.simulateConflictsNone') }}</span>
            </div>
            <q-banner v-if="simulateResult && !simulateResult.operatorAllowed" dense class="bg-negative text-white rounded-borders">
              <template #avatar>
                <q-icon name="lock" />
              </template>
              {{ t('loginRestriction.selfLockMessage') }}
            </q-banner>
          </q-card-section>
          <q-card-actions align="right">
            <q-btn flat no-caps :label="t('loginRestriction.simulateCancel')" @click="simulateDialogOpen = false" />
            <q-btn
              v-if="simulateResult && !simulateResult.operatorAllowed"
              flat
              no-caps
              color="negative"
              :label="t('loginRestriction.forceSave')"
              :loading="saving"
              @click="onForceSubmit"
            />
            <q-btn
              v-else
              unelevated
              no-caps
              color="primary"
              :label="t('loginRestriction.simulateConfirm')"
              :loading="saving"
              @click="onConfirmSubmit"
            />
          </q-card-actions>
        </q-card>
      </q-dialog>
    </template>
  </CrudTablePage>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { Notify, Dialog, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useLoginRestrictionStore } from '../../stores';
import {
  loginRestrictionService,
  userService,
  type LoginRestrictionEntity,
  type LoginRestrictionSimulateResult,
} from '../../services';
import { formatDateTime } from '../../utils/datetime';

const { t } = useI18n();
const router = useRouter();
const store = useLoginRestrictionStore();

const tableRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

// fixed 布局列宽以表头为准：style/headerStyle 必须同步设置（withAutoColWidths 对已有 style 的列直接透传）。
// 分配原则（§4.13）：标识/长文本列（targetValue/reason）留 auto 弹性吃剩余空间，
// 徽章/开关/数值/时间/操作列按内容给固定 px，避免 9 列均分"拉跑道"。
const colW = (px: number): Pick<QTableColumn, 'style' | 'headerStyle'> => {
  const w = `width: ${px}px`;
  return { style: w, headerStyle: w };
};
const colAuto: Pick<QTableColumn, 'style' | 'headerStyle'> = { style: 'width: auto', headerStyle: 'width: auto' };

const columns = computed<QTableColumn[]>(() => [
  // 徽章列："用户限制" 4 字徽章 ≈ 76px + 单元格 padding 32 → 110
  { name: 'targetType', label: t('loginRestriction.targetType'), field: 'targetType', align: 'left', ...colW(110) },
  // 主标识列（CIDR/用户名），弹性
  { name: 'targetValue', label: t('loginRestriction.targetValue'), field: 'targetValue', align: 'left', ...colAuto },
  { name: 'action', label: t('loginRestriction.action'), field: 'action', align: 'left', ...colW(90) },
  // 状态列内嵌 q-toggle（≈48px + padding）
  { name: 'status', label: t('loginRestriction.status'), field: 'status', align: 'center', ...colW(90) },
  // datetime(≈148) + "已过期"徽章(≈66) + padding 32，过期行完整显示不截断
  { name: 'expireTime', label: t('loginRestriction.expireTime'), field: 'expireTime', align: 'left', ...colW(250) },
  // 数值列右对齐；表头"命中次数"需 96
  { name: 'hitCount', label: t('loginRestriction.hitCount'), field: 'hitCount', align: 'right', ...colW(100) },
  // 长文本原因列，弹性（截断由全局 table-tooltip 兜底）
  { name: 'reason', label: t('common.reason'), field: 'reason', align: 'left', ...colAuto },
  { name: 'createTime', label: t('common.createTime'), field: 'createTime', align: 'left', sortable: true, ...colW(190) },
  // 两个图标按钮（≈64 + padding 32）
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false, ...colW(100) },
]);

const targetTypeOptions = computed(() => [
  { label: t('loginRestriction.targetTypeIp'), value: 'IP' },
  { label: t('loginRestriction.targetTypeUser'), value: 'USER' },
]);

const actionOptions = computed(() => [
  { label: t('loginRestriction.actionBlock'), value: 'BLOCK' },
  { label: t('loginRestriction.actionAllow'), value: 'ALLOW' },
]);

const searchFields = computed(() => [
  { name: 'targetType', label: t('loginRestriction.targetType'), type: 'select' as const, options: targetTypeOptions.value },
  { name: 'action', label: t('loginRestriction.action'), type: 'select' as const, options: actionOptions.value },
  { name: 'targetValue', label: t('loginRestriction.targetValue'), type: 'input' as const, fuzzy: true },
]);

// IP/CIDR 客户端校验（服务端 validateLoginRestriction 同口径，提前拦截明显错误）
const ipTargetRules = computed(() => [
  (val: string) => !!val || t('loginRestriction.targetValue'),
  (val: string) => {
    const v = (val ?? '').trim();
    if (/^\d{1,3}(\.\d{1,3}){3}(\/\d{1,2})?$/.test(v)) return true;
    if (/^[0-9a-fA-F:]+(\/\d{1,3})?$/.test(v) && v.includes(':')) return true;
    return t('loginRestriction.targetValueInvalid');
  },
]);

const drawerOpen = ref(false);
const saving = ref(false);
const togglingId = ref<string | number | null>(null);
const editingId = ref<string | number | null>(null);
const form = ref({
  targetType: '',
  targetValue: '',
  action: '',
  reason: '',
  expireTimeLocal: '',
});

const simulateDialogOpen = ref(false);
const simulateResult = ref<LoginRestrictionSimulateResult | null>(null);

// USER 目标下拉（选择代替手填；label 展示 nickname+username，value 为 username——规则按用户名匹配）
const allUserOptions = ref<{ label: string; value: string }[]>([]);
const userOptions = ref<{ label: string; value: string }[]>([]);

onMounted(async () => {
  try {
    const resp = await userService.list({ noPaging: true, fieldMask: 'id,username,nickname' });
    allUserOptions.value = (resp.items ?? []).map((u) => ({
      label: `${u.nickname || u.username || u.id} (${u.username ?? ''})`,
      value: u.username ?? '',
    }));
    userOptions.value = allUserOptions.value;
  } catch {
    // error notification handled by request interceptor
  }
});

function onFilterUser(val: string, update: (cb: () => void) => void) {
  update(() => {
    const kw = val.toLowerCase();
    userOptions.value = kw
      ? allUserOptions.value.filter((o) => o.label.toLowerCase().includes(kw))
      : allUserOptions.value;
  });
}

// 切换目标类型时清空目标值，避免 IP/用户名语义串扰
function onTargetTypeChange() {
  form.value.targetValue = '';
}

const drawerTitle = computed(() => (editingId.value !== null ? t('common.edit') : t('common.create')));

function targetTypeColor(targetType: unknown): string {
  switch (targetType) {
    case 'IP':
      return 'blue';
    case 'USER':
      return 'orange';
    default:
      return 'grey';
  }
}

function targetTypeLabel(targetType: unknown): string {
  switch (targetType) {
    case 'IP':
      return t('loginRestriction.targetTypeIp');
    case 'USER':
      return t('loginRestriction.targetTypeUser');
    default:
      return String(targetType ?? '');
  }
}

function actionColor(action: unknown): string {
  switch (action) {
    case 'BLOCK':
      return 'red';
    case 'ALLOW':
      return 'green';
    default:
      return 'grey';
  }
}

function actionLabel(action: unknown): string {
  switch (action) {
    case 'BLOCK':
      return t('loginRestriction.actionBlock');
    case 'ALLOW':
      return t('loginRestriction.actionAllow');
    default:
      return String(action ?? '');
  }
}

function isExpired(iso?: string | null): boolean {
  if (!iso) return false;
  const d = new Date(iso);
  return !Number.isNaN(d.getTime()) && d.getTime() < Date.now();
}

// datetime-local 值（YYYY-MM-DDTHH:mm）转 RFC3339；空串返回 undefined 表示永不过期
function toRFC3339(local: string): string | undefined {
  if (!local) return undefined;
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

// RFC3339 转 datetime-local 输入框值（本地时区 YYYY-MM-DDTHH:mm）
function toLocalInput(iso?: string | null): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function resetForm() {
  form.value = {
    targetType: '',
    targetValue: '',
    action: '',
    reason: '',
    expireTimeLocal: '',
  };
}

function onCreate() {
  editingId.value = null;
  resetForm();
  drawerOpen.value = true;
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as LoginRestrictionEntity;
  editingId.value = entity.id ?? null;
  form.value = {
    targetType: entity.targetType ?? '',
    targetValue: entity.targetValue ?? '',
    action: entity.action ?? '',
    reason: entity.reason ?? '',
    expireTimeLocal: toLocalInput(entity.expireTime),
  };
  drawerOpen.value = true;
}

function buildPayload(): Partial<LoginRestrictionEntity> {
  return {
    targetType: form.value.targetType,
    targetValue: form.value.targetValue,
    action: form.value.action,
    reason: form.value.reason,
    // 空输入 → null（显式清空过期时间，走 updateMask null → SetNull 链路）；undefined 会被 mask 过滤导致无法清空
    expireTime: toRFC3339(form.value.expireTimeLocal) ?? null,
  };
}

async function doSubmit(payload: Partial<LoginRestrictionEntity>, force: boolean) {
  if (editingId.value !== null) {
    if (force) {
      await loginRestrictionService.updateForce(editingId.value, payload);
    } else {
      await store.update(editingId.value, payload);
    }
  } else {
    if (force) {
      await loginRestrictionService.createForce(payload);
    } else {
      await store.create(payload);
    }
  }
}

// 提交链路：先 Simulate 预演 → 影响报告对话框 → 确认后真正落库（自锁场景仅能 force）
async function onSubmit() {
  saving.value = true;
  try {
    const payload = buildPayload();
    const sim = await loginRestrictionService.simulate(
      editingId.value !== null ? { ...payload, id: Number(editingId.value) } : payload,
      editingId.value !== null ? 'CHANGE_TYPE_UPDATE' : 'CHANGE_TYPE_CREATE',
    );
    simulateResult.value = sim;
    simulateDialogOpen.value = true;
  } catch {
    // error notification handled by request interceptor
  } finally {
    saving.value = false;
  }
}

async function onConfirmSubmit() {
  saving.value = true;
  try {
    await doSubmit(buildPayload(), false);
    Notify.create({ type: 'positive', message: editingId.value !== null ? t('common.updateSuccess') : t('common.createSuccess') });
    simulateDialogOpen.value = false;
    drawerOpen.value = false;
    await tableRef.value?.fetchData();
  } catch {
    // error notification handled by request interceptor
  } finally {
    saving.value = false;
  }
}

async function onForceSubmit() {
  saving.value = true;
  try {
    await doSubmit(buildPayload(), true);
    Notify.create({ type: 'positive', message: editingId.value !== null ? t('common.updateSuccess') : t('common.createSuccess') });
    simulateDialogOpen.value = false;
    drawerOpen.value = false;
    await tableRef.value?.fetchData();
  } catch {
    // error notification handled by request interceptor
  } finally {
    saving.value = false;
  }
}

// 启停开关：状态即操作（部分更新 status 字段，复用 Update + updateMask）
async function onToggle(row: Record<string, unknown>) {
  const entity = row as unknown as LoginRestrictionEntity;
  if (entity.id === undefined || togglingId.value !== null) return;
  togglingId.value = entity.id;
  const next = (entity.status ?? 'ON') === 'ON' ? 'OFF' : 'ON';
  try {
    await store.update(entity.id, { status: next });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await tableRef.value?.fetchData();
  } catch (err) {
    // 自锁护栏 409：停用 ALLOW 规则可能锁死操作者，提供 force 强制更新入口（与删除/表单提交同语义）
    const reason = (err as { response?: { data?: { reason?: string } } })?.response?.data?.reason;
    if (reason === 'LOGIN_RESTRICTION_SELF_LOCK') {
      Dialog.create({
        title: t('loginRestriction.selfLockTitle'),
        message: t('loginRestriction.selfLockMessage'),
        cancel: { flat: true, noCaps: true, label: t('loginRestriction.simulateCancel') },
        ok: { flat: true, noCaps: true, color: 'negative', label: t('loginRestriction.forceSave') },
        persistent: true,
      }).onOk(async () => {
        try {
          await loginRestrictionService.updateForce(entity.id as number, { status: next });
          Notify.create({ type: 'positive', message: t('common.updateSuccess') });
          await tableRef.value?.fetchData();
        } catch {
          // error notification handled by request interceptor
        }
      });
    }
  } finally {
    togglingId.value = null;
  }
}

async function onDelete(row: Record<string, unknown>) {
  try {
    await store.remove(row.id as string | number);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await tableRef.value?.fetchData();
  } catch (err) {
    // 自锁护栏 409：删除 ALLOW 规则可能锁死操作者，提供 force 强制删除入口
    const reason = (err as { response?: { data?: { reason?: string } } })?.response?.data?.reason;
    if (reason === 'LOGIN_RESTRICTION_SELF_LOCK') {
      Dialog.create({
        title: t('loginRestriction.selfLockTitle'),
        message: t('loginRestriction.selfLockMessage'),
        cancel: { flat: true, noCaps: true, label: t('loginRestriction.simulateCancel') },
        ok: { flat: true, noCaps: true, color: 'negative', label: t('loginRestriction.forceDelete') },
        persistent: true,
      }).onOk(async () => {
        try {
          await loginRestrictionService.deleteForce(row.id as string | number);
          Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
          await tableRef.value?.fetchData();
        } catch {
          // error notification handled by request interceptor
        }
      });
    }
  }
}

function goGatewayIpRules() {
  void router.push({ path: '/gateway/security', query: { tab: 'ip-rules' } });
}

async function fetchDataFn(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
}) {
  return store.fetchList({
    page: params.page,
    pageSize: params.pageSize,
    noPaging: params.noPaging,
    formValues: params.formValues,
  });
}
</script>
