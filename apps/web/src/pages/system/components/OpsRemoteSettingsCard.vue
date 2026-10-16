<template>
  <q-card flat class="ops-settings-card q-mb-md">
    <q-card-section class="row items-center justify-between q-pb-sm">
      <div>
        <div class="text-subtitle1 text-weight-bold">
          <q-icon name="admin_panel_settings" size="20px" class="q-mr-xs" />{{ t('opsSettings.title') }}
        </div>
        <div class="text-caption text-grey q-mt-xs">{{ t('opsSettings.subtitle') }}</div>
      </div>
      <q-btn
        unelevated
        color="primary"
        icon="save"
        size="sm"
        :label="t('common.save')"
        :loading="saving"
        :disable="loading || loadError"
        @click="onSave"
      />
    </q-card-section>

    <q-separator dark />

    <q-card-section v-if="loading" class="row justify-center q-pa-lg">
      <q-spinner-dots size="32px" color="primary" />
    </q-card-section>

    <!-- F7：加载失败显式错误态（原静默落空表单，误导用户以为配置为空并保存覆盖） -->
    <q-card-section v-else-if="loadError" class="column items-center q-pa-lg">
      <q-icon name="error_outline" size="32px" color="negative" />
      <div class="text-caption text-grey q-mt-sm">{{ t('opsSettings.loadFailed') }}</div>
      <q-btn
        outline
        dense
        color="primary"
        icon="refresh"
        size="sm"
        class="q-mt-sm"
        :label="t('common.retry')"
        @click="loadSettings"
      />
    </q-card-section>

    <q-card-section v-else class="row q-col-gutter-lg">
      <!-- 高危资产分组（ops.high_risk_asset_groups，JSON 数组） -->
      <div class="col-12 col-md-5">
        <div class="ops-settings-label">{{ t('opsSettings.highRiskGroups') }}</div>
        <div class="ops-settings-hint">{{ t('opsSettings.highRiskGroupsHint') }}</div>
        <q-select
          v-model="form.highRiskGroupIds"
          :options="groupOptions"
          :loading="groupsLoading"
          multiple
          emit-value
          map-options
          use-chips
          dense
          outlined
          options-dense
          :placeholder="t('opsSettings.highRiskGroupsPlaceholder')"
        />
      </div>

      <!-- 单设备并发会话上限（ops.max_sessions_per_asset） -->
      <div class="col-12 col-md-3">
        <div class="ops-settings-label">{{ t('opsSettings.maxSessions') }}</div>
        <div class="ops-settings-hint">{{ t('opsSettings.maxSessionsHint') }}</div>
        <q-input
          v-model.number="form.maxSessions"
          type="number"
          dense
          outlined
          :min="1"
          :max="10"
          :rules="[(v) => (Number.isInteger(v) && v >= 1 && v <= 10) || t('opsSettings.maxSessionsInvalid')]"
        />
      </div>

      <!-- 二次确认开关（ops.session_confirm_required） -->
      <div class="col-12 col-md-4">
        <div class="ops-settings-label">{{ t('opsSettings.confirmRequired') }}</div>
        <div class="ops-settings-hint">{{ t('opsSettings.confirmRequiredHint') }}</div>
        <q-toggle
          v-model="form.confirmRequired"
          color="primary"
          :label="form.confirmRequired ? t('opsSettings.switchOn') : t('opsSettings.switchOff')"
        />
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
// 远程运维设置卡（远程运维能力增强 F5，概要设计 §5.3/B10）。
// 友好编辑 admin system_configs 的三个 ops.* 键（原始键值见下方 CRUD 表）：
//   ops.high_risk_asset_groups    JSON 数组（资产分组 ID，operator 档对其远控/WOL 需二次确认）
//   ops.max_sessions_per_asset    整数（单设备并发主控会话上限，默认 2）
//   ops.session_confirm_required  布尔（高危分组二次确认总开关，默认 true）
// 保存策略：键已存在 → update；不存在 → create（configType 分别为 json/number/bool）。
import { onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import {
  assetGroupService,
  systemConfigService,
  type AssetGroupTreeNode,
  type SystemConfigEntity,
} from '../../../services';
import { invalidateOpsSecuritySettingsCache } from '../../opstools/shared';

const { t } = useI18n();

const KEY_GROUPS = 'ops.high_risk_asset_groups';
const KEY_MAX_SESSIONS = 'ops.max_sessions_per_asset';
const KEY_CONFIRM = 'ops.session_confirm_required';

const loading = ref(true);
const saving = ref(false);
const groupsLoading = ref(false);
// F7：设置加载失败错误态（区别于「配置为空」——失败时禁止保存，防误覆盖线上配置）
const loadError = ref(false);

const form = ref({
  highRiskGroupIds: [] as number[],
  maxSessions: 2,
  confirmRequired: true,
});

// 已存在的配置行（键 → 行），保存时决定 update/create
const existingRows = new Map<string, SystemConfigEntity>();

interface GroupOption {
  label: string;
  value: number;
}

const groupOptions = ref<GroupOption[]>([]);

function flattenGroups(nodes: AssetGroupTreeNode[], prefix: string, out: GroupOption[]) {
  for (const n of nodes) {
    const label = prefix ? `${prefix} / ${n.name ?? ''}` : (n.name ?? '');
    const id = Number(n.id);
    if (Number.isFinite(id) && id > 0) out.push({ label, value: id });
    if (n.children?.length) flattenGroups(n.children, label, out);
  }
}

async function loadGroups() {
  groupsLoading.value = true;
  try {
    const resp = await assetGroupService.tree();
    const out: GroupOption[] = [];
    flattenGroups(resp.items ?? [], '', out);
    groupOptions.value = out;
  } catch {
    // 拦截器已提示
  } finally {
    groupsLoading.value = false;
  }
}

async function loadSettings() {
  loading.value = true;
  loadError.value = false;
  try {
    const resp = await systemConfigService.list({ noPaging: true });
    const items = (resp.items ?? []) as SystemConfigEntity[];
    existingRows.clear();
    for (const row of items) {
      if (row.configKey) existingRows.set(row.configKey, row);
    }
    // 高危分组：JSON 数组解析（坏值降级空数组，与后端 parseOpsSettings 同口径）
    const rawGroups = existingRows.get(KEY_GROUPS)?.configValue ?? '';
    try {
      const ids = JSON.parse(rawGroups || '[]') as unknown[];
      form.value.highRiskGroupIds = ids.map((v) => Number(v)).filter((v) => Number.isInteger(v) && v > 0);
    } catch {
      form.value.highRiskGroupIds = [];
    }
    const rawMax = Number(existingRows.get(KEY_MAX_SESSIONS)?.configValue);
    form.value.maxSessions = Number.isInteger(rawMax) && rawMax >= 1 && rawMax <= 10 ? rawMax : 2;
    form.value.confirmRequired = existingRows.get(KEY_CONFIRM)?.configValue !== 'false';
  } catch {
    loadError.value = true; // F7：显式错误态（拦截器 toast + 卡片内重试入口）
  } finally {
    loading.value = false;
  }
}

async function upsert(key: string, value: string, configType: string, description: string) {
  const hit = existingRows.get(key);
  if (hit?.id) {
    await systemConfigService.update(hit.id, { configValue: value });
  } else {
    await systemConfigService.create({ configKey: key, configValue: value, configType, description });
  }
}

async function onSave() {
  const max = Number(form.value.maxSessions);
  if (!Number.isInteger(max) || max < 1 || max > 10) {
    Notify.create({ type: 'negative', message: t('opsSettings.maxSessionsInvalid') });
    return;
  }
  saving.value = true;
  try {
    await upsert(KEY_GROUPS, JSON.stringify(form.value.highRiskGroupIds), 'json', t('opsSettings.highRiskGroups'));
    await upsert(KEY_MAX_SESSIONS, String(max), 'number', t('opsSettings.maxSessions'));
    await upsert(KEY_CONFIRM, form.value.confirmRequired ? 'true' : 'false', 'bool', t('opsSettings.confirmRequired'));
    Notify.create({ type: 'positive', message: t('opsSettings.saved') });
    invalidateOpsSecuritySettingsCache(); // 高危确认判定缓存失效，下一轮 needsSessionConfirm 即按新设置
    await loadSettings(); // 回读同步 existingRows（新建行拿到 id）
  } catch {
    // 拦截器已提示
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void loadSettings();
  void loadGroups();
});
</script>

<style lang="sass">
.ops-settings-card
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  border-radius: 12px

  .ops-settings-label
    font-weight: 600
    margin-bottom: 2px

  .ops-settings-hint
    font-size: 12px
    color: var(--color-text-secondary)
    margin-bottom: 8px
</style>
