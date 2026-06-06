<template>
  <div class="overview-token-comp">
    <div v-if="!slices.length" class="overview-empty overview-empty--compact">暂无 Token 构成数据</div>
    <div v-else ref="chartEl" class="overview-token-comp__chart" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { EChartsCoreOption } from 'echarts/core';
import type { ModelUsageSummary } from '../../features/usage/types';
import { baseChartOption, usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';

const props = defineProps<{
  summary: ModelUsageSummary | null | undefined;
}>();

const chartEl = ref<HTMLElement | null>(null);

type TokenSlice = { name: string; value: number; color: string };

const slices = computed<TokenSlice[]>(() => {
  const s = props.summary;
  if (!s || s.total_tokens <= 0) return [];
  const palette = usageChartPalette();
  const inputVal = s.input_tokens ?? 0;
  const outputVal = s.output_tokens ?? 0;
  const otherVal = Math.max(0, s.total_tokens - inputVal - outputVal);
  const result: TokenSlice[] = [
    { name: '输入 Token', value: inputVal, color: palette.accent },
    { name: '输出 Token', value: outputVal, color: palette.series[2] ?? 'var(--color-info, #60a5fa)' },
  ];
  if (otherVal > 0) {
    result.push({ name: '其他', value: otherVal, color: palette.series[3] ?? 'var(--chart-color-skills, #a78bfa)' });
  }
  return result.filter((sl) => sl.value > 0);
});

function buildOption(): EChartsCoreOption {
  if (!slices.value.length) return {};
  const palette = usageChartPalette();
  return baseChartOption({
    tooltip: {
      trigger: 'item',
      valueFormatter: (v: number) => new Intl.NumberFormat('zh-CN').format(v),
    },
    legend: {
      orient: 'vertical',
      right: 0,
      top: 'middle',
      textStyle: { color: palette.text, fontSize: 11 },
      itemWidth: 10,
      itemHeight: 10,
      itemGap: 8,
    },
    series: [
      {
        type: 'pie',
        radius: ['48%', '72%'],
        center: ['32%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 3 },
        label: { show: false },
        data: slices.value.map((sl) => ({
          name: sl.name,
          value: sl.value,
          itemStyle: { color: sl.color },
        })),
      },
    ],
  });
}

useUsageChart(chartEl, buildOption, () => [slices.value]);
</script>
