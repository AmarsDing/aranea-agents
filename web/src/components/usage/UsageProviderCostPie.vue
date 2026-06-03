<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">Provider 费用占比</div>
      <div class="text-caption overview-section-caption">{{ providerCaption }}</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!providerSlices.length" class="overview-empty">暂无 Provider 费用数据</div>
      <div v-else ref="providerChartEl" class="usage-breakdown-chart" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { EChartsCoreOption } from 'echarts/core';
import type { ModelUsageBreakdownRow } from '../../features/usage/types';
import { usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';
import { formatUsdCompact } from '../../features/usage/moneyFormat';
import {
  USAGE_BREAKDOWN_TOP_N,
  buildProviderCostSlicesFromTopModels,
  type UsageBreakdownSlice,
} from '../../features/usage/usageBreakdownSlices';

const props = defineProps<{
  topModels: ModelUsageBreakdownRow[];
}>();

const providerSlices = computed(() => buildProviderCostSlicesFromTopModels(props.topModels));
const providerChartEl = ref<HTMLElement | null>(null);

const providerCaption = computed(() => `基于概览 Top 模型样本聚合（最多 ${USAGE_BREAKDOWN_TOP_N} 个 Provider）`);

function pieOption(slices: UsageBreakdownSlice[]): EChartsCoreOption {
  const palette = usageChartPalette();
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    tooltip: { trigger: 'item', valueFormatter: (v: number) => formatUsdCompact(v * 1_000_000) },
    legend: { orient: 'vertical', right: 0, top: 'middle', textStyle: { color: palette.text } },
    series: [
      {
        type: 'pie',
        radius: ['42%', '68%'],
        center: ['38%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 4 },
        label: { color: palette.text, formatter: '{b}\n{d}%' },
        data: slices.map((s, i) => ({
          name: s.name,
          value: s.value,
          itemStyle: { color: palette.series[i % palette.series.length] },
        })),
      },
    ],
  };
}

useUsageChart(
  providerChartEl,
  () => pieOption(providerSlices.value),
  () => [providerSlices.value],
);
</script>
