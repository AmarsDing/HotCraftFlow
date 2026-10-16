<template>
  <div class="role-tree-toolbar row items-center no-wrap q-gutter-xs">
    <q-input
      :model-value="filter"
      dense
      outlined
      clearable
      debounce="150"
      :placeholder="placeholder"
      class="role-tree-toolbar__search col"
      @update:model-value="(val) => emit('update:filter', String(val ?? ''))"
    >
      <template #prepend>
        <q-icon name="search" size="18px" />
      </template>
    </q-input>
    <span class="role-tree-toolbar__count">
      {{ total != null ? t('role.selectedOfTotal', { selected, total }) : t('role.selectedCount', { count: selected }) }}
    </span>
    <q-btn flat dense round icon="unfold_more" size="sm" @click="emit('expand')">
      <q-tooltip>{{ t('role.expandAll') }}</q-tooltip>
    </q-btn>
    <q-btn flat dense round icon="unfold_less" size="sm" @click="emit('collapse')">
      <q-tooltip>{{ t('role.collapseAll') }}</q-tooltip>
    </q-btn>
    <q-btn flat dense no-caps size="sm" color="primary" :label="t('role.selectAll')" @click="emit('select-all')" />
    <q-btn flat dense no-caps size="sm" :label="t('role.clearAll')" @click="emit('clear-all')" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

// 菜单/API 授权树通用工具栏：搜索过滤 + 已选计数 + 展开收起 + 全选清空
// total 传入时显示「已选 x/y」比例，未传则仅显示已选数
defineProps<{
  filter: string;
  placeholder: string;
  selected: number;
  total?: number;
}>();

const emit = defineEmits<{
  (e: 'update:filter', val: string): void;
  (e: 'expand'): void;
  (e: 'collapse'): void;
  (e: 'select-all'): void;
  (e: 'clear-all'): void;
}>();

const { t } = useI18n();
</script>

<style lang="sass" scoped>
.role-tree-toolbar
  &__count
    flex-shrink: 0
    font-size: 12px
    font-weight: 600
    color: var(--color-text-secondary)
    padding: 2px 10px
    border-radius: 10px
    background: var(--glass-surface)
    white-space: nowrap
</style>
