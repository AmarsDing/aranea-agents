<template>
  <svg class="ov-spark" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" aria-hidden="true" focusable="false">
    <polyline
      v-if="polyline"
      :points="polyline"
      fill="none"
      stroke="currentColor"
      stroke-width="1.6"
      stroke-linejoin="round"
      stroke-linecap="round"
      vector-effect="non-scaling-stroke"
    />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const W = 100;
const H = 28;
const PAD = 2;

const props = defineProps<{ points: number[] }>();

/** 归一化到 viewBox；全相等画中线，空序列不渲染。 */
const polyline = computed(() => {
  const pts = props.points;
  if (!pts || pts.length < 2) return '';
  const min = Math.min(...pts);
  const max = Math.max(...pts);
  const span = max - min;
  const stepX = (W - PAD * 2) / (pts.length - 1);
  return pts
    .map((v, i) => {
      const x = PAD + i * stepX;
      const y = span === 0 ? H / 2 : H - PAD - ((v - min) / span) * (H - PAD * 2);
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
});
</script>
