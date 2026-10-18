<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <q-btn flat dense round icon="arrow_back" @click="goBack" />
      <div class="text-h5 q-ml-sm">变体矩阵</div>
      <code class="q-ml-sm text-subtitle2">{{ shortId(setId) }}</code>
      <q-badge v-if="set" class="q-ml-sm" :color="setStateMeta(set.state).color" outline>{{
        setStateMeta(set.state).label
      }}</q-badge>
      <q-badge v-if="set?.templateId" class="q-ml-xs" color="secondary" outline>模板 {{ set.templateId }}</q-badge>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchSet" />
      <q-btn
        outline
        dense
        no-caps
        color="warning"
        icon="difference"
        label="重算差异化"
        :loading="recalcing"
        @click="doRecalcDiff"
      />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="precision_manufacturing"
        label="批量渲染已选"
        :disable="selectedCount === 0"
        :loading="rendering"
        @click="doRender"
      />
    </div>

    <!-- 差异化告警（强提示不硬阻断） -->
    <q-banner v-if="set?.diffCheck?.warn" dense rounded class="bg-orange-1 text-orange-10 q-mb-md">
      <template #avatar><q-icon name="warning" /></template>
      差异化告警：最相似一对变体相似度 {{ (set.diffCheck.minPairSimilarity ?? 0).toFixed(2) }}（≥0.90
      平台查重风险高）。{{ set.diffCheck.summary }}
      <q-btn flat dense no-caps size="sm" label="查看相似对" @click="diffOpen = true" />
    </q-banner>

    <!-- 选区工具条 -->
    <div class="row items-center q-mb-sm q-gutter-sm">
      <div class="text-caption text-grey-8">
        已选 <b>{{ selectedCount }}</b> / {{ set?.variants?.length ?? 0 }} 个变体（只渲染 selected）
      </div>
      <q-badge v-if="selDirty" color="warning" outline>选区未保存</q-badge>
      <q-space />
      <q-btn flat dense no-caps size="sm" label="全选" @click="selectAll(true)" />
      <q-btn flat dense no-caps size="sm" label="全不选" @click="selectAll(false)" />
      <q-btn
        unelevated
        dense
        no-caps
        size="sm"
        color="primary"
        icon="save"
        label="保存选区"
        :disable="!selDirty"
        :loading="savingSel"
        @click="saveSelection"
      />
    </div>

    <div v-if="loading && !set" class="row justify-center q-pa-xl"><q-spinner-dots size="64px" color="primary" /></div>

    <!-- 网格：行 = 内容版本，列 = 画幅 × Hook -->
    <div v-else-if="set" class="mm-matrix">
      <table class="mm-table">
        <thead>
          <tr>
            <th class="mm-corner">内容版本 \ 画幅×Hook</th>
            <th v-for="c in columns" :key="c.key" class="text-center">
              <div>{{ c.aspect }}</div>
              <div class="text-caption text-grey-7">{{ c.hookLabel }}</div>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.contentVersion">
            <th class="text-left">
              <div>{{ row.label }}</div>
              <div class="text-caption text-grey-7">
                <code>{{ shortId(row.contentVersion) }}</code>
              </div>
            </th>
            <td v-for="c in columns" :key="c.key" class="text-center">
              <div
                v-if="cell(row.contentVersion, c.key)"
                class="mm-cell"
                :class="{ 'mm-cell-selected': cell(row.contentVersion, c.key)!.selected }"
              >
                <q-checkbox
                  dense
                  :model-value="cell(row.contentVersion, c.key)!.selected"
                  @update:model-value="toggleVariant(cell(row.contentVersion, c.key)!, $event)"
                />
                <div class="mm-cell-badges">
                  <q-badge dense outline :color="renderStateMeta(cell(row.contentVersion, c.key)!.renderState).color">
                    {{ renderStateMeta(cell(row.contentVersion, c.key)!.renderState).label }}
                  </q-badge>
                  <q-badge v-if="qcBlocked(cell(row.contentVersion, c.key)!.qc)" dense color="negative">QC✗</q-badge>
                  <q-badge v-else-if="cell(row.contentVersion, c.key)!.qc" dense color="positive">QC✓</q-badge>
                  <q-badge v-if="cell(row.contentVersion, c.key)!.copyDegraded" dense outline color="orange-9"
                    >文案降级</q-badge
                  >
                </div>
                <div class="text-caption text-grey-7">
                  {{ fmtDuration(cell(row.contentVersion, c.key)!.durationSec) }}
                </div>
                <q-tooltip v-if="cell(row.contentVersion, c.key)!.renderError">{{
                  cell(row.contentVersion, c.key)!.renderError
                }}</q-tooltip>
              </div>
              <div v-else class="text-grey-5">—</div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 渲染进度卡 -->
    <q-card v-if="progress" flat bordered class="q-mt-md">
      <q-card-section>
        <div class="row items-center q-mb-sm">
          <div class="text-subtitle1">渲染进度</div>
          <q-space />
          <div class="text-caption text-grey-7">ETA：{{ fmtEta(progress.etaSec) }}</div>
        </div>
        <q-linear-progress
          :value="Math.max(0, Math.min(1, progress.overallProgress ?? 0))"
          size="14px"
          color="primary"
          rounded
          class="q-mb-sm"
        >
          <div class="absolute-full flex flex-center">
            <q-badge
              color="white"
              text-color="primary"
              :label="`${Math.round((progress.overallProgress ?? 0) * 100)}%`"
            />
          </div>
        </q-linear-progress>
        <div class="row q-gutter-sm text-caption">
          <q-chip dense outline color="grey">总数 {{ progress.total ?? 0 }}</q-chip>
          <q-chip dense outline color="warning">排队 {{ progress.queued ?? 0 }}</q-chip>
          <q-chip dense outline color="info">运行 {{ progress.running ?? 0 }}</q-chip>
          <q-chip dense outline color="positive">完成 {{ progress.done ?? 0 }}</q-chip>
          <q-chip dense outline color="negative">失败 {{ progress.failed ?? 0 }}</q-chip>
          <q-chip dense outline color="grey">取消 {{ progress.cancelled ?? 0 }}</q-chip>
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            icon="list"
            label="渲染队列明细"
            @click="router.push('/media/mme/render')"
          />
        </div>
      </q-card-section>
    </q-card>

    <div class="row q-mt-md q-gutter-sm">
      <q-btn
        unelevated
        no-caps
        color="secondary"
        icon="rate_review"
        label="前往审核台"
        @click="router.push(`/media/mme/variant-sets/${setId}/review`)"
      />
    </div>

    <!-- 相似对明细对话框 -->
    <q-dialog v-model="diffOpen">
      <q-card style="min-width: 560px; max-width: 80vw">
        <q-card-section class="text-h6">相似变体对（≥0.90 告警）</q-card-section>
        <q-card-section>
          <q-markup-table v-if="(set?.diffCheck?.pairs?.length ?? 0) > 0" flat bordered dense>
            <thead>
              <tr>
                <th>变体 A</th>
                <th>变体 B</th>
                <th>视觉</th>
                <th>Hook</th>
                <th>文案</th>
                <th>综合</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="(p, i) in set?.diffCheck?.pairs || []"
                :key="i"
                :class="(p.overall ?? 0) >= 0.9 ? 'bg-orange-1' : ''"
              >
                <td>
                  <code>{{ shortId(p.variantA) }}</code>
                </td>
                <td>
                  <code>{{ shortId(p.variantB) }}</code>
                </td>
                <td>{{ (p.visual ?? 0).toFixed(2) }}</td>
                <td>{{ (p.hook ?? 0).toFixed(2) }}</td>
                <td>{{ (p.copy ?? 0).toFixed(2) }}</td>
                <td>
                  <b>{{ (p.overall ?? 0).toFixed(2) }}</b>
                </td>
              </tr>
            </tbody>
          </q-markup-table>
          <div v-else class="text-caption text-grey-7">
            尚无相似对数据（未渲染的变体无视觉维度；文案维度基于 SimHash）。
          </div>
          <div class="text-caption text-grey-7 q-mt-sm">
            差异化检查为强提示、不硬阻断；审核台可正常批准，平台查重风险由审核人权衡。
          </div>
        </q-card-section>
        <q-card-actions align="right"><q-btn v-close-popup flat no-caps label="关闭" /></q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { mmeRenderService, mmeVariantService } from '../../../services/index';
import {
  errMessage,
  fmtDuration,
  fmtEta,
  qcBlocked,
  renderStateMeta,
  setStateMeta,
  shortId,
  type RenderResponse,
  type SetRenderProgress,
  type VariantItem,
  type VariantSetView,
} from './types';

const route = useRoute();
const router = useRouter();
const setId = String(route.params.id ?? '');

const set = ref<VariantSetView | null>(null);
const loading = ref(false);
const savingSel = ref(false);
const recalcing = ref(false);
const rendering = ref(false);
const selDirty = ref(false);
const diffOpen = ref(false);
const progress = ref<SetRenderProgress | null>(null);

let pollTimer: ReturnType<typeof setInterval> | null = null;

const columns = computed(() => {
  const out: { key: string; aspect: string; hookLabel: string }[] = [];
  for (const v of set.value?.variants ?? []) {
    const key = `${v.aspect}|${v.hookId}`;
    if (!out.some((c) => c.key === key)) {
      out.push({ key, aspect: v.aspect ?? '?', hookLabel: v.hookId ?? '-' });
    }
  }
  return out;
});

const rows = computed(() => {
  const out: { contentVersion: string; label: string }[] = [];
  for (const v of set.value?.variants ?? []) {
    if (!out.some((r) => r.contentVersion === v.contentVersion)) {
      out.push({ contentVersion: v.contentVersion ?? '', label: v.contentVersion || '-' });
    }
  }
  return out;
});

const cellIndex = computed(() => {
  const m = new Map<string, VariantItem>();
  for (const v of set.value?.variants ?? []) {
    m.set(`${v.contentVersion}|${v.aspect}|${v.hookId}`, v);
  }
  return m;
});

function cell(contentVersion: string, colKey: string): VariantItem | undefined {
  const [aspect, hookId] = colKey.split('|');
  return cellIndex.value.get(`${contentVersion}|${aspect}|${hookId}`);
}

const selectedCount = computed(() => (set.value?.variants ?? []).filter((v) => v.selected).length);

function goBack() {
  if (set.value?.taskId) router.push(`/media/mme/tasks/${set.value.taskId}`);
  else router.push('/media/mme/tasks');
}

async function fetchSet() {
  loading.value = true;
  try {
    set.value = await mmeVariantService.getSet(setId);
    selDirty.value = false;
    await fetchProgress();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

async function fetchProgress() {
  try {
    const p = await mmeRenderService.getSetProgress(setId);
    progress.value = p.total ? p : null;
  } catch {
    // 无渲染任务时后端返回空进度/404 属正常，不打扰
    progress.value = null;
  }
}

function toggleVariant(v: VariantItem, selected: boolean) {
  v.selected = !!selected;
  selDirty.value = true;
}

function selectAll(selected: boolean) {
  for (const v of set.value?.variants ?? []) v.selected = selected;
  selDirty.value = true;
}

async function saveSelection() {
  savingSel.value = true;
  try {
    const ids = (set.value?.variants ?? []).filter((v) => v.selected).map((v) => v.id ?? '');
    set.value = await mmeVariantService.updateSelection(setId, ids);
    selDirty.value = false;
    Notify.create({ type: 'positive', message: `选区已保存（${selectedCount.value} 个变体）` });
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    savingSel.value = false;
  }
}

async function doRender() {
  if (selDirty.value) {
    Notify.create({ type: 'warning', message: '选区未保存，请先「保存选区」再渲染' });
    return;
  }
  rendering.value = true;
  try {
    const r: RenderResponse = await mmeVariantService.render(setId);
    Notify.create({
      type: 'positive',
      message: `已入队 ${r.queued ?? 0} 个渲染任务（跳过 ${r.skipped ?? 0}，已完成 ${r.alreadyDone ?? 0}）`,
    });
    await fetchSet();
    startPolling();
  } catch (e) {
    // MME_QUEUE_UNAVAILABLE 等如实上屏（不伪造入队）
    Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
  } finally {
    rendering.value = false;
  }
}

async function doRecalcDiff() {
  recalcing.value = true;
  try {
    const d = await mmeVariantService.recalcDiffCheck(setId);
    Notify.create({
      type: d.warn ? 'warning' : 'positive',
      message: d.warn ? `差异化告警：最小相似度 ${(d.minPairSimilarity ?? 0).toFixed(2)}` : '差异化检查通过',
    });
    await fetchSet();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    recalcing.value = false;
  }
}

function startPolling() {
  stopPolling();
  pollTimer = setInterval(async () => {
    await fetchProgress();
    const p = progress.value;
    if (p && (p.queued ?? 0) === 0 && (p.running ?? 0) === 0) {
      stopPolling();
      await fetchSet();
    }
  }, 3000);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

onMounted(async () => {
  await fetchSet();
  const p = progress.value;
  if (p && ((p.queued ?? 0) > 0 || (p.running ?? 0) > 0)) startPolling();
});
onBeforeUnmount(stopPolling);
</script>

<style scoped>
.mm-matrix {
  overflow-x: auto;
}

.mm-table {
  border-collapse: collapse;
  min-width: 640px;
  width: 100%;
}

.mm-table th,
.mm-table td {
  border: 1px solid rgb(0 0 0 / 12%);
  padding: 8px;
  vertical-align: top;
}

.mm-corner {
  min-width: 160px;
}

.mm-cell {
  border-radius: 4px;
  padding: 4px;
  min-width: 110px;
}

.mm-cell-selected {
  background: rgb(25 118 210 / 8%);
}

.mm-cell-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  justify-content: center;
}
</style>
