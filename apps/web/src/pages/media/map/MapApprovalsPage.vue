<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MAP 审批队列</div>
      <q-badge class="q-ml-sm" color="warning">{{ total }} 待审</q-badge>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <q-banner dense rounded class="bg-orange-1 text-orange-10 q-mb-md">
      <template #avatar><q-icon name="gavel" /></template>
      审批与发布是两个独立操作：此处只审批。通过后仍须到详情页显式发布。🔴 你不能审批自己提交的内容。
    </q-banner>

    <div v-if="loading && items.length === 0" class="row justify-center q-pa-xl">
      <q-spinner-dots size="64px" color="primary" />
    </div>

    <div v-else-if="items.length === 0" class="row justify-center q-pa-xl text-grey-7">
      <div class="text-center">
        <q-icon name="fact_check" size="64px" class="q-mb-sm" />
        <div>审批队列已清空</div>
      </div>
    </div>

    <div v-else class="row q-col-gutter-md">
      <div v-for="u in items" :key="u.unit_id" class="col-12 col-md-6">
        <q-card flat bordered>
          <q-card-section>
            <div class="row items-center">
              <code class="text-subtitle2">{{ u.unit_id }}</code>
              <q-space />
              <q-badge :color="visibilityMeta(u.visibility_default).color" outline>
                <q-icon :name="visibilityMeta(u.visibility_default).icon" class="q-mr-xs" />
                {{ visibilityMeta(u.visibility_default).label }}
              </q-badge>
            </div>
            <div v-if="u.visibility_default !== 'private'" class="text-negative text-caption q-mt-xs">
              ⚠️ 非私密可见性：{{ u.visibility_override_reason || '（无升级原因，发布门必拒）' }}
            </div>
          </q-card-section>

          <q-card-section class="q-pt-none">
            <div class="text-caption text-grey-7">目标平台</div>
            <div class="q-gutter-xs q-mt-xs">
              <q-chip v-for="t in u.targets || []" :key="t" dense outline color="primary">{{ t }}</q-chip>
            </div>
            <div class="row q-mt-sm text-caption text-grey-8 q-gutter-md">
              <span>提交人：<b>{{ u.submitted_by }}</b></span>
              <span>提交时间：{{ fmtTime(u.create_time) }}</span>
              <span v-if="u.scheduled_for">排期：{{ fmtTime(u.scheduled_for) }}（{{ u.schedule_mode }}）</span>
            </div>
          </q-card-section>

          <q-separator />
          <q-card-actions align="right">
            <q-btn flat dense no-caps icon="visibility" label="详情" @click="goDetail(u)" />
            <q-btn
              outline dense no-caps color="negative" icon="close" label="驳回"
              @click="openReject(u)"
            />
            <q-btn
              unelevated dense no-caps color="positive" icon="check" label="通过"
              :disable="isSelf(u)"
              :loading="actingId === u.unit_id"
              @click="confirmApprove(u)"
            >
              <q-tooltip v-if="isSelf(u)">🔴 禁自批：提交人与审批人不得为同一人</q-tooltip>
            </q-btn>
          </q-card-actions>
        </q-card>
      </div>
    </div>

    <div class="row justify-center q-mt-md">
      <q-pagination
        v-model="page"
        :max="maxPage"
        direction-links
        boundary-numbers
        @update:model-value="fetchData"
      />
    </div>

    <!-- 驳回对话框（🔴 reason 必填） -->
    <q-dialog v-model="rejectOpen" persistent>
      <q-card style="min-width: 420px">
        <q-card-section class="text-h6">驳回 {{ rejectTarget?.unit_id }}</q-card-section>
        <q-card-section>
          <q-input
            v-model="rejectReason"
            type="textarea"
            outlined
            autofocus
            label="驳回原因（必填，记入审计）"
            :rules="[(v) => (v && v.trim().length > 0) || '驳回必须填写原因']"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="rejectOpen = false" />
          <q-btn
            unelevated no-caps color="negative" label="确认驳回"
            :disable="!rejectReason.trim()"
            :loading="actingId === rejectTarget?.unit_id"
            @click="doReject"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Dialog, Notify } from 'quasar';
import { mapService } from '../../../services/index';
import { useAuthStore } from '../../../stores/auth';
import {
  errMessage, fmtTime, visibilityMeta,
  type MapDispatchUnitItem,
} from './types';

const router = useRouter();
const authStore = useAuthStore();

const items = ref<MapDispatchUnitItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 20;
const loading = ref(false);
const actingId = ref('');

const maxPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

/** 🔴 禁自批（UI 层；服务端 CanApprove 同源拦截，双保险）。 */
function isSelf(u: MapDispatchUnitItem): boolean {
  return !!authStore.user?.username && u.submitted_by === authStore.user.username;
}

async function fetchData() {
  loading.value = true;
  try {
    const res = await mapService.listApprovalQueue(page.value, pageSize);
    items.value = res.items ?? [];
    total.value = res.total ?? 0;
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

function goDetail(u: MapDispatchUnitItem) {
  router.push(`/media/map/dispatch-units/${u.unit_id}`);
}

/** 审批通过（二次确认，防止误触）。 */
function confirmApprove(u: MapDispatchUnitItem) {
  const nonPrivate = u.visibility_default !== 'private';
  Dialog.create({
    title: '审批通过确认',
    message: nonPrivate
      ? `该单元可见性为 <b class="text-negative">${u.visibility_default}</b>，通过后将允许其进入发布队列。确认？`
      : `确认通过 ${u.unit_id}？通过后仍需显式发布。`,
    html: true,
    cancel: { flat: true, noCaps: true, label: '取消' },
    ok: { unelevated: true, noCaps: true, color: 'positive', label: '通过审批' },
  }).onOk(async () => {
    actingId.value = u.unit_id ?? '';
    try {
      await mapService.approveDispatchUnit(u.unit_id ?? '');
      Notify.create({ type: 'positive', message: '已通过审批' });
      await fetchData();
    } catch (e) {
      // MAP_SELF_APPROVAL / MAP_INVALID_STATE 等按 reason 提示
      Notify.create({ type: 'negative', message: errMessage(e) });
    } finally {
      actingId.value = '';
    }
  });
}

const rejectOpen = ref(false);
const rejectTarget = ref<MapDispatchUnitItem | null>(null);
const rejectReason = ref('');

function openReject(u: MapDispatchUnitItem) {
  rejectTarget.value = u;
  rejectReason.value = '';
  rejectOpen.value = true;
}

async function doReject() {
  const u = rejectTarget.value;
  if (!u) return;
  actingId.value = u.unit_id ?? '';
  try {
    await mapService.rejectDispatchUnit(u.unit_id ?? '', rejectReason.value.trim());
    Notify.create({ type: 'positive', message: '已驳回' });
    rejectOpen.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    actingId.value = '';
  }
}

onMounted(fetchData);
</script>
