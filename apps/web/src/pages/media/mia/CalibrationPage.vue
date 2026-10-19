<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MIA 规则校准</div>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <!-- 🔴 固定提示（不可折叠）：校准参考，非预测准确率承诺；样本量 <30 不具统计意义 -->
    <q-banner dense rounded class="bg-amber-1 text-amber-10 q-mb-md">
      <template #avatar><q-icon name="policy" /></template>
      校准数据仅为事后参考，非预测准确率承诺；样本量 &lt; {{ CALIBRATION_MIN_SAMPLES }} 不具统计意义。
      传导规则均为「假设非因果」（confidence ≤ 0.6）。
    </q-banner>

    <!-- 时间窗 -->
    <div class="row q-col-gutter-sm q-mb-md items-center">
      <div class="col-6 col-sm-3">
        <q-input v-model="since" outlined dense type="date" label="起始日" />
      </div>
      <div class="col-6 col-sm-3">
        <q-input v-model="until" outlined dense type="date" label="截止日" />
      </div>
      <div class="col-auto">
        <q-btn outline dense no-caps color="primary" icon="search" label="查询" @click="fetchData" />
      </div>
      <div class="col-auto" v-if="report">
        <q-badge color="grey-8" class="q-ml-sm">窗口 {{ report.window || '-' }}</q-badge>
        <q-badge color="grey-8" class="q-ml-sm">总样本 {{ report.total_samples ?? 0 }}</q-badge>
        <q-badge v-if="report.enabled === false" color="orange" class="q-ml-sm">校准未启用</q-badge>
      </div>
    </div>

    <!-- 校准桶 -->
    <q-card flat bordered class="q-mb-md">
      <q-card-section>
        <div class="text-subtitle2 q-mb-sm">分桶校准（按规则）</div>
        <q-table
          flat bordered
          :rows="report?.buckets ?? []"
          :columns="bucketColumns"
          row-key="rule_id"
          :loading="loading"
          :rows-per-page-options="[50]"
          hide-pagination
        >
          <template #body-cell-samples="props">
            <q-td :props="props">
              {{ props.row.samples ?? 0 }}
              <q-badge
                v-if="(props.row.samples ?? 0) < CALIBRATION_MIN_SAMPLES" color="orange" class="q-ml-xs"
              >样本不足</q-badge>
            </q-td>
          </template>
          <template #body-cell-hit_rate="props">
            <q-td :props="props">
              <span v-if="(props.row.samples ?? 0) >= CALIBRATION_MIN_SAMPLES" class="text-weight-bold">
                {{ ((props.row.hit_rate ?? 0) * 100).toFixed(1) }}%
              </span>
              <!-- 🔴 样本不足不展示命中率百分比（不具统计意义，避免误导） -->
              <span v-else class="text-grey-6">—（{{ ((props.row.hit_rate ?? 0) * 100).toFixed(1) }}% 仅供参考）</span>
            </q-td>
          </template>
          <template #no-data>
            <div class="q-pa-md text-grey-7">
              暂无校准样本——事后验证回填（FR-MIA-09）随验证调度落地后自动累计。
            </div>
          </template>
        </q-table>
        <div v-if="report?.note" class="text-caption text-grey-7 q-mt-sm">{{ report.note }}</div>
      </q-card-section>
    </q-card>

    <!-- 传导规则 -->
    <q-card flat bordered>
      <q-card-section>
        <div class="text-subtitle2 q-mb-sm">跨资产传导规则（每条带 confidence 与 note）</div>
        <q-table
          flat bordered
          :rows="rules"
          :columns="ruleColumns"
          row-key="id"
          :loading="loading"
          :rows-per-page-options="[50]"
          hide-pagination
        >
          <template #body-cell-trigger="props">
            <q-td :props="props">
              <span class="text-body2">
                {{ props.row.rule?.trigger?.asset }}
                <q-badge :color="props.row.rule?.trigger?.direction === 'up' ? 'negative' : 'positive'" class="q-mx-xs">
                  {{ props.row.rule?.trigger?.direction === 'up' ? '↑' : '↓' }}
                </q-badge>
                ≥ {{ ((props.row.rule?.trigger?.min_move_pct ?? 0) * 100).toFixed(1) }}%
              </span>
            </q-td>
          </template>
          <template #body-cell-to="props">
            <q-td :props="props">
              <span class="text-weight-bold">{{ props.row.rule?.to_asset }}</span>
              <q-badge :color="directionMeta(props.row.rule?.to_direction).color" class="q-ml-xs">
                {{ directionMeta(props.row.rule?.to_direction).label }}
              </q-badge>
            </q-td>
          </template>
          <template #body-cell-confidence="props">
            <q-td :props="props">
              {{ props.row.rule?.confidence?.toFixed(2) ?? '-' }}
              <q-badge color="deep-orange" class="q-ml-xs">假设非因果</q-badge>
              <q-badge v-if="(props.row.rule?.confidence ?? 0) > 0.6" color="negative" class="q-ml-xs">
                ⚠ 超 0.6 上限（违规数据）
              </q-badge>
            </q-td>
          </template>
          <template #body-cell-note="props">
            <q-td :props="props">
              <span v-if="props.row.rule?.note" class="text-caption">{{ props.row.rule.note }}</span>
              <!-- 🔴 note 必填，缺失即违规数据（后端入库已硬校验，这里防御性展示） -->
              <span v-else class="text-negative text-caption">⚠ note 缺失（违规数据）</span>
            </q-td>
          </template>
          <template #body-cell-hit="props">
            <q-td :props="props">
              {{ props.row.correct_count ?? 0 }} / {{ props.row.hit_count ?? 0 }}
              <q-badge
                v-if="(props.row.hit_count ?? 0) < CALIBRATION_MIN_SAMPLES" color="orange" class="q-ml-xs"
              >样本不足</q-badge>
            </q-td>
          </template>
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-badge :color="props.row.enabled ? 'positive' : 'grey-5'">
                {{ props.row.enabled ? '启用' : '停用' }}
              </q-badge>
            </q-td>
          </template>
          <template #no-data>
            <div class="q-pa-md text-grey-7">暂无传导规则——启动时 IngestUsecase 应播种 6 条默认规则。</div>
          </template>
        </q-table>
      </q-card-section>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Notify } from 'quasar';
import { miaImpactService } from '../../../services/index';
import {
  directionMeta, errMessage, CALIBRATION_MIN_SAMPLES,
  type CalibrationReport, type TransmissionRuleItem,
} from './types';

const loading = ref(false);
const report = ref<CalibrationReport | null>(null);
const rules = ref<TransmissionRuleItem[]>([]);

// 默认窗：近 30 天（YYYY-MM-DD → ISO 传后端）
function isoDaysAgo(days: number): string {
  const d = new Date(Date.now() - days * 86400000);
  return d.toISOString().slice(0, 10);
}
const since = ref(isoDaysAgo(30));
const until = ref(isoDaysAgo(0));

const bucketColumns = [
  { name: 'rule_id', label: '规则', field: 'rule_id', align: 'left' as const },
  { name: 'samples', label: '样本量', field: 'samples', align: 'center' as const },
  { name: 'hit_rate', label: '命中率', field: 'hit_rate', align: 'center' as const },
];

const ruleColumns = [
  { name: 'rule_id', label: '规则', field: (r: TransmissionRuleItem) => r.rule?.rule_id, align: 'left' as const },
  { name: 'trigger', label: '触发条件', field: (r: TransmissionRuleItem) => r.rule?.trigger?.asset, align: 'left' as const },
  { name: 'to', label: '传导目标', field: (r: TransmissionRuleItem) => r.rule?.to_asset, align: 'left' as const },
  { name: 'confidence', label: '置信度', field: (r: TransmissionRuleItem) => r.rule?.confidence, align: 'left' as const },
  { name: 'note', label: 'note（必填）', field: (r: TransmissionRuleItem) => r.rule?.note, align: 'left' as const },
  { name: 'hit', label: '命中/触发', field: 'correct_count', align: 'center' as const },
  { name: 'enabled', label: '状态', field: 'enabled', align: 'left' as const },
];

async function fetchData() {
  loading.value = true;
  // 两请求独立成败：一端失败不拖垮另一端；失败侧清空避免陈旧数据冒充新鲜查询
  const [repRes, ruleRes] = await Promise.allSettled([
    miaImpactService.getCalibration({
      since: since.value ? `${since.value}T00:00:00Z` : undefined,
      until: until.value ? `${until.value}T23:59:59Z` : undefined,
    }),
    miaImpactService.listTransmissionRules(false),
  ]);
  if (repRes.status === 'fulfilled') {
    report.value = repRes.value;
  } else {
    report.value = null;
    Notify.create({ type: 'negative', message: `校准报告：${errMessage(repRes.reason)}` });
  }
  if (ruleRes.status === 'fulfilled') {
    rules.value = ruleRes.value.items ?? [];
  } else {
    rules.value = [];
    Notify.create({ type: 'negative', message: `传导规则：${errMessage(ruleRes.reason)}` });
  }
  loading.value = false;
}

onMounted(fetchData);
</script>
