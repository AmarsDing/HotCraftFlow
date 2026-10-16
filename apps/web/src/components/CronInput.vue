<template>
  <q-input
    :model-value="modelValue"
    :placeholder="placeholder"
    :label="label"
    :stack-label="stackLabel"
    :hint="hint"
    :rules="rules"
    :disable="disable"
    dense
    outlined
    clearable
    @update:model-value="onInput"
  >
    <template #append>
      <q-icon name="schedule" class="cursor-pointer">
        <q-tooltip>{{ t('dataMgmt.cronPreset') }}</q-tooltip>
        <q-popup-proxy transition-show="scale" transition-hide="scale">
          <q-list dense class="cron-preset-list">
            <q-item v-for="p in presets" :key="p.value" v-close-popup clickable @click="onPreset(p.value)">
              <q-item-section>{{ p.label }}</q-item-section>
              <q-item-section side class="cron-preset-list__expr">{{ p.value }}</q-item-section>
            </q-item>
          </q-list>
        </q-popup-proxy>
      </q-icon>
    </template>
  </q-input>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ValidationRule } from 'quasar';

// Cron 输入框：手输 + 常用周期预设（选择即填入，仍可继续编辑）
withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    label?: string;
    stackLabel?: boolean;
    hint?: string;
    rules?: ValidationRule[];
    disable?: boolean;
  }>(),
  { placeholder: '', label: '', stackLabel: false, hint: '', rules: undefined, disable: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const { t } = useI18n();

const presets = computed(() => [
  { label: t('dataMgmt.cronPresetHourly'), value: '0 * * * *' },
  { label: t('dataMgmt.cronPresetDaily'), value: '0 3 * * *' },
  { label: t('dataMgmt.cronPresetWeekly'), value: '0 3 * * 1' },
  { label: t('dataMgmt.cronPresetMonthly'), value: '0 3 1 * *' },
]);

function onPreset(val: string) {
  emit('update:modelValue', val);
}

function onInput(val: string | number | null) {
  emit('update:modelValue', val == null ? '' : String(val));
}
</script>

<style lang="sass">
.cron-preset-list
  min-width: 260px
  background: var(--glass-elevated)
  border: 1px solid var(--glass-border)
  border-radius: 12px
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

.cron-preset-list__expr
  font-family: monospace
  font-size: 12px
  color: var(--color-text-tertiary)
</style>
