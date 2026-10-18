<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MAP 账号池</div>
      <q-badge class="q-ml-sm" color="grey">{{ total }} 个账号</q-badge>
      <q-space />
      <q-btn unelevated dense no-caps color="primary" icon="add" label="登记账号" class="q-mr-sm" @click="openCreate" />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
    </div>

    <q-banner dense rounded class="bg-blue-1 text-blue-10 q-mb-md">
      <template #avatar><q-icon name="shield" /></template>
      凭据🔴永不回显（仅有 has_credential 标志）。凭据失效请走「重新授权」——既不清凭据也不建重复账号。
    </q-banner>

    <q-table
      flat bordered
      :rows="items"
      :columns="columns"
      row-key="account_id"
      :loading="loading"
      :rows-per-page-options="[20]"
      :pagination="{ rowsPerPage: 20 }"
    >
      <template #body-cell-health="props">
        <q-td :props="props">
          <q-badge :color="healthMeta(props.row.health).color">
            <q-icon :name="healthMeta(props.row.health).icon" class="q-mr-xs" />
            {{ healthMeta(props.row.health).label }}
          </q-badge>
        </q-td>
      </template>
      <template #body-cell-paused_until="props">
        <q-td :props="props">
          <span v-if="props.row.health === 'paused' && props.row.paused_until">
            恢复于 {{ fmtTime(props.row.paused_until) }}
          </span>
          <span v-else>-</span>
        </q-td>
      </template>
      <template #body-cell-last_error="props">
        <q-td :props="props">
          <span v-if="props.row.last_error" class="text-negative text-caption">{{ props.row.last_error }}</span>
          <span v-else>-</span>
        </q-td>
      </template>
      <template #body-cell-credential="props">
        <q-td :props="props">
          <span v-if="props.row.has_credential">✅ 已存（{{ props.row.credential_type }}）</span>
          <span v-else class="text-grey-6">未持凭据</span>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-toggle
            :model-value="props.row.enabled"
            dense
            color="positive"
            checked-icon="check"
            unchecked-icon="close"
            :disable="togglingId === props.row.account_id"
            @update:model-value="(v) => toggle(props.row, Boolean(v))"
          >
            <q-tooltip>{{ props.row.enabled ? '停用账号' : '启用账号' }}</q-tooltip>
          </q-toggle>
          <q-btn
            v-if="props.row.health === 'credential_invalid'"
            flat dense no-caps color="negative" icon="key" label="重新授权"
            @click="openReauth(props.row)"
          />
        </q-td>
      </template>
      <template #no-data>
        <div class="q-pa-lg text-grey-7">账号池为空——点击「登记账号」添加第一个发布账号。</div>
      </template>
    </q-table>

    <!-- 重新授权（credential_invalid）：凭据一次传输即被服务端清零 -->
    <q-dialog v-model="reauthOpen" persistent>
      <q-card style="min-width: 440px">
        <q-card-section class="text-h6">重新授权 {{ reauthTarget?.display_name }}</q-card-section>
        <q-card-section>
          <q-select
            v-model="reauthType" outlined dense class="q-mb-md"
            :options="['oauth', 'cookie', 'api_key']" label="凭据类型"
          />
          <q-input
            v-model="reauthCred" outlined type="password"
            label="新凭据（token / cookie / api-key）"
            hint="🔴 仅经 TLS 传输一次，服务端加密入库；本页不回显、不留存"
            :rules="[(v) => !!v || '请粘贴新凭据']"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="reauthOpen = false" />
          <q-btn
            unelevated no-caps color="primary" label="提交新凭据"
            :disable="!reauthCred"
            :loading="actingId === reauthTarget?.account_id"
            @click="doReauth"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 登记账号 -->
    <q-dialog v-model="createOpen" persistent>
      <q-card style="min-width: 460px">
        <q-card-section class="text-h6">登记发布账号</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-select
            v-model="createForm.channel_id" outlined dense emit-value map-options
            :options="channelOptions" label="所属渠道（platform:slot）"
            :rules="[(v) => !!v || '渠道必选']"
          />
          <q-input v-model="createForm.display_name" outlined dense label="显示名称（脱敏友好，勿含账号密码）" />
          <q-select
            v-model="createForm.credential_type" outlined dense
            :options="['oauth', 'cookie', 'api_key']" label="凭据类型"
          />
          <q-input
            v-model="createForm.credential" outlined type="password"
            label="凭据明文（只在本次传输中出现一次）"
            :rules="[(v) => !!v || '凭据必填']"
          />
          <q-toggle v-model="createForm.enabled" label="登记后启用" color="positive" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="createOpen = false" />
          <q-btn
            unelevated no-caps color="primary" label="登记"
            :disable="!createForm.channel_id || !createForm.credential"
            :loading="creating"
            @click="doCreate"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { Notify } from 'quasar';
import { mapService } from '../../../services/index';
import {
  errMessage, fetchAllPages, fmtTime, healthMeta,
  type MapAccount,
} from './types';

const items = ref<MapAccount[]>([]);
const total = ref(0);
const loading = ref(false);
const actingId = ref('');
const togglingId = ref('');

const columns = [
  { name: 'display_name', label: '名称', field: 'display_name', align: 'left' as const },
  { name: 'platform', label: '平台', field: 'platform_key', align: 'left' as const },
  { name: 'slot', label: '槽位', field: 'slot', align: 'left' as const },
  { name: 'health', label: '健康', field: 'health', align: 'left' as const },
  { name: 'consecutive_failures', label: '连败', field: 'consecutive_failures', align: 'center' as const },
  { name: 'paused_until', label: '熔断恢复', field: 'paused_until', align: 'left' as const },
  { name: 'published_24h', label: '24h 发布', field: 'published_24h', align: 'center' as const },
  { name: 'credential', label: '凭据', field: 'credential_type', align: 'left' as const },
  { name: 'last_error', label: '最近错误（已脱敏）', field: 'last_error', align: 'left' as const },
  { name: 'actions', label: '操作', field: 'account_id', align: 'left' as const },
];

async function fetchData() {
  loading.value = true;
  try {
    const res = await fetchAllPages((p, ps) => mapService.listAccounts(p, ps));
    items.value = res.items;
    total.value = res.total;
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

async function toggle(a: MapAccount, enabled: boolean) {
  togglingId.value = a.account_id ?? '';
  try {
    await mapService.toggleAccount(a.account_id ?? '', enabled);
    a.enabled = enabled;
    Notify.create({ type: 'positive', message: enabled ? '已启用' : '已停用' });
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    togglingId.value = '';
  }
}

const reauthOpen = ref(false);
const reauthTarget = ref<MapAccount | null>(null);
const reauthType = ref<'oauth' | 'cookie' | 'api_key'>('oauth');
const reauthCred = ref('');

function openReauth(a: MapAccount) {
  reauthTarget.value = a;
  reauthType.value = (a.credential_type as 'oauth' | 'cookie' | 'api_key') || 'oauth';
  reauthCred.value = '';
  reauthOpen.value = true;
}

async function doReauth() {
  const a = reauthTarget.value;
  if (!a) return;
  actingId.value = a.account_id ?? '';
  try {
    await mapService.reauthAccount(a.account_id ?? '', reauthType.value, reauthCred.value);
    reauthCred.value = ''; // 🔴 本地即刻丢弃明文
    Notify.create({ type: 'positive', message: '重新授权完成，账号恢复健康' });
    reauthOpen.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    actingId.value = '';
  }
}

const createOpen = ref(false);
const creating = ref(false);
const channelOptions = ref<{ label: string; value: string }[]>([]);
const createForm = ref({
  channel_id: '',
  display_name: '',
  credential_type: 'oauth' as 'oauth' | 'cookie' | 'api_key',
  credential: '',
  enabled: true,
});

async function openCreate() {
  createForm.value = { channel_id: '', display_name: '', credential_type: 'oauth', credential: '', enabled: true };
  createOpen.value = true;
  try {
    const res = await fetchAllPages((p, ps) => mapService.listChannels(p, ps));
    channelOptions.value = res.items
      .filter((c) => c.enabled)
      .map((c) => ({
        label: `${c.platform_key}:${c.slot}${c.region ? `（${c.region}）` : ''}`,
        value: c.channel_id ?? '',
      }))
      .filter((o) => o.value);
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  }
}

async function doCreate() {
  creating.value = true;
  try {
    await mapService.createAccount({
      channel_id: createForm.value.channel_id,
      display_name: createForm.value.display_name,
      credential_type: createForm.value.credential_type,
      credential: createForm.value.credential,
      enabled: createForm.value.enabled,
    });
    createForm.value.credential = ''; // 🔴 本地即刻丢弃明文
    Notify.create({ type: 'positive', message: '账号已登记（凭据已加密入库）' });
    createOpen.value = false;
    await fetchData();
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    creating.value = false;
  }
}

onMounted(fetchData);
</script>
