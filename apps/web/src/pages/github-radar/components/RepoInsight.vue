<template>
  <!-- AI 总结：三级（brief / summary / deep），生成 / 缓存 / 错误态 -->
  <div class="repo-insight" data-cy="insight-panel">
    <div class="row q-gutter-sm q-mb-md">
      <q-btn
        v-for="lv in levels"
        :key="lv.value"
        dense
        no-caps
        outline
        :color="activeLevel === lv.value ? 'primary' : undefined"
        :icon="lv.icon"
        :label="lv.label"
        @click="selectLevel(lv.value)"
      >
        <q-tooltip>{{ lv.desc }}</q-tooltip>
      </q-btn>
      <q-space />
      <q-btn
        dense
        flat
        no-caps
        color="primary"
        icon="refresh"
        :loading="loading"
        :disable="!llmReady"
        :label="
          current
            ? current.cached
              ? t('githubRadar.regenerate')
              : t('githubRadar.generate')
            : t('githubRadar.generateSummary')
        "
        @click="generate(activeLevel)"
      >
        <q-tooltip v-if="!llmReady">{{ t('githubRadar.llmNotConfigured') }}</q-tooltip>
      </q-btn>
    </div>

    <q-banner v-if="!llmReady" dense rounded class="q-mb-md bg-warning text-white">
      {{ t('githubRadar.llmNotConfigured') }}
      <template #action>
        <q-btn flat dense :label="t('githubRadar.goSettings')" @click="$emit('open-settings')" />
      </template>
    </q-banner>

    <!-- 请求级失败统一为内嵌 banner（弃 Notify 双通道），可原地重试 -->
    <q-banner v-if="apiError" dense rounded class="q-mb-md bg-negative text-white" data-cy="insight-error">
      {{ apiError }}
      <template #action>
        <q-btn v-if="llmReady" flat dense :label="t('common.retry')" @click="generate(activeLevel)" />
      </template>
    </q-banner>

    <div v-if="loading" class="repo-insight__loading">
      <q-skeleton type="text" width="45%" />
      <q-skeleton type="text" width="90%" />
      <q-skeleton type="text" width="80%" />
      <q-skeleton type="rect" height="72px" class="q-mt-xs" />
      <div class="text-caption q-mt-sm" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.aiAnalyzing', { name: fullName, level: levelDesc }) }}
      </div>
    </div>

    <template v-else-if="current">
      <q-banner v-if="current.error" dense rounded class="q-mb-md bg-negative text-white">
        {{ current.error }}
      </q-banner>
      <template v-if="current.content_markdown">
        <MarkdownRenderer :source="current.content_markdown" />
        <div class="repo-insight__meta text-caption q-mt-sm" style="color: var(--color-text-secondary)">
          <span v-if="current.cached"><q-icon name="cached" size="13px" /> {{ t('githubRadar.cachedResult') }} · </span>
          <span v-if="current.model">{{ t('githubRadar.modelLabel', { model: current.model }) }} · </span>
          <span v-if="current.duration_ms"
            >{{ t('githubRadar.durationSec', { sec: (current.duration_ms / 1000).toFixed(1) }) }} ·
          </span>
          <span v-if="current.generated_at">{{ formatTime(current.generated_at) }}</span>
        </div>
      </template>
    </template>

    <div v-else class="dash-card__empty app-empty-state-center--sm">
      <q-icon name="auto_awesome" size="36px" />
      <div class="text-body2 q-mt-sm">{{ t('githubRadar.noInsight') }}</div>
      <div class="text-caption" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.noInsightHint') }}
      </div>
      <q-btn
        class="q-mt-md"
        unelevated
        no-caps
        color="primary"
        icon="auto_awesome"
        :disable="!llmReady"
        :label="t('githubRadar.generateSummary')"
        @click="generate(activeLevel)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { radarRepoService } from '../../../services';
import MarkdownRenderer from '../../../components/MarkdownRenderer.vue';
import { INSIGHT_LEVELS, normalizeInsight, type AIInsight } from '../types';

const props = defineProps<{ fullName: string; llmReady?: boolean; prefetched?: AIInsight | null }>();
defineEmits<{ 'open-settings': [] }>();

const { t } = useI18n();

// 等级 label/desc 走 i18n（types.ts 中的 INSIGHT_LEVELS 保留 value/icon 作为静态数据源）
const levels = computed(() =>
  INSIGHT_LEVELS.map((l) => ({
    ...l,
    label: t(`githubRadar.insightLevel.${l.value}.label`),
    desc: t(`githubRadar.insightLevel.${l.value}.desc`),
  })),
);
const activeLevel = ref<'brief' | 'summary' | 'deep'>('summary');
const loading = ref(false);
const apiError = ref('');
const results = ref<Record<string, AIInsight>>({});

const current = computed(() => results.value[activeLevel.value] || null);
const levelDesc = computed(() => levels.value.find((l) => l.value === activeLevel.value)?.desc || '');

watch(
  () => props.fullName,
  () => {
    results.value = props.prefetched ? { [props.prefetched.level]: props.prefetched } : {};
    apiError.value = '';
  },
);

// 详情面板打开时带出的 cache-only 快照：仅作种子秒显（不烧配额）；
// 用户手动 generate 后新结果会按等级覆盖它。预取在 loadDetail 完成后才到达，故需独立 watch。
watch(
  () => props.prefetched,
  (p) => {
    if (!p) return;
    results.value = { ...results.value, [p.level]: p };
  },
);

async function generate(level: 'brief' | 'summary' | 'deep') {
  if (loading.value || !props.llmReady) return;
  loading.value = true;
  apiError.value = '';
  try {
    const res = await radarRepoService.generateInsight(props.fullName, level);
    if (res.insight) {
      results.value = {
        ...results.value,
        [level]: normalizeInsight(res.insight as unknown as Record<string, unknown>),
      };
    }
  } catch (e) {
    apiError.value = t('githubRadar.insightFailed', { msg: (e as Error).message });
  } finally {
    loading.value = false;
  }
}

// 切档不自动生成（防烧每日 3 次/级配额）：无本地结果时显示「生成」CTA 由用户显式触发
function selectLevel(level: 'brief' | 'summary' | 'deep') {
  activeLevel.value = level;
  apiError.value = '';
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}
</script>

<style scoped lang="sass">
.repo-insight__loading
  display: flex
  flex-direction: column
  gap: var(--space-2)
  padding: var(--space-2) 0
</style>
