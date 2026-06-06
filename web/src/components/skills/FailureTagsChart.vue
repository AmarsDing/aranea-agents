<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">失败标签分布</div>
      <div class="text-caption overview-section-caption">基于当前筛选条件下的经验报告统计</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!slices.length" class="overview-empty">暂无失败标签数据</div>
      <div v-else ref="chartEl" class="failure-tags-chart" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { EChartsCoreOption } from 'echarts/core';
import { usageChartPalette, ensureUsageEcharts, echarts } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';

const props = defineProps<{
  failureTags: Record<string, number>;
}>();

const chartEl = ref<HTMLElement | null>(null);

const slices = computed(() =>
  Object.entries(props.failureTags)
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value),
);

function pieOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  return {
    textStyle: { color: palette.text, fontFamily: 'inherit' },
    tooltip: { trigger: 'item', valueFormatter: (v: number) => `${v} 次` },
    legend: { orient: 'vertical', right: 0, top: 'middle', textStyle: { color: palette.text } },
    series: [
      {
        type: 'pie',
        radius: ['42%', '68%'],
        center: ['38%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: { borderRadius: 4 },
        label: { color: palette.text, formatter: '{b}\n{d}%' },
        data: slices.value.map((s, i) => ({
          name: s.name,
          value: s.value,
          itemStyle: { color: palette.series[i % palette.series.length] },
        })),
      },
    ],
  };
}

useUsageChart(
  chartEl,
  pieOption,
  () => [slices.value],
);
</script>

<style scoped lang="sass">
.failure-tags-chart
  width: 100%
  height: 260px
</style>
