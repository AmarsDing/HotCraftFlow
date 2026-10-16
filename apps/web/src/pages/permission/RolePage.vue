<template>
  <q-page class="crud-page q-pa-lg">
    <div class="role-page-header row items-center justify-between q-mb-lg">
      <div>
        <div class="role-page-title">{{ t('role.title') }}</div>
        <div class="role-page-subtitle">{{ t('role.subtitle') }}</div>
      </div>
      <q-btn color="primary" icon="add" :label="t('common.create')" unelevated @click="onCreateRole" />
    </div>

    <q-splitter v-model="splitterModel" :limits="[14, 26]" unit="%" class="role-splitter">
      <template #before>
        <q-card class="crud-split-card column no-wrap full-height">
          <q-card-section class="role-list-header row items-center justify-between q-pb-sm">
            <div class="role-list-title">{{ t('role.title') }}</div>
            <div class="role-list-count">{{ filteredRoles.length }} {{ t('role.roleCount') }}</div>
          </q-card-section>
          <q-card-section class="q-pt-none q-pb-sm">
            <div class="row items-center no-wrap q-gutter-sm">
              <q-input
                v-model="roleSearch"
                dense
                outlined
                :placeholder="t('common.search')"
                clearable
                class="col role-list-search"
              >
                <template #prepend>
                  <q-icon name="search" size="18px" />
                </template>
              </q-input>
              <q-btn flat dense round icon="add" color="primary" class="role-add-btn" @click="onCreateRole">
                <q-tooltip>{{ t('common.create') }}</q-tooltip>
              </q-btn>
            </div>
          </q-card-section>
          <q-separator />
          <q-scroll-area class="col">
            <q-list dense class="role-list">
              <q-item
                v-for="role in filteredRoles"
                :key="role.id"
                clickable
                :active="selectedRoleId === String(role.id)"
                active-class="role-list-item--active"
                class="role-list-item"
                @click="onSelectRole(role)"
              >
                <q-item-section>
                  <q-item-label class="role-list-item__name">
                    {{ role.name }}
                    <q-badge v-if="isBuiltin(role)" class="role-builtin-badge" :label="t('role.builtin')" />
                  </q-item-label>
                </q-item-section>
                <q-item-section side>
                  <!-- 内置角色（seed 角色）禁止删除，避免误删导致权限体系损坏 -->
                  <q-icon v-if="isBuiltin(role)" name="lock" size="16px" class="role-builtin-icon">
                    <q-tooltip>{{ t('role.builtinNoDelete') }}</q-tooltip>
                  </q-icon>
                  <q-btn
                    v-else
                    flat
                    dense
                    round
                    icon="delete"
                    size="sm"
                    class="crud-actions__btn crud-actions__btn--danger"
                    :class="{ 'role-delete-btn--active': selectedRoleId === String(role.id) }"
                    @click.stop.prevent
                  >
                    <q-tooltip>{{ t('common.delete') }}</q-tooltip>
                    <q-popup-confirm @confirm="onDeleteRole(role)">
                      {{ t('common.confirmDelete') }}
                    </q-popup-confirm>
                  </q-btn>
                </q-item-section>
              </q-item>
              <q-item v-if="filteredRoles.length === 0" class="role-list-empty">
                <q-item-section class="text-center">
                  <q-icon name="search_off" size="32px" color="grey" class="q-mb-sm" />
                  <div class="text-grey text-caption">{{ t('common.noData') }}</div>
                </q-item-section>
              </q-item>
            </q-list>
          </q-scroll-area>
        </q-card>
      </template>

      <template #after>
        <q-card class="crud-split-card column no-wrap full-height">
          <!-- 未选择角色：统一空状态 -->
          <div v-if="!selectedDetail" class="role-empty-state col">
            <q-icon name="admin_panel_settings" size="64px" color="accent" class="q-mb-md" />
            <div class="role-empty-state__title">{{ t('role.selectRoleHint') }}</div>
            <div class="role-empty-state__hint">{{ t('role.selectRoleDesc') }}</div>
          </div>
          <template v-else>
            <!-- 角色概览头：权限画像（菜单/API/数据范围/绑定用户），点击统计卡直达对应 tab -->
            <div class="role-hero">
              <div class="role-hero__main row items-center no-wrap q-gutter-md">
                <div class="role-hero__avatar">
                  <q-icon name="admin_panel_settings" size="24px" />
                </div>
                <div class="col role-hero__body">
                  <div class="row items-center q-gutter-sm">
                    <span class="role-hero__name">{{ selectedDetail.name }}</span>
                    <q-badge outline color="accent" :label="selectedDetail.code" />
                    <q-badge v-if="isBuiltin(selectedDetail)" class="role-builtin-badge" :label="t('role.builtin')" />
                    <q-badge v-if="selectedDetail.status !== 'ON'" color="warning" :label="t('common.disabled')" />
                  </div>
                  <div v-if="selectedDetail.remark" class="role-hero__remark ellipsis">
                    {{ selectedDetail.remark }}
                  </div>
                </div>
                <div class="role-hero__stats row no-wrap q-gutter-sm">
                  <div class="role-stat" @click="tab = 'menu'">
                    <div class="role-stat__num">{{ selectedDetail.menus?.length ?? 0 }}</div>
                    <div class="role-stat__label">{{ t('role.statMenus') }}</div>
                  </div>
                  <div class="role-stat" @click="tab = 'api'">
                    <div class="role-stat__num">{{ selectedDetail.apis?.length ?? 0 }}</div>
                    <div class="role-stat__label">{{ t('role.statApis') }}</div>
                  </div>
                  <div class="role-stat" @click="tab = 'data'">
                    <div class="role-stat__num role-stat__num--text">{{ scopeLabel }}</div>
                    <div class="role-stat__label">{{ t('role.statScope') }}</div>
                  </div>
                  <div class="role-stat role-stat--users">
                    <div class="role-stat__num">{{ boundUsers.length }}</div>
                    <div class="role-stat__label">{{ t('role.statUsers') }}</div>
                    <q-tooltip
                      anchor="bottom middle"
                      self="top middle"
                      :offset="[0, 8]"
                      class="role-users-tip"
                      max-width="260px"
                    >
                      <template v-if="boundUsers.length">
                        <div v-for="u in boundUsers.slice(0, 8)" :key="u.id" class="role-users-tip__item">
                          {{ u.nickname || u.username }}
                        </div>
                        <div v-if="boundUsers.length > 8" class="role-users-tip__more">
                          {{ t('role.moreUsers', { count: boundUsers.length - 8 }) }}
                        </div>
                      </template>
                      <template v-else>{{ t('role.noBoundUser') }}</template>
                    </q-tooltip>
                  </div>
                </div>
              </div>
            </div>
            <q-tabs v-model="tab" align="left" active-color="primary" indicator-color="primary" class="role-tabs">
              <q-tab name="basic">
                <span class="role-tab-label">
                  {{ t('role.tabBasic') }}
                  <i v-if="dirtyMap.basic" class="role-tab-dot" />
                </span>
              </q-tab>
              <q-tab name="menu">
                <span class="role-tab-label">
                  {{ t('role.tabMenu') }}
                  <i v-if="dirtyMap.menu" class="role-tab-dot" />
                </span>
              </q-tab>
              <q-tab name="data">
                <span class="role-tab-label">{{ t('role.tabData') }}</span>
              </q-tab>
              <q-tab name="api">
                <span class="role-tab-label">
                  {{ t('role.tabApi') }}
                  <i v-if="dirtyMap.api" class="role-tab-dot" />
                </span>
              </q-tab>
            </q-tabs>
            <q-separator />
            <!-- keep-alive：切 tab 不销毁面板，未保存的勾选/表单状态保留 -->
            <q-tab-panels v-model="tab" keep-alive class="col scroll role-tab-panels">
              <q-tab-panel name="basic">
                <RoleBasicPanel
                  :role="selectedDetail"
                  :roles="flatRoles"
                  @saved="onBasicSaved"
                  @dirty-change="(val) => (dirtyMap.basic = val)"
                />
              </q-tab-panel>
              <q-tab-panel name="menu">
                <RoleMenuPanel :role="selectedDetail" @dirty-change="(val) => (dirtyMap.menu = val)" />
              </q-tab-panel>
              <q-tab-panel name="data">
                <RoleDataPanel :role-id="Number(selectedDetail.id)" @scope-change="(s) => (dataScope = s)" />
              </q-tab-panel>
              <q-tab-panel name="api">
                <RoleApiPanel :role="selectedDetail" @dirty-change="(val) => (dirtyMap.api = val)" />
              </q-tab-panel>
            </q-tab-panels>
          </template>
        </q-card>
      </template>
    </q-splitter>

    <!-- 未保存修改切换守卫 -->
    <q-dialog v-model="guardOpen" persistent>
      <q-card style="min-width: 360px">
        <q-card-section class="row items-center q-gutter-sm">
          <q-icon name="warning_amber" color="warning" size="24px" />
          <span class="text-h6">{{ t('role.unsavedTitle') }}</span>
        </q-card-section>
        <q-card-section class="q-pt-none">{{ t('role.unsavedMsg') }}</q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat :label="t('role.keepEditing')" />
          <q-btn unelevated color="primary" :label="t('role.discardSwitch')" @click="onDiscardAndSwitch" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 新建角色 FormDrawer -->
    <FormDrawer v-model="roleDrawerOpen" :title="t('common.create')" :saving="savingRole" @submit="onSubmitRole">
      <q-input
        v-model="roleForm.name"
        :placeholder="t('role.name')"
        dense
        outlined
        :rules="[(val) => !!val || t('common.required')]"
      />
      <q-input v-model="roleForm.code" :placeholder="t('role.code')" dense outlined />
      <q-select
        v-model="roleForm.parentId"
        :options="parentOptions"
        :placeholder="t('role.parentId')"
        emit-value
        map-options
        clearable
        dense
        outlined
      />
      <q-input v-model.number="roleForm.sortId" :placeholder="t('common.sort')" dense outlined type="number" />
      <q-toggle
        :model-value="roleForm.status === 'ON'"
        :label="roleForm.status === 'ON' ? t('common.enabled') : t('common.disabled')"
        @update:model-value="(val) => (roleForm.status = val ? 'ON' : 'OFF')"
      />
      <q-input v-model="roleForm.remark" :placeholder="t('common.remark')" dense outlined type="textarea" rows="2" />
    </FormDrawer>
  </q-page>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import FormDrawer from '../../components/FormDrawer.vue';
import RoleBasicPanel from './components/RoleBasicPanel.vue';
import RoleMenuPanel from './components/RoleMenuPanel.vue';
import RoleDataPanel from './components/RoleDataPanel.vue';
import RoleApiPanel from './components/RoleApiPanel.vue';
import { useRoleStore, buildTree, type TreeNode } from '../../stores';
import { dataPermissionService, userService, type RoleEntity, type UserEntity } from '../../services';

const { t } = useI18n();
const roleStore = useRoleStore();

// seed 内置角色：禁止删除（删除后 seed 重启会重建，但中间窗口期用户权限会异常）
const BUILTIN_ROLE_CODES = new Set(['super', 'gly', 'xx', 'yb', 'gc', 'jw']);

const tab = ref('basic');
const selectedRoleId = ref<string | null>(null);
const selectedDetail = ref<RoleEntity | null>(null);
const flatRoles = ref<RoleEntity[]>([]);
const roleSearch = ref('');
const splitterModel = ref(18);

// 概览头统计：数据范围主规则 + 绑定用户（按角色 code 匹配 user.roles）
const dataScope = ref('');
const boundUsers = ref<UserEntity[]>([]);

// 各 tab 未保存状态：basic/menu/api 由面板 dirty-change 上报；data 为即选即存无脏态
const dirtyMap = reactive({ basic: false, menu: false, api: false });
const anyDirty = computed(() => dirtyMap.basic || dirtyMap.menu || dirtyMap.api);
const guardOpen = ref(false);
const pendingRole = ref<RoleEntity | null>(null);

const filteredRoles = computed<RoleEntity[]>(() => {
  const kw = roleSearch.value.trim().toLowerCase();
  if (!kw) return flatRoles.value;
  return flatRoles.value.filter(
    (r) => (r.name || '').toLowerCase().includes(kw) || (r.code || '').toLowerCase().includes(kw),
  );
});

function isBuiltin(role: RoleEntity): boolean {
  return !!role.code && BUILTIN_ROLE_CODES.has(role.code);
}

// 数据范围主规则 scope → 文案（与 RoleDataPanel 的 primaryRule 口径一致：按 id 升序第一条）
const scopeLabel = computed(() => {
  switch (dataScope.value) {
    case 'all':
      return t('dataPermission.scopeAll');
    case 'dept':
      return t('dataPermission.scopeDept');
    case 'custom':
      return t('dataPermission.scopeCustom');
    case 'self':
      return t('dataPermission.scopeSelf');
    default:
      return t('role.scopeNone');
  }
});

// 概览头统计加载：数据范围 + 绑定用户（失败静默降级为空，不打断主流程）
async function loadOverview(role: RoleEntity) {
  dataScope.value = '';
  boundUsers.value = [];
  const roleId = Number(role.id);
  if (!roleId) return;
  try {
    const resp = await dataPermissionService.list({ noPaging: true, formValues: { roleId } });
    const primary = [...resp.items].sort((a, b) => Number(a.id) - Number(b.id))[0];
    dataScope.value = (primary?.scope ?? '').toLowerCase();
  } catch {
    /* 概览统计降级为空 */
  }
  try {
    const resp = await userService.list({ noPaging: true });
    boundUsers.value = role.code ? resp.items.filter((u) => u.roles?.includes(role.code!)) : [];
  } catch {
    /* 概览统计降级为空 */
  }
}

// 新建角色抽屉的上级角色选项（无当前角色，无需排除）
const parentOptions = computed<{ label: string; value: number }[]>(() => {
  const tree = buildTree(flatRoles.value);
  const options: { label: string; value: number }[] = [];
  const walk = (nodes: TreeNode<RoleEntity>[], level = 0) => {
    for (const n of nodes) {
      const indent = level > 0 ? '│ '.repeat(level - 1) + '├ ' : '';
      options.push({ label: indent + (n.name || ''), value: Number(n.id) });
      if (n.children?.length) walk(n.children, level + 1);
    }
  };
  walk(tree);
  return options;
});

// ===== 角色列表 =====
async function loadRoles() {
  const resp = await roleStore.fetchList({ noPaging: true });
  // 按 sortId + id 稳定排序，避免后端默认序导致的列表跳动
  flatRoles.value = [...resp.items].sort((a, b) => (a.sortId ?? 0) - (b.sortId ?? 0) || Number(a.id) - Number(b.id));
}

function onSelectRole(role: RoleEntity) {
  if (selectedRoleId.value === String(role.id)) return;
  // 有未保存修改时先弹守卫，放弃修改才真正切换
  if (anyDirty.value) {
    pendingRole.value = role;
    guardOpen.value = true;
    return;
  }
  void applySelect(role);
}

async function applySelect(role: RoleEntity) {
  if (!role.id) return;
  selectedRoleId.value = String(role.id);
  // 列表接口不返回 menus/apis 关联数据，需调用 Get 获取详情
  try {
    const detail = await roleStore.fetchDetail(role.id);
    selectedDetail.value = { ...detail };
  } catch {
    // 加载详情失败：仅填充列表已有的基础字段，菜单勾选清空并提示，避免静默清空导致误判为无权限
    Notify.create({ type: 'negative', message: t('common.loadFailed') });
    selectedDetail.value = { ...role, menus: [], apis: [] };
  }
  void loadOverview(selectedDetail.value);
}

function onDiscardAndSwitch() {
  guardOpen.value = false;
  if (pendingRole.value) void applySelect(pendingRole.value);
  pendingRole.value = null;
}

// 基础信息保存后：刷新列表（名称/排序可能变化）+ 原位合并详情（保持对象引用，
// 不打断菜单/API tab 中未保存的编辑）
async function onBasicSaved() {
  await loadRoles();
  if (!selectedRoleId.value || !selectedDetail.value) return;
  try {
    const fresh = await roleStore.fetchDetail(selectedRoleId.value);
    Object.assign(selectedDetail.value, fresh);
  } catch {
    // axios 拦截器已提示
  }
}

async function onDeleteRole(role: RoleEntity) {
  if (!role.id) return;
  try {
    await roleStore.remove(role.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    const wasSelected = selectedRoleId.value === String(role.id);
    if (wasSelected) {
      selectedRoleId.value = null;
      selectedDetail.value = null;
    }
    await loadRoles();
    // 删除当前选中角色后自动选中剩余第一个，保持「始终有选中」避免空态死路
    if (wasSelected && flatRoles.value.length > 0) {
      await applySelect(flatRoles.value[0]);
    }
  } catch {
    // axios 拦截器已提示
  }
}

// ===== 新建角色 =====
interface RoleForm {
  name: string;
  code: string;
  parentId: number | null;
  sortId: number;
  status: string;
  remark: string;
}
function defaultRoleForm(): RoleForm {
  return { name: '', code: '', parentId: null, sortId: 0, status: 'ON', remark: '' };
}
const roleDrawerOpen = ref(false);
const savingRole = ref(false);
const roleForm = ref<RoleForm>(defaultRoleForm());

function onCreateRole() {
  roleForm.value = defaultRoleForm();
  roleDrawerOpen.value = true;
}

async function onSubmitRole() {
  savingRole.value = true;
  try {
    const { parentId, ...rest } = roleForm.value;
    const values: Record<string, unknown> = { ...rest };
    if (parentId !== null && parentId !== 0) values.parentId = parentId;
    await roleStore.create(values);
    Notify.create({ type: 'positive', message: t('common.createSuccess') });
    roleDrawerOpen.value = false;
    const createdName = roleForm.value.name;
    const createdCode = roleForm.value.code;
    await loadRoles();
    // 新建成功后自动选中新角色，直接进入权限配置流程（store.create 不返回 id，按 name+code 匹配最新一条）
    const created = [...flatRoles.value]
      .reverse()
      .find((r) => r.name === createdName && (r.code || '') === createdCode);
    if (created) await applySelect(created);
  } catch {
    // axios 拦截器已提示
  } finally {
    savingRole.value = false;
  }
}

onMounted(async () => {
  await loadRoles();
  // 默认选中第一个角色：消除空态死路，进页即见权限画像
  if (!selectedRoleId.value && flatRoles.value.length > 0) {
    await applySelect(flatRoles.value[0]);
  }
});
</script>

<style lang="sass">
.role-page-header
  .role-page-title
    font-size: 24px
    font-weight: 700
    color: var(--color-text-heading)
    letter-spacing: 0.2px

  .role-page-subtitle
    margin-top: 4px
    font-size: 13px
    color: var(--color-text-secondary)

.role-splitter
  height: calc(100vh - 170px)

.role-list-header
  .role-list-title
    font-size: 15px
    font-weight: 700
    color: var(--color-text-heading)

  .role-list-count
    font-size: 12px
    color: var(--color-text-tertiary)
    padding: 2px 8px
    border-radius: 10px
    background: var(--glass-surface)

.role-list-search
  .q-field__control
    background: var(--glass-surface)
    border-radius: 10px

.role-add-btn
  border: 1px solid color-mix(in srgb, var(--q-primary) 40%, var(--glass-border))
  background: color-mix(in srgb, var(--q-primary) 10%, transparent)

  &:hover
    background: color-mix(in srgb, var(--q-primary) 20%, transparent)

.role-list
  padding: 10px 16px

// 角色项卡片化：玻璃底 + 描边 + 上下间隔，hover/选中态描边提亮
.role-list-item
  margin: 0 6px 10px
  padding: 12px 14px
  border-radius: 12px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  transition: background 0.15s, border-color 0.15s, box-shadow 0.15s
  position: relative
  overflow: hidden

  &:last-child
    margin-bottom: 2px

  &::before
    content: ''
    position: absolute
    left: 0
    top: 12px
    bottom: 12px
    width: 3px
    border-radius: 0 3px 3px 0
    background: var(--color-accent)
    opacity: 0
    transition: opacity 0.15s

  &:hover:not(.role-list-item--active)
    background: var(--glass-surface-hover)
    border-color: color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))

    &::before
      opacity: 0.5

.role-list-item--active
  background: color-mix(in srgb, var(--color-accent) 12%, var(--glass-surface))
  border-color: color-mix(in srgb, var(--color-accent) 55%, var(--glass-border))
  box-shadow: 0 4px 16px color-mix(in srgb, var(--color-accent) 14%, transparent)

  &::before
    opacity: 1

  .role-list-item__name
    color: var(--color-text-heading)
    font-weight: 600
    font-size: 15px

  .role-delete-btn--active
    color: var(--color-on-accent)

.role-list-item__name
  color: var(--color-text-primary)
  font-weight: 500
  font-size: 15px

.role-builtin-badge
  margin-left: 6px
  font-size: 10px
  font-weight: 600
  color: var(--color-text-tertiary)
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

.role-builtin-icon
  color: var(--color-text-tertiary)

.role-list-empty
  padding: 32px 16px
  color: var(--color-text-secondary)

.role-hero
  position: relative
  padding: 18px 16px 14px
  flex-shrink: 0

  // 顶部渐变光条（科技感，与告警卡片同范式）
  &::before
    content: ''
    position: absolute
    top: 0
    left: 16px
    right: 16px
    height: 2px
    border-radius: 2px
    background: linear-gradient(90deg, var(--color-accent), color-mix(in srgb, var(--color-accent) 20%, transparent))

  .role-hero__avatar
    width: 46px
    height: 46px
    border-radius: 12px
    display: flex
    align-items: center
    justify-content: center
    flex-shrink: 0
    color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)
    border: 1px solid color-mix(in srgb, var(--color-accent) 30%, transparent)
    box-shadow: 0 0 16px color-mix(in srgb, var(--color-accent) 22%, transparent)

  .role-hero__body
    min-width: 0

  .role-hero__name
    font-size: 17px
    font-weight: 700
    color: var(--color-text-heading)

  .role-hero__remark
    margin-top: 3px
    font-size: 12px
    color: var(--color-text-secondary)

.role-hero__stats
  flex-shrink: 0

.role-stat
  position: relative
  min-width: 78px
  padding: 8px 14px
  border-radius: 12px
  text-align: center
  cursor: pointer
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  transition: border-color 0.15s, box-shadow 0.15s, transform 0.1s

  &:hover
    border-color: color-mix(in srgb, var(--color-accent) 55%, transparent)
    box-shadow: 0 0 12px color-mix(in srgb, var(--color-accent) 20%, transparent)
    transform: translateY(-1px)

  .role-stat__num
    font-size: 17px
    font-weight: 700
    line-height: 1.3
    color: var(--color-text-heading)

  .role-stat__num--text
    font-size: 13px
    color: var(--color-accent)

  .role-stat__label
    margin-top: 2px
    font-size: 11px
    color: var(--color-text-tertiary)
    white-space: nowrap

.role-users-tip
  .role-users-tip__item
    font-size: 12px
    line-height: 1.8

  .role-users-tip__more
    font-size: 11px
    opacity: 0.75

.role-tabs
  min-height: 48px
  padding: 0 8px

  .q-tab
    text-transform: none
    font-weight: 600
    color: var(--color-text-secondary)

  .q-tab--active
    color: var(--color-accent)

.role-tab-label
  display: inline-flex
  align-items: center
  gap: 5px

.role-tab-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-warning)
  flex-shrink: 0

.role-tab-panels
  background: transparent

.role-empty-state
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  height: 100%
  min-height: 320px
  color: var(--color-text-secondary)
  text-align: center

  .role-empty-state__title
    font-size: 16px
    font-weight: 600
    color: var(--color-text-heading)
    margin-bottom: 6px

  .role-empty-state__hint
    font-size: 13px
    color: var(--color-text-secondary)

.role-section-title
  font-size: 15px
  font-weight: 700
  color: var(--color-text-heading)
  position: relative
  padding-left: 12px

  &::before
    content: ''
    position: absolute
    left: 0
    top: 4px
    bottom: 4px
    width: 4px
    border-radius: 2px
    background: var(--color-accent)

.role-tree
  max-height: calc(100vh - 380px)
  overflow-y: auto
  border: 1px solid var(--glass-border)
  border-radius: 12px
  padding: 12px
  background: color-mix(in srgb, var(--glass-surface) 50%, transparent)

  .q-tree__node--selected
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)
    border-radius: 8px

body.body--dark
  .role-page-header .role-page-title
    color: var(--color-text-heading)

  .role-list-item--active
    background: color-mix(in srgb, var(--color-accent) 18%, var(--glass-surface))

  .role-tree
    background: color-mix(in srgb, var(--glass-surface) 35%, transparent)
</style>
