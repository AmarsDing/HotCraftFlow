<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MME 渲染队列</div>
      <q-space />
      <q-toggle v-model="autoRefresh" label="自动刷新（3s）" dense class="q-mr-md" />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <div class="row q-col-gutter-sm q-mb-sm">
      <div class="col-auto">
        <q-select
          v-model="stateFilter"
          dense
          outlined
          emit-value
          map-options
          :options="stateOptions"
          label="状态筛选"
          style="min-width: 140px"
          @update:model-value="onSearch"
        />
      </div>
      <div class="col" style="max-width: 360px">
        <q-input
          v-model="variantFilter"
          dense
          outlined
          clearable
          placeholder="按 variant_id 过滤"
          @keyup.enter="onSearch"
          @clear="onSearch"
        >
          <template #append><q-icon name="search" class="cursor-pointer" @click="onSearch" /></template>
        </q-input>
      </div>
    </div>

    <q-table flat bordered :rows="items" :columns="columns" row-key="id" :loading="loading" hide-bottom>
      <template #body-cell-state="props">
        <q-td :props="props">
          <q-badge :color="renderStateMeta(props.row.state).color" outline>
            <q-icon :name="renderStateMeta(props.row.state).icon" size="14px" class="q-mr-xs" />
            {{ renderStateMeta(props.row.state).label }}
          </q-badge>
        </q-td>
      </template>
      <template #body-cell-progress="props">
        <q-td :props="props" style="min-width: 160px">
          <q-linear-progress
            :value="Math.max(0, Math.min(1, props.row.progress ?? 0))"
            size="10px"
            rounded
            :color="props.row.state === 'failed' ? 'negative' : props.row.state === 'done' ? 'positive' : 'primary'"
          />
          <div class="text-caption text-grey-7">
            {{ Math.round((props.row.progress ?? 0) * 100) }}%<template v-if="(props.row.etaSec ?? -1) >= 0">
              · ETA {{ fmtEta(props.row.etaSec) }}</template
            >
          </div>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn
            v-if="props.row.state === 'failed'"
            flat
            dense
            round
            color="primary"
            icon="replay"
            :loading="actingId === props.row.id"
            @click="doRetry(props.row)"
            ><q-tooltip>重试（attempt+1；保留上次 intermediate 供排查）</q-tooltip></q-btn
          >
          <q-btn
            v-if="props.row.state === 'queued' || props.row.state === 'running'"
            flat
            dense
            round
            color="warning"
            icon="cancel"
            :loading="actingId === props.row.id"
            @click="doCancel(props.row)"
            ><q-tooltip>取消</q-tooltip></q-btn
          >
          <q-btn
            v-if="props.row.errorMsg || props.row.intermediatePath"
            flat
            dense
            round
            :icon="isExpanded(props.row.id ?? '') ? 'expand_less' : 'expand_more'"
            @click="toggleExpand(props.row.id ?? '')"
            ><q-tooltip>错误明细 / 中间产物</q-tooltip></q-btn
          >
        </q-td>
      </template>
      <template #no-data>
        <div class="row justify-center q-pa-lg text-grey-7">
          <div class="text-center">
            <q-icon name="precision_manufacturing" size="48px" class="q-mb-sm" />
            <div>渲染队列为空</div>
          </div>
        </div>
      </template>
    </q-table>

    <!-- 错误展开行（独立渲染，避免 q-table expand slot 与列定制交叠） -->
    <div v-for="row in expandedRows" :key="'err-' + row.id">
      <q-banner v-if="row.errorMsg || row.intermediatePath" dense rounded class="bg-grey-2 q-my-xs">
        <div class="text-caption">
          <div>
            <b>{{ shortId(row.id) }}</b
            ><template v-if="row.errorCode">
              · <code>{{ row.errorCode }}</code></template
            >
          </div>
          <div v-if="row.errorMsg" class="text-negative" style="white-space: pre-wrap">{{ row.errorMsg }}</div>
          <div v-if="row.intermediatePath" class="text-grey-8">
            中间产物：<code>{{ row.intermediatePath }}</code
            >（保留供 ffmpeg 排查）
          </div>
        </div>
      </q-banner>
    </div>

    <div class="row justify-center q-mt-md">
      <q-pagination v-model="page" :max="maxPage" direction-links boundary-numbers @update:model-value="fetchData" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Dialog, Notify } from 'quasar';
import { mmeRenderService } from '../../../services/index';
import { errMessage, fmtEta, fmtTime, renderStateMeta, shortId, type RenderJob } from './types';

const items = ref<RenderJob[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 20;
const loading = ref(false);
const actingId = ref('');
const stateFilter = ref('');
const variantFilter = ref('');
const autoRefresh = ref(true);
const expandedKeys = ref<string[]>([]);

const maxPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));
const expandedRows = computed(() => items.value.filter((r) => expandedKeys.value.includes(r.id ?? '')));

const stateOptions = [
  { label: '全部状态', value: '' },
  { label: '排队中', value: 'queued' },
  { label: '运行中', value: 'running' },
  { label: '完成', value: 'done' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'cancelled' },
];

const columns = [
  { name: 'id', label: 'Job', field: (r: RenderJob) => shortId(r.id), align: 'left' as const },
  { name: 'variantId', label: '变体', field: (r: RenderJob) => shortId(r.variantId), align: 'left' as const },
  { name: 'aspect', label: '画幅', field: (r: RenderJob) => r.aspect || '-', align: 'left' as const },
  { name: 'state', label: '状态', field: 'state', align: 'left' as const },
  { name: 'progress', label: '进度', field: 'progress', align: 'left' as const },
  { name: 'attempt', label: '尝试', field: (r: RenderJob) => r.attempt ?? 0, align: 'center' as const },
  { name: 'workerId', label: 'Worker', field: (r: RenderJob) => r.workerId || '-', align: 'left' as const },
  { name: 'startedAt', label: '开始', field: (r: RenderJob) => fmtTime(r.startedAt), align: 'left' as const },
  { name: 'finishedAt', label: '结束', field: (r: RenderJob) => fmtTime(r.finishedAt), align: 'left' as const },
  { name: 'actions', label: '操作', field: 'id', align: 'right' as const },
];

async function fetchData() {
  loading.value = true;
  try {
    const formValues: Record<string, unknown> = {};
    if (stateFilter.value) formValues.state = stateFilter.value;
    if (variantFilter.value.trim()) formValues.variant_id = variantFilter.value.trim();
    const res = await mmeRenderService.listJobs({ page: page.value, pageSize, formValues });
    items.value = res.items ?? [];
    total.value = res.total ?? 0;
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

function onSearch() {
  page.value = 1;
  fetchData();
}

function isExpanded(id: string) {
  return expandedKeys.value.includes(id);
}

function toggleExpand(id: string) {
  if (isExpanded(id)) expandedKeys.value = expandedKeys.value.filter((x) => x !== id);
  else expandedKeys.value.push(id);
}

async function doRetry(row: RenderJob) {
  actingId.value = row.id ?? '';
  try {
    await mmeRenderService.retryJob(row.id ?? '');
    Notify.create({ type: 'positive', message: '已重新入队' });
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    actingId.value = '';
  }
}

function doCancel(row: RenderJob) {
  Dialog.create({
    title: '取消渲染任务',
    message: `确认取消 ${shortId(row.id)}？运行中的任务将尽快终止，不产生产物。`,
    cancel: { flat: true, noCaps: true, label: '再想想' },
    ok: { unelevated: true, noCaps: true, color: 'warning', label: '取消任务' },
  }).onOk(async () => {
    actingId.value = row.id ?? '';
    try {
      await mmeRenderService.cancelJob(row.id ?? '');
      Notify.create({ type: 'positive', message: '已取消' });
      await fetchData();
    } catch (e) {
      Notify.create({ type: 'negative', message: errMessage(e) });
    } finally {
      actingId.value = '';
    }
  });
}

let timer: ReturnType<typeof setInterval> | null = null;

function applyTimer() {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  if (autoRefresh.value) {
    timer = setInterval(() => {
      if (!loading.value) fetchData();
    }, 3000);
  }
}

watch(autoRefresh, applyTimer);
onMounted(() => {
  fetchData();
  applyTimer();
});
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});
</script>
