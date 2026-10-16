<template>
  <q-page class="crud-page q-pa-lg menu-page">
    <!-- Header -->
    <div class="row items-center q-mb-md">
      <div class="text-h6 text-weight-bold">{{ t('menuEntity.title') }}</div>
      <q-space />
      <q-input
        v-model="filterText"
        :placeholder="t('menuEntity.searchPlaceholder')"
        dense
        outlined
        clearable
        class="menu-search q-mr-sm"
      >
        <template #prepend>
          <q-icon name="search" />
        </template>
      </q-input>
      <q-btn flat dense round icon="refresh" @click="loadData" class="q-mr-sm">
        <q-tooltip>{{ t('common.retry') }}</q-tooltip>
      </q-btn>
      <q-btn unelevated color="primary" icon="add" :label="t('common.create')" @click="onCreate" />
    </div>

    <!-- 统计条（少数派信息：类型分布 + 禁用数） -->
    <div class="row items-center q-mb-md q-gutter-sm">
      <q-chip dense outline color="blue" icon="folder" :label="t('menuEntity.statFolder', { n: typeStats.FOLDER })" />
      <q-chip dense outline color="green" icon="article" :label="t('menuEntity.statMenu', { n: typeStats.MENU })" />
      <q-chip dense outline color="grey" icon="smart_button" :label="t('menuEntity.statButton', { n: typeStats.BUTTON })" />
      <q-chip v-if="typeStats.OFF > 0" dense outline color="warning" icon="visibility_off" :label="t('menuEntity.statDisabled', { n: typeStats.OFF })" />
    </div>

    <!-- Column header -->
    <div class="menu-row menu-row--head">
      <div class="menu-col-name">{{ t('menuEntity.colTitle') }}</div>
      <div class="menu-col-type">{{ t('menuEntity.type') }}</div>
      <div class="menu-col-path">{{ t('menuEntity.path') }}</div>
      <div class="menu-col-sort">{{ t('common.sort') }}</div>
      <div class="menu-col-status">{{ t('common.status') }}</div>
      <div class="menu-col-actions">{{ t('common.actions') }}</div>
    </div>

    <!-- Tree list -->
    <q-list separator>
      <q-item
        v-for="row in flatRows"
        :key="row.id"
        tag="div"
        class="menu-row menu-row--item"
        :style="{ paddingLeft: 16 + row.depth * 24 + 'px' }"
      >
        <div class="menu-col-name row items-center no-wrap">
          <q-btn
            v-if="row.hasChildren"
            size="xs"
            flat
            dense
            round
            :icon="row.expanded ? 'expand_more' : 'chevron_right'"
            @click="toggle(row.id)"
            class="q-mr-xs"
          />
          <span v-else class="menu-leaf q-mr-sm" />
          <q-icon v-if="row.icon" :name="row.icon" size="20px" class="q-mr-sm text-grey-6" />
          <span class="text-weight-medium">{{ row.label }}</span>
          <q-badge
            v-if="roleCountFor(row) > 0"
            color="info"
            :label="roleCountFor(row)"
            class="q-ml-sm cursor-pointer"
            @click.stop="openRoleDrawer(row)"
          >
            <q-tooltip>{{ t('menuEntity.refRolesTooltip', { n: roleCountFor(row) }) }}</q-tooltip>
          </q-badge>
        </div>
        <div class="menu-col-type">
          <q-badge
            :color="typeColor(row.type)"
            :label="typeLabel(row.type)"
            outline
          />
        </div>
        <div class="menu-col-path text-grey-7 ellipsis">{{ row.path || '—' }}</div>
        <div class="menu-col-sort text-grey-7">{{ row.source.meta?.order ?? 0 }}</div>
        <div class="menu-col-status">
          <q-toggle
            :model-value="row.status === 'ON'"
            :disable="statusSaving.has(row.id)"
            color="positive"
            @update:model-value="(val) => onToggleStatus(row, !!val)"
            dense
          >
            <q-tooltip>
              {{ row.status === 'ON' ? t('common.enabled') : t('common.disabled') }}
            </q-tooltip>
          </q-toggle>
        </div>
        <div class="menu-col-actions row items-center no-wrap justify-end">
          <q-btn size="sm" flat round dense color="primary" icon="add" @click="onAddChild(row)">
            <q-tooltip>{{ t('menuEntity.addChild') }}</q-tooltip>
          </q-btn>
          <q-btn size="sm" flat round dense color="grey-7" icon="edit" @click="onEdit(row)">
            <q-tooltip>{{ t('common.edit') }}</q-tooltip>
          </q-btn>
          <q-btn size="sm" flat round dense color="negative" icon="delete" @click="askDelete(row)">
            <q-tooltip>{{ t('common.delete') }}</q-tooltip>
          </q-btn>
        </div>
      </q-item>
      <q-item v-if="flatRows.length === 0" class="menu-row menu-row--empty">
        <div class="full-width text-center text-grey-7 q-pa-md">{{ t('common.noData') }}</div>
      </q-item>
    </q-list>

    <!-- Form drawer -->
    <FormDrawer
      v-model="drawerOpen"
      :title="drawerTitle"
      :saving="saving"
      @submit="onSubmit"
    >
      <!-- 组 1：基本信息 -->
      <div class="form-group-title">
        <q-icon name="info" size="16px" class="form-group-icon" />
        <span>{{ t('menuEntity.groupBasic') }}</span>
        <div class="form-group-line" />
      </div>
      <q-input
        v-model="form.name"
        :placeholder="t('menuEntity.name')"
        dense
        outlined
        :rules="[(val) => !!val || t('common.required')]"
      />
      <q-input v-model="form.title" :placeholder="t('menuEntity.title2')" dense outlined />
      <q-select
        v-model="form.type"
        :placeholder="t('menuEntity.type')"
        :options="typeOptions"
        emit-value
        map-options
        dense
        outlined
      />
      <q-select
        v-model="form.parentId"
        :placeholder="t('menuEntity.parentId')"
        :options="parentMenuOptions"
        :display-value="parentSelectDisplay"
        option-value="value"
        option-label="label"
        emit-value
        map-options
        dense
        outlined
        clearable
      />
      <q-input
        v-model.number="form.sort"
        :placeholder="t('common.sort')"
        dense
        outlined
        type="number"
      />
      <q-toggle
        :model-value="form.status === 'ON'"
        :label="form.status === 'ON' ? t('common.enabled') : t('common.disabled')"
        @update:model-value="(val) => (form.status = val ? 'ON' : 'OFF')"
      />

      <!-- 组 2：路由配置（按钮无路由属性） -->
      <template v-if="form.type !== 'BUTTON'">
        <div class="form-group-title">
          <q-icon name="route" size="16px" class="form-group-icon" />
          <span>{{ t('menuEntity.groupRoute') }}</span>
          <div class="form-group-line" />
        </div>
        <q-input v-model="form.path" :placeholder="t('menuEntity.path')" dense outlined />
        <q-select
          v-if="form.type !== 'BUTTON'"
          v-model="form.component"
          :placeholder="t('menuEntity.componentSelectPlaceholder')"
          :options="componentOptions"
          :hint="t('menuEntity.componentSelectHint')"
          use-input
          new-value-mode="add-unique"
          emit-value
          map-options
          dense
          outlined
          @filter="filterComponents"
        />
        <q-input
          v-model="form.redirect"
          :placeholder="t('menuEntity.redirect')"
          :hint="form.type === 'FOLDER' ? t('menuEntity.redirectFolderHint') : undefined"
          dense
          outlined
        />
        <q-input v-model="form.alias" :placeholder="t('menuEntity.alias')" dense outlined />
        <q-input v-model="form.link" :placeholder="t('menuEntity.link')" dense outlined />
        <q-input v-if="form.type === 'MENU'" v-model="form.iframeSrc" :placeholder="t('menuEntity.iframeSrc')" dense outlined />
      </template>

      <!-- 组 3：显示控制（按钮无显示属性） -->
      <template v-if="form.type !== 'BUTTON'">
        <div class="form-group-title">
          <q-icon name="visibility" size="16px" class="form-group-icon" />
          <span>{{ t('menuEntity.groupDisplay') }}</span>
          <div class="form-group-line" />
        </div>
        <IconPicker v-model="form.icon" :placeholder="t('menuEntity.icon')" />
        <div class="row q-col-gutter-x-md">
          <q-toggle v-model="form.isHidden" :label="t('menuEntity.isHidden')" class="col-6" />
          <q-toggle v-model="form.hideInTab" :label="t('menuEntity.hideInTab')" class="col-6" />
          <q-toggle v-model="form.hideInBreadcrumb" :label="t('menuEntity.hideInBreadcrumb')" class="col-6" />
          <q-toggle v-model="form.hideChildrenInMenu" :label="t('menuEntity.hideChildrenInMenu')" class="col-6" />
          <q-toggle v-if="form.type === 'MENU'" v-model="form.affixTab" :label="t('menuEntity.affixTab')" class="col-6" />
          <q-toggle v-if="form.type === 'MENU'" v-model="form.openInNewWindow" :label="t('menuEntity.openInNewWindow')" class="col-6" />
          <q-toggle v-if="form.type === 'MENU'" v-model="form.isCache" :label="t('menuEntity.isCache')" class="col-6" />
        </div>
      </template>

      <!-- 组 4：权限（目录不携带权限码） -->
      <template v-if="form.type !== 'FOLDER'">
        <div class="form-group-title">
          <q-icon name="lock" size="16px" class="form-group-icon" />
          <span>{{ t('menuEntity.groupPermission') }}</span>
          <div class="form-group-line" />
        </div>
        <q-select
          v-model="form.authorities"
          :placeholder="t('menuEntity.permissions')"
          :options="authorityCandidates"
          multiple
          use-chips
          use-input
          new-value-mode="add-unique"
          dense
          outlined
        />
        <div v-if="authorityTopCandidates.length" class="q-mt-xs">
          <div class="text-caption text-grey-7 q-mb-xs">{{ t('menuEntity.authorityCandidatesHint') }}</div>
          <q-chip
            v-for="auth in authorityTopCandidates"
            :key="auth"
            dense
            clickable
            outline
            color="primary"
            size="sm"
            :label="auth"
            @click="addAuthority(auth)"
          />
        </div>
      </template>
    </FormDrawer>

    <!-- 引用角色清单抽屉（右侧滑入） -->
    <q-dialog v-model="roleDrawerOpen" position="right">
      <q-card style="width: 340px; max-width: 90vw">
        <q-card-section class="row items-center q-pb-none">
          <div class="text-h6">{{ t('menuEntity.refRolesTitle') }}</div>
          <q-space />
          <q-btn icon="close" flat round dense v-close-popup />
        </q-card-section>
        <q-card-section>
          <div class="text-caption text-grey-7 q-mb-sm">{{ roleDrawerMenuLabel }}</div>
          <q-list v-if="selectedRoleNames.length" dense separator>
            <q-item v-for="name in selectedRoleNames" :key="name" class="q-px-xs">
              <q-item-section avatar style="min-width: 28px">
                <q-icon name="admin_panel_settings" size="18px" color="primary" />
              </q-item-section>
              <q-item-section>{{ name }}</q-item-section>
            </q-item>
          </q-list>
          <div v-else class="text-grey-7 q-pa-sm">{{ t('menuEntity.noRefRoles') }}</div>
        </q-card-section>
      </q-card>
    </q-dialog>

    <!-- Delete confirm dialog（含影响面评估） -->
    <q-dialog v-model="confirmDeleteOpen">
      <q-card style="min-width: 360px">
        <q-card-section>
          <div class="text-h6">{{ t('common.confirmDelete') }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ pendingDelete?.label }}</div>
          <div class="q-mt-sm row items-center no-wrap">
            <q-spinner-dots v-if="impactLoading" size="20px" color="primary" class="q-mr-sm" />
            <span v-if="impactLoading" class="text-grey-7">{{ t('menuEntity.impactLoading') }}</span>
            <span v-else-if="impactText" class="text-body2">{{ impactText }}</span>
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="t('common.cancel')" v-close-popup />
          <q-btn unelevated color="negative" :label="t('common.confirm')" :disable="impactLoading" @click="onDelete" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Disable confirm dialog（含影响面评估） -->
    <q-dialog v-model="confirmDisableOpen">
      <q-card style="min-width: 360px">
        <q-card-section>
          <div class="text-h6">{{ t('menuEntity.confirmDisable') }}</div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ pendingDisable?.label }}</div>
          <div class="q-mt-sm row items-center no-wrap">
            <q-spinner-dots v-if="impactLoading" size="20px" color="primary" class="q-mr-sm" />
            <span v-if="impactLoading" class="text-grey-7">{{ t('menuEntity.impactLoading') }}</span>
            <span v-else-if="impactText" class="text-body2">{{ impactText }}</span>
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="t('common.cancel')" v-close-popup />
          <q-btn unelevated color="warning" :label="t('common.confirm')" :disable="impactLoading" @click="onDisableConfirm" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import FormDrawer from '../../components/FormDrawer.vue';
import IconPicker from '../../components/IconPicker.vue';
import { useMenuStore, useRoleStore } from '../../stores';
import { menuService } from '../../services';
import type { MenuEntity, MenuMeta } from '../../services';
import { getComponentMapOptions } from '../../router/componentMap';

const { t } = useI18n();
const menuStore = useMenuStore();
const roleStore = useRoleStore();

interface MenuForm {
  id?: string;
  name: string;
  title: string;
  type: string;
  icon: string;
  path: string;
  component: string;
  redirect: string;
  alias: string;
  link: string;
  iframeSrc: string;
  parentId: string;
  sort: number;
  status: string;
  isHidden: boolean;
  hideInTab: boolean;
  hideInBreadcrumb: boolean;
  hideChildrenInMenu: boolean;
  affixTab: boolean;
  openInNewWindow: boolean;
  isCache: boolean;
  authorities: string[];
}

interface FlatRow {
  id: string;
  label: string;
  icon?: string;
  type?: string;
  status?: string;
  path?: string;
  depth: number;
  hasChildren: boolean;
  expanded: boolean;
  source: MenuEntity;
}

function defaultForm(): MenuForm {
  return {
    name: '',
    title: '',
    type: 'MENU',
    icon: '',
    path: '',
    component: '',
    redirect: '',
    alias: '',
    link: '',
    iframeSrc: '',
    parentId: '',
    sort: 0,
    status: 'ON',
    isHidden: false,
    hideInTab: false,
    hideInBreadcrumb: false,
    hideChildrenInMenu: false,
    affixTab: false,
    openInNewWindow: false,
    isCache: false,
    authorities: [],
  };
}

/** 列表/下拉展示名：优先显示标题，没有再回退路由名 */
function menuDisplayName(m: MenuEntity): string {
  return (m.meta?.title || m.name || '').trim();
}

/** proto int32 运行时是 number；q-select map-options 用 ===，必须与 option.value 同为 string */
function normalizeMenuId(id: string | number | undefined | null): string {
  if (id == null || id === '') return '';
  const s = String(id);
  return s === '0' ? '' : s;
}

const form = ref<MenuForm>(defaultForm());
const drawerOpen = ref(false);
const saving = ref(false);
const isEdit = ref(false);
const confirmDeleteOpen = ref(false);
const pendingDelete = ref<FlatRow | null>(null);
const confirmDisableOpen = ref(false);
const pendingDisable = ref<FlatRow | null>(null);
// 影响面评估（删除/禁用共用：对话框打开时只存在一种待办）
const impactLoading = ref(false);
const impactInfo = ref<{ roleCount?: number; userCount?: number } | null>(null);
// 菜单→角色引用关系（徽标 + 抽屉）
const roleRefMap = ref<Map<number, number[]>>(new Map());
const roleDrawerOpen = ref(false);
const roleDrawerMenuId = ref<number | null>(null);
const roleDrawerMenuLabel = ref('');
const expanded = ref<Set<string>>(new Set());
// 状态切换防连点（请求中行 id）
const statusSaving = ref<Set<string>>(new Set());
// 本地过滤关键字（名称/标题/路径，命中分支自动展开）
const filterText = ref('');

// 类型分布统计（统计条：禁用数仅 >0 显示，少数派原则）
const typeStats = computed(() => {
  const stats = { FOLDER: 0, MENU: 0, BUTTON: 0, OFF: 0 };
  const walk = (menus: MenuEntity[]) => {
    for (const m of menus) {
      if (m.type === 'FOLDER') stats.FOLDER++;
      else if (m.type === 'MENU') stats.MENU++;
      else if (m.type === 'BUTTON') stats.BUTTON++;
      if (m.status === 'OFF') stats.OFF++;
      if (m.children?.length) walk(m.children);
    }
  };
  walk(menuStore.list);
  return stats;
});

const drawerTitle = computed(() => (isEdit.value ? t('common.edit') : t('common.create')));

const typeOptions = computed(() => [
  { label: t('menuEntity.typeFolder'), value: 'FOLDER' },
  { label: t('menuEntity.typeMenu'), value: 'MENU' },
  { label: t('menuEntity.typeButton'), value: 'BUTTON' },
]);

// 前端组件候选（componentMap 登记清单；支持关键字过滤，也允许输入未登记的新组件路径）
const allComponentOptions = getComponentMapOptions();
const componentOptions = ref<string[]>(allComponentOptions);

function filterComponents(val: string, update: (fn: () => void) => void) {
  update(() => {
    const kw = val.trim().toLowerCase();
    componentOptions.value = kw
      ? allComponentOptions.filter((o) => o.toLowerCase().includes(kw))
      : allComponentOptions;
  });
}

// 权限码候选：聚合全部菜单 meta.authority 按频次降序（复用既有权限码，保持命名一致）
const authorityRanked = computed<string[]>(() => {
  const freq = new Map<string, number>();
  const walk = (menus: MenuEntity[]) => {
    for (const m of menus) {
      for (const a of m.meta?.authority ?? []) {
        freq.set(a, (freq.get(a) ?? 0) + 1);
      }
      if (m.children?.length) walk(m.children);
    }
  };
  walk(menuStore.list);
  return [...freq.entries()].sort((a, b) => b[1] - a[1]).map(([code]) => code);
});

// 下拉候选：剔除已选
const authorityCandidates = computed(() =>
  authorityRanked.value.filter((code) => !form.value.authorities.includes(code)),
);

// 快捷 chips：频次 top8，剔除已选
const authorityTopCandidates = computed(() => authorityCandidates.value.slice(0, 8));

function addAuthority(code: string) {
  if (!form.value.authorities.includes(code)) {
    form.value.authorities = [...form.value.authorities, code];
  }
}

// 拍扁菜单树为可见行（含层级和展开态）；搜索时按名称/路径过滤并自动展开命中分支
const flatRows = computed<FlatRow[]>(() => {
  const kw = filterText.value.trim().toLowerCase();
  const out: FlatRow[] = [];
  const walk = (menus: MenuEntity[], depth: number, forceVisible: boolean) => {
    for (const m of menus) {
      const id = m.id ? String(m.id) : '';
      const hasChildren = !!m.children?.length;
      const selfHit =
        !kw ||
        (m.meta?.title || '').toLowerCase().includes(kw) ||
        (m.name || '').toLowerCase().includes(kw) ||
        (m.path || '').toLowerCase().includes(kw);
      // 搜索态：自身或祖先命中即显示；命中分支全量展开
      if (kw && !selfHit && !forceVisible) {
        // 后代可能命中：先递归收集，若子树有命中则本节点也要显示
        if (hasChildren && m.children) {
          const before = out.length;
          walk(m.children, depth + 1, false);
          // 子树有新增行说明存在命中后代，本节点补插到子树行之前
          if (out.length > before) {
            out.splice(before, 0, {
              id,
              label: menuDisplayName(m),
              icon: m.meta?.icon,
              type: m.type,
              status: m.status,
              path: m.path,
              depth,
              hasChildren,
              expanded: true,
              source: m,
            });
          }
        }
        continue;
      }
      const isOpen = kw ? true : expanded.value.has(id);
      out.push({
        id,
        label: menuDisplayName(m),
        icon: m.meta?.icon,
        type: m.type,
        status: m.status,
        path: m.path,
        depth,
        hasChildren,
        expanded: isOpen && hasChildren,
        source: m,
      });
      if (isOpen && hasChildren && m.children) {
        walk(m.children, depth + 1, kw ? selfHit : false);
      }
    }
  };
  walk(menuStore.list, 0, false);
  return out;
});

// 上级菜单下拉：排除自身及全部后代，防止循环父子；BUTTON 不可作为父级
const parentMenuOptions = computed(() => {
  const options: { label: string; value: string }[] = [];
  const skipId = isEdit.value && form.value.id ? form.value.id : undefined;
  // 收集自身后代 id（编辑态）：排除自身+后代子树
  const excluded = new Set<string>(skipId ? [String(skipId)] : []);
  if (skipId) {
    const collect = (menus: MenuEntity[]): boolean => {
      for (const m of menus) {
        if (String(m.id) === String(skipId)) {
          const mark = (ns: MenuEntity[]) => {
            for (const c of ns) {
              excluded.add(String(c.id));
              if (c.children?.length) mark(c.children);
            }
          };
          mark(m.children || []);
          return true;
        }
        if (m.children?.length && collect(m.children)) return true;
      }
      return false;
    };
    collect(menuStore.list);
  }
  const walk = (menus: MenuEntity[], depth = 0) => {
    menus.forEach((m) => {
      if (!m.id || m.type === 'BUTTON') return;
      if (excluded.has(String(m.id))) return;
      const indent = depth > 0 ? '  '.repeat(depth) + '└ ' : '';
      options.push({
        label: `${indent}${menuDisplayName(m)}`,
        value: String(m.id),
      });
      if (m.children?.length) walk(m.children, depth + 1);
    });
  };
  walk(menuStore.list);
  return options;
});

// 选中项去掉树缩进，只显示名称（避免 map-options 类型不一致时露出 id/路由名）
const parentSelectDisplay = computed(() => {
  const pid = form.value.parentId;
  if (pid === '' || pid == null) return undefined;
  const found = parentMenuOptions.value.find((o) => String(o.value) === String(pid));
  return found ? found.label.replace(/^[\s└│├─]+/, '').trim() : undefined;
});

function typeColor(type?: string): string {
  if (type === 'FOLDER') return 'blue';
  if (type === 'MENU') return 'green';
  if (type === 'BUTTON') return 'grey';
  return 'grey';
}

function typeLabel(type?: string): string {
  if (type === 'FOLDER') return t('menuEntity.typeFolder');
  if (type === 'MENU') return t('menuEntity.typeMenu');
  if (type === 'BUTTON') return t('menuEntity.typeButton');
  return type || '';
}

function toggle(id: string) {
  if (expanded.value.has(id)) expanded.value.delete(id);
  else expanded.value.add(id);
}

async function loadData() {
  await menuStore.fetchList({ noPaging: true });
  // 默认所有根节点展开
  menuStore.list.forEach((m) => m.id && expanded.value.add(String(m.id)));
  // 并行拉取角色列表 + 菜单角色引用关系
  await Promise.all([
    roleStore.fetchList({ noPaging: true }).catch(() => {/* 静默降级 */}),
    loadRoleRefs().catch(() => {/* 静默降级 */}),
  ]);
}

async function loadRoleRefs() {
  const { items } = await menuService.listRoleRefs();
  const map = new Map<number, number[]>();
  if (items) {
    for (const item of items) {
      if (item.menuId != null && item.roleIds) {
        map.set(item.menuId, item.roleIds);
      }
    }
  }
  roleRefMap.value = map;
}

function openRoleDrawer(row: FlatRow) {
  const id = Number(row.id);
  roleDrawerMenuId.value = id;
  roleDrawerMenuLabel.value = row.label;
  roleDrawerOpen.value = true;
}

const selectedRoleNames = computed(() => {
  const roleIds = roleRefMap.value.get(roleDrawerMenuId.value ?? -1) ?? [];
  if (!roleIds.length || !roleStore.list.length) return [];
  const idMap = new Map(roleStore.list.map((r) => [Number(r.id), r.name]));
  return roleIds.map((id) => idMap.get(Number(id)) ?? `Role #${id}`).filter(Boolean);
});

function roleCountFor(row: FlatRow): number {
  return roleRefMap.value.get(Number(row.id))?.length ?? 0;
}

function onCreate() {
  isEdit.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

function onAddChild(row: FlatRow) {
  isEdit.value = false;
  form.value = defaultForm();
  form.value.parentId = row.id;
  drawerOpen.value = true;
}

function onEdit(row: FlatRow) {
  isEdit.value = true;
  const e = row.source;
  form.value = {
    id: e.id ? String(e.id) : undefined,
    name: e.name || '',
    title: e.meta?.title || '',
    type: e.type || 'MENU',
    icon: e.meta?.icon || '',
    path: e.path || '',
    component: e.component || '',
    redirect: e.redirect || '',
    alias: e.alias || '',
    link: e.meta?.link || '',
    iframeSrc: e.meta?.iframeSrc || '',
    parentId: normalizeMenuId(e.parentId),
    sort: e.meta?.order ?? 0,
    status: e.status ?? 'ON',
    isHidden: !!e.meta?.hideInMenu,
    hideInTab: !!e.meta?.hideInTab,
    hideInBreadcrumb: !!e.meta?.hideInBreadcrumb,
    hideChildrenInMenu: !!e.meta?.hideChildrenInMenu,
    affixTab: !!e.meta?.affixTab,
    openInNewWindow: !!e.meta?.openInNewWindow,
    isCache: !!e.meta?.keepAlive,
    authorities: e.meta?.authority ?? [],
  };
  drawerOpen.value = true;
}

async function onSubmit() {
  // MENU/FOLDER 必须有路由路径（按钮/纯权限点除外）
  if (form.value.type !== 'BUTTON' && !form.value.path.trim()) {
    Notify.create({ type: 'negative', message: t('menuEntity.pathRequired') });
    return;
  }
  // 目录：经典分组填 redirect；容器目录（气象数据）可只留组件、不填 redirect
  if (form.value.type === 'FOLDER' && !form.value.redirect.trim() && !form.value.component.trim()) {
    Notify.create({ type: 'negative', message: t('menuEntity.redirectOrComponentRequired') });
    return;
  }
  saving.value = true;
  try {
    const {
      id,
      authorities,
      sort,
      isHidden,
      hideInTab,
      hideInBreadcrumb,
      hideChildrenInMenu,
      affixTab,
      openInNewWindow,
      isCache,
      title,
      icon,
      link,
      iframeSrc,
      parentId,
      ...rest
    } = form.value;
    const meta: MenuMeta = {
      title: title || undefined,
      icon: icon || undefined,
      hideInMenu: isHidden,
      hideInTab,
      hideInBreadcrumb,
      hideChildrenInMenu,
      affixTab,
      openInNewWindow,
      keepAlive: isCache,
      link: link || undefined,
      iframeSrc: iframeSrc || undefined,
      order: sort,
      authority: authorities.filter(Boolean),
    };
    const values: Partial<MenuEntity> = { ...rest, meta };
    // parentId 为空串/未选择时不携带（proto uint32 无法解析空串；清空父级即根节点）
    if (parentId !== '' && parentId != null) {
      values.parentId = String(parentId);
    }
    if (isEdit.value && id) {
      await menuStore.update(id, values);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await menuStore.create(values);
      Notify.create({ type: 'positive', message: t('common.createSuccess') });
    }
    drawerOpen.value = false;
    await loadData();
  } catch {
    /* axios interceptor handles */
  } finally {
    saving.value = false;
  }
}

// 影响面描述文案（删除/禁用对话框共用）
const impactText = computed(() => {
  if (!impactInfo.value) return '';
  const roles = impactInfo.value.roleCount ?? 0;
  const users = impactInfo.value.userCount ?? 0;
  if (pendingDelete.value) {
    return roles === 0
      ? t('menuEntity.impactDeleteNone')
      : t('menuEntity.impactDelete', { roles, users });
  }
  if (pendingDisable.value) {
    return roles === 0 ? '' : t('menuEntity.impactDisable', { roles, users });
  }
  return '';
});

// 拉取影响面（失败降级为 null：删除走保守文案，禁用直接放行，axios 拦截器已提示错误）
async function fetchImpact(id: string) {
  impactLoading.value = true;
  impactInfo.value = null;
  try {
    impactInfo.value = await menuService.getImpact(id);
  } catch {
    impactInfo.value = null;
  } finally {
    impactLoading.value = false;
  }
}

function askDelete(row: FlatRow) {
  // 存在子节点的菜单禁止直接删除，避免产生孤儿节点（与后端 Delete 校验对齐的前置提示）
  if (row.hasChildren) {
    Notify.create({ type: 'negative', message: t('common.deleteFailedHasChildren') });
    return;
  }
  pendingDelete.value = row;
  confirmDeleteOpen.value = true;
  void fetchImpact(row.id);
}

async function onDelete() {
  const row = pendingDelete.value;
  if (!row?.id) return;
  confirmDeleteOpen.value = false;
  try {
    await menuStore.remove(row.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await loadData();
  } catch {
    /* axios interceptor handles */
  } finally {
    pendingDelete.value = null;
    impactInfo.value = null;
  }
}

// 切换启用态：启用一步到位；禁用先评估影响面再确认
async function onToggleStatus(row: FlatRow, val: boolean) {
  if (!row.id) return;
  if (val) {
    // 启用：一步到位
    try {
      await menuStore.update(row.id, { status: 'ON' });
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
      await loadData();
    } catch {
      /* axios interceptor handles */
    }
    return;
  }
  // 禁用：二次确认
  pendingDisable.value = row;
  confirmDisableOpen.value = true;
  void fetchImpact(row.id);
}

async function onDisableConfirm() {
  const row = pendingDisable.value;
  if (!row?.id) return;
  confirmDisableOpen.value = false;
  statusSaving.value.add(row.id);
  try {
    await menuStore.update(row.id, { status: 'OFF' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await loadData();
  } catch {
    /* axios interceptor handles */
  } finally {
    statusSaving.value.delete(row.id);
    pendingDisable.value = null;
    impactInfo.value = null;
  }
}

onMounted(loadData);
</script>

<style lang="sass" scoped>
.menu-page
  .menu-search
    width: 260px
  .menu-row
    display: grid
    grid-template-columns: 2fr 80px 1.4fr 56px 80px 160px
    gap: 16px
    align-items: center
    padding: 8px 12px
    border-radius: 8px
  .menu-row--head
    font-size: 12px
    font-weight: 700
    color: var(--color-text-secondary)
    background: var(--glass-surface)
    margin-bottom: 4px
    border: 1px solid var(--glass-border)
  .menu-row--item
    margin-bottom: 2px
    transition: background 0.15s
    border-bottom: 1px solid var(--glass-border)
    &:hover
      background: var(--glass-surface-hover)
  .menu-row--empty
    border-bottom: none
  .q-list--separator > .q-item-type + .q-item-type
    border-color: var(--glass-border)
  .menu-col-name
    min-width: 0
  .menu-col-path
    min-width: 0
  .menu-leaf
    display: inline-block
    width: 20px
    height: 20px
  .form-group-title
    display: flex
    align-items: center
    gap: 8px
    margin: 16px 0 4px
    font-size: 13px
    font-weight: 700
    color: var(--color-text-secondary)
    &:first-child
      margin-top: 0
  .form-group-icon
    color: var(--q-primary)
  .form-group-line
    flex: 1
    height: 1px
    background: linear-gradient(to right, var(--q-primary), transparent)
    opacity: 0.4
</style>
