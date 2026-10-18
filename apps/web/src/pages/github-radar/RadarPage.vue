<template>
  <div class="app-standard-page radar-page">
    <header class="app-page-hero app-page-hero--feature">
      <div>
        <div class="app-page-kicker">GitHub Radar</div>
        <h1 class="app-page-title">{{ t('githubRadar.title') }}</h1>
        <p class="app-page-subtitle">{{ t('githubRadar.subtitle') }}</p>
      </div>
      <div class="row items-center q-gutter-sm">
        <q-btn flat dense round icon="refresh" :aria-label="t('common.refresh')" @click="reload">
          <q-tooltip>{{ t('common.refresh') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          icon="notifications"
          :aria-label="t('githubRadar.notifications')"
          @click="openNotifications"
        >
          <q-badge v-if="unreadNotify > 0" color="negative" floating>{{ unreadNotify }}</q-badge>
          <q-tooltip>{{ t('githubRadar.notifications') }}</q-tooltip>
        </q-btn>
        <q-btn flat dense round icon="settings" :aria-label="t('githubRadar.settings')" @click="settingsOpen = true">
          <q-tooltip>{{ t('githubRadar.settings') }}</q-tooltip>
        </q-btn>
      </div>
    </header>

    <!-- 筛选栏：FrameWork .app-filter-bar -->
    <div class="app-filter-bar q-mb-md">
      <q-btn-toggle
        v-model="mode"
        class="app-filter-bar__field--auto"
        dense
        unelevated
        toggle-color="primary"
        :options="[
          { label: t('githubRadar.mode.trending'), value: 'trending' },
          { label: t('githubRadar.mode.rising'), value: 'rising' },
        ]"
      />
      <q-btn-toggle
        v-if="mode === 'trending'"
        v-model="since"
        class="app-filter-bar__field--sm"
        dense
        outline
        toggle-color="primary"
        :options="[
          { label: t('githubRadar.since.today'), value: 'today' },
          { label: t('githubRadar.since.week'), value: 'week' },
          { label: t('githubRadar.since.month'), value: 'month' },
        ]"
      />
      <q-select
        v-if="mode === 'trending'"
        v-model="sort"
        class="app-filter-bar__field--sm"
        dense
        outlined
        emit-value
        map-options
        :options="sortOptions"
      />
      <q-input
        v-model="query"
        class="app-filter-bar__field"
        dense
        outlined
        clearable
        :placeholder="
          mode === 'rising' ? t('githubRadar.searchPlaceholderRising') : t('githubRadar.searchPlaceholderTrending')
        "
        debounce="400"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="language"
        class="app-filter-bar__field--sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        :options="languageOptions"
        :placeholder="t('githubRadar.languagePlaceholder')"
      />
      <q-input
        v-if="mode === 'trending'"
        v-model="topic"
        class="app-filter-bar__field--sm"
        dense
        outlined
        clearable
        placeholder="Topic"
        debounce="400"
      />
      <q-select
        v-if="mode === 'trending'"
        v-model="license"
        class="app-filter-bar__field--sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        :options="licenseOptions"
        :placeholder="t('githubRadar.licensePlaceholder')"
      />
      <q-toggle
        v-if="mode === 'trending'"
        v-model="onlyNew"
        dense
        :label="t('githubRadar.onlyNew')"
        class="app-filter-bar__field--auto"
      />
      <div class="app-filter-bar__actions">
        <q-btn outline dense no-caps :label="t('common.reset')" @click="clearFilters" />
        <q-btn
          v-if="!isWide"
          flat
          dense
          no-caps
          color="primary"
          :label="`${t('githubRadar.watchList')} (${watches.length})`"
          icon="bookmarks"
          @click="railOpen = true"
        />
      </div>
    </div>

    <q-banner v-if="stale" dense rounded class="q-mb-md app-banner-warning">
      {{ t('githubRadar.staleBanner') }}{{ staleReason ? `：${staleReason}` : '' }}
    </q-banner>

    <!-- KPI：仅热门模式 -->
    <div v-if="mode === 'trending'" class="app-stat-strip q-mb-md">
      <div class="app-stat-card app-stat-card--static">
        <div class="app-stat-card__icon"><q-icon name="whatshot" size="22px" /></div>
        <div class="app-stat-card__body">
          <div class="app-stat-card__value">{{ statTotal }}</div>
          <div class="app-stat-card__label">{{ t('githubRadar.statTotal', { label: sinceLabel }) }}</div>
        </div>
      </div>
      <div
        class="app-stat-card app-stat-card--success"
        :class="{ 'is-filter-on': onlyNew }"
        role="button"
        tabindex="0"
        @click="onlyNew = !onlyNew"
        @keydown.enter="onlyNew = !onlyNew"
        @keydown.space.prevent="onlyNew = !onlyNew"
      >
        <div class="app-stat-card__icon"><q-icon name="fiber_new" size="22px" /></div>
        <div class="app-stat-card__body">
          <div class="app-stat-card__value">{{ newCount }}</div>
          <div class="app-stat-card__label">{{ t('githubRadar.statNew') }}</div>
        </div>
      </div>
      <div class="app-stat-card app-stat-card--info app-stat-card--static">
        <div class="app-stat-card__icon"><q-icon name="trending_up" size="22px" /></div>
        <div class="app-stat-card__body">
          <div class="app-stat-card__value">{{ risingCount }}</div>
          <div class="app-stat-card__label">{{ t('githubRadar.statRising') }}</div>
        </div>
      </div>
      <div class="app-stat-card app-stat-card--violet app-stat-card--static">
        <div class="app-stat-card__icon"><q-icon name="code" size="22px" /></div>
        <div class="app-stat-card__body">
          <div class="app-stat-card__value">{{ topLanguageLabel }}</div>
          <div class="app-stat-card__label">{{ t('githubRadar.statLangTop') }}</div>
        </div>
      </div>
      <div class="app-stat-card app-stat-card--static">
        <div class="app-stat-card__icon"><q-icon name="policy" size="22px" /></div>
        <div class="app-stat-card__body">
          <div class="app-stat-card__value">{{ topLicenseLabel }}</div>
          <div class="app-stat-card__label">{{ t('githubRadar.statLicenseTop') }}</div>
        </div>
      </div>
    </div>

    <div class="radar-page__body row no-wrap">
      <div class="radar-page__main col">
        <div class="row items-center q-mb-sm">
          <div class="text-caption" style="color: var(--color-text-secondary)">
            <template v-if="refreshedLabel">{{ t('githubRadar.refreshedAt', { time: refreshedLabel }) }}</template>
            <template v-else>{{ t('githubRadar.notRefreshed') }}</template>
          </div>
          <q-space />
          <span class="text-caption" style="color: var(--color-text-secondary)">{{ boardCaption }}</span>
        </div>

        <div v-if="loading && !items.length" class="radar-grid">
          <div v-for="n in 6" :key="n" class="dash-card radar-skeleton" />
        </div>
        <div v-else-if="items.length" class="radar-grid">
          <RepoCard
            v-for="repo in items"
            :key="repo.id || repo.full_name"
            :repo="repo"
            :watched="watchedNames.has(repo.full_name)"
            :since="since"
            @open="onOpen"
            @watch="onWatch"
          />
        </div>
        <div v-else class="dash-card">
          <div class="dash-card__empty app-empty-state-center--sm">
            <q-icon name="travel_explore" size="48px" />
            <div class="text-h6">{{ emptyTitle }}</div>
            <div class="text-caption" style="color: var(--color-text-secondary)">{{ emptyHint }}</div>
            <q-btn
              v-if="hasFilters"
              class="q-mt-md"
              outline
              color="primary"
              :label="t('githubRadar.clearFilters')"
              @click="clearFilters"
            />
          </div>
        </div>

        <div v-if="pageCount > 1" class="row justify-center q-mt-md">
          <q-pagination
            v-model="page"
            :max="pageCount"
            :max-pages="7"
            boundary-numbers
            direction-links
            color="primary"
            active-color="primary"
          />
        </div>
      </div>

      <div v-if="isWide" class="radar-page__rail">
        <WatchRail
          :items="watches"
          :unread="unreadEvents"
          :events="events"
          :marking-read="markingRead"
          @add="openAddWatch"
          @remove="onRemoveWatch"
          @mark-all-read="markAllEventsRead"
        />
      </div>
    </div>

    <q-drawer v-if="!isWide" v-model="railOpen" side="right" overlay bordered :width="320">
      <WatchRail
        :items="watches"
        :unread="unreadEvents"
        :events="events"
        :marking-read="markingRead"
        @add="openAddWatch"
        @remove="onRemoveWatch"
        @mark-all-read="markAllEventsRead"
      />
    </q-drawer>

    <!-- 详情（双栏面板：概览 / AI 分析 / 源码 / 克隆） -->
    <RepoDetailPanel
      v-model="detailOpen"
      :repo="detailRepo"
      :initial-tab="detailTab"
      :watched="detailRepo ? watchedNames.has(detailRepo.full_name) : false"
      :llm-ready="llmReady"
      :clone-enabled="cloneEnabled"
      @watch="onWatch"
      @open-settings="settingsOpen = true"
    />

    <!-- 克隆（卡片快捷入口） -->
    <CloneDialog v-model="cloneOpen" :full-name="cloneTarget" :clone-enabled="cloneEnabled" />

    <!-- 通知 -->
    <q-dialog v-model="notifyOpen">
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">{{ t('githubRadar.notifications') }}</div>
          <div class="app-glass-dialog__subtitle">{{ t('githubRadar.notificationsSubtitle') }}</div>
        </q-card-section>
        <q-card-section class="app-glass-dialog__scroll app-dialog-body">
          <div v-for="n in notifications" :key="n.id" class="q-py-sm" :class="{ 'radar-notify--read': n.read }">
            <div class="text-body2 text-weight-medium">{{ n.title }}</div>
            <div class="text-caption" style="color: var(--color-text-secondary)">{{ n.body }}</div>
          </div>
          <div v-if="!notifications.length" class="text-caption" style="color: var(--color-text-secondary)">
            {{ t('githubRadar.noNotifications') }}
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn
            v-if="unreadNotify > 0"
            flat
            :label="t('githubRadar.markAllRead')"
            :loading="markingRead"
            @click="markAllNotificationsRead"
          />
          <q-btn v-close-popup flat :label="t('common.close')" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 设置：GitHub Token + LLM + 克隆 -->
    <FormDrawer
      v-model="settingsOpen"
      :title="t('githubRadar.settings')"
      :saving="savingConfig"
      :width="460"
      dense
      @submit="saveConfig"
    >
      <div class="text-caption q-mb-xs text-weight-bold" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.settingsGitHub') }}
      </div>
      <div class="text-caption q-mb-sm" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.tokenHint') }}
        <template v-if="configMasked"> · {{ t('githubRadar.tokenCurrent', { masked: configMasked }) }}</template>
      </div>
      <q-input
        v-model="tokenInput"
        dense
        outlined
        type="password"
        :placeholder="t('githubRadar.tokenPlaceholder')"
        :rules="[(v) => !!String(v || '').trim() || t('githubRadar.tokenRequired')]"
      />

      <q-separator class="q-my-md" />

      <div class="text-caption q-mb-xs text-weight-bold" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.llmTitle') }}
      </div>
      <q-toggle v-model="llmEnabledInput" dense :label="t('githubRadar.llmEnable')" class="q-mb-sm" />
      <q-input
        v-model="llmBaseUrlInput"
        dense
        outlined
        class="q-mb-sm"
        :label="t('githubRadar.llmBaseUrl')"
        placeholder="https://api.openai.com/v1"
        :hint="t('githubRadar.llmBaseUrlHint')"
      />
      <q-input
        v-model="llmKeyInput"
        dense
        outlined
        type="password"
        class="q-mb-sm"
        :label="t('githubRadar.llmApiKey')"
        placeholder="sk-…"
        :hint="
          llmKeyMasked ? t('githubRadar.llmApiKeyCurrent', { masked: llmKeyMasked }) : t('githubRadar.llmApiKeyEmpty')
        "
      />
      <q-input
        v-model="llmModelInput"
        dense
        outlined
        class="q-mb-sm"
        :label="t('githubRadar.llmModel')"
        placeholder="gpt-4o-mini"
      />

      <q-separator class="q-my-md" />

      <div class="text-caption q-mb-xs text-weight-bold" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.cloneTitle') }}
      </div>
      <q-toggle v-model="cloneEnabledInput" dense :label="t('githubRadar.cloneEnable')" class="q-mb-sm" />
      <q-input
        v-model="cloneDirInput"
        dense
        outlined
        :label="t('githubRadar.cloneDir')"
        :placeholder="t('githubRadar.cloneDirPlaceholder')"
        :disable="!cloneEnabledInput"
      />
    </FormDrawer>

    <!-- 添加盯盘 -->
    <q-dialog v-model="addWatchOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">{{ t('githubRadar.addWatch') }}</div>
          <div class="app-glass-dialog__subtitle">{{ t('githubRadar.addWatchHint') }}</div>
        </q-card-section>
        <q-card-section class="app-glass-dialog__body app-dialog-body">
          <q-input
            v-model="addWatchName"
            dense
            outlined
            autofocus
            :placeholder="t('githubRadar.addWatchPlaceholder')"
            @keyup.enter="submitAddWatch"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat :label="t('common.cancel')" />
          <q-btn
            unelevated
            color="primary"
            :label="t('common.save')"
            :disable="!addWatchName.trim()"
            :loading="addingWatch"
            @click="submitAddWatch"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
/**
 * github热门追踪 — FrameWork 壳内页面（v2）
 * UI：.app-standard-page / .app-page-hero / .app-filter-bar / .dash-card / .app-stat-strip /
 *     .app-glass-dialog / FormDrawer + CSS tokens（禁止旧 radar-* 独立色板）
 * API：radar*Service (kratos gen) + 本地类型；snake/camel 双读适配
 * v2：日/周/月窗口 · 排序（热度/总星/更新/新晋）· License 过滤 · AI 三级总结 · 源码查看 · 一键克隆
 */
import { ref, computed, watch, onMounted } from 'vue';
import { useQuasar, Notify } from 'quasar';
import { useI18n } from 'vue-i18n';
import FormDrawer from '../../components/FormDrawer.vue';
import { radarTrendingService, radarWatchService, radarNotifyService, radarConfigService } from '../../services';
import RepoCard from './components/RepoCard.vue';
import WatchRail from './components/WatchRail.vue';
import RepoDetailPanel from './components/RepoDetailPanel.vue';
import CloneDialog from './components/CloneDialog.vue';
import type { RepoItem, WatchItem, WatchEvent, RadarNotification } from './types';

const $q = useQuasar();
const { t } = useI18n();

// ── 筛选 / 榜单状态 ──
const mode = ref<'trending' | 'rising'>('trending');
const since = ref<'today' | 'week' | 'month'>('today');
const sort = ref('hot');
const query = ref('');
const language = ref<string | null>(null);
const topic = ref('');
const license = ref<string | null>(null);
const onlyNew = ref(false);
const items = ref<RepoItem[]>([]);
const languageTop = ref<Record<string, number>>({});
const licenseTop = ref<Record<string, number>>({});
const loading = ref(false);
const stale = ref(false);
const staleReason = ref('');
const refreshedAt = ref('');

// ── 分页（消费后端 page/page_size/total）──
const page = ref(1);
const pageSize = 12;
const total = ref(0);
const boardNewCount = ref(0);
const boardRisingCount = ref(0);

// ── 侧栏 / 通知 / 配置 ──
const watches = ref<WatchItem[]>([]);
const events = ref<WatchEvent[]>([]);
const notifications = ref<RadarNotification[]>([]);
const configMasked = ref('');
const llmReady = ref(false);
const llmKeyMasked = ref('');
const cloneEnabled = ref(false);

// ── Dialog / Drawer 开关 ──
const detailOpen = ref(false);
const detailRepo = ref<RepoItem | null>(null);
const detailTab = ref('overview');
const cloneOpen = ref(false);
const cloneTarget = ref('');
const settingsOpen = ref(false);
const notifyOpen = ref(false);
const railOpen = ref(false);
const addWatchOpen = ref(false);
const addWatchName = ref('');
const addingWatch = ref(false);
const markingRead = ref(false);

// ── 设置表单 ──
const savingConfig = ref(false);
const tokenInput = ref('');
const llmEnabledInput = ref(false);
const llmBaseUrlInput = ref('');
const llmKeyInput = ref('');
const llmModelInput = ref('');
const cloneEnabledInput = ref(false);
const cloneDirInput = ref('');

// ── computed ──
const isWide = computed(() => $q.screen.width >= 1200);

const watchedNames = computed(() => new Set(watches.value.map((w) => w.full_name)));

const unreadEvents = computed(() => events.value.filter((e) => !e.read).length);

const unreadNotify = computed(() => notifications.value.filter((n) => !n.read).length);

const sortOptions = computed(() => [
  { label: t('githubRadar.sort.hot'), value: 'hot' },
  { label: t('githubRadar.sort.stars'), value: 'stars' },
  { label: t('githubRadar.sort.updated'), value: 'updated' },
  { label: t('githubRadar.sort.new'), value: 'new' },
]);

const licenseOptions = computed(() => {
  const keys = Object.keys(licenseTop.value);
  const base = ['MIT', 'Apache-2.0', 'GPL-3.0', 'BSD-3-Clause', 'BSD-2-Clause', 'AGPL-3.0', 'MPL-2.0', 'Unlicense'];
  const set = new Set([...base, ...keys]);
  return [...set].map((l) => ({
    label: licenseTop.value[l] ? `${l} (${licenseTop.value[l]})` : l,
    value: l,
  }));
});

const languageOptions = computed(() => {
  const keys = Object.keys(languageTop.value);
  if (!keys.length) {
    return ['TypeScript', 'Python', 'Go', 'Rust', 'JavaScript'].map((l) => ({
      label: l,
      value: l,
    }));
  }
  return Object.entries(languageTop.value)
    .sort((a, b) => b[1] - a[1])
    .map(([k, n]) => ({ label: `${k} (${n})`, value: k }));
});

const hasFilters = computed(
  () =>
    !!(
      language.value ||
      topic.value ||
      license.value ||
      onlyNew.value ||
      query.value ||
      (mode.value === 'rising' && query.value)
    ),
);

const refreshedLabel = computed(() => formatRefreshed(refreshedAt.value));

const sinceLabel = computed(() => t(`githubRadar.since.${since.value}`));

const boardCaption = computed(() => {
  if (mode.value === 'rising') return t('githubRadar.boardCaptionRising');
  const sortLabel = sortOptions.value.find((o) => o.value === sort.value)?.label || t('githubRadar.sort.hot');
  return t('githubRadar.boardCaptionSort', { label: sinceLabel.value, sort: sortLabel.replace(' ↓', '') });
});

const emptyTitle = computed(() =>
  hasFilters.value ? t('githubRadar.emptyFilteredTitle') : t('githubRadar.emptyTitle'),
);

const emptyHint = computed(() => {
  if (hasFilters.value) return t('githubRadar.emptyFilteredHint');
  if (mode.value === 'rising') return t('githubRadar.emptyRisingHint');
  return t('githubRadar.emptyTrendingHint');
});

// KPI 口径：总数/新晋/上涨均取后端统计（窗口级，不受分页影响），无后端字段时回退当前页计算。
const newCount = computed(() => boardNewCount.value || items.value.filter((r) => r.is_new).length);
const risingCount = computed(() => boardRisingCount.value || items.value.filter((r) => r.stars_delta > 0).length);
const statTotal = computed(() => total.value || items.value.length);
const pageCount = computed(() => Math.ceil(total.value / pageSize));
const topLanguageLabel = computed(() => {
  const entries = Object.entries(languageTop.value).sort((a, b) => b[1] - a[1]);
  return entries[0]?.[0] || '—';
});
const topLicenseLabel = computed(() => {
  const entries = Object.entries(licenseTop.value).sort((a, b) => b[1] - a[1]);
  return entries[0]?.[0] || '—';
});

function formatRefreshed(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  return `${hh}:${mm}`;
}

function asRepo(r: Record<string, unknown>): RepoItem {
  return {
    id: String(r.id ?? ''),
    full_name: String(r.full_name ?? r.fullName ?? ''),
    description: String(r.description ?? ''),
    language: String(r.language ?? ''),
    topics: (r.topics as string[]) || [],
    stars: Number(r.stars ?? 0),
    forks: Number(r.forks ?? 0),
    rank: Number(r.rank ?? 0),
    stars_delta: Number(r.stars_delta ?? r.starsDelta ?? 0),
    is_new: Boolean(r.is_new ?? r.isNew),
    html_url: String(r.html_url ?? r.htmlUrl ?? ''),
    sparkline: ((r.sparkline as number[]) || []).map(Number),
    license: String(r.license ?? '') || undefined,
    license_spdx: String(r.license_spdx ?? r.licenseSpdx ?? '') || undefined,
    homepage: String(r.homepage ?? '') || undefined,
    open_issues: Number(r.open_issues ?? r.openIssues ?? 0) || undefined,
    watchers: Number(r.watchers ?? 0) || undefined,
    contributors: (r.contributors as string[]) || undefined,
    created_at: String(r.created_at ?? r.createdAt ?? '') || undefined,
    updated_at: String(r.updated_at ?? r.updatedAt ?? '') || undefined,
    pushed_at: String(r.pushed_at ?? r.pushedAt ?? '') || undefined,
    clone_url: String(r.clone_url ?? r.cloneUrl ?? '') || undefined,
    ssh_url: String(r.ssh_url ?? r.sshUrl ?? '') || undefined,
    zip_url: String(r.zip_url ?? r.zipUrl ?? '') || undefined,
    default_branch: String(r.default_branch ?? r.defaultBranch ?? '') || undefined,
    languages: ((r.languages as Array<Record<string, unknown>>) || []).map((l) => ({
      name: String(l.name ?? ''),
      bytes: Number(l.bytes ?? 0),
      percent: Number(l.percent ?? 0),
    })),
    readme_excerpt: String(r.readme_excerpt ?? r.readmeExcerpt ?? '') || undefined,
  };
}

function asWatch(r: Record<string, unknown>): WatchItem {
  return {
    id: String(r.id ?? ''),
    full_name: String(r.full_name ?? r.fullName ?? ''),
    stars: Number(r.stars ?? 0),
    stars_delta: Number(r.stars_delta ?? r.starsDelta ?? 0),
    unread_count: Number(r.unread_count ?? r.unreadCount ?? 0),
    sparkline: ((r.sparkline as number[]) || []).map(Number),
    html_url: String(r.html_url ?? r.htmlUrl ?? ''),
  };
}

function asEvent(r: Record<string, unknown>): WatchEvent {
  return {
    id: String(r.id ?? ''),
    full_name: String(r.full_name ?? r.fullName ?? ''),
    kind: String(r.kind ?? ''),
    message: String(r.message ?? ''),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    read: Boolean(r.read),
  };
}

function asNotify(r: Record<string, unknown>): RadarNotification {
  return {
    id: String(r.id ?? ''),
    title: String(r.title ?? ''),
    body: String(r.body ?? ''),
    kind: String(r.kind ?? ''),
    created_at: String(r.created_at ?? r.createdAt ?? ''),
    read: Boolean(r.read),
  };
}

// loadBoardSeq 竞态防护：快速连续切换筛选时，只采纳最后一次请求的响应。
let loadBoardSeq = 0;

async function loadBoard() {
  const seq = ++loadBoardSeq;
  loading.value = true;
  try {
    if (mode.value === 'rising') {
      const data = await radarTrendingService.searchRising({
        query: query.value || undefined,
        language: language.value || undefined,
        page: page.value,
        pageSize,
      });
      const raw = data as unknown as Record<string, unknown>;
      items.value = ((raw.items as unknown[]) || []).map((x) => asRepo(x as Record<string, unknown>));
      stale.value = Boolean(raw.stale);
      staleReason.value = String(raw.stale_reason ?? raw.staleReason ?? '');
      refreshedAt.value = String(raw.refreshed_at ?? raw.refreshedAt ?? '');
      total.value = Number(raw.total ?? items.value.length);
    } else {
      const data = await radarTrendingService.listTrending({
        since: since.value,
        language: language.value || undefined,
        topic: topic.value || undefined,
        license: license.value || undefined,
        onlyNew: onlyNew.value || undefined,
        sort: sort.value,
        query: query.value || undefined,
        page: page.value,
        pageSize,
      });
      const raw = data as unknown as Record<string, unknown>;
      items.value = ((raw.items as unknown[]) || []).map((x) => asRepo(x as Record<string, unknown>));
      stale.value = Boolean(raw.stale);
      staleReason.value = String(raw.stale_reason ?? raw.staleReason ?? '');
      refreshedAt.value = String(raw.refreshed_at ?? raw.refreshedAt ?? '');
      languageTop.value = (raw.language_top ?? raw.languageTop ?? {}) as Record<string, number>;
      licenseTop.value = (raw.license_top ?? raw.licenseTop ?? {}) as Record<string, number>;
      total.value = Number(raw.total ?? items.value.length);
      boardNewCount.value = Number(raw.new_count ?? raw.newCount ?? 0);
      boardRisingCount.value = Number(raw.rising_count ?? raw.risingCount ?? 0);
    }
  } catch (e) {
    if (seq === loadBoardSeq) {
      Notify.create({ type: 'negative', message: t('githubRadar.loadFailed', { msg: (e as Error).message }) });
    }
  } finally {
    if (seq === loadBoardSeq) loading.value = false;
  }
}

async function loadSide() {
  try {
    const [w, ev, n, cfg] = await Promise.all([
      radarWatchService.list(),
      radarWatchService.listEvents(),
      radarNotifyService.list(),
      radarConfigService.get(),
    ]);
    const wr = w as unknown as Record<string, unknown>;
    const er = ev as unknown as Record<string, unknown>;
    const nr = n as unknown as Record<string, unknown>;
    const cr = cfg as unknown as Record<string, unknown>;
    watches.value = ((wr.items as unknown[]) || []).map((x) => asWatch(x as Record<string, unknown>));
    events.value = ((er.items as unknown[]) || []).map((x) => asEvent(x as Record<string, unknown>));
    notifications.value = ((nr.items as unknown[]) || []).map((x) => asNotify(x as Record<string, unknown>));
    configMasked.value = String(cr.github_token_masked ?? cr.githubTokenMasked ?? '');
    llmReady.value = Boolean(cr.llm_enabled ?? cr.llmEnabled) && Boolean(cr.llm_api_key_masked ?? cr.llmApiKeyMasked);
    llmKeyMasked.value = String(cr.llm_api_key_masked ?? cr.llmApiKeyMasked ?? '');
    cloneEnabled.value = Boolean(cr.clone_enabled ?? cr.cloneEnabled);
    // 回填设置表单
    llmEnabledInput.value = Boolean(cr.llm_enabled ?? cr.llmEnabled);
    llmBaseUrlInput.value = String(cr.llm_base_url ?? cr.llmBaseUrl ?? '');
    llmModelInput.value = String(cr.llm_model ?? cr.llmModel ?? '');
    cloneEnabledInput.value = Boolean(cr.clone_enabled ?? cr.cloneEnabled);
    cloneDirInput.value = String(cr.clone_dir ?? cr.cloneDir ?? '');
  } catch {
    /* side panel optional during early wire */
  }
}

function reload() {
  void loadBoard();
  void loadSide();
}

// openNotifications 打开通知弹窗时顺带刷新（保证未读数与事件视图一致）。
function openNotifications() {
  notifyOpen.value = true;
  void loadSide();
}

// markAllEventsRead 盯盘事件全部已读（WatchRail 入口），同时刷新通知视图保持一致。
async function markAllEventsRead() {
  const ids = events.value.filter((e) => !e.read).map((e) => e.id);
  if (!ids.length) return;
  markingRead.value = true;
  try {
    await radarWatchService.markEventsRead(ids);
    events.value = events.value.map((e) => ({ ...e, read: true }));
    notifications.value = notifications.value.map((n) => ({ ...n, read: true }));
    Notify.create({ type: 'positive', message: t('githubRadar.markedAllRead') });
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message });
  } finally {
    markingRead.value = false;
  }
}

// markAllNotificationsRead 全部已读：传具体未读 id（后端拒绝空 ids，语义=显式指定）。
async function markAllNotificationsRead() {
  const ids = notifications.value.filter((n) => !n.read).map((n) => n.id);
  if (!ids.length) return;
  markingRead.value = true;
  try {
    await radarNotifyService.markRead(ids);
    notifications.value = notifications.value.map((n) => ({ ...n, read: true }));
    events.value = events.value.map((e) => ({ ...e, read: true }));
    Notify.create({ type: 'positive', message: t('githubRadar.markedAllRead') });
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message });
  } finally {
    markingRead.value = false;
  }
}

function clearFilters() {
  query.value = '';
  language.value = null;
  topic.value = '';
  license.value = null;
  onlyNew.value = false;
}

function onOpen(repo: RepoItem, tab = 'overview') {
  detailRepo.value = repo;
  detailTab.value = tab;
  detailOpen.value = true;
}

async function onWatch(repo: RepoItem) {
  if (watchedNames.value.has(repo.full_name)) return;
  try {
    await radarWatchService.add(repo.full_name);
    await loadSide();
    Notify.create({ type: 'positive', message: t('githubRadar.watched', { name: repo.full_name }) });
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message });
  }
}

function openAddWatch() {
  addWatchName.value = '';
  addWatchOpen.value = true;
}

async function submitAddWatch() {
  const full = addWatchName.value.trim();
  if (!full) return;
  addingWatch.value = true;
  try {
    await radarWatchService.add(full);
    await loadSide();
    addWatchOpen.value = false;
    addWatchName.value = '';
    Notify.create({ type: 'positive', message: t('githubRadar.watched', { name: full }) });
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message });
  } finally {
    addingWatch.value = false;
  }
}

async function onRemoveWatch(id: string) {
  try {
    await radarWatchService.remove(id);
    await loadSide();
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message });
  }
}

async function saveConfig() {
  savingConfig.value = true;
  try {
    const cfg = await radarConfigService.set({
      githubToken: tokenInput.value || undefined,
      llmEnabled: llmEnabledInput.value,
      llmBaseUrl: llmBaseUrlInput.value || undefined,
      llmApiKey: llmKeyInput.value || undefined,
      llmModel: llmModelInput.value || undefined,
      cloneEnabled: cloneEnabledInput.value,
      cloneDir: cloneDirInput.value || undefined,
    });
    const raw = cfg as unknown as Record<string, unknown>;
    configMasked.value = String(raw.github_token_masked ?? raw.githubTokenMasked ?? '');
    llmKeyMasked.value = String(raw.llm_api_key_masked ?? raw.llmApiKeyMasked ?? '');
    llmReady.value = Boolean(raw.llm_enabled ?? raw.llmEnabled) && !!llmKeyMasked.value;
    cloneEnabled.value = Boolean(raw.clone_enabled ?? raw.cloneEnabled);
    tokenInput.value = '';
    llmKeyInput.value = '';
    settingsOpen.value = false;
    Notify.create({ type: 'positive', message: t('githubRadar.saved') });
  } catch (e) {
    Notify.create({ type: 'negative', message: (e as Error).message });
  } finally {
    savingConfig.value = false;
  }
}

// 筛选变化先归位第 1 页（页码 watcher 统一触发加载，避免双请求）。
watch([mode, since, sort, query, language, topic, license, onlyNew], () => {
  if (page.value !== 1) {
    page.value = 1;
  } else {
    void loadBoard();
  }
});

watch(page, () => {
  void loadBoard();
});

onMounted(() => {
  void loadBoard();
  void loadSide();
});
</script>

<style scoped lang="sass">
.radar-page__body
  gap: var(--space-4)
  align-items: stretch

.radar-page__main
  min-width: 0

.radar-page__rail
  flex: 0 0 300px
  max-width: 300px

.radar-grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr))
  gap: var(--space-4)

.radar-skeleton
  min-height: 190px
  opacity: 0.55

.radar-notify--read
  opacity: 0.55
</style>
