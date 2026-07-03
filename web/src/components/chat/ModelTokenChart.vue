<template>
  <div class="model-token-chart">
    <div v-if="!data.series.length && !loading" class="model-token-chart__empty">
      {{ t('chat.noModelTokenData') }}
    </div>
    <div v-else-if="loading" class="model-token-chart__loading">
      <q-spinner-dots size="20px" color="accent" />
    </div>
    <template v-else>
      <div ref="chartEl" class="model-token-chart__canvas" />
      <div class="model-token-chart__legend">
        <div v-for="s in data.series" :key="s.key" class="model-token-chart__legend-item">
          <span class="model-token-chart__legend-dot" :style="{ background: colorFor(s.key) }" />
          <span class="model-token-chart__legend-label ellipsis">{{ s.label }}</span>
          <span class="model-token-chart__legend-value">{{ formatTokens(s.totalAll) }}</span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EChartsCoreOption } from 'echarts/core';
import { baseChartOption, usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';
import type { SessionModelTokens, ModelTokenSeries } from '../../features/chat/useSessionModelTokens';

const props = defineProps<{
  data: SessionModelTokens;
  loading?: boolean;
}>();

const { t } = useI18n();
const chartEl = ref<HTMLElement | null>(null);

const palette = computed(() => usageChartPalette());

const colorFor = (key: string): string => {
  const colors = palette.value.series;
  let hash = 0;
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) >>> 0;
  return colors[hash % colors.length];
};

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

function buildOption(): EChartsCoreOption {
  const p = palette.value;
  const turns = props.data.turns;
  // Build per-turn total-token series for each model. Use null for turns where the
  // model didn't run so each line only spans its actual turns.
  const series = props.data.series.map((s: ModelTokenSeries) => {
    const pointMap = new Map<number, number>();
    for (const pt of s.points) pointMap.set(pt.turn, pt.totalTokens);
    return {
      name: s.label,
      type: 'line' as const,
      smooth: true,
      showSymbol: true,
      symbolSize: 5,
      lineStyle: { width: 2 },
      itemStyle: { color: colorFor(s.key) },
      data: turns.map((tn) => (pointMap.has(tn) ? pointMap.get(tn) : null)),
    };
  });

  return baseChartOption({
    tooltip: {
      trigger: 'axis',
      valueFormatter: (v: unknown) => (v == null ? '-' : formatTokens(Number(v))),
    },
    legend: { show: false },
    grid: { left: 40, right: 12, top: 12, bottom: 24 },
    xAxis: {
      type: 'category',
      data: turns.map((tn) => `#${tn}`),
      axisLabel: { color: p.text, fontSize: 10 },
      axisLine: { lineStyle: { color: p.border } },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        color: p.text,
        fontSize: 10,
        formatter: (v: number) => formatTokens(v),
      },
      splitLine: { lineStyle: { color: p.border } },
    },
    series,
  });
}

useUsageChart(chartEl, buildOption, () => [props.data]);
</script>

<style scoped lang="sass">
.model-token-chart
  width: 320px
  max-width: 100%

.model-token-chart__canvas
  width: 100%
  height: 140px

.model-token-chart__legend
  display: flex
  flex-direction: column
  gap: 2px
  padding: 6px 4px 2px
  max-height: 96px
  overflow-y: auto

.model-token-chart__legend-item
  display: flex
  align-items: center
  gap: 6px
  font-size: 11px
  color: var(--color-text-secondary)

.model-token-chart__legend-dot
  width: 8px
  height: 8px
  border-radius: 50%
  flex-shrink: 0

.model-token-chart__legend-label
  flex: 1
  min-width: 0

.model-token-chart__legend-value
  color: var(--color-text-tertiary)
  font-variant-numeric: tabular-nums

.model-token-chart__empty,
.model-token-chart__loading
  width: 320px
  height: 180px
  display: flex
  align-items: center
  justify-content: center
  color: var(--color-text-tertiary)
  font-size: 12px
</style>
