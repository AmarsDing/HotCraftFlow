<template>
  <q-page class="crud-page q-pa-lg dict-page">
    <!-- 摘要条：单行紧凑统计（分类/枚举/参数/条目/业务引用），替代装饰性统计卡组 -->
    <div class="dict-summary q-mb-md">
      <q-icon name="menu_book" size="16px" class="dict-summary__icon" />
      <span class="dict-summary__item">{{ t('dict.summaryCats', { n: categories.length }) }}</span>
      <span class="dict-summary__sep" />
      <span class="dict-summary__item">{{ t('dict.summaryEnum', { n: enumCount }) }}</span>
      <span class="dict-summary__sep" />
      <span class="dict-summary__item">{{ t('dict.summaryParam', { n: paramCount }) }}</span>
      <span class="dict-summary__sep" />
      <span class="dict-summary__item">{{ t('dict.summaryEntries', { n: totalEntries }) }}</span>
      <template v-if="consumedCount > 0">
        <span class="dict-summary__sep" />
        <span class="dict-summary__item dict-summary__item--link">
          <q-icon name="link" size="13px" />
          {{ t('dict.summaryConsumed', { n: consumedCount }) }}
        </span>
      </template>
    </div>

    <q-splitter :limits="[16, 32]" :model-value="22" unit="%" disable class="dict-splitter">
      <template #before>
        <q-card class="crud-split-card dict-nav column no-wrap full-height">
          <q-card-section class="dict-nav__header">
            <div class="dict-nav__title">
              <span>{{ t('dict.category') }}</span>
              <span class="dict-nav__title-count">{{ filteredCategoryMeta.length }}</span>
              <!-- 分类管理入口：新建分类实体（可先建空分类再加条目） -->
              <q-btn
                flat
                dense
                round
                icon="add"
                size="sm"
                class="dict-nav__add"
                :aria-label="t('dict.addCategory')"
                @click="onCreateCategory"
              >
                <q-tooltip>{{ t('dict.addCategoryTip') }}</q-tooltip>
              </q-btn>
            </div>
            <!-- 双视图：枚举选项 / 系统参数（两类字典用户期望不同，分视图消除混淆） -->
            <q-tabs
              v-model="kindTab"
              dense
              no-caps
              narrow-indicator
              class="dict-nav__tabs"
              active-color="primary"
              indicator-color="primary"
            >
              <q-tab name="enum" :label="t('dict.tabEnum')" />
              <q-tab name="param" :label="t('dict.tabParam')" />
            </q-tabs>
            <q-input
              v-model="categoryFilter"
              :placeholder="t('dict.searchCategory')"
              dense
              outlined
              clearable
              class="dict-nav__search"
            >
              <template #prepend>
                <q-icon name="search" size="15px" />
              </template>
            </q-input>
          </q-card-section>
          <q-separator />
          <q-scroll-area class="col">
            <div class="dict-nav__list">
              <!-- 「全部」卡片：回到当前视图下无分类过滤的全量条目 -->
              <div
                class="dict-nav-row"
                :class="{ 'dict-nav-row--active': selectedCategory === '' }"
                @click="onSelectCategory('')"
              >
                <span class="dict-nav-row__name">{{ t('dict.all') }}</span>
                <span class="dict-nav-row__count">{{ tabEntryTotal }}</span>
              </div>
              <!-- 分类行：名称/编码 + 业务接入/内置标记 + 条目数（去装饰，信息优先） -->
              <div
                v-for="cat in filteredCategoryMeta"
                :key="cat.name"
                class="dict-nav-row"
                :class="{ 'dict-nav-row--active': selectedCategory === cat.name }"
                @click="onSelectCategory(cat.name)"
              >
                <div class="dict-nav-row__main">
                  <div class="dict-nav-row__line">
                    <span class="dict-nav-row__name" :title="cat.desc || cat.name">{{ cat.desc || cat.name }}</span>
                    <!-- 编辑分类（悬停显现，点击不触发选中） -->
                    <q-btn
                      flat
                      dense
                      round
                      icon="edit"
                      size="xs"
                      class="dict-nav-row__edit"
                      :aria-label="t('dict.editCategoryTip')"
                      @click.stop="onEditCategory(cat)"
                    >
                      <q-tooltip>{{ t('dict.editCategoryTip') }}</q-tooltip>
                    </q-btn>
                    <q-icon v-if="cat.builtin" name="lock" size="12px" class="dict-nav-row__lock">
                      <q-tooltip>{{ t('dict.builtinTag') }}</q-tooltip>
                    </q-icon>
                    <q-icon v-if="cat.consumers.length" name="link" size="13px" class="dict-nav-row__link">
                      <q-tooltip>{{ cat.consumers.join('\n') }}</q-tooltip>
                    </q-icon>
                    <span class="dict-nav-row__count">{{ cat.count }}</span>
                  </div>
                  <div v-if="cat.desc" class="dict-nav-row__code" :title="cat.name">{{ cat.name }}</div>
                </div>
              </div>
              <div v-if="filteredCategoryMeta.length === 0" class="dict-nav__empty">
                {{ t('common.noData') }}
              </div>
            </div>
          </q-scroll-area>
        </q-card>
      </template>

      <template #after>
        <CrudTablePage
          ref="crudRef"
          :title="tableTitle"
          :columns="columns"
          :search-fields="searchFields"
          row-key="id"
          fill-height
          :fetch-data-fn="fetchData"
          @create="onCreate"
        >
          <template #body-cell-key="{ row }">
            <span class="dict-key" :title="String(row.key ?? '')">{{ row.key }}</span>
          </template>

          <template #body-cell-category="{ row }">
            <q-badge outline class="dict-cat-badge">
              {{ row.category }}
              <q-tooltip>
                <div class="text-weight-medium">{{ row.category }}</div>
                <div v-if="row.categoryDesc" class="text-caption">{{ row.categoryDesc }}</div>
              </q-tooltip>
            </q-badge>
          </template>

          <template #body-cell-value="{ row }">
            <!-- 只显示字典值；说明/全称收进悬停浮层，避免双行撑高与错位 -->
            <span class="dict-value">
              {{ row.value || '-' }}
              <q-tooltip v-if="row.value || row.valueDesc" class="dict-value-tip">
                <div class="dict-value-tip__label">{{ t('dict.value') }}</div>
                <div class="dict-value-tip__value">{{ row.value || '-' }}</div>
                <template v-if="row.valueDesc">
                  <div class="dict-value-tip__label">{{ t('dict.valueDesc') }}</div>
                  <div class="dict-value-tip__desc">{{ row.valueDesc }}</div>
                </template>
              </q-tooltip>
            </span>
          </template>

          <template #body-cell-valueDataType="{ row }">
            <q-badge
              v-if="row.valueDataType"
              class="dict-type"
              :class="`dict-type--${row.valueDataType}`"
              :label="dataTypeLabel(String(row.valueDataType))"
            />
            <span v-else class="dict-cell-empty">-</span>
          </template>

          <template #body-cell-status="{ row }">
            <!-- 状态列内嵌开关（项目约定：color=positive + 行 id 防连点 + tooltip 当前状态）；
                 内置参数条目锁定——生效通道是"值"，启停会造成"已停用"假象（后端同口径 409） -->
            <q-toggle
              :model-value="row.status === 'ON'"
              color="positive"
              dense
              :disable="togglingId === row.id || isBuiltinParamRow(row)"
              @update:model-value="(val) => onToggleStatus(row, !!val)"
            >
              <q-tooltip>
                {{ isBuiltinParamRow(row) ? t('dict.builtinEntryLocked') : row.status === 'ON' ? t('common.enabled') : t('common.disabled') }}
              </q-tooltip>
            </q-toggle>
          </template>

          <template #body-cell-actions="{ row }">
            <div class="crud-actions">
              <q-btn class="crud-actions__btn" flat dense round icon="edit" @click="onEdit(row)">
                <q-tooltip>{{ t('common.edit') }}</q-tooltip>
              </q-btn>
              <!-- 复制条目：同分类/值/类型预填，仅需补新键（批量建相似条目最少操作路径） -->
              <q-btn class="crud-actions__btn" flat dense round icon="content_copy" @click="onDuplicate(row)">
                <q-tooltip>{{ t('dict.duplicate') }}</q-tooltip>
              </q-btn>
              <q-btn
                class="crud-actions__btn crud-actions__btn--danger"
                flat
                dense
                round
                icon="delete"
                :disable="isBuiltinParamRow(row)"
              >
                <q-tooltip v-if="isBuiltinParamRow(row)">{{ t('dict.builtinEntryLocked') }}</q-tooltip>
                <q-tooltip v-else>{{ t('common.delete') }}</q-tooltip>
                <q-popup-confirm v-if="!isBuiltinParamRow(row)" @confirm="onDelete(row)">
                  {{ deleteConfirmText(row) }}
                </q-popup-confirm>
              </q-btn>
            </div>
          </template>

          <template #extra>
            <FormDrawer
              ref="drawerRef"
              v-model="drawerOpen"
              :title="drawerTitle"
              :saving="saving"
              :width="560"
              @submit="onSubmit(false)"
            >
              <!-- Ctrl+Enter = 保存并继续（连续建同分类条目） -->
              <div @keydown.ctrl.enter.prevent="onSaveContinue">
                <q-banner v-if="consumedModuleName" dense rounded class="dict-consumed-banner">
                  <template #avatar>
                    <q-icon name="link" />
                  </template>
                  {{ t('dict.consumedHint', { module: consumedModuleName }) }}
                </q-banner>

                <!-- 编辑态：编码/分类是条目身份（type+item_key 联合唯一），只读上下文条呈现，
                     表单聚焦"改值"；换编码走「复制条目」。新建态才是完整表单。 -->
                <div v-if="isEdit" class="dict-context">
                  <span class="dict-context__key" :title="form.key">{{ form.key }}</span>
                  <q-badge outline class="dict-cat-badge dict-context__cat" :title="form.category">
                    {{ form.category }}
                  </q-badge>
                  <span v-if="form.categoryDesc" class="dict-context__desc" :title="form.categoryDesc">
                    {{ form.categoryDesc }}
                  </span>
                  <q-icon name="lock" size="13px" class="dict-context__lock">
                    <q-tooltip>{{ t('dict.identityLocked') }}</q-tooltip>
                  </q-icon>
                </div>

                <!-- 基础信息：90% 场景只需 键/分类/值 三项，其余默认不打扰 -->
                <div class="dict-form-section">
                  <q-icon name="tune" size="14px" />
                  <span>{{ isEdit ? t('dict.sectionValue') : t('dict.sectionBasic') }}</span>
                </div>
                <template v-if="!isEdit">
                  <q-input
                    ref="keyInputRef"
                    v-model="form.key"
                    :label="t('dict.key')"
                    dense
                    outlined
                    maxlength="64"
                    class="dict-mono"
                    :hint="t('dict.keyHint')"
                    :rules="[(val) => !!val || t('common.required')]"
                  />
                  <q-select
                    v-model="form.category"
                    :options="categoryOptions"
                    :label="t('dict.category')"
                    dense
                    outlined
                    use-input
                    hide-selected
                    fill-input
                    input-debounce="0"
                    new-value-mode="add-unique"
                    maxlength="64"
                    class="dict-mono"
                    @filter="onFilterCategory"
                    @update:model-value="onCategoryPicked"
                  />
                  <!-- 分类名称是分类级元数据：既有分类只读继承（紧凑只读行，不占输入框），仅新分类时可写 -->
                  <div v-if="isCategoryLocked" class="dict-cat-inherit">
                    <q-icon name="lock" size="13px" />
                    <span class="dict-cat-inherit__text" :title="form.categoryDesc || form.category">{{
                      form.categoryDesc || form.category
                    }}</span>
                    <span class="dict-cat-inherit__tip">{{ t('dict.categoryDescLocked') }}</span>
                  </div>
                  <q-input
                    v-else
                    v-model="form.categoryDesc"
                    :label="t('dict.categoryName')"
                    dense
                    outlined
                    maxlength="128"
                  />
                </template>
                <q-input
                  v-model="form.value"
                  :label="t('dict.value')"
                  dense
                  outlined
                  type="textarea"
                  :rows="form.valueDataType === 'json' ? 8 : 3"
                  maxlength="2048"
                  counter
                  :hint="t('dict.valueHint')"
                  :class="form.valueDataType === 'json' || form.valueDataType === 'number' ? 'dict-mono' : ''"
                  :rules="[valueTypeRule]"
                  @blur="onValueBlur"
                >
                  <template v-if="canFormatJson" #append>
                    <q-icon name="data_object" size="16px" class="cursor-pointer dict-format-btn" @click="onFormatJson">
                      <q-tooltip>{{ t('dict.formatJson') }}</q-tooltip>
                    </q-icon>
                  </template>
                </q-input>
                <q-input v-model="form.valueDesc" :label="t('dict.valueDesc')" dense outlined />

                <!-- 高级选项：低频字段折叠收纳，header caption 摘要当前值免展开可见 -->
                <q-expansion-item
                  v-model="advancedOpen"
                  dense
                  class="dict-advanced"
                  :label="t('dict.sectionAdvanced')"
                  :caption="advancedSummary"
                >
                  <div class="row q-col-gutter-sm">
                    <div class="col-6">
                      <q-select
                        v-model="form.valueDataType"
                        :options="dataTypeOptions"
                        :label="t('dict.valueDataType')"
                        dense
                        outlined
                        clearable
                        emit-value
                        map-options
                        @update:model-value="typeTouched = true"
                      />
                    </div>
                    <div class="col-6">
                      <q-input v-model.number="form.sortId" :label="t('common.sort')" dense outlined type="number" />
                    </div>
                    <div class="col-12">
                      <q-toggle
                        :model-value="form.status === 'ON'"
                        :label="form.status === 'ON' ? t('common.enabled') : t('common.disabled')"
                        :disable="isBuiltinParamForm"
                        @update:model-value="(val) => (form.status = val ? 'ON' : 'OFF')"
                      >
                        <q-tooltip v-if="isBuiltinParamForm">{{ t('dict.builtinEntryLocked') }}</q-tooltip>
                      </q-toggle>
                    </div>
                    <div class="col-12">
                      <q-input
                        v-model="form.remark"
                        :label="t('common.remark')"
                        dense
                        outlined
                        type="textarea"
                        rows="2"
                      />
                    </div>
                  </div>
                </q-expansion-item>
              </div>
              <template #footer-extra>
                <q-btn
                  v-if="!isEdit"
                  outline
                  icon="playlist_add"
                  :label="t('dict.saveContinue')"
                  :loading="saving"
                  @click="onSaveContinue"
                />
              </template>
            </FormDrawer>

            <!-- 分类管理弹窗（实体化）：新建（编码+名称+用途）/ 编辑（名称/用途/备注，改名服务端批量同步条目） -->
            <q-dialog v-model="catDialogOpen">
              <q-card class="dict-cat-dialog column no-wrap">
                <q-card-section class="row items-center justify-between q-pb-sm">
                  <div class="text-subtitle1 text-weight-bold">
                    {{ catDialogMode === 'create' ? t('dict.addCategory') : t('dict.editCategoryTip') }}
                  </div>
                  <q-btn flat round dense icon="close" :aria-label="t('common.close')" @click="catDialogOpen = false" />
                </q-card-section>
                <q-separator />
                <q-card-section class="q-gutter-md q-pt-md">
                  <!-- 模型说明：分类为独立实体，用途决定其条目如何被使用 -->
                  <div class="dict-cat-dialog__hint">
                    <q-icon name="info" size="15px" />
                    <span>{{ catDialogMode === 'create' ? t('dict.catCreateHint') : t('dict.catEditHint') }}</span>
                  </div>
                  <q-input
                    v-model="catForm.code"
                    :label="t('dict.categoryCode')"
                    dense
                    outlined
                    maxlength="64"
                    class="dict-mono"
                    :readonly="catDialogMode === 'edit'"
                    :hint="catDialogMode === 'create' ? t('dict.categoryCodeHint') : undefined"
                    :rules="[(val) => !!val || t('common.required')]"
                  />
                  <q-input v-model="catForm.name" :label="t('dict.categoryName')" dense outlined maxlength="128" />
                  <q-select
                    v-model="catForm.kind"
                    :options="kindOptions"
                    :label="t('dict.kind')"
                    dense
                    outlined
                    emit-value
                    map-options
                    :disable="catDialogMode === 'edit' && catForm.builtin"
                  >
                    <q-tooltip v-if="catDialogMode === 'edit' && catForm.builtin">
                      {{ t('dict.builtinKindLocked') }}
                    </q-tooltip>
                  </q-select>
                  <q-input
                    v-model="catForm.remark"
                    :label="t('common.remark')"
                    dense
                    outlined
                    type="textarea"
                    rows="2"
                  />
                  <!-- 业务消费方：服务端注册表权威（只读），命中即业务接入分类 -->
                  <div v-if="catDialogMode === 'edit' && catForm.consumers.length" class="dict-cat-consumers">
                    <div class="dict-cat-consumers__label">
                      <q-icon name="link" size="13px" />
                      <span>{{ t('dict.consumedBy') }}</span>
                    </div>
                    <div v-for="c in catForm.consumers" :key="c" class="dict-cat-consumers__item">{{ c }}</div>
                  </div>
                </q-card-section>
                <q-separator />
                <q-card-section class="row items-center q-gutter-sm">
                  <!-- 删除分类：仅非内置空分类（服务端护栏兜底，此处仅做入口可见性） -->
                  <q-btn
                    v-if="catDialogMode === 'edit' && !catForm.builtin"
                    flat
                    color="negative"
                    icon="delete"
                    :label="t('dict.deleteCategory')"
                    :disable="catSaving"
                  >
                    <q-popup-confirm @confirm="onDeleteCategory">
                      {{ t('dict.deleteCategoryConfirm', { code: catForm.code }) }}
                    </q-popup-confirm>
                  </q-btn>
                  <q-space />
                  <q-btn flat :label="t('common.cancel')" :disable="catSaving" @click="catDialogOpen = false" />
                  <q-btn
                    unelevated
                    color="primary"
                    :label="t('common.save')"
                    :loading="catSaving"
                    @click="onSaveCategory"
                  />
                </q-card-section>
              </q-card>
            </q-dialog>
          </template>
        </CrudTablePage>
      </template>
    </q-splitter>
  </q-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, type QTableColumn, type QInput } from 'quasar';
import CrudTablePage from '../../components/CrudTablePage.vue';
import FormDrawer from '../../components/FormDrawer.vue';
import { useDictStore } from '../../stores';
import { dictService, type DictEntity, type DictCategoryEntity } from '../../services';

const { t } = useI18n();
const store = useDictStore();

const crudRef = ref<InstanceType<typeof CrudTablePage> | null>(null);
const selectedCategory = ref<string>('');
// 双视图：枚举选项（给下拉/业务编码提供候选）/ 系统参数（后端代码读取的运行时配置）
const kindTab = ref<'enum' | 'param'>('enum');

interface FormState {
  id?: string;
  key: string;
  category: string;
  categoryDesc: string;
  value: string;
  valueDesc: string;
  valueDataType: string | null;
  sortId: number;
  status: string;
  remark: string;
}

function defaultForm(): FormState {
  return {
    key: '',
    category: selectedCategory.value || '',
    categoryDesc: '',
    value: '',
    valueDesc: '',
    valueDataType: null,
    sortId: 0,
    status: 'ON',
    remark: '',
  };
}

const form = ref<FormState>(defaultForm());
const drawerOpen = ref(false);
const saving = ref(false);
const isEdit = ref(false);

const drawerRef = ref<InstanceType<typeof FormDrawer> | null>(null);
const keyInputRef = ref<QInput | null>(null);
// 用户手动改过类型后不再自动推断（尊重显式选择）
const typeTouched = ref(false);
const advancedOpen = ref(false);

// 新建时标题带分类上下文（"新建条目 · cable_type"），减少"在给谁加"的回认成本
const drawerTitle = computed(() => {
  if (isEdit.value) return t('common.edit');
  return form.value.category ? t('dict.createInCategory', { cat: form.value.category }) : t('common.create');
});

// ── 分类实体数据源（P2：dict_category 表，条目数/用途/内置/消费方均由服务端权威下发）──
const categories = ref<DictCategoryEntity[]>([]);
async function refreshCategories() {
  const resp = await dictService.listCategories();
  categories.value = resp.items ?? [];
}

// 分类元信息视图模型：code + 中文名 + 条目数 + 用途/内置/消费方（面板与表单 combobox 共用）
interface CategoryMeta {
  name: string; // 分类编码（= dict.category）
  desc: string; // 分类中文名
  count: number;
  kind: string; // enum / param
  builtin: boolean;
  consumers: string[];
}
const categoryMeta = computed<CategoryMeta[]>(() =>
  categories.value.map((c) => ({
    name: c.code,
    desc: c.name ?? '',
    count: c.entryCount ?? 0,
    kind: c.kind ?? 'enum',
    builtin: c.builtin ?? false,
    consumers: c.consumers ?? [],
  })),
);

// 分类面板搜索 + 双视图 kind 过滤
const categoryFilter = ref('');
const filteredCategoryMeta = computed<CategoryMeta[]>(() => {
  const kw = categoryFilter.value.trim().toLowerCase();
  return categoryMeta.value.filter(
    (c) => c.kind === kindTab.value && (!kw || c.name.toLowerCase().includes(kw) || c.desc.toLowerCase().includes(kw)),
  );
});

// 摘要条统计（全部来自分类实体，无需拉取全量条目）
const enumCount = computed(() => categoryMeta.value.filter((c) => c.kind === 'enum').length);
const paramCount = computed(() => categoryMeta.value.filter((c) => c.kind === 'param').length);
const totalEntries = computed(() => categoryMeta.value.reduce((sum, c) => sum + c.count, 0));
const consumedCount = computed(() => categoryMeta.value.filter((c) => c.consumers.length > 0).length);
// 「全部」行计数 = 当前视图下各分类条目数之和
const tabEntryTotal = computed(() =>
  categoryMeta.value.filter((c) => c.kind === kindTab.value).reduce((sum, c) => sum + c.count, 0),
);

// 业务消费方说明（服务端注册表权威）：面板 tooltip / 表单横幅 / 删除警示共用
function consumerName(cat: string): string {
  const meta = categoryMeta.value.find((c) => c.name === cat);
  return meta && meta.consumers.length ? meta.consumers.join('；') : '';
}

// 内置参数类条目（值被后端代码读取）：启停/删除锁定，仅允许改值（后端同口径 409 护栏）
function isBuiltinParamRow(row: Record<string, unknown>): boolean {
  const cat = (row as unknown as DictEntity).category ?? '';
  const meta = categoryMeta.value.find((c) => c.name === cat);
  return !!meta && meta.builtin && meta.kind === 'param';
}

// 当前表单分类是否为内置参数类（编辑态锁定高级区的状态开关）
const isBuiltinParamForm = computed(() => {
  const meta = categoryMeta.value.find((c) => c.name === form.value.category);
  return !!meta && meta.builtin && meta.kind === 'param';
});

// 分类 combobox 选项（可输入过滤，可新建；新编码由服务端自动登记为 enum 分类）
const categoryOptions = ref<string[]>([]);
// 用户已键入但未回车/点选的分类文本：new-value-mode 仅在 Enter/选项时提交，
// 直接点保存会静默丢弃键入值并沿用默认分类，onSubmit 兜底采纳
let lastCategoryInput = '';
function onFilterCategory(val: string, update: (fn: () => void) => void) {
  lastCategoryInput = val.trim();
  update(() => {
    const kw = val.toLowerCase();
    categoryOptions.value = categoryMeta.value.map((c) => c.name).filter((n) => !kw || n.toLowerCase().includes(kw));
  });
}

// 选中已有分类时自动带出分类中文名（避免同分类多行描述不一致）
function onCategoryPicked(val: string | null) {
  if (!val) return;
  lastCategoryInput = '';
  const meta = categoryMeta.value.find((c) => c.name === val);
  if (meta && meta.desc && !form.value.categoryDesc) {
    form.value.categoryDesc = meta.desc;
  }
}

// 字典值数据类型选项（对应后端 value_data_type 列）
const dataTypeOptions = computed(() => [
  { label: t('dict.dataTypeString'), value: 'string' },
  { label: t('dict.dataTypeNumber'), value: 'number' },
  { label: t('dict.dataTypeBoolean'), value: 'boolean' },
  { label: t('dict.dataTypeJson'), value: 'json' },
]);
function dataTypeLabel(v: string): string {
  return dataTypeOptions.value.find((o) => o.value === v)?.label ?? v;
}

// 分类用途选项（枚举选项 / 系统参数）
const kindOptions = computed(() => [
  { label: t('dict.kindEnum'), value: 'enum' },
  { label: t('dict.kindParam'), value: 'param' },
]);

// ── 表单智能化：类型自动推断 / 值按类型校验 / JSON 一键格式化 / 分类名称继承锁 ──
function isValidJson(v: string): boolean {
  try {
    JSON.parse(v);
    return true;
  } catch {
    return false;
  }
}

// 值失焦自动推断类型（仅新建且未手选类型时）：数字→number，true/false→boolean，合法 JSON→json
function onValueBlur() {
  if (typeTouched.value || isEdit.value) return;
  const v = form.value.value.trim();
  let detected = 'string';
  if (v) {
    if (/^-?\d+(\.\d+)?$/.test(v)) detected = 'number';
    else if (/^(true|false)$/i.test(v)) detected = 'boolean';
    else if (/^[{[]/.test(v) && isValidJson(v)) detected = 'json';
  }
  form.value.valueDataType = detected;
}

// 值按声明类型即时校验（空值不校验；string 无约束）
function valueTypeRule(val: string): true | string {
  const v = (val ?? '').trim();
  if (!v || !form.value.valueDataType) return true;
  switch (form.value.valueDataType) {
    case 'number':
      return /^-?\d+(\.\d+)?$/.test(v) || t('dict.numberInvalid');
    case 'boolean':
      return /^(true|false)$/i.test(v) || t('dict.booleanInvalid');
    case 'json':
      return isValidJson(v) || t('dict.jsonInvalid');
    default:
      return true;
  }
}

// 合法 JSON 且非美化态时显示一键格式化按钮
const canFormatJson = computed(() => {
  if (form.value.valueDataType !== 'json') return false;
  const v = form.value.value.trim();
  if (!v || !isValidJson(v)) return false;
  return v !== JSON.stringify(JSON.parse(v), null, 2);
});
function onFormatJson() {
  form.value.value = JSON.stringify(JSON.parse(form.value.value.trim()), null, 2);
}

// 既有分类的名称只读继承（分类级元数据，避免同分类各条目描述各自为政）
const isCategoryLocked = computed(() => categoryMeta.value.some((c) => c.name === form.value.category));

// 高级区 header 摘要：类型/排序/状态免展开一眼可见
const advancedSummary = computed(() =>
  [
    form.value.valueDataType ? dataTypeLabel(form.value.valueDataType) : t('dict.dataTypeString'),
    `${t('common.sort')} ${form.value.sortId}`,
    form.value.status === 'ON' ? t('common.enabled') : t('common.disabled'),
  ].join(' · '),
);

// 当前表单分类的业务消费方名称（未命中为 ''）
const consumedModuleName = computed(() => consumerName(form.value.category));

// 删除确认文案：被消费分类追加业务影响警示
function deleteConfirmText(row: Record<string, unknown>) {
  const cat = (row as unknown as DictEntity).category ?? '';
  const consumer = consumerName(cat);
  if (consumer) {
    return t('dict.deleteConsumed', { module: consumer });
  }
  return t('common.confirmDelete');
}

// 定宽策略：fixed 布局下 N-1 列显式定宽、仅"字典值"列 width:auto 独占全部剩余宽度，
// 表头与内容列宽天然一致（不会出现操作列吃掉剩余空间 / 表头内容错位的分摊歧义）
const colW = (px: number) => ({ style: `width: ${px}px`, headerStyle: `width: ${px}px` });
const colAuto = { style: 'width: auto', headerStyle: 'width: auto' };
const columns = computed<QTableColumn[]>(() => {
  const base: QTableColumn[] = [
    { name: 'key', label: t('dict.key'), field: 'key', align: 'left', sortable: true, ...colW(150) },
    { name: 'category', label: t('dict.category'), field: 'category', align: 'left', sortable: true, ...colW(280) },
    { name: 'value', label: t('dict.value'), field: 'value', align: 'left', ...colAuto },
    { name: 'valueDataType', label: t('dict.valueDataType'), field: 'valueDataType', align: 'center', ...colW(110) },
    { name: 'status', label: t('common.status'), field: 'status', align: 'center', ...colW(80) },
    {
      name: 'createTime',
      label: t('common.createTime'),
      field: 'createTime',
      align: 'center',
      sortable: true,
      ...colW(190),
    },
    { name: 'actions', label: t('common.actions'), field: 'actions', align: 'center', sortable: false, ...colW(130) },
  ];
  // 选中分类后所有行同属该分类：隐藏分类列消除重复信息，宽度让给值列
  return selectedCategory.value ? base.filter((c) => c.name !== 'category') : base;
});

// 右侧标题随选中分类切换上下文（"字典项 · {分类}"），回「全部」时恢复总标题
const selectedCatDisplay = computed(() => {
  if (!selectedCategory.value) return '';
  const meta = categoryMeta.value.find((c) => c.name === selectedCategory.value);
  return meta?.desc || selectedCategory.value;
});
const tableTitle = computed(() =>
  selectedCatDisplay.value ? t('dict.entriesInCategory', { cat: selectedCatDisplay.value }) : t('dict.title'),
);

const searchFields = computed(() => [
  { name: 'key', label: t('dict.key'), type: 'input' as const },
  { name: 'value', label: t('dict.value'), type: 'input' as const },
]);

async function fetchData(params: {
  page: number;
  pageSize: number;
  noPaging: boolean;
  formValues: Record<string, unknown>;
}) {
  const merged = { ...params.formValues };
  if (selectedCategory.value) {
    merged.category = selectedCategory.value;
  } else {
    // 未选分类（「全部」行）：表格与双视图同口径，只列当前用途分类下的条目
    merged.categoryKind = kindTab.value;
  }
  const resp = await store.fetchList({
    page: params.page,
    pageSize: params.pageSize,
    noPaging: params.noPaging,
    formValues: merged,
  });
  return { items: resp.items as unknown as Record<string, unknown>[], total: resp.total };
}

// 双视图切换：原选中分类在新视图不可见，重置回「全部」避免隐藏过滤态
watch(kindTab, () => {
  if (selectedCategory.value) {
    selectedCategory.value = '';
  }
  crudRef.value?.fetchData();
});

function onSelectCategory(cat: string) {
  selectedCategory.value = cat;
  crudRef.value?.fetchData();
}

function onCreate() {
  isEdit.value = false;
  typeTouched.value = false;
  advancedOpen.value = false;
  form.value = defaultForm();
  drawerOpen.value = true;
}

function fillFormFromEntity(entity: DictEntity) {
  form.value = {
    id: entity.id ? String(entity.id) : undefined,
    key: entity.key || '',
    category: entity.category || '',
    categoryDesc: entity.categoryDesc || '',
    value: entity.value || '',
    valueDesc: entity.valueDesc || '',
    valueDataType: entity.valueDataType ?? null,
    sortId: entity.sortId ?? 0,
    status: entity.status ?? 'ON',
    remark: entity.remark || '',
  };
}

function onEdit(row: Record<string, unknown>) {
  const entity = row as unknown as DictEntity;
  isEdit.value = true;
  typeTouched.value = true; // 编辑态保护既有类型，不做自动推断
  fillFormFromEntity(entity);
  // 高级区含非默认值时自动展开，避免"藏着的值"被忽略
  advancedOpen.value = !!(
    entity.remark ||
    (entity.sortId ?? 0) !== 0 ||
    (entity.valueDataType && entity.valueDataType !== 'string')
  );
  drawerOpen.value = true;
}

// 复制条目：除键外全部预填（id 清空走新建），同分类相似条目只需补一个新键
function onDuplicate(row: Record<string, unknown>) {
  const entity = row as unknown as DictEntity;
  isEdit.value = false;
  typeTouched.value = true; // 复制携带类型，不再自动推断
  fillFormFromEntity(entity);
  form.value.id = undefined;
  form.value.key = '';
  drawerOpen.value = true;
}

async function onSubmit(continueAdding: boolean) {
  // combobox 键入未提交的分类文本兜底采纳（与 Enter/new-value-mode 语义一致）
  if (lastCategoryInput && lastCategoryInput !== form.value.category) {
    form.value.category = lastCategoryInput;
  }
  saving.value = true;
  try {
    const { id, ...values } = form.value;
    if (isEdit.value && id) {
      await store.update(id, values);
      Notify.create({ type: 'positive', message: t('common.updateSuccess') });
      drawerOpen.value = false;
    } else {
      await store.create(values);
      Notify.create({ type: 'positive', message: t('common.createSuccess') });
      if (continueAdding) {
        // 连续添加：保留分类/描述/类型/排序/状态上下文，仅清键/值/值描述，聚焦回键输入
        form.value = { ...form.value, id: undefined, key: '', value: '', valueDesc: '' };
        await nextTick();
        keyInputRef.value?.focus();
      } else {
        drawerOpen.value = false;
      }
    }
    await crudRef.value?.fetchData();
    // 条目数/新分类登记在服务端完成，刷新分类实体即可（无需拉全量条目）
    await refreshCategories();
  } catch {
    // error notification handled by axios interceptor
  } finally {
    saving.value = false;
  }
}

// 保存并继续（Ctrl+Enter / 底部按钮）：复用 FormDrawer 暴露的同一套 q-form 校验
async function onSaveContinue() {
  if (isEdit.value || saving.value) return;
  const ok = await drawerRef.value?.validate();
  if (ok) await onSubmit(true);
}

async function onDelete(row: Record<string, unknown>) {
  const entity = row as unknown as DictEntity;
  if (!entity.id) return;
  try {
    await store.remove(entity.id);
    Notify.create({ type: 'positive', message: t('common.deleteSuccess') });
    await crudRef.value?.fetchData();
    await refreshCategories();
  } catch {
    // error notification handled by axios interceptor
  }
}

// 状态列开关请求中行 id（防连点，null=空闲，与全项目启停开关约定一致）
const togglingId = ref<string | number | null>(null);

async function onToggleStatus(row: Record<string, unknown>, val: boolean) {
  const entity = row as unknown as DictEntity;
  if (!entity.id || togglingId.value !== null) return;
  togglingId.value = entity.id;
  try {
    await store.update(entity.id, { status: val ? 'ON' : 'OFF' });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    await crudRef.value?.fetchData();
  } catch {
    // error notification handled by axios interceptor
  } finally {
    togglingId.value = null;
  }
}

onMounted(async () => {
  // 默认「全部」分类（selectedCategory=''），分类面板走独立分类实体接口
  await refreshCategories();
});

// ── 分类管理（P2 分类实体化：dict_category 独立实体 CRUD，告别"首个条目"组合 hack）──
const catDialogOpen = ref(false);
const catDialogMode = ref<'create' | 'edit'>('create');
const catSaving = ref(false);
const catForm = ref({
  code: '',
  name: '',
  kind: 'enum',
  builtin: false,
  remark: '',
  consumers: [] as string[],
});

function onCreateCategory() {
  catDialogMode.value = 'create';
  // 新建分类用途默认跟随当前视图（参数视图下建参数分类，减少一次切换）
  catForm.value = { code: '', name: '', kind: kindTab.value, builtin: false, remark: '', consumers: [] };
  catDialogOpen.value = true;
}

function onEditCategory(cat: CategoryMeta) {
  catDialogMode.value = 'edit';
  catForm.value = {
    code: cat.name,
    name: cat.desc,
    kind: cat.kind,
    builtin: cat.builtin,
    remark: categories.value.find((c) => c.code === cat.name)?.remark ?? '',
    consumers: [...cat.consumers],
  };
  catDialogOpen.value = true;
}

async function onSaveCategory() {
  const code = catForm.value.code.trim();
  if (!code) return;
  catSaving.value = true;
  try {
    if (catDialogMode.value === 'create') {
      await dictService.createCategory({
        code,
        name: catForm.value.name.trim(),
        kind: catForm.value.kind,
        remark: catForm.value.remark,
      });
    } else {
      // 改名由服务端单点同步该分类全部条目的 categoryDesc（替代前端 N 次循环更新）
      await dictService.updateCategory(code, {
        name: catForm.value.name.trim(),
        kind: catForm.value.kind,
        remark: catForm.value.remark,
      });
    }
    Notify.create({ type: 'positive', message: t('dict.categorySaved') });
    catDialogOpen.value = false;
    await crudRef.value?.fetchData();
    await refreshCategories();
  } catch {
    // error notification handled by axios interceptor
  } finally {
    catSaving.value = false;
  }
}

async function onDeleteCategory() {
  const code = catForm.value.code;
  if (!code) return;
  catSaving.value = true;
  try {
    await dictService.deleteCategory(code);
    Notify.create({ type: 'positive', message: t('dict.categoryDeleted') });
    catDialogOpen.value = false;
    if (selectedCategory.value === code) {
      selectedCategory.value = '';
    }
    await crudRef.value?.fetchData();
    await refreshCategories();
  } catch {
    // error notification handled by axios interceptor（服务端护栏文案透传）
  } finally {
    catSaving.value = false;
  }
}
</script>

<style lang="sass">
// ── 摘要条：单行紧凑统计，与页面同节奏（替代原 4 张装饰性统计卡） ──
.dict-summary
  display: flex
  align-items: center
  gap: 10px
  padding: 8px 14px
  border-radius: 10px
  background: color-mix(in srgb, var(--glass-surface) 72%, transparent)
  border: 1px solid var(--glass-border)
  font-size: 0.8rem
  color: var(--color-text-secondary)

.dict-summary__icon
  color: var(--color-accent)

.dict-summary__item
  font-variant-numeric: tabular-nums
  white-space: nowrap

.dict-summary__item--link
  display: inline-flex
  align-items: center
  gap: 4px
  color: var(--color-accent)

.dict-summary__sep
  width: 1px
  height: 12px
  background: var(--glass-border)

// ── 分栏高度：视口扣除页头/摘要条；分隔竖线隐藏（拖拽已 disable） ──
.dict-splitter
  height: calc(100vh - 172px)

  .q-splitter__separator
    display: none

  // 右栏与左分类卡顶/底边对齐：fill 模式去掉 q-pa-lg 四边内距，仅保留栏间距
  // （外层 q-page 自带 q-pa-lg 提供页面外边距，右栏再留 24px 会比左栏矮一截）
  .crud-page--fill
    padding: 0 0 0 16px

// ── 分类导航面板：去装饰（无色点/占比条/霓虹），信息优先的紧凑行 ──
// 滚动条根治：q-scrollarea 内容层宽度锁定 100%（默认随 max-content 撑宽，
// nowrap 文本会把内容层顶出横向滚动条）；宽度锁死后由 min-width:0 链路上的 ellipsis 接管截断
.dict-nav
  overflow: hidden

  .q-scrollarea__content
    width: 100%

.dict-nav__header
  padding-bottom: 12px

.dict-nav__title
  display: flex
  align-items: center
  gap: 8px
  font-size: 0.86rem
  font-weight: 600
  color: var(--color-text-heading)

.dict-nav__title-count
  margin-left: auto
  font-size: 0.72rem
  font-weight: 600
  font-variant-numeric: tabular-nums
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--color-text-secondary) 8%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-text-secondary) 20%, transparent)
  border-radius: 999px
  padding: 1px 9px

// 新建分类入口：标题行尾小按钮，悬停点亮
.dict-nav__add
  color: var(--color-text-secondary)
  transition: color 0.15s ease, background 0.15s ease

  &:hover
    color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)

// 双视图 tabs：贴标题行下方，紧凑无大写
.dict-nav__tabs
  margin-top: 8px

  :deep(.q-tab)
    min-height: 32px
    padding: 0 12px
    font-size: 0.8rem

.dict-nav__search
  margin-top: 10px

// ── 分类行列表：行间细分隔线，行内名称/编码/标记/计数 ──
.dict-nav__list
  display: flex
  flex-direction: column
  padding: 4px 0

.dict-nav-row
  display: flex
  align-items: center
  padding: 8px 14px
  cursor: pointer
  border-left: 3px solid transparent
  transition: background 0.12s ease, border-color 0.12s ease

  &:hover
    background: color-mix(in srgb, var(--color-accent) 6%, transparent)

    .dict-nav-row__edit
      opacity: 1

// 选中态：左侧色条 + 浅底（与全站列表选中语言一致，无霓虹发光）
.dict-nav-row--active
  border-left-color: var(--color-accent)
  background: color-mix(in srgb, var(--color-accent) 10%, transparent)

  .dict-nav-row__name
    color: var(--color-accent)
    font-weight: 600

.dict-nav-row__main
  flex: 1
  min-width: 0

.dict-nav-row__line
  display: flex
  align-items: center
  gap: 6px
  min-width: 0

.dict-nav-row__name
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  font-size: 0.84rem
  color: var(--color-text-primary)
  line-height: 1.35

.dict-nav-row__code
  margin-top: 2px
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  font-size: 0.72rem
  font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace
  color: var(--color-text-tertiary)

// 分类行编辑入口：默认隐身，悬停行显现（不挤占常态布局）
.dict-nav-row__edit
  opacity: 0
  flex: none
  color: var(--color-text-tertiary)
  transition: opacity 0.15s ease, color 0.15s ease

  &:hover
    color: var(--color-accent)

.dict-nav-row__lock
  flex: none
  color: var(--color-text-tertiary)

.dict-nav-row__link
  flex: none
  color: var(--color-accent)

.dict-nav-row__count
  flex: none
  font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace
  font-size: 0.72rem
  font-weight: 600
  line-height: 18px
  padding: 0 6px
  border-radius: 6px
  color: var(--color-text-secondary)
  border: 1px solid color-mix(in srgb, var(--color-text-secondary) 24%, transparent)
  font-variant-numeric: tabular-nums

.dict-nav__empty
  padding: 18px 12px
  text-align: center
  font-size: 0.78rem
  color: var(--color-text-tertiary)

// ── 表单：分区标题 / 等宽输入 / 高级折叠区 ──
// 编辑态身份上下文条：编码(mono) + 分类徽章 + 分类描述，一行只读呈现"在改谁"
.dict-context
  display: flex
  align-items: center
  gap: 8px
  min-width: 0
  padding: 9px 12px
  border-radius: 10px
  background: color-mix(in srgb, var(--color-accent) 7%, var(--glass-surface))
  border: 1px solid color-mix(in srgb, var(--color-accent) 22%, var(--glass-border))

.dict-context__key
  font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace
  font-size: 0.86rem
  font-weight: 600
  color: var(--color-accent)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.dict-context__cat
  flex: none
  max-width: 220px
  overflow: hidden
  text-overflow: ellipsis

.dict-context__desc
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  font-size: 0.76rem
  color: var(--color-text-secondary)

.dict-context__lock
  flex: none
  color: var(--color-text-tertiary)

// 新建态·既有分类的名称继承行：紧凑只读，替代大输入框
.dict-cat-inherit
  display: flex
  align-items: center
  gap: 7px
  min-width: 0
  padding: 0 2px
  margin-top: -6px
  font-size: 0.76rem
  color: var(--color-text-tertiary)

  .q-icon
    flex: none

.dict-cat-inherit__text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  color: var(--color-text-secondary)

.dict-cat-inherit__tip
  flex: none
  margin-left: auto
  opacity: 0.75

.dict-form-section
  display: flex
  align-items: center
  gap: 8px
  margin: 2px 0 -8px
  font-size: 0.78rem
  font-weight: 600
  letter-spacing: 0.05em
  color: var(--color-text-secondary)

  .q-icon
    color: var(--color-accent)

  &::after
    content: ''
    flex: 1
    height: 1px
    background: linear-gradient(90deg, color-mix(in srgb, var(--color-accent) 32%, transparent), transparent)

// 键/分类/JSON·数值 值用等宽字体（编码可读性）
.dict-mono
  .q-field__native,
  .q-field__input
    font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace

.dict-lock-icon
  color: var(--color-text-tertiary)

.dict-format-btn
  color: var(--color-text-secondary)
  transition: color 0.15s ease

  &:hover
    color: var(--color-accent)

// 高级选项折叠区：玻璃描边卡片化，与基础字段视觉分层
.dict-advanced
  border: 1px solid var(--glass-border)
  border-radius: 10px
  overflow: hidden

  .q-item
    min-height: 40px
    padding: 4px 12px

  .q-item__label
    font-size: 0.8rem
    font-weight: 600
    color: var(--color-text-secondary)

  .q-item__label--caption
    font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace
    font-size: 0.72rem
    color: var(--color-text-tertiary)

  .q-expansion-item__content
    padding: 2px 12px 12px

// ── 表格自定义单元格 ──
.dict-key
  font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace
  font-size: 0.82rem
  font-weight: 600
  color: var(--color-accent)

.dict-cat-badge
  color: var(--color-accent-cyan)
  font-size: 0.75rem
  font-weight: 600
  cursor: default

// 字典值：单行截断（fixed 布局 td 自身 ellipsis 不穿透子元素，需块级自截断）
.dict-value
  display: block
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  color: var(--color-text-primary)

// 悬停浮层：值全称 + 值说明（长 JSON 可换行读完，超高内部滚动）
.dict-value-tip
  max-width: 560px
  max-height: 320px
  overflow-y: auto

.dict-value-tip__label
  font-size: 0.68rem
  font-weight: 600
  letter-spacing: 0.04em
  opacity: 0.62
  margin-top: 2px

  &:first-child
    margin-top: 0

.dict-value-tip__value
  font-family: ui-monospace, 'Cascadia Mono', Consolas, monospace
  font-size: 0.78rem
  word-break: break-all
  white-space: pre-wrap

.dict-value-tip__desc
  font-size: 0.76rem
  word-break: break-all
  white-space: pre-wrap

.dict-type
  font-size: 0.72rem
  font-weight: 600
  padding: 2px 8px
  border-radius: 6px
  border: 1px solid transparent

.dict-type--string
  color: var(--color-accent-blue)
  background: color-mix(in srgb, var(--color-accent-blue) 13%, transparent)
  border-color: color-mix(in srgb, var(--color-accent-blue) 35%, transparent)

.dict-type--number
  color: var(--color-accent-cyan)
  background: color-mix(in srgb, var(--color-accent-cyan) 13%, transparent)
  border-color: color-mix(in srgb, var(--color-accent-cyan) 35%, transparent)

.dict-type--boolean
  color: var(--color-success)
  background: color-mix(in srgb, var(--color-success) 13%, transparent)
  border-color: color-mix(in srgb, var(--color-success) 35%, transparent)

.dict-type--json
  color: var(--color-alarm-emergency)
  background: color-mix(in srgb, var(--color-alarm-emergency) 13%, transparent)
  border-color: color-mix(in srgb, var(--color-alarm-emergency) 35%, transparent)

.dict-cell-empty
  color: var(--color-text-tertiary)

// 业务消费提示条：与玻璃态 token 对齐，暗色下呈霓虹描边
.dict-consumed-banner
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  border: 1px solid color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))
  color: var(--color-text-primary)
  border-radius: 10px

// 分类管理弹窗：玻璃提升层卡片，宽度紧凑
.dict-cat-dialog
  width: 440px
  max-width: 92vw
  border-radius: 16px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  box-shadow: var(--glass-inner-highlight), 0 18px 50px rgba(0, 0, 0, 0.28)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

// 弹窗内模型说明条：信息图标 + 辅助色小字
.dict-cat-dialog__hint
  display: flex
  align-items: flex-start
  gap: 6px
  font-size: 0.76rem
  line-height: 1.5
  color: var(--color-text-secondary)
  padding: 8px 10px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 7%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 18%, transparent)

  .q-icon
    color: var(--color-accent)
    flex: none
    margin-top: 1px

// 消费方只读清单：标签 + 逐行说明
.dict-cat-consumers
  padding: 8px 10px
  border-radius: 8px
  border: 1px dashed color-mix(in srgb, var(--color-accent) 30%, transparent)
  font-size: 0.76rem

.dict-cat-consumers__label
  display: flex
  align-items: center
  gap: 5px
  font-weight: 600
  color: var(--color-accent)
  margin-bottom: 4px

.dict-cat-consumers__item
  color: var(--color-text-secondary)
  line-height: 1.6
</style>
