<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MIA 事件卡</div>
      <q-badge class="q-ml-sm" color="grey">{{ total }} 张</q-badge>
      <q-space />
      <q-btn
        unelevated dense no-caps color="purple" icon="call_merge" label="合并所选"
        class="q-mr-sm"
        :disable="selected.length < 2 || acting"
        @click="confirmMerge"
      >
        <q-tooltip>勾选 ≥2 张卡，合并到第一张并锁定另存</q-tooltip>
      </q-btn>
      <q-btn flat dense round icon="refresh" :loading="loading" @click="() => fetchData()" />
    </div>

    <q-banner dense rounded class="bg-blue-1 text-blue-10 q-mb-md">
      <template #avatar><q-icon name="hub" /></template>
      事件卡由 IC 归一化情报自动聚类生成；拆分/合并是人工干预且全程锁卡审计（避免干扰聚类过程践踏）。
    </q-banner>

    <!-- 过滤区 -->
    <div class="row q-col-gutter-sm q-mb-md items-center">
      <div class="col-12 col-sm-3">
        <q-select
          v-model="filters.category" outlined dense emit-value map-options clearable
          :options="categoryOptions" label="分类"
          @update:model-value="onFilterChange"
        />
      </div>
      <div class="col-12 col-sm-4">
        <q-input
          v-model="filters.keyword" outlined dense label="关键词（标题/摘要）" clearable
          @keyup.enter="onFilterChange" @clear="onFilterChange"
        />
      </div>
      <div class="col-12 col-sm-2">
        <q-btn outline dense no-caps color="primary" icon="search" label="查询" @click="onFilterChange" />
      </div>
    </div>

    <q-table
      flat bordered
      :rows="items"
      :columns="columns"
      row-key="event_id"
      :loading="loading"
      selection="multiple"
      v-model:selected="selected"
      v-model:pagination="pagination"
      :rows-per-page-options="[20, 50]"
      @request="onRequest"
    >
      <template #body-cell-title="props">
        <q-td :props="props">
          <div class="row items-center no-wrap">
            <q-icon
              v-if="props.row.split_locked" name="lock" size="16px" color="orange" class="q-mr-xs"
            >
              <q-tooltip>已人工干预（拆分/合并锁定另存）</q-tooltip>
            </q-icon>
            <span class="ellipsis" style="max-width: 380px">{{ props.row.title }}</span>
          </div>
          <div v-if="props.row.summary" class="text-caption text-grey-7 ellipsis" style="max-width: 380px">
            {{ props.row.summary }}
          </div>
        </q-td>
      </template>
      <template #body-cell-sources="props">
        <q-td :props="props">
          <q-badge color="grey-8">{{ props.row.source_count ?? props.row.source_intel_ids?.length ?? 0 }} 源</q-badge>
          <span class="text-caption text-grey-7 q-ml-xs ellipsis" style="max-width: 220px; display: inline-block; vertical-align: middle">
            {{ (props.row.domains ?? []).join('、') }}
          </span>
        </q-td>
      </template>
      <template #body-cell-occurred_at="props">
        <q-td :props="props">{{ fmtTime(props.row.occurred_at) }}</q-td>
      </template>
      <template #body-cell-merged_by="props">
        <q-td :props="props">
          <span v-if="props.row.merged_by" class="text-caption">{{ props.row.merged_by }}</span>
          <span v-else class="text-grey-5">-</span>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn
            flat dense no-caps color="deep-purple" icon="call_split" label="拆分"
            :disable="acting"
            @click="openSplit(props.row)"
          />
        </q-td>
      </template>
      <template #no-data>
        <div class="q-pa-lg text-grey-7">
          暂无事件卡——ICI intel 消费者接入（M2 待 IC 提供 intel-by-topic 端口）后自动聚类产出。
        </div>
      </template>
    </q-table>

    <!-- 拆分对话框：勾选拆出的来源指针 -->
    <q-dialog v-model="splitOpen" persistent>
      <q-card style="min-width: 520px; max-width: 720px">
        <q-card-section class="text-h6">
          拆分事件卡
          <div class="text-caption text-grey-7">{{ splitTarget?.title }}</div>
        </q-card-section>
        <q-card-section>
          <div class="text-body2 q-mb-sm">
            勾选要<b>拆出</b>到新卡的来源指针（至少 1 个，且不能全选）：
          </div>
          <q-option-group
            v-model="splitSelection"
            type="checkbox"
            :options="splitOptions"
            color="deep-purple"
          />
          <q-banner dense rounded class="bg-grey-2 text-grey-8 q-mt-md">
            拆分后原卡与新卡均锁定另存（split_locked），可追溯。
          </q-banner>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="splitOpen = false" />
          <q-btn
            unelevated no-caps color="deep-purple" label="执行拆分"
            :disable="!splitValid"
            :loading="acting"
            @click="doSplit"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { Dialog, Notify } from 'quasar';
import { miaEventService } from '../../../services/index';
import { errMessage, fmtTime, type EventCardItem } from './types';

const items = ref<EventCardItem[]>([]);
const total = ref(0);
const loading = ref(false);
const acting = ref(false);
const selected = ref<EventCardItem[]>([]);
const pagination = ref({ page: 1, rowsPerPage: 20, rowsNumber: 0 });

const filters = ref({
  category: null as string | null,
  keyword: '',
});

const categoryOptions = [
  { label: '市场相关（c_market）', value: 'c_market' },
  { label: '科技（c_tech）', value: 'c_tech' },
  { label: '其他（c_other）', value: 'c_other' },
];

const columns = [
  { name: 'title', label: '事件', field: 'title', align: 'left' as const },
  { name: 'category', label: '分类', field: 'category', align: 'left' as const },
  { name: 'sources', label: '来源', field: 'source_count', align: 'left' as const },
  { name: 'occurred_at', label: '发生时间', field: 'occurred_at', align: 'left' as const },
  { name: 'topic_id', label: '主题', field: 'topic_id', align: 'left' as const },
  { name: 'merged_by', label: '干预人', field: 'merged_by', align: 'left' as const },
  { name: 'actions', label: '操作', field: 'event_id', align: 'left' as const },
];

async function fetchData(opts: { keepSelection?: boolean } = {}) {
  loading.value = true;
  // 服务端分页：翻页/过滤/刷新后旧选择不可见——强制清空，避免把屏幕外的卡误并入合并请求
  if (!opts.keepSelection) selected.value = [];
  try {
    const res = await miaEventService.list({
      page: pagination.value.page,
      pageSize: pagination.value.rowsPerPage,
      category: filters.value.category ?? undefined,
      keyword: filters.value.keyword || undefined,
    });
    items.value = res.items ?? [];
    total.value = res.total ?? 0;
    pagination.value.rowsNumber = total.value;
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

function onRequest(props: { pagination: { page: number; rowsPerPage: number } }) {
  pagination.value.page = props.pagination.page;
  pagination.value.rowsPerPage = props.pagination.rowsPerPage;
  fetchData();
}

// 过滤变化：回到第 1 页再查（否则旧页码命中过滤后的空页，误导为"无数据"）
function onFilterChange() {
  pagination.value.page = 1;
  fetchData();
}

// ---------- 拆分 ----------

const splitOpen = ref(false);
const splitTarget = ref<EventCardItem | null>(null);
const splitSelection = ref<string[]>([]);

const splitOptions = computed(() =>
  (splitTarget.value?.source_intel_ids ?? []).map((id) => ({ label: id, value: id })),
);

const splitValid = computed(() => {
  const totalIds = splitTarget.value?.source_intel_ids?.length ?? 0;
  return splitSelection.value.length >= 1 && splitSelection.value.length < totalIds;
});

function openSplit(row: EventCardItem) {
  if ((row.source_intel_ids?.length ?? 0) < 2) {
    Notify.create({ type: 'warning', message: '该卡只有单个来源，无法拆分' });
    return;
  }
  splitTarget.value = row;
  splitSelection.value = [];
  splitOpen.value = true;
}

async function doSplit() {
  const target = splitTarget.value;
  if (!target?.event_id) return;
  acting.value = true;
  try {
    const res = await miaEventService.split(target.event_id, splitSelection.value);
    Notify.create({
      type: 'positive',
      message: `拆分完成：新卡 ${res.new_item?.event_id ?? ''}（两卡均已锁定另存）`,
    });
    splitOpen.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    acting.value = false;
  }
}

// ---------- 合并 ----------

function confirmMerge() {
  const ids = selected.value.map((r) => r.event_id).filter((v): v is string => Boolean(v));
  if (ids.length < 2) return;
  Dialog.create({
    title: '合并事件卡',
    message: `将把 ${ids.length} 张卡合并到第一张（${ids[0]}），合并后目标卡锁定另存。确认继续？`,
    cancel: { flat: true, noCaps: true, label: '取消' },
    ok: { unelevated: true, noCaps: true, color: 'purple', label: '执行合并' },
    persistent: true,
  }).onOk(async () => {
    acting.value = true;
    try {
      const res = await miaEventService.merge(ids);
      Notify.create({
        type: 'positive',
        message: `合并完成：${res.merged_item?.event_id ?? ids[0]}（已锁定另存）`,
      });
      selected.value = [];
      await fetchData();
    } catch (e) {
      Notify.create({ type: 'negative', message: errMessage(e) });
    } finally {
      acting.value = false;
    }
  });
}

onMounted(fetchData);
</script>
