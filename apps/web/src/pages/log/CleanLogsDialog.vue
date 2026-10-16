<template>
  <q-dialog :model-value="modelValue" @update:model-value="onUpdateModel">
    <q-card style="min-width: 420px; max-width: 90vw">
      <q-card-section class="row items-center q-pb-none">
        <div class="text-h6">{{ title }}</div>
        <q-space />
        <q-btn icon="close" flat round dense :disable="loading" @click="onUpdateModel(false)" />
      </q-card-section>
      <q-card-section>
        <q-select
          v-model="keepDays"
          :options="keepDaysOptions"
          :label="keepDaysLabel"
          dense
          outlined
          emit-value
          map-options
        />
        <div class="text-caption text-negative q-mt-sm">{{ warning }}</div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="t('common.cancel')" :disable="loading" @click="onUpdateModel(false)" />
        <q-btn unelevated color="negative" :label="t('common.confirm')" :loading="loading" @click="onConfirm" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';

// 日志手动清理弹窗：选择保留天数 → 物理删除更早日志（不可恢复），操作/登录日志两页共用。
const props = defineProps<{
  modelValue: boolean;
  title: string;
  keepDaysLabel: string;
  // 保留天数选项文案模板（i18n 插值由父页命名空间提供）
  keepDaysOptionText: (days: number) => string;
  warning: string;
  defaultDays: number;
  cleanFn: (days: number) => Promise<number>;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  done: [deleted: number];
}>();

const { t } = useI18n();

// 预设保留天数（项目约定"选择代替手填"），与后端 1~3650 校验区间一致
const keepDayValues = [7, 30, 90, 180, 365];
const keepDaysOptions = computed(() => keepDayValues.map((d) => ({ label: props.keepDaysOptionText(d), value: d })));

const keepDays = ref(props.defaultDays);
const loading = ref(false);

// 每次打开重置为默认保留天数，避免残留上次选择
watch(
  () => props.modelValue,
  (open) => {
    if (open) keepDays.value = props.defaultDays;
  },
);

function onUpdateModel(value: boolean) {
  emit('update:modelValue', value);
}

async function onConfirm() {
  loading.value = true;
  try {
    const deleted = await props.cleanFn(keepDays.value);
    emit('done', deleted);
    emit('update:modelValue', false);
  } catch {
    // 错误提示由 axios 拦截器统一弹出
  } finally {
    loading.value = false;
  }
}
</script>
