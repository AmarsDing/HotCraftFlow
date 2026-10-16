<template>
  <aside class="dash-card watch-rail column">
    <div class="dash-card__header">
      <div class="dash-card__title">盯盘</div>
      <q-badge v-if="unread > 0" color="primary" class="q-ml-sm">{{ unread }}</q-badge>
      <q-space />
      <q-btn dense flat round icon="add" aria-label="添加盯盘" @click="$emit('add')">
        <q-tooltip>添加盯盘</q-tooltip>
      </q-btn>
    </div>

    <div class="watch-rail__events">
      <div class="text-caption" style="color: var(--color-text-secondary)">盯盘变更</div>
      <template v-if="events?.length">
        <div v-for="e in events" :key="e.id" class="text-body2 q-py-xs">{{ e.message }}</div>
      </template>
      <div v-else class="text-caption q-py-xs" style="color: var(--color-text-secondary)">
        暂无变更
        <div class="q-mt-xs">盯盘仓库有星标或 Release 变化时会出现在这里</div>
      </div>
    </div>

    <div class="col scroll watch-rail__list">
      <div v-for="w in items" :key="w.id" class="watch-rail__row">
        <div class="row items-center no-wrap">
          <span class="text-body2 ellipsis" style="min-width: 0">{{ w.full_name }}</span>
          <q-space />
          <span class="watch-rail__delta">
            Δ{{ w.stars_delta }}
            <q-tooltip>相对盯盘基线</q-tooltip>
          </span>
          <q-btn
            dense
            flat
            round
            size="sm"
            icon="close"
            class="q-ml-xs"
            aria-label="移除"
            @click="$emit('remove', w.id)"
          />
        </div>
        <Sparkline
          v-if="w.sparkline?.length"
          class="q-mt-xs"
          :values="w.sparkline"
          :width="48"
          :height="16"
        />
      </div>
      <div v-if="!items.length" class="dash-card__empty q-pa-md">
        <div class="text-body2">尚未盯盘</div>
        <div class="text-caption q-mt-xs" style="color: var(--color-text-secondary)">
          从榜上点书签开始盯盘，或点右上角 +
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import Sparkline from './Sparkline.vue'
import type { WatchEvent, WatchItem } from '../types'

withDefaults(
  defineProps<{ items: WatchItem[]; unread: number; events?: WatchEvent[] }>(),
  { events: () => [] },
)
defineEmits<{ add: []; remove: [string] }>()
</script>

<style scoped lang="sass">
.watch-rail
  height: 100%
  min-height: 420px
  padding: 0
  overflow: hidden

.watch-rail__events
  padding: var(--space-3) var(--space-4)
  border-bottom: 1px solid var(--glass-border)

.watch-rail__list
  padding: var(--space-2)

.watch-rail__row
  padding: var(--space-2) var(--space-3)
  border-radius: 10px
  &:hover
    background: var(--glass-surface-hover)

.watch-rail__delta
  font-size: var(--text-xs)
  font-weight: 600
  color: var(--color-success)
  font-variant-numeric: tabular-nums
</style>
