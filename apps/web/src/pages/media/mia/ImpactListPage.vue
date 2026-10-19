<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MIA 影响卡</div>
      <q-badge class="q-ml-sm" color="grey">{{ total }} 张</q-badge>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <q-banner dense rounded class="bg-amber-1 text-amber-10 q-mb-md">
      <template #avatar><q-icon name="policy" /></template>
      研究/监测辅助，非投资建议；模型输出须人工复核。传导结论均为「假设非因果」。行情只读，本模块不存在任何交易入口。
    </q-banner>

    <!-- 过滤区 -->
    <div class="row q-col-gutter-sm q-mb-md items-center">
      <div class="col-12 col-sm-3">
        <q-select
          v-model="filters.direction" outlined dense emit-value map-options
          :options="directionOptions" label="方向" clearable
          @update:model-value="onFilterChange"
        />
      </div>
      <div class="col-12 col-sm-2">
        <q-input
          v-model.number="filters.minScore" outlined dense type="number" min="0" max="100"
          label="最低影响分" clearable
          @update:model-value="onFilterChange"
        />
      </div>
      <div class="col-12 col-sm-3">
        <q-toggle
          v-model="filters.includeExpired" label="包含已过期卡（TTL 到期默认隐藏）"
          @update:model-value="onFilterChange"
        />
      </div>
    </div>

    <q-table
      flat bordered
      :rows="items"
      :columns="columns"
      row-key="impact_id"
      :loading="loading"
      v-model:pagination="pagination"
      :rows-per-page-options="[20, 50]"
      @request="onRequest"
    >
      <template #body-cell-title="props">
        <q-td :props="props">
          <div class="row items-center no-wrap">
            <q-badge v-if="props.row.no_mapping" color="orange" class="q-mr-xs">未映射</q-badge>
            <q-badge v-if="props.row.pushed" color="blue" class="q-mr-xs">已推送</q-badge>
            <span class="ellipsis" style="max-width: 360px">{{ props.row.title }}</span>
          </div>
        </q-td>
      </template>
      <template #body-cell-direction="props">
        <q-td :props="props">
          <q-badge :color="directionMeta(props.row.direction).color">
            <q-icon :name="directionMeta(props.row.direction).icon" class="q-mr-xs" />
            {{ directionMeta(props.row.direction).label }}
          </q-badge>
        </q-td>
      </template>
      <template #body-cell-impact_score="props">
        <q-td :props="props">
          <span class="text-weight-bold">{{ fmtScore(props.row.impact_score) }}</span>
        </q-td>
      </template>
      <template #body-cell-sentiment="props">
        <q-td :props="props">
          <q-badge :color="sentimentMeta(props.row.sentiment_label).color">
            {{ sentimentMeta(props.row.sentiment_label).label }}
          </q-badge>
          <span class="text-caption text-grey-7 q-ml-xs">
            {{ props.row.sentiment_score?.toFixed(2) ?? '-' }} · {{ engineLabel(props.row.sentiment_engine) }}
          </span>
        </q-td>
      </template>
      <template #body-cell-ttl="props">
        <q-td :props="props">
          <q-badge v-if="ttlRemaining(props.row.expires_at).expired" color="grey-5">已过期</q-badge>
          <span v-else class="text-caption">
            剩 {{ ttlRemaining(props.row.expires_at).text }} / {{ props.row.ttl_hours }}h
          </span>
        </q-td>
      </template>
      <template #body-cell-occurred_at="props">
        <q-td :props="props">{{ fmtTime(props.row.occurred_at) }}</q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn
            flat dense no-caps color="primary" icon="visibility" label="详情"
            @click="goDetail(props.row)"
          />
        </q-td>
      </template>
      <template #no-data>
        <div class="q-pa-lg text-grey-7">
          暂无影响卡——检查 IC 采集链是否产出 c_market 情报、MIA 是否启用、行级日志的合规硬校验是否拒绝（免责声明/path/confidence）。
        </div>
      </template>
    </q-table>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { miaImpactService } from '../../../services/index';
import {
  directionMeta, engineLabel, errMessage, fmtScore, fmtTime, sentimentMeta, ttlRemaining,
  type ImpactCardListItem,
} from './types';

const router = useRouter();

const items = ref<ImpactCardListItem[]>([]);
const total = ref(0);
const loading = ref(false);
const pagination = ref({ page: 1, rowsPerPage: 20, rowsNumber: 0 });

const filters = ref({
  direction: null as string | null,
  minScore: null as number | null,
  includeExpired: false,
});

const directionOptions = [
  { label: '看涨', value: 'bullish' },
  { label: '看跌', value: 'bearish' },
  { label: '中性', value: 'neutral' },
];

const columns = [
  { name: 'title', label: '事件', field: 'title', align: 'left' as const },
  { name: 'direction', label: '方向', field: 'direction', align: 'left' as const },
  { name: 'impact_score', label: '影响分', field: 'impact_score', align: 'center' as const, sortable: false },
  { name: 'sentiment', label: '情感', field: 'sentiment_label', align: 'left' as const },
  { name: 'affected_count', label: '映射', field: 'affected_count', align: 'center' as const },
  { name: 'ttl', label: 'TTL', field: 'expires_at', align: 'left' as const },
  { name: 'occurred_at', label: '发生时间', field: 'occurred_at', align: 'left' as const },
  { name: 'actions', label: '操作', field: 'impact_id', align: 'left' as const },
];

async function fetchData() {
  loading.value = true;
  try {
    const res = await miaImpactService.list({
      page: pagination.value.page,
      pageSize: pagination.value.rowsPerPage,
      direction: filters.value.direction ?? undefined,
      minScore: filters.value.minScore ?? undefined,
      includeExpired: filters.value.includeExpired,
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

function goDetail(row: ImpactCardListItem) {
  if (!row.impact_id) return;
  router.push({ name: 'media-mia-impact-detail', params: { id: row.impact_id } });
}

onMounted(fetchData);
</script>
