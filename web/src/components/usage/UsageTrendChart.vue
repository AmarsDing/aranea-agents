<template>
  <q-card flat class="overview-panel overview-trend-panel">
    <q-card-section class="row items-center justify-between q-col-gutter-sm">
      <div>
        <div class="text-h6 overview-section-title">消耗趋势</div>
        <div class="text-caption overview-section-caption">{{ hourly ? '按小时' : '按天' }} · {{ metricCaption }}</div>
      </div>
      <div class="row q-gutter-sm items-center">
        <q-btn-toggle
          v-model="metric"
          dense
          no-caps
          unelevated
          toggle-color="primary"
          :options="metricOptions"
          class="app-metric-toggle app-metric-toggle--sm"
        />
        <q-chip dense outline class="overview-chip">{{ points.length }} {{ hourly ? '小时' : '天' }}</q-chip>
      </div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!points.length" class="overview-empty">暂无趋势数据</div>
      <div v-else ref="chartEl" class="app-trend-chart app-trend-chart--lg" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { EChartsCoreOption } from 'echarts/core';
import type { ModelUsageTrendPoint } from '../../features/usage/types';
import { baseChartOption, usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';
import {
  USAGE_TREND_METRIC_OPTIONS,
  formatTrendLabel,
  successRateStackFromPoint,
  trendMetricValue,
  trendMetricYAxisName,
  type UsageTrendMetric,
} from '../../features/usage/usageTrendMetrics';

const props = defineProps<{
  points: ModelUsageTrendPoint[];
  hourly?: boolean;
}>();

const metric = ref<UsageTrendMetric>('tokens');
const chartEl = ref<HTMLElement | null>(null);
const metricOptions = USAGE_TREND_METRIC_OPTIONS;

const metricCaption = computed(() => metricOptions.find((o) => o.value === metric.value)?.label ?? '');

function buildOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  const labels = props.points.map((p) => formatTrendLabel(p.date_key, !!props.hourly));

  if (metric.value === 'success_rate') {
    const stacks = props.points.map(successRateStackFromPoint);
    return baseChartOption({
      tooltip: { trigger: 'axis', valueFormatter: (v: number) => `${v}%` },
      legend: { top: 4, textStyle: { color: palette.text } },
      yAxis: { type: 'value', max: 100, name: '%', axisLabel: { formatter: '{value}%' } },
      xAxis: { type: 'category', data: labels, axisLabel: { color: palette.text } },
      series: [
        {
          name: '成功',
          type: 'bar',
          stack: 'rate',
          itemStyle: { color: palette.positive },
          data: stacks.map((s) => s.successPct),
        },
        {
          name: '失败/取消',
          type: 'bar',
          stack: 'rate',
          itemStyle: { color: palette.negative },
          data: stacks.map((s) => s.failurePct),
        },
      ],
    });
  }

  const isCost = metric.value === 'cost';
  const data = props.points.map((p) => trendMetricValue(p, metric.value));

  return baseChartOption({
    tooltip: {
      trigger: 'axis',
      valueFormatter: (v: number) => (isCost ? `$${Number(v).toFixed(4)}` : String(v)),
    },
    yAxis: { type: 'value', name: trendMetricYAxisName(metric.value) },
    xAxis: { type: 'category', data: labels, axisLabel: { color: palette.text } },
    series: [
      {
        name: metricCaption.value,
        type: isCost ? 'line' : 'bar',
        smooth: isCost,
        itemStyle: { color: palette.accent },
        areaStyle: isCost ? { opacity: 0.12, color: palette.accent } : undefined,
        data,
      },
    ],
  });
}

useUsageChart(chartEl, buildOption, () => [props.points, metric.value, props.hourly]);
</script>
