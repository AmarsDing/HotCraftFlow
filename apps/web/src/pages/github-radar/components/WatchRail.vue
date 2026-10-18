<template>
  <aside class="dash-card watch-rail column">
    <div class="dash-card__header">
      <div class="dash-card__title">{{ t('githubRadar.watchRail') }}</div>
      <q-badge v-if="unread > 0" color="primary" class="q-ml-sm">{{ unread }}</q-badge>
      <q-space />
      <q-btn dense flat round icon="add" :aria-label="t('githubRadar.watchRailAdd')" @click="$emit('add')">
        <q-tooltip>{{ t('githubRadar.watchRailAdd') }}</q-tooltip>
      </q-btn>
    </div>

    <div class="watch-rail__events">
      <div class="row items-center no-wrap">
        <div class="text-caption" style="color: var(--color-text-secondary)">
          {{ t('githubRadar.watchChanges') }}
        </div>
        <q-space />
        <q-btn
          v-if="unreadEvents > 0"
          dense
          flat
          no-caps
          size="sm"
          :label="t('githubRadar.markAllRead')"
          :loading="markingRead"
          @click="$emit('markAllRead')"
        />
      </div>
      <template v-if="events?.length">
        <div v-for="e in events" :key="e.id" class="text-body2 q-py-xs" :class="{ 'watch-rail__event--read': e.read }">
          {{ e.message }}
        </div>
      </template>
      <div v-else class="text-caption q-py-xs" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.watchNoChanges') }}
        <div class="q-mt-xs">{{ t('githubRadar.watchNoChangesHint') }}</div>
      </div>
    </div>

    <div class="col scroll watch-rail__list">
      <div v-for="w in items" :key="w.id" class="watch-rail__row">
        <div class="row items-center no-wrap">
          <span class="text-body2 ellipsis" style="min-width: 0">{{ w.full_name }}</span>
          <q-space />
          <span class="watch-rail__delta" :class="deltaClass(w.stars_delta)">
            {{ formatDelta(w.stars_delta) }}
            <q-tooltip>{{ t('githubRadar.watchDeltaTooltip') }}</q-tooltip>
          </span>
          <q-btn
            dense
            flat
            round
            size="sm"
            icon="close"
            class="q-ml-xs"
            :aria-label="t('githubRadar.watchRemove')"
            @click="$emit('remove', w.id)"
          />
        </div>
        <Sparkline v-if="w.sparkline?.length" class="q-mt-xs" :values="w.sparkline" :width="48" :height="16" />
      </div>
      <div v-if="!items.length" class="dash-card__empty q-pa-md">
        <div class="text-body2">{{ t('githubRadar.watchEmpty') }}</div>
        <div class="text-caption q-mt-xs" style="color: var(--color-text-secondary)">
          {{ t('githubRadar.watchEmptyHint') }}
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import Sparkline from './Sparkline.vue';
import type { WatchEvent, WatchItem } from '../types';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{ items: WatchItem[]; unread: number; events?: WatchEvent[]; markingRead?: boolean }>(),
  { events: () => [], markingRead: false },
);
defineEmits<{ add: []; remove: [string]; markAllRead: [] }>();

const unreadEvents = computed(() => (props.events ?? []).filter((e) => !e.read).length);

/** 增量格式：+N / -N（修复原 Δ-5 双重负号读法）。 */
function formatDelta(delta: number): string {
  return delta > 0 ? `+${delta}` : String(delta);
}

function deltaClass(delta: number): string {
  if (delta > 0) return 'watch-rail__delta--up';
  if (delta < 0) return 'watch-rail__delta--down';
  return '';
}
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

.watch-rail__event--read
  opacity: 0.55

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
  font-variant-numeric: tabular-nums

.watch-rail__delta--up
  color: var(--color-success)

.watch-rail__delta--down
  color: var(--color-danger)
</style>
