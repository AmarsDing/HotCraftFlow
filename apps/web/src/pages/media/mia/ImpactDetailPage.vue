<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <q-btn flat dense round icon="arrow_back" class="q-mr-sm" @click="router.back()">
        <q-tooltip>返回</q-tooltip>
      </q-btn>
      <div class="text-h5">影响卡详情</div>
      <q-badge v-if="detail" class="q-ml-sm" color="grey">{{ impactId }}</q-badge>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <!-- 🔴 免责声明固定展示（不可折叠、不可隐藏；详细设计 §8 红线条款） -->
    <q-banner dense rounded class="bg-amber-1 text-amber-10 q-mb-md disclaimer-bar">
      <template #avatar><q-icon name="policy" /></template>
      {{ disclaimerText }}
      <span v-if="disclaimerMissing" class="text-negative text-weight-bold q-ml-sm">
        ⚠ disclaimer 字段缺失，展示兜底文案——违规数据
      </span>
    </q-banner>

    <div v-if="loading && !detail" class="row justify-center q-pa-xl">
      <q-spinner-dots size="48px" color="primary" />
    </div>

    <template v-else-if="detail && card">
      <!-- 事件概要 -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="text-subtitle1 text-weight-bold q-mb-xs">{{ card.event?.title || '-' }}</div>
          <div v-if="card.event?.summary" class="text-body2 text-grey-8 q-mb-sm">{{ card.event.summary }}</div>
          <div class="row q-col-gutter-md text-caption text-grey-7">
            <div>发生时间：{{ fmtTime(card.event?.occurred_at) }}</div>
            <div>来源数：{{ card.event?.sources?.length ?? 0 }}</div>
            <div v-if="card.event?.sources?.length" class="ellipsis" style="max-width: 420px">
              域名：{{ card.event.sources.join('、') }}
            </div>
            <div>创建时间：{{ fmtTime(detail.created_at) }}</div>
            <div v-if="detail.expires_at">
              TTL：{{ ttlRemaining(detail.expires_at).expired ? '已过期' : `剩 ${ttlRemaining(detail.expires_at).text}` }}
            </div>
          </div>
          <div class="row q-gutter-xs q-mt-sm">
            <q-badge v-if="detail.no_mapping" color="orange">未映射到标的</q-badge>
            <q-badge v-if="detail.pushed" color="blue">已推送</q-badge>
            <q-badge v-if="detail.rule_version" color="grey-7">规则版本 {{ detail.rule_version }}</q-badge>
          </div>
        </q-card-section>
      </q-card>

      <!-- 情感 -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="text-subtitle2 q-mb-sm">情感判定</div>
          <div class="row items-center q-gutter-sm">
            <q-badge :color="sentimentMeta(card.sentiment?.label).color" class="text-body2">
              {{ sentimentMeta(card.sentiment?.label).label }}
            </q-badge>
            <span class="text-body2">score {{ card.sentiment?.score?.toFixed(2) ?? '-' }}</span>
            <q-badge outline color="grey-7">引擎：{{ engineLabel(card.sentiment?.engine) }}</q-badge>
            <q-badge outline color="grey-7">语言：{{ card.sentiment?.lang || '-' }}</q-badge>
          </div>
          <q-banner
            v-if="card.sentiment?.engine === 'none' || card.sentiment?.label === 'unknown'"
            dense rounded class="bg-grey-2 text-grey-8 q-mt-sm"
          >
            <template #avatar><q-icon name="block" /></template>
            🔴 情感引擎不可用 → 未产出方向结论（不猜）。本条不含任何方向推断。
          </q-banner>
        </q-card-section>
      </q-card>

      <!-- 四因子面板 -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="row items-center q-mb-sm">
            <div class="text-subtitle2">影响评分（四因子合成）</div>
            <q-space />
            <div class="text-h6 text-primary">{{ fmtScore(card.impact?.score) }}</div>
          </div>
          <div class="row q-col-gutter-md">
            <div v-for="f in factorRows" :key="f.key" class="col-12 col-sm-6 col-md-3">
              <div class="text-caption text-grey-7 q-mb-xs">{{ f.label }}</div>
              <q-linear-progress
                :value="f.value" :color="f.color" track-color="grey-3" rounded size="12px"
              />
              <div class="text-caption q-mt-xs">{{ f.display }}</div>
            </div>
          </div>
          <div class="text-caption text-grey-7 q-mt-sm">
            公式：score = 100 × |情感| × 信源权重 × 相关度 × 时间衰减（algo/mia 纯函数；TTL={{ card.impact?.ttl_hours ?? '-' }}h）
          </div>
        </q-card-section>
      </q-card>

      <!-- 受影响标的 -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="text-subtitle2 q-mb-sm">受影响标的（{{ card.affected?.length ?? 0 }}）</div>
          <q-list v-if="card.affected?.length" separator>
            <q-item v-for="(a, i) in card.affected" :key="i">
              <q-item-section>
                <q-item-label>
                  <span class="text-weight-bold q-mr-sm">{{ a.symbol }}</span>
                  <q-badge outline color="grey-7" class="q-mr-xs">{{ assetClassLabel(a.asset_class) }}</q-badge>
                  <q-badge :color="directionMeta(a.direction).color" class="q-mr-xs">
                    <q-icon :name="directionMeta(a.direction).icon" class="q-mr-xs" />
                    {{ directionMeta(a.direction).label }}
                  </q-badge>
                  <!-- 🔴 传导项：假设非因果（confidence 已限 ≤0.6） -->
                  <q-badge v-if="a.is_transmission" color="deep-orange" class="q-mr-xs">
                    <q-icon name="alt_route" class="q-mr-xs" />假设非因果
                  </q-badge>
                  <q-badge v-if="a.rule_id" outline color="deep-orange">规则 {{ a.rule_id }}</q-badge>
                </q-item-label>
                <q-item-label caption class="q-mt-xs">
                  confidence {{ a.confidence?.toFixed(2) ?? '-' }}
                  <template v-if="a.path && a.path.length">
                    · 路径：
                    <span class="text-grey-8">{{ a.path.join(' → ') }}</span>
                  </template>
                  <template v-else>
                    <!-- 🔴 无 path 不得渲染为结论（违规数据处理） -->
                    <span class="text-negative">· ⚠ 无传导路径——按合规不视为有效结论（数据违约，请联系管理员排查入库校验）</span>
                  </template>
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="text-grey-7 text-body2">
            本卡未映射到任何标的——请维护映射表 A/B 后，同主题新事件将自动命中。
          </div>
        </q-card-section>
      </q-card>

      <!-- 事后验证（校准回填） -->
      <q-card v-if="card.verified?.checked_at" flat bordered class="q-mb-md">
        <q-card-section>
          <div class="text-subtitle2 q-mb-sm">事后验证（校准）</div>
          <div class="row q-col-gutter-md text-body2">
            <div>验证时间：{{ fmtTime(card.verified.checked_at) }}</div>
            <div>实际变动：{{ card.verified.actual_move != null ? `${(card.verified.actual_move * 100).toFixed(2)}%` : '-' }}</div>
            <div>
              命中：
              <q-badge :color="card.verified.hit ? 'positive' : 'negative'">
                {{ card.verified.hit ? '是' : '否' }}
              </q-badge>
            </div>
          </div>
        </q-card-section>
      </q-card>

      <!-- 🔴 底部重申免责声明（固定展示纪律：首尾双保险） -->
      <q-banner dense rounded class="bg-amber-1 text-amber-10 disclaimer-bar">
        <template #avatar><q-icon name="policy" /></template>
        {{ disclaimerText }}
        <span v-if="disclaimerMissing" class="text-negative text-weight-bold q-ml-sm">
          ⚠ disclaimer 字段缺失，展示兜底文案——违规数据
        </span>
      </q-banner>
    </template>

    <q-banner v-else dense rounded class="bg-red-1 text-red-10">
      <template #avatar><q-icon name="error" /></template>
      影响卡不存在或已被清理（TTL 过期后由保留策略回收）。
    </q-banner>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { miaImpactService } from '../../../services/index';
import {
  assetClassLabel, directionMeta, engineLabel, errMessage, fmtScore, fmtTime, sentimentMeta, ttlRemaining,
  MIA_IMPACT_DISCLAIMER_FALLBACK,
  type GetImpactCardResponse,
  type ImpactCard,
} from './types';

const route = useRoute();
const router = useRouter();
const impactId = String(route.params.id ?? '');

const detail = ref<GetImpactCardResponse | null>(null);
const loading = ref(false);

const card = computed<ImpactCard | undefined>(() => detail.value?.card ?? undefined);

// 🔴 免责声明以后端卡片字段为准（契约强制非空）；字段缺失时兜底文案 + 显式违规标记——
// 违约数据不得被兜底掩盖（与本页 pathless affected 的防御性展示纪律一致）。
const disclaimerMissing = computed(() => detail.value != null && !card.value?.disclaimer);
const disclaimerText = computed(() => card.value?.disclaimer || MIA_IMPACT_DISCLAIMER_FALLBACK);

// 四因子：情感取绝对值参与合成，展示时保留原始符号说明
const factorRows = computed(() => {
  const f = card.value?.impact?.factors;
  const fmtPct = (v?: number) => (v == null ? '-' : v.toFixed(3));
  return [
    {
      key: 'sentiment',
      label: '情感强度 |sentiment|',
      value: Math.min(Math.abs(f?.sentiment ?? 0), 1),
      display: fmtPct(f?.sentiment),
      color: (f?.sentiment ?? 0) >= 0 ? 'negative' : 'positive',
    },
    {
      key: 'source_weight',
      label: '信源权重 source_weight',
      value: Math.min(f?.source_weight ?? 0, 1),
      display: fmtPct(f?.source_weight),
      color: 'blue',
    },
    {
      key: 'relevance',
      label: '相关度 top3 relevance',
      value: Math.min(f?.relevance ?? 0, 1),
      display: fmtPct(f?.relevance),
      color: 'purple',
    },
    {
      key: 'decay',
      label: '时间衰减 exp(-age/ttl)',
      value: Math.min(f?.decay ?? 0, 1),
      display: fmtPct(f?.decay),
      color: 'teal',
    },
  ];
});

async function fetchData() {
  if (!impactId) return;
  loading.value = true;
  try {
    detail.value = await miaImpactService.get(impactId);
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
    detail.value = null;
  } finally {
    loading.value = false;
  }
}

onMounted(fetchData);
</script>

<style scoped>
.disclaimer-bar {
  border-left: 4px solid var(--q-warning);
}
</style>
