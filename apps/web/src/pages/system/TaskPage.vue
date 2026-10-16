<template>
  <CrudTablePage
    ref="crudRef"
    :title="t('task.title')"
    :columns="columns"
    :search-fields="searchFields"
    row-key="id"
    :no-paging="true"
    :fetch-data-fn="fetchData"
    @create="onCreate"
  >
    <template #body-cell-type="{ row }">
      <q-badge
        :color="typeColor(row.type as string)"
        :label="typeLabel(row.type as string)"
      />
      <q-badge
        v-if="(row as unknown as TaskEntity).builtin"
        class="q-ml-xs"
        color="deep-purple"
        outline
        :label="t('task.builtin')"
      />
    </template>

    <template #body-cell-lastExecution="{ row }">
      <template v-if="lastExec(row)">
        <q-badge
          :color="execStatusColor(lastExec(row)?.status)"
          :label="execStatusLabel(lastExec(row)?.status)"
        />
        <div class="text-caption text-grey-7">
          {{ formatDateTime(lastExec(row)?.startedAt) }}
          <span v-if="lastExec(row)?.durationMs != null">
            · {{ formatDuration(lastExec(row)?.durationMs) }}
          </span>
        </div>
        <div
          v-if="lastExec(row)?.error"
          class="text-caption text-negative ellipsis"
          style="max-width: 220px"
        >
          {{ lastExec(row)?.error }}
        </div>
      </template>
      <span v-else class="text-grey-6">{{ t('task.neverRun') }}</span>
    </template>

    <template #body-cell-name="{ row }">
      <!-- 显示名优先，留空回退处理器名；技术名 caption 辅助辨识 -->
      <div>{{ (row as unknown as TaskEntity).name || row.typeName }}</div>
      <div v-if="(row as unknown as TaskEntity).name" class="text-caption text-grey-7">
        {{ row.typeName }}
      </div>
    </template>

    <template #body-cell-cronSpec="{ row }">
      <div>{{ row.cronSpec }}</div>
      <div v-if="cronDesc(row.cronSpec as string)" class="text-caption text-grey-7">
        {{ cronDesc(row.cronSpec as string) }}
      </div>
    </template>

    <template #body-cell-taskPayload="{ row }">
      <div class="ellipsis" style="max-width: 260px">
        {{ row.taskPayload }}
        <q-tooltip v-if="row.taskPayload" max-width="480px" class="text-pre-wrap">{{ row.taskPayload }}</q-tooltip>
      </div>
    </template>

    <template #body-cell-remark="{ row }">
      <div class="ellipsis" style="max-width: 200px">
        {{ row.remark }}
        <q-tooltip v-if="row.remark" max-width="360px" class="text-pre-wrap">{{ row.remark }}</q-tooltip>
      </div>
    </template>

    <template #body-cell-enable="{ row }">
      <!-- 启用状态列内嵌开关：状态即操作（项目约定：q-toggle color=positive + 行 id 防连点 + tooltip 当前状态） -->
      <q-toggle
        :model-value="!!row.enable"
        color="positive"
        dense
        :disable="togglingId === rowId(row)"
        @update:model-value="(val) => onToggleEnabled(row, !!val)"
      >
        <q-tooltip>{{ row.enable ? t('common.enabled') : t('common.disabled') }}</q-tooltip>
      </q-toggle>
    </template>

    <template #body-cell-actions="{ row }">
      <div class="crud-actions">
        <q-btn class="crud-actions__btn" flat dense round icon="play_arrow" :loading="runningId === rowId(row)" @click="onRunNow(row)">
          <q-tooltip>{{ t('task.runNow') }} — {{ t('task.runNowHint') }}</q-tooltip>
        </q-btn>
        <q-btn class="crud-actions__btn" flat dense round icon="history" @click="onShowHistory(row)">
          <q-tooltip>{{ t('task.execHistory') }}</q-tooltip>
        </q-btn>
        <q-btn class="crud-actions__btn" flat dense round icon="edit" @click="onEdit(row)">
          <q-tooltip>{{ t('common.edit') }}</q-tooltip>
        </q-btn>
        <q-btn
          class="crud-actions__btn crud-actions__btn--danger"
          flat dense round icon="delete"
          :disable="!!(row as unknown as TaskEntity).builtin"
        >
          <q-tooltip>
            {{ (row as unknown as TaskEntity).builtin ? t('task.builtinNoDelete') : t('common.delete') }}
          </q-tooltip>
          <q-popup-confirm v-if="!(row as unknown as TaskEntity).builtin" @confirm="onDelete(row)">
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
        <!-- 基本信息 -->
        <div class="form-group-title">{{ t('task.groupBasic') }}</div>
        <q-select
          v-model="form.type"
          :placeholder="t('common.type')"
          :options="typeOptions"
          emit-value
          map-options
          dense
          outlined
          disable
          :hint="t('task.typeHint')"
        />
        <q-input
          v-model="form.name"
          :placeholder="t('task.name')"
          dense
          outlined
          maxlength="64"
          :hint="t('task.nameHint')"
        />
        <q-select
          v-model="form.typeName"
          :placeholder="t('task.handler')"
          :options="handlerOptions"
          option-value="value"
          option-label="label"
          emit-value
          map-options
          dense
          outlined
          :disable="isEdit && form.builtin"
          :hint="typeNameFieldHint"
          :rules="[(val) => !!val || t('common.required')]"
        >
          <template #option="{ itemProps, opt }">
            <q-item v-bind="itemProps">
              <q-item-section>
                <q-item-label>{{ opt.label }}</q-item-label>
                <q-item-label caption>{{ opt.description }}</q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-select>
        <q-toggle
          v-model="form.enable"
          :label="form.enable ? t('common.enabled') : t('common.disabled')"
        />

        <!-- 调度规则 -->
        <div class="form-group-title">{{ t('task.groupSchedule') }}</div>
        <q-select
          v-if="form.type === 'PERIODIC'"
          :model-value="cronPresetPicked"
          :options="cronPresetOptions"
          :placeholder="t('task.cronPresets')"
          emit-value
          map-options
          dense
          outlined
          options-dense
          @update:model-value="(val) => onApplyCronPreset(val as string | null)"
        />
        <q-input
          v-if="form.type === 'PERIODIC'"
          v-model="form.cronSpec"
          :placeholder="t('common.cronSpec')"
          dense
          outlined
          :hint="t('task.cronHint')"
          :rules="cronRules"
        />
        <div v-if="form.type === 'PERIODIC' && formCronDesc" class="text-caption text-primary q-mb-sm">
          {{ formCronDesc }}
          <template v-if="formNextRuns.length">
            <br />{{ t('task.nextRuns') }}：{{ formNextRuns.join('；') }}
          </template>
        </div>
        <div v-if="form.type !== 'PERIODIC'" class="text-caption text-grey-7 q-mb-sm">
          {{ t('task.nonPeriodicHint') }}
        </div>

        <!-- 执行参数 -->
        <div class="form-group-title">{{ t('task.groupExec') }}</div>

        <!-- 参数填写指引卡片：选中处理器后规则+示例直接可见；未选时提示先选处理器 -->
        <div v-if="selectedHandler" class="payload-guide q-mb-sm">
          <div class="payload-guide__row">
            <q-icon name="rule" size="16px" color="primary" />
            <span>{{ selectedHandler.payloadDescription || t('task.payloadHint') }}</span>
          </div>
          <div v-if="selectedHandler.payloadExample" class="payload-guide__row">
            <q-icon name="data_object" size="16px" color="primary" />
            <code class="payload-guide__code">{{ selectedHandler.payloadExample }}</code>
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              color="primary"
              icon="auto_fix_high"
              :label="t('task.fillExample')"
              @click="fillPayloadExample"
            />
          </div>
        </div>
        <div v-else class="payload-guide payload-guide--muted q-mb-sm">
          <q-icon name="info_outline" size="16px" />
          <span>{{ t('task.selectHandlerFirst') }}</span>
        </div>

        <q-input
          v-model="form.taskPayload"
          :placeholder="t('common.payload')"
          dense
          outlined
          type="textarea"
          rows="3"
          :hint="t('task.payloadHint')"
          :rules="payloadRules"
        />
        <div class="row q-col-gutter-sm">
          <q-input
            v-model.number="form.retryCount"
            class="col-6"
            type="number"
            min="0"
            max="25"
            :placeholder="t('task.retryCount')"
            dense
            outlined
            :hint="t('task.retryHint')"
          />
          <q-input
            v-model.number="form.timeoutSec"
            class="col-6"
            type="number"
            min="0"
            :placeholder="t('task.timeoutSec')"
            dense
            outlined
            :hint="t('task.timeoutHint')"
          />
        </div>
        <q-input
          v-model="form.remark"
          :placeholder="t('common.remark')"
          dense
          outlined
          type="textarea"
          rows="2"
        />
      </FormDrawer>

      <q-dialog v-model="historyOpen">
        <q-card style="min-width: 720px; max-width: 90vw">
          <q-card-section class="row items-center q-pb-none">
            <div class="text-h6">{{ t('task.execHistory') }} — {{ historyTaskName }}</div>
            <q-space />
            <q-btn v-close-popup flat dense round icon="close" />
          </q-card-section>
          <q-card-section>
            <q-table
              :rows="historyRows"
              :columns="historyColumns"
              row-key="id"
              flat
              dense
              :loading="historyLoading"
              :pagination="{ rowsPerPage: 10 }"
            >
              <template #body-cell-status="{ row: execRow }">
                <q-badge
                  :color="execStatusColor(execRow.status)"
                  :label="execStatusLabel(execRow.status)"
                />
              </template>
              <template #body-cell-startedAt="{ row: execRow }">
                {{ formatDateTime(execRow.startedAt) }}
              </template>
              <template #body-cell-finishedAt="{ row: execRow }">
                {{ formatDateTime(execRow.finishedAt) }}
              </template>
              <template #body-cell-durationMs="{ row: execRow }">
                {{ formatDuration(execRow.durationMs) }}
              </template>
              <template #no-data>
                <div class="full-width text-center text-grey-6 q-pa-md">
                  {{ t('task.noExecutions') }}
                </div>
              </template>
            </q-table>
          </q-card-section>
        </q-card>
      </q-dialog>
    </template>
  </CrudTablePage>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useTaskStore } from '../../stores';
import { taskService } from '../../services';
import type { TaskEntity, TaskExecutionEntity, TaskOptionEntity } from '../../services';
import { formatDateTime } from '../../utils/datetime';
import { describeCron, nextRuns } from '../../utils/cron';

const { t, locale } = useI18n();
const store = useTaskStore();

const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);

interface FormState {
  id?: string;
  type: string;
  typeName: string;
  name: string;
  cronSpec: string;
  taskPayload: string;
  retryCount: number;
  timeoutSec: number;
  enable: boolean;
  remark: string;
  builtin?: boolean;
}

function defaultForm(): FormState {
  return {
    type: 'PERIODIC',
    typeName: '',
    name: '',
    cronSpec: '',
    taskPayload: '',
    retryCount: 0,
    timeoutSec: 0,
    enable: true,
    remark: '',
    builtin: false,
  };
}

const form = ref<FormState>(defaultForm());
const drawerOpen = ref(false);
const saving = ref(false);
const isEdit = ref(false);

// 编辑态标题带任务名（显示名优先，回退处理器名），提升辨识度
const drawerTitle = computed(() => {
  const base = isEdit.value ? t('common.edit') : t('common.create');
  const label = form.value.name || form.value.typeName;
  return isEdit.value && label ? `${base} — ${label}` : base;
});

// 类型收敛：仅支持创建周期任务（一次性任务由程序侧发起，手动触发走 RunTask）
const typeOptions = computed(() => [
  { label: t('task.typePeriodic'), value: 'PERIODIC' },
]);

// 已注册处理器下拉（数据源 GET /api/v1/monitor/task-handlers，与后端创建校验白名单同源）
interface HandlerOption {
  label: string;
  value: string;
  description: string;
  payloadDescription: string;
  payloadExample: string;
}
const handlerOptions = ref<HandlerOption[]>([]);

// 选中的处理器（数据关联：类型名 → 描述/参数规则/示例载荷 全联动）
const selectedHandler = computed(
  () => handlerOptions.value.find((o) => o.value === form.value.typeName) || null,
);
const selectedHandlerDesc = computed(() => selectedHandler.value?.description || '');

// typeName 字段 hint：内置锁定 > 处理器描述 > 默认说明
const typeNameFieldHint = computed(() => {
  if (isEdit.value && form.value.builtin) return t('task.builtinLocked');
  if (selectedHandlerDesc.value) return selectedHandlerDesc.value;
  return form.value.typeName ? t('task.handlerDescFallback') : t('task.typeNameHint');
});

// 填入示例载荷（一键回填，用户可再改）
function fillPayloadExample() {
  const example = selectedHandler.value?.payloadExample;
  if (!example) return;
  form.value.taskPayload = example;
  Notify.create({ type: 'positive', message: t('task.exampleFilled') });
}

// 新建时选中处理器自动预填示例载荷（仅当载荷为空，不覆盖用户已填内容；编辑态不动既有值）
watch(
  () => form.value.typeName,
  (val) => {
    if (isEdit.value || !val || form.value.taskPayload) return;
    const example = selectedHandler.value?.payloadExample;
    if (example) form.value.taskPayload = example;
  },
);

onMounted(async () => {
  try {
    const resp = await taskService.listHandlers();
    handlerOptions.value = (resp.items || [])
      .filter((i) => !!i.typeName)
      .map((i) => ({
        label: i.typeName as string,
        value: i.typeName as string,
        description: i.description || '',
        payloadDescription: i.payloadDescription || '',
        payloadExample: i.payloadExample || '',
      }));
  } catch {
    // error notification handled by axios interceptor
  }
});

// cron 五段式即时校验（轻量预检，最终以后端 robfig/cron 解析为准）
const CRON_FIELD_RE = /^[0-9A-Za-z*,\-/?#LW]+$/;
const cronRules = computed(() => [
  (val: string) => !!val || t('task.cronRequired'),
  (val: string) => {
    const fields = (val || '').trim().split(/\s+/);
    if (fields.length !== 5) return t('task.cronInvalid');
    return fields.every((f) => CRON_FIELD_RE.test(f)) || t('task.cronInvalid');
  },
]);

// cron 人性化描述（表格列与表单预览共用；locale 感知）
function cronDesc(spec?: string): string {
  if (!spec) return '';
  return describeCron(spec, locale.value.startsWith('zh') ? 'zh' : 'en');
}

function formatTick(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

// 表单 cron 预览：描述 + 下 3 次执行时间（非法表达式为空，由 rules 提示）
const formCronDesc = computed(() => cronDesc(form.value.cronSpec));
const formNextRuns = computed(() => {
  if (!form.value.cronSpec?.trim()) return [] as string[];
  return nextRuns(form.value.cronSpec, 3).map(formatTick);
});

// 常用 cron 规则预设（选择代替手填：点选自动填入，可再微调；v-model 不绑定，纯动作式）
const cronPresetOptions = computed(() => [
  { label: t('task.presetEveryMinute'), value: '* * * * *' },
  { label: t('task.presetEvery5Min'), value: '*/5 * * * *' },
  { label: t('task.presetEvery30Min'), value: '*/30 * * * *' },
  { label: t('task.presetHourly'), value: '0 * * * *' },
  { label: t('task.presetDaily0000'), value: '0 0 * * *' },
  { label: t('task.presetDaily0330'), value: '30 3 * * *' },
  { label: t('task.presetDaily0800'), value: '0 8 * * *' },
  { label: t('task.presetWeeklyMon0800'), value: '0 8 * * 1' },
  { label: t('task.presetMonthly1st0000'), value: '0 0 1 * *' },
]);

// 预设下拉展示值（选中即填入 cron 输入框后清空，保持动作式语义）
const cronPresetPicked = ref<string | null>(null);

function onApplyCronPreset(spec: string | null) {
  if (spec) form.value.cronSpec = spec;
  cronPresetPicked.value = null;
}

// payload 允许留空（提交时归一化为 '{}'），填写时必须是合法 JSON——后端列为 jsonb，空串/非法 JSON 会触发 500
const payloadRules = computed(() => [
  (val: string) => {
    const v = (val || '').trim();
    if (!v) return true;
    try {
      JSON.parse(v);
      return true;
    } catch {
      return t('task.payloadInvalid');
    }
  },
]);

const columns = computed<QTableColumn[]>(() => [
  { name: 'type', label: t('common.type'), field: 'type', align: 'left' },
  { name: 'name', label: t('task.name'), field: 'name', align: 'left', sortable: true },
  { name: 'taskPayload', label: t('common.payload'), field: 'taskPayload', align: 'left' },
  { name: 'cronSpec', label: t('common.cronSpec'), field: 'cronSpec', align: 'left' },
  { name: 'enable', label: t('common.enabled'), field: 'enable', align: 'center' },
  { name: 'lastExecution', label: t('task.lastExecution'), field: 'lastExecution', align: 'left' },
  { name: 'remark', label: t('common.remark'), field: 'remark', align: 'left' },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const searchFields = computed(() => [
  { name: 'name', label: t('task.name'), type: 'input' as const },
  { name: 'typeName', label: t('task.handler'), type: 'input' as const },
]);

function rowId(row: Record<string, unknown>): string {
  return String((row as unknown as TaskEntity).id ?? '');
}

function lastExec(row: Record<string, unknown>): TaskExecutionEntity | undefined {
  return (row as unknown as TaskEntity).lastExecution;
}

function execStatusColor(status?: string): string {
  if (status === 'ExecutionStatus_Success') return 'positive';
  if (status === 'ExecutionStatus_Failed') return 'negative';
  return 'grey';
}

function execStatusLabel(status?: string): string {
  if (status === 'ExecutionStatus_Success') return t('task.execSuccess');
  if (status === 'ExecutionStatus_Failed') return t('task.execFailed');
  return '-';
}

function formatDuration(ms?: number): string {
  if (ms == null) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function typeColor(type: string): string {
  switch (type) {
    case 'PERIODIC':
      return 'orange';
    case 'DELAY':
      return 'green';
    case 'WAIT_RESULT':
      return 'red';
    default:
      return 'grey';
  }
}

function typeLabel(type: string): string {
  switch (type) {
    case 'PERIODIC':
      return t('task.typePeriodic');
    case 'DELAY':
      return t('task.typeDelay');
    case 'WAIT_RESULT':
      return t('task.typeWaitResult');
    default:
      return type || '';
  }
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

function onCreate() {
  isEdit.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as TaskEntity;
  isEdit.value = true;
  form.value = {
    id: entity.id ? String(entity.id) : undefined,
    type: entity.type || 'PERIODIC',
    typeName: entity.typeName || '',
    name: entity.name || '',
    cronSpec: entity.cronSpec || '',
    taskPayload: entity.taskPayload || '',
    retryCount: entity.taskOptions?.retryCount ?? 0,
    timeoutSec: durationToSec(entity.taskOptions?.timeout),
    enable: entity.enable ?? true,
    remark: entity.remark || '',
    builtin: entity.builtin ?? false,
  };
  drawerOpen.value = true;
}

// protobuf Duration JSON（"300s"）↔ 秒；非法/空值按 0（不限制）处理
function durationToSec(v?: string): number {
  if (!v) return 0;
  const m = /^(\d+(?:\.\d+)?)s$/.exec(v.trim());
  return m ? Math.round(Number(m[1])) : 0;
}

async function onSubmit() {
  saving.value = true;
  try {
    const { id, builtin: _builtin, retryCount, timeoutSec, ...values } = form.value;
    // 空 payload 归一化为 '{}'：后端 task_payload 为 jsonb 列，空串会触发 22P02 500
    if (!values.taskPayload || !values.taskPayload.trim()) values.taskPayload = '{}';
    // task_options 覆盖语义：有值发送选项对象；编辑态全零发送空对象清零（nil=不触碰，无法清空）
    const options: TaskOptionEntity = {};
    if (retryCount > 0) options.retryCount = retryCount;
    if (timeoutSec > 0) options.timeout = `${timeoutSec}s`;
    const hasOptions = Object.keys(options).length > 0;
    if (isEdit.value && id) {
      await store.update(id, { ...values, taskOptions: hasOptions ? options : {} });
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await store.create(hasOptions ? { ...values, taskOptions: options } : values);
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
  const entity = row as unknown as TaskEntity;
  if (!entity.id) return;
  try {
    await store.remove(entity.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  }
}

// 状态列开关请求中行 id（防连点，空串=空闲；标杆：remediation RemediationPoliciesPage）
const togglingId = ref('');

async function onToggleEnabled(row: Record<string, unknown>, val: boolean) {
  const entity = row as unknown as TaskEntity;
  if (!entity.id || togglingId.value !== '') return;
  togglingId.value = String(entity.id);
  try {
    await store.update(entity.id, { enable: val });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  } finally {
    togglingId.value = '';
  }
}

// ===================== 立即执行 =====================
const runningId = ref('');

async function onRunNow(row: Record<string, unknown>) {
  const entity = row as unknown as TaskEntity;
  if (!entity.id) return;
  runningId.value = String(entity.id);
  try {
    await taskService.run(entity.id);
    Notify.create({ type: 'positive', message: t('task.runEnqueued') });
    // 入队异步执行，延迟刷新让执行记录落库后回显
    window.setTimeout(() => void crudRef.value?.fetchData(), 2000);
  } catch {
    // error notification handled by axios interceptor
  } finally {
    runningId.value = '';
  }
}

// ===================== 执行历史 =====================
const historyOpen = ref(false);
const historyLoading = ref(false);
const historyRows = ref<TaskExecutionEntity[]>([]);
const historyTaskName = ref('');

const historyColumns = computed<QTableColumn[]>(() => [
  { name: 'status', label: t('task.execStatus'), field: 'status', align: 'center' },
  { name: 'startedAt', label: t('task.execStartedAt'), field: 'startedAt', align: 'left' },
  { name: 'finishedAt', label: t('task.execFinishedAt'), field: 'finishedAt', align: 'left' },
  { name: 'durationMs', label: t('task.execDuration'), field: 'durationMs', align: 'right' },
  { name: 'error', label: t('task.execError'), field: 'error', align: 'left' },
]);

async function onShowHistory(row: Record<string, unknown>) {
  const entity = row as unknown as TaskEntity;
  if (!entity.id) return;
  historyTaskName.value = entity.typeName || '';
  historyRows.value = [];
  historyOpen.value = true;
  historyLoading.value = true;
  try {
    const resp = await taskService.listExecutions(entity.id);
    historyRows.value = resp.items || [];
  } catch {
    // error notification handled by axios interceptor
  } finally {
    historyLoading.value = false;
  }
}
</script>

<style scoped lang="sass">
// 表单分组标题：细分隔线 + 小标题，贴合 glass 主题 token
.form-group-title
  font-size: 13px
  font-weight: 600
  letter-spacing: 0.5px
  color: var(--color-text-heading)
  padding-top: 10px
  margin-top: 2px
  border-top: 1px solid var(--glass-border)

  &:first-child
    border-top: none
    padding-top: 0
    margin-top: 0

// 参数填写指引卡片：主题色淡底 + 细描边，规则/示例直接可见（替代不起眼的小灰字 hint）
.payload-guide
  border: 1px solid var(--glass-border)
  border-left: 3px solid var(--q-primary)
  border-radius: 6px
  background: color-mix(in srgb, var(--q-primary) 8%, transparent)
  padding: 8px 10px
  font-size: 12px
  line-height: 1.6
  color: var(--color-text-heading)
  display: flex
  flex-direction: column
  gap: 4px

  &--muted
    border-left-color: var(--glass-border)
    background: color-mix(in srgb, var(--color-text-heading) 5%, transparent)
    color: var(--color-text-caption, #9aa4b2)
    flex-direction: row
    align-items: center
    gap: 6px

  &__row
    display: flex
    align-items: center
    gap: 6px
    flex-wrap: wrap

  &__code
    font-family: 'JetBrains Mono', Consolas, monospace
    font-size: 12px
    background: color-mix(in srgb, var(--q-primary) 12%, transparent)
    border-radius: 4px
    padding: 1px 8px
    word-break: break-all
</style>
