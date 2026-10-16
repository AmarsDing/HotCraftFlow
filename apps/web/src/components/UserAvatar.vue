<template>
  <q-avatar :size="`${size}px`" class="user-avatar">
    <img v-if="imgUrl" :src="imgUrl" :alt="name" class="user-avatar__img" />
    <span v-else class="user-avatar__letter">{{ letter }}</span>
  </q-avatar>
</template>

<script lang="ts">
// 模块级 object URL 缓存：同一路径（如同一值班人出现在日历多个格子）只拉取一次 blob。
// 头像上传走 UUID 新路径（userService.uploadAvatar），换头像即换 key，缓存天然不失效问题。
const objectUrlCache = new Map<string, string>();
// in-flight 去重：同帧并发挂载的 N 个同头像实例共享同一拉取 Promise，避免各自发重复请求
const inflightCache = new Map<string, Promise<string>>();

function fetchOnce(rel: string): Promise<string> {
  let p = inflightCache.get(rel);
  if (!p) {
    p = fileService
      .fetchBlob(rel)
      .then((blob) => {
        const url = URL.createObjectURL(blob);
        objectUrlCache.set(rel, url);
        inflightCache.delete(rel);
        return url;
      })
      .catch((e) => {
        inflightCache.delete(rel);
        throw e;
      });
    inflightCache.set(rel, p);
  }
  return p;
}
</script>

<script setup lang="ts">
// 用户头像渲染：avatar 值为 http(s)/data/blob URL 时直显；
// 为存储相对路径（或 file:download URL）时经 fileService.fetchBlob 鉴权拉取 blob 转 object URL；
// 无头像时回退首字母。
import { computed, ref, watch } from 'vue';
import { fileService } from '../services';

const props = withDefaults(
  defineProps<{
    avatar?: string;
    name?: string;
    size?: number;
  }>(),
  { avatar: '', name: '', size: 32 },
);

const imgUrl = ref('');

const letter = computed(() => {
  const s = (props.name || '').trim();
  return s ? s.slice(0, 1).toUpperCase() : '?';
});

function isDirectUrl(v: string): boolean {
  // http(s)/协议相对/data/blob URL，以及同源根相对路径（/avatars/... 等 public 静态资产）均可直接渲染
  return /^(https?:)?\/\//.test(v) || v.startsWith('data:') || v.startsWith('blob:') || v.startsWith('/');
}

// file:download?path=<rel> 形式提取相对路径，其余按相对路径处理
function extractRelPath(v: string): string {
  const idx = v.indexOf('path=');
  return idx >= 0 ? decodeURIComponent(v.slice(idx + 5)) : v;
}

async function load(rel: string) {
  const cached = objectUrlCache.get(rel);
  if (cached) {
    imgUrl.value = cached;
    return;
  }
  try {
    const url = await fetchOnce(rel);
    // 拉取期间 avatar 可能已变更，仅当仍是当前值时上屏
    if (props.avatar && extractRelPath(props.avatar) === rel) {
      imgUrl.value = url;
    }
  } catch {
    imgUrl.value = '';
  }
}

watch(
  () => props.avatar,
  (v) => {
    const val = (v || '').trim();
    if (!val) {
      imgUrl.value = '';
    } else if (isDirectUrl(val)) {
      imgUrl.value = val;
    } else {
      void load(extractRelPath(val));
    }
  },
  { immediate: true },
);
</script>

<style lang="sass" scoped>
// q-avatar 默认圆形（border-radius: 50%），覆盖为圆角正方形（随尺寸等比）
.user-avatar
  border-radius: 20%
  background: color-mix(in srgb, var(--color-accent) 16%, transparent)
  color: var(--color-accent)
  font-weight: 600
  border: 1px solid color-mix(in srgb, var(--color-accent) 30%, transparent)

  &__img
    object-fit: cover

  &__letter
    line-height: 1
</style>
