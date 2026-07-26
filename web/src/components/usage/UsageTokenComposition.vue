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
    { name: '输入 Token', value: inputVal, color: palette.series[0] },
    { name: '输出 Token', value: outputVal, color: palette.series[1] },
  ];
  if (otherVal > 0) {
    result.push({ name: '其他', value: otherVal, color: palette.series[5] });
  }
  return result.filter((sl) => sl.value > 0);
});

function buildOption(): EChartsCoreOption {
  if (!slices.value.length) return {};
  const palette = usageChartPalette();
  const total = slices.value.reduce((sum, sl) => sum + sl.value, 0);
  const pctOf = (name: string) => {
    const sl = slices.value.find((it) => it.name === name);
    return sl && total > 0 ? ((sl.value / total) * 100).toFixed(1) : '0.0';
  };
  return baseChartOption({
    tooltip: {
      trigger: 'item',
      valueFormatter: (v: number) => new Intl.NumberFormat('zh-CN').format(v),
    },
    legend: {
      orient: 'vertical',
      right: 0,
      top: 'middle',
      icon: 'circle',
      itemWidth: 8,
      itemHeight: 8,
      itemGap: 10,
      textStyle: { color: palette.text, fontSize: 11 },
      formatter: (name: string) => `${name}  ${pctOf(name)}%`,
    },
    series: [
      {
        type: 'pie',
        radius: ['52%', '74%'],
        center: ['30%', '50%'],
        itemStyle: {
          borderRadius: 4,
          borderColor: palette.surface,
          borderWidth: 2,
        },
        label: { show: false },
        labelLine: { show: false },
        emphasis: { scale: true, scaleSize: 4 },
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
