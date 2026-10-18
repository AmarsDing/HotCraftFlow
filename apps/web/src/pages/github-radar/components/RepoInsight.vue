<template>
  <!-- AI 总结：三级（brief / summary / deep），生成 / 缓存 / 错误态 -->
  <div class="repo-insight">
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
        :label="
          current
            ? current.cached
              ? t('githubRadar.regenerate')
              : t('githubRadar.generate')
            : t('githubRadar.generateSummary')
        "
        @click="generate(activeLevel)"
      />
    </div>

    <q-banner v-if="!llmReady" dense rounded class="q-mb-md bg-warning text-white">
      {{ t('githubRadar.llmNotConfigured') }}
      <template #action>
        <q-btn flat dense :label="t('githubRadar.goSettings')" @click="$emit('open-settings')" />
      </template>
    </q-banner>

    <div v-if="loading" class="repo-insight__loading">
      <q-spinner-dots size="32px" color="primary" />
      <div class="text-caption q-mt-sm" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.aiAnalyzing', { name: fullName, level: levelDesc }) }}
      </div>
    </div>

    <template v-else-if="current">
      <q-banner v-if="current.error" dense rounded class="q-mb-md bg-negative text-white" data-cy="insight-error">
        {{ current.error }}
      </q-banner>
      <template v-if="current.content_markdown">
        <MarkdownView :source="current.content_markdown" />
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
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import { radarRepoService } from '../../../services';
import MarkdownView from './MarkdownView.vue';
import { INSIGHT_LEVELS, type AIInsight } from '../types';

const props = defineProps<{ fullName: string; llmReady?: boolean }>();
defineEmits<{ 'open-settings': [] }>();

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
const results = ref<Record<string, AIInsight>>({});

const current = computed(() => results.value[activeLevel.value] || null);
const levelDesc = computed(() => levels.value.find((l) => l.value === activeLevel.value)?.desc || '');
const { t } = useI18n();

watch(
  () => props.fullName,
  () => {
    results.value = {};
  },
);

async function generate(level: 'brief' | 'summary' | 'deep') {
  if (loading.value) return;
  loading.value = true;
  try {
    const res = await radarRepoService.generateInsight(props.fullName, level);
    if (res.insight) {
      results.value = { ...results.value, [level]: res.insight as unknown as AIInsight };
    }
  } catch (e) {
    Notify.create({ type: 'negative', message: t('githubRadar.insightFailed', { msg: (e as Error).message }) });
  } finally {
    loading.value = false;
  }
}

function selectLevel(level: 'brief' | 'summary' | 'deep') {
  activeLevel.value = level;
  if (!results.value[level]) {
    void generate(level);
  }
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
  align-items: center
  padding: var(--space-6)
</style>
