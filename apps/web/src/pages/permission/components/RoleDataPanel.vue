<template>
  <div class="role-data-panel">
    <!-- 数据范围卡片：替代原「表格 CRUD + 手写 JSON」的心智负担，选择即生效 -->
    <div class="role-section-title">{{ t('role.scopeCardsTitle') }}</div>
    <div class="role-data-subtitle">{{ t('role.scopeCardsSubtitle') }}</div>
    <div class="role-scope-grid">
      <div
        v-for="card in scopeCards"
        :key="card.scope"
        class="role-scope-card"
        :class="{ 'role-scope-card--active': currentScope === card.scope }"
        @click="onSelectScope(card.scope)"
      >
        <q-icon :name="card.icon" size="24px" class="role-scope-card__icon" />
        <div class="role-scope-card__label">{{ card.label }}</div>
        <div class="role-scope-card__desc">{{ card.desc }}</div>
        <q-icon v-if="currentScope === card.scope" name="check_circle" size="18px" class="role-scope-card__check" />
      </div>
    </div>

    <!-- 自定义部门：选中 custom 范围时内联展开，原弹窗交互省一步 -->
    <div v-if="currentScope === 'custom'" class="role-custom-dept">
      <div class="row items-center justify-between q-mb-sm">
        <div>
          <div class="role-section-title">{{ t('roleDept.title') }}</div>
          <div class="role-data-subtitle">{{ t('role.customDeptHint') }}</div>
        </div>
        <q-btn
          color="primary"
          icon="save"
          :label="t('roleDept.batchBind')"
          unelevated
          :loading="savingDepts"
          @click="onSaveDepts"
        />
      </div>
      <q-tree
        v-model:ticked="tickedDepts"
        :nodes="deptTreeNodes"
        node-key="id"
        label-key="name"
        children-key="children"
        tick-strategy="leaf"
        default-expand-all
        class="role-tree role-custom-dept__tree"
      />
    </div>

    <!-- 高级规则：字段级 JSON 精细控制，折叠收纳避免干扰主流程 -->
    <q-expansion-item
      class="role-advanced"
      icon="tune"
      :label="t('role.advancedTitle')"
      :caption="t('role.advancedDesc')"
      header-class="role-advanced__header"
    >
      <div class="row items-center justify-end q-mb-sm">
        <q-btn color="primary" icon="add" :label="t('common.create')" unelevated dense @click="onCreateRule" />
      </div>
      <q-table
        :rows="dataPermissions"
        :columns="dpColumns"
        flat
        row-key="id"
        :loading="loading"
        :rows-per-page-options="[10, 20, 50]"
      >
        <template #body-cell-scope="{ row }">
          <q-td>
            <q-badge :color="scopeColor(row.scope)" :label="scopeLabel(row.scope)" />
          </q-td>
        </template>
        <template #body-cell-status="{ row }">
          <q-td>
            <q-toggle :model-value="row.status === 'ON'" @update:model-value="(val) => onToggleStatus(row, !!val)" />
          </q-td>
        </template>
        <template #body-cell-actions="{ row }">
          <q-td>
            <div class="crud-actions">
              <q-btn class="crud-actions__btn" flat dense round icon="edit" @click="onEditRule(row)">
                <q-tooltip>{{ t('common.edit') }}</q-tooltip>
              </q-btn>
              <q-btn class="crud-actions__btn crud-actions__btn--danger" flat dense round icon="delete">
                <q-tooltip>{{ t('common.delete') }}</q-tooltip>
                <q-popup-confirm @confirm="onDeleteRule(row)">
                  {{ t('common.confirmDelete') }}
                </q-popup-confirm>
              </q-btn>
            </div>
          </q-td>
        </template>
      </q-table>
    </q-expansion-item>

    <!-- 高级规则 FormDrawer -->
    <FormDrawer v-model="ruleDrawerOpen" :title="ruleDrawerTitle" :saving="savingRule" @submit="onSubmitRule">
      <q-banner dense rounded class="role-rule-banner">
        <q-icon name="info" size="16px" class="q-mr-xs" />{{ t('role.advancedEffectiveHint') }}
      </q-banner>
      <q-input
        v-model="ruleForm.name"
        :placeholder="t('dataPermission.name')"
        dense
        outlined
        :rules="[(val) => !!val || t('common.required')]"
      />
      <q-select
        v-model="ruleForm.field"
        :placeholder="t('dataPermission.field')"
        :options="fieldOptions"
        emit-value
        map-options
        clearable
        dense
        outlined
        :hint="t('role.fieldHint')"
      />
      <q-select
        v-model="ruleForm.scope"
        :placeholder="t('dataPermission.scope')"
        :options="scopeOptions"
        emit-value
        map-options
        dense
        outlined
      />
      <q-input
        v-model="ruleForm.ruleJson"
        :placeholder="t('role.ruleJsonPlaceholder')"
        dense
        outlined
        type="textarea"
        rows="2"
        :rules="[validateRuleJson]"
        :hint="t('role.ruleJsonHint')"
      />
      <q-toggle
        :model-value="ruleForm.status === 'ON'"
        :label="ruleForm.status === 'ON' ? t('common.enabled') : t('common.disabled')"
        @update:model-value="(val) => (ruleForm.status = val ? 'ON' : 'OFF')"
      />
    </FormDrawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import FormDrawer from '../../../components/FormDrawer.vue';
import { useDepartmentStore, buildTree } from '../../../stores';
import {
  dataPermissionService,
  roleDeptService,
  type DataPermissionEntity,
  type DepartmentEntity,
} from '../../../services';

const props = defineProps<{
  roleId: number;
}>();

// scope-change：主规则范围变化时上报父级（RolePage 概览头「数据范围」统计卡同步显示）
const emit = defineEmits<{
  (e: 'scope-change', scope: string): void;
}>();

const { t } = useI18n();
const departmentStore = useDepartmentStore();

const dataPermissions = ref<DataPermissionEntity[]>([]);
const loading = ref(false);
const applying = ref(false);
const tickedDepts = ref<number[]>([]);
const savingDepts = ref(false);

// 主数据范围：该角色按 id 升序的第一条规则，卡片选择即对其更新（无则创建）
const primaryRule = computed<DataPermissionEntity | undefined>(
  () => [...dataPermissions.value].sort((a, b) => Number(a.id) - Number(b.id))[0],
);
const currentScope = computed(() => (primaryRule.value?.scope ?? '').toLowerCase());

// 范围变化实时上报（加载完成/卡片切换均触发），父级概览头据此同步
watch(currentScope, (val) => emit('scope-change', val), { immediate: true });

interface ScopeCard {
  scope: string;
  icon: string;
  label: string;
  desc: string;
}
const scopeCards = computed<ScopeCard[]>(() => [
  { scope: 'all', icon: 'public', label: t('dataPermission.scopeAll'), desc: t('role.scopeAllDesc') },
  { scope: 'dept', icon: 'corporate_fare', label: t('dataPermission.scopeDept'), desc: t('role.scopeDeptDesc') },
  { scope: 'custom', icon: 'account_tree', label: t('dataPermission.scopeCustom'), desc: t('role.scopeCustomDesc') },
  { scope: 'self', icon: 'person', label: t('dataPermission.scopeSelf'), desc: t('role.scopeSelfDesc') },
]);

const scopeOptions = computed(() => [
  { label: t('dataPermission.scopeAll'), value: 'all' },
  { label: t('dataPermission.scopeDept'), value: 'dept' },
  { label: t('dataPermission.scopeCustom'), value: 'custom' },
  { label: t('dataPermission.scopeSelf'), value: 'self' },
]);

// 数据归属字段候选：与 proto 注释（dept_id/user_id）对齐，避免手拼出错
const fieldOptions = computed(() => [
  { label: `dept_id — ${t('role.fieldDeptId')}`, value: 'dept_id' },
  { label: `user_id — ${t('role.fieldUserId')}`, value: 'user_id' },
]);

// 规则 JSON 合法性校验：留空合法（不附加条件），非空必须是可解析的 JSON 对象
function validateRuleJson(val: string): true | string {
  if (!val || !val.trim()) return true;
  try {
    const parsed: unknown = JSON.parse(val);
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      return t('role.ruleJsonInvalid');
    }
    return true;
  } catch {
    return t('role.ruleJsonInvalid');
  }
}

const dpColumns = computed<QTableColumn[]>(() => [
  { name: 'name', label: t('dataPermission.name'), field: 'name', align: 'left' },
  { name: 'field', label: t('dataPermission.field'), field: 'field', align: 'left' },
  { name: 'scope', label: t('dataPermission.scope'), field: 'scope', align: 'center' },
  { name: 'status', label: t('common.status'), field: 'status', align: 'center' },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
]);

const deptTreeNodes = computed(() => buildTree<DepartmentEntity>(departmentStore.list));

function scopeColor(scope?: string): string {
  // DB 枚举为大写（ALL/DEPT/...），统一小写后比对
  switch ((scope ?? '').toLowerCase()) {
    case 'all':
      return 'red';
    case 'dept':
      return 'orange';
    case 'custom':
      return 'blue';
    case 'self':
      return 'green';
    default:
      return 'grey';
  }
}
function scopeLabel(scope?: string): string {
  switch ((scope ?? '').toLowerCase()) {
    case 'all':
      return t('dataPermission.scopeAll');
    case 'dept':
      return t('dataPermission.scopeDept');
    case 'custom':
      return t('dataPermission.scopeCustom');
    case 'self':
      return t('dataPermission.scopeSelf');
    default:
      return scope || '-';
  }
}

async function loadRules() {
  loading.value = true;
  try {
    const resp = await dataPermissionService.list({
      noPaging: true,
      formValues: { roleId: props.roleId },
    });
    dataPermissions.value = resp.items;
  } finally {
    loading.value = false;
  }
}

async function loadDeptBindings() {
  if (departmentStore.list.length === 0) {
    await departmentStore.fetchList({ noPaging: true });
  }
  const resp = await roleDeptService.listByRole(props.roleId);
  tickedDepts.value = resp.items.map((rd) => Number(rd.deptId));
}

watch(
  () => props.roleId,
  (roleId) => {
    if (!roleId) return;
    void loadRules().then(() => {
      if (currentScope.value === 'custom') void loadDeptBindings();
    });
  },
  { immediate: true },
);

// 卡片选择即保存：更新主规则 scope；无规则时按默认名称创建
async function onSelectScope(scope: string) {
  if (applying.value || scope === currentScope.value) return;
  applying.value = true;
  try {
    if (primaryRule.value?.id) {
      await dataPermissionService.update(primaryRule.value.id, { scope });
    } else {
      await dataPermissionService.create({
        roleId: props.roleId,
        name: t('role.defaultScopeName'),
        scope,
        status: 'ON',
      });
    }
    Notify.create({ type: 'positive', message: t('role.scopeUpdated') });
    await loadRules();
    if (scope === 'custom') await loadDeptBindings();
  } catch {
    // axios 拦截器已提示
  } finally {
    applying.value = false;
  }
}

async function onSaveDepts() {
  savingDepts.value = true;
  try {
    await roleDeptService.batchBind(props.roleId, tickedDepts.value);
    Notify.create({ type: 'positive', message: t('roleDept.bindSuccess') });
  } catch {
    // axios 拦截器已提示
  } finally {
    savingDepts.value = false;
  }
}

// ===== 高级规则 CRUD =====
interface RuleForm {
  id?: string;
  name: string;
  field: string;
  scope: string;
  ruleJson: string;
  status: string;
}
function defaultRuleForm(): RuleForm {
  return { name: '', field: '', scope: 'self', ruleJson: '', status: 'ON' };
}
const ruleForm = ref<RuleForm>(defaultRuleForm());
const ruleDrawerOpen = ref(false);
const savingRule = ref(false);
const isEditRule = ref(false);
const ruleDrawerTitle = computed(() => (isEditRule.value ? t('common.edit') : t('common.create')));

function onCreateRule() {
  isEditRule.value = false;
  ruleForm.value = defaultRuleForm();
  ruleDrawerOpen.value = true;
}

function onEditRule(row: Record<string, unknown>) {
  const entity = row as unknown as DataPermissionEntity;
  isEditRule.value = true;
  ruleForm.value = {
    id: entity.id != null ? String(entity.id) : undefined,
    name: entity.name || '',
    field: entity.field || '',
    scope: entity.scope || 'self',
    ruleJson: entity.ruleJson || '',
    status: entity.status ?? 'ON',
  };
  ruleDrawerOpen.value = true;
}

async function onSubmitRule() {
  savingRule.value = true;
  try {
    const values: Partial<DataPermissionEntity> = {
      roleId: props.roleId,
      name: ruleForm.value.name,
      field: ruleForm.value.field,
      scope: ruleForm.value.scope,
      ruleJson: ruleForm.value.ruleJson,
      status: ruleForm.value.status,
    };
    if (isEditRule.value && ruleForm.value.id) {
      await dataPermissionService.update(ruleForm.value.id, values);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    } else {
      await dataPermissionService.create(values);
      Notify.create({ type: 'positive', message: t('common.createSuccess') });
    }
    ruleDrawerOpen.value = false;
    await loadRules();
  } catch {
    // axios 拦截器已提示
  } finally {
    savingRule.value = false;
  }
}

async function onDeleteRule(row: Record<string, unknown>) {
  const entity = row as unknown as DataPermissionEntity;
  if (!entity.id) return;
  try {
    await dataPermissionService.delete(entity.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await loadRules();
  } catch {
    // axios 拦截器已提示
  }
}

async function onToggleStatus(row: Record<string, unknown>, val: boolean) {
  const entity = row as unknown as DataPermissionEntity;
  if (!entity.id) return;
  try {
    await dataPermissionService.update(entity.id, { status: val ? 'ON' : 'OFF' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await loadRules();
  } catch {
    // axios 拦截器已提示
  }
}
</script>

<style lang="sass" scoped>
.role-data-subtitle
  margin-top: 4px
  font-size: 12px
  color: var(--color-text-secondary)

.role-scope-grid
  margin-top: 12px
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr))
  gap: 12px

.role-scope-card
  position: relative
  padding: 14px
  border-radius: 12px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  cursor: pointer
  transition: border-color 0.15s, background 0.15s, transform 0.1s

  &:hover
    background: var(--glass-surface-hover)
    transform: translateY(-1px)

  &--active
    border-color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))

.role-scope-card__icon
  color: var(--color-text-secondary)
  margin-bottom: 8px

  .role-scope-card--active &
    color: var(--color-accent)

.role-scope-card__label
  font-size: 14px
  font-weight: 600
  color: var(--color-text-heading)

.role-scope-card__desc
  margin-top: 4px
  font-size: 12px
  color: var(--color-text-secondary)

.role-scope-card__check
  position: absolute
  top: 10px
  right: 10px
  color: var(--color-accent)

.role-custom-dept
  margin-top: 20px

.role-custom-dept__tree
  max-height: 320px

.role-advanced
  margin-top: 24px
  border: 1px solid var(--glass-border)
  border-radius: 12px
  background: var(--glass-surface)

.role-rule-banner
  background: color-mix(in srgb, var(--color-accent) 8%, transparent)
  color: var(--color-text-secondary)
  font-size: 12px
</style>
