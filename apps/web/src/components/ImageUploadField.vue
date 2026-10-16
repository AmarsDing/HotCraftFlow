<template>
  <div class="image-upload-field row items-center q-gutter-md">
    <DeviceImage :src="modelValue" :size="previewSize" :fallback-icon="fallbackIcon" />
    <div class="column q-gutter-xs">
      <div class="row q-gutter-xs">
        <q-btn
          unelevated
          color="primary"
          icon="cloud_upload"
          :label="modelValue ? t('asset.editImage') : t('asset.uploadImage')"
          :loading="uploading"
          @click="onPickOrEdit"
        />
        <q-btn
          v-if="modelValue"
          flat
          color="negative"
          icon="delete_outline"
          :label="t('asset.removeImage')"
          :disable="uploading"
          @click="emit('update:modelValue', '')"
        />
      </div>
      <div class="text-caption text-grey">{{ t('asset.imageUploadHint') }}</div>
      <q-btn
        v-if="modelValue"
        flat
        dense
        color="primary"
        icon="photo_library"
        :label="t('asset.reuploadImage')"
        @click="onReupload"
      />
    </div>
    <input
      ref="fileInputRef"
      type="file"
      accept="image/png,image/jpeg,image/webp,image/gif"
      hidden
      @change="onFilePicked"
    />

    <!-- 裁剪弹窗（复用头像控件：拖拽调位置、滚轮缩放、确认输出方形 PNG） -->
    <AvatarCropper
      v-model="cropperOpen"
      :src="cropperSrc"
      :confirming="uploading"
      :title="t('asset.imageEditTitle')"
      :hint="t('asset.imageCropHint')"
      :output-size="512"
      @confirm="onCropConfirm"
    />
  </div>
</template>

<script setup lang="ts">
// 设备/分类图片上传编辑控件：与头像同一 AvatarCropper 裁剪链路（滚轮缩放+拖拽+方形导出 512px），
// 上传走 file:upload（images 桶 device-images/ 目录），值为存储相对路径。
// 新建无图=点按钮选文件即裁；已有图=点按钮直接打开裁剪弹窗编辑当前图，点"更换"分支在弹窗内重新选图。
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import AvatarCropper from './AvatarCropper.vue';
import DeviceImage from './DeviceImage.vue';
import { fileService } from '../services';
import { request } from '../services/request';

const props = withDefaults(
  defineProps<{
    modelValue?: string; // 存储相对路径
    previewSize?: number;
    fallbackIcon?: string;
  }>(),
  { modelValue: '', previewSize: 72, fallbackIcon: 'devices' },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const { t } = useI18n();

const uploading = ref(false);
const cropperOpen = ref(false);
const cropperSrc = ref('');
const fileInputRef = ref<HTMLInputElement | null>(null);

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

async function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

// 已有图：拉取当前图进裁剪器直接编辑；无图：触发文件选择
async function onPickOrEdit() {
  const val = (props.modelValue || '').trim();
  if (!val) {
    fileInputRef.value?.click();
    return;
  }
  uploading.value = true;
  try {
    const blob = await fileService.fetchBlob(val);
    cropperSrc.value = await blobToDataURL(blob);
    cropperOpen.value = true;
  } catch {
    Notify.create({ type: 'warning', message: t('asset.imageUploadHint') });
  } finally {
    uploading.value = false;
  }
}

async function onFilePicked(evt: Event) {
  const input = evt.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  if (!/^image\/(png|jpe?g|webp|gif)$/.test(file.type)) {
    Notify.create({ type: 'warning', message: t('user.avatarTypeInvalid') });
    return;
  }
  if (file.size > 10 * 1024 * 1024) {
    Notify.create({ type: 'warning', message: t('user.avatarTooLarge') });
    return;
  }
  cropperSrc.value = await readAsDataURL(file);
  cropperOpen.value = true;
}

// 已有图时另提供"更换图片"：重新选本地文件进裁剪器
function onReupload() {
  fileInputRef.value?.click();
}

// 裁剪确认 → 上传 images 桶 device-images/<uuid>.png → 回写存储相对路径
async function onCropConfirm(blob: Blob) {
  uploading.value = true;
  try {
    const file = new File([blob], 'device-image.png', { type: 'image/png' });
    const fd = new FormData();
    fd.append('file', file);
    const resp = await request.post<{ url?: string }>('/api/v1/monitor/file:upload', fd, {
      params: { bucketName: 'images', objectName: `device-images/${crypto.randomUUID()}.png` },
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    const url = resp?.url || '';
    const idx = url.indexOf('path=');
    const rel = idx >= 0 ? decodeURIComponent(url.slice(idx + 5)) : url;
    if (!rel) throw new Error('上传返回缺少文件路径');
    emit('update:modelValue', rel);
    cropperOpen.value = false;
  } catch {
    // error notification handled by axios interceptor
  } finally {
    uploading.value = false;
  }
}
</script>
