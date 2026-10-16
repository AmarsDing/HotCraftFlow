<template>
  <q-dialog :model-value="modelValue" @update:model-value="emit('update:modelValue', $event)">
    <q-card
      class="avatar-change column no-wrap"
      :class="{ 'avatar-change--drop': dragOver }"
      @dragenter.prevent="onDragEnter"
      @dragover.prevent
      @dragleave.prevent="onDragLeave"
      @drop.prevent="onDrop"
    >
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">{{ t('profile.avatarDialogTitle') }}</div>
        <q-space />
        <q-btn icon="close" flat round dense :disable="busy" @click="close" />
      </q-card-section>

      <!-- 头像舞台：大图预览（类微信查看头像），拖拽文件至此直接换 -->
      <q-card-section class="column items-center q-gutter-xs">
        <div class="avatar-change__stage">
          <!-- previewUrl（预设静态图等直显场景）优先；否则 UserAvatar（存储路径 blob 拉取/URL 直显/首字母回退） -->
          <img
            v-if="previewUrl"
            :src="previewUrl"
            :alt="name"
            class="avatar-change__avatar avatar-change__preview"
          />
          <UserAvatar v-else :avatar="avatar" :name="name" :size="220" class="avatar-change__avatar" />
          <div v-if="busy" class="avatar-change__busy">
            <q-spinner color="white" size="40px" />
          </div>
          <div v-if="dragOver" class="avatar-change__droptip">
            <q-icon name="file_download" size="28px" />
            <span>{{ t('profile.avatarDropNow') }}</span>
          </div>
        </div>
        <div class="avatar-change__name">{{ name }}</div>
        <div v-if="username" class="avatar-change__username">@{{ username }}</div>
      </q-card-section>

      <!-- 操作列表（类微信 action sheet 风格） -->
      <q-card-section class="q-pt-xs q-pb-sm">
        <div class="avatar-change__actions">
          <button type="button" class="avatar-change__action" :disabled="busy" @click="pickFile">
            <q-icon name="photo_camera" size="20px" class="avatar-change__action-icon" />
            <span class="avatar-change__action-label">{{ t('profile.editAvatar') }}</span>
            <q-icon name="chevron_right" size="18px" class="avatar-change__action-arrow" />
          </button>
          <button
            v-if="avatar && !previewUrl"
            type="button"
            class="avatar-change__action avatar-change__action--danger"
            :disabled="busy"
            @click="onRemove"
          >
            <q-icon name="delete_outline" size="20px" class="avatar-change__action-icon" />
            <span class="avatar-change__action-label">{{ t('profile.removeAvatar') }}</span>
          </button>
        </div>
        <div class="avatar-change__hint">{{ t('profile.avatarDropHint') }}</div>
      </q-card-section>

      <input
        ref="fileInputRef"
        type="file"
        accept="image/png,image/jpeg,image/webp,image/gif"
        hidden
        @change="onFilePicked"
      />
    </q-card>

    <!-- 裁剪弹窗（堆叠于本弹窗之上：拖拽调位置、滚轮缩放、实时预览） -->
    <AvatarCropper v-model="cropperOpen" :src="cropperSrc" :confirming="busy" @confirm="onCropConfirm" />
  </q-dialog>
</template>

<script setup lang="ts">
// 类微信头像弹窗：大图预览 + 更换/移除操作列表。
// 增强交互：支持拖拽图片到弹窗、Ctrl+V 粘贴截图直接进入裁剪；
// 裁剪确认后上传 avatars 桶 → updateMe 落库 → refreshProfile 同步顶栏。
import { ref, watch, onBeforeUnmount } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import UserAvatar from './UserAvatar.vue';
import AvatarCropper from './AvatarCropper.vue';
import { authService, userService } from '../services';
import { useAuthStore } from '../stores';

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    avatar?: string; // 当前头像（存储相对路径或 URL）
    name?: string; // 昵称（首字母回退用）
    username?: string;
    // 大预览静态图 URL（预设头像等非存储场景直显）；传入时隐藏"移除头像"（预设图不可移除）
    previewUrl?: string;
    // 自定义落库链路（参数为新头像值，空串=移除）；缺省走用户头像链路（updateMe + 刷新顶栏），
    // 智能体等非用户场景传入
    persistAvatar?: (avatar: string) => Promise<void>;
  }>(),
  { avatar: '', name: '', username: '', previewUrl: '', persistAvatar: undefined },
);

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  saved: [avatar: string]; // 更换/移除成功后的新头像值（空串=已移除）
}>();

const { t } = useI18n();
const auth = useAuthStore();

const busy = ref(false);
const cropperOpen = ref(false);
const cropperSrc = ref('');
const fileInputRef = ref<HTMLInputElement | null>(null);
const dragOver = ref(false);
let dragDepth = 0;

function close() {
  emit('update:modelValue', false);
}

function pickFile() {
  fileInputRef.value?.click();
}

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

// 统一文件入口：本地选择 / 拖拽 / 粘贴均经此校验后进裁剪器
async function acceptFile(file: File | undefined) {
  if (!file || busy.value) return;
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

function onFilePicked(evt: Event) {
  const input = evt.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  void acceptFile(file);
}

function onDragEnter() {
  dragDepth += 1;
  dragOver.value = true;
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) dragOver.value = false;
}

function onDrop(evt: DragEvent) {
  dragDepth = 0;
  dragOver.value = false;
  void acceptFile(evt.dataTransfer?.files?.[0]);
}

// Ctrl+V 粘贴截图：弹窗打开期间监听 window paste
function onPaste(evt: ClipboardEvent) {
  const file = Array.from(evt.clipboardData?.files ?? []).find((f) => f.type.startsWith('image/'));
  if (file) {
    evt.preventDefault();
    void acceptFile(file);
  }
}

// 落库：外部传入 persistAvatar 时委托外部（如智能体头像写回）；缺省走用户头像链路（updateMe + 顶栏同步）
async function persist(avatar: string) {
  if (props.persistAvatar) {
    await props.persistAvatar(avatar);
    return;
  }
  await authService.updateMe({ avatar });
  await auth.refreshProfile();
}

// 裁剪确认 → 上传 avatars 桶 → 落库 → 顶栏同步 → 收尾
async function onCropConfirm(blob: Blob) {
  busy.value = true;
  try {
    const file = new File([blob], 'avatar.png', { type: 'image/png' });
    const rel = await userService.uploadAvatar(file);
    await persist(rel);
    cropperOpen.value = false;
    emit('saved', rel);
    close();
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    busy.value = false;
  }
}

// 移除头像：回退首字母默认头像（UserAvatar 无图自动回退）
async function onRemove() {
  busy.value = true;
  try {
    await persist('');
    emit('saved', '');
    close();
    Notify.create({ type: 'positive', message: t('profile.avatarRemoved') });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    busy.value = false;
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      window.addEventListener('paste', onPaste);
    } else {
      window.removeEventListener('paste', onPaste);
      dragDepth = 0;
      dragOver.value = false;
    }
  },
);

onBeforeUnmount(() => {
  window.removeEventListener('paste', onPaste);
});
</script>

<style lang="sass" scoped>
.avatar-change
  min-width: 380px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))
  border-radius: 18px
  transition: border-color 0.18s ease

  &--drop
    border-color: var(--color-accent)

  &__stage
    position: relative
    border-radius: 20%
    overflow: hidden

  &__avatar
    font-size: 84px

  &__preview
    width: 220px
    height: 220px
    object-fit: cover
    display: block

  &__busy
    position: absolute
    inset: 0
    display: flex
    align-items: center
    justify-content: center
    background: rgba(0, 0, 0, 0.45)

  &__droptip
    position: absolute
    inset: 0
    display: flex
    flex-direction: column
    align-items: center
    justify-content: center
    gap: 6px
    font-size: var(--text-sm)
    font-weight: 600
    color: var(--color-on-accent)
    background: color-mix(in srgb, var(--color-accent) 55%, rgba(0, 0, 0, 0.35))
    border: 2px dashed var(--color-on-accent)
    border-radius: 20%
    pointer-events: none

  &__name
    margin-top: var(--space-2)
    font-size: var(--text-md)
    font-weight: 700
    color: var(--color-text-heading)

  &__username
    font-size: var(--text-xs)
    color: var(--color-text-tertiary)

  // 操作列表：类微信 action sheet 行
  &__actions
    display: flex
    flex-direction: column
    border-radius: 12px
    overflow: hidden
    border: 1px solid var(--glass-border)

  &__action
    display: flex
    align-items: center
    gap: 10px
    width: 100%
    padding: 12px 14px
    border: none
    background: color-mix(in srgb, var(--glass-surface) 72%, transparent)
    color: var(--color-text-primary)
    font-size: var(--text-sm)
    font-weight: 600
    cursor: pointer
    transition: background 0.15s ease

    & + .avatar-change__action
      border-top: 1px solid var(--glass-border)

    &:hover:not(:disabled)
      background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface))

    &:disabled
      opacity: 0.55
      cursor: not-allowed

    &--danger
      color: var(--q-negative)

      .avatar-change__action-icon
        color: var(--q-negative)

  &__action-icon
    color: var(--color-accent)

  &__action-label
    flex: 1
    text-align: left

  &__action-arrow
    color: var(--color-text-tertiary)

  &__hint
    margin-top: var(--space-2)
    text-align: center
    font-size: var(--text-xs)
    color: var(--color-text-tertiary)
</style>
