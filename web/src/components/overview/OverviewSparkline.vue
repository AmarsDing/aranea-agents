<template>
  <svg class="ov-spark" :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="none" aria-hidden="true" focusable="false">
    <defs>
      <linearGradient :id="gradientId" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color="currentColor" stop-opacity="0.28" />
        <stop offset="100%" stop-color="currentColor" stop-opacity="0" />
      </linearGradient>
    </defs>
    <path v-if="area" :d="area" :fill="`url(#${gradientId})`" stroke="none" />
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
    <circle v-if="lastPoint" :cx="lastPoint.x" :cy="lastPoint.y" r="2" fill="currentColor" />
  </svg>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue';

const W = 100;
const H = 28;
const PAD = 2;

const props = defineProps<{ points: number[] }>();
const gradientId = `ov-spark-${useId()}`;

/** 归一化到 viewBox；全相等画中线，空序列不渲染。 */
const coords = computed(() => {
  const pts = props.points;
  if (!pts || pts.length < 2) return [];
  const min = Math.min(...pts);
  const max = Math.max(...pts);
  const span = max - min;
  const stepX = (W - PAD * 2) / (pts.length - 1);
  return pts.map((v, i) => ({
    x: PAD + i * stepX,
    y: span === 0 ? H / 2 : H - PAD - ((v - min) / span) * (H - PAD * 2),
  }));
});

const polyline = computed(() => coords.value.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' '));

const area = computed(() => {
  const pts = coords.value;
  if (!pts.length) return '';
  const line = pts.map((p) => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' L');
  return `M${line} L${pts[pts.length - 1].x.toFixed(2)},${H} L${pts[0].x.toFixed(2)},${H} Z`;
});

const lastPoint = computed(() => coords.value[coords.value.length - 1] ?? null);
</script>
