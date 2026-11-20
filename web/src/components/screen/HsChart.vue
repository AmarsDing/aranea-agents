<template>
  <div ref="el" class="hs-chart" role="img" :aria-label="label" />
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { EChartsCoreOption } from 'echarts/core';
import { useUsageChart } from '../../features/usage/useUsageChart';

// build 每次渲染都重新调用：切换昼夜后颜色令牌要重新读
const props = defineProps<{ build: () => EChartsCoreOption; deps: unknown[]; label: string }>();
const el = ref<HTMLElement | null>(null);
useUsageChart(
  el,
  () => props.build(),
  () => props.deps,
);
</script>
