<template>
  <div class="role-tree-panel">
    <RoleTreeToolbar
      v-model:filter="filter"
      :placeholder="t('role.filterApi')"
      :selected="ticked.length"
      :total="leafIds.length"
      @expand="treeRef?.expandAll()"
      @collapse="treeRef?.collapseAll()"
      @select-all="onSelectAll"
      @clear-all="onClearAll"
    />
    <q-tree
      ref="treeRef"
      v-model:ticked="ticked"
      :nodes="treeNodes"
      node-key="id"
      label-key="name"
      children-key="children"
      class="role-tree"
      tick-strategy="leaf"
      :filter="filter"
      :filter-method="filterMethod"
      default-expand-all
    >
      <template #default-header="{ node }">
        <!-- 模块分组节点：模块名 + 接口数 + 已选进度 -->
        <template v-if="node.children">
          <q-icon name="folder_open" size="18px" class="q-mr-xs role-api-module__icon" />
          <span class="role-api-module__name">{{ node.name }}</span>
          <span class="role-api-module__count">{{ t('role.apiModuleCount', { count: node.children.length }) }}</span>
          <span class="role-api-module__picked" :class="{ 'role-api-module__picked--full': modulePicked(node) === node.children.length }">
            {{ t('role.modulePicked', { picked: modulePicked(node), total: node.children.length }) }}
          </span>
        </template>
        <!-- 接口叶子节点：方法徽标 + 路径 + 描述；下线/禁用资源灰显并带状态徽标（不过滤，保持绑定可管理） -->
        <template v-else>
          <q-badge :color="methodColor(node.method)" :label="node.method" class="role-api-method" />
          <span class="role-api-path" :class="{ 'role-api-path--muted': node.muted }">{{ node.path }}</span>
          <q-badge v-if="node.delisted" color="grey-7" :label="t('apiResource.delisted')" class="role-api-flag">
            <q-tooltip>{{ t('apiResource.delistedHint') }}</q-tooltip>
          </q-badge>
          <q-badge v-else-if="node.disabled" color="grey-6" :label="t('common.disabled')" class="role-api-flag" />
          <span v-if="node.desc" class="role-api-desc">{{ node.desc }}</span>
        </template>
      </template>
    </q-tree>
    <div class="role-save-bar">
      <span class="role-save-bar__info">{{ t('role.selectedCount', { count: ticked.length }) }}</span>
      <q-btn
        :loading="saving"
        :disable="!dirty"
        color="primary"
        icon="save"
        :label="t('role.saveApis')"
        unelevated
        @click="onSave"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTree } from 'quasar';
import RoleTreeToolbar from './RoleTreeToolbar.vue';
import { useApiResourceStore, useRoleStore } from '../../../stores';
import type { RoleEntity } from '../../../services';
import { methodColor } from '../../../utils/http-method';

const props = defineProps<{
  role: RoleEntity;
}>();

const emit = defineEmits<{
  (e: 'dirty-change', dirty: boolean): void;
}>();

const { t } = useI18n();
const apiResourceStore = useApiResourceStore();
const roleStore = useRoleStore();

const ticked = ref<number[]>([]);
const snapshot = ref('');
const saving = ref(false);
const filter = ref('');
const treeRef = ref<QTree | null>(null);

interface ApiLeafNode {
  id: number;
  name: string;
  method: string;
  path: string;
  desc: string;
  delisted: boolean;
  disabled: boolean;
  muted: boolean;
}
interface ApiModuleNode {
  id: number;
  name: string;
  children: ApiLeafNode[];
}

// 按 module 分组聚合 API 资源；模块节点 id 从 1000000 起合成，leaf tick 策略保证合成 id 不会入库
const treeNodes = computed<ApiModuleNode[]>(() => {
  const moduleMap = new Map<string, ApiModuleNode>();
  let moduleId = 1000000;
  for (const api of apiResourceStore.list) {
    const mod = api.module || 'Default';
    if (!moduleMap.has(mod)) {
      moduleMap.set(mod, { id: moduleId++, name: mod, children: [] });
    }
    const delisted = api.delisted === true;
    const disabled = api.status === 'OFF';
    moduleMap.get(mod)!.children.push({
      id: Number(api.id),
      name: `${api.method || ''} ${api.path || ''}`.trim(),
      method: (api.method || '').toUpperCase(),
      path: api.path || '',
      desc: api.description || '',
      delisted,
      disabled,
      muted: delisted || disabled,
    });
  }
  return Array.from(moduleMap.values());
});

const leafIds = computed<number[]>(() => treeNodes.value.flatMap((m) => m.children.map((c) => c.id)));

// 模块内已选接口数：模块节点实时显示授权进度（已选 x/y）
const tickedSet = computed(() => new Set(ticked.value.map(Number)));
function modulePicked(node: ApiModuleNode): number {
  return node.children.filter((c) => tickedSet.value.has(c.id)).length;
}

// 仅在切换角色（id 变化）或 API 资源异步加载完成（空→非空）时重置勾选集；
// 不因父级刷新详情而清空用户未保存的编辑
function syncFromRole() {
  const leafSet = new Set(leafIds.value);
  ticked.value = (props.role.apis ?? []).map(Number).filter((id) => leafSet.has(id));
  snapshot.value = JSON.stringify([...ticked.value].sort((a, b) => a - b));
}
watch(() => props.role.id, syncFromRole, { immediate: true });
watch(leafIds, (ids, prev) => {
  if (prev.length === 0 && ids.length > 0) syncFromRole();
});

const dirty = computed(() => JSON.stringify([...ticked.value].sort((a, b) => a - b)) !== snapshot.value);
watch(dirty, (val) => emit('dirty-change', val));

// 方法/路径/描述任一命中即保留节点
function filterMethod(node: ApiLeafNode, keyword: string): boolean {
  const kw = keyword.toLowerCase();
  return (node.name || '').toLowerCase().includes(kw) || (node.desc || '').toLowerCase().includes(kw);
}

function onSelectAll() {
  ticked.value = [...leafIds.value];
}

function onClearAll() {
  ticked.value = [];
}

async function onSave() {
  if (!props.role.id) return;
  saving.value = true;
  try {
    await roleStore.update(props.role.id, { apis: ticked.value });
    Notify.create({ type: 'positive', message: t('role.apiSaved') });
    snapshot.value = JSON.stringify([...ticked.value].sort((a, b) => a - b));
  } catch {
    // axios 拦截器已提示
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  if (apiResourceStore.list.length === 0) void apiResourceStore.fetchList({ noPaging: true });
});
</script>

<style lang="sass" scoped>
.role-tree-panel
  display: flex
  flex-direction: column
  gap: 12px

.role-api-module__icon
  color: var(--color-text-secondary)

.role-api-module__name
  font-weight: 600
  color: var(--color-text-heading)

.role-api-module__count
  margin-left: 8px
  font-size: 11px
  color: var(--color-text-tertiary)

.role-api-module__picked
  margin-left: 6px
  font-size: 10px
  font-weight: 600
  padding: 1px 8px
  border-radius: 8px
  color: var(--color-text-secondary)
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

  &--full
    color: var(--color-accent)
    border-color: color-mix(in srgb, var(--color-accent) 45%, transparent)
    background: color-mix(in srgb, var(--color-accent) 10%, transparent)

.role-api-method
  min-width: 52px
  justify-content: center
  font-weight: 700
  font-size: 10px
  letter-spacing: 0.4px
  margin-right: 8px

.role-api-path
  font-family: monospace
  font-size: 12px
  color: var(--color-text-primary)

.role-api-desc
  margin-left: 8px
  font-size: 12px
  color: var(--color-text-tertiary)

.role-save-bar
  position: sticky
  bottom: 0
  z-index: 2
  display: flex
  align-items: center
  justify-content: space-between
  gap: 12px
  padding: 10px 12px
  margin: 0 -4px -4px
  border-radius: 12px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

  &__info
    font-size: 12px
    font-weight: 600
    color: var(--color-text-secondary)
</style>
