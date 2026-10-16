<template>
  <q-select
    :model-value="modelValue"
    :options="options"
    :placeholder="placeholder || 'Device'"
    dense
    outlined
    clearable
    emit-value
    map-options
    class="fw-device-tree-select"
    @update:model-value="onUpdate"
  />
</template>

<script setup lang="ts">
/** FrameWork stub: device tree selector removed with TwinMonitor domain */
withDefaults(
  defineProps<{
    modelValue?: string | number | null;
    placeholder?: string;
    options?: { label: string; value: string | number }[];
  }>(),
  { modelValue: null, placeholder: "", options: () => [] },
);
const emit = defineEmits<{ "update:modelValue": [v: string | number | null | undefined] }>();
function onUpdate(v: string | number | null | undefined) {
  emit("update:modelValue", v ?? null);
}
</script>