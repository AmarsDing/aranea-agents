<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="provider-trend-dialog app-dialog-card app-dialog-card--lg app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">模型历史趋势</div>
          <div v-if="row" class="app-glass-dialog__subtitle">{{ providerDisplayName }} / {{ modelDisplayName }}</div>
        </div>
        <q-btn v-close-popup flat round dense icon="close" aria-label="关闭" />
      </q-card-section>

      <q-separator />

      <div class="app-glass-dialog__scroll">
        <q-card-section v-if="row" class="app-glass-dialog__body provider-trend-dialog__body">
          <q-inner-loading :showing="loading">
            <q-spinner color="primary" size="32px" />
          </q-inner-loading>

          <div class="app-trend-kpi-grid">
            <div class="app-trend-kpi-card">
              <span class="app-trend-kpi-label">热度</span>
              <span class="app-trend-kpi-value">{{ hotnessScore }}</span>
              <div class="app-trend-kpi-bar" role="presentation">
                <div class="app-trend-kpi-bar__fill" :style="{ width: `${hotnessScore}%` }" />
              </div>
            </div>
            <div class="app-trend-kpi-card">
              <span class="app-trend-kpi-label">30 天调用</span>
              <span class="app-trend-kpi-value">{{ formatCount(overview?.range.call_count) }}</span>
            </div>
            <div class="app-trend-kpi-card">
              <span class="app-trend-kpi-label">30 天 Token</span>
              <span class="app-trend-kpi-value">{{ formatCount(overview?.range.total_tokens) }}</span>
            </div>
            <div class="app-trend-kpi-card">
              <span class="app-trend-kpi-label">30 天费用</span>
              <span class="app-trend-kpi-value">{{ formatMicroUsd(overview?.range.total_cost_micro_usd) }}</span>
            </div>
          </div>

          <div class="app-trend-chart-panel">
            <div class="app-trend-chart-toolbar row items-center justify-between q-col-gutter-sm">
              <div>
                <div class="app-trend-chart-title">用量趋势</div>
                <div class="app-trend-chart-caption">近 30 天 · {{ metricCaption }}</div>
              </div>
              <q-btn-toggle
                :model-value="metric"
                dense
                no-caps
                unelevated
                toggle-color="primary"
                :options="metricOptions"
                class="app-metric-toggle"
                @update:model-value="emit('update:metric', $event)"
              />
            </div>
            <div v-if="!trends.length && !loading" class="app-trend-chart-empty">暂无历史趋势数据</div>
            <div v-show="trends.length || loading" ref="chartEl" class="app-trend-chart" />
          </div>

          <div class="app-trend-detail-grid">
            <div v-for="item in detailItems" :key="item.label" class="app-trend-detail-item">
              <span class="app-trend-detail-label">{{ item.label }}</span>
              <span class="app-trend-detail-value">{{ item.value }}</span>
            </div>
          </div>
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { graphic, type EChartsCoreOption } from 'echarts/core';
import type { ModelUsageOverview } from '../../features/usage/types';
import type { PlatformResource, ProviderConfig } from '../../features/platform/types';
import { toNullableNumber } from '../../features/platform/providerUtils';
import { baseChartOption, readCssVar, usageChartPalette } from '../../features/usage/usageEcharts';
import { useUsageChart } from '../../features/usage/useUsageChart';
import {
  formatTrendLabel,
  trendMetricValue,
  trendMetricYAxisName,
  type UsageTrendMetric,
} from '../../features/usage/usageTrendMetrics';
import {
  formatTps,
  formatContextWindow,
  formatCount,
  formatMicroUsd,
  formatPercent,
  formatLatency,
  formatCompact,
  getProviderConfig,
} from './providerModelUi';

const props = defineProps<{
  modelValue: boolean;
  row: PlatformResource | null;
  metric: UsageTrendMetric;
  metricOptions: { label: string; value: UsageTrendMetric }[];
  overview: ModelUsageOverview | null;
  loading: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'update:metric': [value: UsageTrendMetric];
}>();

const config = computed(() => (props.row ? getProviderConfig(props.row) : {}));
const chartEl = ref<HTMLElement | null>(null);

const trends = computed(() => props.overview?.trends ?? []);
const metricCaption = computed(() => props.metricOptions.find((o) => o.value === props.metric)?.label ?? '');
const providerDisplayName = computed(() =>
  props.row ? config.value.provider_display_name || props.row.provider || props.row.key : '',
);
const modelDisplayName = computed(() => (props.row ? props.row.name || props.row.model || '未设置模型' : ''));

const latestUsedAt = computed(() => {
  const latest = trends.value.filter((point) => point.call_count > 0).at(-1);
  return latest?.date_key || config.value.last_used_at || '—';
});

const hotnessScore = computed(() => {
  const score = toNullableNumber(config.value.model_hotness_score);
  return score === null ? 0 : Math.max(0, Math.min(100, Math.round(score)));
});

const detailItems = computed(() => [
  { label: '成功率', value: formatPercent(props.overview?.range.success_rate) },
  { label: '平均延迟', value: formatLatency(props.overview?.range.avg_latency_ms) },
  { label: 'TPS', value: formatTps(props.overview?.range.avg_tokens_per_second ?? config.value.tokens_per_second) },
  { label: '最近调用', value: latestUsedAt.value },
  { label: '上下文', value: formatContextWindow(config.value.context_window_k) },
  { label: '最大输出', value: formatCount(config.value.max_output_tokens) },
]);

function buildChartOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  const accent = palette.series[2] ?? readCssVar('--color-accent', '#E9A23B');
  const glassElevated = readCssVar('--glass-elevated', 'rgba(15, 23, 42, 0.92)');
  const textPrimary = readCssVar('--color-text-primary', '#e2e8f0');
  const onAccent = readCssVar('--color-on-accent', '#fff');
  const points = trends.value;
  const labels = points.map((p) => formatTrendLabel(p.date_key, false));
  const isCost = props.metric === 'cost';
  const data = points.map((p) => trendMetricValue(p, props.metric));

  return baseChartOption({
    animationDuration: 480,
    grid: { left: 8, right: 12, top: 28, bottom: 8, containLabel: true },
    tooltip: {
      trigger: 'axis',
      backgroundColor: glassElevated,
      borderColor: colorMix(accent, 0.25),
      textStyle: { color: textPrimary, fontSize: 12 },
      valueFormatter: (v: number) => (isCost ? `$${Number(v).toFixed(4)}` : formatCount(v)),
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: labels,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: palette.text, fontSize: 11, margin: 10 },
    },
    yAxis: {
      type: 'value',
      name: trendMetricYAxisName(props.metric),
      nameTextStyle: { color: palette.text, fontSize: 11, padding: [0, 0, 0, 4] },
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: palette.text,
        fontSize: 11,
        formatter: (v: number) => (isCost ? `$${v.toFixed(2)}` : formatCompact(v)),
      },
      splitLine: { lineStyle: { color: palette.border, type: 'dashed' } },
    },
    series: [
      {
        name: metricCaption.value,
        type: 'line',
        smooth: 0.35,
        symbol: 'circle',
        symbolSize: 6,
        showSymbol: points.length <= 14,
        lineStyle: { width: 2.5, color: accent, shadowColor: colorMix(accent, 0.35), shadowBlur: 8 },
        itemStyle: { color: accent, borderColor: onAccent, borderWidth: 1 },
        areaStyle: {
          color: new graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: colorMix(accent, 0.28) },
            { offset: 0.65, color: colorMix(accent, 0.06) },
            { offset: 1, color: colorMix(accent, 0) },
          ]),
        },
        data,
      },
    ],
  });
}

function colorMix(color: string, alpha: number): string {
  if (color.startsWith('rgba')) {
    return color.replace(/[\d.]+\)$/, `${alpha})`);
  }
  if (color.startsWith('rgb')) {
    return color.replace('rgb', 'rgba').replace(')', `, ${alpha})`);
  }
  if (color.startsWith('#')) {
    const r = parseInt(color.slice(1, 3), 16);
    const g = parseInt(color.slice(3, 5), 16);
    const b = parseInt(color.slice(5, 7), 16);
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
  }
  return `rgba(233, 162, 59, ${alpha})`;
}

useUsageChart(chartEl, buildChartOption, () => [trends.value, props.metric, props.loading]);
</script>
