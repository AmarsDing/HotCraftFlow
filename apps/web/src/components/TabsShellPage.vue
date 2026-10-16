<template>
  <q-page class="tabs-shell-page">
    <q-card flat class="tabs-shell-card">
      <div v-if="$slots.toolbar" class="tabs-shell-toolbar">
        <slot name="toolbar" />
      </div>
      <q-tabs
        v-model="activeTab"
        dense
        no-caps
        align="left"
        class="tabs-shell-tabs"
        active-color="primary"
        indicator-color="primary"
      >
        <q-tab v-for="tab in tabs" :key="tab.name" :name="tab.name" :label="t(tab.labelKey)" :icon="tab.icon" />
      </q-tabs>

      <q-separator />

      <!-- keep-alive 必须加在 q-tab-panels 上（Quasar 原生属性）：QTabPanel 非激活时会卸载插槽内容，
           把 keep-alive 写进插槽内会随面板一起销毁导致保活失效；懒加载仍由 v-if="loadedTabs[name]" 保证 -->
      <q-tab-panels v-model="activeTab" class="tabs-shell-panels" animated keep-alive>
        <q-tab-panel v-for="tab in tabs" :key="tab.name" :name="tab.name" class="q-pa-none">
          <component :is="tab.component" v-if="loadedTabs[tab.name]" embedded v-bind="tabProps" />
        </q-tab-panel>
      </q-tab-panels>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, type Component } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';

// 通用 Tab 容器页：tab 懒加载 + keep-alive 保活 + ?tab= 与 URL 同步（旧路径经静态 redirect 携带 tab query 进入）。
// 与 GatewayTabsPage 同构，额外支持 toolbar 插槽（如容器资源的共享端点选择器）与 tabProps 透传。
export interface TabsShellTab {
  name: string;
  labelKey: string;
  icon: string;
  component: Component;
}

const props = withDefaults(
  defineProps<{
    tabs: TabsShellTab[];
    // 透传给每个嵌入子页的 props（如 { endpointId }）
    tabProps?: Record<string, unknown>;
  }>(),
  { tabProps: undefined },
);

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

onMounted(() => {
  syncTabFromQuery();
  loadedTabs.value[activeTab.value] = true;
});
</script>

<style lang="sass">
.tabs-shell-page
  padding: 16px

.tabs-shell-card
  border-radius: 12px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

.tabs-shell-toolbar
  padding: 12px 16px 0

.tabs-shell-panels
  background: transparent
  .q-tab-panel
    padding: 0
</style>
