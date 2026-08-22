<template>
  <div class="evaluation-trend-chart">
    <div class="row items-center justify-between q-mb-sm">
      <div class="text-caption text-grey-7">{{ t('evaluationPage.trendChartTitle') }}</div>
      <q-select
        :model-value="metric"
        dense
        outlined
        emit-value
        map-options
        class="app-field-sm"
        :options="metricOptions"
        @update:model-value="$emit('update:metric', String($event ?? 'exact_match_score'))"
      />
    </div>
    <div ref="chartEl" class="evaluation-trend-chart__canvas" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EChartsCoreOption } from 'echarts/core';
import type { EvalTrendPoint } from '../../features/evaluation/types';
import { usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';

const props = defineProps<{
  points: EvalTrendPoint[];
  metric: string;
}>();

defineEmits<{
  'update:metric': [value: string];
}>();

const { t } = useI18n();
const chartEl = ref<HTMLElement | null>(null);

const metricOptions = computed(() => [
  { label: 'Exact', value: 'exact_match_score' },
  { label: 'Contains', value: 'contains_match_score' },
  { label: 'LLM Judge', value: 'llm_judge_score' },
  { label: 'Tool', value: 'tool_call_accuracy' },
]);

function scoreOf(p: EvalTrendPoint): number {
  const key = props.metric as keyof EvalTrendPoint;
  const v = p[key];
  return typeof v === 'number' ? v : 0;
}

function option(): EChartsCoreOption {
  const p = usageChartPalette();
  const rows = [...props.points].slice().reverse();
  return {
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis' },
    grid: { left: 36, right: 12, top: 16, bottom: 24 },
    xAxis: {
      type: 'category',
      data: rows.map((r) => r.created_at.slice(5, 16)),
      axisLabel: { color: p.text, fontSize: 10 },
    },
    yAxis: {
      type: 'value',
      min: 0,
      max: 1,
      axisLabel: { color: p.text, fontSize: 10 },
    },
    series: [
      {
        type: 'line',
        smooth: true,
        showSymbol: rows.length < 16,
        data: rows.map(scoreOf),
        lineStyle: { color: p.accent, width: 2 },
        itemStyle: { color: p.accent },
      },
    ],
  };
}

useUsageChart(chartEl, option, () => [props.points, props.metric]);
</script>

<style scoped lang="sass">
.evaluation-trend-chart__canvas
  height: 180px
  width: 100%
</style>
