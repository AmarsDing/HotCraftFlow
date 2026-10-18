<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <q-btn flat dense round icon="arrow_back" @click="router.push(`/media/mme/variant-sets/${setId}`)" />
      <div class="text-h5 q-ml-sm">审核台</div>
      <code class="q-ml-sm text-subtitle2">{{ shortId(setId) }}</code>
      <q-badge v-if="set" class="q-ml-sm" :color="setStateMeta(set.state).color" outline>{{
        setStateMeta(set.state).label
      }}</q-badge>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchSet" />
      <q-btn
        unelevated
        dense
        no-caps
        color="positive"
        icon="check_circle"
        label="批准所选"
        :disable="approveIds.length === 0"
        :loading="submitting"
        @click="confirmReview"
      />
      <q-btn
        unelevated
        dense
        no-caps
        color="secondary"
        icon="send"
        label="送 MAP（Dispatch）"
        :disable="!canDispatch"
        :loading="dispatching"
        @click="confirmDispatch"
      >
        <q-tooltip v-if="!canDispatch">须先完成批准（state=approved）才可送 MAP</q-tooltip>
      </q-btn>
    </div>

    <q-banner dense rounded class="bg-blue-1 text-blue-10 q-mb-md">
      <template #avatar><q-icon name="gavel" /></template>
      QC 硬门禁：boundary / subtitle 任一未过的变体<b>禁止批准</b>（后端同源强制）；驳回<b>必须填写原因</b>；审核与
      Dispatch 是两个独立操作。
    </q-banner>
    <q-banner v-if="set?.diffCheck?.warn" dense rounded class="bg-orange-1 text-orange-10 q-mb-md">
      <template #avatar><q-icon name="warning" /></template>
      差异化告警：最小相似度
      {{ (set.diffCheck.minPairSimilarity ?? 0).toFixed(2) }}，平台查重风险由审核人权衡（不硬阻断）。
    </q-banner>

    <div class="row items-center q-mb-sm q-gutter-sm text-caption text-grey-8">
      <span
        >已勾选批准 <b>{{ approveIds.length }}</b> 个；已填驳回 <b>{{ rejectCount }}</b> 个</span
      >
      <q-space />
      <q-btn flat dense no-caps size="sm" label="勾选全部 QC 合格" @click="pickAllQcOk" />
      <q-btn flat dense no-caps size="sm" label="清空勾选" @click="approveIds = []" />
    </div>

    <div v-if="loading && !set" class="row justify-center q-pa-xl"><q-spinner-dots size="64px" color="primary" /></div>

    <div v-else class="row q-col-gutter-md">
      <div v-for="v in set?.variants ?? []" :key="v.id" class="col-12 col-sm-6 col-md-4 col-lg-3">
        <q-card flat bordered :class="{ 'mm-reject-card': !!rejections[v.id ?? ''] }">
          <!-- 预览区：懒加载 blob（<video> 无法带 Authorization 头） -->
          <div class="mm-preview bg-black flex flex-center">
            <video
              v-if="previewUrls[v.id ?? '']"
              :src="previewUrls[v.id ?? '']"
              controls
              preload="metadata"
              class="mm-video"
            />
            <div v-else class="text-center text-grey-5">
              <q-icon name="smart_display" size="40px" />
              <div v-if="v.renderState === 'done' && v.videoPath">
                <q-btn
                  flat
                  dense
                  no-caps
                  color="white"
                  icon="play_circle"
                  label="加载预览"
                  :loading="previewLoading === v.id"
                  @click="loadPreview(v)"
                />
              </div>
              <div v-else class="text-caption q-px-md">
                {{ v.renderState === 'done' ? '无视频路径' : renderStateMeta(v.renderState).label }}
              </div>
            </div>
          </div>

          <q-card-section class="q-py-sm">
            <div class="row items-center q-gutter-xs">
              <q-badge dense outline color="primary">{{ v.aspect }}</q-badge>
              <q-badge dense outline color="secondary">{{ v.hookId || '-' }}</q-badge>
              <q-badge dense outline color="grey">{{ fmtDuration(v.durationSec) }}</q-badge>
              <q-space />
              <code class="text-caption">{{ shortId(v.id) }}</code>
            </div>
            <div class="row q-mt-xs q-gutter-xs">
              <template v-if="v.qc">
                <q-badge dense :color="v.qc.boundary_ok === false ? 'negative' : 'positive'" outline>
                  边界{{ v.qc.boundary_ok === false ? '✗' : '✓' }}
                </q-badge>
                <q-badge dense :color="v.qc.subtitle_ok === false ? 'negative' : 'positive'" outline>
                  字幕{{ v.qc.subtitle_ok === false ? '✗' : '✓' }}
                </q-badge>
                <q-badge dense outline color="grey">QC {{ ((v.qc.score ?? 0) * 100).toFixed(0) }}分</q-badge>
              </template>
              <q-badge v-else dense outline color="grey">未渲染，无 QC</q-badge>
              <q-badge v-if="v.copyDegraded" dense outline color="orange-9">文案降级</q-badge>
            </div>
            <div v-if="v.renderError" class="text-negative text-caption q-mt-xs ellipsis-2-lines">
              {{ v.renderError }}
            </div>
            <div v-if="v.copy" class="text-caption text-grey-8 q-mt-xs">
              <div v-for="(c, platform) in v.copy" :key="platform" class="ellipsis">
                <b>{{ platform }}</b
                >：{{ c.title || '（无标题）' }}
              </div>
            </div>
          </q-card-section>

          <q-separator />
          <q-card-actions align="between">
            <q-checkbox
              dense
              :model-value="approveIds.includes(v.id ?? '')"
              :disable="qcBlocked(v.qc) || !!rejections[v.id ?? '']"
              label="批准"
              @update:model-value="toggleApprove(v, $event)"
            >
              <q-tooltip v-if="qcBlocked(v.qc)">🔴 QC 硬门禁：未过边界/字幕检查，禁止批准</q-tooltip>
            </q-checkbox>
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              color="negative"
              icon="close"
              label="驳回"
              :disable="approveIds.includes(v.id ?? '') || v.renderState !== 'done'"
              @click="openReject(v)"
            />
          </q-card-actions>
          <q-banner v-if="rejections[v.id ?? '']" dense class="bg-red-1 text-negative text-caption q-pa-xs">
            <q-icon name="gpp_bad" class="q-mr-xs" />驳回：{{ rejections[v.id ?? ''] }}
            <q-btn flat dense round size="xs" icon="undo" @click="removeRejection(v)" />
          </q-banner>
        </q-card>
      </div>
    </div>

    <!-- 驳回对话框（🔴 reason 必填） -->
    <q-dialog v-model="rejectOpen" persistent>
      <q-card style="min-width: 420px">
        <q-card-section class="text-h6">驳回 {{ shortId(rejectTarget?.id) }}</q-card-section>
        <q-card-section>
          <q-input
            v-model="rejectReason"
            type="textarea"
            outlined
            autofocus
            label="驳回原因（必填，记入审核结果）"
            :rules="[(v: string) => (v && v.trim().length > 0) || '驳回必须填写原因']"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="rejectOpen = false" />
          <q-btn
            unelevated
            no-caps
            color="negative"
            label="填入驳回"
            :disable="!rejectReason.trim()"
            @click="fillRejection"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Dispatch 结果 -->
    <q-dialog v-model="dispatchOpen">
      <q-card style="min-width: 480px">
        <q-card-section class="text-h6">已送 MAP</q-card-section>
        <q-card-section class="text-caption">
          <div>
            契约 VariantSet：<code>{{ shortId(dispatchResult?.variant_set_id) }}</code>
          </div>
          <div>变体数：{{ dispatchResult?.variants?.length ?? 0 }}</div>
          <div class="text-grey-7 q-mt-xs">
            事件 <code>mm.mme.variant.ready.v1</code> 已发布到 NATS，MAP 侧据此建分发单元。
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps label="关闭" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            label="前往 MAP 审批队列"
            @click="router.push('/media/map/approvals')"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Dialog, Notify } from 'quasar';
import { mmeVariantService } from '../../../services/index';
import {
  errMessage,
  fetchMmeMediaUrl,
  fmtDuration,
  qcBlocked,
  renderStateMeta,
  setStateMeta,
  shortId,
  type MmeContractVariantSet,
  type VariantItem,
  type VariantSetView,
} from './types';

const route = useRoute();
const router = useRouter();
const setId = String(route.params.id ?? '');

const set = ref<VariantSetView | null>(null);
const loading = ref(false);
const submitting = ref(false);
const dispatching = ref(false);
const approveIds = ref<string[]>([]);
const rejections = reactive<Record<string, string>>({});
const previewUrls = reactive<Record<string, string>>({});
const previewLoading = ref('');

const rejectOpen = ref(false);
const rejectTarget = ref<VariantItem | null>(null);
const rejectReason = ref('');

const dispatchOpen = ref(false);
const dispatchResult = ref<MmeContractVariantSet | null>(null);

const rejectCount = computed(() => Object.keys(rejections).length);
const canDispatch = computed(() => set.value?.state === 'approved');

async function fetchSet() {
  loading.value = true;
  try {
    set.value = await mmeVariantService.getSet(setId);
    approveIds.value = [];
    for (const k of Object.keys(rejections)) delete rejections[k];
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

async function loadPreview(v: VariantItem) {
  previewLoading.value = v.id ?? '';
  try {
    previewUrls[v.id ?? ''] = await fetchMmeMediaUrl(v.videoPath ?? '');
  } catch (e) {
    Notify.create({ type: 'negative', message: `预览加载失败：${errMessage(e)}` });
  } finally {
    previewLoading.value = '';
  }
}

function toggleApprove(v: VariantItem, checked: boolean) {
  const id = v.id ?? '';
  if (!id) return;
  if (checked) {
    if (!approveIds.value.includes(id)) approveIds.value.push(id);
  } else {
    approveIds.value = approveIds.value.filter((x) => x !== id);
  }
}

function pickAllQcOk() {
  // 🔴 QC 硬门禁同步：只勾选 qc 双方均非 false 的已渲染变体
  approveIds.value = (set.value?.variants ?? [])
    .filter(
      (v) =>
        v.renderState === 'done' &&
        v.qc &&
        v.qc.boundary_ok !== false &&
        v.qc.subtitle_ok !== false &&
        !rejections[v.id ?? ''],
    )
    .map((v) => v.id ?? '');
}

function openReject(v: VariantItem) {
  rejectTarget.value = v;
  rejectReason.value = rejections[v.id ?? ''] || '';
  rejectOpen.value = true;
}

function fillRejection() {
  const id = rejectTarget.value?.id ?? '';
  if (!id) return;
  rejections[id] = rejectReason.value.trim();
  rejectOpen.value = false;
}

function removeRejection(v: VariantItem) {
  delete rejections[v.id ?? ''];
}

function confirmReview() {
  Dialog.create({
    title: '批量审核确认',
    html: true,
    message: `批准 <b>${approveIds.value.length}</b> 个变体；驳回 <b>${rejectCount.value}</b> 个（均附原因）。确认提交？`,
    cancel: { flat: true, noCaps: true, label: '取消' },
    ok: { unelevated: true, noCaps: true, color: 'positive', label: '提交审核' },
  }).onOk(async () => {
    submitting.value = true;
    try {
      const rej = Object.entries(rejections).map(([variantId, reason]) => ({ variantId, reason }));
      set.value = await mmeVariantService.review(setId, approveIds.value, rej);
      Notify.create({ type: 'positive', message: `审核完成，当前状态：${setStateMeta(set.value.state).label}` });
      approveIds.value = [];
      for (const k of Object.keys(rejections)) delete rejections[k];
    } catch (e) {
      // QC 硬门禁等后端强制 → 原因如实上屏
      Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
      await fetchSet();
    } finally {
      submitting.value = false;
    }
  });
}

function confirmDispatch() {
  Dialog.create({
    title: '送 MAP 确认',
    html: true,
    message: `将产出契约 VariantSet 并发布 <code>mm.mme.variant.ready.v1</code> 事件。送 MAP 后由 MAP 侧另走「审批 → 发布」流程（与 MME 审核相互独立）。确认？`,
    cancel: { flat: true, noCaps: true, label: '取消' },
    ok: { unelevated: true, noCaps: true, color: 'secondary', label: '送 MAP' },
  }).onOk(async () => {
    dispatching.value = true;
    try {
      dispatchResult.value = await mmeVariantService.dispatch(setId);
      dispatchOpen.value = true;
    } catch (e) {
      Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
    } finally {
      dispatching.value = false;
    }
  });
}

onMounted(fetchSet);
onBeforeUnmount(() => {
  for (const url of Object.values(previewUrls)) URL.revokeObjectURL(url);
});
</script>

<style scoped>
.mm-preview {
  aspect-ratio: 16 / 9;
  min-height: 140px;
}

.mm-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.mm-reject-card {
  border-color: var(--q-negative);
}
</style>
