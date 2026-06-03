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
        <span class="overview-stat-card__value" :class="card.valueClass">{{ card.value }}</span>
        <span v-if="card.delta" class="overview-stat-card__delta" :class="card.deltaClass">{{ card.delta }}</span>
      </div>
      <div class="overview-stat-card__bottom">
        <span class="overview-stat-card__bottom-left" :class="card.subClass">{{ card.sub }}</span>
        <span class="overview-stat-card__bottom-right">{{ card.caption }}</span>
      </div>
      <div v-if="card.bar" class="overview-stat-card__bar">
        <div class="overview-stat-card__bar-fill" :class="card.bar.fillClass" :style="{ width: card.bar.width }" />
      </div>
      <div v-if="card.forecast" class="overview-stat-card__forecast" :class="card.forecastClass">
        <q-icon name="trending_up" size="12px" />
        {{ card.forecast }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ModelUsageOverview } from '../../features/usage/types';
import { formatUsdFromMicro, formatCount } from '../../features/usage/moneyFormat';

const { t } = useI18n();

const props = defineProps<{
  overview: ModelUsageOverview | null;
}>();

interface StatCard {
  label: string;
  icon: string;
  iconClass?: string;
  value: string;
  valueClass?: string;
  delta?: string;
  deltaClass?: string;
  sub?: string;
  subClass?: string;
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
      label: t('overviewPage.metricTodayCalls'),
      icon: 'phone_in_talk',
      value: fmtCount(today?.call_count),
      valueClass: 'overview-stat-card__value--tech-blue',
      delta: fmtDelta(today?.call_count, yesterday?.call_count),
      deltaClass:
        (today?.call_count ?? 0) >= (yesterday?.call_count ?? 0)
          ? 'overview-stat-card__delta--up'
          : 'overview-stat-card__delta--down',
      sub: fmtPct(today?.success_rate),
      subClass: 'overview-stat-card__bottom-left--tech',
      caption: `${t('overviewPage.metricYesterday')} ${fmtCount(yesterday?.call_count)}`,
    },
    {
      label: t('overviewPage.metricTodayCost'),
      icon: 'payments',
      iconClass: 'overview-stat-card__icon-wrap--accent',
      value: fmtMoney(today?.total_cost_micro_usd),
      valueClass: 'overview-stat-card__value--tech-amber',
      delta: fmtDelta(today?.total_cost_micro_usd, yesterday?.total_cost_micro_usd),
      deltaClass:
        (today?.total_cost_micro_usd ?? 0) >= (yesterday?.total_cost_micro_usd ?? 0)
          ? 'overview-stat-card__delta--up'
          : 'overview-stat-card__delta--down',
      sub: `${t('overviewPage.metricMonthCost')} ${fmtMoney(month?.total_cost_micro_usd)}`,
      subClass: '',
      caption: `${t('overviewPage.metricAvgPerCall')} ${fmtMoney(today?.call_count ? (today.total_cost_micro_usd ?? 0) / today.call_count : 0)}`,
    },
    {
      label: t('overviewPage.metricTodayTokens'),
      icon: 'data_usage',
      value: fmtCount(today?.total_tokens),
      valueClass: 'overview-stat-card__value--tech-cyan',
      delta: fmtDelta(today?.total_tokens, yesterday?.total_tokens),
      deltaClass:
        (today?.total_tokens ?? 0) >= (yesterday?.total_tokens ?? 0)
          ? 'overview-stat-card__delta--up'
          : 'overview-stat-card__delta--down',
      sub: `${t('overviewPage.metricInput')} ${fmtCount(today?.input_tokens)} / ${t('overviewPage.metricOutput')} ${fmtCount(today?.output_tokens)}`,
      subClass: '',
      caption: `${t('overviewPage.metricAvgSpeed')} ${today?.avg_tokens_per_second ? today.avg_tokens_per_second.toFixed(1) : '—'} tok/s`,
    },
    {
      label: t('overviewPage.metricSuccessRate'),
      icon: 'verified',
      value: fmtPct(today?.success_rate),
      valueClass: 'overview-stat-card__value--tech-green',
      sub: `${t('overviewPage.metricFailed')} ${fmtCount(today?.failed_count)}`,
      subClass: '',
      caption: `${t('overviewPage.metricAvgLatency')} ${today?.avg_latency_ms ? Math.round(today.avg_latency_ms) : '—'}ms`,
    },
  ];

  if (dash && dash.configured_count > 0) {
    const util = dash.max_utilization_ratio ?? 0;
    const pct = Math.round(util * 100);
    let fillClass: string;
    if (pct >= 90) fillClass = 'overview-stat-card__bar-fill--danger';
    else if (pct >= 70) fillClass = 'overview-stat-card__bar-fill--warn';
    else fillClass = 'overview-stat-card__bar-fill--ok';

    let forecast: string | undefined;
    let forecastClass = '';
    if (month && dash.total_cap_micro_usd > 0) {
      const now = new Date();
      const daysInMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate();
      const dayOfMonth = now.getDate();
      if (dayOfMonth > 0 && daysInMonth > 0) {
        const dailyAvg = (month.total_cost_micro_usd ?? 0) / dayOfMonth;
        const f = (dailyAvg * daysInMonth) / 1_000_000;
        forecast = `${t('overviewPage.metricForecast')} $${f.toFixed(2)}`;
        if (dailyAvg * daysInMonth > dash.total_cap_micro_usd) forecastClass = 'overview-stat-card__forecast--danger';
      }
    }

    result.push({
      label: t('overviewPage.metricBudgetUsage'),
      icon: 'account_balance_wallet',
      iconClass:
        pct >= 90
          ? 'overview-stat-card__icon-wrap--danger'
          : pct >= 70
            ? 'overview-stat-card__icon-wrap--warn'
            : undefined,
      value: `${pct}%`,
      valueClass:
        pct >= 90
          ? 'overview-stat-card__value--tech-red'
          : pct >= 70
            ? 'overview-stat-card__value--tech-amber'
            : 'overview-stat-card__value--tech-green',
      caption: `${dash.configured_count} ${t('overviewPage.metricAgents')} · $${((dash.total_spent_micro_usd ?? 0) / 1_000_000).toFixed(2)} / $${((dash.total_cap_micro_usd ?? 0) / 1_000_000).toFixed(2)}`,
      bar: { width: `${Math.min(pct, 100)}%`, fillClass },
      forecast,
      forecastClass,
    });
  }

  return result;
});

function fmtCount(v?: number) {
  return formatCount(v);
}
function fmtMoney(v?: number) {
  return formatUsdFromMicro(v);
}
function fmtPct(v?: number) {
  if (v == null) return '—';
  return `${(v * 100).toFixed(1)}%`;
}
function fmtDelta(cur?: number, prev?: number) {
  const p = prev ?? 0;
  const n = cur ?? 0;
  if (p === 0 && n === 0) return '0%';
  if (p === 0) return t('overviewPage.metricNew');
  const d = ((n - p) / p) * 100;
  return `${d >= 0 ? '+' : ''}${d.toFixed(1)}%`;
}
</script>
