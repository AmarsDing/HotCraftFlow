<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MIA 资产映射</div>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <q-banner dense rounded class="bg-blue-1 text-blue-10 q-mb-md">
      <template #avatar><q-icon name="edit_note" /></template>
      映射表是长期运营资产，必须非技术可维护——notes 必填（无理由的映射不可信）。CSV 按唯一键幂等 upsert，行级错误整批不中断。
    </q-banner>

    <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary" class="q-mb-md">
      <q-tab name="entity" label="表 A：实体 → 标的" />
      <q-tab name="narrative" label="表 B：叙事 → 币种" />
    </q-tabs>

    <!-- ==================== 表 A ==================== -->
    <template v-if="tab === 'entity'">
      <div class="row q-col-gutter-sm q-mb-md items-center">
        <div class="col-12 col-sm-3">
          <q-input v-model="fA.keyword" outlined dense label="关键词（实体/别名/标的）" clearable @keyup.enter="onFilterChangeA" />
        </div>
        <div class="col-6 col-sm-2">
          <q-select v-model="fA.entityType" outlined dense emit-value map-options clearable :options="entityTypeOptions" label="实体类型" @update:model-value="onFilterChangeA" />
        </div>
        <div class="col-6 col-sm-2">
          <q-select v-model="fA.assetClass" outlined dense emit-value map-options clearable :options="assetClassOptions" label="资产类别" @update:model-value="onFilterChangeA" />
        </div>
        <div class="col-6 col-sm-2">
          <q-toggle v-model="fA.enabledOnly" label="仅看启用" @update:model-value="onFilterChangeA" />
        </div>
        <div class="col-auto">
          <q-btn unelevated dense no-caps color="primary" icon="add" label="新增" class="q-mr-sm" @click="openA()" />
          <q-btn outline dense no-caps color="primary" icon="upload_file" label="CSV 导入" class="q-mr-sm" @click="pickCsv('entity')" />
          <q-btn outline dense no-caps color="primary" icon="download" label="导出" @click="doExportCsv('entity')" />
        </div>
      </div>

      <q-table
        flat bordered :rows="rowsA" :columns="columnsA" row-key="id" :loading="loading"
        v-model:pagination="pgA" :rows-per-page-options="[50, 100]" @request="onRequestA"
      >
        <template #body-cell-alias="props">
          <q-td :props="props">
            <span class="text-caption">{{ (props.row.entity_alias ?? []).join('；') || '-' }}</span>
          </q-td>
        </template>
        <template #body-cell-entity_type="props">
          <q-td :props="props">{{ entityTypeLabel(props.row.entity_type) }}</q-td>
        </template>
        <template #body-cell-asset_class="props">
          <q-td :props="props">{{ assetClassLabel(props.row.asset_class) }}</q-td>
        </template>
        <template #body-cell-direction_bias="props">
          <q-td :props="props">
            <q-badge :color="directionMeta(props.row.direction_bias).color">{{ directionMeta(props.row.direction_bias).label }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-notes="props">
          <q-td :props="props">
            <span class="text-caption ellipsis" style="max-width: 240px; display: inline-block">{{ props.row.notes }}</span>
          </q-td>
        </template>
        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-badge :color="props.row.enabled ? 'positive' : 'grey-5'">{{ props.row.enabled ? '启用' : '停用' }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn flat dense no-caps color="primary" icon="edit" label="编辑" @click="openA(props.row)" />
            <q-btn flat dense no-caps color="negative" icon="delete" label="删除" @click="doDeleteA(props.row)" />
          </q-td>
        </template>
        <template #no-data>
          <div class="q-pa-lg text-grey-7">表 A 为空——用「CSV 导入」灌入出厂种子（≥200 条）。</div>
        </template>
      </q-table>
    </template>

    <!-- ==================== 表 B ==================== -->
    <template v-else>
      <div class="row q-col-gutter-sm q-mb-md items-center">
        <div class="col-12 col-sm-3">
          <q-input v-model="fB.keyword" outlined dense label="关键词（叙事/关键词/币种）" clearable @keyup.enter="onFilterChangeB" />
        </div>
        <div class="col-6 col-sm-2">
          <q-toggle v-model="fB.enabledOnly" label="仅看启用" @update:model-value="onFilterChangeB" />
        </div>
        <div class="col-auto">
          <q-btn unelevated dense no-caps color="primary" icon="add" label="新增" class="q-mr-sm" @click="openB()" />
          <q-btn outline dense no-caps color="primary" icon="upload_file" label="CSV 导入" class="q-mr-sm" @click="pickCsv('narrative')" />
          <q-btn outline dense no-caps color="primary" icon="download" label="导出" @click="doExportCsv('narrative')" />
        </div>
      </div>

      <q-table
        flat bordered :rows="rowsB" :columns="columnsB" row-key="id" :loading="loading"
        v-model:pagination="pgB" :rows-per-page-options="[50, 100]" @request="onRequestB"
      >
        <template #body-cell-keywords="props">
          <q-td :props="props">
            <span class="text-caption">{{ (props.row.keywords ?? []).join('；') }}</span>
          </q-td>
        </template>
        <template #body-cell-coins="props">
          <q-td :props="props">
            <q-badge v-for="c in props.row.coins ?? []" :key="c" outline color="orange" class="q-mr-xs">{{ c }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-hint="props">
          <q-td :props="props">
            <q-badge :color="directionMeta(props.row.default_sentiment_hint).color">{{ directionMeta(props.row.default_sentiment_hint).label }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-enabled="props">
          <q-td :props="props">
            <q-badge :color="props.row.enabled ? 'positive' : 'grey-5'">{{ props.row.enabled ? '启用' : '停用' }}</q-badge>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn flat dense no-caps color="primary" icon="edit" label="编辑" @click="openB(props.row)" />
            <q-btn flat dense no-caps color="negative" icon="delete" label="删除" @click="doDeleteB(props.row)" />
          </q-td>
        </template>
        <template #no-data>
          <div class="q-pa-lg text-grey-7">表 B 为空——用「CSV 导入」灌入出厂种子（≥30 条）。</div>
        </template>
      </q-table>
    </template>

    <!-- 表 A 编辑对话框 -->
    <q-dialog v-model="dlgA" persistent>
      <q-card style="min-width: 520px">
        <q-card-section class="text-h6">{{ formA.id ? '编辑映射' : '新增映射' }}</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input v-model="formA.entity_name" outlined dense label="实体名（如：美联储）" :rules="[(v) => !!v || '实体名必填']" />
          <q-input v-model="formA.entity_alias" outlined dense label="别名（分号 ; 分隔，如：Fed;FOMC）" />
          <div class="row q-col-gutter-md">
            <div class="col-6">
              <q-select v-model="formA.entity_type" outlined dense emit-value map-options :options="entityTypeOptions" label="实体类型" />
            </div>
            <div class="col-6">
              <q-select v-model="formA.asset_class" outlined dense emit-value map-options :options="assetClassOptions" label="资产类别" />
            </div>
          </div>
          <q-input v-model="formA.symbol" outlined dense label="标的（如：SPY / BTC/USDT）" :rules="[(v) => !!v || '标的必填']" />
          <div class="row q-col-gutter-md">
            <div class="col-6">
              <q-select v-model="formA.direction_bias" outlined dense emit-value map-options :options="biasOptions" label="方向倾向" />
            </div>
            <div class="col-6">
              <q-input v-model.number="formA.confidence" outlined dense type="number" min="0" max="1" step="0.01" label="置信度 (0–1)" />
            </div>
          </div>
          <q-input
            v-model="formA.notes" outlined dense type="textarea" autogrow
            label="notes 映射理由（必填——无理由的映射不可信）"
            :rules="[(v) => !!v || 'notes 必填']"
          />
          <q-toggle v-model="formA.enabled" label="启用" color="positive" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="dlgA = false" />
          <q-btn
            unelevated no-caps color="primary" label="保存"
            :disable="!formA.entity_name || !formA.symbol || !formA.notes"
            :loading="saving" @click="doSaveA"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 表 B 编辑对话框 -->
    <q-dialog v-model="dlgB" persistent>
      <q-card style="min-width: 520px">
        <q-card-section class="text-h6">{{ formB.id ? '编辑叙事映射' : '新增叙事映射' }}</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input v-model="formB.narrative" outlined dense label="叙事名（如：比特币减半）" :rules="[(v) => !!v || '叙事名必填']" />
          <q-input v-model="formB.keywords" outlined dense label="关键词（分号 ; 分隔，如：减半;halving）" :rules="[(v) => !!v || '关键词必填']" />
          <q-input v-model="formB.coins" outlined dense label="币种（分号 ; 分隔，如：BTC;LTC）" :rules="[(v) => !!v || '币种必填']" />
          <div class="row q-col-gutter-md">
            <div class="col-6">
              <q-select v-model="formB.default_sentiment_hint" outlined dense emit-value map-options :options="biasOptions" label="默认情感提示" />
            </div>
            <div class="col-6">
              <q-input
                v-model.number="formB.ttl_hours" outlined dense type="number" min="1" max="24"
                label="TTL 小时（1–24，衰减纪律）"
                :rules="[(v) => (v >= 1 && v <= 24) || '1–24 之间']"
              />
            </div>
          </div>
          <q-input v-model.number="formB.confidence" outlined dense type="number" min="0" max="1" step="0.01" label="置信度 (0–1)" />
          <q-toggle v-model="formB.enabled" label="启用" color="positive" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="dlgB = false" />
          <q-btn
            unelevated no-caps color="primary" label="保存"
            :disable="!formB.narrative || !formB.keywords || !formB.coins"
            :loading="saving" @click="doSaveB"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- CSV 导入结果 -->
    <q-dialog v-model="importResultOpen">
      <q-card style="min-width: 480px; max-width: 680px">
        <q-card-section class="text-h6">CSV 导入结果</q-card-section>
        <q-card-section>
          <div class="text-body2 q-mb-sm">
            新建 {{ importResult?.created ?? 0 }} · 更新 {{ importResult?.updated ?? 0 }} · 跳过 {{ importResult?.skipped ?? 0 }}
          </div>
          <q-list v-if="importResult?.errors?.length" bordered separator dense>
            <q-item v-for="(err, i) in importResult.errors" :key="i">
              <q-item-section class="text-negative text-caption">{{ err }}</q-item-section>
            </q-item>
          </q-list>
          <div v-else class="text-positive text-body2">全部行导入成功，零拒绝。</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="关闭" @click="importResultOpen = false" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <input ref="csvInput" type="file" accept=".csv,text/csv" style="display: none" @change="onCsvPicked" />
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { Dialog, Notify } from 'quasar';
import { miaMappingService } from '../../../services/index';
import {
  assetClassLabel, directionMeta, entityTypeLabel, errMessage,
  type EntityAssetMappingItem, type MiaImportCsvResponse, type NarrativeCoinMappingItem,
} from './types';

const tab = ref<'entity' | 'narrative'>('entity');
const loading = ref(false);
const saving = ref(false);

const entityTypeOptions = [
  { label: '人物', value: 'person' },
  { label: '机构', value: 'org' },
  { label: '政策', value: 'policy' },
  { label: '技术', value: 'tech' },
  { label: '地区', value: 'place' },
  { label: '主题', value: 'topic' },
];
const assetClassOptions = [
  { label: '股票', value: 'equity' },
  { label: '指数', value: 'index' },
  { label: '外汇', value: 'fx' },
  { label: '商品', value: 'commodity' },
  { label: '加密', value: 'crypto' },
];
const biasOptions = [
  { label: '看涨', value: 'bullish' },
  { label: '看跌', value: 'bearish' },
  { label: '中性', value: 'neutral' },
];

// ---------- 表 A ----------
const rowsA = ref<EntityAssetMappingItem[]>([]);
const pgA = ref({ page: 1, rowsPerPage: 50, rowsNumber: 0 });
const fA = ref({ keyword: '', entityType: null as string | null, assetClass: null as string | null, enabledOnly: false });

const columnsA = [
  { name: 'entity_name', label: '实体', field: 'entity_name', align: 'left' as const },
  { name: 'alias', label: '别名', field: 'entity_alias', align: 'left' as const },
  { name: 'entity_type', label: '类型', field: 'entity_type', align: 'left' as const },
  { name: 'asset_class', label: '类别', field: 'asset_class', align: 'left' as const },
  { name: 'symbol', label: '标的', field: 'symbol', align: 'left' as const },
  { name: 'direction_bias', label: '倾向', field: 'direction_bias', align: 'left' as const },
  { name: 'confidence', label: '置信', field: 'confidence', align: 'center' as const },
  { name: 'source', label: '来源', field: 'source', align: 'left' as const },
  { name: 'notes', label: '理由', field: 'notes', align: 'left' as const },
  { name: 'enabled', label: '状态', field: 'enabled', align: 'left' as const },
  { name: 'actions', label: '操作', field: 'id', align: 'left' as const },
];

async function fetchA() {
  const res = await miaMappingService.listEntity({
    page: pgA.value.page,
    pageSize: pgA.value.rowsPerPage,
    keyword: fA.value.keyword || undefined,
    entityType: fA.value.entityType ?? undefined,
    assetClass: fA.value.assetClass ?? undefined,
    enabledOnly: fA.value.enabledOnly,
  });
  rowsA.value = res.items ?? [];
  pgA.value.rowsNumber = res.total ?? 0;
}
function onRequestA(props: { pagination: { page: number; rowsPerPage: number } }) {
  pgA.value.page = props.pagination.page;
  pgA.value.rowsPerPage = props.pagination.rowsPerPage;
  fetchData();
}
// 过滤变化：回到第 1 页再查
function onFilterChangeA() {
  pgA.value.page = 1;
  fetchData();
}

const dlgA = ref(false);
const formA = ref({
  id: 0, entity_name: '', entity_alias: '', entity_type: 'org', asset_class: 'equity',
  symbol: '', direction_bias: 'neutral', confidence: 0.6, notes: '', enabled: true,
});

function openA(row?: EntityAssetMappingItem) {
  formA.value = row?.id
    ? {
        id: row.id, entity_name: row.entity_name ?? '', entity_alias: (row.entity_alias ?? []).join(';'),
        entity_type: row.entity_type ?? 'org', asset_class: row.asset_class ?? 'equity',
        symbol: row.symbol ?? '', direction_bias: row.direction_bias ?? 'neutral',
        confidence: row.confidence ?? 0.6, notes: row.notes ?? '', enabled: row.enabled ?? true,
      }
    : { id: 0, entity_name: '', entity_alias: '', entity_type: 'org', asset_class: 'equity', symbol: '', direction_bias: 'neutral', confidence: 0.6, notes: '', enabled: true };
  dlgA.value = true;
}

function splitList(s: string): string[] {
  return s.split(';').map((x) => x.trim()).filter(Boolean);
}

async function doSaveA() {
  saving.value = true;
  try {
    const item: EntityAssetMappingItem = {
      id: formA.value.id || undefined,
      entity_name: formA.value.entity_name.trim(),
      entity_alias: splitList(formA.value.entity_alias),
      entity_type: formA.value.entity_type,
      asset_class: formA.value.asset_class,
      symbol: formA.value.symbol.trim(),
      direction_bias: formA.value.direction_bias,
      confidence: formA.value.confidence,
      source: 'manual',
      notes: formA.value.notes.trim(),
      enabled: formA.value.enabled,
      updated_at: undefined,
    };
    if (formA.value.id) {
      await miaMappingService.updateEntity(formA.value.id, item);
    } else {
      await miaMappingService.createEntity(item);
    }
    Notify.create({ type: 'positive', message: '已保存' });
    dlgA.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    saving.value = false;
  }
}

function doDeleteA(row: EntityAssetMappingItem) {
  Dialog.create({
    title: '删除映射',
    message: `删除「${row.entity_name} → ${row.symbol}」？删除后同实体新事件不再命中该标的。`,
    cancel: { flat: true, noCaps: true },
    ok: { unelevated: true, noCaps: true, color: 'negative', label: '删除' },
    persistent: true,
  }).onOk(async () => {
    try {
      await miaMappingService.deleteEntity(row.id ?? 0);
      Notify.create({ type: 'positive', message: '已删除' });
      await fetchData();
    } catch (e) {
      Notify.create({ type: 'negative', message: errMessage(e) });
    }
  });
}

// ---------- 表 B ----------
const rowsB = ref<NarrativeCoinMappingItem[]>([]);
const pgB = ref({ page: 1, rowsPerPage: 50, rowsNumber: 0 });
const fB = ref({ keyword: '', enabledOnly: false });

const columnsB = [
  { name: 'narrative', label: '叙事', field: 'narrative', align: 'left' as const },
  { name: 'keywords', label: '关键词', field: 'keywords', align: 'left' as const },
  { name: 'coins', label: '币种', field: 'coins', align: 'left' as const },
  { name: 'hint', label: '默认倾向', field: 'default_sentiment_hint', align: 'left' as const },
  { name: 'ttl_hours', label: 'TTL(h)', field: 'ttl_hours', align: 'center' as const },
  { name: 'confidence', label: '置信', field: 'confidence', align: 'center' as const },
  { name: 'enabled', label: '状态', field: 'enabled', align: 'left' as const },
  { name: 'actions', label: '操作', field: 'id', align: 'left' as const },
];

async function fetchB() {
  const res = await miaMappingService.listNarrative({
    page: pgB.value.page,
    pageSize: pgB.value.rowsPerPage,
    keyword: fB.value.keyword || undefined,
    enabledOnly: fB.value.enabledOnly,
  });
  rowsB.value = res.items ?? [];
  pgB.value.rowsNumber = res.total ?? 0;
}
function onRequestB(props: { pagination: { page: number; rowsPerPage: number } }) {
  pgB.value.page = props.pagination.page;
  pgB.value.rowsPerPage = props.pagination.rowsPerPage;
  fetchData();
}
function onFilterChangeB() {
  pgB.value.page = 1;
  fetchData();
}

const dlgB = ref(false);
const formB = ref({
  id: 0, narrative: '', keywords: '', coins: '', default_sentiment_hint: 'neutral',
  ttl_hours: 12, confidence: 0.6, enabled: true,
});

function openB(row?: NarrativeCoinMappingItem) {
  formB.value = row?.id
    ? {
        id: row.id, narrative: row.narrative ?? '', keywords: (row.keywords ?? []).join(';'),
        coins: (row.coins ?? []).join(';'), default_sentiment_hint: row.default_sentiment_hint ?? 'neutral',
        ttl_hours: row.ttl_hours ?? 12, confidence: row.confidence ?? 0.6, enabled: row.enabled ?? true,
      }
    : { id: 0, narrative: '', keywords: '', coins: '', default_sentiment_hint: 'neutral', ttl_hours: 12, confidence: 0.6, enabled: true };
  dlgB.value = true;
}

async function doSaveB() {
  saving.value = true;
  try {
    const item: NarrativeCoinMappingItem = {
      id: formB.value.id || undefined,
      narrative: formB.value.narrative.trim(),
      keywords: splitList(formB.value.keywords),
      coins: splitList(formB.value.coins),
      default_sentiment_hint: formB.value.default_sentiment_hint,
      ttl_hours: formB.value.ttl_hours,
      confidence: formB.value.confidence,
      enabled: formB.value.enabled,
      updated_at: undefined,
    };
    if (formB.value.id) {
      await miaMappingService.updateNarrative(formB.value.id, item);
    } else {
      await miaMappingService.createNarrative(item);
    }
    Notify.create({ type: 'positive', message: '已保存' });
    dlgB.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    saving.value = false;
  }
}

function doDeleteB(row: NarrativeCoinMappingItem) {
  Dialog.create({
    title: '删除叙事映射',
    message: `删除「${row.narrative}」？删除后命中该关键词的事件不再传导到 ${(row.coins ?? []).join('/')}。`,
    cancel: { flat: true, noCaps: true },
    ok: { unelevated: true, noCaps: true, color: 'negative', label: '删除' },
    persistent: true,
  }).onOk(async () => {
    try {
      await miaMappingService.deleteNarrative(row.id ?? 0);
      Notify.create({ type: 'positive', message: '已删除' });
      await fetchData();
    } catch (e) {
      Notify.create({ type: 'negative', message: errMessage(e) });
    }
  });
}

// ---------- CSV 导入/导出 ----------
const csvInput = ref<HTMLInputElement | null>(null);
const csvTarget = ref<'entity' | 'narrative'>('entity');
const importResultOpen = ref(false);
const importResult = ref<MiaImportCsvResponse | null>(null);

function pickCsv(which: 'entity' | 'narrative') {
  csvTarget.value = which;
  csvInput.value?.click();
}

async function onCsvPicked(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = ''; // 允许重复选同一文件
  if (!file) return;
  try {
    const text = await file.text();
    const res = csvTarget.value === 'entity'
      ? await miaMappingService.importEntityCsv(text)
      : await miaMappingService.importNarrativeCsv(text);
    importResult.value = res;
    importResultOpen.value = true;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  }
}

function downloadText(text: string, filename: string) {
  const blob = new Blob([text], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

async function doExportCsv(which: 'entity' | 'narrative') {
  try {
    const csv = which === 'entity'
      ? await miaMappingService.exportEntityCsv()
      : await miaMappingService.exportNarrativeCsv();
    downloadText(csv, which === 'entity' ? 'mia_entity_mapping.csv' : 'mia_narrative_mapping.csv');
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  }
}

async function fetchData() {
  loading.value = true;
  try {
    if (tab.value === 'entity') await fetchA();
    else await fetchB();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

watch(tab, fetchData);
onMounted(fetchData);
</script>
