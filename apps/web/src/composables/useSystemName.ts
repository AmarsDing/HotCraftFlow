import { computed, watch, onMounted, type ComputedRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { useSystemNameStore } from '../stores/system-name';

// useSystemName：系统名称显示口径统一入口。
// displayName = DB 配置值（优先）→ i18n 兜底（未拉取/拉取失败/值为空）。
// 组件挂载即幂等拉取一次；浏览器页签标题（syncDocumentTitle=true 时）随名称同步。
export function useSystemName(opts: { syncDocumentTitle?: boolean } = {}): {
  displayName: ComputedRef<string>;
} {
  const { t } = useI18n();
  const store = useSystemNameStore();

  onMounted(() => {
    void store.fetchSystemName();
  });

  const displayName = computed(() => store.name || t('common.appTitle'));

  if (opts.syncDocumentTitle) {
    watch(
      displayName,
      (v) => {
        if (typeof document !== 'undefined') document.title = v;
      },
      { immediate: true },
    );
  }

  return { displayName };
}
