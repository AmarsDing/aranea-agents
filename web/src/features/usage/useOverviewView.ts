/**
 * 概览页 view-model：把 useOverviewPage 的窗口数据 + useAttentionBoard 的注意力队列
 * 组装为 7 分区组件的展示绑定（裁决横幅 / RED 瓷贴 / 预算 / 趋势 / 页脚）。
 *
 * 约束：Page 只做路由下钻与模板拼装，所有口径换算、tone 判定、文案都收在本层；
 * Page script 保持 ≤200 行（前端分层铁律），派生逻辑一律下沉到本 composable。
 */
import { computed, onUnmounted, ref, type ComputedRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatCount, formatLatencyMs, formatUsdCompact, formatUsdFromMicro } from './moneyFormat';
import { fmtClock } from '../../components/overview/ovTime';
import type { OvBudget, OvCacheHit, OvRedTile, OvTone, OvVerdictChip } from '../../components/overview/ovTypes';
import type { AttentionBoard } from './types';
import type { useOverviewPage } from './useOverviewPage';

type OverviewPageBindings = ReturnType<typeof useOverviewPage>;

/** 数据源最新更新超过该阈值 → 横幅 LIVE 转为「可能滞后」。 */
const STALE_THRESHOLD_MS = 120_000;
const STALE_TICK_MS = 30_000;

function rateTone(rate: number): OvTone {
  if (rate >= 0.99) return 'ok';
  if (rate >= 0.95) return 'warn';
  return 'danger';
}

function pct1(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}

const LANE_META: Record<string, { icon: string; key: string }> = {
  alerts: { icon: 'notifications_active', key: 'laneAlerts' },
  failed_runs: { icon: 'error_outline', key: 'laneFailedRuns' },
  call_errors: { icon: 'bolt', key: 'laneCallErrors' },
  backlog: { icon: 'inventory_2', key: 'laneBacklog' },
};

export function useOverviewView(page: OverviewPageBindings, board: ComputedRef<AttentionBoard>) {
  const { t } = useI18n();

  // —— ① 裁决横幅 ——
  const verdict = computed(() => board.value.worstSeverity);
  const totalAttention = computed(() => Object.values(board.value.counts).reduce((a, b) => a + b, 0));
  const headline = computed(() =>
    t(
      verdict.value === 'ok'
        ? 'overviewPage.v2.verdictOk'
        : verdict.value === 'critical'
          ? 'overviewPage.v2.verdictCritical'
          : 'overviewPage.v2.verdictWarn',
    ),
  );
  const subline = computed(() =>
    verdict.value === 'ok'
      ? t('overviewPage.v2.verdictOkSub')
      : t('overviewPage.v2.verdictSub', { count: totalAttention.value }),
  );
  const chips = computed<OvVerdictChip[]>(() =>
    board.value.lanes.map((lane) => {
      const m = LANE_META[lane.lane] ?? { icon: 'info', key: 'laneAlerts' };
      const severity: OvVerdictChip['severity'] = lane.items.some((i) => i.severity === 'critical')
        ? 'critical'
        : lane.count > 0
          ? 'warning'
          : 'ok';
      return { key: lane.lane, icon: m.icon, label: `${t(`overviewPage.v2.${m.key}`)} ${lane.count}`, severity };
    }),
  );
  const updatedAtClock = computed(() => fmtClock(board.value.updatedAt || page.usageUpdatedAt.value));

  /** 数据滞后检测：ticker 驱动的 wall-clock 比较（updatedAt 停更时 stale 才会翻转）。 */
  const nowTick = ref(Date.now());
  const nowTicker = setInterval(() => {
    nowTick.value = Date.now();
  }, STALE_TICK_MS);
  onUnmounted(() => clearInterval(nowTicker));
  const latestUpdateMs = computed(() => {
    const b = Date.parse(board.value.updatedAt || '');
    const u = Date.parse(page.usageUpdatedAt.value || '');
    const bm = Number.isFinite(b) ? b : 0;
    const um = Number.isFinite(u) ? u : 0;
    return Math.max(bm, um);
  });
  const stale = computed(() => latestUpdateMs.value > 0 && nowTick.value - latestUpdateMs.value > STALE_THRESHOLD_MS);

  // —— ③ RED 组 ——
  const modelTiles = computed<OvRedTile[]>(() => {
    const s = page.rangeSummary.value;
    const calls = s?.call_count ?? 0;
    return [
      {
        id: 'calls',
        label: t('overviewPage.v2.metricCalls'),
        value: formatCount(calls),
        caption: t(
          page.trendGranularity.value === 'hour'
            ? 'overviewPage.v2.capCallsSparkHour'
            : 'overviewPage.v2.capCallsSparkDay',
        ),
        delta: page.callDelta.value,
        spark: page.callSparkline.value,
        clickable: true,
      },
      {
        id: 'callSuccessRate',
        label: t('overviewPage.v2.metricCallSuccessRate'),
        value: calls > 0 ? pct1(s!.success_rate) : '—',
        caption:
          calls > 0
            ? t('overviewPage.v2.yesterdayRate', { value: pct1(page.yesterdaySummary.value?.success_rate ?? 0) })
            : t('overviewPage.v2.noCalls'),
        tone: calls > 0 ? rateTone(s!.success_rate) : 'neutral',
        clickable: true,
      },
      {
        id: 'burnRate',
        label: t('overviewPage.v2.metricBurnRate'),
        value: page.burnRatePerHour.value != null ? `${formatUsdCompact(page.burnRatePerHour.value)}/h` : '—',
        caption: t('overviewPage.v2.burnRateCap'),
        tone: 'info',
        clickable: true,
      },
    ];
  });
  const runTiles = computed<OvRedTile[]>(() => {
    const rm = page.runnerMetrics.value;
    const total = rm?.total_runs ?? 0;
    return [
      {
        id: 'runCount',
        label: t('overviewPage.v2.metricRunCount'),
        value: formatCount(total),
        caption: t('overviewPage.v2.metricRunCountCap'),
      },
      {
        id: 'runPassRate',
        label: t('overviewPage.v2.metricRunPassRate'),
        value: total > 0 ? pct1(rm!.success_rate) : '—',
        caption:
          total > 0 ? t('overviewPage.v2.metricRunErrors', { count: rm!.error_runs }) : t('overviewPage.v2.noRuns'),
        tone: total > 0 ? rateTone(rm!.success_rate) : 'neutral',
      },
      {
        id: 'p50',
        label: t('overviewPage.v2.metricP50'),
        value: total > 0 ? formatLatencyMs(rm?.p50_duration_ms) : '—',
        tone: 'info',
      },
      {
        id: 'p95',
        label: t('overviewPage.v2.metricP95'),
        value: total > 0 ? formatLatencyMs(rm?.p95_duration_ms) : '—',
        tone: 'info',
      },
    ];
  });

  // —— ④ 成本与预算 ——
  const costValueUsd = computed(() =>
    formatUsdCompact((page.rangeSummary.value?.total_cost_micro_usd ?? 0) / 1_000_000),
  );
  const budget = computed<OvBudget>(() => {
    const q = page.quota.value;
    const configured = (q?.configured_count ?? 0) > 0;
    const pct = q?.max_utilization_ratio ?? 0;
    const forecast = page.monthEndForecastUsd.value;
    return {
      configured,
      pct,
      spentUsd: formatUsdFromMicro(q?.total_spent_micro_usd),
      capUsd: formatUsdFromMicro(q?.total_cap_micro_usd),
      forecastUsd: forecast != null ? formatUsdCompact(forecast) : null,
      lowConfidence: new Date().getDate() < 5,
      tone: pct >= 0.95 ? 'danger' : pct >= 0.8 ? 'warn' : 'ok',
    };
  });
  const cacheView = computed<OvCacheHit>(() => {
    const s = page.cacheHitSummary.value;
    return { denied: page.cacheHitDenied.value, samples: s.samples, p50: s.p50, low: s.low };
  });

  // —— ⑤ 趋势 ——
  const trendPoints = computed(() => page.overview.value?.trends ?? []);
  const trendLabels = computed(() =>
    trendPoints.value.map((p) =>
      page.trendGranularity.value === 'hour' ? p.date_key.slice(11, 16) || p.date_key : p.date_key.slice(5, 10),
    ),
  );
  const trendCalls = computed(() => trendPoints.value.map((p) => p.call_count ?? 0));
  const trendFailures = computed(() => trendPoints.value.map((p) => p.failed_count ?? 0));
  const trendCosts = computed(() => trendPoints.value.map((p) => (p.total_cost_micro_usd ?? 0) / 1_000_000));

  // —— ⑥ 组织与编制 ——
  const worstAgentRows = computed(() =>
    page.worstAgents.value.map((a) => ({
      key: a.agent_key || a.agent_id,
      name: a.agent_key || a.agent_id,
      calls: a.call_count ?? 0,
      successRate: a.success_rate ?? 1,
    })),
  );

  // —— ⑦ 新鲜度页脚 ——
  const footSources = computed(() => [
    { label: t('overviewPage.v2.srcAttention'), time: fmtClock(board.value.updatedAt) },
    { label: t('overviewPage.v2.srcUsage'), time: fmtClock(page.usageUpdatedAt.value) },
  ]);
  const footNotes = computed(() => [
    t('overviewPage.v2.noteUtc'),
    t('overviewPage.v2.noteProvider'),
    ...(page.unpricedRows.value.length ? [t('overviewPage.v2.noteUnpriced')] : []),
  ]);

  return {
    verdict,
    headline,
    subline,
    chips,
    updatedAtClock,
    stale,
    modelTiles,
    runTiles,
    costValueUsd,
    budget,
    cacheView,
    trendLabels,
    trendCalls,
    trendFailures,
    trendCosts,
    worstAgentRows,
    footSources,
    footNotes,
  };
}
