<!-- Material Icons 图标选择器：下拉网格 + 关键字过滤；支持手动输入自定义图标名兜底 -->
<template>
  <q-select
    :model-value="modelValue"
    :options="filteredOptions"
    use-input
    input-debounce="150"
    new-value-mode="add-unique"
    :placeholder="placeholder"
    dense
    outlined
    clearable
    @filter="onFilter"
    @new-value="onNewValue"
    @update:model-value="(val) => emit('update:modelValue', val ?? '')"
  >
    <template v-if="modelValue" #prepend>
      <q-icon :name="modelValue" size="20px" />
    </template>

    <template #option="{ itemProps, opt }">
      <q-item v-bind="itemProps" dense>
        <q-item-section avatar style="min-width: 28px">
          <q-icon :name="opt" size="20px" />
        </q-item-section>
        <q-item-section class="text-caption">{{ opt }}</q-item-section>
      </q-item>
    </template>

    <template #no-option>
      <q-item dense>
        <q-item-section class="text-grey-7 text-center q-pa-sm">
          {{ t('menuEntity.iconPickerNoMatch') }}
        </q-item-section>
      </q-item>
    </template>
  </q-select>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { materialIconNames } from '../config/materialIconNames';

defineProps<{
  modelValue: string;
  placeholder?: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const { t } = useI18n();

// 过滤关键字为空时只展示前若干条，避免 2122 条全量渲染卡顿
const FULL_LIMIT = 200;
const keyword = ref('');

const filteredOptions = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw) return materialIconNames.slice(0, FULL_LIMIT);
  return materialIconNames.filter((n) => n.includes(kw)).slice(0, FULL_LIMIT);
});

function onFilter(val: string, update: (fn: () => void) => void) {
  update(() => {
    keyword.value = val;
  });
}

// 手动输入不在库中的图标名（如 img:xxx / 自定义字体类）时回车采纳，保留旧 q-input 的开放性
type NewValueDone = (item?: unknown, mode?: 'add' | 'toggle' | 'add-unique') => void;
function onNewValue(val: string, done: NewValueDone) {
  const v = val.trim();
  if (v) {
    emit('update:modelValue', v);
    done(v, 'add-unique');
  } else {
    done('');
  }
}
</script>
