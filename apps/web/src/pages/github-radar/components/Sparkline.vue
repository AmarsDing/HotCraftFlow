<template>
  <svg
    class="radar-spark"
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    aria-hidden="true"
  >
    <polyline
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
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{ values: number[]; width?: number; height?: number; stroke?: string }>(),
  {
    width: 48,
    height: 16,
    stroke: 'var(--color-accent)',
  },
)

const points = computed(() => {
  const vals = props.values?.length ? props.values : [0]
  const min = Math.min(...vals)
  const max = Math.max(...vals)
  const span = max - min || 1
  const w = props.width
  const h = props.height
  const step = vals.length > 1 ? w / (vals.length - 1) : w
  return vals
    .map((v, i) => {
      const x = i * step
      const y = h - ((v - min) / span) * (h - 2) - 1
      return `${x},${y}`
    })
    .join(' ')
})
</script>

<style scoped>
.radar-spark {
  display: inline-block;
  vertical-align: middle;
  flex-shrink: 0;
}
</style>
