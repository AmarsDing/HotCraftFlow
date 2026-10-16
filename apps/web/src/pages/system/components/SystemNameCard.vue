<template>
  <q-card flat class="system-name-card q-mb-md">
    <q-card-section class="row items-center justify-between q-pb-sm">
      <div>
        <div class="text-subtitle1 text-weight-bold">
          <q-icon name="badge" size="20px" class="q-mr-xs" />{{ t('systemNameSettings.title') }}
        </div>
        <div class="text-caption text-grey q-mt-xs">{{ t('systemNameSettings.subtitle') }}</div>
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

    <!-- 加载失败显式错误态（与 OpsRemoteSettingsCard F7 同口径：防误把"加载失败"当"配置为空"覆盖保存） -->
    <q-card-section v-else-if="loadError" class="column items-center q-pa-lg">
      <q-icon name="error_outline" size="32px" color="negative" />
      <div class="text-caption text-grey q-mt-sm">{{ t('systemNameSettings.loadFailed') }}</div>
      <q-btn
        outline
        dense
        color="primary"
        icon="refresh"
        size="sm"
        class="q-mt-sm"
        :label="t('common.retry')"
        @click="loadName"
      />
    </q-card-section>

    <q-card-section v-else class="row q-col-gutter-lg">
      <div class="col-12 col-md-6">
        <div class="system-name-card__label">{{ t('systemNameSettings.label') }}</div>
        <div class="system-name-card__hint">{{ t('systemNameSettings.hint') }}</div>
        <q-input
          v-model="name"
          dense
          outlined
          maxlength="64"
          :rules="[(v) => !!(v && v.trim()) || t('systemNameSettings.required')]"
          @keyup.enter="onSave"
        />
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
// 系统名称设置卡：友好编辑 system_configs 的 system_name 键（原始键值见下方 CRUD 表）。
// 保存后写入 system-name store：顶栏/侧栏品牌区/登录页/浏览器页签即时同步（无需刷新），
// 并写 localStorage 缓存供未登录的登录页沿用。
// 保存策略：键已存在 → update；不存在 → create（老库 seed 前数据缺失兜底）。
import { onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import { systemConfigService, type SystemConfigEntity } from '../../../services';
import { useSystemNameStore } from '../../../stores/system-name';

const { t } = useI18n();
const systemNameStore = useSystemNameStore();

const KEY = 'system_name';

const loading = ref(true);
const saving = ref(false);
const loadError = ref(false);
const name = ref('');

// 已存在的配置行（保存时决定 update/create）
let existingRow: SystemConfigEntity | null = null;

async function loadName() {
  loading.value = true;
  loadError.value = false;
  try {
    const resp = await systemConfigService.list({ noPaging: true, formValues: { configKey: KEY } });
    const items = (resp.items ?? []) as SystemConfigEntity[];
    existingRow = items.find((it) => it.configKey === KEY) ?? null;
    name.value = (existingRow?.configValue ?? '').trim();
  } catch {
    loadError.value = true; // 拦截器已提示
  } finally {
    loading.value = false;
  }
}

async function onSave() {
  const v = name.value.trim();
  if (!v) {
    Notify.create({ type: 'negative', message: t('systemNameSettings.required') });
    return;
  }
  saving.value = true;
  try {
    if (existingRow?.id) {
      await systemConfigService.update(existingRow.id, { configValue: v });
    } else {
      await systemConfigService.create({
        configKey: KEY,
        configValue: v,
        configType: 'string',
        description: '系统名称',
      });
    }
    systemNameStore.setName(v); // 全站展示位即时同步（含 localStorage 缓存与浏览器页签标题）
    Notify.create({ type: 'positive', message: t('systemNameSettings.saved') });
    await loadName(); // 回读同步 existingRow（新建行拿到 id）
  } catch {
    // 拦截器已提示
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void loadName();
});
</script>

<style lang="sass">
.system-name-card
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  border-radius: 12px

  &__label
    font-weight: 600
    margin-bottom: 2px

  &__hint
    font-size: 12px
    color: var(--color-text-secondary)
    margin-bottom: 8px
</style>
