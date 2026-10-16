<template>
  <div class="role-basic-panel">
    <div class="role-section-title">{{ t('role.tabBasic') }}</div>
    <q-form class="role-basic-form" @submit.prevent="onSave">
      <div class="role-form-grid">
        <q-input
          v-model="form.name"
          :placeholder="t('role.name')"
          dense
          outlined
          :rules="[(val) => !!val || t('common.required')]"
        />
        <q-input
          v-model="form.code"
          :placeholder="t('role.code')"
          dense
          outlined
          :readonly="isBuiltinCode"
          :hint="isBuiltinCode ? t('role.builtinCodeHint') : undefined"
        />
        <q-select
          v-model="form.parentId"
          :options="parentOptions"
          :placeholder="t('role.parentId')"
          emit-value
          map-options
          clearable
          dense
          outlined
        />
        <q-input v-model.number="form.sortId" :placeholder="t('common.sort')" dense outlined type="number" />
        <q-toggle
          class="role-form-grid__status"
          :model-value="form.status === 'ON'"
          :label="form.status === 'ON' ? t('common.enabled') : t('common.disabled')"
          @update:model-value="(val) => (form.status = val ? 'ON' : 'OFF')"
        />
        <q-input
          v-model="form.remark"
          class="role-form-grid__full"
          :placeholder="t('common.remark')"
          dense
          outlined
          type="textarea"
          rows="2"
        />
      </div>
      <div class="role-form-actions row items-center q-gutter-md">
        <q-btn
          :loading="saving"
          :disable="!dirty"
          color="primary"
          icon="save"
          :label="t('common.save')"
          type="submit"
          unelevated
        />
        <span v-if="dirty" class="role-dirty-hint">{{ t('role.unsavedTitle') }}</span>
      </div>
    </q-form>

    <!-- 只读元信息：帮助管理员确认数据时效与来源 -->
    <div class="role-meta">
      <div class="role-section-title">{{ t('role.metaTitle') }}</div>
      <div class="role-meta__grid">
        <div class="role-meta__item">
          <span class="role-meta__label">ID</span>
          <span class="role-meta__value">{{ role.id }}</span>
        </div>
        <div class="role-meta__item">
          <span class="role-meta__label">{{ t('role.parentName') }}</span>
          <span class="role-meta__value">{{ parentName }}</span>
        </div>
        <div class="role-meta__item">
          <span class="role-meta__label">{{ t('role.createTime') }}</span>
          <span class="role-meta__value">{{ formatTime(role.createTime) }}</span>
        </div>
        <div class="role-meta__item">
          <span class="role-meta__label">{{ t('role.updateTime') }}</span>
          <span class="role-meta__value">{{ formatTime(role.updateTime) }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import { useRoleStore, buildTree, type TreeNode } from '../../../stores';
import type { RoleEntity } from '../../../services';
import { formatDateTime } from '../../../utils/datetime';

const props = defineProps<{
  role: RoleEntity;
  roles: RoleEntity[];
}>();

const emit = defineEmits<{
  (e: 'saved'): void;
  (e: 'dirty-change', dirty: boolean): void;
}>();

const { t } = useI18n();
const roleStore = useRoleStore();

interface BasicForm {
  name: string;
  code: string;
  parentId: number | null;
  sortId: number;
  status: string;
  remark: string;
}

const form = ref<BasicForm>({ name: '', code: '', parentId: null, sortId: 0, status: 'ON', remark: '' });
const snapshot = ref('');
const saving = ref(false);

// 快照比较驱动 dirty 状态；父级据此在切换角色前弹守卫
const dirty = computed(() => snapshot.value !== '' && JSON.stringify(form.value) !== snapshot.value);
watch(dirty, (val) => emit('dirty-change', val));

function fillForm(role: RoleEntity) {
  form.value = {
    name: role.name || '',
    code: role.code || '',
    parentId: role.parentId != null && role.parentId !== '' ? Number(role.parentId) : null,
    sortId: role.sortId ?? 0,
    status: role.status ?? 'ON',
    remark: role.remark || '',
  };
  snapshot.value = JSON.stringify(form.value);
}

// 仅切换角色（id 变化）时重置表单；父级同 id 刷新详情（Object.assign）不打断编辑
watch(
  () => props.role.id,
  () => fillForm(props.role),
  { immediate: true },
);

// 上级角色下拉：排除自身及全部后代，防止循环父子（仅排除自身时，后代仍可被选为父级成环）
const parentOptions = computed<{ label: string; value: number }[]>(() => {
  const selfId = props.role.id != null ? String(props.role.id) : '';
  // 收集自身后代 id：在完整树上定位自身节点后遍历其子树
  const excluded = new Set<string>([selfId]);
  const fullTree = buildTree(props.roles);
  const collectDescendants = (nodes: TreeNode<RoleEntity>[]) => {
    for (const n of nodes) {
      if (String(n.id) === selfId) {
        const mark = (ns: TreeNode<RoleEntity>[]) => {
          for (const c of ns) {
            excluded.add(String(c.id));
            if (c.children?.length) mark(c.children);
          }
        };
        mark(n.children || []);
        return true;
      }
      if (n.children?.length && collectDescendants(n.children)) return true;
    }
    return false;
  };
  if (selfId) collectDescendants(fullTree);

  const tree = buildTree(props.roles.filter((r) => !excluded.has(String(r.id))));
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

// 内置 seed 角色 code 保护：code 是 seed 幂等键与 casbin 策略主体，编辑会导致重复播种/策略错乱
// 与后端 roleSeedRecords 及 RolePage.BUILTIN_ROLE_CODES 保持一致（仅 6 个 seed 角色）
const BUILTIN_ROLE_CODES = new Set(['super', 'gly', 'xx', 'yb', 'gc', 'jw']);
const isBuiltinCode = computed(() => BUILTIN_ROLE_CODES.has(props.role.code || ''));

// 元信息「上级角色」显示名：从全量角色列表反查，无上级显示顶级说明
const parentName = computed(() => {
  const pid = props.role.parentId;
  if (pid == null || pid === '') return t('role.noParent');
  const parent = props.roles.find((r) => String(r.id) === String(pid));
  return parent?.name || `#${pid}`;
});

function formatTime(val?: string): string {
  return formatDateTime(val);
}

async function onSave() {
  if (!props.role.id) return;
  saving.value = true;
  try {
    const { parentId, ...rest } = form.value;
    const values: Record<string, unknown> = { ...rest };
    if (parentId !== null && parentId !== 0) values.parentId = parentId;
    await roleStore.update(props.role.id, values);
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    snapshot.value = JSON.stringify(form.value);
    emit('saved');
  } catch {
    // axios 拦截器已提示
  } finally {
    saving.value = false;
  }
}
</script>

<style lang="sass" scoped>
.role-basic-form
  margin-top: 16px

// 双列栅格：消除单列 520px 布局右侧大片空白；remark 跨两列
.role-form-grid
  display: grid
  grid-template-columns: repeat(auto-fit, minmax(240px, 340px))
  gap: 4px 16px

  // 字段盒高统一：无 rules/hint 的字段补 20px 底部占位（与 FormDrawer 同范式），
  // 避免 name(带 rules) 行与其他行垂直错位
  .q-field:not(.q-field--with-bottom)
    padding-bottom: 20px

.role-form-grid__status
  align-self: start
  margin-top: 4px

.role-form-grid__full
  grid-column: 1 / -1

.role-form-actions
  padding-top: 8px

.role-dirty-hint
  font-size: 12px
  color: var(--color-warning)

.role-meta
  margin-top: 28px

.role-meta__grid
  margin-top: 12px
  display: grid
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr))
  gap: 12px

.role-meta__item
  display: flex
  flex-direction: column
  gap: 4px
  padding: 10px 12px
  border-radius: 10px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)

.role-meta__label
  font-size: 11px
  color: var(--color-text-tertiary)

.role-meta__value
  font-size: 13px
  font-weight: 600
  color: var(--color-text-primary)
</style>
