<template>
  <q-btn
    v-bind="$attrs"
    flat
    round
    dense
    icon="notifications"
    :aria-label="t('myNotice.bellLabel')"
    @click="onOpen"
  >
    <q-badge
      v-if="notice.unreadCount > 0"
      floating
      color="negative"
      rounded
      :label="notice.unreadCount > 99 ? '99+' : notice.unreadCount"
    />
    <q-menu v-model="menuOpen" anchor="bottom right" self="top right" class="notice-bell-menu">
      <div class="notice-bell-panel">
        <div class="notice-bell-panel__header">
          <span class="text-weight-bold">{{ t('myNotice.title') }}</span>
          <q-btn
            flat
            dense
            size="sm"
            color="primary"
            :label="t('myNotice.markAllRead')"
            :disable="notice.unreadCount === 0"
            @click="onMarkAllRead"
          />
        </div>
        <q-separator />
        <div v-if="notice.recentLoading" class="notice-bell-panel__empty">
          <q-spinner-dots size="24px" color="primary" />
        </div>
        <div v-else-if="notice.recentItems.length === 0" class="notice-bell-panel__empty text-grey">
          {{ t('myNotice.empty') }}
        </div>
        <q-list v-else dense class="notice-bell-panel__list">
          <q-item
            v-for="item in notice.recentItems"
            :key="item.recipientId"
            v-ripple
            clickable
            class="notice-bell-item"
            @click="onOpenItem(item)"
          >
            <q-item-section side top class="notice-bell-item__dot-wrap">
              <span
                class="notice-bell-item__dot"
                :class="{ 'notice-bell-item__dot--read': item.status !== 'RECEIVED' }"
              />
            </q-item-section>
            <q-item-section>
              <q-item-label
                class="notice-bell-item__subject"
                :class="{ 'text-weight-bold': item.status === 'RECEIVED' }"
                lines="1"
              >
                {{ item.subject }}
              </q-item-label>
              <q-item-label caption lines="2">{{ item.content }}</q-item-label>
              <q-item-label caption class="notice-bell-item__time">{{ formatTime(item.createTime) }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
        <q-separator />
        <div class="notice-bell-panel__footer">
          <q-btn
            v-close-popup
            flat
            dense
            size="sm"
            color="primary"
            :label="t('myNotice.viewAll')"
            @click="goCenter"
          />
        </div>
      </div>
    </q-menu>
  </q-btn>

  <q-dialog v-model="centerOpen">
    <NoticeCenterPage v-if="centerOpen" embedded @close="centerOpen = false" />
  </q-dialog>

  <q-dialog v-model="detailOpen">
    <q-card class="notice-detail-card">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6 notice-detail-card__subject">{{ activeItem?.subject }}</div>
        <q-space />
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-card-section class="text-caption text-grey">
        {{ formatTime(activeItem?.createTime) }}
      </q-card-section>
      <q-card-section class="notice-detail-card__content">{{ activeItem?.content }}</q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useMyNoticeStore } from '../stores/my-notice';
import type { MyNoticeItem } from '../services';
import NoticeCenterPage from '../pages/notice/NoticeCenterPage.vue';

defineOptions({ inheritAttrs: false });

const { t } = useI18n();
const notice = useMyNoticeStore();

const menuOpen = ref(false);
const centerOpen = ref(false);
const detailOpen = ref(false);
const activeItem = ref<MyNoticeItem | null>(null);

function onOpen() {
  void notice.fetchRecent(10);
}

async function onMarkAllRead() {
  await notice.markRead();
}

async function onOpenItem(item: MyNoticeItem) {
  activeItem.value = item;
  detailOpen.value = true;
  if (item.status === 'RECEIVED' && item.recipientId !== undefined) {
    await notice.markRead([item.recipientId]);
  }
}

function goCenter() {
  menuOpen.value = false;
  // 等当前点击冒泡结束再开弹层，否则会被当成「点了弹层外」立刻关掉
  void nextTick(() => {
    window.setTimeout(() => {
      centerOpen.value = true;
    }, 0);
  });
}

function formatTime(iso?: string): string {
  if (!iso) return '-';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
</script>

<style lang="sass">
.notice-bell-panel
  width: 360px
  max-width: 90vw
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

  &__header
    display: flex
    align-items: center
    justify-content: space-between
    padding: 10px 14px

  &__footer
    display: flex
    justify-content: center
    padding: 4px

  &__empty
    display: flex
    justify-content: center
    padding: 28px 0

  &__list
    max-height: 420px
    overflow-y: auto

.notice-bell-item
  &__dot-wrap
    min-width: 14px
    padding-right: 0

  &__dot
    width: 7px
    height: 7px
    border-radius: 50%
    background: var(--color-accent)
    margin-top: 6px

    &--read
      background: transparent

  &__subject
    color: var(--color-text-heading)

  &__time
    opacity: 0.7

.notice-detail-card
  min-width: 360px
  max-width: 560px

  &__subject
    word-break: break-all

  &__content
    white-space: pre-wrap
    word-break: break-word
    color: var(--color-text-primary)
</style>
