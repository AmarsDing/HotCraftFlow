<template>
  <article
    class="dash-card repo-card"
    role="button"
    tabindex="0"
    :aria-label="t('githubRadar.detailAria', { name: repo.full_name })"
    @click="$emit('open', repo)"
    @keydown.enter="$emit('open', repo)"
    @keydown.space.prevent="$emit('open', repo)"
  >
    <div class="repo-card__top">
      <span class="repo-card__rank">#{{ repo.rank || '—' }}</span>
      <q-badge v-if="repo.is_new" color="primary" :label="t('githubRadar.newBadge')" />
      <span v-if="repo.language" class="repo-card__lang">
        <span class="repo-card__lang-dot" :style="{ background: langColor(repo.language) }" aria-hidden="true" />
        {{ repo.language }}
      </span>
      <q-space />
      <div class="repo-card__actions">
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="auto_awesome"
          :aria-label="t('githubRadar.aiSummary')"
          @click.stop="$emit('open', repo, 'insight')"
        >
          <q-tooltip>{{ t('githubRadar.aiSummary') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="download"
          :aria-label="t('githubRadar.clone')"
          @click.stop="$emit('open', repo, 'clone')"
        >
          <q-tooltip>{{ t('githubRadar.clone') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="code"
          :aria-label="t('githubRadar.viewSource')"
          @click.stop="$emit('open', repo, 'source')"
        >
          <q-tooltip>{{ t('githubRadar.viewSource') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          :icon="watched ? 'bookmark' : 'bookmark_border'"
          :color="watched ? 'primary' : undefined"
          :aria-label="watched ? t('githubRadar.unwatch') : t('githubRadar.watch')"
          @click.stop="$emit('watch', repo)"
        >
          <q-tooltip>{{ watched ? t('githubRadar.unwatch') : t('githubRadar.watch') }}</q-tooltip>
        </q-btn>
      </div>
    </div>

    <div class="repo-card__name-line">
      <span v-if="parts.owner" class="repo-card__owner">{{ parts.owner }} /</span>
      <span class="repo-card__name">{{ parts.name }}</span>
    </div>

    <div class="repo-card__desc">{{ descText }}</div>

    <div class="repo-card__meta">
      <span class="repo-card__stars" :title="t('githubRadar.meta.starsTooltip', { n: exactCount(repo.stars) })">
        <q-icon name="star" size="13px" /> {{ abbrevCount(repo.stars) }}
      </span>
      <span
        v-if="repo.stars_delta !== 0"
        class="repo-card__delta"
        :class="{ 'repo-card__delta--down': repo.stars_delta < 0 }"
      >
        {{ repo.stars_delta > 0 ? '+' : '' }}{{ abbrevCount(repo.stars_delta) }} {{ sinceLabel }}
        <q-tooltip>{{ t('githubRadar.deltaTooltip', { n: exactCount(repo.stars_delta) }) }}</q-tooltip>
      </span>
      <span :title="t('githubRadar.meta.forksTooltip', { n: exactCount(repo.forks) })">
        <q-icon name="call_split" size="13px" /> {{ abbrevCount(repo.forks) }}
      </span>
      <span v-if="repo.open_issues" :title="t('githubRadar.meta.issuesTooltip', { n: exactCount(repo.open_issues) })">
        <q-icon name="error_outline" size="13px" /> {{ abbrevCount(repo.open_issues) }}
      </span>
    </div>

    <div class="repo-card__meta repo-card__meta--sub">
      <q-badge v-if="repo.license_spdx" outline color="grey-7" class="repo-card__license" :label="repo.license_spdx">
        <q-tooltip>{{ repo.license || repo.license_spdx }}</q-tooltip>
      </q-badge>
      <span v-else class="repo-card__license repo-card__license--none" :title="t('githubRadar.noLicenseTooltip')">{{
        t('githubRadar.noLicense')
      }}</span>
      <a
        v-if="repo.homepage"
        class="repo-card__home"
        :href="repo.homepage"
        target="_blank"
        rel="noopener"
        :title="repo.homepage"
        :aria-label="t('githubRadar.homepage')"
        @click.stop
      >
        <q-icon name="language" size="14px" />
      </a>
      <span v-if="repo.watchers" :title="t('githubRadar.meta.watchersTooltip', { n: exactCount(repo.watchers) })">
        <q-icon name="visibility" size="13px" /> {{ abbrevCount(repo.watchers) }}
      </span>
      <span v-if="repo.pushed_at" :title="t('githubRadar.pushedAt', { time: repo.pushed_at })">
        <q-icon name="update" size="13px" /> {{ relTime(repo.pushed_at, t) }}
      </span>
    </div>

    <div class="repo-card__footer">
      <Sparkline v-if="repo.sparkline?.length" :values="repo.sparkline" :width="56" :height="18" />
      <q-space />
      <div v-if="repo.topics?.length" class="repo-card__topics">
        <q-chip v-for="tp in topicChips" :key="tp" dense outline size="sm" color="primary">{{ tp }}</q-chip>
        <q-chip v-if="topicsOverflow > 0" dense outline size="sm" color="grey-7" class="repo-card__topics-more">
          +{{ topicsOverflow }}
          <q-tooltip
            >{{ t('githubRadar.topicsMoreTooltip', { n: topicsOverflow }) }}：{{ topicsRest.join(', ') }}</q-tooltip
          >
        </q-chip>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import Sparkline from './Sparkline.vue';
import { abbrevCount, exactCount, langColor, relTime, splitFullName, type RepoItem } from '../types';

const { t } = useI18n();

const props = withDefaults(defineProps<{ repo: RepoItem; watched?: boolean; since?: 'today' | 'week' | 'month' }>(), {
  watched: false,
  since: 'today',
});
defineEmits<{ open: [RepoItem, string?]; watch: [RepoItem] }>();

const parts = computed(() => splitFullName(props.repo.full_name));
const descText = computed(() => props.repo.description?.trim() || t('githubRadar.noDesc'));
const topicChips = computed(() => (props.repo.topics ?? []).slice(0, 4));
const topicsRest = computed(() => (props.repo.topics ?? []).slice(4));
const topicsOverflow = computed(() => topicsRest.value.length);
const sinceLabel = computed(() => t(`githubRadar.since.${props.since}`));
</script>

<style scoped lang="sass">
.repo-card
  padding: var(--space-4)
  cursor: pointer
  display: flex
  flex-direction: column
  gap: var(--space-2)
  transition: border-color .15s ease, box-shadow .15s ease
  &:hover
    border-color: var(--glass-border-hover)

.repo-card__top
  display: flex
  align-items: center
  gap: var(--space-2)

.repo-card__rank
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-accent)

.repo-card__lang
  display: inline-flex
  align-items: center
  gap: 4px
  font-size: var(--text-xs)
  color: var(--color-text-secondary)

.repo-card__lang-dot
  display: inline-block
  width: 9px
  height: 9px
  border-radius: 50%

.repo-card__actions
  display: flex
  align-items: center
  opacity: 0
  transition: opacity .15s ease
  .repo-card:hover &,
  .repo-card:focus-within &
    opacity: 1
  @media (hover: none)
    opacity: 1

.repo-card__name-line
  font-size: var(--text-md)
  line-height: 1.35
  word-break: break-word

.repo-card__owner
  color: var(--color-text-secondary)
  font-weight: 500

.repo-card__name
  color: var(--color-text-heading, var(--color-text-primary))
  font-weight: 700

.repo-card__desc
  font-size: var(--text-sm)
  color: var(--color-text-secondary)
  line-height: 1.45
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  overflow: hidden
  min-height: 2.9em

.repo-card__meta
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--space-3)
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums

.repo-card__meta--sub
  gap: var(--space-2)

.repo-card__stars
  font-weight: 600
  color: var(--color-text-primary)

.repo-card__delta
  font-weight: 600
  color: var(--color-success)

.repo-card__delta--down
  color: var(--color-danger)

.repo-card__home
  display: inline-flex
  align-items: center
  color: var(--color-text-secondary)
  &:hover
    color: var(--color-accent)

.repo-card__license
  font-size: 11px

.repo-card__license--none
  color: var(--color-text-tertiary, var(--color-text-secondary))

.repo-card__footer
  display: flex
  align-items: center
  gap: var(--space-1)
  flex-wrap: wrap
  margin-top: auto

.repo-card__topics
  display: flex
  flex-wrap: wrap
  gap: 4px
</style>
