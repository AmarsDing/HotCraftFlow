<template>
  <q-card flat class="screen-branding-card q-mb-md">
    <q-card-section class="row items-center justify-between q-pb-sm">
      <div>
        <div class="text-subtitle1 text-weight-bold">
          <q-icon name="screenshot_monitor" size="20px" class="q-mr-xs" />{{ t('screenBranding.title') }}
        </div>
        <div class="text-caption text-grey q-mt-xs">{{ t('screenBranding.subtitle') }}</div>
      </div>
      <q-btn
        unelevated
        color="primary"
        icon="save"
        size="sm"
        :label="t('common.save')"
        :loading="saving"
        :disable="loading || loadError || uploading"
        @click="onSave"
      />
    </q-card-section>

    <q-separator dark />

    <q-card-section v-if="loading" class="row justify-center q-pa-lg">
      <q-spinner-dots size="32px" color="primary" />
    </q-card-section>

    <!-- 加载失败显式错误态（与 SystemNameCard 同口径：防误把"加载失败"当"配置为空"覆盖保存） -->
    <q-card-section v-else-if="loadError" class="column items-center q-pa-lg">
      <q-icon name="error_outline" size="32px" color="negative" />
      <div class="text-caption text-grey q-mt-sm">{{ t('screenBranding.loadFailed') }}</div>
      <q-btn
        outline
        dense
        color="primary"
        icon="refresh"
        size="sm"
        class="q-mt-sm"
        :label="t('common.retry')"
        @click="loadConfig"
      />
    </q-card-section>

    <q-card-section v-else class="q-gutter-lg">
      <!-- 标题文本（留空保存 = 删行回落默认标题） -->
      <div class="row q-col-gutter-lg">
        <div class="col-12 col-md-6">
          <div class="screen-branding-card__label">{{ t('screenBranding.titleLabel') }}</div>
          <div class="screen-branding-card__hint">{{ t('screenBranding.titleHint') }}</div>
          <q-input v-model="title" dense outlined maxlength="64" clearable @keyup.enter="onSave" />
        </div>
      </div>

      <!-- 标题字体样式（全回落 = 删行回落顶栏默认 CSS） -->
      <div>
        <div class="screen-branding-card__label">{{ t('screenBranding.styleLabel') }}</div>
        <div class="screen-branding-card__hint">{{ t('screenBranding.styleHint') }}</div>
        <div class="row q-col-gutter-md q-mt-xs">
          <div class="col-12 col-sm-6 col-md-4">
            <q-select
              v-model="fontFamily"
              dense
              outlined
              emit-value
              map-options
              :label="t('screenBranding.fontFamily')"
              :options="fontFamilyOptions"
            />
            <q-input
              v-if="fontFamily === CUSTOM_FONT"
              v-model="customFontFamily"
              dense
              outlined
              class="q-mt-xs"
              :label="t('screenBranding.fontFamilyCustom')"
              maxlength="128"
            />
          </div>
          <div class="col-6 col-sm-3 col-md-2">
            <q-input
              v-model.number="fontSize"
              dense
              outlined
              type="number"
              min="16"
              max="40"
              :label="t('screenBranding.fontSize')"
              clearable
            />
          </div>
          <div class="col-6 col-sm-3 col-md-2">
            <q-select
              v-model="fontWeight"
              dense
              outlined
              emit-value
              map-options
              clearable
              :label="t('screenBranding.fontWeight')"
              :options="fontWeightOptions"
            />
          </div>
          <div class="col-6 col-sm-3 col-md-2">
            <q-input
              v-model.number="letterSpacing"
              dense
              outlined
              type="number"
              min="0"
              max="0.5"
              step="0.02"
              :label="t('screenBranding.letterSpacing')"
              clearable
            />
          </div>
          <div class="col-6 col-sm-3 col-md-2">
            <q-input v-model="color" dense outlined :label="t('screenBranding.color')" clearable>
              <template #append>
                <q-icon name="colorize" class="cursor-pointer">
                  <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                    <q-color v-model="color" />
                  </q-popup-proxy>
                </q-icon>
              </template>
            </q-input>
          </div>
        </div>
      </div>

      <!-- 背景图（images 桶相对路径；清除 = 删行回落昼夜纯色底） -->
      <div class="row q-col-gutter-lg items-start">
        <div class="col-12 col-md-6">
          <div class="screen-branding-card__label">{{ t('screenBranding.bgLabel') }}</div>
          <div class="screen-branding-card__hint">{{ t('screenBranding.bgHint') }}</div>
          <div class="row items-center q-gutter-sm">
            <q-file
              v-model="pickedFile"
              dense
              outlined
              accept="image/*"
              class="col"
              :loading="uploading"
              :label="t('screenBranding.bgPick')"
              @update:model-value="onPick"
            >
              <template #prepend>
                <q-icon name="image" />
              </template>
            </q-file>
            <q-btn
              v-if="bgImage"
              flat
              dense
              color="negative"
              icon="delete_outline"
              :label="t('screenBranding.bgClear')"
              @click="bgImage = ''"
            />
          </div>
          <div v-if="bgImage" class="text-caption text-grey q-mt-xs ellipsis">{{ bgImage }}</div>
        </div>
        <div v-if="bgImage" class="col-12 col-md-6">
          <div class="screen-branding-card__preview">
            <img v-if="bgPreviewUrl" :src="bgPreviewUrl" :alt="t('screenBranding.bgPreview')" />
            <div v-else class="screen-branding-card__preview-empty">
              <q-spinner-dots size="24px" color="primary" />
            </div>
          </div>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
// 大屏品牌配置卡（airport-twin 专属）：system_configs 三键友好编辑——
//   screen.twin.title        标题文本（留空 = 删行回落默认标题）
//   screen.twin.title_style  标题字体样式 JSON（全部回落 = 删行回落顶栏默认 CSS）
//   screen.twin.bg_image     背景图相对路径（images 桶；清除 = 删行回落昼夜纯色底）
// 保存策略：键已存在 → update；不存在 → create；清空 → delete（config_value NotEmpty 约束不允许空串行）。
// 背景图上传立即写库（不产生孤儿行）；删除已存背景图行后旧文件留在 images 桶不清理（对象存储无 GC）。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import { fileService, systemConfigService, type SystemConfigEntity } from '../../../services';
import { request } from '../../../services/request';
import { SCREEN_BG_IMAGE_KEY, SCREEN_TITLE_KEY, SCREEN_TITLE_STYLE_KEY } from '../../airport-twin/data/airportTwinRepo';
import type { ScreenTitleStyle } from '../../airport-twin/types';

const { t } = useI18n();

/** 字族「自定义」哨兵值（select 绑定；真实 CSS 值在 customFontFamily） */
const CUSTOM_FONT = '__custom__';

const loading = ref(true);
const saving = ref(false);
const loadError = ref(false);
const uploading = ref(false);

// 表单态（与库行一一对应；空串/null = 未配置回落默认）
const title = ref('');
const fontFamily = ref('');
const customFontFamily = ref('');
const fontSize = ref<number | null>(null);
const fontWeight = ref<number | null>(null);
const letterSpacing = ref<number | null>(null);
const color = ref('');
const bgImage = ref('');
const pickedFile = ref<File | null>(null);
const bgPreviewUrl = ref('');

// 已存在的配置行（保存时决定 update/create/delete；null = 键不存在）
let existingRows: Partial<Record<string, SystemConfigEntity>> = {};

const fontFamilyOptions = computed(() => [
  { label: t('screenBranding.fontDefault'), value: '' },
  { label: t('screenBranding.fontHei'), value: '"PingFang SC", "Microsoft YaHei", "Noto Sans SC", sans-serif' },
  { label: t('screenBranding.fontSong'), value: '"Songti SC", "SimSun", serif' },
  { label: t('screenBranding.fontKai'), value: '"Kaiti SC", "KaiTi", serif' },
  { label: t('screenBranding.fontCustom'), value: CUSTOM_FONT },
]);

const fontWeightOptions = computed(() => [
  { label: '400', value: 400 },
  { label: '500', value: 500 },
  { label: '600', value: 600 },
  { label: '700', value: 700 },
]);

/** 有效字族值（自定义态取 customFontFamily，未填 = 未配置） */
function effectiveFontFamily(): string {
  if (fontFamily.value === CUSTOM_FONT) return customFontFamily.value.trim();
  return fontFamily.value;
}

/** 表单 → title_style JSON 对象（全回落 → null = 删行） */
function buildStyle(): ScreenTitleStyle | null {
  const style: ScreenTitleStyle = {};
  const family = effectiveFontFamily();
  if (family) style.fontFamily = family;
  if (fontSize.value != null && fontSize.value > 0) style.fontSize = fontSize.value;
  if (fontWeight.value != null && fontWeight.value > 0) style.fontWeight = fontWeight.value;
  if (letterSpacing.value != null && letterSpacing.value >= 0) style.letterSpacing = letterSpacing.value;
  const c = color.value.trim();
  if (c) style.color = c;
  return Object.keys(style).length > 0 ? style : null;
}

/** 库行 → 表单回填（坏 JSON 容忍：逐字段读取，缺字段回落） */
function applyStyle(raw: string | undefined): void {
  let obj: Record<string, unknown> | null = null;
  try {
    if (raw) obj = JSON.parse(raw) as Record<string, unknown>;
  } catch {
    obj = null;
  }
  const family = typeof obj?.fontFamily === 'string' ? obj.fontFamily : '';
  const preset = fontFamilyOptions.value.find((o) => o.value === family && o.value !== '');
  if (family && !preset) {
    fontFamily.value = CUSTOM_FONT;
    customFontFamily.value = family;
  } else {
    fontFamily.value = preset ? family : '';
    customFontFamily.value = '';
  }
  fontSize.value = typeof obj?.fontSize === 'number' && obj.fontSize > 0 ? obj.fontSize : null;
  fontWeight.value = typeof obj?.fontWeight === 'number' && obj.fontWeight > 0 ? obj.fontWeight : null;
  letterSpacing.value = typeof obj?.letterSpacing === 'number' && obj.letterSpacing >= 0 ? obj.letterSpacing : null;
  color.value = typeof obj?.color === 'string' ? obj.color : '';
}

// 背景图预览（鉴权 blob → objectURL；换路径/清空回收旧 URL）
watch(bgImage, (path) => {
  if (bgPreviewUrl.value) {
    URL.revokeObjectURL(bgPreviewUrl.value);
    bgPreviewUrl.value = '';
  }
  if (!path) return;
  fileService
    .fetchBlob(path)
    .then((blob) => {
      // 竞态守备：拉取期间路径已变更 → 丢弃
      if (bgImage.value !== path) return;
      bgPreviewUrl.value = URL.createObjectURL(blob);
    })
    .catch(() => undefined);
});

onBeforeUnmount(() => {
  if (bgPreviewUrl.value) URL.revokeObjectURL(bgPreviewUrl.value);
});

async function loadConfig() {
  loading.value = true;
  loadError.value = false;
  try {
    const keys = [SCREEN_TITLE_KEY, SCREEN_TITLE_STYLE_KEY, SCREEN_BG_IMAGE_KEY];
    const resps = await Promise.all(
      keys.map((key) => systemConfigService.list({ noPaging: true, formValues: { configKey: key } })),
    );
    const rows: Partial<Record<string, SystemConfigEntity>> = {};
    resps.forEach((resp, i) => {
      const hit = (resp.items ?? []).find((it) => it.configKey === keys[i]);
      if (hit) rows[keys[i]] = hit;
    });
    existingRows = rows;
    title.value = (rows[SCREEN_TITLE_KEY]?.configValue ?? '').trim();
    applyStyle(rows[SCREEN_TITLE_STYLE_KEY]?.configValue);
    bgImage.value = (rows[SCREEN_BG_IMAGE_KEY]?.configValue ?? '').trim();
  } catch {
    loadError.value = true; // 拦截器已提示
  } finally {
    loading.value = false;
  }
}

/** 单键落库：值非空 → update/create；空 → 行在则删（回落默认语义） */
async function upsertKey(key: string, value: string, configType: string, description: string): Promise<void> {
  const row = existingRows[key];
  if (value) {
    if (row?.id) {
      await systemConfigService.update(row.id, { configValue: value });
    } else {
      await systemConfigService.create({ configKey: key, configValue: value, configType, description });
    }
  } else if (row?.id) {
    await systemConfigService.delete(row.id);
  }
}

/** 背景图上传（立即写库——路径需落库后才能在大屏侧预览生效，避免「保存了又忘保存」两阶段心智） */
async function onPick(file: File | null) {
  if (!file) return;
  uploading.value = true;
  try {
    const ext = (file.name.split('.').pop() || 'png').toLowerCase();
    const objectName = `screen/${crypto.randomUUID()}.${ext}`;
    const fd = new FormData();
    fd.append('file', file);
    const resp = await request.post<{ url?: string }>('/api/v1/monitor/file:upload', fd, {
      params: { bucketName: 'images', objectName },
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    const url = resp?.url ?? '';
    const idx = url.indexOf('path=');
    const rel = idx >= 0 ? decodeURIComponent(url.slice(idx + 5)) : '';
    if (!rel) throw new Error('upload response missing path');
    await upsertKey(SCREEN_BG_IMAGE_KEY, rel, 'string', '机场大屏背景图');
    bgImage.value = rel;
    await loadConfig();
    Notify.create({ type: 'positive', message: t('screenBranding.bgUploaded') });
  } catch {
    // 拦截器已提示（含无 path 异常走下方 finally 复位）
  } finally {
    uploading.value = false;
    pickedFile.value = null;
  }
}

async function onSave() {
  saving.value = true;
  try {
    const style = buildStyle();
    await Promise.all([
      upsertKey(SCREEN_TITLE_KEY, title.value.trim(), 'string', '机场大屏标题'),
      upsertKey(SCREEN_TITLE_STYLE_KEY, style ? JSON.stringify(style) : '', 'json', '机场大屏标题字体样式'),
      upsertKey(SCREEN_BG_IMAGE_KEY, bgImage.value.trim(), 'string', '机场大屏背景图'),
    ]);
    Notify.create({ type: 'positive', message: t('screenBranding.saved') });
    await loadConfig(); // 回读同步 existingRows（新建行拿到 id）
  } catch {
    // 拦截器已提示
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  void loadConfig();
});
</script>

<style lang="sass">
.screen-branding-card
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

  &__preview
    border: 1px dashed var(--glass-border)
    border-radius: 8px
    overflow: hidden
    height: 140px
    display: flex
    align-items: center
    justify-content: center

    img
      width: 100%
      height: 100%
      object-fit: cover

  &__preview-empty
    display: flex
    align-items: center
    justify-content: center
    width: 100%
    height: 100%
</style>
