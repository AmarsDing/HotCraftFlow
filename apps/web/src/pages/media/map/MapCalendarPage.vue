<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MAP 矩阵日历</div>
      <q-badge outline class="q-ml-sm">只读</q-badge>
      <q-space />
      <q-btn flat dense round icon="chevron_left" @click="shiftMonth(-1)" />
      <div class="text-subtitle1 q-mx-md">{{ monthLabel }}</div>
      <q-btn flat dense round icon="chevron_right" @click="shiftMonth(1)" />
      <q-btn flat dense no-caps label="今天" class="q-ml-md" @click="goToday" />
      <q-btn flat dense round icon="refresh" class="q-ml-sm" :loading="loading" @click="fetchData" />
    </div>

    <div v-if="loading && items.length === 0" class="row justify-center q-pa-xl">
      <q-spinner-dots size="64px" color="primary" />
    </div>

    <div v-else-if="items.length === 0" class="row justify-center q-pa-xl text-grey-7">
      <div class="text-center">
        <q-icon name="event_busy" size="64px" class="q-mb-sm" />
        <div>尚无排期</div>
        <div class="text-caption">本月没有排入排期的分发单元。</div>
      </div>
    </div>

    <div v-else class="cal-grid">
      <div v-for="w in weekHeader" :key="w" class="cal-header">{{ w }}</div>
      <div
        v-for="day in monthDays"
        :key="day.key"
        class="cal-cell"
        :class="{ 'cal-out-month': !day.inMonth, 'cal-today': day.isToday }"
      >
        <div class="row items-center justify-between">
          <span class="text-caption text-grey-8">{{ day.dayNum }}</span>
          <q-icon v-if="day.isToday" name="today" size="14px" color="primary" />
        </div>
        <div class="q-gutter-xs q-mt-xs">
          <template v-for="it in itemsByDay[day.iso] || []">
            <q-chip
              v-for="t in it.targets || []" :key="`${it.unit_id}:${t}`"
              dense size="sm" outline
              :color="publishStateColor(it.publish_state)"
              class="cal-chip"
              :label="`${t} · ${chipState(it)}`"
              @click="goDetail(it)"
            >
              <q-tooltip>
                {{ it.schedule_mode }} 排期 {{ fmtTime(it.scheduled_for) }} · 审批 {{ it.approval_state }} · 发布 {{ it.publish_state }}（点击查看详情）
              </q-tooltip>
            </q-chip>
          </template>
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { mapService } from '../../../services/index';
import {
  errMessage, fmtTime, publishStateColor,
  type MapCalendarItem,
} from './types';

const router = useRouter();
const items = ref<MapCalendarItem[]>([]);
const loading = ref(false);

const cursor = ref(new Date()); // 任意落在目标月内的日期
const weekHeader = ['一', '二', '三', '四', '五', '六', '日'];

const monthLabel = computed(() => `${cursor.value.getFullYear()} 年 ${cursor.value.getMonth() + 1} 月`);

const monthRange = computed(() => {
  const first = new Date(cursor.value.getFullYear(), cursor.value.getMonth(), 1);
  const last = new Date(cursor.value.getFullYear(), cursor.value.getMonth() + 1, 0, 23, 59, 59, 999);
  return { from: first, to: last };
});

function toIso(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 按月视图构建 6×7 网格（周一起始）。 */
const monthDays = computed(() => {
  const { from } = monthRange.value;
  const start = new Date(from);
  const dow = (start.getDay() + 6) % 7; // 周日=0 → 周一=0
  start.setDate(start.getDate() - dow);
  const todayIso = toIso(new Date());
  const cells: { key: string; iso: string; dayNum: number; inMonth: boolean; isToday: boolean }[] = [];
  for (let i = 0; i < 42; i += 1) {
    const d = new Date(start);
    d.setDate(start.getDate() + i);
    const iso = toIso(d);
    cells.push({
      key: iso,
      iso,
      dayNum: d.getDate(),
      inMonth: d.getMonth() === cursor.value.getMonth(),
      isToday: iso === todayIso,
    });
  }
  return cells;
});

const itemsByDay = computed(() => {
  const m: Record<string, MapCalendarItem[]> = {};
  for (const it of items.value) {
    if (!it.scheduled_for) continue;
    const d = new Date(it.scheduled_for);
    if (Number.isNaN(d.getTime())) continue;
    const key = toIso(d);
    (m[key] ||= []).push(it);
  }
  return m;
});

function chipState(it: MapCalendarItem): string {
  if (it.publish_state === 'published') return '已发布';
  if (it.approval_state === 'approved') return '已批待发';
  if (it.approval_state === 'rejected') return '已驳回';
  return '待审';
}

async function fetchData() {
  loading.value = true;
  try {
    const { from, to } = monthRange.value;
    const res = await mapService.getCalendar(from.toISOString(), to.toISOString());
    items.value = res.items ?? [];
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

function shiftMonth(delta: number) {
  const d = new Date(cursor.value);
  d.setMonth(d.getMonth() + delta);
  cursor.value = d;
  fetchData();
}

function goToday() {
  cursor.value = new Date();
  fetchData();
}

function goDetail(it: MapCalendarItem) {
  if (it.unit_id) router.push(`/media/map/dispatch-units/${it.unit_id}`);
}

onMounted(fetchData);
</script>

<style scoped>
.cal-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 4px;
}

.cal-header {
  text-align: center;
  color: var(--q-grey-7);
  font-weight: 500;
  padding: 4px 0;
}

.cal-cell {
  min-height: 110px;
  border: 1px solid rgb(0 0 0 / 8%);
  border-radius: 8px;
  padding: 6px;
  overflow: hidden;
}

.cal-out-month {
  opacity: 40%;
}

.cal-today {
  border-color: var(--q-primary);
  box-shadow: 0 0 0 1px var(--q-primary);
}

.cal-chip {
  cursor: pointer;
  width: 100%;
  justify-content: flex-start;
}
</style>
