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
import type { ModelUsageOverview, ModelUsageSummary } from '../../features/usage/types';
import { formatUsdFromMicro, formatCount } from '../../features/usage/moneyFormat';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    overview: ModelUsageOverview | null;
    /** 当前筛选的时间范围。today 使用 overview.today + yesterday 环比；其他值使用 overview.range + today 参考。 */
    range?: string;
  }>(),
  {
    range: 'today',
  },
);

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

/** 根据 range 返回 i18n 标签中的范围文本（今日 / 7 天 / 30 天 / 本月）。 */
const rangeLabel = computed(() => {
  switch (props.range) {
    case '7d':
      return t('overviewPage.range7d');
    case '30d':
      return t('overviewPage.range30d');
    case 'month':
      return t('overviewPage.rangeMonth');
    case 'today':
    default:
      return t('overviewPage.rangeToday');
  }
});

/** range==='today' 时主数据为 today，对比基准为 yesterday；否则主数据为 range，参考为 today。 */
const isToday = computed(() => props.range === 'today');

const primary = computed<ModelUsageSummary | undefined>(() =>
  isToday.value ? props.overview?.today : props.overview?.range,
);
const reference = computed<ModelUsageSummary | undefined>(() =>
  isToday.value ? props.overview?.yesterday : props.overview?.today,
);
const month = computed<ModelUsageSummary | undefined>(() => props.overview?.month);
const dash = computed(() => props.overview?.quota_dashboard);

const cards = computed<StatCard[]>(() => {
  const cur = primary.value;
  const ref = reference.value;
  const mon = month.value;

  const callsLabel = isToday.value
    ? t('overviewPage.metricTodayCalls')
    : t('overviewPage.metricRangeCalls', { range: rangeLabel.value });
  const costLabel = isToday.value
    ? t('overviewPage.metricTodayCost')
    : t('overviewPage.metricRangeCost', { range: rangeLabel.value });
  const tokensLabel = isToday.value
    ? t('overviewPage.metricTodayTokens')
    : t('overviewPage.metricRangeTokens', { range: rangeLabel.value });

  // range≠today 时不展示环比 delta（后端暂无 prev_range_summary），改为显示 today 占比 caption。
  const showDelta = isToday.value;

  const result: StatCard[] = [
    {
      label: callsLabel,
      icon: 'phone_in_talk',
      value: fmtCount(cur?.call_count),
      valueClass: 'overview-stat-card__value--tech-blue',
      delta: showDelta ? fmtDelta(cur?.call_count, ref?.call_count) : undefined,
      deltaClass:
        (cur?.call_count ?? 0) >= (ref?.call_count ?? 0)
          ? 'overview-stat-card__delta--up'
          : 'overview-stat-card__delta--down',
      sub: fmtPct(cur?.success_rate),
      subClass: 'overview-stat-card__bottom-left--tech',
      caption: refCaption(ref?.call_count, t('overviewPage.metricTodayRef')),
    },
    {
      label: costLabel,
      icon: 'payments',
      iconClass: 'overview-stat-card__icon-wrap--accent',
      value: fmtMoney(cur?.total_cost_micro_usd),
      valueClass: 'overview-stat-card__value--tech-amber',
      delta: showDelta ? fmtDelta(cur?.total_cost_micro_usd, ref?.total_cost_micro_usd) : undefined,
      deltaClass:
        (cur?.total_cost_micro_usd ?? 0) >= (ref?.total_cost_micro_usd ?? 0)
          ? 'overview-stat-card__delta--up'
          : 'overview-stat-card__delta--down',
      sub: `${t('overviewPage.metricMonthCost')} ${fmtMoney(mon?.total_cost_micro_usd)}`,
      subClass: '',
      caption: `${t('overviewPage.metricAvgPerCall')} ${fmtMoney(cur?.call_count ? (cur.total_cost_micro_usd ?? 0) / cur.call_count : 0)}`,
    },
    {
      label: tokensLabel,
      icon: 'data_usage',
      value: fmtCount(cur?.total_tokens),
      valueClass: 'overview-stat-card__value--tech-cyan',
      delta: showDelta ? fmtDelta(cur?.total_tokens, ref?.total_tokens) : undefined,
      deltaClass:
        (cur?.total_tokens ?? 0) >= (ref?.total_tokens ?? 0)
          ? 'overview-stat-card__delta--up'
          : 'overview-stat-card__delta--down',
      sub: `${t('overviewPage.metricInput')} ${fmtCount(cur?.input_tokens)} / ${t('overviewPage.metricOutput')} ${fmtCount(cur?.output_tokens)}`,
      subClass: '',
      caption: `${t('overviewPage.metricAvgSpeed')} ${cur?.avg_tokens_per_second ? cur.avg_tokens_per_second.toFixed(1) : '—'} tok/s`,
    },
    {
      label: t('overviewPage.metricSuccessRate'),
      icon: 'verified',
      value: fmtPct(cur?.success_rate),
      valueClass: 'overview-stat-card__value--tech-green',
      sub: `${t('overviewPage.metricFailed')} ${fmtCount(cur?.failed_count)}`,
      subClass: '',
      caption: `${t('overviewPage.metricAvgLatency')} ${cur?.avg_latency_ms ? Math.round(cur.avg_latency_ms) : '—'}ms`,
    },
  ];

  if (dash.value && dash.value.configured_count > 0) {
    const util = dash.value.max_utilization_ratio ?? 0;
    const pct = Math.round(util * 100);
    let fillClass: string;
    if (pct >= 90) fillClass = 'overview-stat-card__bar-fill--danger';
    else if (pct >= 70) fillClass = 'overview-stat-card__bar-fill--warn';
    else fillClass = 'overview-stat-card__bar-fill--ok';

    let forecast: string | undefined;
    let forecastClass = '';
    if (mon && dash.value.total_cap_micro_usd > 0) {
      const now = new Date();
      const daysInMonth = new Date(now.getFullYear(), now.getMonth() + 1, 0).getDate();
      const dayOfMonth = now.getDate();
      if (dayOfMonth > 0 && daysInMonth > 0) {
        const dailyAvg = (mon.total_cost_micro_usd ?? 0) / dayOfMonth;
        const f = (dailyAvg * daysInMonth) / 1_000_000;
        forecast = `${t('overviewPage.metricForecast')} $${f.toFixed(2)}`;
        if (dailyAvg * daysInMonth > dash.value.total_cap_micro_usd)
          forecastClass = 'overview-stat-card__forecast--danger';
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
      caption: `${dash.value.configured_count} ${t('overviewPage.metricAgents')} · $${((dash.value.total_spent_micro_usd ?? 0) / 1_000_000).toFixed(2)} / $${((dash.value.total_cap_micro_usd ?? 0) / 1_000_000).toFixed(2)}`,
      bar: { width: `${Math.min(pct, 100)}%`, fillClass },
      forecast,
      forecastClass,
    });
  }

  return result;
});

/** range=today 时 caption 显示"昨日 N"；range≠today 时 caption 显示"今日 N"。 */
function refCaption(value: number | undefined, label: string) {
  return `${label} ${fmtCount(value)}`;
}

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
