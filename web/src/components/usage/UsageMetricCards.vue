<template>
  <div class="overview-stats-row">
    <div v-for="card in cards" :key="card.label" class="overview-stat-card">
      <div class="overview-stat-card__header">
        <div class="overview-stat-card__icon-wrap" :class="card.iconClass">
          <q-icon :name="card.icon" size="18px" />
        </div>
        <div class="overview-stat-card__label">{{ card.label }}</div>
      </div>
      <div class="overview-stat-card__body">
        <span class="overview-stat-card__value">{{ card.value }}</span>
        <span v-if="card.delta" class="overview-stat-card__delta" :class="card.deltaClass">{{ card.delta }}</span>
      </div>
      <div class="overview-stat-card__footer">
        <template v-if="card.caption">{{ card.caption }}</template>
        <div v-if="card.bar" class="overview-stat-card__bar">
          <div class="overview-stat-card__bar-fill" :class="card.bar.fillClass" :style="{ width: card.bar.width }" />
        </div>
        <div v-if="card.forecast" class="overview-stat-card__forecast" :class="card.forecastClass">
          <q-icon name="trending_up" size="12px" />
          {{ card.forecast }}
        </div>
      </div>
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

interface StatCard {
  label: string;
  icon: string;
  iconClass?: string;
  value: string;
  delta?: string;
  deltaClass?: string;
  caption?: string;
  bar?: { width: string; fillClass: string };
  forecast?: string;
  forecastClass?: string;
}

const cards = computed<StatCard[]>(() => {
  const today = props.overview?.today;
  const yesterday = props.overview?.yesterday;
  const month = props.overview?.month;
  const dash = props.overview?.quota_dashboard;

  const result: StatCard[] = [
    {
      label: "今日调用",
      icon: "phone_in_talk",
      value: fmtCount(today?.call_count),
      delta: fmtDelta(today?.call_count, yesterday?.call_count),
      deltaClass: (today?.call_count ?? 0) >= (yesterday?.call_count ?? 0) ? "overview-stat-card__delta--up" : "overview-stat-card__delta--down",
      caption: `昨日 ${fmtCount(yesterday?.call_count)}`
    },
    {
      label: "今日费用",
      icon: "payments",
      iconClass: "overview-stat-card__icon-wrap--accent",
      value: fmtMoney(today?.total_cost_micro_usd),
      delta: fmtDelta(today?.total_cost_micro_usd, yesterday?.total_cost_micro_usd),
      deltaClass: (today?.total_cost_micro_usd ?? 0) >= (yesterday?.total_cost_micro_usd ?? 0) ? "overview-stat-card__delta--up" : "overview-stat-card__delta--down",
      caption: `本月 ${fmtMoney(month?.total_cost_micro_usd)}`
    },
    {
      label: "今日 Token",
      icon: "data_usage",
      value: fmtCount(today?.total_tokens),
      caption: `输入 ${fmtCount(today?.input_tokens)} / 输出 ${fmtCount(today?.output_tokens)}`
    }
  ];

  if (dash && dash.configured_count > 0) {
    const util = dash.max_utilization_ratio ?? 0;
    const pct = Math.round(util * 100);
    let fillClass: string;
    if (pct >= 90) fillClass = "overview-stat-card__bar-fill--danger";
    else if (pct >= 70) fillClass = "overview-stat-card__bar-fill--warn";
    else fillClass = "overview-stat-card__bar-fill--ok";

    let forecast: string | undefined;
    let forecastClass = "";
    if (month && dash.total_cap_micro_usd > 0) {
      const now = new Date();
      const daysInMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate();
      const dayOfMonth = now.getDate();
      if (dayOfMonth > 0 && daysInMonth > 0) {
        const dailyAvg = (month.total_cost_micro_usd ?? 0) / dayOfMonth;
        const f = (dailyAvg * daysInMonth) / 1_000_000;
        forecast = `预计月底 $${f.toFixed(2)}`;
        if (dailyAvg * daysInMonth > dash.total_cap_micro_usd) forecastClass = "overview-stat-card__forecast--danger";
      }
    }

    result.push({
      label: "月预算使用率",
      icon: "account_balance_wallet",
      iconClass: pct >= 90 ? "overview-stat-card__icon-wrap--danger" : pct >= 70 ? "overview-stat-card__icon-wrap--warn" : undefined,
      value: `${pct}%`,
      caption: `${dash.configured_count} 个 Agent · 已用 $${((dash.total_spent_micro_usd ?? 0) / 1_000_000).toFixed(2)} / $${((dash.total_cap_micro_usd ?? 0) / 1_000_000).toFixed(2)}`,
      bar: { width: `${Math.min(pct, 100)}%`, fillClass },
      forecast,
      forecastClass
    });
  }

  return result;
});

function fmtCount(v?: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(v ?? 0);
}
function fmtMoney(v?: number) {
  return formatUsdFromMicro(v);
}
function fmtDelta(cur?: number, prev?: number) {
  const p = prev ?? 0;
  const n = cur ?? 0;
  if (p === 0) return n > 0 ? "+100%" : "0%";
  const d = ((n - p) / p) * 100;
  return `${d >= 0 ? "+" : ""}${d.toFixed(1)}%`;
}
</script>