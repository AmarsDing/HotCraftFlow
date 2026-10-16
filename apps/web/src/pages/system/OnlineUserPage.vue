<template>
  <q-page class="crud-page q-pa-lg">
    <q-card flat class="crud-table-card">
      <q-card-section class="row items-center q-pb-sm q-gutter-sm">
        <div class="text-h6">{{ t('session.title') }}</div>
        <q-badge class="session-stats-badge" outline>
          {{ t('session.sessionStats', { sessions: rows.length, users: uniqueUserCount }) }}
        </q-badge>
        <q-space />
        <span class="session-auto-refresh">
          <q-icon name="schedule" size="14px" />
          {{ t('session.autoRefreshHint') }}
        </span>
        <q-input
          v-model="keyword"
          dense
          outlined
          clearable
          class="app-filter-bar__field app-filter-bar__field--lg"
          :placeholder="t('session.filterPlaceholder')"
        >
          <template #prepend>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-btn flat dense round icon="refresh" :loading="loading" @click="loadList">
          <q-tooltip>{{ t('common.refresh') }}</q-tooltip>
        </q-btn>
      </q-card-section>

      <q-separator />

      <q-table
        :rows="filteredRows"
        :columns="columns"
        row-key="token"
        :loading="loading"
        :pagination="{ rowsPerPage: 20 }"
        flat
      >
        <!-- 默认单元格：空值归一 '-'、ISO 时间自动转本地、截断悬停全文 -->
        <template #body-cell="slotData">
          <q-td :props="slotData" :title="qTableCellText(slotData, false)">
            {{ qTableCellText(slotData, true) }}
          </q-td>
        </template>

        <template #body-cell-username="slotData">
          <q-td :props="slotData" :title="slotData.row.username">
            {{ slotData.row.username || '-' }}
            <q-badge v-if="isCurrentSession(slotData.row)" color="primary" class="q-ml-xs">
              {{ t('session.currentSession') }}
            </q-badge>
          </q-td>
        </template>

        <template #body-cell-deptName="slotData">
          <q-td :props="slotData" :title="deptNameMap[slotData.row.deptId]">
            {{ deptNameMap[slotData.row.deptId] || '-' }}
          </q-td>
        </template>

        <!-- 客户端列：解析 UA 摘要展示（浏览器 · 操作系统），完整 UA 悬停可见 -->
        <template #body-cell-userAgent="slotData">
          <q-td :props="slotData" :title="slotData.row.userAgent || ''">
            {{ uaSummary(slotData.row.userAgent) }}
          </q-td>
        </template>

        <template #body-cell-actions="slotData">
          <q-td auto-width class="text-center">
            <div class="crud-actions">
              <q-btn
                class="crud-actions__btn crud-actions__btn--danger"
                flat
                dense
                round
                icon="power_off"
                :disable="isCurrentUser(slotData.row)"
              >
                <q-tooltip>
                  {{ isCurrentUser(slotData.row) ? t('session.selfOfflineForbidden') : t('session.forceOffline') }}
                </q-tooltip>
                <q-popup-confirm v-if="!isCurrentUser(slotData.row)" @confirm="onForceOffline(slotData.row)">
                  {{ t('session.forceOfflineConfirm') }}
                </q-popup-confirm>
              </q-btn>
            </div>
          </q-td>
        </template>
      </q-table>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import { sessionService, type OnlineUserEntity } from '../../services';
import { useAuthStore, useDepartmentStore } from '../../stores';
import { qTableCellText, withAutoColWidths } from '../../utils/table-columns';

const { t } = useI18n();
const auth = useAuthStore();
const departmentStore = useDepartmentStore();

const rows = ref<OnlineUserEntity[]>([]);
const loading = ref(false);
const keyword = ref('');
// deptId → 部门名称映射，用于在线用户表格展示部门列（后端仅返回 deptId）
const deptNameMap = ref<Record<number, string>>({});

const columns = computed<QTableColumn[]>(() =>
  withAutoColWidths([
    { name: 'username', label: t('session.username'), field: 'username', align: 'left', sortable: true },
    { name: 'nickname', label: t('session.nickname'), field: 'nickname', align: 'left' },
    { name: 'deptName', label: t('session.deptName'), field: 'deptId', align: 'left' },
    { name: 'loginIp', label: t('session.loginIp'), field: 'loginIp', align: 'center' },
    { name: 'loginLocation', label: t('session.loginLocation'), field: 'loginLocation', align: 'center' },
    { name: 'userAgent', label: t('session.userAgent'), field: 'userAgent', align: 'left' },
    { name: 'loginTime', label: t('session.loginTime'), field: 'loginTime', align: 'center', sortable: true },
    { name: 'expiresAt', label: t('session.expiresAt'), field: 'expiresAt', align: 'center', sortable: true },
    { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
  ]),
);

// 在线用户去重计数（同用户多端登录产生多条会话）
const uniqueUserCount = computed(() => new Set(rows.value.map((r) => r.userId)).size);

// 本地即时过滤：用户名 / 昵称 / IP / 位置 / 客户端
const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return rows.value;
  return rows.value.filter((r) =>
    [r.username, r.nickname, r.loginIp, r.loginLocation, r.userAgent].some((v) => v?.toLowerCase().includes(kw)),
  );
});

// 当前浏览器会话的脱敏 token（后端 maskToken 同款：前 8 + **** + 后 4），用于标记“当前”行
const myTokenMask = computed(() => {
  const token = localStorage.getItem('framework_access_token') || '';
  return token.length > 12 ? `${token.slice(0, 8)}****${token.slice(-4)}` : '';
});

function isCurrentSession(row: OnlineUserEntity): boolean {
  return !!myTokenMask.value && row.token === myTokenMask.value;
}

// 强制下线为用户粒度（撤销该用户全部 access token），当前登录账户禁用，避免把自己踢下线
function isCurrentUser(row: OnlineUserEntity): boolean {
  const id = Number(auth.user?.id);
  return !!id && row.userId === id;
}

// UA 摘要：浏览器 · 操作系统；无法识别时截断原文
function uaSummary(ua?: string): string {
  if (!ua) return '-';
  let browser = '';
  if (/Edg\//.test(ua)) browser = 'Edge';
  else if (/OPR\//.test(ua)) browser = 'Opera';
  else if (/Chrome\//.test(ua)) browser = 'Chrome';
  else if (/Firefox\//.test(ua)) browser = 'Firefox';
  else if (/Micromessenger\//.test(ua)) browser = 'WeChat';
  else if (/Version\/.*Safari\//.test(ua)) browser = 'Safari';

  let os = '';
  if (/Windows NT 10/.test(ua)) os = 'Windows 10/11';
  else if (/Windows NT 6\.3/.test(ua)) os = 'Windows 8.1';
  else if (/Windows NT 6\.1/.test(ua)) os = 'Windows 7';
  else if (/Mac OS X/.test(ua)) os = 'macOS';
  else if (/Android/.test(ua)) os = 'Android';
  else if (/iPhone|iPad/.test(ua)) os = 'iOS';
  else if (/Linux/.test(ua)) os = 'Linux';

  if (browser && os) return `${browser} · ${os}`;
  if (browser || os) return browser || os;
  return ua.length > 40 ? `${ua.slice(0, 40)}…` : ua;
}

async function loadList() {
  loading.value = true;
  try {
    const resp = await sessionService.listOnline();
    rows.value = resp.items || [];
  } catch {
    // error notification handled by axios interceptor
  } finally {
    loading.value = false;
  }
}

// 加载部门列表并构建 deptId→name 映射；失败不阻塞在线用户展示
async function loadDeptMap() {
  try {
    const resp = await departmentStore.fetchList({ noPaging: true });
    const map: Record<number, string> = {};
    for (const dept of resp.items) {
      if (dept.id != null) map[Number(dept.id)] = dept.name || '';
    }
    deptNameMap.value = map;
  } catch {
    // 部门加载失败时保留空映射，列展示 '-'
  }
}

async function onForceOffline(row: OnlineUserEntity) {
  if (!row.userId) return;
  try {
    await sessionService.forceOffline(row.userId);
    Notify.create({ type: 'positive', message: t('session.forceOfflineSuccess') });
    await loadList();
  } catch {
    // error notification handled by axios interceptor
  }
}

// 30 秒自动刷新在线会话列表；组件卸载时清理定时器，避免内存泄漏与重复请求
const REFRESH_INTERVAL_MS = 30_000;
let refreshTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  loadList();
  loadDeptMap();
  refreshTimer = setInterval(loadList, REFRESH_INTERVAL_MS);
});

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
});
</script>

<style lang="sass">
.session-stats-badge
  font-size: 12px
  font-weight: 600
  padding: 4px 10px
  border-radius: 8px
  color: var(--color-accent)
  border-color: color-mix(in srgb, var(--color-accent) 35%, transparent)
  background: color-mix(in srgb, var(--color-accent) 8%, transparent)

.session-auto-refresh
  display: inline-flex
  align-items: center
  gap: 4px
  font-size: 12px
  color: var(--color-text-tertiary)
  white-space: nowrap

body.body--dark .session-stats-badge
  box-shadow: 0 0 10px color-mix(in srgb, var(--color-accent) 22%, transparent)
</style>
