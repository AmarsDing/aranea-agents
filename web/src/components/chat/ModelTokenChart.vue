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
      <div class="model-token-chart__hover">
        <template v-if="hovered">
          <div class="model-token-chart__hover-head">
            <span class="model-token-chart__hover-title">#{{ hovered.point.turn }} · {{ hovered.seriesLabel }}</span>
            <span class="model-token-chart__hover-total">{{ formatTokens(hovered.point.totalTokens) }}</span>
          </div>
          <ContextBudgetBreakdown v-if="hovered.point.budget" :budget="hovered.point.budget" />
          <div v-else class="model-token-chart__hover-simple">
            <div class="model-token-chart__hover-row">
              <span>{{ t('chat.modelTokenTipTotal') }}</span>
              <span class="model-token-chart__hover-value">{{ formatTokens(hovered.point.totalTokens) }}</span>
            </div>
            <div class="model-token-chart__hover-row">
              <span>{{ t('chat.modelTokenTipPrompt') }}</span>
              <span class="model-token-chart__hover-value">{{ formatTokens(hovered.point.inputTokens) }}</span>
            </div>
          </div>
        </template>
        <div v-else class="model-token-chart__hint">{{ t('chat.modelTokenHoverHint') }}</div>
      </div>
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
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EChartsCoreOption } from 'echarts/core';
import { baseChartOption, usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';
import type { SessionModelTokens, ModelTokenSeries, ModelTokenPoint } from '../../features/chat/useSessionModelTokens';
import ContextBudgetBreakdown from './ContextBudgetBreakdown.vue';

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
  // model didn't run so each line only spans its actual turns. Each datum carries
  // the full point so the tooltip can show prompt tokens alongside the total.
  const series = props.data.series.map((s: ModelTokenSeries) => {
    const pointMap = new Map<number, ModelTokenPoint>();
    for (const pt of s.points) pointMap.set(pt.turn, pt);
    return {
      name: s.label,
      type: 'line' as const,
      smooth: true,
      showSymbol: true,
      symbolSize: 5,
      lineStyle: { width: 2 },
      itemStyle: { color: colorFor(s.key) },
      data: turns.map((tn) => {
        const pt = pointMap.get(tn);
        return pt ? { value: pt.totalTokens, point: pt } : null;
      }),
    };
  });

  return baseChartOption({
    // Native tooltip disabled — per-turn prompt breakdown is rendered as a Vue
    // panel below the chart (see hover events bound on the chart instance).
    tooltip: { show: false },
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

const { chartRef } = useUsageChart(chartEl, buildOption, () => [props.data]);

// ── Per-point hover: show the turn's prompt breakdown in a Vue panel ─────
const hovered = ref<{ seriesLabel: string; point: ModelTokenPoint } | null>(null);

watch(
  chartRef,
  (chart, _prev, onCleanup) => {
    if (!chart) return;
    const onOver = (param: unknown) => {
      const p = param as { componentType?: string; seriesName?: string; data?: { point?: ModelTokenPoint } };
      if (p.componentType !== 'series' || !p.data?.point) return;
      hovered.value = { seriesLabel: p.seriesName ?? '', point: p.data.point };
    };
    const onOut = () => {
      hovered.value = null;
    };
    chart.on('mouseover', onOver);
    chart.on('mouseout', onOut);
    chart.getZr().on('globalout', onOut);
    onCleanup(() => {
      chart.off('mouseover', onOver);
      chart.off('mouseout', onOut);
      chart.getZr().off('globalout', onOut);
    });
  },
  { immediate: true },
);
</script>

<style scoped lang="sass">
.model-token-chart
  width: 320px
  max-width: 100%

.model-token-chart__canvas
  width: 100%
  height: 140px

.model-token-chart__hover
  margin-top: 4px
  min-height: 20px

.model-token-chart__hint
  font-size: 10px
  color: var(--color-text-tertiary)
  padding: 2px 4px

.model-token-chart__hover-head
  display: flex
  align-items: center
  justify-content: space-between
  font-size: 11px
  color: var(--color-text-secondary)
  margin-bottom: 4px

.model-token-chart__hover-title
  font-weight: 500
  color: var(--color-text-primary)

.model-token-chart__hover-total
  font-variant-numeric: tabular-nums
  color: var(--color-text-tertiary)

.model-token-chart__hover-simple
  display: flex
  flex-direction: column
  gap: 3px

.model-token-chart__hover-row
  display: flex
  align-items: center
  justify-content: space-between
  font-size: 11px
  color: var(--color-text-secondary)

.model-token-chart__hover-value
  color: var(--color-text-primary)
  font-variant-numeric: tabular-nums

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
