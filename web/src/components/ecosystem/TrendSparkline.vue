<template>
  <svg :viewBox="`0 0 ${width} ${height}`" class="trend-sparkline" preserveAspectRatio="none">
    <polyline
      :points="points"
      fill="none"
      :stroke="color"
      stroke-width="2"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
    <polygon :points="areaPoints" :fill="color" opacity="0.12" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(
  defineProps<{
    values: number[];
    width?: number;
    height?: number;
    color?: string;
  }>(),
  { width: 160, height: 40, color: 'var(--color-accent)' },
);

const coords = computed(() => {
  const vals = props.values;
  if (vals.length === 0) return [];
  const max = Math.max(...vals, 1);
  const min = Math.min(...vals, 0);
  const span = max - min || 1;
  const stepX = props.width / Math.max(vals.length - 1, 1);
  return vals.map((v, i) => ({
    x: i * stepX,
    y: props.height - 4 - ((v - min) / span) * (props.height - 8),
  }));
});

const points = computed(() => coords.value.map((c) => `${c.x},${c.y}`).join(' '));
const areaPoints = computed(() => {
  if (coords.value.length === 0) return '';
  return `0,${props.height} ${points.value} ${props.width},${props.height}`;
});
</script>

<style scoped>
.trend-sparkline {
  width: 100%;
  height: 40px;
  display: block;
}
</style>
