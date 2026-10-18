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
        <q-toggle
          :model-value="unreadOnly"
          dense
          size="sm"
          :label="t('githubRadar.watchUnreadOnly')"
          @update:model-value="$emit('update:unreadOnly', $event)"
        />
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
        <div v-for="e in visibleEvents" :key="e.id" class="watch-rail__event row items-center no-wrap q-py-xs">
          <q-icon :name="kindIcon(e.kind)" size="14px" class="watch-rail__event-icon" aria-hidden="true" />
          <div class="col" style="min-width: 0">
            <div class="text-body2" :class="{ 'watch-rail__event--read': e.read }">{{ e.message }}</div>
            <div class="text-caption watch-rail__event-time">{{ relTime(e.created_at, t) }}</div>
          </div>
          <q-btn
            v-if="!e.read"
            dense
            flat
            round
            size="sm"
            icon="done"
            :aria-label="t('githubRadar.watchEventRead')"
            @click="$emit('read', e.id)"
          >
            <q-tooltip>{{ t('githubRadar.watchEventRead') }}</q-tooltip>
          </q-btn>
        </div>
        <!-- 事件长尾折叠：上限 200 条全铺会占满侧栏首屏，截最新 8 条 -->
        <div
          v-if="hiddenEventCount > 0"
          class="text-caption q-px-sm q-pb-xs"
          style="color: var(--color-text-tertiary, var(--color-text-secondary))"
        >
          {{ t('githubRadar.watchMoreEvents', { n: hiddenEventCount }) }}
        </div>
      </template>
      <div v-else class="text-caption q-py-xs" style="color: var(--color-text-secondary)">
        {{ t('githubRadar.watchNoChanges') }}
        <div class="q-mt-xs">{{ t('githubRadar.watchNoChangesHint') }}</div>
      </div>
    </div>

    <!-- 加载骨架 -->
    <div v-if="loading" class="watch-rail__list q-pa-sm">
      <div v-for="n in 4" :key="n" class="q-px-sm q-py-sm">
        <q-skeleton type="text" width="70%" />
        <q-skeleton type="text" width="40%" class="q-mt-xs" />
      </div>
    </div>

    <div v-else class="col scroll watch-rail__list">
      <!-- 盯盘行：可点击开详情（键盘 Enter/Space 可达），未读徽标醒目 -->
      <div
        v-for="w in items"
        :key="w.id"
        class="watch-rail__row"
        role="button"
        tabindex="0"
        :aria-label="t('githubRadar.watchOpenHint') + ' ' + w.full_name"
        @click="$emit('open', w)"
        @keydown.enter="$emit('open', w)"
        @keydown.space.prevent="$emit('open', w)"
      >
        <div class="row items-center no-wrap">
          <q-badge v-if="w.unread_count" color="primary" floating>{{ w.unread_count }}</q-badge>
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
            @click.stop
          >
            <q-popup-confirm @confirm="$emit('remove', w.id)">
              {{ t('githubRadar.watchRemoveConfirm', { name: w.full_name }) }}
            </q-popup-confirm>
            <q-tooltip>{{ t('githubRadar.watchRemove') }}</q-tooltip>
          </q-btn>
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
import { radarKindIcon as kindIcon, relTime, type WatchEvent, type WatchItem } from '../types';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    items: WatchItem[];
    unread: number;
    events?: WatchEvent[];
    markingRead?: boolean;
    loading?: boolean;
    unreadOnly?: boolean;
  }>(),
  { events: () => [], markingRead: false, loading: false, unreadOnly: false },
);
defineEmits<{
  add: [];
  remove: [string];
  markAllRead: [];
  open: [WatchItem];
  read: [string];
  'update:unreadOnly': [boolean];
}>();

const unreadEvents = computed(() => (props.events ?? []).filter((e) => !e.read).length);

// 事件区只铺最新 8 条，更早的折叠成计数提示（后端自带上限 200，前端再压首屏密度）
const EVENTS_CAP = 8;
const visibleEvents = computed(() => (props.events ?? []).slice(0, EVENTS_CAP));
const hiddenEventCount = computed(() => Math.max((props.events?.length ?? 0) - EVENTS_CAP, 0));

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

.watch-rail__event-icon
  color: var(--color-text-secondary)
  margin-right: var(--space-2)

.watch-rail__event--read
  color: var(--color-text-tertiary, var(--color-text-secondary))

.watch-rail__event-time
  color: var(--color-text-tertiary, var(--color-text-secondary))

.watch-rail__list
  padding: var(--space-2)

.watch-rail__row
  position: relative
  padding: var(--space-2) var(--space-3)
  border-radius: 10px
  cursor: pointer
  &:hover
    background: var(--glass-surface-hover)
  &:focus-visible
    outline: 2px solid var(--color-accent)
    outline-offset: -2px

.watch-rail__delta
  font-size: var(--text-xs)
  font-weight: 600
  font-variant-numeric: tabular-nums

.watch-rail__delta--up
  color: var(--color-success)

.watch-rail__delta--down
  color: var(--color-danger)
</style>
