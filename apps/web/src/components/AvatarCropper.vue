<template>
  <q-dialog
    :model-value="modelValue"
    persistent
    @update:model-value="emit('update:modelValue', $event)"
  >
    <q-card class="avatar-cropper column no-wrap">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">{{ title || t('user.avatarEditTitle') }}</div>
        <q-space />
        <q-btn icon="close" flat round dense :disable="confirming" @click="onCancel" />
      </q-card-section>

      <q-card-section class="avatar-cropper__body">
        <div
          class="avatar-cropper__viewport"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointercancel="onPointerUp"
          @wheel.prevent="onWheel"
        >
          <img
            v-if="imgReady"
            :src="src"
            class="avatar-cropper__img"
            :style="imgStyle"
            draggable="false"
            alt=""
          />
          <div class="avatar-cropper__mask" />
        </div>

        <!-- 实时预览：与导出结果逐像素一致（按取景比例缩放），覆盖个人中心/顶栏两档尺寸 -->
        <div v-if="imgReady" class="avatar-cropper__previews">
          <div class="avatar-cropper__preview-item">
            <div class="avatar-cropper__preview" style="width: 88px; height: 88px">
              <img :src="src" :style="previewStyle(88)" draggable="false" alt="" />
            </div>
            <span class="avatar-cropper__preview-caption">{{ t('profile.avatarPreviewProfile') }}</span>
          </div>
          <div class="avatar-cropper__preview-item">
            <div class="avatar-cropper__preview" style="width: 32px; height: 32px">
              <img :src="src" :style="previewStyle(32)" draggable="false" alt="" />
            </div>
            <span class="avatar-cropper__preview-caption">{{ t('profile.avatarPreviewTopbar') }}</span>
          </div>
        </div>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <div class="row items-center full-width q-gutter-sm">
          <q-icon name="zoom_out" size="18px" class="avatar-cropper__zoom-icon" />
          <q-slider v-model="scale" :min="MIN_SCALE" :max="MAX_SCALE" :step="0.01" class="col" />
          <q-icon name="zoom_in" size="18px" class="avatar-cropper__zoom-icon" />
        </div>
        <div class="text-caption text-grey q-mt-xs">{{ hint || t('user.avatarCropHint') }}</div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat :label="t('common.cancel')" :disable="confirming" @click="onCancel" />
        <q-btn
          unelevated
          color="primary"
          :label="t('common.confirm')"
          :loading="confirming"
          @click="onConfirm"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
// 图片裁剪弹窗（类微信）：圆角取景框，拖拽调位置、滚轮/滑块缩放，
// 确认后按可视区裁切输出 PNG Blob（天然压缩，任意大照片输入均可）。
// 头像默认输出 256x256；设备图等场景可经 outputSize 调大（如 512 保细节）。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    src: string; // dataURL
    confirming?: boolean;
    title?: string; // 弹窗标题（缺省=调整头像）
    hint?: string; // 操作提示（缺省=头像提示文案）
    outputSize?: number; // 导出边长 px（缺省 256；设备图建议 512）
  }>(),
  { confirming: false, title: '', hint: '', outputSize: 256 },
);

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  confirm: [blob: Blob];
  cancel: [];
}>();

const { t } = useI18n();

// 取景框边长（CSS 同步）；导出边长由 outputSize 决定
const VIEW = 320;
const MIN_SCALE = 1;
const MAX_SCALE = 8;

const img = new Image();
const imgReady = ref(false);
let naturalW = 0;
let naturalH = 0;
// cover 适配系数：natural * fit 后刚好覆盖取景框
let fit = 1;

const scale = ref(1);
const offsetX = ref(0);
const offsetY = ref(0);

const imgStyle = computed(() => ({
  width: `${naturalW * fit * scale.value}px`,
  height: `${naturalH * fit * scale.value}px`,
  transform: `translate(${offsetX.value}px, ${offsetY.value}px)`,
}));

// 预览缩略样式：取景框 VIEW → size 等比缩放，几何与 onConfirm 导出完全一致
function previewStyle(size: number) {
  const k = size / VIEW;
  return {
    width: `${naturalW * fit * scale.value * k}px`,
    height: `${naturalH * fit * scale.value * k}px`,
    transform: `translate(${offsetX.value * k}px, ${offsetY.value * k}px)`,
  };
}

function clampOffsets() {
  const w = naturalW * fit * scale.value;
  const h = naturalH * fit * scale.value;
  offsetX.value = Math.min(0, Math.max(VIEW - w, offsetX.value));
  offsetY.value = Math.min(0, Math.max(VIEW - h, offsetY.value));
}

// 以取景框中心为锚点缩放：中心下的图像点保持不动
watch(scale, (nv, ov) => {
  if (!imgReady.value || ov === 0) return;
  const ratio = nv / ov;
  const c = VIEW / 2;
  offsetX.value = c - (c - offsetX.value) * ratio;
  offsetY.value = c - (c - offsetY.value) * ratio;
  clampOffsets();
});

function resetView() {
  fit = Math.max(VIEW / naturalW, VIEW / naturalH);
  scale.value = 1;
  offsetX.value = (VIEW - naturalW * fit) / 2;
  offsetY.value = (VIEW - naturalH * fit) / 2;
  imgReady.value = true;
}

watch(
  () => [props.modelValue, props.src] as const,
  ([open, src]) => {
    if (!open || !src) {
      imgReady.value = false;
      return;
    }
    imgReady.value = false;
    img.onload = () => {
      naturalW = img.naturalWidth;
      naturalH = img.naturalHeight;
      resetView();
    };
    img.src = src;
  },
  { immediate: true },
);

// 拖拽（pointer capture 保证移出取景框仍连续跟踪）
let dragging = false;
let lastX = 0;
let lastY = 0;

function onPointerDown(e: PointerEvent) {
  if (!imgReady.value) return;
  (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  dragging = true;
  lastX = e.clientX;
  lastY = e.clientY;
}

function onPointerMove(e: PointerEvent) {
  if (!dragging) return;
  offsetX.value += e.clientX - lastX;
  offsetY.value += e.clientY - lastY;
  lastX = e.clientX;
  lastY = e.clientY;
  clampOffsets();
}

function onPointerUp(e: PointerEvent) {
  dragging = false;
  try {
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  } catch {
    // 未持有时忽略
  }
}

function onWheel(e: WheelEvent) {
  if (!imgReady.value) return;
  const next = scale.value * (e.deltaY < 0 ? 1.12 : 1 / 1.12);
  scale.value = Math.min(MAX_SCALE, Math.max(MIN_SCALE, next));
}

function onCancel() {
  emit('cancel');
  emit('update:modelValue', false);
}

function onConfirm() {
  if (!imgReady.value) return;
  const out = props.outputSize;
  const k = fit * scale.value;
  const sx = -offsetX.value / k;
  const sy = -offsetY.value / k;
  const sSize = VIEW / k;
  const canvas = document.createElement('canvas');
  canvas.width = out;
  canvas.height = out;
  const ctx = canvas.getContext('2d');
  if (!ctx) return;
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = 'high';
  ctx.drawImage(img, sx, sy, sSize, sSize, 0, 0, out, out);
  canvas.toBlob((blob) => {
    if (blob) emit('confirm', blob);
  }, 'image/png');
}
</script>

<style lang="sass" scoped>
.avatar-cropper
  min-width: 480px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

  &__body
    display: flex
    align-items: center
    justify-content: center
    gap: var(--space-4)

  &__viewport
    position: relative
    width: 320px
    height: 320px
    flex-shrink: 0
    overflow: hidden
    cursor: grab
    touch-action: none
    user-select: none
    -webkit-user-select: none
    background: var(--canvas-base)
    border-radius: 8px

    &:active
      cursor: grabbing

  &__img
    position: absolute
    top: 0
    left: 0
    max-width: none
    max-height: none
    pointer-events: none

  // 圆角正方形取景框（与 UserAvatar 20% 圆角渲染一致）：
  // 充满整个可视区（导出区域=可视区，框外半透明遮罩经 box-shadow 外扩）
  &__mask
    position: absolute
    inset: 0
    border-radius: 20%
    box-shadow: 0 0 0 9999px rgba(0, 0, 0, 0.55)
    border: 1px solid color-mix(in srgb, var(--color-accent) 45%, transparent)
    pointer-events: none

  &__previews
    display: flex
    flex-direction: column
    gap: var(--space-3)

  &__preview-item
    display: flex
    flex-direction: column
    align-items: center
    gap: 6px

  &__preview
    position: relative
    overflow: hidden
    border-radius: 20%
    background: var(--canvas-base)
    border: 1px solid var(--glass-border)

    img
      position: absolute
      top: 0
      left: 0
      max-width: none
      max-height: none
      pointer-events: none

  &__preview-caption
    font-size: var(--text-xs)
    color: var(--color-text-tertiary)

  &__zoom-icon
    color: var(--color-text-secondary)
</style>
