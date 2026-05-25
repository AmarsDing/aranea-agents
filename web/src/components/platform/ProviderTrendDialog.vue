<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="provider-trend-dialog app-dialog-card app-dialog-card--lg">
      <q-card-section class="app-glass-dialog__head row items-start justify-between">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">模型历史趋势</div>
          <div v-if="row" class="app-glass-dialog__subtitle">
            {{ providerDisplayName }} / {{ modelDisplayName }}
          </div>
        </div>
        <q-btn flat round dense icon="close" v-close-popup aria-label="关闭" />
      </q-card-section>

      <q-separator />

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
              @update:model-value="emit('update:metric', $event)"
              dense
              no-caps
              unelevated
              toggle-color="primary"
              :options="metricOptions"
              class="app-metric-toggle"
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
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { graphic, type EChartsCoreOption } from "echarts/core";
import type { ModelUsageOverview } from "../../features/usage/types";
import type { PlatformResource } from "../../features/platform/types";
import { baseChartOption, usageChartPalette } from "../../features/usage/usageEcharts";
import { useUsageChart } from "../../features/usage/useUsageChart";
import {
  formatTrendLabel,
  trendMetricValue,
  trendMetricYAxisName,
  type UsageTrendMetric
} from "../../features/usage/usageTrendMetrics";

type ProviderConfig = {
  provider_display_name?: string;
  context_window_k?: number | string | null;
  max_output_tokens?: number | string | null;
  tokens_per_second?: number | string | null;
  model_hotness_score?: number | string | null;
  last_used_at?: string;
};

const props = defineProps<{
  modelValue: boolean;
  row: PlatformResource | null;
  metric: UsageTrendMetric;
  metricOptions: { label: string; value: UsageTrendMetric }[];
  overview: ModelUsageOverview | null;
  loading: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  "update:metric": [value: UsageTrendMetric];
}>();

const config = computed(() => (props.row ? getConfig(props.row) : {}));
const chartEl = ref<HTMLElement | null>(null);

const trends = computed(() => props.overview?.trends ?? []);
const metricCaption = computed(() => props.metricOptions.find((o) => o.value === props.metric)?.label ?? "");
const providerDisplayName = computed(() =>
  props.row ? config.value.provider_display_name || props.row.provider || props.row.key : ""
);
const modelDisplayName = computed(() => (props.row ? props.row.name || props.row.model || "未设置模型" : ""));

const latestUsedAt = computed(() => {
  const latest = trends.value.filter((point) => point.call_count > 0).at(-1);
  return latest?.date_key || config.value.last_used_at || "—";
});

const hotnessScore = computed(() => {
  const score = toNullableNumber(config.value.model_hotness_score);
  return score === null ? 0 : Math.max(0, Math.min(100, Math.round(score)));
});

const detailItems = computed(() => [
  { label: "成功率", value: formatPercent(props.overview?.range.success_rate) },
  { label: "平均延迟", value: formatLatency(props.overview?.range.avg_latency_ms) },
  { label: "TPS", value: formatTps(props.overview?.range.avg_tokens_per_second ?? config.value.tokens_per_second) },
  { label: "最近调用", value: latestUsedAt.value },
  { label: "上下文", value: formatContextWindow(config.value.context_window_k) },
  { label: "最大输出", value: formatCount(config.value.max_output_tokens) }
]);

function buildChartOption(): EChartsCoreOption {
  const palette = usageChartPalette();
  const accent = palette.series[2] ?? "#00e5ff";
  const points = trends.value;
  const labels = points.map((p) => formatTrendLabel(p.date_key, false));
  const isCost = props.metric === "cost";
  const data = points.map((p) => trendMetricValue(p, props.metric));

  return baseChartOption({
    animationDuration: 480,
    grid: { left: 8, right: 12, top: 28, bottom: 8, containLabel: true },
    tooltip: {
      trigger: "axis",
      backgroundColor: "rgba(15, 23, 42, 0.92)",
      borderColor: "rgba(0, 229, 255, 0.25)",
      textStyle: { color: "#e2e8f0", fontSize: 12 },
      valueFormatter: (v: number) => (isCost ? `$${Number(v).toFixed(4)}` : formatCount(v))
    },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: labels,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: palette.text, fontSize: 11, margin: 10 }
    },
    yAxis: {
      type: "value",
      name: trendMetricYAxisName(props.metric),
      nameTextStyle: { color: palette.text, fontSize: 11, padding: [0, 0, 0, 4] },
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: palette.text,
        fontSize: 11,
        formatter: (v: number) => (isCost ? `$${v.toFixed(2)}` : formatCompact(v))
      },
      splitLine: { lineStyle: { color: palette.border, type: "dashed" } }
    },
    series: [
      {
        name: metricCaption.value,
        type: "line",
        smooth: 0.35,
        symbol: "circle",
        symbolSize: 6,
        showSymbol: points.length <= 14,
        lineStyle: { width: 2.5, color: accent, shadowColor: "rgba(0, 229, 255, 0.35)", shadowBlur: 8 },
        itemStyle: { color: accent, borderColor: "#fff", borderWidth: 1 },
        areaStyle: {
          color: new graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: "rgba(0, 229, 255, 0.28)" },
            { offset: 0.65, color: "rgba(0, 229, 255, 0.06)" },
            { offset: 1, color: "rgba(0, 229, 255, 0)" }
          ])
        },
        data
      }
    ]
  });
}

useUsageChart(chartEl, buildChartOption, () => [trends.value, props.metric, props.loading]);

function getConfig(row: PlatformResource): ProviderConfig {
  if (!row.config_json) return {};
  try {
    const value = JSON.parse(row.config_json) as ProviderConfig;
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

function formatTps(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  const rounded = numberValue >= 100 ? Math.round(numberValue) : Math.round(numberValue * 10) / 10;
  return `${rounded} tok/s`;
}

function formatContextWindow(value: ProviderConfig["context_window_k"]) {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? "—" : `${numberValue}K`;
}

function formatCount(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(numberValue);
}

function formatCompact(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 10_000) return `${(value / 1_000).toFixed(0)}k`;
  return String(value);
}

function formatMicroUsd(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `$${(numberValue / 1_000_000).toFixed(4)}`;
}

function formatPercent(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `${Math.round(numberValue * 100)}%`;
}

function formatLatency(value: unknown) {
  const numberValue = toNullableNumber(value);
  if (numberValue === null) return "—";
  return `${Math.round(numberValue)} ms`;
}

function toNullableNumber(value: unknown) {
  if (value === "" || value === null || value === undefined) return null;
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}
</script>
