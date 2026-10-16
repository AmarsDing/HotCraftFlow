<template>
  <div class="role-tree-panel">
    <RoleTreeToolbar
      v-model:filter="filter"
      :placeholder="t('role.filterMenu')"
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
      label-key="label"
      children-key="children"
      class="role-tree"
      tick-strategy="leaf"
      :filter="filter"
      :filter-method="filterMethod"
      default-expand-all
    >
      <template #default-header="{ node }">
        <div class="row items-center no-wrap">
          <q-icon v-if="node.meta?.icon" :name="node.meta.icon" size="16px" class="role-menu-node__icon" />
          <span>{{ node.label }}</span>
          <!-- 类型徽标仅标注少数派（目录/按钮），MENU 为默认大多数不标注，避免满屏绿徽标噪声 -->
          <q-badge v-if="node.type === 'FOLDER'" color="blue" :label="t('menuEntity.typeFolder')" class="role-menu-node__type" />
          <q-badge v-else-if="node.type === 'BUTTON'" color="grey" :label="t('menuEntity.typeButton')" class="role-menu-node__type" />
        </div>
      </template>
    </q-tree>
    <!-- 粘性保存条：长树滚动到底部也可直接保存，未变更时禁用 -->
    <div class="role-save-bar">
      <span class="role-save-bar__info">{{ t('role.selectedCount', { count: ticked.length }) }}</span>
      <q-btn
        :loading="saving"
        :disable="!dirty"
        color="primary"
        icon="save"
        :label="t('role.saveMenus')"
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
import { useMenuStore, useRoleStore } from '../../../stores';
import type { MenuEntity, RoleEntity } from '../../../services';

const props = defineProps<{
  role: RoleEntity;
}>();

const emit = defineEmits<{
  (e: 'dirty-change', dirty: boolean): void;
}>();

const { t } = useI18n();
const menuStore = useMenuStore();
const roleStore = useRoleStore();

const ticked = ref<number[]>([]);
const snapshot = ref('');
const saving = ref(false);
const filter = ref('');
const treeRef = ref<QTree | null>(null);

interface MenuTreeNode extends MenuEntity {
  label: string;
  children?: MenuTreeNode[];
}

function buildNodes(menus: MenuEntity[]): MenuTreeNode[] {
  return menus.map((m) => ({
    ...m,
    label: m.meta?.title || m.name,
    children: m.children?.length ? buildNodes(m.children) : undefined,
  }));
}

const treeNodes = computed<MenuTreeNode[]>(() => buildNodes(menuStore.list));

// 叶子节点 id 集合：leaf tick 策略下只有叶子可独立勾选
const leafIds = computed<number[]>(() => {
  const ids: number[] = [];
  const walk = (nodes: MenuTreeNode[]) => {
    for (const n of nodes) {
      if (n.children?.length) walk(n.children);
      else ids.push(Number(n.id));
    }
  };
  walk(treeNodes.value);
  return ids;
});

// 父链映射：保存时补全祖先 id，否则后端 BuildTree 会把父级未授权的菜单提升为顶层，侧栏分组塌陷
const parentMap = computed<Map<number, number>>(() => {
  const map = new Map<number, number>();
  const walk = (nodes: MenuTreeNode[], parentId: number) => {
    for (const n of nodes) {
      map.set(Number(n.id), parentId);
      if (n.children?.length) walk(n.children, Number(n.id));
    }
  };
  walk(treeNodes.value, 0);
  return map;
});

function normalize(ids: number[]): number[] {
  const leafSet = new Set(leafIds.value);
  return ids.filter((id) => leafSet.has(Number(id))).map(Number);
}

// 仅在切换角色（id 变化）或菜单树异步加载完成（空→非空）时重置勾选集；
// 不因父级刷新详情（同 id 对象字段更新）而清空用户未保存的编辑
function syncFromRole() {
  ticked.value = normalize(props.role.menus ? [...props.role.menus] : []);
  snapshot.value = JSON.stringify([...ticked.value].sort((a, b) => a - b));
}
watch(() => props.role.id, syncFromRole, { immediate: true });
watch(leafIds, (ids, prev) => {
  if (prev.length === 0 && ids.length > 0) syncFromRole();
});

const dirty = computed(() => JSON.stringify([...ticked.value].sort((a, b) => a - b)) !== snapshot.value);
watch(dirty, (val) => emit('dirty-change', val));

function filterMethod(node: MenuTreeNode, keyword: string): boolean {
  return (node.label || '').toLowerCase().includes(keyword.toLowerCase());
}

function onSelectAll() {
  ticked.value = [...leafIds.value];
}

function onClearAll() {
  ticked.value = [];
}

function withAncestors(ids: number[]): number[] {
  const set = new Set(ids);
  for (const id of ids) {
    let pid = parentMap.value.get(id) ?? 0;
    while (pid !== 0 && !set.has(pid)) {
      set.add(pid);
      pid = parentMap.value.get(pid) ?? 0;
    }
  }
  return Array.from(set);
}

async function onSave() {
  if (!props.role.id) return;
  saving.value = true;
  try {
    await roleStore.update(props.role.id, { menus: withAncestors(ticked.value) });
    Notify.create({ type: 'positive', message: t('role.menuSaved') });
    snapshot.value = JSON.stringify([...ticked.value].sort((a, b) => a - b));
  } catch {
    // axios 拦截器已提示
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  if (menuStore.list.length === 0) void menuStore.fetchList({ noPaging: true });
});
</script>

<style lang="sass" scoped>
.role-tree-panel
  display: flex
  flex-direction: column
  gap: 12px

.role-menu-node__icon
  margin-right: 6px
  color: var(--color-text-secondary)

.role-menu-node__type
  margin-left: 8px
  font-size: 10px
  font-weight: 600

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
