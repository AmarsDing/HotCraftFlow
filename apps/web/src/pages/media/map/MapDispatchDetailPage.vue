<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <q-btn flat dense round icon="arrow_back" @click="router.back()" />
      <div class="text-h5 q-ml-sm">分发单元详情</div>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <div v-if="loading && !unit" class="row justify-center q-pa-xl">
      <q-spinner-dots size="64px" color="primary" />
    </div>

    <div v-else-if="!unit" class="row justify-center q-pa-xl text-grey-7">未找到该分发单元</div>

    <template v-else>
      <!-- 🔴 非私密可见性警示条（NFR-MAP-01） -->
      <q-banner
        v-if="unit.visibility_default !== 'private'"
        dense rounded class="bg-red-1 text-negative q-mb-md"
      >
        <template #avatar><q-icon name="warning" color="negative" /></template>
        非私密可见性（{{ unit.visibility_default }}）。发布时必须二次确认；升级原因：{{ unit.visibility_override_reason || '缺失' }}
      </q-banner>

      <!-- 状态时间线 -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section>
          <div class="row items-center q-gutter-md">
            <div class="text-subtitle2"><code>{{ unit.unit_id }}</code></div>
            <q-space />
            <q-badge :color="approvalStateColor(unit.approval_state)">
              审批：{{ unit.approval_state || 'draft' }}
            </q-badge>
            <q-badge :color="publishStateColor(unit.publish_state)">
              发布：{{ unit.publish_state || 'pending' }}
            </q-badge>
            <q-badge :color="visibilityMeta(unit.visibility_default).color" outline>
              {{ visibilityMeta(unit.visibility_default).label }}
            </q-badge>
          </div>
          <div class="row text-caption text-grey-8 q-gutter-md q-mt-sm">
            <span>提交人：<b>{{ unit.submitted_by }}</b></span>
            <span v-if="unit.approver">审批人：<b>{{ unit.approver }}</b>（{{ fmtTime(unit.approved_at) }}）</span>
            <span v-if="unit.reject_reason" class="text-negative">驳回原因：{{ unit.reject_reason }}</span>
            <span v-if="unit.scheduled_for">排期：{{ fmtTime(unit.scheduled_for) }}（{{ unit.schedule_mode }}）</span>
          </div>
          <div class="q-gutter-xs q-mt-sm">
            <q-chip v-for="t in unit.targets || []" :key="t" dense outline color="primary">{{ t }}</q-chip>
          </div>
        </q-card-section>

        <q-separator />
        <q-card-actions align="right">
          <q-btn
            unelevated dense no-caps color="primary" icon="publish" label="发布"
            :disable="!canPublish"
            :loading="acting === 'publish'"
            @click="openPublishConfirm"
          >
            <q-tooltip v-if="!canPublish">{{ publishDisabledReason }}</q-tooltip>
          </q-btn>
          <q-btn
            outline dense no-caps color="negative" icon="cancel" label="取消单元"
            :disable="unit.publish_state === 'published'"
            :loading="acting === 'cancel'"
            @click="openCancel"
          />
        </q-card-actions>
      </q-card>

      <!-- 发布回执 -->
      <q-card flat bordered class="q-mb-md">
        <q-card-section class="text-subtitle2">发布回执（{{ receipts.length }}）</q-card-section>
        <q-card-section v-if="receipts.length === 0" class="q-pt-none text-grey-7">
          尚无回执——发布后每个目标平台生成一条（post_id / 可见性 / 操作者）。
        </q-card-section>
        <q-list v-else separator>
          <q-item v-for="r in receipts" :key="r.receipt_id">
            <q-item-section>
              <q-item-label>
                <b>{{ r.platform_key }}</b>
                <code class="q-ml-sm">{{ r.post_id }}</code>
              </q-item-label>
              <q-item-label caption>
                可见性 {{ r.visibility }} · {{ fmtTime(r.published_at) }} · 操作者 {{ r.operator }}
              </q-item-label>
            </q-item-section>
            <q-item-section v-if="r.post_url" side>
              <q-btn flat dense round icon="open_in_new" type="a" :href="r.post_url" target="_blank" />
            </q-item-section>
          </q-item>
        </q-list>
      </q-card>

      <!-- 审批记录（审计：actor/action/reason/ip） -->
      <q-card flat bordered>
        <q-card-section class="text-subtitle2">审批与操作记录（{{ records.length }}）</q-card-section>
        <q-table
          flat dense
          :rows="records"
          :columns="recordColumns"
          row-key="id"
          :rows-per-page-options="[20]"
          :pagination="{ rowsPerPage: 20 }"
        >
          <template #body-cell-action="props">
            <q-td :props="props"><q-badge outline>{{ props.row.action }}</q-badge></q-td>
          </template>
          <template #body-cell-states="props">
            <q-td :props="props">{{ props.row.from_state || '-' }} → {{ props.row.to_state || '-' }}</q-td>
          </template>
          <template #body-cell-result="props">
            <q-td :props="props">
              <q-badge :color="props.row.result === 'deny' ? 'negative' : 'positive'" outline>
                {{ props.row.result || '-' }}
              </q-badge>
            </q-td>
          </template>
          <template #body-cell-create_time="props">
            <q-td :props="props">{{ fmtTime(props.row.create_time) }}</q-td>
          </template>
        </q-table>
      </q-card>
    </template>

    <!-- 发布二次确认对话框（AdR-MAP-03：approve≠publish；非 private 必须勾选确认） -->
    <q-dialog v-model="publishOpen" persistent>
      <q-card style="min-width: 440px">
        <q-card-section class="text-h6">发布确认</q-card-section>
        <q-card-section>
          <div class="q-mb-sm">
            目标：<q-chip v-for="t in unit?.targets || []" :key="t" dense outline>{{ t }}</q-chip>
          </div>
          <div v-if="isNonPrivate" class="text-negative q-mb-sm">
            🔴 可见性为 <b>{{ unit?.visibility_default }}</b>（非私密），发布即对外可见。
          </div>
          <q-checkbox
            v-model="confirmPublic"
            :label="isNonPrivate ? '我确认以非私密可见性发布（二次确认，记入审计）' : '确认发布（发布与审批是两个独立操作）'"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="publishOpen = false" />
          <q-btn
            unelevated no-caps color="primary" label="执行发布"
            :disable="isNonPrivate && !confirmPublic"
            :loading="acting === 'publish'"
            @click="doPublish"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 取消单元 -->
    <q-dialog v-model="cancelOpen" persistent>
      <q-card style="min-width: 420px">
        <q-card-section class="text-h6">取消分发单元</q-card-section>
        <q-card-section>
          <q-input v-model="cancelReason" outlined label="取消原因（可选，记入审计）" type="textarea" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="返回" @click="cancelOpen = false" />
          <q-btn
            unelevated no-caps color="negative" label="确认取消"
            :loading="acting === 'cancel'"
            @click="doCancel"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { Notify } from 'quasar';
import { mapService } from '../../../services/index';
import {
  approvalStateColor, errMessage, fmtTime, publishStateColor, visibilityMeta,
  type MapApprovalRecordItem, type MapDispatchUnitItem, type MapReceipt,
} from './types';

const route = useRoute();
const router = useRouter();

const unit = ref<MapDispatchUnitItem | null>(null);
const records = ref<MapApprovalRecordItem[]>([]);
const receipts = ref<MapReceipt[]>([]);
const loading = ref(false);
const acting = ref('');

const recordColumns = [
  { name: 'id', label: 'ID', field: 'id', align: 'left' as const },
  { name: 'action', label: '动作', field: 'action', align: 'left' as const },
  { name: 'states', label: '状态迁移', field: 'from_state', align: 'left' as const },
  { name: 'actor', label: '操作者', field: 'actor', align: 'left' as const },
  { name: 'reason', label: '原因', field: 'reason', align: 'left' as const },
  { name: 'visibility_at_action', label: '当时可见性', field: 'visibility_at_action', align: 'left' as const },
  { name: 'ip', label: 'IP', field: 'ip', align: 'left' as const },
  { name: 'result', label: '结果', field: 'result', align: 'left' as const },
  { name: 'create_time', label: '时间', field: 'create_time', align: 'left' as const },
];

const isNonPrivate = computed(() => !!unit.value && unit.value.visibility_default !== 'private');

const canPublish = computed(() =>
  !!unit.value
  && unit.value.approval_state === 'approved'
  && ['pending', 'failed'].includes(unit.value.publish_state || 'pending'),
);

const publishDisabledReason = computed(() => {
  if (!unit.value) return '';
  if (unit.value.approval_state !== 'approved') return '审批门：单元未获批准（草稿/驳回须先通过审批）';
  if (unit.value.publish_state === 'published') return '已发布（幂等：重复发布会被服务端拒绝）';
  if (unit.value.publish_state === 'publishing') return '发布进行中';
  return '不可发布';
});

async function fetchData() {
  const id = String(route.params.id || '');
  if (!id) return;
  loading.value = true;
  try {
    const res = await mapService.getDispatchUnit(id);
    unit.value = res.unit ?? null;
    records.value = res.records ?? [];
    receipts.value = res.receipts ?? [];
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

const publishOpen = ref(false);
const confirmPublic = ref(false);

function openPublishConfirm() {
  confirmPublic.value = false;
  publishOpen.value = true;
}

async function doPublish() {
  const id = unit.value?.unit_id;
  if (!id) return;
  acting.value = 'publish';
  try {
    // 🔴 非 private 必须 confirm_public=true（服务端双校验）。
    const res = await mapService.publishDispatchUnit(id, confirmPublic.value);
    Notify.create({
      type: 'positive',
      message: res.warnings?.length
        ? `已发布（${res.publish_state}），注意：${res.warnings.join('；')}`
        : `发布结果：${res.publish_state}`,
    });
    publishOpen.value = false;
    await fetchData();
  } catch (e) {
    // MAP_NOT_APPROVED / PUBLIC_CONFIRM_REQUIRED / QUOTA_EXCEEDED / CREDENTIAL_INVALID 等如实提示
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    acting.value = '';
  }
}

const cancelOpen = ref(false);
const cancelReason = ref('');

function openCancel() {
  cancelReason.value = '';
  cancelOpen.value = true;
}

async function doCancel() {
  const id = unit.value?.unit_id;
  if (!id) return;
  acting.value = 'cancel';
  try {
    await mapService.cancelDispatchUnit(id, cancelReason.value.trim());
    Notify.create({ type: 'positive', message: '已取消' });
    cancelOpen.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    acting.value = '';
  }
}

onMounted(fetchData);
</script>
