<template>
  <!-- 仓库详情面板：元数据 + AI 分析 + 源码 + 克隆（双栏 Dialog） -->
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog repo-detail" data-cy="repo-detail">
      <q-card-section class="app-glass-dialog__head">
        <div class="row items-center no-wrap">
          <div class="col">
            <div class="app-glass-dialog__title">{{ repo?.full_name }}</div>
            <div class="app-glass-dialog__subtitle ellipsis-2-lines">
              {{ repo?.description || t('githubRadar.noDesc') }}
            </div>
          </div>
          <q-btn v-close-popup flat dense round icon="close" :aria-label="t('common.close')" />
        </div>
        <div class="row items-center q-gutter-xs q-mt-sm">
          <q-badge v-if="repo?.license_spdx" outline color="grey-7" :label="repo.license_spdx">
            <q-tooltip>{{ repo.license || repo.license_spdx }}</q-tooltip>
          </q-badge>
          <span v-if="repo?.language" class="repo-detail__lang">
            <span class="repo-detail__lang-dot" :style="{ background: langColor(repo.language) }" aria-hidden="true" />
            {{ repo.language }}
          </span>
          <span class="repo-detail__stat"><q-icon name="star" size="13px" /> {{ exactCount(repo?.stars || 0) }}</span>
          <span v-if="repo?.stars_delta" class="repo-detail__delta"
            >{{ repo.stars_delta > 0 ? '+' : '' }}{{ exactCount(repo.stars_delta) }}
            {{ t('githubRadar.windowGrowth') }}</span
          >
          <span class="repo-detail__stat"
            ><q-icon name="call_split" size="13px" /> {{ exactCount(repo?.forks || 0) }}</span
          >
          <span v-if="repo?.open_issues" class="repo-detail__stat"
            ><q-icon name="error_outline" size="13px" /> {{ exactCount(repo.open_issues) }}</span
          >
          <q-space />
          <q-btn
            v-if="repo?.html_url"
            outline
            dense
            no-caps
            color="primary"
            icon="open_in_new"
            label="GitHub"
            :href="repo.html_url"
            target="_blank"
            tag="a"
          />
          <q-btn
            v-if="repo && !watched"
            unelevated
            dense
            no-caps
            color="primary"
            icon="bookmark_border"
            :label="t('githubRadar.watch')"
            @click="$emit('watch', repo)"
          />
          <q-btn v-else-if="repo" flat dense no-caps icon="bookmark" :label="t('githubRadar.watchingBadge')" disable />
        </div>
      </q-card-section>

      <q-card-section class="app-glass-dialog__body app-dialog-body">
        <div v-if="loading && !detail" class="row items-center justify-center q-pa-lg">
          <q-spinner size="28px" color="primary" />
        </div>

        <template v-else>
          <div class="repo-detail__layout">
            <!-- 左栏：元数据 -->
            <aside class="repo-detail__meta">
              <div class="repo-detail__meta-title text-caption text-weight-bold">{{ t('githubRadar.repoInfo') }}</div>
              <div class="repo-detail__meta-row">
                <span>License</span>
                <b>{{ detail?.license || t('githubRadar.notDetected') }}</b>
              </div>
              <div class="repo-detail__meta-row">
                <span>{{ t('githubRadar.primaryLanguage') }}</span>
                <b>{{ detail?.language || '—' }}</b>
              </div>
              <div class="repo-detail__meta-row">
                <span>Watchers</span>
                <b>{{ exactCount(detail?.watchers || 0) }}</b>
              </div>
              <div class="repo-detail__meta-row">
                <span>{{ t('githubRadar.defaultBranch') }}</span>
                <b>{{ detail?.default_branch || '—' }}</b>
              </div>
              <div class="repo-detail__meta-row">
                <span>{{ t('githubRadar.createdAt') }}</span>
                <b>{{ formatDate(detail?.created_at) }}</b>
              </div>
              <div class="repo-detail__meta-row">
                <span>{{ t('githubRadar.pushedAtLabel') }}</span>
                <b>{{ relTime(detail?.pushed_at) }}</b>
              </div>
              <div v-if="detail?.homepage" class="repo-detail__meta-row">
                <span>{{ t('githubRadar.homepage') }}</span>
                <b class="ellipsis"
                  ><a :href="detail.homepage" target="_blank" rel="noopener">{{ detail.homepage }}</a></b
                >
              </div>

              <div v-if="detail?.languages?.length" class="repo-detail__langs">
                <div class="repo-detail__meta-title text-caption text-weight-bold q-mt-sm">
                  {{ t('githubRadar.languageComposition') }}
                </div>
                <div class="repo-detail__lang-bar">
                  <span
                    v-for="l in detail.languages.slice(0, 6)"
                    :key="l.name"
                    :style="{ width: `${l.percent}%`, background: langColor(l.name) }"
                    :title="`${l.name} ${l.percent?.toFixed(1)}%`"
                  />
                </div>
                <div class="repo-detail__lang-legend">
                  <span v-for="l in detail.languages.slice(0, 6)" :key="l.name">
                    <i :style="{ background: langColor(l.name) }" />{{ l.name }} {{ l.percent?.toFixed(1) }}%
                  </span>
                </div>
              </div>

              <div v-if="detail?.topics?.length" class="q-mt-sm">
                <div class="repo-detail__meta-title text-caption text-weight-bold">Topics</div>
                <div class="row q-gutter-xs q-mt-xs">
                  <q-chip v-for="tp in detail.topics" :key="tp" dense outline size="sm" color="primary">{{
                    tp
                  }}</q-chip>
                </div>
              </div>

              <div v-if="detail?.contributors?.length" class="q-mt-sm">
                <div class="repo-detail__meta-title text-caption text-weight-bold">
                  {{ t('githubRadar.topContributors') }}
                </div>
                <div class="repo-detail__contribs">
                  <span v-for="c in detail.contributors" :key="c">{{ c }}</span>
                </div>
              </div>
            </aside>

            <!-- 右栏：Tab -->
            <div class="repo-detail__tabs">
              <q-tabs v-model="tab" dense no-caps align="left" active-color="primary" indicator-color="primary">
                <q-tab name="overview" icon="description" :label="t('githubRadar.tabOverview')" />
                <q-tab name="insight" icon="auto_awesome" :label="t('githubRadar.tabInsight')" />
                <q-tab name="source" icon="code" :label="t('githubRadar.viewSource')" />
                <q-tab name="clone" icon="download" :label="t('githubRadar.clone')" />
              </q-tabs>

              <q-separator />

              <q-tab-panels v-model="tab" animated class="repo-detail__panels">
                <q-tab-panel name="overview">
                  <div v-if="readme" class="repo-detail__readme">
                    <MarkdownView :source="readme.slice(0, 12000)" />
                    <div
                      v-if="readme.length > 12000"
                      class="text-caption q-mt-sm"
                      style="color: var(--color-text-secondary)"
                    >
                      {{ t('githubRadar.readmeTruncated') }}
                    </div>
                  </div>
                  <div v-else class="dash-card__empty app-empty-state-center--sm">
                    <q-icon name="description" size="36px" />
                    <div class="text-body2 q-mt-sm">{{ t('githubRadar.noReadme') }}</div>
                  </div>
                </q-tab-panel>

                <q-tab-panel name="insight">
                  <RepoInsight
                    v-if="repo"
                    :full-name="repo.full_name"
                    :llm-ready="llmReady"
                    @open-settings="$emit('open-settings')"
                  />
                </q-tab-panel>

                <q-tab-panel name="source" class="repo-detail__panel--flush">
                  <SourceBrowser v-if="repo" :full-name="repo.full_name" />
                </q-tab-panel>

                <q-tab-panel name="clone">
                  <div class="clone-panel">
                    <div class="clone-panel__row">
                      <span class="clone-panel__label">HTTPS</span>
                      <code>{{ detail?.clone_url || `https://github.com/${repo?.full_name}.git` }}</code>
                      <q-btn flat dense round size="sm" icon="content_copy" @click="copy(detail?.clone_url || '')">
                        <q-tooltip>{{ t('githubRadar.copy') }}</q-tooltip>
                      </q-btn>
                    </div>
                    <div class="clone-panel__row">
                      <span class="clone-panel__label">SSH</span>
                      <code>{{ detail?.ssh_url || `git@github.com:${repo?.full_name}.git` }}</code>
                      <q-btn flat dense round size="sm" icon="content_copy" @click="copy(detail?.ssh_url || '')">
                        <q-tooltip>{{ t('githubRadar.copy') }}</q-tooltip>
                      </q-btn>
                    </div>
                    <div class="clone-panel__row">
                      <span class="clone-panel__label">ZIP</span>
                      <q-btn
                        outline
                        dense
                        no-caps
                        color="primary"
                        icon="archive"
                        :label="t('githubRadar.downloadZip')"
                        :href="zipUrl"
                        target="_blank"
                        tag="a"
                      />
                    </div>
                    <div v-if="cloneEnabled" class="clone-panel__row">
                      <span class="clone-panel__label">{{ t('githubRadar.cloneServer') }}</span>
                      <q-btn
                        outline
                        dense
                        no-caps
                        color="primary"
                        icon="cloud_download"
                        :loading="cloning"
                        :label="t('githubRadar.cloneToServer')"
                        @click="cloneToServer"
                      />
                    </div>
                    <q-banner
                      v-if="serverMsg"
                      dense
                      rounded
                      class="q-mt-sm"
                      :class="serverOk ? 'bg-positive text-white' : 'bg-warning text-white'"
                    >
                      {{ serverMsg }}
                    </q-banner>
                  </div>
                </q-tab-panel>
              </q-tab-panels>
            </div>
          </div>
        </template>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { computed, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { radarRepoService } from '../../../services';
import MarkdownView from './MarkdownView.vue';
import RepoInsight from './RepoInsight.vue';
import SourceBrowser from './SourceBrowser.vue';
import { exactCount, langColor, relTime, type RepoItem } from '../types';

const { t } = useI18n();
const props = defineProps<{
  modelValue: boolean;
  repo: RepoItem | null;
  initialTab?: string;
  watched?: boolean;
  llmReady?: boolean;
  cloneEnabled?: boolean;
}>();
defineEmits<{ 'update:modelValue': [boolean]; watch: [RepoItem]; 'open-settings': [] }>();

const $q = useQuasar();
const tab = ref('overview');
const loading = ref(false);
const detail = ref<RepoItem | null>(null);
const readme = ref('');
const cloning = ref(false);
const serverMsg = ref('');
const serverOk = ref(false);

const zipUrl = computed(() =>
  props.repo ? `https://github.com/${props.repo.full_name}/archive/refs/heads/HEAD.zip` : '',
);

watch(
  () => props.modelValue,
  async (open) => {
    if (!open || !props.repo) return;
    tab.value = props.initialTab || 'overview';
    loading.value = true;
    serverMsg.value = '';
    try {
      const res = await radarRepoService.getRepo(props.repo.full_name, { includeReadme: true });
      if (res.repo) detail.value = res.repo as unknown as RepoItem;
      readme.value = res.readme || '';
    } catch {
      detail.value = null;
    } finally {
      loading.value = false;
    }
  },
);

async function copy(text: string) {
  if (!text) return;
  try {
    await navigator.clipboard.writeText(text);
    $q.notify({ type: 'positive', message: t('githubRadar.copied'), position: 'top' });
  } catch {
    $q.notify({ type: 'negative', message: t('githubRadar.copyFailed') });
  }
}

async function cloneToServer() {
  if (!props.repo || cloning.value) return;
  cloning.value = true;
  try {
    const res = await radarRepoService.clone(props.repo.full_name, true);
    serverOk.value = !!res.cloned;
    serverMsg.value = res.message || '';
  } catch (e) {
    serverOk.value = false;
    serverMsg.value = (e as Error).message;
  } finally {
    cloning.value = false;
  }
}

function formatDate(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
</script>

<style scoped lang="sass">
.repo-detail__layout
  display: grid
  grid-template-columns: 260px 1fr
  gap: var(--space-4)
  @media (max-width: 860px)
    grid-template-columns: 1fr

.repo-detail__lang
  display: inline-flex
  align-items: center
  gap: 4px
  font-size: var(--text-xs)
  color: var(--color-text-secondary)

.repo-detail__lang-dot
  width: 9px
  height: 9px
  border-radius: 50%

.repo-detail__stat
  display: inline-flex
  align-items: center
  gap: 3px
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums

.repo-detail__delta
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-success)

.repo-detail__meta
  border-right: 1px solid var(--glass-border)
  padding-right: var(--space-4)
  @media (max-width: 860px)
    border-right: none
    padding-right: 0

.repo-detail__meta-title
  color: var(--color-text-secondary)
  margin-bottom: var(--space-1)

.repo-detail__meta-row
  display: flex
  justify-content: space-between
  gap: var(--space-2)
  font-size: var(--text-xs)
  padding: 3px 0
  color: var(--color-text-secondary)
  b
    color: var(--color-text-primary)
    font-weight: 600
    text-align: right
    word-break: break-all

.repo-detail__lang-bar
  display: flex
  height: 8px
  border-radius: 4px
  overflow: hidden
  margin: var(--space-1) 0

.repo-detail__lang-legend
  display: flex
  flex-wrap: wrap
  gap: var(--space-2)
  font-size: 11px
  color: var(--color-text-secondary)
  span
    display: inline-flex
    align-items: center
    gap: 4px
  i
    width: 8px
    height: 8px
    border-radius: 50%

.repo-detail__contribs
  display: flex
  flex-wrap: wrap
  gap: 4px
  font-size: 11px
  color: var(--color-text-secondary)

.repo-detail__tabs
  min-width: 0

.repo-detail__panels
  min-height: 380px

.repo-detail__panel--flush
  padding: 0

.repo-detail__readme
  max-height: 420px
  overflow: auto
  padding-right: var(--space-2)

.clone-panel__row
  display: flex
  align-items: center
  gap: var(--space-2)
  margin-bottom: var(--space-3)
  code
    flex: 1
    min-width: 0
    overflow-x: auto
    white-space: nowrap
    background: var(--glass-surface)
    border: 1px solid var(--glass-border)
    border-radius: 6px
    padding: var(--space-1) var(--space-2)
    font-size: var(--text-xs)
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace

.clone-panel__label
  flex: 0 0 56px
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-text-secondary)

.ellipsis-2-lines
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  overflow: hidden
</style>
