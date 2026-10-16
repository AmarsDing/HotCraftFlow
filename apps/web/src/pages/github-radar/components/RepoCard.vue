<template>
  <article
    class="dash-card repo-card"
    role="button"
    tabindex="0"
    @click="$emit('open', repo)"
    @keydown.enter="$emit('open', repo)"
  >
    <div class="repo-card__top">
      <span class="repo-card__rank">#{{ repo.rank || '—' }}</span>
      <q-badge v-if="repo.is_new" color="primary" label="新晋" />
      <q-space />
      <q-btn
        flat
        dense
        round
        size="sm"
        :icon="watched ? 'bookmark' : 'bookmark_border'"
        :color="watched ? 'primary' : undefined"
        :aria-label="watched ? '取消盯盘' : '加入盯盘'"
        @click.stop="$emit('watch', repo)"
      >
        <q-tooltip>{{ watched ? '取消盯盘' : '加入盯盘' }}</q-tooltip>
      </q-btn>
    </div>

    <div class="repo-card__name-line">
      <span v-if="parts.owner" class="repo-card__owner">{{ parts.owner }} /</span>
      <span class="repo-card__name">{{ parts.name }}</span>
    </div>

    <div class="repo-card__desc">{{ descText }}</div>

    <div class="repo-card__meta">
      <span v-if="repo.language">
        <span class="repo-card__lang-dot" aria-hidden="true" />{{ repo.language }}
      </span>
      <span>★ {{ abbrevCount(repo.stars) }}</span>
      <span>Fork {{ abbrevCount(repo.forks) }}</span>
    </div>

    <div class="repo-card__footer">
      <span class="repo-card__delta">
        +{{ repo.stars_delta }} {{ sinceLabel }}
        <q-tooltip>榜窗口增量</q-tooltip>
      </span>
      <Sparkline
        v-if="repo.sparkline?.length"
        class="q-ml-xs"
        :values="repo.sparkline"
        :width="48"
        :height="16"
      />
      <q-space />
      <div v-if="topicChips.length" class="repo-card__topics">
        <q-chip
          v-for="t in topicChips"
          :key="t"
          dense
          outline
          size="sm"
          color="primary"
        >{{ t }}</q-chip>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Sparkline from './Sparkline.vue'
import { abbrevCount, splitFullName, type RepoItem } from '../types'

const props = withDefaults(
  defineProps<{ repo: RepoItem; watched?: boolean; since?: 'today' | 'week' }>(),
  { watched: false, since: 'today' },
)
defineEmits<{ open: [RepoItem]; watch: [RepoItem] }>()

const parts = computed(() => splitFullName(props.repo.full_name))
const descText = computed(() => props.repo.description?.trim() || '暂无描述')
const topicChips = computed(() => (props.repo.topics ?? []).slice(0, 3))
const sinceLabel = computed(() => (props.since === 'week' ? '本周' : '今日'))
</script>

<style scoped lang="sass">
.repo-card
  padding: var(--space-4)
  cursor: pointer
  transition: border-color .15s ease, box-shadow .15s ease
  &:hover
    border-color: var(--glass-border-hover)

.repo-card__top
  display: flex
  align-items: center
  gap: var(--space-2)
  margin-bottom: var(--space-2)

.repo-card__rank
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-accent)

.repo-card__name-line
  font-size: var(--text-md)
  line-height: 1.35
  margin-bottom: var(--space-2)
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
  margin-bottom: var(--space-3)

.repo-card__meta
  display: flex
  flex-wrap: wrap
  gap: var(--space-3)
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
  margin-bottom: var(--space-2)

.repo-card__lang-dot
  display: inline-block
  width: 8px
  height: 8px
  border-radius: 50%
  background: var(--color-accent)
  margin-right: 4px
  vertical-align: middle

.repo-card__footer
  display: flex
  align-items: center
  gap: var(--space-1)
  flex-wrap: wrap

.repo-card__delta
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-success)

.repo-card__topics
  display: flex
  flex-wrap: wrap
  gap: 4px
</style>
