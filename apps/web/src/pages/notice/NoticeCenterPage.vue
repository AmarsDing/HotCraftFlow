<template>
  <div class="q-pa-md">
    <div class="text-h6">通知中心</div>
    <div class="text-grey">FrameWork 骨架占位页</div>
    <q-btn v-if="embedded" flat label="关闭" @click="$emit('close')" />
  </div>
</template>
<script setup lang="ts">
defineOptions({ name: "NoticeCenterPage" });
defineProps<{ embedded?: boolean }>();
defineEmits<{ close: [] }>();
</script>
