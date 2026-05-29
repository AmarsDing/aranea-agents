<template>
  <div class="overview-metrics-tier">
    <div v-if="budgetCard" class="overview-metric-card overview-metric-card--budget app-metrics-grid__item">
      <q-card-section class="overview-metric-card__body">
        <div class="overview-metric-card__icon-row">
          <q-icon name="account_balance_wallet" class="overview-metric-card__icon" />
          <span class="overview-metric-card__label">{{ budgetCard.label }}</span>
        </div>
        <div class="overview-metric-card__value overview-metric-card__value--lg">{{ budgetCard.value }}</div>
        <div class="overview-metric-card__caption" :class="budgetCard.toneClass">{{ budgetCard.caption }}</div>
        <div v-if="budgetCard.toneClass" class="overview-metric-card__bar">
          <div class="overview-metric-card__bar-fill" :class="budgetBarClass" :style="{ width: budgetBarWidth }" />
        </div>
      </q-card-section>
    </div>

    <div class="overview-metrics-primary">
      <q-card v-for="item in primaryCards" :key="item.label" flat class="overview-metric-card overview-metric-card--primary app-metrics-grid__item">
        <q-card-section class="overview-metric-card__body">
          <div class="overview-metric-card__label">{{ item.label }}</div>
          <div class="overview-metric-card__value">{{ item.value }}</div>
          <div class="overview-metric-card__caption" :class="item.toneClass">{{ item.caption }}</div>
        </q-card-section>
      </q-card>
    </div>

    <div class="overview-metrics-secondary">
      <q-card v-for="item in secondaryCards" :key="item.label" flat class="overview-metric-card overview-metric-card--secondary app-metrics-grid__item">
        <q-card-section class="overview-metric-card__body">
          <div class="overview-metric-card__label">{{ item.label }}</div>
          <div class="overview-metric-card__value overview-metric-card__value--sm">{{ item.value }}</div>
          <div class="overview-metric-card__caption" :class="item.toneClass">{{ item.caption }}</div>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ModelUsageOverview } from "../../features/usage/types";
import { formatUsdFromMicro } from "../../features/usage/moneyFormat";

const props = defineProps<{
  overview: ModelUsageOverview | null;
}>();

const budgetCard = computed(() => {
  const dash = props.overview?.quota_dashboard;
  if (!dash || dash.configured_count <= 0) return null;
  const util = dash.max_utilization_ratio ?? 0;
  return {
    label: "月预算使用率",
    value: `${Math.round(util * 100)}%`,
    caption: `${dash.configured_count} 个 Agent · 已用 $${((dash.total_spent_micro_usd ?? 0) / 1_000_000).toFixed(2)} / $${((dash.total_cap_micro_usd ?? 0) / 1_000_000).toFixed(2)}`,
    toneClass: util >= 0.9 ? "overview-metric-card__caption--danger" : util >= 0.7 ? "overview-metric-card__caption--warn" : "",
    util
  };
});

const budgetBarClass = computed(() => {
  const util = budgetCard.value?.util ?? 0;
  if (util >= 0.9) return "overview-metric-card__bar-fill--danger";
  if (util >= 0.7) return "overview-metric-card__bar-fill--warn";
  return "overview-metric-card__bar-fill--ok";
});

const budgetBarWidth = computed(() => {
  const util = budgetCard.value?.util ?? 0;
  return `${Math.min(util * 100, 100)}%`;
});

const primaryCards = computed(() => {
  const today = props.overview?.today;
  const month = props.overview?.month;
  return [
    {
      label: "今日调用",
      value: formatCount(today?.call_count),
      caption: `较昨日 ${formatDelta(today?.call_count, props.overview?.yesterday.call_count)}`,
      toneClass: deltaToneClass(today?.call_count, props.overview?.yesterday.call_count)
    },
    {
      label: "今日费用",
      value: formatMoney(today?.total_cost_micro_usd),
      caption: `较昨日 ${formatDelta(today?.total_cost_micro_usd, props.overview?.yesterday.total_cost_micro_usd)}`,
      toneClass: deltaToneClass(today?.total_cost_micro_usd, props.overview?.yesterday.total_cost_micro_usd)
    },
    {
      label: "今日 Token",
      value: formatCount(today?.total_tokens),
      caption: `输入 ${formatCount(today?.input_tokens)} / 输出 ${formatCount(today?.output_tokens)}`,
      toneClass: ""
    },
    {
      label: "本月费用",
      value: formatMoney(month?.total_cost_micro_usd),
      caption: `本月调用 ${formatCount(month?.call_count)} 次`,
      toneClass: ""
    }
  ];
});

const secondaryCards = computed(() => {
  const today = props.overview?.today;
  return [
    {
      label: "平均延迟",
      value: formatLatency(today?.avg_latency_ms),
      caption: `成功率 ${formatPercent(today?.success_rate)}`,
      toneClass: ""
    },
    {
      label: "平均 TPS",
      value: formatTps(today?.avg_tokens_per_second),
      caption: `区间 TPS ${formatTps(props.overview?.range.avg_tokens_per_second)}`,
      toneClass: ""
    }
  ];
});

function formatCount(value?: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value ?? 0);
}

function formatMoney(value?: number) {
  return formatUsdFromMicro(value);
}

function formatLatency(value?: number) {
  return `${Math.round(value ?? 0)} ms`;
}

function formatTps(value?: number) {
  return `${(value ?? 0).toFixed(1)} tok/s`;
}

function formatPercent(value?: number) {
  return `${Math.round((value ?? 0) * 100)}%`;
}

function formatDelta(current?: number, previous?: number) {
  const prev = previous ?? 0;
  const next = current ?? 0;
  if (prev === 0) return next > 0 ? "+100%" : "0%";
  const delta = ((next - prev) / prev) * 100;
  return `${delta >= 0 ? "+" : ""}${delta.toFixed(1)}%`;
}

function deltaToneClass(current?: number, previous?: number) {
  return (current ?? 0) >= (previous ?? 0) ? "overview-metric-card__caption--up" : "overview-metric-card__caption--down";
}
</script>
