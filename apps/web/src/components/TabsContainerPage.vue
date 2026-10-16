<template>
  <q-page class="tabs-container-page">
    <q-card flat class="tabs-container-card">
      <q-tabs
        v-model="activeTab"
        dense
        no-caps
        outside-arrows
        mobile-arrows
        align="left"
        class="tabs-container-tabs"
        active-color="primary"
        indicator-color="primary"
      >
        <q-tab v-for="tab in tabs" :key="tab.name" :name="tab.name" :label="t(tab.labelKey)" :icon="tab.icon" />
      </q-tabs>

      <q-separator />

      <!-- keep-alive 必须加在 q-tab-panels 上：QTabPanel 非激活时会卸载插槽内容，
           写在面板内的 keep-alive 会随面板一起销毁，保活失效 -->
      <q-tab-panels v-model="activeTab" class="tabs-container-panels" animated keep-alive>
        <q-tab-panel v-for="tab in tabs" :key="tab.name" :name="tab.name">
          <!-- ErrorBoundary 兜底：异步 tab 组件 import 失败/渲染抛错时给出错误态+重试，
               而非静默空白（2026-09-12 报表中心「报表模板不显示」实证：dev 陈旧依赖图致
               子页 chunk 拒绝，原是零反馈全空白） -->
          <ErrorBoundary>
            <component :is="tab.component" v-if="loadedTabs[tab.name]" embedded v-bind="tab.componentProps ?? {}" />
          </ErrorBoundary>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, type Component } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import ErrorBoundary from './ErrorBoundary.vue';

// 通用 Tab 容器页：多子页整合为单菜单入口（tab 懒加载 + keep-alive 保活 + ?tab= 与 URL 同步，
// 旧路径经 routes.ts 静态 redirect 携带 tab query 进入）。模式源自 gateway/GatewayTabsPage。
// 子页需支持 embedded 属性（CrudTablePage 原生支持；自定义页根节点按 embedded 切换 div/q-page）。
export interface ContainerTab {
  name: string;
  labelKey: string;
  icon: string;
  component: Component;
  // 组件入参（可选）：同组件多实例挂载时传差异化 props，如设备分类 Tab 的 categoryCode（24-08 开发文档 §3.1）
  componentProps?: Record<string, unknown>;
}

const props = defineProps<{ tabs: ContainerTab[] }>();

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const activeTab = ref(props.tabs[0]?.name ?? '');
const loadedTabs = ref<Record<string, boolean>>({});

function syncTabFromQuery() {
  const q = route.query.tab as string;
  if (q && props.tabs.some((tab) => tab.name === q)) {
    activeTab.value = q;
  }
}

watch(activeTab, (tab) => {
  loadedTabs.value[tab] = true;
  router.replace({ query: { ...route.query, tab } });
});

// 同路由内 ?tab= 变化（如策略页跳转规则 tab）需响应式切换
watch(
  () => route.query.tab,
  () => syncTabFromQuery(),
);

onMounted(() => {
  syncTabFromQuery();
  loadedTabs.value[activeTab.value] = true;
});
</script>

<style lang="sass">
.tabs-container-page
  padding: 16px

.tabs-container-card
  border-radius: 12px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

.tabs-container-tabs
  min-height: 48px
  padding: 0 4px

  .q-tabs__content
    gap: 2px

  .q-tab
    min-height: 44px
    padding: 6px 14px
    border-radius: 8px 8px 0 0
    color: var(--color-text-secondary)
    transition: color 0.15s ease, background 0.15s ease

    .q-tab__icon
      font-size: 20px
      margin-bottom: 2px

    .q-tab__label
      font-size: 13px
      font-weight: 500
      letter-spacing: 0.02em
      line-height: 1.2
      white-space: nowrap

  .q-tab--active
    color: var(--q-primary)
    background: color-mix(in srgb, var(--q-primary) 10%, transparent)
    font-weight: 600

    .q-tab__label
      font-weight: 600

  .q-tabs__arrow
    color: var(--color-icon-muted)

.tabs-container-panels
  background: transparent
  .q-tab-panel
    padding: 0
</style>
