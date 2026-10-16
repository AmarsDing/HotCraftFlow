import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn } from 'quasar';
import type CrudTablePage from '../components/CrudTablePage.vue';
import type { SearchField } from '../components/CrudTablePage.vue';
import { buildTree, type TreeNode } from '../stores';

/** 树形实体的最小形状（organization/department/position 共享） */
export interface TreeEntity {
  id?: string | number;
  name: string;
  parentId?: string | number;
  sortId?: number;
  status?: string;
  remark?: string;
  createTime?: string;
}

/** composable 所需的 store 形状（Pinia makeCrudStore 产出的 store 子集） */
interface TreeStore<T> {
  fetchList(params: {
    page?: number;
    pageSize?: number;
    noPaging?: boolean;
    formValues?: Record<string, unknown>;
    orderBy?: string;
  }): Promise<{ items: T[]; total: number }>;
  create(values: Partial<T>): Promise<void>;
  update(id: string | number, values: Partial<T>): Promise<void>;
  remove(id: string | number): Promise<void>;
}

export interface TreeFormState {
  id?: string;
  name: string;
  parentId: string | number | null;
  organizationId?: string | number | null;
  /** 部门领导用户ID（仅 department 页 showLeader 时生效） */
  leaderId?: string | number | null;
  code?: string;
  sortId: number;
  status: string;
  remark: string;
}

/**
 * 树形 CRUD 页面逻辑复用（organization / department / position 三页共享）。
 * 返回模板所需的全部响应式状态与回调，调用方仅需提供 store 实例。
 * options.showOrganization：部门页开启，表格展示所属组织列（后端 fillOrganizationNames 填充）。
 * options.showLeader：部门页开启，表单/提交/列展示 leaderId（后端 fillLeaderNames 填充 leaderName；领导字段存于本 composable 的 form，人员下拉选项由页面注入）。
 */
export function useTreeCrudPage<T extends TreeEntity>(
  store: TreeStore<T>,
  options?: { showOrganization?: boolean; showLeader?: boolean },
) {
  const { t } = useI18n();
  const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);
  // 保存扁平列表用于构建父节点选项
  const flatItems = ref<T[]>([]);

  function defaultForm(): TreeFormState {
    return { name: '', parentId: null, organizationId: null, leaderId: null, code: '', sortId: 0, status: 'ON', remark: '' };
  }

  const form = ref<TreeFormState>(defaultForm());
  const drawerOpen = ref(false);
  const saving = ref(false);
  const isEdit = ref(false);

  const drawerTitle = computed(() => (isEdit.value ? t('common.edit') : t('common.create')));

  /** 父节点下拉选项（带缩进体现层级，排除当前编辑节点及其全部后代避免循环引用） */
  const parentOptions = computed<{ label: string; value: number }[]>(() => {
    const currentId = form.value.id;
    // 收集当前节点的后代 id：在完整树上定位自身后遍历子树
    const excluded = new Set<string>(currentId != null ? [String(currentId)] : []);
    if (currentId != null) {
      const fullTree = buildTree(flatItems.value);
      const collect = (nodes: TreeNode<T>[]): boolean => {
        for (const n of nodes) {
          if (String(n.id) === String(currentId)) {
            const mark = (ns: TreeNode<T>[]) => {
              for (const c of ns) {
                excluded.add(String(c.id));
                if (c.children?.length) mark(c.children);
              }
            };
            mark(n.children || []);
            return true;
          }
          if (n.children?.length && collect(n.children)) return true;
        }
        return false;
      };
      collect(fullTree as TreeNode<T>[]);
    }

    const tree = buildTree(flatItems.value.filter((it) => !excluded.has(String(it.id))));
    const options: { label: string; value: number }[] = [];
    const walk = (nodes: TreeNode<T>[], level = 0) => {
      for (const n of nodes) {
        const indent = level > 0 ? '│ '.repeat(level - 1) + '├ ' : '';
        options.push({ label: indent + (n.name || ''), value: Number(n.id) });
        if (n.children?.length) walk(n.children, level + 1);
      }
    };
    walk(tree as TreeNode<T>[]);
    return options;
  });

  const columns = computed<QTableColumn[]>(() => {
    const cols: QTableColumn[] = [
      { name: 'name', label: t('common.name'), field: 'name', align: 'left', sortable: true },
    ];
    if (options?.showOrganization) {
      cols.push({ name: 'organizationName', label: t('department.organizationId'), field: 'organizationName', align: 'left' });
    }
    if (options?.showLeader) {
      // leaderName 由后端 fillLeaderNames 联表填充；列宽按姓名两字+留白，名称显示 -' 兜底在渲染时处理
      cols.push({ name: 'leaderName', label: t('department.leaderId'), field: 'leaderName', align: 'left' });
    }
    cols.push(
      { name: 'sortId', label: t('common.sort'), field: 'sortId', align: 'center', sortable: true },
      { name: 'status', label: t('common.status'), field: 'status', align: 'center' },
      { name: 'remark', label: t('common.remark'), field: 'remark', align: 'left' },
      { name: 'createTime', label: t('common.createTime'), field: 'createTime', align: 'center', sortable: true },
      { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false },
    );
    return cols;
  });

  const searchFields = computed<SearchField[]>(() => [
    { name: 'name', label: t('common.name'), type: 'input' },
    {
      name: 'status',
      label: t('common.status'),
      type: 'select',
      options: [
        { label: t('common.enabled'), value: 'ON' },
        { label: t('common.disabled'), value: 'OFF' },
      ],
    },
  ]);

  async function fetchData(params: {
    page: number;
    pageSize: number;
    noPaging: boolean;
    formValues: Record<string, unknown>;
    orderBy?: string;
  }) {
    const resp = await store.fetchList({
      page: params.page,
      pageSize: params.pageSize,
      noPaging: params.noPaging,
      formValues: params.formValues,
      orderBy: params.orderBy,
    });
    flatItems.value = resp.items;
    const tree = buildTree(resp.items);
    return { items: tree as unknown as Record<string, unknown>[], total: resp.total };
  }

  function onCreate() {
    isEdit.value = false;
    form.value = defaultForm();
    drawerOpen.value = true;
  }

  function onEdit(row: Record<string, unknown>) {
    const entity = row as unknown as T;
    isEdit.value = true;
    const entityAny = entity as unknown as Record<string, unknown>;
    form.value = {
      id: entity.id != null ? String(entity.id) : undefined,
      name: entity.name || '',
      parentId: entity.parentId != null && entity.parentId !== '' ? entity.parentId : null,
      organizationId:
        entityAny.organizationId != null && entityAny.organizationId !== ''
          ? (entityAny.organizationId as string | number)
          : null,
      leaderId:
        entityAny.leaderId != null && entityAny.leaderId !== ''
          ? (entityAny.leaderId as string | number)
          : null,
      code: (entityAny.code as string) || '',
      sortId: entity.sortId ?? 0,
      status: entity.status ?? 'ON',
      remark: entity.remark || '',
    };
    drawerOpen.value = true;
  }

  async function onSubmit() {
    saving.value = true;
    try {
      const { id, parentId, organizationId, leaderId, ...rest } = form.value;
      // parentId / organizationId / leaderId 为空时不传，让后端 SetNillable 保持 nil
      const values: Record<string, unknown> = { ...rest };
      if (parentId !== null && parentId !== '' && parentId !== 0) {
        values.parentId = Number(parentId);
      }
      if (organizationId !== null && organizationId !== '' && organizationId !== 0) {
        values.organizationId = Number(organizationId);
      }
      if (leaderId !== null && leaderId !== '' && leaderId !== 0) {
        values.leaderId = Number(leaderId);
      }
      if (isEdit.value && id) {
        await store.update(id, values as Partial<T>);
        Notify.create({ type: 'positive', message: t('common.updateSuccess') });
      } else {
        await store.create(values as Partial<T>);
        Notify.create({ type: 'positive', message: t('common.createSuccess') });
      }
      drawerOpen.value = false;
      await crudRef.value?.fetchData();
    } catch {
      // error notification handled by axios interceptor
    } finally {
      saving.value = false;
    }
  }

  async function onDelete(row: Record<string, unknown>) {
    const entity = row as unknown as T;
    if (!entity.id) return;
    // 树形节点存在子节点时禁止删除，避免后端级联失败或产生孤儿节点
    const children = (row as { children?: unknown[] }).children;
    if (Array.isArray(children) && children.length > 0) {
      Notify.create({ type: 'negative', message: t('common.deleteFailedHasChildren') });
      return;
    }
    try {
      await store.remove(entity.id);
      Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
      await crudRef.value?.fetchData();
    } catch {
      // error notification handled by axios interceptor
    }
  }

  async function onToggleStatus(row: Record<string, unknown>, val: boolean) {
    const entity = row as unknown as T;
    if (!entity.id) return;
    try {
      await store.update(entity.id, { status: val ? 'ON' : 'OFF' } as Partial<T>);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
      await crudRef.value?.fetchData();
    } catch {
      // error notification handled by axios interceptor
    }
  }

  return {
    t,
    crudRef,
    form,
    drawerOpen,
    saving,
    isEdit,
    drawerTitle,
    columns,
    searchFields,
    parentOptions,
    fetchData,
    onCreate,
    onEdit,
    onSubmit,
    onDelete,
    onToggleStatus,
  };
}
