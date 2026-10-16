<template>
  <!-- 命中错误：渲染可恢复的错误占位，而非静默空白（root cause：异步 tab/子组件加载失败原先是零反馈全空白） -->
  <div v-if="error" class="err-boundary column items-center justify-center q-pa-xl">
    <q-icon name="cloud_off" size="42px" class="err-boundary__icon q-mb-sm" />
    <div class="text-body1 text-weight-medium q-mb-xs">{{ title }}</div>
    <div class="text-caption err-boundary__msg q-mb-md">{{ errorMessage }}</div>
    <div class="row q-gutter-sm">
      <q-btn unelevated color="primary" icon="refresh" :label="t('common.retry')" @click="retry" />
      <q-btn outline icon="restart_alt" :label="t('common.reloadPage')" @click="reloadPage" />
    </div>
  </div>
  <!-- 正常：宿主 div 透传插槽（display:contents 不产生布局盒，等同无包装）。
       retrySeq 作 key 挂在宿主元素上——key 必须落在真实元素才会重建整棵子树、令异步组件重发 import；
       直接挂在 <slot> 上无效（插槽 vnode 归父组件渲染上下文所有，key 不随边界重建） -->
  <div v-else :key="retrySeq" class="err-boundary-host">
    <slot />
  </div>
</template>

<script setup lang="ts">
// 通用错误边界（渲染级 + 异步组件加载级）。
//
// 背景（2026-09-12 报表中心「报表模板不显示」排查）：
// TabsContainerPage 的 tab 子页经 defineAsyncComponent 懒加载。dev 场景下 Vite 依赖预打包
// 失效（新增依赖/配置重载后 ?v= 哈希漂移）、或子组件 setup 抛错时，异步组件 import 拒绝 /
// 渲染失败会被 Vue 静默吞掉——tab 栏照常渲染、面板全空白、控制台外零反馈，排障只能靠猜。
//
// 本组件以 onErrorCaptured 捕获后代错误（含异步组件 loader 拒绝，Vue 3 会向最近错误边界冒泡），
// 返回 false 阻断继续上报，改渲染明确错误态 + 重试入口。重试通过自增 key 强制子树重建并重发 import。
import { ref, computed, watch, onErrorCaptured } from 'vue';
import { useI18n } from 'vue-i18n';

// resetKey：外部上下文标识（如 route.fullPath），变化时清空错误态并自动重试插槽。
// 与 :key 的区别：仅复位边界内部状态、不重建 ErrorBoundary 自身——MainLayout 用它
// 在换路由时复位，避免 :key 重建子树击穿内层 keep-alive 保活。
const props = withDefaults(defineProps<{ title?: string; resetKey?: string }>(), { title: '', resetKey: undefined });

const { t } = useI18n();

const error = ref<unknown>(null);
// 重试序号：变化即重建插槽子树（异步组件重新 import），配合 v-else 的 :key
const retrySeq = ref(0);

// resetKey 变化 = 上下文已切换（如换路由）：仅清错误态，不动 retrySeq。
// 错误期间插槽本就未渲染，清 error 即让新页面自然重挂；不 bump retrySeq 是为了
// 不重建宿主 div——MainLayout 依赖内层 keep-alive 保活，宿主重建会击穿缓存。
watch(
  () => props.resetKey,
  () => {
    error.value = null;
  },
);

const title = computed(() => props.title || t('common.loadFailed'));

// chunk 加载失败（Failed to fetch dynamically imported module / Loading chunk failed）
// 是 dev 陈旧依赖图最典型的错误，文案上单独提示可硬刷新
const errorMessage = computed(() => {
  const e = error.value;
  const msg = e instanceof Error ? e.message : String(e ?? '');
  if (/dynamically imported module|Loading chunk|Importing a module script failed/i.test(msg)) {
    return t('common.chunkLoadFailedTip');
  }
  return msg || t('common.unknownError');
});

// 返回 false：错误已被边界消化，不再向上（app.config.errorHandler）冒泡，避免重复上报
onErrorCaptured((err) => {
  error.value = err;
  return false;
});

function retry() {
  error.value = null;
  retrySeq.value += 1;
}

function reloadPage() {
  window.location.reload();
}
</script>

<style lang="sass">
// 宿主：display:contents 使自身不生成布局盒，插槽内容布局表现与无包装一致
//（flex/grid 子项、q-page 拉伸等不受影响）；仅作为 key 重建与错误切换的挂载点
.err-boundary-host
  display: contents

.err-boundary
  min-height: 240px
  color: var(--color-text-secondary)

  &__icon
    opacity: 0.55

  &__msg
    max-width: 560px
    text-align: center
    word-break: break-all
    opacity: 0.8
</style>
