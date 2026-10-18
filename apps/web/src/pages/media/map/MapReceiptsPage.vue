<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MAP 发布回执</div>
      <q-badge class="q-ml-sm" color="grey">{{ total }} 条</q-badge>
      <q-space />
      <q-btn
        outline dense no-caps color="primary" icon="download" label="导出 CSV"
        class="q-mr-sm"
        :disable="items.length === 0"
        @click="exportCsv"
      />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <q-table
      flat bordered
      :rows="items"
      :columns="columns"
      row-key="receipt_id"
      :loading="loading"
      :rows-per-page-options="[20, 50]"
      :pagination="{ rowsPerPage: 20 }"
    >
      <template #body-cell-visibility="props">
        <q-td :props="props">
          <q-badge :color="visibilityMeta(props.row.visibility).color" outline>
            {{ visibilityMeta(props.row.visibility).label }}
          </q-badge>
        </q-td>
      </template>
      <template #body-cell-post="props">
        <q-td :props="props">
          <code>{{ props.row.post_id }}</code>
          <q-btn
            v-if="props.row.post_url"
            flat dense round size="sm" icon="open_in_new"
            type="a" :href="props.row.post_url" target="_blank"
            class="q-ml-xs"
          >
            <q-tooltip>{{ props.row.post_url }}</q-tooltip>
          </q-btn>
        </q-td>
      </template>
      <template #body-cell-published_at="props">
        <q-td :props="props">{{ fmtTime(props.row.published_at) }}</q-td>
      </template>
      <template #body-cell-unit="props">
        <q-td :props="props">
          <a
            v-if="props.row.dispatch_unit_id"
            class="text-primary cursor-pointer"
            @click="goDetail(props.row.dispatch_unit_id)"
          >{{ props.row.dispatch_unit_id }}</a>
          <span v-else>-</span>
        </q-td>
      </template>
      <template #no-data>
        <div class="q-pa-lg text-grey-7">尚无回执——发布成功后每个目标平台生成一条。</div>
      </template>
    </q-table>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { mapService } from '../../../services/index';
import {
  errMessage, fetchAllPages, fmtTime, visibilityMeta,
  type MapReceipt,
} from './types';

const router = useRouter();
const items = ref<MapReceipt[]>([]);
const total = ref(0);
const loading = ref(false);

const columns = [
  { name: 'receipt_id', label: '回执号', field: 'receipt_id', align: 'left' as const },
  { name: 'unit', label: '分发单元', field: 'dispatch_unit_id', align: 'left' as const },
  { name: 'platform_key', label: '平台', field: 'platform_key', align: 'left' as const },
  { name: 'post', label: '平台帖 ID', field: 'post_id', align: 'left' as const },
  { name: 'visibility', label: '可见性', field: 'visibility', align: 'left' as const },
  { name: 'published_at', label: '发布时间', field: 'published_at', align: 'left' as const },
  { name: 'operator', label: '操作者', field: 'operator', align: 'left' as const },
];

async function fetchData() {
  loading.value = true;
  try {
    // 全量拉取以便 CSV 导出与总数徽标一致（pageSize 受服务端上限约束）
    const res = await fetchAllPages((p, ps) => mapService.listReceipts(p, ps));
    items.value = res.items;
    total.value = res.total;
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

function goDetail(unitId: string) {
  router.push(`/media/map/dispatch-units/${unitId}`);
}

/** FR-PUB-07：回执客户端 CSV 导出（字段与回执视图一一对应）。 */
function exportCsv() {
  const header = ['receipt_id', 'dispatch_unit_id', 'account_id', 'platform_key', 'post_id', 'post_url', 'visibility', 'published_at', 'operator'];
  const esc = (v: unknown): string => {
    const s = v == null ? '' : String(v);
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const lines = [header.join(',')];
  for (const r of items.value) {
    lines.push([
      r.receipt_id, r.dispatch_unit_id, r.account_id, r.platform_key,
      r.post_id, r.post_url, r.visibility, r.published_at, r.operator,
    ].map(esc).join(','));
  }
  // 前置 BOM 保证 Excel 中文/utf-8 正常（用显式转义，避免源码内嵌不可见字符被 lint 判违规）
  const blob = new Blob(['\uFEFF' + lines.join('\n')], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '');
  a.href = url;
  a.download = `map-receipts-${stamp}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  Notify.create({ type: 'positive', message: `已导出 ${items.value.length} 条回执` });
}

onMounted(fetchData);
</script>
