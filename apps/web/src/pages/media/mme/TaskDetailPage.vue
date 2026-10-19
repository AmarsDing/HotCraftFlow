<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <q-btn flat dense round icon="arrow_back" @click="router.push('/media/mme/tasks')" />
      <div class="text-h5 q-ml-sm">{{ task?.title || '任务详情' }}</div>
      <q-badge v-if="task" class="q-ml-sm" :color="taskStateMeta(task.state).color" outline>
        <q-icon :name="taskStateMeta(task.state).icon" size="14px" class="q-mr-xs" />
        {{ taskStateMeta(task.state).label }}
      </q-badge>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchAll" />
    </div>

    <q-banner v-if="task?.errorMsg" dense rounded class="bg-red-1 text-negative q-mb-md">
      <template #avatar><q-icon name="error" /></template>
      {{ task.errorMsg }}
    </q-banner>

    <!-- 素材信息卡 -->
    <q-card flat bordered class="q-mb-md">
      <q-card-section>
        <div class="text-subtitle1 q-mb-sm">素材信息</div>
        <div v-if="task?.asset" class="row q-col-gutter-md text-caption">
          <div class="col-auto">
            Asset：<code>{{ shortId(task.asset.assetId) }}</code>
          </div>
          <div class="col-auto">
            时长：<b>{{ fmtDuration(task.asset.durationSec) }}</b>
          </div>
          <div class="col-auto">
            分辨率：<b>{{ task.asset.width || '?' }}×{{ task.asset.height || '?' }}</b>
          </div>
          <div class="col-auto">
            大小：<b>{{ fmtBytes(task.asset.sizeBytes) }}</b>
          </div>
          <div class="col-auto">
            引用计数：<b>{{ task.asset.refCount ?? 0 }}</b>
          </div>
          <div class="col-auto">
            指纹：<code>{{ shortId(task.asset.fingerprint) }}</code>
          </div>
        </div>
        <div v-else class="text-caption text-grey-7">
          {{ task?.assetId ? '素材已入库（详情加载中/不可见）' : '素材未就绪：URL 任务等待下载，本地任务等待入库。' }}
        </div>
        <div class="row q-mt-sm text-caption text-grey-8 q-gutter-md">
          <span
            >来源：<code>{{ task?.sourceType }}://{{ task?.sourceRef }}</code></span
          >
          <span>创建：{{ fmtTime(task?.createdAt) }}</span>
          <span v-if="task?.asrEngine">ASR：{{ task.asrEngine }}</span>
          <span v-if="task?.promptVersion">Prompt：{{ task.promptVersion }}</span>
        </div>
        <div v-if="task?.userIntent" class="row q-mt-xs text-caption text-grey-8">意图：{{ task.userIntent }}</div>
      </q-card-section>
      <q-separator />
      <q-card-actions>
        <q-btn
          outline
          dense
          no-caps
          color="primary"
          icon="psychology"
          label="触发分析（ASR→高光）"
          :loading="analyzing"
          @click="doAnalyze"
        />
        <q-tooltip
          >缺 whisper/Ollama 时如实返回 MME_ASR_FAIL / MME_LLM_UNAVAILABLE；无 AI 时可全程使用下方手工 EDL</q-tooltip
        >
      </q-card-actions>
    </q-card>

    <!-- 手工 EDL 高光编辑 -->
    <q-card flat bordered class="q-mb-md">
      <q-card-section>
        <div class="row items-center q-mb-sm">
          <div class="text-subtitle1">内容版本（高光候选 · 手工 EDL）</div>
          <q-badge v-if="dirty" class="q-ml-sm" color="warning" outline>未保存</q-badge>
          <q-space />
          <q-btn flat dense no-caps icon="add" label="添加一条" @click="addHighlight" />
          <q-btn
            unelevated
            dense
            no-caps
            color="primary"
            icon="save"
            label="保存 EDL"
            :disable="!dirty || !highlightsValid"
            :loading="savingHl"
            @click="saveHighlights"
          />
        </div>
        <q-markup-table v-if="highlights.length > 0" flat bordered separator="cell">
          <thead>
            <tr>
              <th class="text-left">标签</th>
              <th class="text-left" style="width: 110px">开始(秒)</th>
              <th class="text-left" style="width: 110px">结束(秒)</th>
              <th class="text-left">备注</th>
              <th class="text-left" style="width: 90px">来源</th>
              <th class="text-right" style="width: 60px"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(h, idx) in highlights" :key="idx">
              <td><q-input v-model="h.label" dense borderless placeholder="如：开场钩子" /></td>
              <td><q-input v-model.number="h.startSec" dense borderless type="number" min="0" step="0.1" /></td>
              <td><q-input v-model.number="h.endSec" dense borderless type="number" min="0" step="0.1" /></td>
              <td><q-input v-model="h.note" dense borderless placeholder="可选" /></td>
              <td>
                <q-badge dense outline :color="h.origin === 'auto' ? 'secondary' : 'primary'">{{
                  h.origin || 'manual'
                }}</q-badge>
              </td>
              <td class="text-right">
                <q-btn flat dense round color="negative" icon="close" @click="removeHighlight(idx)" />
              </td>
            </tr>
            <tr v-for="(msg, i) in highlightsErrors" v-show="msg" :key="'e' + i">
              <td colspan="6" class="text-negative text-caption">{{ msg }}</td>
            </tr>
          </tbody>
        </q-markup-table>
        <div v-else class="text-caption text-grey-7">
          暂无内容版本。点击「添加一条」手工录入高光区间（无 AI 时的内容版本来源），或先「触发分析」由 ASR+LLM 产出。
        </div>
      </q-card-section>
    </q-card>

    <!-- 三维矩阵展开 -->
    <q-card flat bordered>
      <q-card-section>
        <div class="text-subtitle1 q-mb-sm">矩阵展开（内容版本 × 画幅 × Hook）</div>
        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7 q-mb-xs">画幅（A 维度）</div>
            <q-option-group v-model="expandForm.aspects" type="checkbox" :options="ASPECT_OPTIONS" dense />
          </div>
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7 q-mb-xs">内容版本过滤（可选；留空 = 全部高光）</div>
            <q-select
              v-model="expandForm.contentVersionIds"
              dense
              outlined
              multiple
              use-chips
              :options="
                highlights.map((h) => ({
                  label: `${h.label || '(无标签)'} ${fmtDuration(h.startSec)}→${fmtDuration(h.endSec)}`,
                  value: h.id ?? '',
                }))
              "
              emit-value
              map-options
              :disable="highlights.length === 0"
            />
          </div>
          <div class="col-12 col-md-4">
            <div class="text-caption text-grey-7 q-mb-xs">混剪模板（片头/片尾/转场/字皮）</div>
            <q-select
              v-model="expandForm.templateId"
              dense
              outlined
              clearable
              :options="templates.filter((t) => t.enabled).map((t) => ({ label: t.name, value: t.id ?? '' }))"
              emit-value
              map-options
              label="不选 = 裸剪"
            />
          </div>
        </div>

        <div class="row items-center q-mt-md q-mb-xs">
          <div class="text-caption text-grey-7">Hook（前 3s，C 维度）</div>
          <q-space />
          <q-btn flat dense no-caps size="sm" icon="add" label="加 Hook" @click="addHook" />
        </div>
        <div v-for="(h, idx) in expandForm.hooks" :key="idx" class="row q-col-gutter-sm q-mb-xs items-center">
          <div class="col-3"><q-input v-model="h.label" dense outlined placeholder="Hook 名" /></div>
          <div class="col"><q-input v-model="h.text" dense outlined placeholder="前 3s 文案/贴纸文本" /></div>
          <div class="col-auto">
            <q-btn
              flat
              dense
              round
              color="negative"
              icon="close"
              :disable="expandForm.hooks.length <= 1"
              @click="expandForm.hooks.splice(idx, 1)"
            />
          </div>
        </div>

        <div class="row items-center q-mt-sm">
          <div class="text-caption text-grey-7">
            预计变体数：<b>{{ estimatedCount }}</b
            >（超过 60 后端以 MME_EXPAND_TOO_LARGE 拒绝）
          </div>
          <q-space />
          <q-btn
            unelevated
            no-caps
            color="primary"
            icon="apps"
            label="展开矩阵"
            :disable="!canExpand"
            :loading="expanding"
            @click="doExpand"
          />
        </div>
        <div v-if="highlights.length === 0" class="text-caption text-orange-9 q-mt-xs">
          需先保存至少一条内容版本（高光）才能展开。
        </div>
      </q-card-section>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { mmeTaskService, mmeVariantService } from '../../../services/index';
import {
  ASPECT_OPTIONS,
  errMessage,
  fmtBytes,
  fmtDuration,
  fmtTime,
  shortId,
  taskStateMeta,
  type HighlightCandidate,
  type HookSpec,
  type MontageTask,
  type MontageTemplate,
} from './types';

const route = useRoute();
const router = useRouter();
const taskId = String(route.params.id ?? '');

const task = ref<MontageTask | null>(null);
const loading = ref(false);
const analyzing = ref(false);
const savingHl = ref(false);
const expanding = ref(false);

const highlights = ref<HighlightCandidate[]>([]);
// 快照比对判定脏态：避免 deep watch 在加载后误标“未保存”
const highlightSnapshot = ref('[]');
const dirty = computed(() => JSON.stringify(highlights.value) !== highlightSnapshot.value);
const templates = ref<MontageTemplate[]>([]);

const expandForm = ref<{
  aspects: string[];
  contentVersionIds: string[];
  templateId: string | null;
  hooks: HookSpec[];
}>({
  aspects: ['9:16'],
  contentVersionIds: [],
  templateId: null,
  hooks: [{ id: 'h1', label: '悬念开场', text: '' }],
});

// 设置面板（W1）：展开默认值来自 mme.yaml 生效配置（GetSettings），
// 后端 ExpandMatrix 对空 aspects/hooks 也走同一配置——双端同源。
const HOOK_LABELS = ['Hook A', 'Hook B', 'Hook C', 'Hook D'];
const HOOK_IDS = ['hook_a', 'hook_b', 'hook_c', 'hook_d'];
const settingsLoaded = ref(false);

async function fetchSettings() {
  try {
    const s = await mmeTaskService.getSettings();
    if (s.defaultAspects && s.defaultAspects.length > 0) {
      expandForm.value.aspects = [...s.defaultAspects];
    }
    // 与后端 DefaultHooks 同规则：数量取配置（下限 2、上限 4），仅在用户未改默认单条时预填
    const n = Math.min(Math.max(s.defaultHookCount || 2, 2), 4);
    const untouchedDefault = expandForm.value.hooks.length === 1 && expandForm.value.hooks[0].id === 'h1';
    if (untouchedDefault) {
      expandForm.value.hooks = HOOK_IDS.slice(0, n).map((id, i) => ({
        id,
        label: HOOK_LABELS[i],
        text: '',
      }));
    }
  } catch {
    // 设置源不可用时保持页面内硬编码默认（后端仍按配置兜底），不阻断页面
  } finally {
    settingsLoaded.value = true;
  }
}

const estimatedCount = computed(() => {
  const cv =
    expandForm.value.contentVersionIds.length > 0 ? expandForm.value.contentVersionIds.length : highlights.value.length;
  return cv * expandForm.value.aspects.length * expandForm.value.hooks.length;
});

const canExpand = computed(
  () =>
    highlights.value.length > 0 &&
    expandForm.value.aspects.length > 0 &&
    expandForm.value.hooks.length > 0 &&
    estimatedCount.value > 0,
);

const highlightsValid = computed(() =>
  highlights.value.every((h) => (h.startSec ?? 0) >= 0 && (h.endSec ?? 0) > (h.startSec ?? 0)),
);

const highlightsErrors = computed(() =>
  highlights.value.map((h, i) =>
    (h.endSec ?? 0) > (h.startSec ?? 0) ? '' : `第 ${i + 1} 条：结束秒数必须大于开始秒数`,
  ),
);

async function fetchAll() {
  loading.value = true;
  try {
    const [t, hl] = await Promise.all([mmeTaskService.get(taskId), mmeTaskService.listHighlights(taskId)]);
    task.value = t;
    highlights.value = hl.items ?? [];
    highlightSnapshot.value = JSON.stringify(highlights.value);
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

async function fetchTemplates() {
  try {
    const res = await mmeVariantService.listTemplates();
    templates.value = res.items ?? [];
  } catch (e) {
    Notify.create({ type: 'warning', message: `模板加载失败：${errMessage(e)}` });
  }
}

function addHighlight() {
  highlights.value.push({ id: '', label: '', startSec: 0, endSec: 5, note: '', origin: 'manual' });
}

function removeHighlight(idx: number) {
  highlights.value.splice(idx, 1);
}

async function saveHighlights() {
  if (!highlightsValid.value) return;
  savingHl.value = true;
  try {
    const res = await mmeTaskService.upsertHighlights(taskId, highlights.value);
    highlights.value = res.items ?? [];
    highlightSnapshot.value = JSON.stringify(highlights.value);
    Notify.create({ type: 'positive', message: `已保存 ${highlights.value.length} 条内容版本` });
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    savingHl.value = false;
  }
}

function addHook() {
  expandForm.value.hooks.push({ id: `h${expandForm.value.hooks.length + 1}`, label: '', text: '' });
}

async function doAnalyze() {
  analyzing.value = true;
  try {
    const t = await mmeTaskService.analyze(taskId);
    Notify.create({ type: 'positive', message: `分析已触发，当前状态：${taskStateMeta(t.state).label}` });
    await fetchAll();
  } catch (e) {
    Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
  } finally {
    analyzing.value = false;
  }
}

async function doExpand() {
  expanding.value = true;
  try {
    const set = await mmeVariantService.expand({
      taskId,
      contentVersionIds: expandForm.value.contentVersionIds.length > 0 ? expandForm.value.contentVersionIds : undefined,
      aspects: expandForm.value.aspects,
      hooks: expandForm.value.hooks,
      templateId: expandForm.value.templateId || undefined,
    });
    Notify.create({ type: 'positive', message: `已展开 ${set.variantCount ?? 0} 个变体` });
    router.push(`/media/mme/variant-sets/${set.id}`);
  } catch (e) {
    // MME_EXPAND_TOO_LARGE / 无交集等如实上屏
    Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
  } finally {
    expanding.value = false;
  }
}

onMounted(() => {
  fetchAll();
  fetchTemplates();
  fetchSettings();
});
</script>
