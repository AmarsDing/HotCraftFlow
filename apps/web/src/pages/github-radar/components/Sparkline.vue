<template>
  <svg class="radar-spark" :width="width" :height="height" :viewBox="`0 0 ${width} ${height}`" aria-hidden="true">
    <!-- 单值时退化为圆点：一个点的折线不可见，误导为「无数据」 -->
    <circle v-if="singleDot" :cx="singleDot.x" :cy="singleDot.y" r="2" :fill="stroke" />
    <polyline
      v-else
      fill="none"
      :stroke="stroke"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
      :points="points"
    />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{ values: number[]; width?: number; height?: number; stroke?: string }>(), {
  width: 48,
  height: 16,
  // 图表序列色 token（UI 规范：图表一律走 --chart-series-*，accent 仅作兜底）
  stroke: 'var(--chart-series-1, var(--color-accent))',
});

const geometry = computed(() => {
  const vals = props.values?.length ? props.values : [];
  const min = Math.min(...vals);
  const max = Math.max(...vals);
  const span = max - min || 1;
  const w = props.width;
  const h = props.height;
  // 单值圆点居中（贴左边框会像被裁掉）；多值等距分布
  const step = vals.length > 1 ? w / (vals.length - 1) : w;
  return vals.map((v, i) => ({
    x: vals.length === 1 ? w / 2 : i * step,
    y: h - ((v - min) / span) * (h - 2) - 1,
  }));
});

const singleDot = computed(() => (geometry.value.length === 1 ? geometry.value[0] : null));

const points = computed(() => geometry.value.map((p) => `${p.x},${p.y}`).join(' '));
</script>

<style scoped>
.radar-spark {
  display: inline-block;
  vertical-align: middle;
  flex-shrink: 0;
}
</style>
